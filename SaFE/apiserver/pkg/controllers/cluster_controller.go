/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package controllers

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commoncluster "github.com/AMD-AIG-AIMA/SAFE/common/pkg/cluster"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

// Watchdog tuning for the data-plane direct-call clients. These factories are
// informer-less (DisableInformer), so the shared ClientFactory does not probe
// them; this controller probes them itself and rebuilds a factory whose
// connection is persistently broken (e.g. after an endpoint change or cert
// rotation), which the Has()-guarded addClientFactory would otherwise never
// refresh.
const (
	clusterHealthInterval = 30 * time.Second
	clusterProbeTimeout   = 5 * time.Second
	clusterFailThreshold  = 3
)

type ClusterReconciler struct {
	ctx           context.Context
	client.Client
	clientManager *commonutils.ObjectManager
}

// SetupClusterController sets up the cluster controller with the manager.
func SetupClusterController(ctx context.Context, mgr manager.Manager) error {
	r := &ClusterReconciler{
		Client:        mgr.GetClient(),
		ctx:           ctx,
		clientManager: commonutils.NewObjectManagerSingleton(),
	}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		For(&v1.Cluster{}, builder.WithPredicates(r.relevantChangePredicate())).
		Complete(r)
	if err != nil {
		return err
	}
	r.runFactoryWatchdog(ctx)
	return nil
}

// relevantChangePredicate returns a predicate that filters cluster events for reconciliation.
// It triggers reconciliation when a cluster becomes ready, is being deleted, or transitions to ready state.
func (r *ClusterReconciler) relevantChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			cluster, ok := e.Object.(*v1.Cluster)
			if !ok || !cluster.IsReady() {
				return false
			}
			return true
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldCluster, ok1 := e.ObjectOld.(*v1.Cluster)
			newCluster, ok2 := e.ObjectNew.(*v1.Cluster)
			if !ok1 || !ok2 {
				return false
			}
			if (!oldCluster.IsReady() && newCluster.IsReady()) ||
				(oldCluster.GetDeletionTimestamp().IsZero() && !newCluster.GetDeletionTimestamp().IsZero()) {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			_, ok := e.Object.(*v1.Cluster)
			if !ok {
				return false
			}
			return true
		},
	}
}

// Reconcile implements the reconciliation loop.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	cluster := new(v1.Cluster)
	err := r.Get(ctx, req.NamespacedName, cluster)
	if err != nil {
		return ctrlruntime.Result{}, client.IgnoreNotFound(err)
	}
	if !cluster.GetDeletionTimestamp().IsZero() {
		return ctrlruntime.Result{}, r.deleteClientFactory(cluster)
	}
	if err = r.addClientFactory(r.ctx, cluster); err != nil {
		klog.Errorf("failed to add cluster clients, err: %v", err)
		return ctrlruntime.Result{}, err
	}
	return ctrlruntime.Result{}, nil
}

// deleteClientFactory removes the Kubernetes client factory for a cluster being deleted.
// It cleans up the client manager and releases resources associated with the cluster.
func (r *ClusterReconciler) deleteClientFactory(cluster *v1.Cluster) error {
	if r.clientManager == nil {
		return nil
	}
	if err := r.clientManager.Delete(cluster.Name); err != nil {
		klog.Errorf("failed to delete cluster clients, err: %v", err)
		return err
	}
	klog.Infof("delete cluster %s clients", cluster.Name)
	return nil
}

// addClientFactory creates and registers a new Kubernetes client factory for a ready cluster.
// It retrieves cluster endpoint and credentials, then initializes a client factory for communicating with the cluster.
func (r *ClusterReconciler) addClientFactory(ctx context.Context, cluster *v1.Cluster) error {
	if !cluster.IsReady() {
		return nil
	}
	if r.clientManager == nil {
		return fmt.Errorf("failed to initialize cluster client manager for cluster %s", cluster.Name)
	}
	if r.clientManager.Has(cluster.Name) {
		return nil
	}
	return r.createClientFactory(ctx, cluster)
}

// createClientFactory builds a data-plane direct-call client factory and stores
// it via AddOrReplace, which atomically releases any existing entry. Shared by
// addClientFactory (initial setup) and the watchdog (rebuild on a broken
// connection); on failure the previous factory is left intact for retry.
func (r *ClusterReconciler) createClientFactory(ctx context.Context, cluster *v1.Cluster) error {
	endpoint, err := commoncluster.GetEndpoint(ctx, r.Client, cluster)
	if err != nil {
		return err
	}
	controlPlane := &cluster.Status.ControlPlaneStatus
	k8sClientFactory, err := commonclient.NewClientFactory(ctx, cluster.Name, endpoint,
		controlPlane.CertData, controlPlane.KeyData, controlPlane.CAData, commonclient.DisableInformer)
	if err != nil {
		return err
	}
	r.clientManager.AddOrReplace(cluster.Name, k8sClientFactory)
	klog.Infof("add cluster %s clients", cluster.Name)
	return nil
}

// runFactoryWatchdog probes each data-plane client on an interval and rebuilds a
// cluster's factory after sustained probe failures, so a persistently broken
// direct-call connection (endpoint change, cert rotation) self-heals instead of
// failing every request until a restart. Probe is activity-independent and any
// received HTTP status counts as healthy, so idle/limited-RBAC clusters do not
// trigger needless rebuilds.
func (r *ClusterReconciler) runFactoryWatchdog(ctx context.Context) {
	failures := map[string]int{}
	go wait.UntilWithContext(ctx, func(ctx context.Context) {
		keys, objs := r.clientManager.GetAll()
		live := make(map[string]bool, len(keys))
		for i, name := range keys {
			factory, ok := objs[i].(*commonclient.ClientFactory)
			if !ok {
				continue
			}
			live[name] = true
			probeCtx, cancel := context.WithTimeout(ctx, clusterProbeTimeout)
			err := factory.Probe(probeCtx)
			cancel()
			if err == nil {
				delete(failures, name)
				continue
			}
			failures[name]++
			klog.ErrorS(err, "data-plane cluster health probe failed",
				"cluster", name, "consecutiveFailures", failures[name])
			if failures[name] < clusterFailThreshold {
				continue
			}
			if rebuildErr := r.rebuildClientFactory(ctx, name); rebuildErr != nil {
				klog.ErrorS(rebuildErr, "failed to rebuild data-plane cluster client", "cluster", name)
				continue
			}
			klog.InfoS("rebuilt data-plane cluster client after sustained probe failure", "cluster", name)
			delete(failures, name)
		}
		// Drop counters for clusters no longer managed.
		for name := range failures {
			if !live[name] {
				delete(failures, name)
			}
		}
	}, clusterHealthInterval)
}

// rebuildClientFactory re-fetches the Cluster and recreates its client factory.
// createClientFactory's AddOrReplace atomically releases the old (broken) one.
func (r *ClusterReconciler) rebuildClientFactory(ctx context.Context, name string) error {
	cluster := new(v1.Cluster)
	if err := r.Get(ctx, client.ObjectKey{Name: name}, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			_ = r.clientManager.Delete(name)
			return nil
		}
		return err
	}
	if !cluster.IsReady() {
		return nil
	}
	return r.createClientFactory(ctx, cluster)
}
