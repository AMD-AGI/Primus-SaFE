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

	"github.com/prometheus/client_golang/prometheus/testutil"

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
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	rmmetrics "github.com/AMD-AIG-AIMA/SAFE/resource-manager/pkg/metrics"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

func TestJudgeNodeBinding(t *testing.T) {
	cases := []struct {
		name             string
		node             *v1.Node
		target           string
		requester        string
		expectedVerdict  nodeBindVerdict
		expectedInReason string
	}{
		{name: "bind a free node", node: ownedNode("n", ""), target: "ws1", requester: "ws1",
			expectedVerdict: bindProceed},
		{name: "bind a node this workspace already holds", node: ownedNode("n", "ws1"),
			target: "ws1", requester: "ws1", expectedVerdict: bindSettled},
		{name: "bind a node another workspace holds", node: ownedNode("n", "ws2"),
			target: "ws1", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "already bound to ws2"},
		{name: "unbind a node this workspace holds", node: ownedNode("n", "ws1"), target: "",
			requester: "ws1", expectedVerdict: bindProceed},
		{name: "unbind a node that is already free", node: ownedNode("n", ""), target: "",
			requester: "ws1", expectedVerdict: bindSettled},
		// The one the three open-coded copies of this rule all missed: every one of them
		// keyed on a non-empty target, and an unbind's target is always empty.
		{name: "unbind a node another workspace holds", node: ownedNode("n", "ws2"), target: "",
			requester: "ws1", expectedVerdict: bindRefused, expectedInReason: "bound to ws2"},
		{name: "bind a node on somebody else's behalf", node: ownedNode("n", ""),
			target: "ws2", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "ws1 may not bind it to ws2"},
		{name: "bind a node that is being deleted", node: deletingNode(ownedNode("n", "")),
			target: "ws1", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "being deleted"},
		// The half of the managed check admission cannot do: the node passed admission and
		// then lost its managed state, which is the state the check exists to keep out.
		{name: "bind a node that is not managed", node: unmanagedNode(ownedNode("n", "")),
			target: "ws1", requester: "ws1", expectedVerdict: bindRefused,
			expectedInReason: "not managed"},
		// A release still has to go through for a node whose managed state is gone -- that is
		// exactly when the workspace holding it needs to let go.
		{name: "unbind a node that is not managed", node: unmanagedNode(ownedNode("n", "ws1")),
			target: "", requester: "ws1", expectedVerdict: bindProceed},
		{name: "unbind a node that is being deleted", node: deletingNode(ownedNode("n", "ws1")),
			target: "", requester: "ws1", expectedVerdict: bindProceed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			verdict, reason := judgeNodeBinding(c.node, c.target, c.requester)
			assert.Equal(t, verdict, c.expectedVerdict)
			if c.expectedInReason != "" {
				assert.Assert(t, strings.Contains(reason, c.expectedInReason), reason)
			}
		})
	}
}

func deletingNode(node *v1.Node) *v1.Node {
	now := metav1.Now()
	node.DeletionTimestamp = &now
	node.Finalizers = []string{v1.NodeFinalizer}
	return node
}

func unmanagedNode(node *v1.Node) *v1.Node {
	node.Status.ClusterStatus.Phase = v1.NodeManagedFailed
	return node
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

// admissionRules is what admission does to a Workspace write on its way to the API server, in
// the one respect this controller has to live within: a write that moves Spec.Replica and the
// nodes-action annotation together is turned away unless it is a withdrawal, and a withdrawal
// is only one that lands Spec.Replica on exactly the value commonnodes.WithdrawnReplica gives.
// Every write carrying that annotation goes through it -- the webhooks are registered on
// workspaces UPDATE with no object selector, so this controller's own patches are admitted
// like anybody else's.
//
// It is here because the fake client runs no webhooks, and without it these tests pass on a
// withdrawal that never once reaches a real cluster: the whole path is one Patch call, and
// getting that call rejected is not something any assertion about the stored object can see.
// The rule is asserted from the other side too, against the webhooks themselves and in the
// order the API server runs them, in TestWorkspaceAdmitWithdrawalEndToEnd.
func admissionRules() interceptor.Funcs {
	return interceptor.Funcs{
		Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
			patch client.Patch, opts ...client.PatchOption) error {
			workspace, ok := obj.(*v1.Workspace)
			if !ok {
				return c.Patch(ctx, obj, patch, opts...)
			}
			stored := &v1.Workspace{}
			if err := c.Get(ctx, client.ObjectKeyFromObject(workspace), stored); err != nil {
				return err
			}
			if v1.GetWorkspaceNodesAction(stored) != v1.GetWorkspaceNodesAction(workspace) &&
				stored.Spec.Replica != workspace.Spec.Replica &&
				workspace.Spec.Replica != commonnodes.WithdrawnReplica(stored.Spec.Replica,
					parseNodesAction(stored), parseNodesAction(workspace)) {
				return apierrors.NewBadRequest("the operation of specifying nodes and the " +
					"modification of workspace replica cannot be performed simultaneously")
			}
			return c.Patch(ctx, obj, patch, opts...)
		},
	}
}

func storedWorkspace(t *testing.T, cli client.Client, name string) *v1.Workspace {
	t.Helper()
	workspace := &v1.Workspace{}
	assert.NilError(t, cli.Get(context.Background(), client.ObjectKey{Name: name}, workspace))
	return workspace
}

// End to end: a bind that can never succeed is withdrawn rather than retried forever. The
// entry leaves the annotation and the reason is written where whoever asked can read it.
//
// Spec.Replica comes back down in the same patch. The mutating webhook counted this add in
// when it accepted the request, and a count left standing for a node that will never be bound
// is a scale-up onto some other machine -- not what was asked for.
func TestProcessNodesActionWithdrawsARefusedBind(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "ws-other")
	setNodesAction(workspace, map[string]string{node.Name: v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, storedNode(t, cli, node.Name).GetSpecWorkspace(), "ws-other")

	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	// The only add in the request, so the whole charge comes back: 1 -> 0.
	assert.Equal(t, stored.Spec.Replica, 0)
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "already bound to ws-other"))
}

// A node that has gone away between the request being admitted and this controller reaching
// it is refused like any other entry it cannot carry out. Skipping it instead would drop the
// annotation while leaving the replica the webhook added for it in place, and the next sync
// would spend that replica on whatever machine happened to be free.
func TestProcessNodesActionWithdrawsAVanishedNode(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	setNodesAction(workspace, map[string]string{"node1": v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	isUpdated, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	// Nothing is pending after a withdrawal, and saying otherwise waits for a requeue that
	// an annotation merely going away does not produce.
	assert.Equal(t, isUpdated, false)

	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "no longer exists"))
}

// A refusal in a batch takes only its own entry down with it: the surviving entry is still
// applied, and the annotation is rewritten to exactly what is left, which is what the webhook
// reads the withdrawal off and what stops the next reconcile accounting for it twice.
func TestProcessNodesActionWithdrawsOnlyTheRefusedEntry(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 2)
	taken := ownedNode("node1", "ws-other")
	free := ownedNode("node2", "")
	setNodesAction(workspace, map[string]string{
		taken.Name: v1.NodeActionAdd,
		free.Name:  v1.NodeActionAdd,
	})
	cli := fake.NewClientBuilder().WithObjects(taken, free, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, storedNode(t, cli, free.Name).GetSpecWorkspace(), workspace.Name)

	stored := storedWorkspace(t, cli, workspace.Name)
	// One of the two adds withdrawn, so one replica back: 2 -> 1.
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), `{"node2":"add"}`)
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "node1: "))

	// Second pass: node2 is bound now, so the whole request is done. It leaves as a plain
	// clear that touches no reason -- nothing was withdrawn this time, and a reason written
	// again beside a shrinking request is a second withdrawal, and a second refund with it.
	reason := v1.GetAnnotation(stored, v1.WorkspaceNodesActionError)
	_, err = r.processNodesAction(context.Background(), stored)
	assert.NilError(t, err)
	stored = storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Equal(t, v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), reason)
}

// The refund happens exactly once, whatever the contention. Annotation and replica move in a
// single optimistically-locked patch, so a competing write does not take half of it: the patch
// is rejected whole, nothing is stored, and the reconcile that follows recomputes the refusal
// against the request that actually exists rather than replaying the one it had in hand.
//
// The failure this rules out is a workspace losing a replica per requeue. It cannot be seen by
// looking at one pass -- both a correct and a double-counting implementation write 1 the first
// time they get through -- so the test has to lose a patch and then come back.
func TestDropRefusedActionsRefundsExactlyOnceAcrossAConflict(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 2)
	taken := ownedNode("node1", "ws-other")
	free := ownedNode("node2", "")
	setNodesAction(workspace, map[string]string{
		taken.Name: v1.NodeActionAdd,
		free.Name:  v1.NodeActionAdd,
	})
	rules := admissionRules()
	admit := rules.Patch
	conflicts := 1
	rules.Patch = func(ctx context.Context, c client.WithWatch, obj client.Object,
		patch client.Patch, opts ...client.PatchOption) error {
		if _, ok := obj.(*v1.Workspace); ok && conflicts > 0 {
			conflicts--
			return apierrors.NewConflict(schema.GroupResource{Resource: "workspaces"},
				obj.GetName(), errors.New("somebody else got there first"))
		}
		return admit(ctx, c, obj, patch, opts...)
	}
	cli := fake.NewClientBuilder().WithObjects(taken, free, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(rules).Build()
	r := newMockWorkspaceReconciler(cli)

	_, err := r.processNodesAction(context.Background(), workspace)
	assert.Assert(t, apierrors.IsConflict(err))
	// Not half applied: the request is whole and the replica is untouched.
	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, stored.Spec.Replica, 2)
	assert.Equal(t, v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "")
	assert.Assert(t, strings.Contains(v1.GetWorkspaceNodesAction(stored), "node1"))

	// The requeue, off the object as it now stands.
	_, err = r.processNodesAction(context.Background(), stored)
	assert.NilError(t, err)
	stored = storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), `{"node2":"add"}`)

	// And once the request is done there is nothing left to give back, however many times it
	// comes round again.
	_, err = r.processNodesAction(context.Background(), stored)
	assert.NilError(t, err)
	assert.Equal(t, storedWorkspace(t, cli, workspace.Name).Spec.Replica, 1)
}

// A refused remove is withdrawn the same way as a refused add. What differs is the accounting:
// see commonnodes.WithdrawnReplica for why a remove gets no replica back.
func TestProcessNodesActionWithdrawsARefusedRemove(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "ws-other")
	setNodesAction(workspace, map[string]string{node.Name: v1.NodeActionRemove})
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	assert.Equal(t, storedNode(t, cli, node.Name).GetSpecWorkspace(), "ws-other")
	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Assert(t, strings.Contains(
		v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "which is not the workspace asking"))
}

// An entry that is simply already true is not a refusal, and must not be reported as one: the
// mutating webhook skipped it when it counted, so there is nothing charged to give back, and a
// reason annotation appearing alongside the shrinking request is exactly what admission reads
// as a withdrawal -- one that would then be expected to carry a refund it does not owe.
func TestProcessNodesActionLeavesReplicaAloneForASettledEntry(t *testing.T) {
	workspace := genMockWorkspace("cluster", "flavor", 1)
	node := ownedNode("node1", "")
	node.Spec.Workspace = pointer.String(workspace.Name)
	setNodesAction(workspace, map[string]string{node.Name: v1.NodeActionAdd})
	cli := fake.NewClientBuilder().WithObjects(node, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	r := newMockWorkspaceReconciler(cli)

	_, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)
	stored := storedWorkspace(t, cli, workspace.Name)
	assert.Equal(t, stored.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), "")
	assert.Equal(t, v1.GetAnnotation(stored, v1.WorkspaceNodesActionError), "")
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

// A node moving from one workspace to the next passes through the empty label on the way, and
// that intermediate event must not credit the workspace still waiting for the label to arrive.
// Credit it early and syncWorkspace counts a node the workspace does not yet hold as missing,
// binds a spare machine, and hands it back on the following round -- real churn on hardware.
func TestHandleNodeEventDoesNotSettleAWorkspaceTheLabelHasNotReached(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))
	r.setExpectations("ws2", sets.NewSetByKeys("node1"))

	oldNode := ownedNode("node1", "ws1")
	newNode := ownedNode("node1", "")
	newNode.Spec.Workspace = pointer.String("ws2")

	q := resWorkQueue()
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, q)

	assert.Equal(t, r.meetExpectations("ws1"), true, "the workspace the node left is settled")
	assert.Equal(t, r.meetExpectations("ws2"), false, "the incoming workspace is still waiting")

	// And it is settled by its own label arriving.
	arrived := ownedNode("node1", "ws2")
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: newNode, ObjectNew: arrived}, q)
	assert.Equal(t, r.meetExpectations("ws2"), true)
}

// The other side of the same rule. A node unmanaged in the window between a bind writing the
// claim and the label making the round trip loses the claim with no label to lose alongside it,
// so the claim going is the only event its owner will ever get. Miss it and the workspace waits
// on that node for good: meetExpectations never comes true, and processWorkspace returns at the
// top of every round without scaling or syncing status again.
func TestHandleNodeEventSettlesAClaimReleasedBeforeTheLabelArrived(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))

	oldNode := ownedNode("node1", "")
	oldNode.Spec.Workspace = pointer.String("ws1")
	newNode := ownedNode("node1", "")

	q := resWorkQueue()
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, q)

	assert.Equal(t, r.meetExpectations("ws1"), true)
	assert.Equal(t, q.Len(), 1, "and the workspace is woken to notice the node is gone")
}

// The guard that keeps the branch above from undoing the one before it. An ordinary unbind
// drops the claim first and the label after, and it is the label that settles: the workspace
// counts what it holds by label, so crediting the claim would tell it the node was gone while
// it is still counting it, and it would bind a replacement it does not need.
func TestHandleNodeEventDoesNotSettleAClaimThatStillHasItsLabel(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws1", sets.NewSetByKeys("node1"))

	oldNode := ownedNode("node1", "ws1")
	newNode := ownedNode("node1", "ws1")
	newNode.Spec.Workspace = nil

	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: oldNode, ObjectNew: newNode}, resWorkQueue())

	assert.Equal(t, r.meetExpectations("ws1"), false)

	// The label following is what settles it.
	gone := ownedNode("node1", "")
	r.handleNodeEvent().Update(context.Background(),
		event.UpdateEvent{ObjectOld: newNode, ObjectNew: gone}, resWorkQueue())
	assert.Equal(t, r.meetExpectations("ws1"), true)
}

// Reading the other workspaces' claims is what keeps two of them off the same free node, so a
// read that fails has to stop the round rather than scale up against a partial answer.
func TestGetNodesForScalingUpStopsWhenTheClaimsCannotBeRead(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	free := genMockAdminNode("node1", "cluster", nodeFlavor)
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	cli := fake.NewClientBuilder().WithObjects(free, workspace).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, c client.WithWatch, list client.ObjectList,
				opts ...client.ListOption) error {
				if _, ok := list.(*v1.WorkspaceList); ok {
					return apierrors.NewInternalError(errors.New("boom"))
				}
				return c.List(ctx, list, opts...)
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)
	k8sClients := commonclient.NewClientFactoryWithOnlyClient(context.Background(), "cluster",
		k8sfake.NewClientset(genMockK8sNode("node1", "cluster", nodeFlavor.Name, "")))

	_, err := r.getNodesForScalingUp(context.Background(), workspace, k8sClients, 1)
	assert.ErrorContains(t, err, "boom")
}

func bindingCount(t *testing.T, action, outcome string) float64 {
	t.Helper()
	return testutil.ToFloat64(rmmetrics.WorkspaceNodeBindingTotal.WithLabelValues(action, outcome))
}

// The outcome labels are the only thing that separates "binding is busy" from "binding is
// being turned down" on a dashboard, and neither of the two new ones is observable any other
// way -- a refusal and an exhausted retry both surface to the caller as a plain error.
func TestUpdateSingleNodeBindingCountsWhatItTurnedDown(t *testing.T) {
	node := ownedNode("node1", "ws2")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(cli)

	before := bindingCount(t, "bind", "refused")
	_, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.ErrorContains(t, err, "already bound to ws2")
	assert.Equal(t, bindingCount(t, "bind", "refused"), before+1)
}

// Counted once per exhausted call, not once per attempt: a counter that climbs with the retry
// budget measures the budget rather than the contention.
func TestUpdateSingleNodeBindingCountsAnExhaustedRetryOnce(t *testing.T) {
	node := ownedNode("node1", "")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Patch: func(ctx context.Context, c client.WithWatch, obj client.Object,
				patch client.Patch, opts ...client.PatchOption) error {
				return apierrors.NewConflict(schema.GroupResource{Resource: "nodes"},
					obj.GetName(), errors.New("conflict"))
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	before := bindingCount(t, "bind", "conflict")
	_, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.Assert(t, apierrors.IsConflict(err))
	assert.Equal(t, bindingCount(t, "bind", "conflict"), before+1)
}

// A read that stops working -- throttling, an RBAC change -- has to look like binding failing
// rather than like binding traffic going to zero.
func TestUpdateSingleNodeBindingCountsAReadThatFails(t *testing.T) {
	node := ownedNode("node1", "")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				return apierrors.NewInternalError(errors.New("throttled"))
			},
		}).Build()
	r := newMockWorkspaceReconciler(cli)

	before := bindingCount(t, "bind", "failed")
	_, err := r.updateSingleNodeBinding(context.Background(), "ws1", node, "ws1")
	assert.ErrorContains(t, err, "throttled")
	assert.Equal(t, bindingCount(t, "bind", "failed"), before+1)
}

// The one place outside updateSingleNodeBinding that clears spec.workspace. A bind writes spec
// first and waits for the label to make the round trip, so a node unmanaged inside that window
// carries the claim and not the label -- and tying the release to the label would strand it
// for good, since only the owner may release and the owner has been told the node is gone.
func TestCleanupNodeAfterUnmanageReleasesAClaimThatHasNoLabelYet(t *testing.T) {
	node := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	node.Spec.Workspace = pointer.String("ws1")
	cli := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockNodeReconciler(cli)

	assert.NilError(t, r.cleanupNodeAfterUnmanage(context.Background(), node))
	assert.Equal(t, storedNode(t, cli, node.Name).GetSpecWorkspace(), "")
}
