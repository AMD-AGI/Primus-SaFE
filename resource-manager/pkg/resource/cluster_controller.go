/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
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
)

type ClusterReconciler struct {
	*ClusterBaseReconciler
}

func SetupClusterController(mgr manager.Manager) error {
	r := &ClusterReconciler{
		ClusterBaseReconciler: &ClusterBaseReconciler{
			Client: mgr.GetClient(),
		},
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(predicate.ResourceVersionChangedPredicate{})).
		Watches(&corev1.Pod{}, r.enqueueRequestByWorkerPod()).
		Watches(&v1.Node{}, r.enqueueRequestByNode()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup Cluster Controller successfully")
	return nil
}

func (r *ClusterReconciler) enqueueRequestByNode() handler.EventHandler {
	equeue := func(node *v1.Node, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		for _, owner := range node.OwnerReferences {
			if owner.APIVersion == v1.SchemeGroupVersion.String() && owner.Kind == v1.ClusterKind {
				q.Add(ctrlruntime.Request{
					NamespacedName: types.NamespacedName{
						Name: owner.Name,
					},
				})
			}
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, event event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if node, ok := event.Object.(*v1.Node); ok {
				equeue(node, q)
			}
		},
		UpdateFunc: func(ctx context.Context, event event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if node, ok := event.ObjectNew.(*v1.Node); ok {
				equeue(node, q)
			}
		},
		DeleteFunc: func(ctx context.Context, event event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if node, ok := event.Object.(*v1.Node); ok {
				equeue(node, q)
			}
		},
		GenericFunc: nil,
	}
}

func (r *ClusterReconciler) enqueueRequestByWorkerPod() handler.EventHandler {
	enqueue := func(pod *corev1.Pod, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		for _, owner := range pod.OwnerReferences {
			if owner.APIVersion == v1.SchemeGroupVersion.String() && owner.Kind == v1.ClusterKind {
				q.Add(ctrlruntime.Request{
					NamespacedName: types.NamespacedName{
						Name: owner.Name,
					},
				})
			}
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, event event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if pod, ok := event.Object.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		UpdateFunc: func(ctx context.Context, event event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if pod, ok := event.ObjectNew.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		DeleteFunc: func(ctx context.Context, event event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			if pod, ok := event.Object.(*corev1.Pod); ok {
				enqueue(pod, q)
			}
		},
		GenericFunc: nil,
	}
}

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	startTime := time.Now().UTC()
	defer func() {
		klog.V(4).Infof("Finished node reconcile %s cost (%v)", req.Name, time.Since(startTime))
	}()
	cluster := new(v1.Cluster)
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrlruntime.Result{}, nil
		}
		return ctrlruntime.Result{}, err
	}
	if !cluster.DeletionTimestamp.IsZero() {
		return ctrlruntime.Result{}, r.delete(ctx, cluster)
	}
	if err := r.guaranteeClusterControlePlane(ctx, cluster); err != nil {
		return ctrlruntime.Result{}, err
	}

	if err := r.guaranteeStorage(ctx, cluster); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

func (r *ClusterReconciler) delete(ctx context.Context, cluster *v1.Cluster) error {
	if err := r.resetNodesOfCluster(cluster.Name); err != nil {
		return err
	}
	cm := newClusterManager()
	cm.deleteInformer(cluster.Name)
	if err := removeFinalizer(ctx, r.Client, cluster, v1.ClusterFinalizer); err != nil {
		return err
	}
	return nil
}

func (r *ClusterReconciler) resetNodesOfCluster(clusterName string) error {
	req, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{clusterName})
	labelSelector := labels.NewSelector().Add(*req)
	nodeList := &v1.NodeList{}
	if err := r.List(context.Background(), nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		klog.ErrorS(err, "failed to list nodes")
		return err
	}
	for _, n := range nodeList.Items {
		deleteConcernedMeta(&n)
		n.Spec.Cluster = nil
		n.Spec.Workspace = nil
		if err := r.Update(context.Background(), &n); err != nil {
			klog.ErrorS(err, "failed to update node")
			return err
		}

		n.Status = v1.NodeStatus{}
		if err := r.Status().Update(context.Background(), &n); err != nil {
			klog.ErrorS(err, "failed to update node status")
			return err
		}
		klog.Infof("reset the node(%s) after the cluster(%s) is deleted.", n.Name, clusterName)
	}
	return nil
}
