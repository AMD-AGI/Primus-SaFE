/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

func syncTestFlavor() *v1.NodeFlavor {
	return &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "mi300x"},
		Spec: v1.NodeFlavorSpec{Gpu: &v1.GpuChip{
			Product:      "MI300X",
			ResourceName: common.AmdGpu,
			Quantity:     resource.MustParse("8"),
		}},
	}
}

// syncManagedNode is a ready, managed physical node of cluster c1.
func syncManagedNode(name, rv string) *v1.Node {
	n := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:            name,
		ResourceVersion: rv,
		Labels:          map[string]string{v1.ClusterIdLabel: "c1", v1.WorkspaceIdLabel: "ws"},
	}}
	n.Spec.Hostname = pointer.String(name + "-host")
	n.Spec.PrivateIP = "10.0.0.1"
	n.Spec.NodeFlavor = &corev1.ObjectReference{Name: "mi300x"}
	n.Status.MachineStatus.Phase = v1.NodeReady
	n.Status.ClusterStatus.Phase = v1.NodeManaged
	n.Status.Resources = corev1.ResourceList{
		corev1.ResourceName(common.AmdGpu): resource.MustParse("8"),
		corev1.ResourceCPU:                 resource.MustParse("128"),
		corev1.ResourceMemory:              resource.MustParse("1Ti"),
	}
	return n
}

// syncExternalNode is an admitted virtual node with a fresh provider observation.
func syncExternalNode(name, hostKey, frontEndIP string) *v1.Node {
	now := time.Now().UTC()
	n := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   name,
		Labels: map[string]string{v1.ClusterIdLabel: "c1", v1.WorkspaceIdLabel: "ws-ext"},
		Annotations: map[string]string{
			v1.ExternalObservedAtAnnotation: now.Format(time.RFC3339),
			v1.ExternalValidUntilAnnotation: now.Add(time.Hour).Format(time.RFC3339),
		},
	}}
	if frontEndIP != "" {
		n.Annotations[v1.ExternalFrontEndIPv4Annotation] = frontEndIP
	}
	n.Spec.Hostname = pointer.String(name)
	n.Spec.LifecycleMode = v1.NodeLifecycleExternal
	n.Spec.ExternalRef = &v1.NodeExternalRef{Provider: "spur", AllocationId: "alloc-1", Generation: 3, HostKey: hostKey}
	n.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}
	n.Status.Resources = corev1.ResourceList{corev1.ResourceName(common.AmdGpu): resource.MustParse("8")}
	return n
}

func TestBuildNodeSyncPayloadManaged(t *testing.T) {
	n := syncManagedNode("n1", "42")
	p := buildNodeSyncPayload(n, syncTestFlavor())
	assert.Equal(t, "n1", p.Name)
	assert.Equal(t, "n1-host", p.Hostname)
	assert.Equal(t, "ws", p.Workspace)
	assert.Equal(t, n.Labels, p.Labels)
	assert.Equal(t, robustclient.NodeSyncPhaseReady, p.Phase)
	assert.Equal(t, 8, p.GPUCount)
	assert.Equal(t, "MI300X", p.GPUModel)
	assert.Equal(t, 128, p.CPU)
	assert.Equal(t, int64(1024*1024), p.MemMB)
	assert.Equal(t, "10.0.0.1", p.Address)
	assert.Equal(t, robustclient.NodeSyncLifecycleManaged, p.Lifecycle)
	assert.Nil(t, p.External)
	assert.Equal(t, "42", p.ResourceVersion)

	// The private IP falls back to the one the machine reports.
	n.Spec.PrivateIP = ""
	n.Status.MachineStatus.PrivateIP = "10.0.0.2"
	assert.Equal(t, "10.0.0.2", buildNodeSyncPayload(n, nil).Address)

	// Without a flavor the default GPU resource is counted and the model is unknown.
	p = buildNodeSyncPayload(n, nil)
	assert.Equal(t, 8, p.GPUCount)
	assert.Empty(t, p.GPUModel)

	// The flavor names the GPU resource.
	flavor := syncTestFlavor()
	flavor.Spec.Gpu.ResourceName = common.NvidiaGpu
	assert.Zero(t, buildNodeSyncPayload(n, flavor).GPUCount)
}

func TestNodeSyncPhase(t *testing.T) {
	n := syncManagedNode("n1", "1")
	assert.Equal(t, robustclient.NodeSyncPhaseReady, nodeSyncPhase(n))

	n.Status.Unschedulable = true
	assert.Equal(t, robustclient.NodeSyncPhaseUnschedulable, nodeSyncPhase(n))

	n.Status.MachineStatus.Phase = v1.NodeSSHFailed
	assert.Equal(t, robustclient.NodeSyncPhaseNotReady, nodeSyncPhase(n))

	managing := syncManagedNode("n2", "1")
	managing.Status.ClusterStatus.Phase = v1.NodeManaging
	assert.Equal(t, robustclient.NodeSyncPhaseNotReady, nodeSyncPhase(managing))
}

func TestBuildNodeSyncPayloadExternal(t *testing.T) {
	p := buildNodeSyncPayload(syncExternalNode("vk-1", "host-7", "203.0.113.5"), nil)
	assert.Equal(t, robustclient.NodeSyncLifecycleExternal, p.Lifecycle)
	assert.Equal(t, &robustclient.NodeSyncExternal{
		Provider: "spur", AllocationID: "alloc-1", Generation: 3, HostKey: "host-7",
	}, p.External)
	assert.Equal(t, "203.0.113.5", p.Address)
	assert.Equal(t, "vk-1", p.Hostname)
	assert.Equal(t, "ws-ext", p.Workspace)
	assert.Equal(t, robustclient.NodeSyncPhaseReady, p.Phase)
	assert.Equal(t, 8, p.GPUCount)

	// Before the provider freezes the host key and publishes the front-end IP.
	n := syncExternalNode("vk-2", "", "")
	n.Spec.PrivateIP = "10.9.9.9"
	p = buildNodeSyncPayload(n, nil)
	require.NotNil(t, p.External)
	assert.Empty(t, p.External.HostKey)
	assert.Empty(t, p.Address, "an external node never reports the private IP")
	body, err := json.Marshal(p)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "host_key")
	assert.NotContains(t, string(body), `"address"`)

	// A stale provider observation is not ready.
	stale := syncExternalNode("vk-3", "h", "")
	delete(stale.Annotations, v1.ExternalValidUntilAnnotation)
	assert.Equal(t, robustclient.NodeSyncPhaseNotReady, buildNodeSyncPayload(stale, nil).Phase)
}

func TestInNodeSyncScope(t *testing.T) {
	assert.False(t, inNodeSyncScope(nil))
	assert.True(t, inNodeSyncScope(syncManagedNode("n1", "1")))
	assert.True(t, inNodeSyncScope(syncExternalNode("vk-1", "", "")))

	unbound := syncManagedNode("n2", "1")
	delete(unbound.Labels, v1.ClusterIdLabel)
	assert.False(t, inNodeSyncScope(unbound), "a node outside any cluster")

	halfExternal := syncExternalNode("vk-2", "", "")
	halfExternal.Spec.ExternalRef = nil
	assert.False(t, inNodeSyncScope(halfExternal), "an external node without its allocation")

	other := syncManagedNode("n3", "1")
	other.Spec.LifecycleMode = "other"
	assert.False(t, inNodeSyncScope(other), "an unknown lifecycle")
}

func pushedNames(payloads []robustclient.NodeSyncPayload) []string {
	var names []string
	for _, p := range payloads {
		names = append(names, p.Name)
	}
	return names
}

func TestPlanNodeSync(t *testing.T) {
	now := metav1.Now()
	deleting := syncManagedNode("deleting", "9")
	deleting.DeletionTimestamp = &now
	nodes := []*v1.Node{
		syncManagedNode("missing", "5"),
		syncManagedNode("stale", "10"),
		syncManagedNode("current", "4"),
		syncManagedNode("restored", "6"),
		deleting,
	}
	digest := []robustclient.NodeSyncDigestEntry{
		{Name: "stale", SyncVersion: "3"},
		{Name: "current", SyncVersion: "4"},
		{Name: "restored", SyncVersion: "6", Deleted: true},
		{Name: "deleting", SyncVersion: "8"},
		{Name: "orphan", SyncVersion: "1"},
		{Name: "orphan-deleted", SyncVersion: "1", Deleted: true},
	}
	flavors := map[string]*v1.NodeFlavor{"mi300x": syncTestFlavor()}

	plan := planNodeSync(nodes, flavors, digest)
	assert.Equal(t, []string{"missing"}, pushedNames(plan.push[robustPushMissing]))
	assert.Equal(t, []string{"restored"}, pushedNames(plan.push[robustPushRestored]))
	assert.Equal(t, []string{"stale"}, pushedNames(plan.push[robustPushStale]))
	assert.Equal(t, "MI300X", plan.push[robustPushMissing][0].GPUModel)
	assert.ElementsMatch(t, []string{"deleting", "orphan"}, plan.delete)

	// An empty digest pushes every live node and deletes nothing.
	plan = planNodeSync(nodes, flavors, nil)
	assert.Len(t, plan.push[robustPushMissing], 4)
	assert.Empty(t, plan.delete)
}

// fakeNodeRobust serves the node sync API. A nil digest answers digestStatus,
// 404 by default, like a Robust that does not ship the node endpoints.
type fakeNodeRobust struct {
	mu           sync.Mutex
	digest       *robustclient.NodeSyncDigest
	digestStatus int
	pushed       []string
	batches      int
	deleted      []string
}

func (f *fakeNodeRobust) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/nodes/sync/digest":
			if f.digest == nil {
				if f.digestStatus == 0 {
					f.digestStatus = http.StatusNotFound
				}
				w.WriteHeader(f.digestStatus)
				return
			}
			_ = json.NewEncoder(w).Encode(f.digest)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes/sync":
			var p robustclient.NodeSyncPayload
			require.NoError(t, json.NewDecoder(r.Body).Decode(&p))
			f.pushed = append(f.pushed, p.Name)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/nodes/sync/batch":
			var b robustclient.NodeSyncBatchPayload
			require.NoError(t, json.NewDecoder(r.Body).Decode(&b))
			f.batches++
			f.pushed = append(f.pushed, pushedNames(b.Nodes)...)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v1/nodes/sync/"):
			f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/api/v1/nodes/sync/"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newNodeRobustSyncer(t *testing.T, objs ...client.Object) *NodeRobustSyncer {
	t.Helper()
	scheme, err := genMockScheme()
	require.NoError(t, err)
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &NodeRobustSyncer{Client: cl, robustClient: robustclient.NewClient(robustclient.ClientConfig{})}
}

func reconcileNode(t *testing.T, r *NodeRobustSyncer, name string) (ctrlruntime.Result, error) {
	t.Helper()
	return r.Reconcile(context.Background(), ctrlruntime.Request{NamespacedName: types.NamespacedName{Name: name}})
}

func TestNodeRobustReconcilePushesAndDeletes(t *testing.T) {
	node := syncManagedNode("n1", "")
	r := newNodeRobustSyncer(t, node, syncTestFlavor())

	// No Robust client for the cluster yet: left to the periodic reconcile, no requeue.
	res, err := reconcileNode(t, r, "n1")
	require.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)

	fr := &fakeNodeRobust{}
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)
	res, err = reconcileNode(t, r, "n1")
	require.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	assert.Equal(t, []string{"n1"}, fr.pushed)

	// The node is gone: Robust is told about it in the cluster it was pushed to.
	require.NoError(t, r.Delete(context.Background(), node))
	_, err = reconcileNode(t, r, "n1")
	require.NoError(t, err)
	assert.Equal(t, []string{"n1"}, fr.deleted)
	assert.Empty(t, r.lastCluster("n1"))
}

func TestNodeRobustReconcileDeletesTerminatingAndUnboundNodes(t *testing.T) {
	now := metav1.Now()
	terminating := syncManagedNode("terminating", "")
	terminating.DeletionTimestamp = &now
	terminating.Finalizers = []string{"f"}
	unbound := syncManagedNode("unbound", "")
	delete(unbound.Labels, v1.ClusterIdLabel)
	r := newNodeRobustSyncer(t, terminating, unbound)
	fr := &fakeNodeRobust{}
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)
	r.synced.Store("unbound", "c1")

	_, err := reconcileNode(t, r, "terminating")
	require.NoError(t, err)
	_, err = reconcileNode(t, r, "unbound")
	require.NoError(t, err)
	assert.Equal(t, []string{"terminating", "unbound"}, fr.deleted)
	assert.Empty(t, fr.pushed)
}

func TestNodeRobustReconcileHonoursClusterSwitch(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:        "c1",
		Annotations: map[string]string{annotationRobustWorkloadSync: "false"},
	}}
	r := newNodeRobustSyncer(t, syncManagedNode("n1", ""), cluster)
	fr := &fakeNodeRobust{}
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)

	_, err := reconcileNode(t, r, "n1")
	require.NoError(t, err)
	assert.Empty(t, fr.pushed)
}

// Until Robust ships node sync the endpoints answer 404: nothing is retried.
func TestNodeRobustReconcileNotSupported(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	r := newNodeRobustSyncer(t, syncManagedNode("n1", ""))
	r.robustClient.RegisterCluster("c1", srv.URL)

	res, err := reconcileNode(t, r, "n1")
	require.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	require.NoError(t, r.reconcileCluster(context.Background(), r.robustClient.ForCluster("c1"),
		[]*v1.Node{syncManagedNode("n1", "1")}, nil))
}

func countingServer(t *testing.T, status int) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":"a SaFE session or API key is required"}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

// A cluster the digest found without node sync gets no per-node calls.
func TestNodeRobustReconcileSkipsClusterWithoutNodeSync(t *testing.T) {
	srv, calls := countingServer(t, http.StatusOK)
	node := syncManagedNode("n1", "")
	r := newNodeRobustSyncer(t, node, syncTestFlavor())
	r.robustClient.RegisterCluster("c1", srv.URL)
	r.noNodeSync.Store("c1", true)

	res, err := reconcileNode(t, r, "n1")
	require.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)

	r.synced.Store("n1", "c1")
	require.NoError(t, r.Delete(context.Background(), node))
	res, err = reconcileNode(t, r, "n1")
	require.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	assert.Empty(t, r.lastCluster("n1"))
	assert.Zero(t, calls.Load())
}

// An older Robust guards the node routes with a session SaFE does not have:
// 401 or 403 marks the cluster as without node sync instead of requeueing.
func TestNodeRobustReconcileUnauthorizedMarksCluster(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv, calls := countingServer(t, status)
			r := newNodeRobustSyncer(t, syncManagedNode("n1", ""), syncTestFlavor())
			r.robustClient.RegisterCluster("c1", srv.URL)
			fallback := testutil.ToFloat64(robustNodeSyncFallback.WithLabelValues("c1"))

			res, err := reconcileNode(t, r, "n1")
			require.NoError(t, err)
			assert.Equal(t, ctrlruntime.Result{}, res)
			assert.True(t, r.hasNoNodeSync("c1"))
			assert.Equal(t, fallback+1, testutil.ToFloat64(robustNodeSyncFallback.WithLabelValues("c1")))
			assert.Empty(t, r.lastCluster("n1"))

			_, err = reconcileNode(t, r, "n1")
			require.NoError(t, err)
			assert.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestNodeRobustDeleteUnauthorizedMarksCluster(t *testing.T) {
	srv, calls := countingServer(t, http.StatusUnauthorized)
	r := newNodeRobustSyncer(t)
	r.robustClient.RegisterCluster("c1", srv.URL)
	r.synced.Store("gone", "c1")

	res, err := reconcileNode(t, r, "gone")
	require.NoError(t, err)
	assert.Equal(t, ctrlruntime.Result{}, res)
	assert.True(t, r.hasNoNodeSync("c1"))
	assert.Empty(t, r.lastCluster("gone"))
	assert.Equal(t, int32(1), calls.Load())
}

func TestNodeRobustReconcileReturnsPushErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	r := newNodeRobustSyncer(t, syncManagedNode("n1", ""))
	r.robustClient.RegisterCluster("c1", srv.URL)

	_, err := reconcileNode(t, r, "n1")
	require.Error(t, err)
	assert.Empty(t, r.lastCluster("n1"))
}

func TestNodeReconcileClusterAppliesPlan(t *testing.T) {
	fr := &fakeNodeRobust{digest: &robustclient.NodeSyncDigest{Nodes: []robustclient.NodeSyncDigestEntry{
		{Name: "stale", SyncVersion: "1"},
		{Name: "current", SyncVersion: "2"},
		{Name: "orphan", SyncVersion: "1"},
	}}}
	r := newNodeRobustSyncer(t)
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)
	nodes := []*v1.Node{syncManagedNode("missing", "1"), syncManagedNode("stale", "2"), syncManagedNode("current", "2")}

	require.NoError(t, r.reconcileCluster(context.Background(), r.robustClient.ForCluster("c1"), nodes, nil))
	assert.ElementsMatch(t, []string{"missing", "stale"}, fr.pushed)
	assert.Equal(t, []string{"orphan"}, fr.deleted)
}

// A Robust whose node digest answers 404 or 501 has no node sync: the cluster
// is skipped every round without an error, reported once, and synced again
// once the digest is served.
func TestNodeReconcileClusterDigestNotSupported(t *testing.T) {
	for cluster, status := range map[string]int{"c404": http.StatusNotFound, "c501": http.StatusNotImplemented} {
		t.Run(cluster, func(t *testing.T) {
			fr := &fakeNodeRobust{digestStatus: status}
			node := syncManagedNode("a", "1")
			node.Labels[v1.ClusterIdLabel] = cluster
			r := newNodeRobustSyncer(t, node)
			r.robustClient.RegisterCluster(cluster, fr.serve(t).URL)
			fallback := testutil.ToFloat64(robustNodeSyncFallback.WithLabelValues(cluster))
			reconcileErrors := testutil.ToFloat64(robustNodeSyncReconcileErrors.WithLabelValues(cluster))

			for round := 0; round < 2; round++ {
				r.reconcileAll(context.Background())
			}
			assert.Empty(t, fr.pushed)
			assert.Empty(t, fr.deleted)
			assert.Equal(t, fallback+2, testutil.ToFloat64(robustNodeSyncFallback.WithLabelValues(cluster)))
			assert.Equal(t, reconcileErrors, testutil.ToFloat64(robustNodeSyncReconcileErrors.WithLabelValues(cluster)))
			_, reported := r.noNodeSync.Load(cluster)
			assert.True(t, reported)

			fr.mu.Lock()
			fr.digest = &robustclient.NodeSyncDigest{}
			fr.mu.Unlock()
			r.reconcileAll(context.Background())
			assert.Equal(t, []string{"a"}, fr.pushed)
			_, reported = r.noNodeSync.Load(cluster)
			assert.False(t, reported)
		})
	}
}

// The loop reconciles only in-scope nodes, grouped by cluster.
func TestNodeReconcileAllScopesNodes(t *testing.T) {
	unbound := syncManagedNode("unbound", "")
	delete(unbound.Labels, v1.ClusterIdLabel)
	other := syncManagedNode("other", "")
	other.Labels[v1.ClusterIdLabel] = "c2"
	r := newNodeRobustSyncer(t, syncManagedNode("n1", ""), unbound, other, syncTestFlavor())
	fr := &fakeNodeRobust{digest: &robustclient.NodeSyncDigest{}}
	r.robustClient.RegisterCluster("c1", fr.serve(t).URL)

	r.reconcileAll(context.Background())
	assert.Equal(t, []string{"n1"}, fr.pushed)
}
