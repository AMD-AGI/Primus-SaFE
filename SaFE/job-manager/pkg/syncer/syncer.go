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
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/controller"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
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
	dbClient          dbclient.Interface
	*controller.KeyedController[*resourceMessage]
}

// syncerWorkers is the number of concurrent workers for the event queue. The
// queue is keyed by object identity, so the same object is still processed
// serially while different objects fan out across workers.
const syncerWorkers = 8

// clusterClientSetsRetryInterval is the backoff for re-attempting data-plane informer setup.
const clusterClientSetsRetryInterval = 30 * time.Second

// resourceMessageKey identifies the k8s object a message is about. Messages with
// the same key are serialized and coalesced by the KeyedController.
func resourceMessageKey(m *resourceMessage) string {
	return m.cluster + "|" + m.gvk.String() + "|" + m.namespace + "|" + m.name
}

// mergeResourceMessage keeps the latest event for a key, except that a pending
// delete is never overwritten by a non-delete: once an object is known deleted,
// a late add/update event must not resurrect its processing.
func mergeResourceMessage(existing *resourceMessage, existingOK bool, incoming *resourceMessage) *resourceMessage {
	if existingOK && existing.action == ResourceDel && incoming.action != ResourceDel {
		return existing
	}
	return incoming
}

// SetupSyncerController initializes and registers the syncer controller with the manager.
// Sets up watches for Cluster and ResourceTemplate resources.
func SetupSyncerController(ctx context.Context, mgr manager.Manager) error {
	var dbCli dbclient.Interface
	if commonconfig.IsDBEnable() {
		dbCli = dbclient.NewClient()
	}
	r := &SyncerReconciler{
		ctx:               ctx,
		Client:            mgr.GetClient(),
		clusterClientSets: commonutils.NewObjectManagerSingleton(),
		dbClient:          dbCli,
	}
	r.KeyedController = controller.NewKeyedController[*resourceMessage](r, resourceMessageKey, mergeResourceMessage, syncerWorkers)
	if err := r.start(ctx); err != nil {
		return err
	}
	if err := mgr.Add(&clusterClientSetsMaintainer{r: r}); err != nil {
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
	rtList := &v1.ResourceTemplateList{}
	if err := r.List(ctx, rtList); err != nil {
		klog.ErrorS(err, "failed to list ResourceTemplateList")
		return ctrlruntime.Result{}, err
	}
	if retryNeeded := r.ensureClusterClientSets(ctx, c, rtList); retryNeeded {
		return ctrlruntime.Result{RequeueAfter: clusterClientSetsRetryInterval}, nil
	}
	return ctrlruntime.Result{}, nil
}

// ensureClusterClientSets creates or completes data-plane informers for a cluster.
// Returns true when setup is incomplete and should be retried.
func (r *SyncerReconciler) ensureClusterClientSets(ctx context.Context, cluster *v1.Cluster,
	rtList *v1.ResourceTemplateList) bool {
	if !cluster.IsReady() {
		return false
	}
	clientSets, err := GetClusterClientSets(r.clusterClientSets, cluster.Name)
	if err != nil {
		clientSets, err = newClusterClientSets(r.ctx, cluster, r.Client, r.Add)
		if err != nil {
			klog.ErrorS(err, "failed to new cluster clientSets", "cluster", cluster.Name)
			return true
		}
		r.clusterClientSets.AddOrReplace(cluster.Name, clientSets)
		klog.Infof("create cluster clientSets, name: %s", cluster.Name)
	} else if clientSets.needsClientFactoryRefresh(ctx, cluster, r.Client) {
		if err = clientSets.recreateClientFactory(ctx, cluster, r.Client); err != nil {
			klog.ErrorS(err, "failed to recreate cluster clientSets", "cluster", cluster.Name)
			return true
		}
	}
	for i := range rtList.Items {
		if err := clientSets.addResourceTemplate(rtList.Items[i].ToSchemaGVK()); err != nil {
			klog.ErrorS(err, "failed to add resource template",
				"cluster", cluster.Name, "rt", rtList.Items[i].Name)
		}
	}
	if clientSets.needsInformerRetry(rtList) {
		klog.InfoS("cluster client sets incomplete, will retry informer setup",
			"cluster", cluster.Name, "informers", clientSets.informerCount())
		return true
	}
	return false
}

// clusterClientSetsMaintainer retries data-plane informer setup on the elected leader only.
type clusterClientSetsMaintainer struct {
	r *SyncerReconciler
}

// Start implements manager.Runnable.
func (m *clusterClientSetsMaintainer) Start(ctx context.Context) error {
	m.r.maintainClusterClientSets(ctx)
	return nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
func (m *clusterClientSetsMaintainer) NeedLeaderElection() bool {
	return true
}

// maintainClusterClientSets periodically retries informer setup for all clusters.
func (r *SyncerReconciler) maintainClusterClientSets(ctx context.Context) {
	ticker := time.NewTicker(clusterClientSetsRetryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.ensureAllClusterClientSets(ctx)
		}
	}
}

// ensureAllClusterClientSets retries informer setup across every registered cluster.
func (r *SyncerReconciler) ensureAllClusterClientSets(ctx context.Context) {
	clusterList := &v1.ClusterList{}
	if err := r.List(ctx, clusterList); err != nil {
		klog.ErrorS(err, "failed to list clusters for client sets maintenance")
		return
	}
	rtList := &v1.ResourceTemplateList{}
	if err := r.List(ctx, rtList); err != nil {
		klog.ErrorS(err, "failed to list ResourceTemplateList during client sets maintenance")
		return
	}
	for i := range clusterList.Items {
		r.ensureClusterClientSets(ctx, &clusterList.Items[i], rtList)
	}
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
		common.CICDScaleRunnerSetKind, common.CICDEphemeralRunnerKind, common.RayJobKind, common.SandboxKind,
		common.DynamoGraphDeploymentKind, common.InferaDeploymentKind:
		result, err = r.handleJob(ctx, message, clientSets)
	case common.PodKind:
		result, err = r.handlePod(ctx, message, clientSets)
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
	copy := adminWorkload.DeepCopy()
	r.hydrateWorkloadStatusFromDB(ctx, workloadId, copy)
	return copy, nil
}
