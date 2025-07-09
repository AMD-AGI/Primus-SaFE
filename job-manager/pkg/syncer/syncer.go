/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jobutils "github.com/AMD-AIG-AIMA/SAFE/job-manager/pkg/utils"
)

// The SyncerReconciler oversees ResourceTemplate objects from all clusters in the data plane, monitors their changes,
// and synchronizes them with the corresponding workload objects in the admin plane
type SyncerReconciler struct {
	ctx context.Context
	client.Client
	// manage all cluster informer. key: cluster name
	clusterInformers *commonutils.ObjectManager
	*controller.Controller[*resourceMessage]
}

func SetupSyncerController(ctx context.Context, mgr manager.Manager) error {
	r := &SyncerReconciler{
		ctx:              ctx,
		Client:           mgr.GetClient(),
		clusterInformers: commonutils.NewObjectManagerSingleton(),
	}
	r.Controller = controller.NewController[*resourceMessage](r, 1)
	if err := r.start(ctx); err != nil {
		return err
	}

	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(&v1.ResourceTemplate{}, r.resourceTemplateHandler()).
		Complete(r)
	if err != nil {
		return err
	}
	klog.Infof("Setup resource syncer Controller successfully")
	return nil
}

func (r *SyncerReconciler) resourceTemplateHandler() handler.EventHandler {
	handle := func(rt *v1.ResourceTemplate, doAdd bool) {
		keys, objs := r.clusterInformers.GetAll()
		for i, key := range keys {
			informer, ok := objs[i].(*ClusterInformer)
			if !ok {
				continue
			}
			if doAdd {
				if err := informer.addResourceTemplate(rt.ToSchemaGVK()); err != nil {
					klog.ErrorS(err, "failed to add resource template", "cluster", key, "rt", rt)
				}
			} else {
				informer.delResourceTemplate(rt.ToSchemaGVK())
			}
		}
	}
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, evt event.CreateEvent, q v1.RequestWorkQueue) {
			rt, ok := evt.Object.(*v1.ResourceTemplate)
			if !ok {
				return
			}
			handle(rt, true)
		},
		DeleteFunc: func(ctx context.Context, evt event.DeleteEvent, q v1.RequestWorkQueue) {
			rt, ok := evt.Object.(*v1.ResourceTemplate)
			if !ok {
				return
			}
			handle(rt, false)
		},
	}
}

func (r *SyncerReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	c := new(v1.Cluster)
	if err := r.Get(ctx, req.NamespacedName, c); err != nil {
		if apierrors.IsNotFound(err) {
			r.deleteClusterInformer(req.Name)
			err = nil
		}
		return ctrlruntime.Result{}, err
	}
	if quit := r.observe(c); quit {
		return ctrlruntime.Result{}, nil
	}
	if err := r.handle(ctx, c); err != nil {
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

func (r *SyncerReconciler) observe(c *v1.Cluster) bool {
	_, ok := r.clusterInformers.Get(c.Name)
	return ok
}

func (r *SyncerReconciler) handle(ctx context.Context, cluster *v1.Cluster) error {
	informer, err := newClusterInformer(r.ctx, cluster.Name, &cluster.Status.ControlPlaneStatus, r.Client, r.Add)
	if err != nil {
		klog.ErrorS(err, "failed to new cluster informer", "cluster.name", cluster.Name)
		return err
	}
	rtList := &v1.ResourceTemplateList{}
	if err = r.List(ctx, rtList); err != nil {
		klog.ErrorS(err, "failed to list ResourceTemplateList")
		return err
	}
	for _, rt := range rtList.Items {
		if err = informer.addResourceTemplate(rt.ToSchemaGVK()); err != nil {
			klog.ErrorS(err, "failed to add resource template", "cluster", cluster.Name, "rt", rt)
		}
	}
	r.clusterInformers.AddOrReplace(cluster.Name, informer)
	klog.Infof("create cluster informer, name: %s", cluster.Name)
	return nil
}

func (r *SyncerReconciler) deleteClusterInformer(clusterId string) {
	if r.clusterInformers.Delete(clusterId) == nil {
		klog.Infof("delete cluster informer, name: %s", clusterId)
	}
}

// Start implement Runnable interface in controller runtime package.
func (r *SyncerReconciler) start(ctx context.Context) error {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
	return nil
}

func (r *SyncerReconciler) Do(ctx context.Context, message *resourceMessage) (ctrlruntime.Result, error) {
	informer, err := GetClusterInformer(r.clusterInformers, message.cluster)
	if err != nil {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	var result ctrlruntime.Result
	switch message.gvk.Kind {
	case common.PytorchJobKind, common.DeploymentKind, common.StatefulSetKind:
		result, err = r.handleJob(ctx, message, informer)
	case common.PodKind:
		result, err = r.handlePod(ctx, message, informer)
	case common.EventKind:
		result, err = r.handleEvent(ctx, message, informer)
	}
	if jobutils.IsNonRetryableError(err) {
		err = nil
	}
	return result, err
}

func (r *SyncerReconciler) getAdminWorkload(ctx context.Context, workloadId string) (*v1.Workload, error) {
	adminWorkload := &v1.Workload{}
	if err := r.Client.Get(ctx, client.ObjectKey{Name: workloadId}, adminWorkload); err != nil {
		if apierrors.IsNotFound(err) {
			err = nil
		} else {
			klog.ErrorS(err, "failed to get admin workload")
		}
		return nil, err
	}
	return adminWorkload, nil
}
