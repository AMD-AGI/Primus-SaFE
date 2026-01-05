/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	apitypes "k8s.io/apimachinery/pkg/types"
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
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	MaxRetryAttempts = 10
	RetryWaitTime    = time.Millisecond * 200
)

// FailoverReconciler reconciles Workload objects for failover handling
type FailoverReconciler struct {
	client.Client
	failoverConfigs  *commonutils.ObjectManager
	clusterInformers *commonutils.ObjectManager
}

// SetupFailoverController initializes and registers the failover controller with the manager.
func SetupFailoverController(mgr manager.Manager) error {
	r := &FailoverReconciler{
		Client:           mgr.GetClient(),
		failoverConfigs:  commonutils.NewObjectManager(),
		clusterInformers: commonutils.NewObjectManagerSingleton(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Workload{}, builder.WithPredicates(relevantChangePredicate{})).
		Watches(&v1.Fault{}, r.handleFaultEvent()).
		Watches(&corev1.ConfigMap{}, r.handleConfigmapEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Failover Controller successfully")
	return nil
}

type relevantChangePredicate struct {
	predicate.Funcs
}

// Create determines if a Create event should be processed for a Workload.
func (relevantChangePredicate) Create(e event.CreateEvent) bool {
	w, ok := e.Object.(*v1.Workload)
	if !ok {
		return false
	}
	if isFailoverNeeded(w) {
		return true
	}
	return false
}

// Update updates the specified resource.
func (relevantChangePredicate) Update(e event.UpdateEvent) bool {
	oldWorkload, ok1 := e.ObjectOld.(*v1.Workload)
	newWorkload, ok2 := e.ObjectNew.(*v1.Workload)
	if !ok1 || !ok2 {
		return false
	}
	if !isFailoverNeeded(oldWorkload) && isFailoverNeeded(newWorkload) {
		return true
	}
	return false
}

// isFailoverNeeded checks if a workload needs failover handling.
func isFailoverNeeded(workload *v1.Workload) bool {
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

// isDisableFailover checks if failover is disabled for a workload.
func isDisableFailover(workload *v1.Workload) bool {
	if v1.IsWorkloadDisableFailover(workload) {
		return true
	}
	if !v1.IsWorkloadDispatched(workload) || workload.IsEnd() {
		return true
	}
	// Preemption is not subject to retry count limits.
	if v1.IsWorkloadPreempted(workload) {
		return false
	}
	if workload.Spec.MaxRetry <= 0 || v1.GetWorkloadDispatchCnt(workload) > workload.Spec.MaxRetry {
		return true
	}
	return false
}

// handleFaultEvent creates an event handler for Fault resources.
func (r *FailoverReconciler) handleFaultEvent() handler.EventHandler {
	check := func(fault *v1.Fault) bool {
		if fault.Status.Phase != v1.FaultPhaseSucceeded || fault.Spec.Node == nil {
			return false
		}
		return isMonitorIdExists(r.failoverConfigs, strings.ToLower(fault.Spec.MonitorId))
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

// handleFaultEventImpl handles the actual processing of a Fault event.
func (r *FailoverReconciler) handleFaultEventImpl(ctx context.Context, fault *v1.Fault, q v1.RequestWorkQueue) {
	message := fmt.Sprintf("the node %s has fault %s, detail: %s", fault.Spec.Node.K8sName,
		commonfaults.GenerateTaintKey(fault.Spec.MonitorId), fault.Spec.Message)
	klog.Infof("%s, try to do failover", message)

	clusterInformer := r.getClusterInformer(fault.Spec.Node.ClusterName)
	if clusterInformer == nil {
		return
	}
	workloadNames, err := r.getWorkloadsOnFaultNode(ctx, clusterInformer, fault)
	if err != nil {
		return
	}

	f := func(name string) bool {
		for i := 0; i < MaxRetryAttempts; i++ {
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
			time.Sleep(RetryWaitTime)
		}
		return true
	}
	for _, name := range workloadNames {
		if f(name) {
			q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Name: name}})
		}
	}
}

// getClusterInformer retrieves the cluster informer for a given fault's cluster with retry logic.
func (r *FailoverReconciler) getClusterInformer(clusterId string) *syncer.ClusterInformer {
	maxWaitTime := RetryWaitTime * MaxRetryAttempts
	var clusterInformer *syncer.ClusterInformer
	err := backoff.Retry(func() error {
		clusterInformer, _ = syncer.GetClusterInformer(r.clusterInformers, clusterId)
		if clusterInformer != nil {
			return nil
		}
		return fmt.Errorf("failed to get cluster's informer")
	}, maxWaitTime, RetryWaitTime)
	if err != nil || clusterInformer == nil {
		return nil
	}
	return clusterInformer
}

// addFailoverCondition adds a failover condition to a workload's status.
func (r *FailoverReconciler) addFailoverCondition(ctx context.Context, workload *v1.Workload, message string) error {
	reason := commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload))
	cond := jobutils.NewCondition(string(v1.AdminFailover), message, reason)
	if jobutils.FindCondition(workload, cond) != nil {
		return nil
	}

	workload.Status.Conditions = append(workload.Status.Conditions, *cond)
	patchObj := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": workload.ResourceVersion,
		},
		"status": map[string]any{
			"conditions": workload.Status.Conditions,
		},
	}
	p := jsonutils.MarshalSilently(patchObj)
	if err := r.Status().Patch(ctx, workload, client.RawPatch(apitypes.MergePatchType, p)); err != nil {
		klog.ErrorS(err, "failed to patch workload status")
		return err
	}
	return nil
}

// getWorkloadsOnFaultNode retrieves workloads running on a faulty node.
func (r *FailoverReconciler) getWorkloadsOnFaultNode(ctx context.Context,
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

// handleConfigmapEvent creates an event handler for ConfigMap resources.
func (r *FailoverReconciler) handleConfigmapEvent() handler.EventHandler {
	isFailoverConfigMap := func(cm *corev1.ConfigMap) bool {
		return cm.GetName() == common.PrimusFailover && cm.GetNamespace() == common.PrimusSafeNamespace
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			cm, ok := evt.Object.(*corev1.ConfigMap)
			if !ok {
				return
			}
			if !isFailoverConfigMap(cm) {
				return
			}
			addFailoverConfig(cm, r.failoverConfigs)
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldCm, ok1 := evt.ObjectOld.(*corev1.ConfigMap)
			newCm, ok2 := evt.ObjectNew.(*corev1.ConfigMap)
			if !ok1 || !ok2 {
				return
			}
			if !isFailoverConfigMap(oldCm) || !isFailoverConfigMap(newCm) {
				return
			}
			if reflect.DeepEqual(oldCm.Data, newCm.Data) {
				return
			}
			addFailoverConfig(newCm, r.failoverConfigs)
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			cm, ok := evt.Object.(*corev1.ConfigMap)
			if !ok {
				return
			}
			if !isFailoverConfigMap(cm) {
				return
			}
			r.failoverConfigs.Clear()
		},
	}
}

// Reconcile is the main control loop for Workload resources that handles failover.
func (r *FailoverReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	workload := new(v1.Workload)
	if err := r.Get(ctx, req.NamespacedName, workload); err != nil {
		if !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to get admin workload")
			return ctrlruntime.Result{}, err
		}
		return ctrlruntime.Result{}, nil
	}
	if !workload.GetDeletionTimestamp().IsZero() || isDisableFailover(workload) {
		return ctrlruntime.Result{}, nil
	}
	return r.handle(ctx, workload)
}

// handle processes the failover logic for a workload.
func (r *FailoverReconciler) handle(ctx context.Context, workload *v1.Workload) (ctrlruntime.Result, error) {
	clusterInformer, _ := syncer.GetClusterInformer(r.clusterInformers, v1.GetClusterId(workload))
	if clusterInformer == nil {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}
	workloadUnstructured, err := jobutils.BuildWorkloadUnstructured(ctx, r.Client, workload)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = jobutils.DeleteObject(ctx, clusterInformer.ClientFactory(), workloadUnstructured); err != nil {
		klog.ErrorS(err, "failed to delete k8s object", "name", workload.GetName())
		return ctrlruntime.Result{}, err
	}
	message := ""
	if v1.IsWorkloadPreempted(workload) {
		message = "the workload is preempted"
	} else {
		message = "the workload is doing the failover"
	}
	if err = r.addFailoverCondition(ctx, workload, message); err != nil {
		return ctrlruntime.Result{}, err
	}
	klog.Infof("the workload %s is attempting to perform a failover", workload.Name)
	return ctrlruntime.Result{}, nil
}
