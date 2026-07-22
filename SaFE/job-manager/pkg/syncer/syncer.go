/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package syncer

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
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

// The watchdog checks each data-plane cluster's client-factory validity on
// common.DataPlaneHealthProbeInterval. The connection health probing itself
// lives in the shared ClientFactory (which flips validity on a wedged watch);
// this loop only reacts to an invalid factory by rebuilding that cluster's
// informers, since the syncer — unlike other modules — has no reconcile path
// that recreates an existing-but-wedged cluster.

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
	r.runInformerWatchdog(ctx)

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

// runInformerWatchdog starts a background loop that rebuilds a cluster's
// informers once the shared ClientFactory has flipped itself invalid (its
// built-in health probe detects a wedged watch). This lets a silently starved
// syncer self-heal without restarting the whole process. Rebuild targets only
// the affected cluster; a genuinely-down remote just keeps failing the rebuild
// cheaply until it recovers, so there is no restart thrash.
func (r *SyncerReconciler) runInformerWatchdog(ctx context.Context) {
	go wait.UntilWithContext(ctx, r.checkClusterHealth, common.DataPlaneHealthProbeInterval)
}

// checkClusterHealth rebuilds any managed cluster whose client factory is
// invalid. Validity is driven by ClientFactory's own connection probe, so this
// loop stays purely reactive.
func (r *SyncerReconciler) checkClusterHealth(ctx context.Context) {
	keys, objs := r.clusterClientSets.GetAll()
	for i, name := range keys {
		clientSets, ok := objs[i].(*ClusterClientSets)
		if !ok {
			continue
		}
		// Validity is not probed here: the shared ClientFactory runs its own
		// background watchdog (runHealthWatchdog -> Probe) that flips validity on a
		// wedged connection. This loop only consumes that result and reacts by
		// rebuilding the affected cluster's informers.
		factory := clientSets.ClientFactory()
		if factory == nil || factory.IsValid() {
			continue
		}
		klog.InfoS("rebuilding data-plane cluster informers: factory marked invalid",
			"cluster", name, "reason", factory.GetInvalidReason())
		if err := r.rebuildCluster(ctx, name); err != nil {
			// The old (wedged) clientSets stay in place until a new one is built;
			// the next tick retries.
			klog.ErrorS(err, "failed to rebuild data-plane cluster informers", "cluster", name)
			continue
		}
		klog.InfoS("rebuilt data-plane cluster informers", "cluster", name)
	}
}

// rebuildCluster re-fetches the Cluster and rebuilds its client sets/informers.
// handle() replaces the entry via AddOrReplace, which releases the old informers.
func (r *SyncerReconciler) rebuildCluster(ctx context.Context, name string) error {
	c := new(v1.Cluster)
	if err := r.Get(ctx, client.ObjectKey{Name: name}, c); err != nil {
		if apierrors.IsNotFound(err) {
			r.deleteClusterClientSet(name)
			return nil
		}
		return err
	}
	return r.handle(ctx, c)
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
