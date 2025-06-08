/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
)

type FaultReconciler struct {
	*ClusterBaseReconciler
	opt *FaultReconcilerOption
}

type FaultReconcilerOption struct {
	processWait   time.Duration
	maxRetryCount int
}

func SetupFaultController(mgr manager.Manager, opt *FaultReconcilerOption) error {
	r := &FaultReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
		opt: opt,
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Fault{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&v1.Node{}, r.enqueueRequestByNode()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Fault Controller successfully")
	return nil
}

func (r *FaultReconciler) enqueueRequestByNode() handler.EventHandler {
	return handler.Funcs{
		UpdateFunc: func(ctx context.Context, evt event.UpdateEvent, q v1.RequestWorkQueue) {
			oldNode, ok1 := evt.ObjectOld.(*v1.Node)
			newNode, ok2 := evt.ObjectNew.(*v1.Node)
			if !ok1 || !ok2 {
				return
			}
			// delete all faults when unmanaging node
			if oldNode.GetSpecCluster() != "" && newNode.GetSpecCluster() == "" {
				if err := r.deleteAllFaults(ctx, newNode); err != nil {
					klog.ErrorS(err, "failed to delete faults with node", "node", newNode.Name)
				}
			}
			if !reflect.DeepEqual(oldNode.Status.Taints, newNode.Status.Taints) {
				faultList, _ := listFaultsByNode(ctx, r.Client, newNode.Name)
				for _, f := range faultList {
					q.Add(reconcile.Request{NamespacedName: apitypes.NamespacedName{Name: f.Name}})
				}
			}
		},
	}
}

func (r *FaultReconciler) deleteAllFaults(ctx context.Context, node *v1.Node) error {
	op := func() error {
		faultList, err := listFaultsByNode(ctx, r.Client, node.Name)
		if err != nil {
			return err
		}
		for _, fault := range faultList {
			if err = r.Delete(ctx, &fault); client.IgnoreNotFound(err) != nil {
				return err
			}
			klog.Infof("delete fault: %s", fault.Name)
		}
		return nil
	}
	err := backoff.Retry(op, time.Second, 100*time.Millisecond)
	return err
}

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

	if quit := r.observe(fault); quit {
		return ctrlruntime.Result{}, nil
	}
	return r.handle(ctx, fault)
}

func (r *FaultReconciler) delete(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
	err := r.removeNodeTaint(ctx, fault)
	if ignoreError(err) != nil {
		if result, err := r.retry(ctx, fault); err != nil || result.RequeueAfter > 0 {
			return result, err
		}
	}
	return ctrlruntime.Result{}, removeFinalizer(ctx, r.Client, fault, v1.FaultFinalizer)
}

func (r *FaultReconciler) observe(fault *v1.Fault) bool {
	if fault.IsEnd() {
		return true
	}
	return false
}

func (r *FaultReconciler) handle(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
	var err error
	actions := strings.Split(fault.Spec.Action, ",")
	for _, action := range actions {
		switch action {
		case string(TaintAction):
			err = r.taintNode(ctx, fault)
		default:
			continue
		}
		if err != nil {
			break
		}
	}

	phase := v1.FaultPhaseFailed
	if err == nil {
		phase = v1.FaultPhaseSucceeded
	} else {
		klog.ErrorS(err, "failed to handle fault")
		if ignoreError(err) != nil {
			// Stop after exceeding the maximum retry limit.
			if result, err := r.retry(ctx, fault); err != nil || result.RequeueAfter > 0 {
				return result, err
			}
		}
	}
	return ctrlruntime.Result{}, r.updatePhase(ctx, fault, phase)
}

func (r *FaultReconciler) updatePhase(ctx context.Context, fault *v1.Fault, phase v1.FaultPhase) error {
	fault.Status.UpdateTime = &metav1.Time{Time: time.Now().UTC()}
	fault.Status.Phase = phase
	if err := r.Status().Update(ctx, fault); err != nil {
		klog.ErrorS(err, "failed to update fault status", "name", fault.Name)
		return err
	}
	return nil
}

func (r *FaultReconciler) taintNode(ctx context.Context, fault *v1.Fault) error {
	if fault.Spec.Node == nil {
		return nil
	}
	clusterName := v1.GetClusterId(fault)
	adminNode := &v1.Node{}
	err := r.Get(ctx, client.ObjectKey{Name: fault.Spec.Node.AdminName}, adminNode)
	if err != nil {
		return err
	}

	// Check if it has already been processed
	taintKey := commonfaults.GenerateTaintKey(fault.Spec.Id)
	for _, t := range adminNode.Spec.Taints {
		if t.Key == taintKey {
			return nil
		}
	}

	// Add the taint to the node
	adminNode.Spec.Taints = append(adminNode.Spec.Taints, corev1.Taint{
		Key:       taintKey,
		Effect:    corev1.TaintEffectNoSchedule,
		TimeAdded: &metav1.Time{Time: time.Now().UTC()},
	})
	err = r.Update(ctx, adminNode)
	if err != nil {
		klog.ErrorS(err, "failed to update admin node")
		return err
	}
	klog.Infof("taint node, cluster: %s, node-name: %s, key: %s", clusterName, adminNode.Name, taintKey)
	return nil
}

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
	taintKey := commonfaults.GenerateTaintKey(fault.Spec.Id)
	for i, taint := range adminNode.Spec.Taints {
		if taint.Key == taintKey {
			isFound = true
			adminNode.Spec.Taints = append(adminNode.Spec.Taints[:i], adminNode.Spec.Taints[i+1:]...)
			break
		}
	}
	if isFound {
		err = r.Update(ctx, adminNode)
		if err != nil {
			return err
		}
		klog.Infof("remove taint, cluster: %s, node-name: %s, key: %s",
			v1.GetClusterId(fault), adminNode.Name, taintKey)
	}
	return nil
}

func (r *FaultReconciler) retry(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
	if fault == nil {
		return ctrlruntime.Result{}, nil
	}
	count, err := incRetryCount(ctx, r.Client, fault, r.opt.maxRetryCount)
	if err != nil {
		klog.ErrorS(err, "failed to incRetryCount", "name", fault.Name)
		return ctrlruntime.Result{}, err
	}
	if count < r.opt.maxRetryCount {
		// The maximum number of retries has not been reached, try again later
		klog.Infof("fault %s will retry %d times after %v",
			fault.Name, count, r.opt.processWait)
		return ctrlruntime.Result{RequeueAfter: r.opt.processWait}, nil
	}
	return ctrlruntime.Result{}, nil
}
