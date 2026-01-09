/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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

// SyncerReconciler oversees ResourceTemplate objects from all clusters in the data plane, monitors their changes,
// and synchronizes them with the corresponding workload objects in the admin plane
type SyncerReconciler struct {
	ctx context.Context
	client.Client
	// clusterClientSets manages client sets for different clusters
	// Key: cluster name, Value: *ClusterClientSets instance
	clusterClientSets *commonutils.ObjectManager
	*controller.Controller[*resourceMessage]
}

// SetupSyncerController initializes and registers the syncer controller with the manager.
// Sets up watches for Cluster and ResourceTemplate resources.
func SetupSyncerController(ctx context.Context, mgr manager.Manager) error {
	r := &SyncerReconciler{
		ctx:               ctx,
		Client:            mgr.GetClient(),
		clusterClientSets: commonutils.NewObjectManagerSingleton(),
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

// resourceTemplateHandler handles the processing logic for the request.
func (r *SyncerReconciler) resourceTemplateHandler() handler.EventHandler {
	handle := func(rt *v1.ResourceTemplate, doAdd bool) {
		keys, objs := r.clusterClientSets.GetAll()
		for i, key := range keys {
			clientSets, ok := objs[i].(*ClusterClientSets)
			if !ok {
				continue
			}
			if doAdd {
				if err := clientSets.addResourceTemplate(rt.ToSchemaGVK()); err != nil {
					klog.ErrorS(err, "failed to add resource template", "cluster", key, "rt", rt)
				}
			} else {
				clientSets.delResourceTemplate(rt.ToSchemaGVK())
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

// Reconcile is the main control loop for Cluster resources.
// Manages cluster client sets based on cluster lifecycle events.
func (r *SyncerReconciler) Reconcile(ctx context.Context, request ctrlruntime.Request) (ctrlruntime.Result, error) {
	c := new(v1.Cluster)
	if err := r.Get(ctx, request.NamespacedName, c); err != nil {
		if apierrors.IsNotFound(err) {
			r.deleteClusterClientSet(request.Name)
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

// observe checks if a cluster client sets already exists for the given cluster.
func (r *SyncerReconciler) observe(c *v1.Cluster) bool {
	_, ok := r.clusterClientSets.Get(c.Name)
	return ok
}

// handle processes a cluster by creating a new cluster client sets and initializing resource templates.
func (r *SyncerReconciler) handle(ctx context.Context, cluster *v1.Cluster) error {
	clientSets, err := newClusterClientSets(r.ctx, cluster, r.Client, r.Add)
	if err != nil {
		klog.ErrorS(err, "failed to new cluster clientSets", "cluster.name", cluster.Name)
		return err
	}
	rtList := &v1.ResourceTemplateList{}
	if err = r.List(ctx, rtList); err != nil {
		klog.ErrorS(err, "failed to list ResourceTemplateList")
		return err
	}
	for _, rt := range rtList.Items {
		if err = clientSets.addResourceTemplate(rt.ToSchemaGVK()); err != nil {
			klog.ErrorS(err, "failed to add resource template", "cluster", cluster.Name, "rt", rt)
			return err
		}
	}
	r.clusterClientSets.AddOrReplace(cluster.Name, clientSets)
	klog.Infof("create cluster clientSets, name: %s", cluster.Name)
	return nil
}

// deleteClusterClientSet removes and cleans up a cluster clientSets.
func (r *SyncerReconciler) deleteClusterClientSet(clusterId string) {
	if r.clusterClientSets.Delete(clusterId) == nil {
		klog.Infof("delete cluster client set, name: %s", clusterId)
	}
}

// start implements the Runnable interface in controller runtime package.
// Launches worker goroutines for processing resource messages.
func (r *SyncerReconciler) start(ctx context.Context) error {
	for i := 0; i < r.MaxConcurrent; i++ {
		r.Run(ctx)
	}
	return nil
}

// Do process resource messages from cluster clientSets.
// Routes messages to appropriate handlers based on resource type.
// it implements the interface of common.controller.
func (r *SyncerReconciler) Do(ctx context.Context, message *resourceMessage) (ctrlruntime.Result, error) {
	clientSets, err := GetClusterClientSets(r.clusterClientSets, message.cluster)
	if err != nil {
		return ctrlruntime.Result{RequeueAfter: time.Second}, nil
	}

	var result ctrlruntime.Result
	switch message.gvk.Kind {
	case common.PytorchJobKind, common.DeploymentKind, common.StatefulSetKind, common.JobKind,
		common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind:
		result, err = r.handleJob(ctx, message, clientSets)
	case common.PodKind:
		result, err = r.handlePod(ctx, message, clientSets)
	case common.EventKind:
		result, err = r.handleEvent(ctx, message, clientSets)
	}
	if jobutils.IsUnrecoverableError(err) {
		err = nil
	}
	return result, err
}

// getAdminWorkload retrieves an admin workload by ID.
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
