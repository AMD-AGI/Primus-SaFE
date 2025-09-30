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
		Watches(&v1.Node{}, r.handleNodeEvent()).
		Watches(&corev1.ConfigMap{}, r.handleConfigmapEvent()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Fault Controller successfully")
	return nil
}

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

func (r *FaultReconciler) deleteFaults(ctx context.Context, labelSelector labels.Selector) error {
	op := func() error {
		faultList, err := listFaults(ctx, r.Client, labelSelector)
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
	return r.handle(ctx, fault)
}

func (r *FaultReconciler) delete(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
	err := r.removeNodeTaint(ctx, fault)
	if err != nil && !utils.IsNonRetryableError(err) {
		if result, err := r.retry(ctx, fault); err != nil || result.RequeueAfter > 0 {
			return result, err
		}
	}
	return ctrlruntime.Result{}, utils.RemoveFinalizer(ctx, r.Client, fault, v1.FaultFinalizer)
}

func (r *FaultReconciler) handle(ctx context.Context, fault *v1.Fault) (ctrlruntime.Result, error) {
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
	klog.Infof("taint node, cluster: %s, node-name: %s, key: %s", v1.GetClusterId(fault), adminNode.Name, taintKey)
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
	taintKey := commonfaults.GenerateTaintKey(fault.Spec.MonitorId)
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
