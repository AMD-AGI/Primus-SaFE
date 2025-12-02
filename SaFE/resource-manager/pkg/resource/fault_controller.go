/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"reflect"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// FaultReconciler reconciles Fault resources and manages their lifecycle
type FaultReconciler struct {
	*ClusterBaseReconciler
	opt *FaultReconcilerOption
}

// FaultReconcilerOption holds configuration options for the FaultReconciler
type FaultReconcilerOption struct {
	// Wait time between processing attempts
	processWait time.Duration
	// Maximum number of retry attempts
	maxRetryCount int
}

// SetupFaultController initializes and registers the FaultReconciler with the controller manager.
func SetupFaultController(mgr manager.Manager, opt *FaultReconcilerOption) error {
	baseReconciler, err := newClusterBaseReconciler(mgr)
	if err != nil {
		return err
	}
	r := &FaultReconciler{
		ClusterBaseReconciler: baseReconciler,
		opt:                   opt,
	}
	err = ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Fault{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Watches(&corev1.ConfigMap{}, r.handleConfigmapEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Fault Controller successfully")
	return nil
}

// handleNodeEvent handles node events that may affect fault reconciliation.
func (r *FaultReconciler) handleNodeEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			node, ok := evt.Object.(*v1.Node)
			if !ok {
				return
			}
			labelSelector := labels.SelectorFromSet(map[string]string{v1.NodeIdLabel: node.Name})
			faultList, _ := listFaults(ctx, r.Client, labelSelector)
			for _, f := range faultList {
				if !isValidFault(&f, node) {
					r.Delete(ctx, &f)
				}
			}
		},
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 {
				return
			}
			// delete all faults when unmanaging or deleting node
			if (oldNode.GetSpecCluster() != "" && newNode.GetSpecCluster() == "") ||
				oldNode.GetDeletionTimestamp().IsZero() && !newNode.GetDeletionTimestamp().IsZero() {
				labelSelector := labels.SelectorFromSet(map[string]string{v1.NodeIdLabel: newNode.Name})
				if err := r.deleteFaults(ctx, labelSelector); err != nil {
					klog.ErrorS(err, "failed to delete faults with node", "node", newNode.Name)
				}
			}
			if !reflect.DeepEqual(oldNode.Status.Taints, newNode.Status.Taints) {
				labelSelector := labels.SelectorFromSet(map[string]string{v1.NodeIdLabel: newNode.Name})
				faultList, _ := listFaults(ctx, r.Client, labelSelector)
				for _, f := range faultList {
					q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: f.Name}})
				}
			}
		},
	}
}

// handleConfigmapEvent handles configmap events that may affect fault reconciliation.
func (r *FaultReconciler) handleConfigmapEvent() handler.EventHandler {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q v1.RequestWorkQueue) {
			configmap, ok := e.Object.(*corev1.ConfigMap)
			if !ok || configmap.Name != common.PrimusFault {
				return
			}
			configs := parseFaultConfig(configmap)
			faultList, _ := listFaults(ctx, r.Client, labels.Everything())
			for _, f := range faultList {
				conf, ok := configs[f.Spec.MonitorId]
				if !ok || !conf.IsEnable() {
					if err := r.Delete(ctx, &f); err != nil {
						klog.ErrorS(err, "failed to delete fault")
					}
				}
			}
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q v1.RequestWorkQueue) {
			oldConfigmap, ok1 := e.ObjectOld.(*corev1.ConfigMap)
			newConfigmap, ok2 := e.ObjectNew.(*corev1.ConfigMap)
			if !ok1 || !ok2 {
				return
			}
			if newConfigmap.Name != common.PrimusFault {
				return
			}
			oldConfigs := parseFaultConfig(oldConfigmap)
			newConfigs := parseFaultConfig(newConfigmap)
			for key, oldConf := range oldConfigs {
				newConf, ok := newConfigs[key]
				labelSelector := labels.SelectorFromSet(map[string]string{v1.FaultId: newConf.Id})
				if !ok || (oldConf.IsEnable() && !newConf.IsEnable()) {
					if err := r.deleteFaults(ctx, labelSelector); err != nil {
						klog.ErrorS(err, "failed to delete faults")
					}
				} else if oldConf.Action != newConf.Action {
					faultList, _ := listFaults(ctx, r.Client, labelSelector)
					for _, f := range faultList {
						q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: f.Name}})
					}
				}
			}
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q v1.RequestWorkQueue) {
			configmap, ok := e.Object.(*corev1.ConfigMap)
			if !ok || configmap.Name != common.PrimusFault {
				return
			}
			if err := r.deleteFaults(ctx, labels.Everything()); err != nil {
				klog.ErrorS(err, "failed to delete faults")
			}
		},
	}
}

// deleteFaults deletes faults matching the given label selector with retry logic.
func (r *FaultReconciler) deleteFaults(ctx context.Context, labelSelector labels.Selector) error {
	op := func() error {
		faultList, err := listFaults(ctx, r.Client, labelSelector)
		if err != nil {
			return err
		}
		for _, fault := range faultList {
			if err = r.Delete(ctx, &fault); client.IgnoreNotFound(err) != nil {
				klog.ErrorS(err, "failed to delete fault")
				return err
			}
			klog.Infof("delete fault: %s", fault.Name)
		}
		return nil
	}
	err := backoff.Retry(op, time.Second, 100*time.Millisecond)
	return err
}

// Reconcile processes Fault resources to ensure they are in the desired state.
func (r *FaultReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished reconcile %s %s cost (%v)", v1.FaultKind, req.Name, time.Since(startTime))
	}()

	fault := new(v1.Fault)
	if err := r.Get(ctx, req.NamespacedName, fault); err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !fault.GetDeletionTimestamp().IsZero() {
		return r.delete(ctx, fault)
	}
	return r.processFault(ctx, fault)
}

// delete handles fault deletion by removing node taints and finalizers.
func (r *FaultReconciler) delete(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
	err := r.removeNodeTaint(ctx, fault)
	if err != nil && !utils.IsNonRetryableError(err) {
		if result, err := r.retry(ctx, fault); err != nil || result.RequeueAfter > 0 {
			return result, err
		}
	}
	return ctrlruntime.Result{}, utils.RemoveFinalizer(ctx, r.Client, fault, v1.FaultFinalizer)
}

// processFault processes a fault by adding or removing node taints based on fault action.
func (r *FaultReconciler) processFault(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
	var err error
	if fault.Spec.Action == "" {
		err = r.removeNodeTaint(ctx, fault)
	} else {
		actions := strings.Split(fault.Spec.Action, ",")
		for _, action := range actions {
			switch action {
			case string(TaintAction):
				err = r.taintNode(ctx, fault)
			default:
			}
			if err != nil {
				break
			}
		}
	}

	phase := v1.FaultPhaseFailed
	if err == nil {
		phase = v1.FaultPhaseSucceeded
	} else {
		klog.ErrorS(err, "failed to handle fault")
		if !utils.IsNonRetryableError(err) {
			// Stop after exceeding the maximum retry limit.
			if result, err := r.retry(ctx, fault); err != nil || result.RequeueAfter > 0 {
				return result, err
			}
		}
	}
	return ctrlruntime.Result{}, r.updatePhase(ctx, fault, phase)
}

// updatePhase updates the fault's phase status.
func (r *FaultReconciler) updatePhase(ctx context.Context, fault *v1.Fault, phase v1.FaultPhase) error {
	patch := client.MergeFrom(fault.DeepCopy())
	fault.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
	fault.Status.Phase = phase
	if err := r.Status().Patch(ctx, fault, patch); err != nil {
		klog.ErrorS(err, "failed to update fault status", "name", fault.Name)
		return err
	}
	return nil
}

// taintNode adds a taint to the node associated with the fault.
func (r *FaultReconciler) taintNode(ctx context.Context, fault *v1.Fault) error {
	if fault.Spec.Node == nil {
		return nil
	}
	adminNode := &v1.Node{}
	err := r.Get(ctx, client.ObjectKey{Name: fault.Spec.Node.AdminName}, adminNode)
	if err != nil || adminNode.GetSpecCluster() == "" {
		return err
	}

	// Check if it has already been processed
	taintKey := commonfaults.GenerateTaintKey(fault.Spec.MonitorId)
	for _, t := range adminNode.Spec.Taints {
		if t.Key == taintKey {
			return nil
		}
	}

	// Add the taint to the node
	patchObj := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": adminNode.ResourceVersion,
		},
		"spec": map[string]any{
			"taints": append(adminNode.Spec.Taints, corev1.Taint{
				Key:       taintKey,
				Effect:    corev1.TaintEffectNoSchedule,
				TimeAdded: &metav1.Time{Time: time.Now().UTC()},
			}),
		},
	}
	p := jsonutils.MarshalSilently(patchObj)
	if err = r.Patch(ctx, adminNode, client.RawPatch(apitypes.MergePatchType, p)); err != nil {
		klog.ErrorS(err, "failed to update admin node")
		return err
	}
	klog.Infof("taint node, cluster: %s, node-name: %s, key: %s", v1.GetClusterId(fault), adminNode.Name, taintKey)
	return nil
}

// removeNodeTaint removes the taint from the node associated with the fault.
func (r *FaultReconciler) removeNodeTaint(ctx context.Context, fault *v1.Fault) error {
	if fault.Spec.Node == nil {
		return nil
	}
	adminNode := &v1.Node{}
	err := r.Get(ctx, client.ObjectKey{Name: fault.Spec.Node.AdminName}, adminNode)
	if err != nil {
		return client.IgnoreNotFound(err)
	}

	isFound := false
	taintKey := commonfaults.GenerateTaintKey(fault.Spec.MonitorId)
	for i, taint := range adminNode.Spec.Taints {
		if taint.Key == taintKey {
			isFound = true
			adminNode.Spec.Taints = append(adminNode.Spec.Taints[:i], adminNode.Spec.Taints[i+1:]...)
			break
		}
	}
	if !isFound {
		return nil
	}

	patchObj := map[string]any{
		"metadata": map[string]any{
			"resourceVersion": adminNode.ResourceVersion,
		},
		"spec": map[string]any{
			"taints": adminNode.Spec.Taints,
		},
	}
	p := jsonutils.MarshalSilently(patchObj)
	if err = r.Patch(ctx, adminNode, client.RawPatch(apitypes.MergePatchType, p)); err != nil {
		klog.ErrorS(err, "failed to update admin node")
		return err
	}

	klog.Infof("remove taint, cluster: %s, node-name: %s, key: %s",
		v1.GetClusterId(fault), adminNode.Name, taintKey)
	return nil
}

// retry handles fault retry logic by incrementing retry count and scheduling requeue.
func (r *FaultReconciler) retry(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
	if fault == nil {
		return ctrlruntime.Result{}, nil
	}
	count, err := utils.IncRetryCount(ctx, r.Client, fault, r.opt.maxRetryCount)
	if err != nil {
		klog.ErrorS(err, "failed to incRetryCount", "name", fault.Name)
		return ctrlruntime.Result{}, err
	}
	if count <= r.opt.maxRetryCount {
		// The maximum number of retries has not been reached, try again later
		klog.Infof("fault %s will retry %d times after %v",
			fault.Name, count, r.opt.processWait)
		return ctrlruntime.Result{RequeueAfter: r.opt.processWait}, nil
	}
	return ctrlruntime.Result{}, nil
}
