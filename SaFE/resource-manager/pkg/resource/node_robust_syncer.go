/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

const robustPushRestored = "restored"

// nodeSyncPushReasons orders the reconcile pushes.
var nodeSyncPushReasons = []string{robustPushMissing, robustPushRestored, robustPushStale}

var (
	robustNodeSyncErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_node_sync_errors_total",
		Help: "Total number of failed robust node sync operations",
	}, []string{"cluster", "type"})

	robustNodeSyncSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_node_sync_success_total",
		Help: "Total number of successful robust node sync operations",
	}, []string{"cluster", "type"})

	robustNodeSyncPushed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_node_sync_pushed_total",
		Help: "Total number of nodes pushed to robust by the reconcile loop",
	}, []string{"cluster", "reason"})

	robustNodeSyncDeleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_node_sync_deleted_total",
		Help: "Total number of nodes deleted from robust by the reconcile loop",
	}, []string{"cluster"})

	robustNodeSyncReconcileErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_node_sync_reconcile_errors_total",
		Help: "Total number of failed robust node sync reconcile rounds",
	}, []string{"cluster"})

	robustNodeSyncFallback = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "safe_robust_node_sync_fallback_total",
		Help: "Total number of times a cluster was skipped because its robust has no node sync API",
	}, []string{"cluster"})
)

// NodeRobustSyncer pushes the nodes SaFE manages to the Robust API of their
// cluster. In SaFE mode Robust learns nodes from nothing else.
type NodeRobustSyncer struct {
	client.Client
	robustClient *robustclient.Client
	// synced remembers the cluster each node was last pushed to, so a node that
	// is gone or left its cluster can be deleted there without a digest round.
	synced sync.Map
	// noNodeSync holds the clusters found to have a Robust without node sync,
	// by the digest or by a per-node call. Their per-node calls are skipped and
	// the report is made once, until a digest round finds node sync again.
	noNodeSync sync.Map
}

// SetupNodeRobustSyncer registers the node syncer when it is enabled in the config.
func SetupNodeRobustSyncer(mgr manager.Manager, rc *robustclient.Client) error {
	if rc == nil || !commonconfig.IsWorkloadRobustNodeSyncEnable() {
		klog.Info("[node-robust-syncer] disabled or robust client not configured, skipping")
		return nil
	}
	r := &NodeRobustSyncer{Client: mgr.GetClient(), robustClient: rc}
	err := ctrlruntime.NewControllerManagedBy(mgr).
		Named("node-robust-syncer").
		For(&v1.Node{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: 2}).
		Complete(r)
	if err != nil {
		return err
	}

	interval := time.Duration(commonconfig.GetWorkloadRobustSyncIntervalSecond()) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if err = mgr.Add(manager.RunnableFunc(func(ctx context.Context) error {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
				r.reconcileAll(ctx)
			}
		}
	})); err != nil {
		return err
	}
	klog.Info("[node-robust-syncer] controller registered")
	return nil
}

func (r *NodeRobustSyncer) Reconcile(ctx context.Context, req ctrlruntime.Request) (ctrlruntime.Result, error) {
	node := &v1.Node{}
	if err := r.Get(ctx, req.NamespacedName, node); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrlruntime.Result{}, err
		}
		return r.deleteFromRobust(ctx, req.Name, r.lastCluster(req.Name))
	}

	cluster := v1.GetClusterId(node)
	if !node.GetDeletionTimestamp().IsZero() || !inNodeSyncScope(node) {
		if cluster == "" {
			cluster = r.lastCluster(node.Name)
		}
		return r.deleteFromRobust(ctx, node.Name, cluster)
	}
	if last := r.lastCluster(node.Name); last != "" && last != cluster {
		if res, err := r.deleteFromRobust(ctx, node.Name, last); err != nil || !res.IsZero() {
			return res, err
		}
	}

	// Without a Robust client the periodic reconcileAll syncs the cluster once it gets one.
	cc := r.robustClient.ForCluster(cluster)
	if cc == nil {
		klog.V(4).Infof("[node-robust-syncer] cluster %s not available, skip node %s", cluster, node.Name)
		return ctrlruntime.Result{}, nil
	}
	if !r.syncEnabled(ctx, cluster) || r.hasNoNodeSync(cluster) {
		return ctrlruntime.Result{}, nil
	}
	flavor, err := r.nodeFlavor(ctx, node)
	if err != nil {
		return ctrlruntime.Result{}, err
	}
	if err = cc.SyncNode(ctx, buildNodeSyncPayload(node, flavor)); err != nil {
		if nodeSyncUnavailable(err) {
			robustNodeSyncFallback.WithLabelValues(cluster).Inc()
			r.reportNoNodeSync(cluster, err)
			return ctrlruntime.Result{}, nil
		}
		robustNodeSyncErrors.WithLabelValues(cluster, "node").Inc()
		return ctrlruntime.Result{}, fmt.Errorf("sync node %s to robust of cluster %s: %w", node.Name, cluster, err)
	}
	r.noNodeSync.Delete(cluster)
	robustNodeSyncSuccess.WithLabelValues(cluster, "node").Inc()
	r.synced.Store(node.Name, cluster)
	return ctrlruntime.Result{}, nil
}

func (r *NodeRobustSyncer) lastCluster(name string) string {
	if v, ok := r.synced.Load(name); ok {
		return v.(string)
	}
	return ""
}

// deleteFromRobust removes a node from the Robust of cluster. Without a known
// cluster it is left to the reconcile loop.
func (r *NodeRobustSyncer) deleteFromRobust(ctx context.Context, name, cluster string) (ctrlruntime.Result, error) {
	if cluster == "" {
		return ctrlruntime.Result{}, nil
	}
	cc := r.robustClient.ForCluster(cluster)
	if cc == nil {
		return ctrlruntime.Result{}, nil
	}
	if !r.syncEnabled(ctx, cluster) || r.hasNoNodeSync(cluster) {
		r.synced.CompareAndDelete(name, cluster)
		return ctrlruntime.Result{}, nil
	}
	if err := cc.DeleteNode(ctx, name); err != nil {
		if !nodeSyncUnavailable(err) {
			robustNodeSyncErrors.WithLabelValues(cluster, "node_delete").Inc()
			return ctrlruntime.Result{}, fmt.Errorf("delete node %s from robust of cluster %s: %w", name, cluster, err)
		}
		robustNodeSyncFallback.WithLabelValues(cluster).Inc()
		r.reportNoNodeSync(cluster, err)
		r.synced.CompareAndDelete(name, cluster)
		return ctrlruntime.Result{}, nil
	}
	robustNodeSyncSuccess.WithLabelValues(cluster, "node_delete").Inc()
	r.synced.CompareAndDelete(name, cluster)
	return ctrlruntime.Result{}, nil
}

// syncEnabled honours the same per-cluster switch as the workload sync.
func (r *NodeRobustSyncer) syncEnabled(ctx context.Context, cluster string) bool {
	c := &v1.Cluster{}
	if err := r.Get(ctx, client.ObjectKey{Name: cluster}, c); err != nil {
		return true
	}
	return c.Annotations[annotationRobustWorkloadSync] != "false"
}

// nodeFlavor returns the flavor of node, or nil when it has none.
func (r *NodeRobustSyncer) nodeFlavor(ctx context.Context, node *v1.Node) (*v1.NodeFlavor, error) {
	name := node.GetSpecNodeFlavor()
	if name == "" {
		return nil, nil
	}
	flavor := &v1.NodeFlavor{}
	if err := r.Get(ctx, client.ObjectKey{Name: name}, flavor); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return flavor, nil
}

// reconcileAll reconciles every cluster that has a Robust client, in parallel.
func (r *NodeRobustSyncer) reconcileAll(ctx context.Context) {
	nodes := &v1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		klog.Warningf("[node-robust-syncer] reconcile list nodes failed: %v", err)
		return
	}
	flavorList := &v1.NodeFlavorList{}
	if err := r.List(ctx, flavorList); err != nil {
		klog.Warningf("[node-robust-syncer] reconcile list node flavors failed: %v", err)
		return
	}
	flavors := make(map[string]*v1.NodeFlavor, len(flavorList.Items))
	for i := range flavorList.Items {
		flavors[flavorList.Items[i].Name] = &flavorList.Items[i]
	}
	byCluster := make(map[string][]*v1.Node)
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if inNodeSyncScope(node) {
			cluster := v1.GetClusterId(node)
			byCluster[cluster] = append(byCluster[cluster], node)
		}
	}

	var wg sync.WaitGroup
	for _, cluster := range r.robustClient.ClusterNames() {
		cc := r.robustClient.ForCluster(cluster)
		if cc == nil || !r.syncEnabled(ctx, cluster) {
			continue
		}
		wg.Add(1)
		go func(cc *robustclient.ClusterClient, nodes []*v1.Node) {
			defer wg.Done()
			if err := r.reconcileCluster(ctx, cc, nodes, flavors); err != nil {
				klog.Warningf("[node-robust-syncer] reconcile cluster %s failed: %v", cc.ClusterName(), err)
				robustNodeSyncReconcileErrors.WithLabelValues(cc.ClusterName()).Inc()
			}
		}(cc, byCluster[cluster])
	}
	wg.Wait()
}

// reconcileCluster brings Robust's copy of one cluster's nodes in line with
// SaFE's. A Robust whose node digest answers 404 or 501 has no node sync and
// is skipped for the round.
func (r *NodeRobustSyncer) reconcileCluster(ctx context.Context, cc *robustclient.ClusterClient,
	nodes []*v1.Node, flavors map[string]*v1.NodeFlavor) error {
	cluster := cc.ClusterName()
	digest, err := cc.GetNodeSyncDigest(ctx)
	if err != nil {
		if !robustclient.IsNotSupported(err) {
			return fmt.Errorf("get node sync digest: %w", err)
		}
		robustNodeSyncFallback.WithLabelValues(cluster).Inc()
		r.reportNoNodeSync(cluster, err)
		return nil
	}
	r.noNodeSync.Delete(cluster)

	plan := planNodeSync(nodes, flavors, digest.Nodes)
	var errs []error
	for _, reason := range nodeSyncPushReasons {
		if err = r.push(ctx, cc, plan.push[reason], reason); err != nil {
			errs = append(errs, err)
		}
	}
	for _, name := range plan.delete {
		if err = cc.DeleteNode(ctx, name); err != nil {
			robustNodeSyncErrors.WithLabelValues(cluster, "reconcile_delete").Inc()
			errs = append(errs, fmt.Errorf("delete %s: %w", name, err))
			continue
		}
		robustNodeSyncDeleted.WithLabelValues(cluster).Inc()
		r.synced.CompareAndDelete(name, cluster)
	}
	return errors.Join(errs...)
}

func (r *NodeRobustSyncer) hasNoNodeSync(cluster string) bool {
	_, ok := r.noNodeSync.Load(cluster)
	return ok
}

// nodeSyncUnavailable tells whether a per-node call failed because the Robust
// has no node sync for SaFE: the route is missing (404 or 501), or, as on an
// older Robust, guarded by a session SaFE does not have (401 or 403).
func nodeSyncUnavailable(err error) bool {
	if robustclient.IsNotSupported(err) {
		return true
	}
	var se *robustclient.StatusError
	return errors.As(err, &se) &&
		(se.StatusCode == http.StatusUnauthorized || se.StatusCode == http.StatusForbidden)
}

// reportNoNodeSync marks cluster as having no node sync and logs it once,
// until a digest round finds node sync again.
func (r *NodeRobustSyncer) reportNoNodeSync(cluster string, err error) {
	if _, logged := r.noNodeSync.LoadOrStore(cluster, true); !logged {
		klog.Infof("[node-robust-syncer] robust of cluster %s has no node sync API, skipping its nodes: %v", cluster, err)
	}
}

func (r *NodeRobustSyncer) push(ctx context.Context, cc *robustclient.ClusterClient,
	payloads []robustclient.NodeSyncPayload, reason string) error {
	if len(payloads) == 0 {
		return nil
	}
	synced, err := cc.SyncNodes(ctx, payloads)
	robustNodeSyncPushed.WithLabelValues(cc.ClusterName(), reason).Add(float64(synced))
	if err != nil && !robustclient.IsNotSupported(err) {
		robustNodeSyncErrors.WithLabelValues(cc.ClusterName(), reason).Inc()
	}
	return err
}

type nodeSyncPlan struct {
	// push holds the payloads to send, by reason.
	push map[string][]robustclient.NodeSyncPayload
	// delete names the nodes Robust holds that SaFE no longer has in the cluster.
	delete []string
}

// planNodeSync compares the in-scope nodes of one cluster with what its Robust
// holds and returns what to push and what to delete.
func planNodeSync(nodes []*v1.Node, flavors map[string]*v1.NodeFlavor,
	digest []robustclient.NodeSyncDigestEntry) nodeSyncPlan {
	held := make(map[string]robustclient.NodeSyncDigestEntry, len(digest))
	for _, d := range digest {
		held[d.Name] = d
	}
	plan := nodeSyncPlan{push: make(map[string][]robustclient.NodeSyncPayload)}
	live := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		if !node.GetDeletionTimestamp().IsZero() {
			continue
		}
		live[node.Name] = true
		payload := buildNodeSyncPayload(node, flavors[node.GetSpecNodeFlavor()])
		d, ok := held[node.Name]
		switch {
		case !ok:
			plan.push[robustPushMissing] = append(plan.push[robustPushMissing], payload)
		case d.Deleted:
			plan.push[robustPushRestored] = append(plan.push[robustPushRestored], payload)
		case isNewerVersion(payload.ResourceVersion, d.SyncVersion):
			plan.push[robustPushStale] = append(plan.push[robustPushStale], payload)
		}
	}
	for _, d := range digest {
		if !d.Deleted && !live[d.Name] {
			plan.delete = append(plan.delete, d.Name)
		}
	}
	return plan
}

// inNodeSyncScope tells whether a SaFE Node is synced: a managed (physical)
// node or an admitted external virtual node, bound to a cluster.
func inNodeSyncScope(node *v1.Node) bool {
	if node == nil || v1.GetClusterId(node) == "" {
		return false
	}
	if node.DeclaresExternalLifecycle() {
		return node.IsExternal()
	}
	return node.Spec.LifecycleMode == ""
}

// nodeSyncPhase reduces the node state to the NodeSyncPhase values.
func nodeSyncPhase(node *v1.Node) string {
	switch {
	case !node.IsMachineReady() || !node.IsManaged():
		return robustclient.NodeSyncPhaseNotReady
	case node.Status.Unschedulable:
		return robustclient.NodeSyncPhaseUnschedulable
	default:
		return robustclient.NodeSyncPhaseReady
	}
}

// buildNodeSyncPayload maps a SaFE Node to what Robust stores. The resources
// are the node totals the apiserver node view reports; flavor, which may be
// nil, names the GPU resource and model.
func buildNodeSyncPayload(node *v1.Node, flavor *v1.NodeFlavor) robustclient.NodeSyncPayload {
	payload := robustclient.NodeSyncPayload{
		Name:            node.Name,
		Hostname:        node.GetSpecHostName(),
		Workspace:       v1.GetWorkspaceId(node),
		Labels:          node.Labels,
		Annotations:     node.Annotations,
		Phase:           nodeSyncPhase(node),
		ResourceVersion: node.ResourceVersion,
		Lifecycle:       robustclient.NodeSyncLifecycleManaged,
	}

	gpuResource := corev1.ResourceName(common.AmdGpu)
	if flavor != nil && flavor.Spec.Gpu != nil {
		if flavor.Spec.Gpu.ResourceName != "" {
			gpuResource = corev1.ResourceName(flavor.Spec.Gpu.ResourceName)
		}
		payload.GPUModel = string(flavor.Spec.Gpu.Product)
	}
	if q, ok := node.Status.Resources[gpuResource]; ok {
		payload.GPUCount = int(q.Value())
	}
	if q, ok := node.Status.Resources[corev1.ResourceCPU]; ok {
		payload.CPU = int(q.Value())
	}
	if q, ok := node.Status.Resources[corev1.ResourceMemory]; ok {
		payload.MemMB = q.Value() / (1024 * 1024)
	}

	if node.IsExternal() {
		ref := node.Spec.ExternalRef
		payload.Lifecycle = robustclient.NodeSyncLifecycleExternal
		payload.External = &robustclient.NodeSyncExternal{
			Provider:     ref.Provider,
			AllocationID: ref.AllocationId,
			Generation:   ref.Generation,
			HostKey:      ref.HostKey,
		}
		// The front-end IPv4 is the only scrape address of an external node, so its
		// absence is sent as an empty address and clears the one Robust holds.
		address := v1.GetAnnotation(node, v1.ExternalFrontEndIPv4Annotation)
		payload.Address = &address
	} else {
		address := node.Spec.PrivateIP
		if address == "" {
			address = node.Status.MachineStatus.PrivateIP
		}
		if address != "" {
			payload.Address = &address
		}
	}
	return payload
}
