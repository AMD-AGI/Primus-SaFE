/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"testing"
	"time"

	"gotest.tools/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

// markMigrating puts a node in the state the source workspace leaves it in: released, and
// carrying the target that is expected to pick it up.
func markMigrating(node *v1.Node, from, target string) {
	node.Spec.Workspace = nil
	delete(node.Labels, v1.WorkspaceIdLabel)
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
		From:      from,
		Target:    target,
		StartTime: &metav1.Time{Time: time.Now().UTC()},
	})
}

func TestIsNodeEligibleForScalingUp(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	workspace := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	option := *r.option
	option.migrateTimeout = time.Hour
	r.option = &option

	cases := []struct {
		name  string
		mutil func(node *v1.Node)
		want  bool
	}{
		{name: "free node of a matching flavor", want: true},
		{name: "machine not ready", want: false, mutil: func(node *v1.Node) {
			node.Status.MachineStatus.Phase = v1.NodeSSHFailed
		}},
		{name: "not managed", want: false, mutil: func(node *v1.Node) {
			node.Status.ClusterStatus.Phase = v1.NodeManaging
		}},
		{name: "bound by spec", want: false, mutil: func(node *v1.Node) {
			node.Spec.Workspace = ptr.To("other")
		}},
		{name: "bound by label", want: false, mutil: func(node *v1.Node) {
			metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, "other")
		}},
		{name: "flavor mismatch", want: false, mutil: func(node *v1.Node) {
			metav1.SetMetaDataLabel(&node.ObjectMeta, v1.NodeFlavorIdLabel, "another-flavor")
		}},
		{name: "reserved by a migration to another workspace", want: false, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", "ws-b")
		}},
		// Not even for the target. Taking the node here would finish the crossing without the
		// replica the migration was supposed to add, and the source has already given one up.
		{name: "reserved by a migration to this workspace", want: false, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", workspace.Name)
		}},
		{name: "unreadable migration payload does not park the node", want: true, mutil: func(node *v1.Node) {
			metav1.SetMetaDataAnnotation(&node.ObjectMeta, v1.NodeMigrateAnnotation, "{")
		}},
		// The source workspace can be deleted mid-migration, and a node can leave the
		// cluster and come back still carrying the annotation. Nothing would then clear it,
		// so a reservation is only honoured while it is young enough to still be real.
		{name: "a reservation older than the timeout is ignored", want: true, mutil: func(node *v1.Node) {
			markMigrating(node, "ws-a", "ws-b")
			v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
				From:      "ws-a",
				Target:    "ws-b",
				StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * time.Hour)},
			})
		}},
		{name: "a reservation with no start time cannot be aged and is ignored", want: true, mutil: func(node *v1.Node) {
			v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{From: "ws-a", Target: "ws-b"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := genMockAdminNode("node1", clusterName, nodeFlavor)
			node.Status.ClusterStatus.Phase = v1.NodeManaged
			if tc.mutil != nil {
				tc.mutil(node)
			}
			assert.Equal(t, isNodeEligibleForScalingUp(node, workspace), tc.want)
		})
	}
}

// A migration leaves the node unbound and of a matching flavor for as long as it takes the
// target to claim it, which is everything a workspace short of a replica looks for. Nobody
// takes it that way -- not a bystander, and not the target either: arriving through the
// scaling loop means arriving against a replica the target already wanted, so the one the
// migration was supposed to add never is, and the source has already given one up.
func TestScaleUpLeavesAMigratingNodeToItsHandover(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	clusterName := "cluster"
	target := genMockWorkspace(clusterName, nodeFlavor.Name, 1)
	bystander := genMockWorkspace(clusterName, nodeFlavor.Name, 1)

	adminNode := genMockAdminNode("node1", clusterName, nodeFlavor)
	adminNode.Status.ClusterStatus.Phase = v1.NodeManaged
	markMigrating(adminNode, "ws-source", target.Name)

	adminClient := fake.NewClientBuilder().WithObjects(adminNode, target, bystander).
		WithStatusSubresource(target, bystander).WithScheme(scheme.Scheme).Build()
	k8sClient := k8sfake.NewClientset(genMockK8sNode(adminNode.Name, clusterName, nodeFlavor.Name, ""))
	k8sClientFactory := commonclient.NewClientFactoryWithOnlyClient(context.Background(), clusterName, k8sClient)
	r := newMockWorkspaceReconciler(adminClient)

	for _, ws := range []*v1.Workspace{bystander, target} {
		_, err := r.scaleUp(context.Background(), ws, k8sClientFactory, 1)
		assert.NilError(t, err)
		assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode))
		assert.Equal(t, adminNode.GetSpecWorkspace(), "",
			"workspace(%s) took a node in the middle of a crossing", ws.Name)
	}

	// Once the reservation has aged out it is nobody's, and the scaling loop may have it.
	v1.SetNodeMigrateInfo(adminNode, &v1.NodeMigrateInfo{
		From:      "ws-source",
		Target:    target.Name,
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * v1.DefaultNodeMigrateTimeout)},
	})
	assert.NilError(t, adminClient.Update(context.Background(), adminNode))
	_, err := r.scaleUp(context.Background(), target, k8sClientFactory, 1)
	assert.NilError(t, err)
	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: adminNode.Name}, adminNode))
	assert.Equal(t, adminNode.GetSpecWorkspace(), target.Name)
}

// migration builds a source workspace carrying a migration of one node to a target, with the
// node in whatever state the caller wants it -- the three the reconciler has to tell apart
// are "still bound here", "released and waiting", and "arrived".
type migration struct {
	reconciler *WorkspaceReconciler
	client     client.Client
	source     *v1.Workspace
	target     *v1.Workspace
	node       *v1.Node
}

func newMigration(t *testing.T, timeout time.Duration,
	placeNode func(node *v1.Node, source, target *v1.Workspace)) *migration {
	t.Helper()
	nodeFlavor := genMockNodeFlavor()
	source := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	target := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	node.Status.ClusterStatus.Phase = v1.NodeManaged
	placeNode(node, source, target)
	metav1.SetMetaDataAnnotation(&source.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{node.Name: v1.BuildMigrateAction(target.Name)})))

	// The same admission rule the real webhook applies to a nodes-action write. Without it
	// these tests judge writes the API server would refuse -- which is how a replica
	// adjustment that could never be persisted passed here once already.
	adminClient := fake.NewClientBuilder().WithObjects(node, source, target).
		WithStatusSubresource(source, target).WithScheme(scheme.Scheme).
		WithInterceptorFuncs(admissionRules()).Build()
	reconciler := newMockWorkspaceReconciler(adminClient)
	option := *reconciler.option
	option.migrateTimeout = timeout
	reconciler.option = &option
	return &migration{
		reconciler: &reconciler, client: adminClient, source: source, target: target, node: node,
	}
}

func boundToSource(node *v1.Node, source, _ *v1.Workspace) {
	node.Spec.Workspace = ptr.To(source.Name)
	metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, source.Name)
}

func (m *migration) reload(t *testing.T) {
	t.Helper()
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.source.Name}, m.source))
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.target.Name}, m.target))
}

// The release and the reservation are one patch: a node unbound without the reservation, even
// for the moment between two writes, is an unassigned node of a matching flavor that any
// workspace in the cluster short of a replica may take.
func TestProcessNodesActionMigrateReleasesAndReserves(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, true)

	m.reload(t)
	assert.Equal(t, m.node.GetSpecWorkspace(), "")
	info := v1.GetNodeMigrateInfo(m.node)
	assert.Assert(t, info != nil)
	assert.Equal(t, info.From, m.source.Name)
	assert.Equal(t, info.Target, m.target.Name)
	assert.Assert(t, info.StartTime != nil)
	// The source keeps the action: it is the record that a migration is under way, and the
	// only thing that brings the reconciler back to finish it.
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
	// Nothing has been asked of the target yet -- the node is still on its way out.
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
}

// Handing the node over means asking the target for it the way a user would, because that
// request is what raises the target's replica. Binding the node here would move it without
// the target ever accounting for it.
func TestProcessNodesActionMigrateHandsOverToTheTarget(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})

	result, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	// Not "updated": a crossing can last as long as the timeout allows when the handover
	// cannot land, and the workspace still has to be synced and scaled in the meantime.
	assert.Equal(t, isUpdated, false)
	// A busy target changes nothing about the node, so no event brings us back to ask again.
	assert.Assert(t, result.RequeueAfter > 0)

	m.reload(t)
	actions, err := parseWorkspaceNodesAction(t, m.target)
	assert.NilError(t, err)
	assert.Equal(t, actions[m.node.Name], v1.NodeActionAdd)
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
	assert.Equal(t, m.node.GetSpecWorkspace(), "")
}

// A target already busy with a node action is the ordinary case, not a failure: a workspace
// takes one at a time. The nodes stay reserved and the handover is retried.
func TestProcessNodesActionMigrateWaitsForABusyTarget(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	busy := string(jsonutils.MarshalSilently(map[string]string{"other-node": v1.NodeActionAdd}))
	metav1.SetMetaDataAnnotation(&m.target.ObjectMeta, v1.WorkspaceNodesAction, busy)
	assert.NilError(t, m.client.Update(context.Background(), m.target))

	result, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)
	assert.Assert(t, result.RequeueAfter > 0)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), busy)
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) != nil)
}

// Once the node has landed there is nothing left to drive, and the action has to go: left
// behind it would block every later node action on the source workspace.
func TestProcessNodesActionMigrateClearsTheActionOnArrival(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, _, target *v1.Workspace) {
		node.Spec.Workspace = ptr.To(target.Name)
		metav1.SetMetaDataLabel(&node.ObjectMeta, v1.WorkspaceIdLabel, target.Name)
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, m.node.GetSpecWorkspace(), m.target.Name)
}

// A migration that never completes must not park the node forever. Past the timeout the
// reservation comes off and the node becomes an ordinary unassigned node -- it does not
// return to the source, which gave it up and had its replica lowered to match.
func TestProcessNodesActionMigrateGivesUpAfterTheTimeout(t *testing.T) {
	m := newMigration(t, time.Minute, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
		v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
			From:      source.Name,
			Target:    target.Name,
			StartTime: &metav1.Time{Time: time.Now().UTC().Add(-time.Hour)},
		})
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
	assert.Equal(t, m.node.GetSpecWorkspace(), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
}

// The reservation can be taken off by hand. The source then has no node to migrate, and has
// to stop rather than hand over a node it no longer holds any claim on.
func TestProcessNodesActionMigrateGivesUpWithoutAReservation(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, _, _ *v1.Workspace) {
		node.Spec.Workspace = nil
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
}

// Binding a node to the workspace it was migrating to is the end of the crossing, so the
// reservation comes off in the same patch. Left on, it would keep every other workspace off
// a node that has already arrived.
func TestUpdateSingleNodeBindingClearsTheReservationOnArrival(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	markMigrating(node, "ws-source", "ws-target")
	adminClient := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	updated, err := r.updateSingleNodeBinding(context.Background(), "ws-target", node, nodeBinding{workspace: "ws-target"})
	assert.NilError(t, err)
	assert.Equal(t, updated, true)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "ws-target")
	assert.Assert(t, v1.GetNodeMigrateInfo(node) == nil)
}

// A node the target already holds still has to have its reservation cleared: the bind landed,
// the reservation write did not, and nothing else would ever take it off.
func TestUpdateSingleNodeBindingClearsAStaleReservation(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	markMigrating(node, "ws-source", "ws-target")
	node.Spec.Workspace = ptr.To("ws-target")
	adminClient := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	_, err := r.updateSingleNodeBinding(context.Background(), "ws-target", node, nodeBinding{workspace: "ws-target"})
	assert.NilError(t, err)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "ws-target")
	assert.Assert(t, v1.GetNodeMigrateInfo(node) == nil)
}

func parseWorkspaceNodesAction(t *testing.T, workspace *v1.Workspace) (map[string]string, error) {
	t.Helper()
	actions := make(map[string]string)
	err := json.Unmarshal([]byte(v1.GetWorkspaceNodesAction(workspace)), &actions)
	return actions, err
}

// drain returns the workspaces an event handler put on the queue.
func drain(q v1.RequestWorkQueue) []string {
	var names []string
	for q.Len() > 0 {
		item, shutdown := q.Get()
		if shutdown {
			break
		}
		names = append(names, item.Name)
		q.Done(item)
	}
	sort.Strings(names)
	return names
}

// The source workspace drives the migration and holds its one action slot until it sees the
// node land. Landing takes the node's workspace label from empty to the target, so waking
// workspaces by label alone wakes the target and leaves the source to find out at its next
// resync -- and every node operation asked of the source in between is refused as busy.
func TestHandleNodeEventWakesTheSourceWhenTheNodeLands(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	h := r.handleNodeEvent()
	handlerFuncs, ok := h.(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})
	assert.Assert(t, ok)

	// Old: released and reserved for the target. New: bound, reservation cleared -- the two
	// halves of the patch that ends a migration.
	released := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	markMigrating(released, "ws-source", "ws-target")
	landed := released.DeepCopy()
	landed.Spec.Workspace = ptr.To("ws-target")
	metav1.SetMetaDataLabel(&landed.ObjectMeta, v1.WorkspaceIdLabel, "ws-target")
	delete(landed.Annotations, v1.NodeMigrateAnnotation)

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	handlerFuncs.Update(context.Background(), event.UpdateEvent{ObjectOld: released, ObjectNew: landed}, q)

	queued := drain(q)
	assert.Assert(t, sliceHas(queued, "ws-source"), "the source was not woken by the landing, queued: %v", queued)
	assert.Assert(t, sliceHas(queued, "ws-target"), "the target was not woken by the landing, queued: %v", queued)
}

// The release half: the node leaves the source and is reserved for a target that has not been
// asked for it yet, so the target has nothing to do -- but the source has to come straight
// back to hand it over rather than waiting out a resync.
func TestHandleNodeEventWakesBothEndsOnRelease(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	handlerFuncs := r.handleNodeEvent().(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})

	bound := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	bound.Spec.Workspace = ptr.To("ws-source")
	metav1.SetMetaDataLabel(&bound.ObjectMeta, v1.WorkspaceIdLabel, "ws-source")
	released := bound.DeepCopy()
	markMigrating(released, "ws-source", "ws-target")

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	handlerFuncs.Update(context.Background(), event.UpdateEvent{ObjectOld: bound, ObjectNew: released}, q)

	queued := drain(q)
	assert.Assert(t, sliceHas(queued, "ws-source"), "queued: %v", queued)
	assert.Assert(t, sliceHas(queued, "ws-target"), "queued: %v", queued)
}

func sliceHas(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// A node that ends up in a third workspace is not ours to hand over. Going on would have the
// source asking the target to take someone else's node, once a pass until the timeout, while
// holding its own action slot for the whole of it.
func TestProcessNodesActionMigrateGivesUpOnANodeBoundElsewhere(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
		node.Spec.Workspace = ptr.To("ws-somewhere-else")
	})

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), "")
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
}

// A release that has to be retried keeps the clock it started with, so a release that keeps
// failing still ages out instead of holding the action slot indefinitely.
func TestClassifyMigrationKeepsTheOriginalStartTime(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)
	// Truncated: the annotation is JSON, and metav1.Time round-trips at second precision.
	started := &metav1.Time{Time: time.Now().UTC().Add(-20 * time.Minute).Truncate(time.Second)}
	v1.SetNodeMigrateInfo(m.node, &v1.NodeMigrateInfo{
		From: m.source.Name, Target: m.target.Name, StartTime: started,
	})

	state, info, _ := m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationRelease)
	assert.Assert(t, info.StartTime != nil)
	assert.Equal(t, info.StartTime.Time.Equal(started.Time), true,
		"the retry re-stamped the clock instead of keeping it")
}

// The expectations gate waits on bindings observed through node labels, which come from the
// data plane. A migration held behind it would never be carried on and never time out, so the
// node would stay released and reserved with nothing driving it.
func TestProcessWorkspaceCarriesAMigrationWithExpectationsOutstanding(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	// Something the data plane has not reported back on yet.
	m.reconciler.setExpectations(m.source.Name, sets.NewSetByKeys("some-other-node"))
	assert.Equal(t, m.reconciler.meetExpectations(m.source.Name), false)

	_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)

	m.reload(t)
	actions, err := parseWorkspaceNodesAction(t, m.target)
	assert.NilError(t, err)
	assert.Equal(t, actions[m.node.Name], v1.NodeActionAdd,
		"the handover did not happen while an unrelated expectation was outstanding")
}

// Once the finalizer is gone there is no workspace left to carry the migration or to give up
// on it, so a node this workspace released has to be let go on the way out. Nothing else
// would ever clear the reservation.
func TestDeleteWorkspaceReleasesTheNodesItWasMigrating(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	controllerutil.AddFinalizer(m.source, v1.WorkspaceFinalizer)
	assert.NilError(t, m.client.Update(context.Background(), m.source))

	assert.NilError(t, m.reconciler.delete(context.Background(), m.source))

	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil,
		"the reservation outlived the workspace that was driving it")
}

// Guessing is the one thing not to do with an action this does not understand: falling
// through to the add branch binds the node with none of the replica accounting a real add is
// admitted with.
func TestProcessNodesActionIgnoresAnUnknownAction(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	workspace := genMockWorkspace("cluster", nodeFlavor.Name, 1)
	node := genMockAdminNode("node1", "cluster", nodeFlavor)
	node.Status.ClusterStatus.Phase = v1.NodeManaged
	metav1.SetMetaDataAnnotation(&workspace.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{node.Name: "migrate:"})))

	adminClient := fake.NewClientBuilder().WithObjects(node, workspace).
		WithStatusSubresource(workspace).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	_, _, err := r.processNodesAction(context.Background(), workspace)
	assert.NilError(t, err)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "", "a malformed action claimed the node")
}

// Two things bind nodes for one workspace: the scaling loop, and the node action a user
// asked for. Replacing what the workspace is waiting on drops whatever the other one had
// outstanding -- the workspace then reads as settled with a binding still in flight, and the
// next scaling decision is taken on counts that have not caught up.
func TestSetExpectationsKeepsWhatIsAlreadyOutstanding(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())

	r.setExpectations("ws-a", sets.NewSetByKeys("scaling-down-node"))
	r.setExpectations("ws-a", sets.NewSetByKeys("node-action-node"))
	assert.Equal(t, r.meetExpectations("ws-a"), false)

	r.observeNode("ws-a", "node-action-node")
	assert.Equal(t, r.meetExpectations("ws-a"), false,
		"the workspace read as settled while the earlier binding was still in flight")

	r.observeNode("ws-a", "scaling-down-node")
	assert.Equal(t, r.meetExpectations("ws-a"), true)
}

// Once a reservation has expired it stops being honoured, so the node can be picked up by
// someone other than its target. The reservation has done its work either way and must come
// off, or it goes on naming workspaces with no part in this node and waking them for it.
func TestUpdateSingleNodeBindingClearsAnExpiredReservationOnWhoeverTakesTheNode(t *testing.T) {
	node := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
		From:      "ws-source",
		Target:    "ws-target",
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * v1.DefaultNodeMigrateTimeout)},
	})
	adminClient := fake.NewClientBuilder().WithObjects(node).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	_, err := r.updateSingleNodeBinding(context.Background(), "ws-unrelated", node, nodeBinding{workspace: "ws-unrelated"})
	assert.NilError(t, err)

	assert.NilError(t, adminClient.Get(context.Background(), client.ObjectKey{Name: node.Name}, node))
	assert.Equal(t, node.GetSpecWorkspace(), "ws-unrelated")
	assert.Assert(t, v1.GetNodeMigrateInfo(node) == nil, "the reservation outlived the crossing")
}

// The node webhook stops honouring a reservation at the shared timeout and cannot read this
// controller's setting, so a longer or unset one here would leave the migration still being
// driven after the node has stopped being protected.
func TestMigrateTimeoutNeverOutlastsTheGuard(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	for _, configured := range []time.Duration{0, -time.Minute, 2 * v1.DefaultNodeMigrateTimeout} {
		option := *r.option
		option.migrateTimeout = configured
		r.option = &option
		assert.Equal(t, r.migrateTimeout(), v1.DefaultNodeMigrateTimeout)
	}
	option := *r.option
	option.migrateTimeout = time.Minute
	r.option = &option
	assert.Equal(t, r.migrateTimeout(), time.Minute)
}

// Nothing settles an expectation but the event saying the binding landed, and a workspace
// still holding one runs nothing at all -- no status, no scaling, and no deletion, since
// removing the finalizer waits behind the same gate. A node deleted before its label was
// written, or an event missed, would leave it that way for good.
func TestExpectationsStopBeingWaitedOnEventually(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws-a", sets.NewSetByKeys("a-node-nobody-will-report-on"))
	assert.Equal(t, r.meetExpectations("ws-a"), false)

	expire(&r, "ws-a", "a-node-nobody-will-report-on")

	assert.Equal(t, r.meetExpectations("ws-a"), true, "the workspace waited for good")
	// Reading does not consume the way out; pruning is what removes it.
	r.RLock()
	_, still := r.expectations["ws-a"]
	r.RUnlock()
	assert.Equal(t, still, true)

	r.pruneExpectations("ws-a")
	r.RLock()
	_, afterPrune := r.expectations["ws-a"]
	r.RUnlock()
	assert.Equal(t, afterPrune, false)
}

// expire backdates one node's deadline so the wait for it has run out.
func expire(r *WorkspaceReconciler, workspaceId, nodeName string) {
	r.Lock()
	defer r.Unlock()
	r.expectations[workspaceId].deadlines[nodeName] = time.Now().Add(-time.Second)
}

// A workspace that keeps binding nodes keeps having entries added, and that is exactly the
// workspace where a stale one hides -- node actions run before the gate and go on adding to
// it. Each node has to age on its own clock.
func TestExpectationsAgeOnTheirOwnClock(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws-a", sets.NewSetByKeys("stale-node"))
	expire(&r, "ws-a", "stale-node")

	// A later binding for another node must not put the stale one back on the clock.
	r.setExpectations("ws-a", sets.NewSetByKeys("fresh-node"))
	assert.Equal(t, r.meetExpectations("ws-a"), false, "the fresh binding is still worth waiting on")

	r.observeNode("ws-a", "fresh-node")
	assert.Equal(t, r.meetExpectations("ws-a"), true, "the stale node was put back on the clock")
}

// Waiting for a workspace that no longer exists reaches a conclusion that is already certain,
// having held the source's one action slot for the whole of the migration timeout to get
// there.
func TestProcessNodesActionMigrateGivesUpWhenTheTargetIsGone(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	assert.NilError(t, m.client.Delete(context.Background(), m.target))

	_, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)

	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.source.Name}, m.source))
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
}

// The admission side refuses a workspace taking on a crossing someone else started; this is
// the same line held where the work is done.
func TestClassifyMigrationGivesUpOnSomeoneElsesCrossing(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, _, target *v1.Workspace) {
		markMigrating(node, "another-workspace", target.Name)
	})
	state, _, _ := m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationAbandoned)
}

// Giving up is final -- the reservation comes off and the source's replica is not given back
// -- so a cache that has not caught up must not be what decides it.
func TestProcessNodesActionMigrateConfirmsAMissingTargetBeforeGivingUp(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	// Gone as far as the cache is concerned, still there as far as the apiserver is. The node
	// goes in too: every read on this path goes through the apiReader now.
	m.reconciler.apiReader = fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(m.target.DeepCopy(), m.node.DeepCopy()).Build()
	m.target.Finalizers = nil
	assert.NilError(t, m.client.Update(context.Background(), m.target))
	assert.NilError(t, m.client.Delete(context.Background(), m.target))
	assert.Assert(t, apierrors.IsNotFound(
		m.client.Get(context.Background(), client.ObjectKey{Name: m.target.Name}, m.target.DeepCopy())))

	_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)

	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.node.Name}, m.node))
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) != nil,
		"a stale cache read was enough to give up on the migration")
	assert.NilError(t, m.client.Get(context.Background(), client.ObjectKey{Name: m.source.Name}, m.source))
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "")
}

// The deadline is only read when something asks, and nothing asks unless the workspace is
// reconciled again. Blocked on the gate, nothing else brings it back: a node released
// mid-migration carries no workspace on its labels for a node event to route by.
func TestProcessWorkspaceAsksToComeBackWhileItIsWaiting(t *testing.T) {
	cs := k8sfake.NewSimpleClientset()
	cluster := testCluster("c1")
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}}
	ws.Spec.Cluster = "c1"
	r := newWorkspaceReconcilerFull(t, cs, cluster, ws)
	r.option = &WorkspaceReconcilerOption{nodeWait: 30 * time.Second}
	r.setExpectations(ws.Name, sets.NewSetByKeys("a-node-still-in-flight"))

	result, err := r.processWorkspace(context.Background(), ws)
	assert.NilError(t, err)
	assert.Assert(t, result.RequeueAfter > 0, "the workspace was left with nothing to bring it back")
}

// An entry is removed when the node settles or when a prune reaches it, and a binding can
// arrive before either has happened. Inheriting the lapsed deadline opens the gate on that
// node at once -- and on every binding of it after that, since each one inherits it again.
func TestSetExpectationsRestartsTheClockOnALapsedEntry(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	r.setExpectations("ws-a", sets.NewSetByKeys("node1"))

	r.Lock()
	r.expectations["ws-a"].deadlines["node1"] = time.Now().Add(-time.Second)
	r.Unlock()
	assert.Equal(t, r.meetExpectations("ws-a"), true)

	// Bound again, so there is something to wait for again.
	r.setExpectations("ws-a", sets.NewSetByKeys("node1"))
	assert.Equal(t, r.meetExpectations("ws-a"), false,
		"the new binding inherited a deadline that had already passed")
}

// A node on its way out is not going anywhere else. Which of the two it becomes depends on
// whether the source still holds it: held while it does, since the replica has already been
// counted out and a withdrawal would not put it back; given up on once it has been released,
// since there is then nothing left to wait for.
func TestClassifyMigrationOnANodeBeingDeleted(t *testing.T) {
	m := newMigration(t, time.Hour, boundToSource)
	now := metav1.NewTime(time.Now())
	m.node.DeletionTimestamp = &now
	m.node.Finalizers = []string{v1.NodeFinalizer}

	state, _, _ := m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationHeld)

	markMigrating(m.node, m.source.Name, m.target.Name)
	m.node.DeletionTimestamp = &now
	state, _, _ = m.reconciler.classifyMigration(m.node, m.source.Name, m.target.Name)
	assert.Equal(t, state, migrationAbandoned)
}

// A node on its way out that the source still holds is kept waiting, not given up on. Giving
// up would leave the workspace holding a node its replica no longer counts -- the count moved
// when the migration was admitted, and a withdrawal does not move it back, no more than it
// does for a removal -- and the workspace would release a healthy node to get back down.
func TestProcessNodesActionMigrateHoldsANodeOnItsWayOut(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		boundToSource(node, source, target)
		node.Finalizers = []string{v1.NodeFinalizer}
	})
	assert.NilError(t, m.client.Delete(context.Background(), m.node))
	before := m.source.Spec.Replica

	result, isUpdated, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, false)
	assert.Assert(t, result.RequeueAfter > 0, "nothing will bring the crossing back")

	m.reload(t)
	assert.Equal(t, m.source.Spec.Replica, before, "the replica moved while the entry was held")
	assert.Assert(t, v1.GetWorkspaceNodesAction(m.source) != "", "the request was dropped")
	assert.Equal(t, m.node.GetSpecWorkspace(), m.source.Name, "the node was let go on its way out")
}

// A node released for a workspace that has gone is withdrawn like any other refusal, so the
// request does not disappear from the console without a word.
func TestProcessNodesActionMigrateRecordsWhyAGoneTargetWasGivenUpOn(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	assert.NilError(t, m.client.Delete(context.Background(), m.target))
	m.reconciler.apiReader = fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(m.node.DeepCopy()).Build()

	_, _, err := m.reconciler.processNodesAction(context.Background(), m.source)
	assert.NilError(t, err)

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.source), "")
	assert.Assert(t, v1.GetAnnotation(m.source, v1.WorkspaceNodesActionError) != "",
		"the request vanished with no reason recorded")
	assert.Assert(t, v1.GetNodeMigrateInfo(m.node) == nil)
}

// The target's mutating webhook rewrites the annotation as it accepts it, so the string read
// back is not the string written. Compared by text, a workspace mistakes its own handover for
// somebody else's job and waits out the timeout beside it.
func TestHandOverToTargetRecognisesItsOwnRequestAfterTheWebhookRewritesIt(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	// What the target carries after its webhook dropped an entry that was already true.
	rewritten := string(jsonutils.MarshalSilently(map[string]string{
		m.node.Name: v1.NodeActionAdd, "already-there": v1.NodeActionAdd,
	}))
	metav1.SetMetaDataAnnotation(&m.target.ObjectMeta, v1.WorkspaceNodesAction, rewritten)
	assert.NilError(t, m.client.Update(context.Background(), m.target))

	assert.NilError(t, m.reconciler.handOverToTarget(context.Background(), m.target.Name,
		[]string{m.node.Name}))

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), rewritten,
		"it wrote over a request that already asked for this node")
}

// A target carrying somebody else's action is waited for, not overwritten.
func TestHandOverToTargetLeavesAnotherRequestAlone(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	other := string(jsonutils.MarshalSilently(map[string]string{"other-node": v1.NodeActionAdd}))
	metav1.SetMetaDataAnnotation(&m.target.ObjectMeta, v1.WorkspaceNodesAction, other)
	assert.NilError(t, m.client.Update(context.Background(), m.target))

	err := m.reconciler.handOverToTarget(context.Background(), m.target.Name, []string{m.node.Name})
	assert.Assert(t, err != nil)
	assert.Assert(t, errors.Is(err, errMigrationTargetBusy), "a busy target read as something worse")

	m.reload(t)
	assert.Equal(t, v1.GetWorkspaceNodesAction(m.target), other)
}

// A plain merge patch carries no resourceVersion, so it never conflicts and never retries --
// it just writes over whatever landed on the target between the read and the write. Here that
// is somebody else's node action, and its replica has already been counted for it.
func TestHandOverToTargetDoesNotOverwriteARequestThatLandedFirst(t *testing.T) {
	m := newMigration(t, time.Hour, func(node *v1.Node, source, target *v1.Workspace) {
		markMigrating(node, source.Name, target.Name)
	})
	competing := string(jsonutils.MarshalSilently(map[string]string{"someone-elses-node": v1.NodeActionAdd}))

	// Slipped in after the handover has read the target and before it writes.
	raced := false
	m.reconciler.Client = fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithObjects(m.node.DeepCopy(), m.source.DeepCopy(), m.target.DeepCopy()).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey,
				obj client.Object, opts ...client.GetOption) error {
				if err := cl.Get(ctx, key, obj, opts...); err != nil {
					return err
				}
				workspace, ok := obj.(*v1.Workspace)
				if !ok || raced || key.Name != m.target.Name {
					return nil
				}
				raced = true
				winner := workspace.DeepCopy()
				metav1.SetMetaDataAnnotation(&winner.ObjectMeta, v1.WorkspaceNodesAction, competing)
				return cl.Update(ctx, winner)
			},
		}).Build()

	err := m.reconciler.handOverToTarget(context.Background(), m.target.Name, []string{m.node.Name})
	assert.Assert(t, err != nil, "the handover reported success over somebody else's request")

	stored := &v1.Workspace{}
	assert.NilError(t, m.reconciler.Get(context.Background(), client.ObjectKey{Name: m.target.Name}, stored))
	assert.Equal(t, v1.GetWorkspaceNodesAction(stored), competing,
		"the request that landed first was overwritten")
}

// A pass can have both: one node to bind, and another already released and waiting to be
// taken. Returning for the binding alone leaves the wait to whatever event the binding
// happens to produce, or to the resync hours later if it produces none.
func TestProcessNodesActionKeepsTheWaitWhenItAlsoBindsSomething(t *testing.T) {
	nodeFlavor := genMockNodeFlavor()
	source := genMockWorkspace("cluster", nodeFlavor.Name, 2)
	target := genMockWorkspace("cluster", nodeFlavor.Name, 1)

	toRelease := genMockAdminNode("node1", "cluster", nodeFlavor)
	toRelease.Status.ClusterStatus.Phase = v1.NodeManaged
	boundToSource(toRelease, source, target)
	waiting := genMockAdminNode("node2", "cluster", nodeFlavor)
	waiting.Status.ClusterStatus.Phase = v1.NodeManaged
	markMigrating(waiting, source.Name, target.Name)

	metav1.SetMetaDataAnnotation(&source.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{
			toRelease.Name: v1.BuildMigrateAction(target.Name),
			waiting.Name:   v1.BuildMigrateAction(target.Name),
		})))
	// Busy, so the second node stays waiting instead of being taken this pass.
	metav1.SetMetaDataAnnotation(&target.ObjectMeta, v1.WorkspaceNodesAction,
		string(jsonutils.MarshalSilently(map[string]string{"someone-else": v1.NodeActionAdd})))

	adminClient := fake.NewClientBuilder().WithObjects(toRelease, waiting, source, target).
		WithStatusSubresource(source, target).WithScheme(scheme.Scheme).Build()
	r := newMockWorkspaceReconciler(adminClient)

	result, isUpdated, err := r.processNodesAction(context.Background(), source)
	assert.NilError(t, err)
	assert.Equal(t, isUpdated, true, "a node was bound this pass")
	assert.Assert(t, result.RequeueAfter > 0, "the node still waiting to be taken was forgotten")
}

// Nothing removes a reservation once it has expired -- it is only ignored where it is read --
// so a node carrying a stale one would wake both of its workspaces on every write it ever
// receives, and the data plane writes node status every few seconds.
func TestHandleNodeEventIgnoresAnExpiredReservation(t *testing.T) {
	r := newMockWorkspaceReconciler(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build())
	handlerFuncs := r.handleNodeEvent().(interface {
		Update(context.Context, event.UpdateEvent, v1.RequestWorkQueue)
	})

	stale := genMockAdminNode("node1", "cluster", genMockNodeFlavor())
	v1.SetNodeMigrateInfo(stale, &v1.NodeMigrateInfo{
		From:      "ws-source",
		Target:    "ws-target",
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * v1.DefaultNodeMigrateTimeout)},
	})
	// The sort of write the data plane makes constantly.
	touched := stale.DeepCopy()
	touched.Status.MachineStatus.HostName = "renamed"

	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer q.ShutDown()
	handlerFuncs.Update(context.Background(), event.UpdateEvent{ObjectOld: stale, ObjectNew: touched}, q)

	assert.Equal(t, len(drain(q)), 0, "a reservation nobody is driving kept waking its workspaces")
}
