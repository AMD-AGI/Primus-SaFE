/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package failover

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/syncer"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
)

type FailoverReconciler struct {
	client.Client
	failoverConfig   *commonutils.ObjectManager
	clusterInformers *commonutils.ObjectManager
}

func SetupFailoverController(mgr manager.Manager) error {
	r := &FailoverReconciler{
		Client:           mgr.GetClient(),
		failoverConfig:   commonutils.NewObjectManager(),
		clusterInformers: commonutils.NewObjectManagerSingleton(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(caredChangePredicate{})).
		Watches(&v1.Fault{}, r.handleFaultEvent()).
		Watches(&corev1.ConfigMap{}, r.updateConfig()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Failover Controller successfully")
	return nil
}

type caredChangePredicate struct {
	predicate.Funcs
}

func (caredChangePredicate) Create(e event.CreateEvent) bool {
	w, ok := e.Object.(*v1.Workload)
	if !ok {
		return false
	}
	if isNeedFailover(w) {
		return true
	}
	return false
}

func (caredChangePredicate) Update(e event.UpdateEvent) bool {
	oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
	newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
	if !ok1 || !ok2 {
		return false
	}
	if !isNeedFailover(oldWorkload) && isNeedFailover(newWorkload) {
		return true
	}
	return false
}

func isNeedFailover(workload *v1.Workload) bool {
	if isDisableFailover(workload) {
		return false
	}
	if v1.IsWorkloadPreempted(workload) {
		return true
	}
	cond := &metav1.Condition{
		Type:   string(v1.K8sFailed),
		Reason: commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload)),
	}
	if jobutils.FindCondition(workload, cond) != nil {
		return true
	}
	return false
}

func isDisableFailover(w *v1.Workload) bool {
	if v1.IsWorkloadDisableFailover(w) {
		return true
	}
	if !v1.IsWorkloadDispatched(w) || w.IsEnd() {
		return true
	}
	if v1.IsWorkloadPreempted(w) {
		return false
	}
	if w.Spec.MaxRetry <= 0 || v1.GetWorkloadDispatchCnt(w) > w.Spec.MaxRetry {
		return true
	}
	return false
}

func (r *FailoverReconciler) handleFaultEvent() handler.EventHandler {
	check := func(fault *v1.Fault) bool {
		if fault.Status.Phase != v1.FaultPhaseSucceeded || fault.Spec.Node == nil {
			return false
		}
		conf := getFailoverConfig(r.failoverConfig, strings.ToLower(fault.Spec.Id))
		if conf == nil || conf.Action != GLOBAL_RESTART {
			return false
		}
		return true
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			fault, ok := evt.Object.(*v1.Fault)
			if !ok || !check(fault) {
				return
			}
			r.handleFaultEventImpl(ctx, fault, q)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			fault, ok := evt.ObjectNew.(*v1.Fault)
			if !ok || !check(fault) {
				return
			}
			r.handleFaultEventImpl(ctx, fault, q)
		},
	}
}

func (r *FailoverReconciler) handleFaultEventImpl(ctx context.Context, fault *v1.Fault, q v1.RequestWorkQueue) {
	message := fmt.Sprintf("the node %s has fault %s, detail: %s", fault.Spec.Node.K8sName,
		commonfaults.GenerateTaintKey(fault.Spec.Id), fault.Spec.Message)
	klog.Infof("%s, try to do failover", message)
	const maxRetry = 10
	waitTime := time.Millisecond * 200
	maxWaitTime := waitTime * maxRetry

	var clusterInformer *syncer.ClusterInformer
	clusterName := fault.Spec.Node.ClusterName
	err := backoff.Retry(func() error {
		clusterInformer, _ = syncer.GetClusterInformer(r.clusterInformers, clusterName)
		if clusterInformer != nil {
			return nil
		}
		return fmt.Errorf("failed to get cluster informer")
	}, maxWaitTime, waitTime)
	if err != nil || clusterInformer == nil {
		return
	}

	workloadNames, err := r.getWorkloadsByFault(ctx, clusterInformer, fault)
	if err != nil {
		return
	}

	f := func(name string) bool {
		for i := 0; i < maxRetry; i++ {
			workload := &v1.Workload{}
			if err = r.Get(ctx, client.ObjectKey{Name: name}, workload); err != nil {
				if apierrors.IsNotFound(err) {
					return false
				}
			} else if isDisableFailover(workload) ||
				workload.CreationTimestamp.After(fault.CreationTimestamp.Time) {
				return false
			} else if r.addFailoverCondition(ctx, workload, message) == nil {
				break
			}
			time.Sleep(waitTime)
		}
		return true
	}
	for _, name := range workloadNames {
		if f(name) {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
		}
	}
}

func (r *FailoverReconciler) addFailoverCondition(ctx context.Context, workload *v1.Workload, message string) error {
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload))
	cond := jobutils.NewCondition(string(v1.AdminFailover), message, reason)
	if jobutils.FindCondition(workload, cond) != nil {
		return nil
	}
	workload.Status.Conditions = append(workload.Status.Conditions, *cond)
	if err := r.Status().Update(ctx, workload); err != nil {
		klog.ErrorS(err, "failed to update workload status")
		return err
	}
	return nil
}

func (r *FailoverReconciler) getWorkloadsByFault(ctx context.Context,
	clusterInformer *syncer.ClusterInformer, fault *v1.Fault) ([]string, error) {
	adminNode := &v1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: fault.Spec.Node.AdminName}, adminNode); err != nil {
		klog.ErrorS(err, "failed to get node", "name", fault.Spec.Node.AdminName)
		return nil, err
	}

	workloadNames, err := commonworkload.GetWorkloadsOfK8sNode(ctx,
		clusterInformer.ClientFactory().ClientSet(), fault.Spec.Node.K8sName, v1.GetWorkspaceId(adminNode))
	if err != nil {
		klog.ErrorS(err, "failed to get workload of node",
			"name", fault.Spec.Node.K8sName, "workspace", v1.GetWorkspaceId(adminNode))
		return nil, err
	}
	return workloadNames, nil
}

func (r *FailoverReconciler) updateConfig() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			cm, ok := evt.Object.(*corev1.ConfigMap)
			if !ok {
				return
			}
			if cm.GetName() != FailoverConfigmapName || cm.GetNamespace() != common.PrimusSafeNamespace {
				return
			}
			addFailoverConfig(cm, r.failoverConfig)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldCm, ok1 := evt.ObjectOld.(*corev1.ConfigMap)
			newCm, ok2 := evt.ObjectNew.(*corev1.ConfigMap)
			if !ok1 || !ok2 {
				return
			}
			if oldCm.GetName() != FailoverConfigmapName || oldCm.GetNamespace() != common.PrimusSafeNamespace {
				return
			}
			if newCm.GetName() != FailoverConfigmapName || newCm.GetNamespace() != common.PrimusSafeNamespace {
				return
			}
			if reflect.DeepEqual(oldCm.Data, newCm.Data) {
				return
			}
			addFailoverConfig(newCm, r.failoverConfig)
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			cm, ok := evt.Object.(*corev1.ConfigMap)
			if !ok {
				return
			}
			if cm.GetName() != FailoverConfigmapName || cm.GetNamespace() != common.PrimusSafeNamespace {
				return
			}
			r.failoverConfig.Clear()
		},
	}
}

func (r *FailoverReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	workload := new(v1.Workload)
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to get admin workload")
		} else {
			err = nil
		}
		return ctrlruntime.Result{}, err
	}
	if !workload.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, nil
	}
	return r.handle(ctx, workload)
}

func (r *FailoverReconciler) handle(ctx context.Context, adminWorkload *v1.Workload) (ctrlruntime.Result, error) {
	clusterInformer, _ := syncer.GetClusterInformer(r.clusterInformers, v1.GetClusterId(adminWorkload))
	if clusterInformer == nil {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	obj, err := jobutils.GenUnstructuredByWorkload(ctx, r.Client, adminWorkload)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = jobutils.DeleteObject(ctx, clusterInformer.ClientFactory(), obj); err != nil {
		klog.ErrorS(err, "failed to delete k8s object", "name", adminWorkload.GetName())
		return ctrlruntime.Result{}, err
	}
	message := ""
	if v1.IsWorkloadPreempted(adminWorkload) {
		message = "the workload is preempted"
	} else {
		message = "the workload does the failover"
	}
	if err = r.addFailoverCondition(ctx, adminWorkload, message); err != nil {
		return ctrlruntime.Result{}, err
	}
	klog.Infof("do failover, workload: %s", adminWorkload.Name)
	return ctrlruntime.Result{}, nil
}
