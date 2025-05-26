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
		For(&v1.Workload{}, builder.WithPredicates(CaredChangePredicate{})).
		Watches(&v1.Fault{}, r.enqueueRequestByFault()).
		Watches(&corev1.ConfigMap{}, r.enqueueRequestByConfigmap()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Failover Controller successfully")
	return nil
}

type CaredChangePredicate struct {
	predicate.Funcs
}

func (CaredChangePredicate) Create(e event.CreateEvent) bool {
	w, ok := e.Object.(*v1.Workload)
	if !ok {
		return false
	}
	if isNeedFailover(w) {
		return true
	}
	return false
}

func (CaredChangePredicate) Update(e event.UpdateEvent) bool {
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
	if v1.IsWorkloadForcedFailover(workload) {
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
	if w.IsEnd() || v1.IsWorkloadDisableFailover(w) {
		return true
	}
	if !v1.IsWorkloadDispatched(w) {
		return true
	}
	if v1.IsWorkloadForcedFailover(w) {
		return false
	}
	if w.Spec.MaxRetry <= 0 || v1.GetWorkloadDispatchCnt(w) > w.Spec.MaxRetry {
		return true
	}
	return false
}

func (r *FailoverReconciler) enqueueRequestByFault() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			fault, ok := evt.Object.(*v1.Fault)
			if !ok || fault.Status.Phase != v1.FaultPhaseSucceeded {
				return
			}
			conf := getFailoverConfig(r.failoverConfig, strings.ToLower(fault.Spec.Id))
			if conf == nil || conf.Action != GLOBAL_RESTART || fault.Spec.Node == nil {
				return
			}
			message := fmt.Sprintf("the node %s has fault %s, detail: %s", fault.Spec.Node.K8sName,
				commonfaults.GenerateTaintKey(fault.Spec.Id), fault.Spec.Message)
			r.enqueue(ctx, fault, q, message, conf.Force)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldFault, ok1 := evt.ObjectOld.(*v1.Fault)
			newFault, ok2 := evt.ObjectNew.(*v1.Fault)
			if !ok1 || !ok2 {
				return
			}
			if oldFault.Status.Phase == v1.FaultPhaseSucceeded || newFault.Status.Phase != v1.FaultPhaseSucceeded {
				return
			}
			conf := getFailoverConfig(r.failoverConfig, strings.ToLower(newFault.Spec.Id))
			if conf == nil || conf.Action != GLOBAL_RESTART || newFault.Spec.Node == nil {
				return
			}
			message := fmt.Sprintf("the node %s has fault %s, detail: %s", newFault.Spec.Node.K8sName,
				commonfaults.GenerateTaintKey(newFault.Spec.Id), newFault.Spec.Message)
			r.enqueue(ctx, newFault, q, message, conf.Force)
		},
	}
}

func (r *FailoverReconciler) enqueue(ctx context.Context,
	fault *v1.Fault, q v1.RequestWorkQueue, message string, isForce bool) {
	klog.Infof("%s, try to do failover", message)
	waitTime := time.Millisecond * 200
	const maxRetry = 10
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

	faultNode := fault.Spec.Node
	workloadNames, err := r.getAllWorkloadsOfNode(ctx, clusterInformer, faultNode.AdminName, faultNode.K8sName)
	if err != nil {
		return
	}

	f := func(name string) bool {
		for i := 0; i < maxRetry; i++ {
			workload := &v1.Workload{}
			if err = r.Get(context.Background(), client.ObjectKey{Name: name}, workload); err != nil {
				if apierrors.IsNotFound(err) {
					return false
				}
				continue
			}
			if !isForce {
				if isDisableFailover(workload) || workload.CreationTimestamp.After(fault.CreationTimestamp.Time) {
					return false
				}
			} else {
				patch := client.MergeFrom(workload.DeepCopy())
				metav1.SetMetaDataAnnotation(&workload.ObjectMeta, v1.WorkloadForcedFoAnnotation, "")
				if err = r.Patch(context.Background(), workload, patch); err != nil {
					continue
				}
			}
			if r.addFailoverCondition(workload, message) == nil {
				break
			}
		}
		return true
	}
	for _, name := range workloadNames {
		if f(name) {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
		}
	}
}

func (r *FailoverReconciler) addFailoverCondition(workload *v1.Workload, message string) error {
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload))
	cond := jobutils.NewCondition(string(v1.AdminFailover), message, reason)
	if jobutils.FindCondition(workload, cond) != nil {
		return nil
	}
	workload.Status.Conditions = append(workload.Status.Conditions, *cond)
	if err := r.Status().Update(context.Background(), workload); err != nil {
		klog.ErrorS(err, "failed to update workload status")
		return err
	}
	return nil
}

func (r *FailoverReconciler) getAllWorkloadsOfNode(ctx context.Context,
	clusterInformer *syncer.ClusterInformer, adminNodeName, k8sNodeName string) ([]string, error) {
	adminNode := &v1.Node{}
	if err := r.Get(ctx, client.ObjectKey{Name: adminNodeName}, adminNode); err != nil {
		klog.ErrorS(err, "failed to get node", "name", adminNodeName)
		return nil, err
	}

	workloadNames, err := commonworkload.GetWorkloadsOfK8sNode(ctx,
		clusterInformer.ClientFactory().ClientSet(), k8sNodeName, v1.GetWorkspaceId(adminNode))
	if err != nil {
		klog.ErrorS(err, "failed to get workload of node",
			"name", k8sNodeName, "workspace", v1.GetWorkspaceId(adminNode))
		return nil, err
	}
	return workloadNames, nil
}

func (r *FailoverReconciler) enqueueRequestByConfigmap() handler.EventHandler {
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
	if isDisableFailover(workload) {
		return ctrlruntime.Result{}, nil
	}
	return r.handle(ctx, workload)
}

func (r *FailoverReconciler) handle(ctx context.Context, adminWorkload *v1.Workload) (ctrlruntime.Result, error) {
	clusterInformer, _ := syncer.GetClusterInformer(r.clusterInformers, v1.GetClusterId(adminWorkload))
	if clusterInformer == nil {
		return ctrlruntime.Result{RequeueAfter: time.Second * 1}, nil
	}
	clientSet := clusterInformer.ClientFactory()
	if err := jobutils.DeleteObject(ctx, clientSet.DynamicClient(), clientSet.Mapper(), adminWorkload); err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		} else {
			klog.ErrorS(err, "failed to delete k8s workload", "name", adminWorkload.GetName())
		}
		return ctrlruntime.Result{}, err
	}
	if v1.IsWorkloadForcedFailover(adminWorkload) {
		patch := client.MergeFrom(adminWorkload.DeepCopy())
		delete(adminWorkload.Annotations, v1.WorkloadForcedFoAnnotation)
		if err := r.Patch(context.Background(), adminWorkload, patch); err != nil {
			return ctrlruntime.Result{}, err
		}
	}
	message := "the workload does the failover"
	if err := r.addFailoverCondition(adminWorkload, message); err != nil {
		return ctrlruntime.Result{}, err
	}
	klog.Infof("do failover, workload: %s", adminWorkload.Name)
	return ctrlruntime.Result{}, nil
}
