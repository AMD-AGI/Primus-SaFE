/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func ownedNode(name, workspace string) *v1.Node {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if workspace != "" {
		node.Spec.Workspace = ptr.To(workspace)
		node.Labels[v1.WorkspaceIdLabel] = workspace
	}
	return node
}

// workspaceNamed is the requesting Workspace, reduced to the only thing
// updateSingleNodeBinding reads off it: its name. It is also the object events
// get attached to, so it has to be a real one and not nil.
func workspaceNamed(name string) *v1.Workspace {
	return &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}}
}

// clusterNodes builds a client factory for a cluster holding exactly the named k8s nodes.
func clusterNodes(names ...string) *commonclient.ClientFactory {
	objs := make([]runtime.Object, 0, len(names))
	for _, name := range names {
		objs = append(objs, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}
	return commonclient.NewClientFactoryWithOnlyClient(context.Background(), "c1", k8sfake.NewClientset(objs...))
}

func TestObserveNodeForAllReleasesEveryWaiter(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	r.setExpectations("ws1", sets.NewSetByKeys("n1", "n2"))
	r.setExpectations("ws2", sets.NewSetByKeys("n1"))
	r.setExpectations("ws3", sets.NewSetByKeys("n3"))

	released := r.observeNodeForAll("n1")
	assert.Equal(t, len(released), 2)
	assert.Equal(t, r.expectations["ws1"].Has("n1"), false)
	assert.Equal(t, r.expectations["ws1"].Has("n2"), true)
	assert.Equal(t, r.meetExpectations("ws2"), true)
	assert.Equal(t, r.meetExpectations("ws3"), false)
}

// A node can be taken over between the moment a workspace registers an expectation on it and
// the moment the node event arrives. The event then carries the new owner only, so the
// original workspace must still be released or its reconcile is blocked for good.
func TestHandleNodeEventReleasesStolenNode(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	r.setExpectations("ws1", sets.NewSetByKeys("n1"))
	h := r.handleNodeEvent().(genericEventHandler)
	q := resWorkQueue()
	defer q.ShutDown()

	oldNode := ownedNode("n1", "")
	newNode := ownedNode("n1", "ws2")
	h.Update(context.Background(), event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, q)

	assert.Equal(t, r.meetExpectations("ws1"), true)
	// ws1 is re-queued so it can pick a different node, ws2 because it now owns one more.
	assert.Equal(t, q.Len(), 2)
}

func TestReservedNodesCoversInFlightAndPendingActions(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	pending := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws2", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{
			"n-add": v1.NodeActionAdd, "n-remove": v1.NodeActionRemove,
		})),
	}}}
	own := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n-own": v1.NodeActionAdd})),
	}}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(pending, own).Build()
	r := newMockWorkspaceReconciler(cl)
	r.setExpectations("ws1", sets.NewSetByKeys("n-self"))
	r.setExpectations("ws2", sets.NewSetByKeys("n-inflight"))

	reserved, err := r.reservedNodes(context.Background(), &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}})
	assert.NilError(t, err)
	assert.Equal(t, reserved.Has("n-inflight"), true)
	assert.Equal(t, reserved.Has("n-add"), true)
	// Unbinding frees a node up, and a workspace never reserves against itself.
	assert.Equal(t, reserved.Has("n-remove"), false)
	assert.Equal(t, reserved.Has("n-self"), false)
	assert.Equal(t, reserved.Has("n-own"), false)
}

func TestUpdateSingleNodeBindingRefusesBoundNode(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "ws2")
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node).Build()
	r := newMockWorkspaceReconciler(cl)

	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), node.DeepCopy(), "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "ws2")
}

func TestUpdateSingleNodeBindingBindsFreeNodeAndUnbinds(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node).Build()
	r := newMockWorkspaceReconciler(cl)

	free := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, free))
	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), free, "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, true)

	bound := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, bound))
	assert.Equal(t, bound.GetSpecWorkspace(), "ws1")

	// Unbinding the owner is always allowed; that is how a node changes hands.
	updated, err = r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), bound, "")
	assert.NilError(t, err)
	assert.Equal(t, updated, true)
}

// A node object we no longer hold the latest version of must never silently overwrite whoever
// bound it in the meantime. The fresh read that opens every attempt catches this one before a
// patch is ever built, so it ends as a refusal rather than as an error -- the same answer the
// caller would have got had it read fresh state to begin with. The optimistic lock is still
// there behind it, for the writes that land in the window between that read and the patch.
func TestUpdateSingleNodeBindingRejectsStaleWrite(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node).Build()
	r := newMockWorkspaceReconciler(cl)

	stale := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stale))
	// Someone else moves the node on before our patch lands.
	winner := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, winner))
	_, err = r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws2"), winner, "ws2")
	assert.NilError(t, err)

	// Our copy still shows the node as free, but its resourceVersion is behind.
	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), stale, "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "ws2")
}

func TestProcessNodesActionSkipsNodeOwnedByAnotherWorkspace(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "ws2")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n1": v1.NodeActionAdd})),
	}}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node, workspace).Build()
	r := newMockWorkspaceReconciler(cl)

	isUpdated, err := r.processNodesAction(context.Background(), workspace, clusterNodes("n1"))
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "ws2")
	// The action is cleared instead of being retried forever against a node we must not take.
	assert.Equal(t, v1.GetWorkspaceNodesAction(workspace), "")
}

func TestProcessNodesActionSkipsNodeMissingFromCluster(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	node.Spec.Hostname = ptr.To("n1")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n1": v1.NodeActionAdd})),
	}}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node, workspace).Build()
	r := newMockWorkspaceReconciler(cl)

	// The cluster holds no k8s node, so the workspace label could never come back and the
	// expectation would never be settled.
	isUpdated, err := r.processNodesAction(context.Background(), workspace, clusterNodes())
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(workspace), "")
	assert.Equal(t, r.meetExpectations("ws1"), true)
}

func TestProcessNodesActionBindsNodePresentInCluster(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	node.Spec.Hostname = ptr.To("n1")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n1": v1.NodeActionAdd})),
	}}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node, workspace).Build()
	r := newMockWorkspaceReconciler(cl)

	isUpdated, err := r.processNodesAction(context.Background(), workspace, clusterNodes("n1"))
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, true)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "ws1")
}

func TestSyncK8sMetadataIgnoresConflictingWorkspaceLabel(t *testing.T) {
	r := newNodeK8sReconciler(t, ownedNode("n1", "ws1"))
	adminNode := &v1.Node{}
	assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, adminNode))
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws2"},
	}}

	assert.NilError(t, r.syncK8sMetadata(context.Background(), adminNode, k8sNode))
	// The data plane says ws2, the admin plane bound the node to ws1. The admin plane wins.
	assert.Equal(t, v1.GetWorkspaceId(adminNode), "ws1")
}

func TestSyncK8sMetadataAcceptsConfirmedWorkspaceLabel(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}
	node.Spec.Workspace = ptr.To("ws1")
	r := newNodeK8sReconciler(t, node)
	adminNode := &v1.Node{}
	assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, adminNode))
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"},
	}}

	assert.NilError(t, r.syncK8sMetadata(context.Background(), adminNode, k8sNode))
	assert.Equal(t, v1.GetWorkspaceId(adminNode), "ws1")
}

func TestSyncK8sMetadataClearsWorkspaceLabelDroppedByDataPlane(t *testing.T) {
	node := ownedNode("n1", "ws1")
	node.Spec.Workspace = nil
	r := newNodeK8sReconciler(t, node)
	adminNode := &v1.Node{}
	assert.NilError(t, r.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, adminNode))
	k8sNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n1"}}

	// Unbinding still works: the label goes away once the data plane no longer carries it.
	assert.NilError(t, r.syncK8sMetadata(context.Background(), adminNode, k8sNode))
	assert.Equal(t, v1.GetWorkspaceId(adminNode), "")
}

// The ownership guard has to be reachable on real, freshly-read state, not only on a node
// object a test hand-built. Every caller filters bound nodes out before calling here, so
// without the re-read between attempts the guard could only ever repeat the answer that
// filter already gave -- an assertion, not a check.
func TestUpdateSingleNodeBindingRetriesOnUnrelatedConflict(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	conflicts := 0
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c ctrlclient.WithWatch, obj ctrlclient.Object,
				patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
				// NodeK8sReconciler mirrors kubelet's node conditions, whose heartbeat moves
				// the admin Node's resourceVersion every few seconds. That is a conflict the
				// binder must absorb, not a competing binder.
				if conflicts == 0 {
					conflicts++
					return apierrors.NewConflict(schema.GroupResource{Resource: "nodes"}, obj.GetName(), errors.New("stale"))
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cl)

	free := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, free))
	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), free, "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, true)
	assert.Equal(t, conflicts, 1)

	bound := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, bound))
	assert.Equal(t, bound.GetSpecWorkspace(), "ws1")
}

// The bind guards all key on a non-empty target, so before this check the unbind path was the
// one way left to take a node from another workspace: post {"n1":"remove"} for a node you do
// not own and it goes back to the pool.
func TestProcessNodesActionRefusesToUnbindAnotherWorkspacesNode(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "ws2")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n1": v1.NodeActionRemove})),
	}}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node, workspace).Build()
	r := newMockWorkspaceReconciler(cl)

	isUpdated, err := r.processNodesAction(context.Background(), workspace, clusterNodes("n1"))
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "ws2")
}

// A workspace under deletion is handed straight to delete() and never processes its
// annotation, so its claims are abandoned rather than pending. Honouring them would reserve
// those nodes against every other workspace for as long as the object lingers.
func TestReservedNodesIgnoresDeletingWorkspace(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	deleting := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name:              "ws2",
		DeletionTimestamp: &metav1.Time{Time: time.Unix(1, 0)},
		Finalizers:        []string{"safe.amd.com/workspace"},
		Annotations: map[string]string{
			v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n-add": v1.NodeActionAdd})),
		},
	}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(deleting).Build()
	r := newMockWorkspaceReconciler(cl)

	reserved, err := r.reservedNodes(context.Background(), &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}})
	assert.NilError(t, err)
	assert.Equal(t, reserved.Has("n-add"), false)
}

// "Better to skip a round of scaling up than to race an explicit binding" is only true if the
// round is actually skipped. Returning the partial set did the opposite of what it claimed:
// every node claimed by an annotation we could not read became fair game.
func TestReservedNodesFailsClosedWhenTheListFails(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	boom := errors.New("apiserver unavailable")
	cl := newWorkspaceClientBuilder(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c ctrlclient.WithWatch, list ctrlclient.ObjectList,
				opts ...ctrlclient.ListOption) error {
				if _, ok := list.(*v1.WorkspaceList); ok {
					return boom
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cl)

	_, err = r.reservedNodes(context.Background(), &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}})
	assert.ErrorContains(t, err, "apiserver unavailable")

	// And the scale-up round it feeds stops there rather than binding unreserved nodes.
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{NodeFlavor: "flavor-a", Cluster: "c1"},
	}
	_, err = r.getNodesForScalingUp(context.Background(), workspace, clusterNodes("n1"), 1)
	assert.ErrorContains(t, err, "apiserver unavailable")
}

func TestJudgeNodeBinding(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		current, target, request string
		want                     nodeBindVerdict
	}{
		{"bind a free node", "", "ws1", "ws1", bindProceed},
		{"bind what we already hold", "ws1", "ws1", "ws1", bindSettled},
		// The rule the whole branch exists for, in both directions.
		{"bind a node someone else holds", "ws2", "ws1", "ws1", bindRefused},
		{"unbind our own node", "ws1", "", "ws1", bindProceed},
		{"unbind someone else's node", "ws2", "", "ws1", bindRefused},
		{"unbind a node nobody holds", "", "", "ws1", bindSettled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			verdict, reason := judgeNodeBinding(tc.current, tc.target, tc.request)
			assert.Equal(t, verdict, tc.want)
			// A refusal that says nothing is a silent drop in the log.
			assert.Equal(t, reason != "", tc.want == bindRefused)
		})
	}
}

// The rule now also holds at the layer that writes, not only at the layer that assembles the
// request -- so an unbind that reached here from anywhere is still checked against the owner.
func TestUpdateSingleNodeBindingRefusesUnbindByNonOwner(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "ws2")
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node).Build()
	r := newMockWorkspaceReconciler(cl)

	owned := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, owned))
	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), owned, "")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "ws2")
}

// Settling an expectation has to drop the map entry, not leave an empty set behind. Only the
// delete path ever removed one, so a workspace that merely scaled kept its entry for the life
// of the process -- and every admin Node event walks the whole map under the write lock.
func TestObserveNodePrunesSettledExpectations(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	r.setExpectations("ws1", sets.NewSetByKeys("n1", "n2"))
	r.setExpectations("ws2", sets.NewSetByKeys("n1"))

	r.observeNode("ws1", "n1")
	_, stillTracked := r.expectations["ws1"]
	assert.Equal(t, stillTracked, true, "ws1 is still waiting on n2")

	r.observeNode("ws1", "n2")
	_, stillTracked = r.expectations["ws1"]
	assert.Equal(t, stillTracked, false)
	assert.Equal(t, r.meetExpectations("ws1"), true)

	// And the same through the path node events take.
	assert.Equal(t, len(r.observeNodeForAll("n1")), 1)
	assert.Equal(t, len(r.expectations), 0)
}

// Reservations are per cluster. A workspace elsewhere cannot be laying claim to this
// cluster's nodes, and parsing its annotation to find that out is the bulk of the work.
func TestReservedNodesIgnoresOtherClusters(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	elsewhere := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-b", Annotations: map[string]string{
			v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n-b": v1.NodeActionAdd})),
		}},
		Spec: v1.WorkspaceSpec{Cluster: "c2"},
	}
	sameCluster := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-a", Annotations: map[string]string{
			v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n-a": v1.NodeActionAdd})),
		}},
		Spec: v1.WorkspaceSpec{Cluster: "c1"},
	}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(elsewhere, sameCluster).Build()
	r := newMockWorkspaceReconciler(cl)

	reserved, err := r.reservedNodes(context.Background(), &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws1"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	})
	assert.NilError(t, err)
	assert.Equal(t, reserved.Has("n-a"), true)
	assert.Equal(t, reserved.Has("n-b"), false)
}

// The informer is attached per cluster by NodeK8sReconciler, so a workspace reconcile can run
// before there is any cache to read. Falling back to the apiserver is what keeps that from
// reading as an empty cluster -- which here would mean "the node is gone".
func TestGetCachedDataPlaneNodeFallsBackWhenThereIsNoInformer(t *testing.T) {
	clients := clusterNodes("k8s-n1")
	assert.Assert(t, dataPlaneNodeLister(clients) == nil)

	node, err := getCachedDataPlaneNode(context.Background(), clients, "k8s-n1")
	assert.NilError(t, err)
	assert.Equal(t, node.Name, "k8s-n1")

	_, err = getCachedDataPlaneNode(context.Background(), clients, "absent")
	assert.Assert(t, apierrors.IsNotFound(err))
}

// clusterNodesWithInformer is clusterNodes with the shared informer the production factories
// carry, started and synced. Without it every test here takes the fallback path, which is the
// one branch that did not need testing.
func clusterNodesWithInformer(t *testing.T, names ...string) (*commonclient.ClientFactory, *k8sfake.Clientset) {
	t.Helper()
	objs := make([]runtime.Object, 0, len(names))
	for _, name := range names {
		objs = append(objs, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}})
	}
	clientSet := k8sfake.NewClientset(objs...)
	clients := commonclient.NewClientFactoryForTestWithInformer("c1", clientSet)
	// Registering has to happen before Start: the factory only runs the informers it has
	// been asked for by then. This is what attachNodeInformer does in production.
	clients.SharedInformerFactory().Core().V1().Nodes().Informer()
	clients.StartInformer()
	assert.Assert(t, clients.WaitForCacheSync(10*time.Second))
	t.Cleanup(func() { _ = clients.Release() })
	return clients, clientSet
}

// countGets records reads that reached the apiserver. The informer lists and watches, it
// never gets, so this counts exactly the round trips the lister was meant to save.
func countGets(clientSet *k8sfake.Clientset, n *int) {
	clientSet.PrependReactor("get", "nodes", func(k8stesting.Action) (bool, runtime.Object, error) {
		*n++
		return false, nil, nil
	})
}

func TestGetCachedDataPlaneNodeReadsTheInformerNotTheApiserver(t *testing.T) {
	clients, clientSet := clusterNodesWithInformer(t, "k8s-n1")
	assert.Assert(t, dataPlaneNodeLister(clients) != nil)
	gets := 0
	countGets(clientSet, &gets)

	node, err := getCachedDataPlaneNode(context.Background(), clients, "k8s-n1")
	assert.NilError(t, err)
	assert.Equal(t, node.Name, "k8s-n1")
	assert.Equal(t, gets, 0, "the whole point of the lister is that this costs no round trip")

	// The lister hands out pointers into the shared cache, which every caller here goes on
	// to mutate or store. Without the copy this scribbles on the informer's own object.
	node.Labels = map[string]string{"scribbled": "on"}
	again, err := getCachedDataPlaneNode(context.Background(), clients, "k8s-n1")
	assert.NilError(t, err)
	assert.Equal(t, len(again.Labels), 0)

	_, err = getCachedDataPlaneNode(context.Background(), clients, "absent")
	assert.Assert(t, apierrors.IsNotFound(err))
}

// The other half of the split. k8sNodeUnavailableReason and NodeReconciler.getK8sNode must keep
// reaching the apiserver even where an informer is available -- one reads NotFound as a
// permanent refusal, the other writes back what it reads.
func TestGetDataPlaneNodeIgnoresTheInformer(t *testing.T) {
	clients, clientSet := clusterNodesWithInformer(t, "k8s-n1")
	assert.Assert(t, dataPlaneNodeLister(clients) != nil)
	gets := 0
	countGets(clientSet, &gets)

	node, err := getDataPlaneNode(context.Background(), clients, "k8s-n1")
	assert.NilError(t, err)
	assert.Equal(t, node.Name, "k8s-n1")
	assert.Equal(t, gets, 1)
}

func conflictOnPatch(count *int) interceptor.Funcs {
	return interceptor.Funcs{
		Patch: func(_ context.Context, _ ctrlclient.WithWatch, obj ctrlclient.Object,
			_ ctrlclient.Patch, _ ...ctrlclient.PatchOption) error {
			*count++
			return apierrors.NewConflict(v1.Resource("nodes"), obj.GetName(),
				errors.New("the object has been modified"))
		},
	}
}

// The re-read between attempts is the only thing that makes the ownership check a check, and
// it is worth nothing if it is served from the same cache that produced the stale object. The
// manager's typed client is exactly that cache, and one retry budget is ~150ms of backoff --
// no basis for assuming a watch has delivered the write that just won.
func TestUpdateSingleNodeBindingRereadsThroughTheUncachedReader(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	patches := 0
	// What the cache still shows: free, and so worth binding.
	cached := newWorkspaceClientBuilder(scheme).
		WithObjects(ownedNode("n1", "")).
		WithInterceptorFuncs(conflictOnPatch(&patches)).Build()
	r := newMockWorkspaceReconciler(cached)
	// What is actually stored: ws2 got there first, which is why the patch conflicted.
	r.apiReader = newWorkspaceClientBuilder(scheme).WithObjects(ownedNode("n1", "ws2")).Build()

	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), ownedNode("n1", ""), "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)
	// Not one wasted patch: the read comes first, so the refusal happens before anything is
	// written. Reading the cache instead would have kept saying "free" and burned the whole
	// budget re-patching a node ws2 owns.
	assert.Equal(t, patches, 0)
}

// A node that started deleting between the caller's filter and this retry must not be *bound*:
// judgeNodeBinding only knows about ownership, so the check has to live here.
//
// Note this is the bind half. The unbind half is the test below, and the two cannot share a
// guard -- see TestUpdateSingleNodeBindingReleasesANodeThatStartedDeleting.
func TestUpdateSingleNodeBindingSkipsANodeThatStartedDeleting(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	deleting := ownedNode("n1", "")
	deleting.DeletionTimestamp = &metav1.Time{Time: time.Unix(1, 0)}
	deleting.Finalizers = []string{"safe.amd.com/node"}
	patches := 0
	cached := newWorkspaceClientBuilder(scheme).
		WithObjects(ownedNode("n1", "")).
		WithInterceptorFuncs(conflictOnPatch(&patches)).Build()
	r := newMockWorkspaceReconciler(cached)
	r.apiReader = newWorkspaceClientBuilder(scheme).WithObjects(deleting).Build()

	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), ownedNode("n1", ""), "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)
	assert.Equal(t, patches, 0)
}

// The unbind half, which the deletion guard above must not catch.
//
// delete() collects every node whose spec.workspace names the workspace -- a deleting node is
// still one of them -- and settles each node's expectation on !updated. A refusal here answers
// "nothing to do", so meetExpectations passes and the finalizer comes off with the node still
// naming a Workspace that no longer exists. judgeNodeBinding then refuses every other
// workspace for that node, forever, because the only one allowed to release it is gone.
//
// The release has to actually land, not merely be attempted: it is the spec write that stops
// the node being unrescuable once the Workspace is collected.
func TestUpdateSingleNodeBindingReleasesANodeThatStartedDeleting(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	deleting := ownedNode("n1", "ws1")
	deleting.DeletionTimestamp = &metav1.Time{Time: time.Unix(1, 0)}
	deleting.Finalizers = []string{"safe.amd.com/node"}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(deleting).Build()
	r := newMockWorkspaceReconciler(cl)

	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), deleting.DeepCopy(), "")
	assert.NilError(t, err)
	assert.Equal(t, updated, true)

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "")
}

// The same thing end to end, which is where it actually bit.
//
// The workspace is deleted while it still owns a node that has itself started deleting. Both
// halves have to hold: the node is released, and the Workspace keeps its finalizer until that
// release is confirmed. Settling on an unbind that never happened is what let the Workspace go
// first and stranded the node.
func TestDeleteReleasesANodeThatStartedDeleting(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "ws1")
	node.DeletionTimestamp = &metav1.Time{Time: time.Unix(1, 0)}
	node.Finalizers = []string{"safe.amd.com/node"}
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name:       "ws1",
		Finalizers: []string{v1.WorkspaceFinalizer},
	}}
	cl := newWorkspaceClientBuilder(scheme).
		WithStatusSubresource(&v1.Workspace{}).
		WithObjects(node, workspace).Build()
	r := newMockWorkspaceReconciler(cl)

	assert.NilError(t, r.delete(context.Background(), workspace))

	stored := &v1.Node{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "n1"}, stored))
	assert.Equal(t, stored.GetSpecWorkspace(), "", "the node must not be left naming a workspace that is going away")

	// Still held: the unbind moved spec, and the expectation waits for the workspace label,
	// which handleNodeEvent's DeleteFunc will settle when the node object finally goes.
	ws := &v1.Workspace{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "ws1"}, ws))
	assert.Equal(t, len(ws.Finalizers), 1, "the workspace must outlive the node it just released")
}

// Losing every conflict has to be reported. It costs nothing to report: concurrent.Exec runs
// every goroutine to completion and returns the first error, so the other nodes in the same
// scale-up all get their turn either way, and the error buys a rate-limited requeue.
//
// Swallowing it is what costs. updated=false with err=nil reads as "there was nothing to do",
// which for a workspace being deleted means removeExpectations, then deleteDataPlaneResources,
// then RemoveFinalizer -- with the node still pointing at a Workspace that no longer exists.
// Nothing recovers that node afterwards: judgeNodeBinding refuses every other workspace, and
// the only workspace allowed to release it is gone.
func TestUpdateSingleNodeBindingReportsLosingEveryConflict(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	patches := 0
	cl := newWorkspaceClientBuilder(scheme).
		WithObjects(ownedNode("n1", "")).
		WithInterceptorFuncs(conflictOnPatch(&patches)).Build()
	r := newMockWorkspaceReconciler(cl)

	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), ownedNode("n1", ""), "ws1")
	assert.Assert(t, apierrors.IsConflict(err), "giving up has to reach the caller: %v", err)
	assert.Equal(t, updated, false)
	assert.Equal(t, patches, 5, "retry.DefaultRetry gives five attempts")
}

// The wedge this whole branch exists to remove, reachable through a node that carries the
// workspace label but no spec.workspace -- what a scaleDown or a delete() hands over after an
// earlier unbind wrote the label and nothing wrote the pointer.
//
// Writing spec.workspace="" over the nil "to make the release explicit" changes no behaviour
// (GetSpecWorkspace maps nil to "") but does report updated=true, which stops the caller from
// settling the expectation -- and the write cannot settle it either, because handleNodeEvent
// only observes on a change of the workspace *label*. The expectation then never clears and
// every later reconcile of the workspace returns early, deletion included.
func TestUnbindingANodeWithNoSpecWorkspaceSettlesItsExpectation(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:   "n1",
		Labels: map[string]string{v1.WorkspaceIdLabel: "ws1"},
	}}
	patches := 0
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c ctrlclient.WithWatch, obj ctrlclient.Object,
				patch ctrlclient.Patch, opts ...ctrlclient.PatchOption) error {
				patches++
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cl)

	err = r.updateNodesBinding(context.Background(),
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}},
		[]*v1.Node{node.DeepCopy()}, map[string]string{"n1": ""})
	assert.NilError(t, err)
	assert.Equal(t, patches, 0, "there is nothing to release; nil already reads as unbound")
	assert.Equal(t, r.meetExpectations("ws1"), true, "the workspace is wedged if this is false")
}

// A deleting workspace's in-flight operations are unbinds -- it is releasing these nodes, not
// claiming them. Reserving them keeps them out of every other workspace's reach for as long as
// the entry lives, which is the life of the process if its own deletion is stuck.
func TestReservedNodesIgnoresADeletingWorkspacesExpectations(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	deleting := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name:              "ws2",
		DeletionTimestamp: &metav1.Time{Time: time.Unix(1, 0)},
		Finalizers:        []string{"safe.amd.com/workspace"},
	}}
	live := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws3"}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(deleting, live).Build()
	r := newMockWorkspaceReconciler(cl)
	r.setExpectations("ws2", sets.NewSetByKeys("n-releasing"))
	r.setExpectations("ws3", sets.NewSetByKeys("n-claiming"))
	// Nobody by this name is in the list. Absence is served from a cache, so it is as likely
	// to mean "created a moment ago" as "gone", and an unattributable bind stays reserved.
	r.setExpectations("ws-unknown", sets.NewSetByKeys("n-mystery"))

	reserved, err := r.reservedNodes(context.Background(), &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}})
	assert.NilError(t, err)
	assert.Equal(t, reserved.Has("n-releasing"), false)
	assert.Equal(t, reserved.Has("n-claiming"), true)
	assert.Equal(t, reserved.Has("n-mystery"), true)
}

// What giving up silently used to cost, end to end.
//
// delete() unbinds every node the workspace still owns, and only once that is done does it
// drop the finalizer and let the Workspace go. When updateSingleNodeBinding answered
// "updated=false, err=nil" after losing all five conflicts, the node's expectation was
// settled, meetExpectations said yes, and the finalizer came off with the node's
// spec.workspace still naming a Workspace that no longer exists -- and no later bind can
// clear it, because judgeNodeBinding refuses every workspace except the one that is gone.
func TestDeleteKeepsItsFinalizerWhenANodeCannotBeReleased(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "ws1")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name:       "ws1",
		Finalizers: []string{v1.WorkspaceFinalizer},
	}}
	patches := 0
	cl := newWorkspaceClientBuilder(scheme).
		WithStatusSubresource(&v1.Workspace{}).
		WithObjects(node, workspace).
		WithInterceptorFuncs(conflictOnPatch(&patches)).Build()
	r := newMockWorkspaceReconciler(cl)

	err = r.delete(context.Background(), workspace)
	assert.Assert(t, apierrors.IsConflict(err), "the unbind failed and delete has to say so: %v", err)
	assert.Equal(t, patches, 5)

	stored := &v1.Workspace{}
	assert.NilError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: "ws1"}, stored))
	assert.Assert(t, len(stored.Finalizers) == 1, "the workspace must outlive the node it still owns")
}

// drainEvents reads whatever the recorder has buffered. FakeRecorder's send is blocking, so
// nothing may be left in the channel by the time the test ends -- and nothing is, because
// every emitter here runs to completion before the assertion.
func drainEvents(recorder *record.FakeRecorder) []string {
	var out []string
	for {
		select {
		case e := <-recorder.Events:
			out = append(out, e)
		default:
			return out
		}
	}
}

// A refusal is a decision about a request a user made through the API. That request already
// returned 200, the annotation carrying it is cleared on the way out, and the resource-manager
// log is not somewhere a user can look -- so the Workspace has to carry the answer.
func TestUpdateSingleNodeBindingEventsTheRefusal(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	cl := newWorkspaceClientBuilder(scheme).WithObjects(ownedNode("n1", "ws2")).Build()
	r := newMockWorkspaceReconciler(cl)
	recorder := record.NewFakeRecorder(4)
	r.recorder = recorder

	updated, err := r.updateSingleNodeBinding(context.Background(), workspaceNamed("ws1"), ownedNode("n1", ""), "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)

	events := drainEvents(recorder)
	assert.Equal(t, len(events), 1)
	assert.Assert(t, strings.Contains(events[0], eventNodeBindRefused), events[0])
	assert.Assert(t, strings.Contains(events[0], "ws2"), "the event has to name who holds the node: %s", events[0])
}

// The other permanent drop on the bind path, and the one furthest from anything a user can
// see: the node exists in the admin plane, so the request looks reasonable, but the cluster
// has no k8s node to put the label on and the bind would wait forever.
func TestProcessNodesActionEventsAMissingK8sNode(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	node.Spec.Hostname = ptr.To("n1")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n1": v1.NodeActionAdd})),
	}}}
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node, workspace).Build()
	r := newMockWorkspaceReconciler(cl)
	recorder := record.NewFakeRecorder(4)
	r.recorder = recorder

	_, err = r.processNodesAction(context.Background(), workspace, clusterNodes())
	assert.NilError(t, err)

	events := drainEvents(recorder)
	assert.Equal(t, len(events), 1)
	assert.Assert(t, strings.Contains(events[0], eventNodeUnavailable), events[0])
}

// The two ways a node can be unbindable-for-now have to read differently, because they call
// for different things: one is waited out, the other is a node that was never registered with
// the cluster it claims to be in.
func TestK8sNodeUnavailableReason(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	clients := clusterNodes("k8s-n1")

	present := ownedNode("n1", "")
	present.Spec.Hostname = ptr.To("k8s-n1")
	assert.Equal(t, r.k8sNodeUnavailableReason(context.Background(), clients, present), "")

	absent := ownedNode("n2", "")
	absent.Spec.Hostname = ptr.To("k8s-n2")
	assert.Assert(t, strings.Contains(
		r.k8sNodeUnavailableReason(context.Background(), clients, absent), "not in the cluster"))

	// No hostname yet: the node has not finished registering, so there is nothing to look up.
	// Distinct from the above -- this one resolves on its own.
	unnamed := ownedNode("n3", "")
	assert.Assert(t, strings.Contains(
		r.k8sNodeUnavailableReason(context.Background(), clients, unnamed), "not known yet"))
}

// A k8s node on its way out is a third answer, not a variant of "not in the cluster". The
// object is still there, so NotFound never fires, and the guard in updateSingleNodeBinding
// only reads the admin Node -- which is still healthy at this point, because it is deleted in
// response to its k8s node going away, not alongside it. Without this check the window in
// between is a bind that succeeds against a node that is about to stop existing.
func TestK8sNodeUnavailableReasonRefusesADeletingK8sNode(t *testing.T) {
	r := newMockWorkspaceReconciler(nil)
	going := &corev1.Node{ObjectMeta: metav1.ObjectMeta{
		Name:              "k8s-n1",
		DeletionTimestamp: &metav1.Time{Time: time.Now()},
		Finalizers:        []string{"safe.amd.com/test"},
	}}
	clients := commonclient.NewClientFactoryWithOnlyClient(
		context.Background(), "c1", k8sfake.NewClientset(going))

	node := ownedNode("n1", "")
	node.Spec.Hostname = ptr.To("k8s-n1")
	reason := r.k8sNodeUnavailableReason(context.Background(), clients, node)
	assert.Assert(t, strings.Contains(reason, "being deleted"), reason)
	// Distinguishable from the node that was never registered: same drop, different fix.
	assert.Assert(t, !strings.Contains(reason, "not in the cluster"), reason)
}

// An admin Node that is gone by the time updateSingleNodeBinding reads it is an answer, not a
// failure. There is no reference left for the workspace to hold, so updated=false is the whole
// truth and the caller settles the expectation on it. Reporting NotFound as an error made
// concurrent.Exec fail the batch instead, so a deleting workspace paid a rate-limited requeue
// and an error log every round for a node that had already stopped existing.
func TestUpdateNodesBindingSettlesAVanishedNodeWithoutAnError(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	// Nothing in the store: the object handed over is a leftover from a cached list.
	cl := newWorkspaceClientBuilder(scheme).Build()
	r := newMockWorkspaceReconciler(cl)
	gone := ownedNode("n1", "ws1")

	err = r.updateNodesBinding(context.Background(), workspaceNamed("ws1"),
		[]*v1.Node{gone}, map[string]string{"n1": ""})
	assert.NilError(t, err)
	// Settled, so the workspace's next pass is free to drop its finalizer -- which is safe
	// here precisely because the node it would have pointed at is gone.
	assert.Equal(t, r.meetExpectations("ws1"), true)
}

// The wedge from the other side: a node that is already in the state the action asks for as
// far as the *label* is concerned.
//
// An expectation waits for the workspace label to make the round trip through the data plane,
// and handleNodeEvent credits it only on a change of that label. A workspace deleted while a
// bind was still in flight leaves the node with spec.workspace cleared and the label never
// set, so the unbind's expectation is waiting for a transition that already happened. Nothing
// writes the admin Node again -- syncK8sMetadata correctly refuses to copy back a data-plane
// label that disagrees with spec -- and the Workspace sits in Terminating until the process
// restarts.
func TestUpdateNodesBindingSettlesANodeWhoseLabelAlreadyReadsTheTarget(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	// The bind wrote spec.workspace; the label never came back. delete() now unbinds, which
	// is a real patch (updated=true) -- and then waits for a label that is already gone.
	node := ownedNode("n1", "ws1")
	delete(node.Labels, v1.WorkspaceIdLabel)
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node).Build()
	r := newMockWorkspaceReconciler(cl)

	assert.NilError(t, r.updateNodesBinding(context.Background(), workspaceNamed("ws1"),
		[]*v1.Node{node.DeepCopy()}, map[string]string{"n1": ""}))
	assert.Equal(t, r.meetExpectations("ws1"), true,
		"nothing is going to move the label, so delete() would wait forever")
}

// And the ordinary case still waits: the label has to come back before the bind counts.
func TestUpdateNodesBindingStillWaitsForTheLabelRoundTrip(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	cl := newWorkspaceClientBuilder(scheme).WithObjects(node).Build()
	r := newMockWorkspaceReconciler(cl)

	assert.NilError(t, r.updateNodesBinding(context.Background(), workspaceNamed("ws1"),
		[]*v1.Node{node.DeepCopy()}, map[string]string{"n1": "ws1"}))
	assert.Equal(t, r.meetExpectations("ws1"), false)
}

// processNodesAction refuses, clears the annotation and emits an event -- none of which is
// retried. Deciding that from the manager's cache means a cache that has not yet caught up
// with a release turns a request the user is entitled to make into a permanent no.
func TestProcessNodesActionJudgesOwnershipUncached(t *testing.T) {
	scheme, err := genMockScheme()
	assert.NilError(t, err)
	node := ownedNode("n1", "")
	node.Spec.Hostname = ptr.To("n1")
	workspace := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1", Annotations: map[string]string{
		v1.WorkspaceNodesAction: string(jsonutils.MarshalSilently(map[string]string{"n1": v1.NodeActionAdd})),
	}}}
	// The cache is behind: it still shows ws2 holding a node ws2 has already given up.
	stale := ownedNode("n1", "ws2")
	stale.Spec.Hostname = ptr.To("n1")
	cl := newWorkspaceClientBuilder(scheme).WithObjects(stale, workspace).Build()
	r := newMockWorkspaceReconciler(cl)
	r.apiReader = newWorkspaceClientBuilder(scheme).WithObjects(node, workspace).Build()
	recorder := record.NewFakeRecorder(4)
	r.recorder = recorder

	isUpdated, err := r.processNodesAction(context.Background(), workspace, clusterNodes("n1"))
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, true)
	assert.Equal(t, len(drainEvents(recorder)), 0, "the node is free; there is nothing to refuse")
}
