/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"strings"
	"testing"

	"gotest.tools/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"

	k8sfake "k8s.io/client-go/kubernetes/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func TestJudgeNodeBinding(t *testing.T) {
	cases := []struct {
		name             string
		current, target  string
		requester        string
		expectedVerdict  nodeBindVerdict
		expectedInReason string
	}{
		{name: "bind a free node", current: "", target: "ws1", requester: "ws1",
			expectedVerdict: bindProceed},
		{name: "bind a node this workspace already holds", current: "ws1", target: "ws1",
			requester: "ws1", expectedVerdict: bindSettled},
		{name: "bind a node another workspace holds", current: "ws2", target: "ws1",
			requester: "ws1", expectedVerdict: bindRefused, expectedInReason: "already bound to ws2"},
		{name: "unbind a node this workspace holds", current: "ws1", target: "",
			requester: "ws1", expectedVerdict: bindProceed},
		{name: "unbind a node that is already free", current: "", target: "",
			requester: "ws1", expectedVerdict: bindSettled},
		// The one the three open-coded copies of this rule all missed: every one of them
		// keyed on a non-empty target, and an unbind's target is always empty.
		{name: "unbind a node another workspace holds", current: "ws2", target: "",
			requester: "ws1", expectedVerdict: bindRefused, expectedInReason: "bound to ws2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verdict, reason := judgeNodeBinding(c.current, c.target, c.requester)
			assert.Equal(t, verdict, c.expectedVerdict)
			if c.expectedInReason != "" {
				assert.Assert(t, strings.Contains(reason, c.expectedInReason), reason)
			}
		})
	}
}

// ownedNode returns a managed admin node bound to the given workspace, or free when it is "".
func ownedNode(name, workspaceId string) *v1.Node {
	node := genMockAdminNode(name, "cluster", genMockNodeFlavor())
	if workspaceId != "" {
		node.Spec.Workspace = pointer.String(workspaceId)
		metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, workspaceId)
	}
	return node
}

func storedNode(t *testing.T, cli client.Client, name string) *v1.Node {
	t.Helper()
	node := &v1.Node{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKey{Name: name}, node))
	return node
}

// A refusal has to reach the caller as an error. The nodes-action annotation is cleared only
// once the whole batch succeeds, and the mutating webhook has already applied the matching
// Spec.Replica change, so a swallowed refusal strands that replica and quietly turns the
// refused bind into an automatic scale-up onto a different node.
func TestUpdateSingleNodeBindingRefusesToTakeANodeFromItsOwner(t *testing.T) {
	node := ownedNode("node1", "ws2")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}

func TestUpdateSingleNodeBindingRefusesAnUnbindFromAnyoneButTheOwner(t *testing.T) {
	node := ownedNode("node1", "ws2")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "")
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "not the workspace asking")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}

func TestUpdateSingleNodeBindingSettlesWhenTheNodeIsAlreadyWhereItShouldBe(t *testing.T) {
	node := ownedNode("node1", "ws1")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)
}

// A node that is gone is an answer, not a failure. Reported as an error it would fail the
// whole batch and buy a rate-limited requeue every round for a request nothing can satisfy.
func TestUpdateSingleNodeBindingTreatsAVanishedNodeAsDone(t *testing.T) {
	cli := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", ownedNode("node1", ""), "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, false)
}

// Binds only. delete() collects deleting nodes too, and a refused unbind would let the
// Workspace finalizer come off with spec.workspace still naming it -- a node no later bind
// can rescue, because only the owner may release it and the owner is gone.
func TestUpdateSingleNodeBindingRefusesToBindADeletingNode(t *testing.T) {
	node := ownedNode("node1", "")
	node.Finalizers = []string{v1.NodeFinalizer}
	node.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "being deleted")
}

func TestUpdateSingleNodeBindingReleasesADeletingNode(t *testing.T) {
	node := ownedNode("node1", "ws1")
	node.Finalizers = []string{v1.NodeFinalizer}
	node.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "")
	assert.NilError(t, err)
	assert.Equal(t, updated, true)
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "")
}

// Losing the optimistic lock means somebody wrote first: read again, judge again. Here the
// second look still says the node is free, so the bind goes through on the retry.
func TestUpdateSingleNodeBindingRetriesAfterAConflict(t *testing.T) {
	node := ownedNode("node1", "")
	conflicts := 0
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if conflicts == 0 {
					conflicts++
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "nodes"}, obj.GetName(), nil)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.NilError(t, err)
	assert.Equal(t, updated, true)
	assert.Equal(t, conflicts, 1)
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws1")
}

// And when the second look says the node changed hands, the retry refuses instead of
// overwriting the workspace that won the race.
func TestUpdateSingleNodeBindingRejudgesAfterAConflict(t *testing.T) {
	node := ownedNode("node1", "")
	conflicts := 0
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if conflicts == 0 {
					conflicts++
					// The writer that beat us lands its bind before returning the conflict.
					winner := &v1.Node{}
					if err := c.Get(ctx, client.ObjectKey{Name: obj.GetName()}, winner); err != nil {
						return err
					}
					winner.Spec.Workspace = pointer.String("ws2")
					if err := c.Update(ctx, winner); err != nil {
						return err
					}
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "nodes"}, obj.GetName(), nil)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}

// The real typed client decodes into the object it is handed without zeroing it first, and
// Spec.Workspace is an omitempty pointer: for a node that is not bound the field is absent on
// the wire, so re-reading through the object a previous attempt already wrote to would leave
// that attempt's own value in place and the retry would judge its own writing as settled. The
// fake client zeroes, which is why this needs an interceptor to reproduce at all.
func TestUpdateSingleNodeBindingReadsIntoAFreshObject(t *testing.T) {
	node := ownedNode("node1", "")
	conflicts := 0
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				stored := &v1.Node{}
				if err := c.Get(ctx, key, stored, opts...); err != nil {
					return err
				}
				target, ok := obj.(*v1.Node)
				if !ok {
					return nil
				}
				// Copy field by field, leaving an absent Spec.Workspace untouched -- what a
				// real decode does, and what the fake client does not.
				target.ObjectMeta = stored.ObjectMeta
				target.Status = stored.Status
				target.Spec.NodeFlavor = stored.Spec.NodeFlavor
				target.Spec.Cluster = stored.Spec.Cluster
				target.Spec.Port = stored.Spec.Port
				if stored.Spec.Workspace != nil {
					target.Spec.Workspace = stored.Spec.Workspace
				}
				return nil
			},
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if conflicts == 0 {
					conflicts++
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "nodes"}, obj.GetName(), nil)
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.NilError(t, err)
	// Reading into the object the first attempt mutated would carry "ws1" into the retry's
	// judgement, which would then answer bindSettled and report nothing written.
	assert.Equal(t, updated, true)
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws1")
}

// An expectation waits for the workspace label to make the round trip through the data plane,
// and handleNodeEvent credits it only on a *change* of that label. A node whose label already
// reads the target has nothing left to wait for, and waiting anyway wedges the workspace:
// every later reconcile returns early on meetExpectations, including the one that deletes it.
func TestUpdateNodesBindingSettlesWhenTheLabelAlreadyReadsTheTarget(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "")
	metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, workspace.Name)
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	nodes := []*v1.Node{node}
	err := r.updateNodesBinding(context.Background(), workspace, nodes,
		buildTargetList(nodes, workspace.Name))
	assert.NilError(t, err)
	assert.Equal(t, r.meetExpectations(workspace.Name), true)
}

func TestUpdateNodesBindingWaitsWhenTheLabelStillHasToArrive(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "")
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	nodes := []*v1.Node{node}
	err := r.updateNodesBinding(context.Background(), workspace, nodes,
		buildTargetList(nodes, workspace.Name))
	assert.NilError(t, err)
	assert.Equal(t, r.meetExpectations(workspace.Name), false)
}

func TestObserveNodeForAllSettlesEveryWaitingWorkspace(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1", "node2"))
	r.setExpectations("ws2", sets.NewSetByKeys("node1"))
	r.setExpectations("ws3", sets.NewSetByKeys("node3"))

	settled := sets.NewSetByKeys(r.observeNodeForAll("node1")...)
	assert.Equal(t, settled.Len(), 2)
	assert.Equal(t, settled.Has("ws1"), true)
	assert.Equal(t, settled.Has("ws2"), true)
	assert.Equal(t, r.meetExpectations("ws1"), false)
	assert.Equal(t, r.meetExpectations("ws2"), true)
	// The emptied entry is dropped, not left behind: this map is walked on every admin Node
	// event, and NodeK8sReconciler writes those nodes every few seconds.
	_, stillThere := r.expectations["ws2"]
	assert.Equal(t, stillThere, false)
}

// The wedge crediting by the event's workspace id alone leaves behind: the bind landed, so
// the workspace is waiting, and then the node is deleted. Its last event names nobody.
func TestHandleNodeEventSettlesAWaitingWorkspaceWhenTheNodeIsDeleted(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))

	q := resWorkQueue()
	defer q.ShutDown()
	r.handleNodeEvent().Delete(context.Background(),
		event.DeleteEvent{Object: ownedNode("node1", "")}, q)
	assert.Equal(t, r.meetExpectations("ws1"), true)
	// Settling alone is not enough -- the workspace also has to be re-queued, or nothing
	// looks at it again until the next unrelated event.
	assert.Equal(t, q.Len(), 1)
	item, _ := q.Get()
	assert.Equal(t, item.Name, "ws1")
}

func TestReservedNodesCoversOtherWorkspacesPendingClaims(t *testing.T) {
	mine := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(mine, map[string]string{"node1": v1.NodeActionAdd})
	other := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(other, map[string]string{"node2": v1.NodeActionAdd, "node3": v1.NodeActionRemove})
	// A workspace under deletion never processes its annotation -- Reconcile hands it to
	// delete() -- so its claims are abandoned, and reserving them would take node4 out of
	// everyone's reach for good.
	dying := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(dying, map[string]string{"node4": v1.NodeActionAdd})
	dying.Finalizers = []string{v1.WorkspaceFinalizer}
	dying.DeletionTimestamp = &metav1.Time{Time: metav1.Now().Time}

	cli := fake.NewClientBuilder().WithObjects(mine, other, dying).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	reserved, err := r.reservedNodes(context.Background(), mine.Name)
	assert.NilError(t, err)
	assert.Equal(t, reserved.Has("node2"), true)
	// A workspace's own claim is not reserved against itself, a remove releases rather than
	// claims, and an abandoned claim is not a claim.
	assert.Equal(t, reserved.Has("node1"), false)
	assert.Equal(t, reserved.Has("node3"), false)
	assert.Equal(t, reserved.Has("node4"), false)
}

// Automatic scaling must not take a node a user explicitly asked for and admission accepted.
func TestGetNodesForScalingUpLeavesAReservedNodeAlone(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	free := genMockAdminNode("node1", "cluster", nodeFlavor)
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	claimant := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	setNodesAction(claimant, map[string]string{"node1": v1.NodeActionAdd})

	cli := fake.NewClientBuilder().WithObjects(free, workspace, claimant).
		WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "cluster",
		k8sfake.NewClientset(genMockK8sNode("node1", "cluster", nodeFlavor.Name, "")))

	nodes, err := r.getNodesForScalingUp(context.Background(), workspace, k8sClients, 1)
	assert.NilError(t, err)
	assert.Equal(t, len(nodes), 0)
}

// End to end: a refused bind leaves the nodes-action annotation in place, so the request stays
// visible and the Spec.Replica the mutating webhook already applied is not left stranded.
func TestProcessNodesActionKeepsTheAnnotationWhenTheBindIsRefused(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "ws-other")
	setNodesAction(workspace, map[string]string{node.Name: v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	_, err := r.processNodesAction(context.Background(), workspace)
	assert.ErrorContains(t, err, "already bound to ws-other")
	assert.Equal(t, storedNode(t, cli, node.Name).GetSpecWorkspace(), "ws-other")

	stored := &v1.Workspace{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKey{Name: workspace.Name}, stored))
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored) != "", true)
}

func setNodesAction(workspace *v1.Workspace, actions map[string]string) {
	metav1.SetMetaDataAnnotation(&workspace.ObjectMeta,
		v1.WorkspaceNodesAction, string(jsonutils.MarshalSilently(actions)))
}

// The contention this whole change exists for: two workspaces reaching for the same free node
// at the same time. Both pass admission, because at the moment each is admitted the node is
// genuinely unowned; both read it and both find it free. What separates them is the optimistic
// lock -- the patch carries the resourceVersion the judgement was made against, so the second
// writer's patch is rejected rather than applied, and its retry re-reads and refuses.
//
// The interceptor makes the interleaving deterministic: the competing write lands after this
// caller has read and judged, but before its patch.
func TestUpdateSingleNodeBindingLetsOnlyOneWorkspaceWin(t *testing.T) {
	node := ownedNode("node1", "")
	raced := false
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				if !raced {
					raced = true
					winner := &v1.Node{}
					if err := c.Get(ctx, client.ObjectKey{Name: obj.GetName()}, winner); err != nil {
						return err
					}
					winner.Spec.Workspace = pointer.String("ws2")
					if err := c.Update(ctx, winner); err != nil {
						return err
					}
				}
				return c.Patch(ctx, obj, patch, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	// Without the lock on the patch, ws1 would overwrite ws2 here and both workspaces would
	// believe they own node1.
	assert.Equal(t, updated, false)
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, storedNode(t, cli, "node1").GetSpecWorkspace(), "ws2")
}
