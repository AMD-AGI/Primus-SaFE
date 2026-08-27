/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

const (
	migrateCluster = "cluster1"
	migrateFlavor  = "flavor1"
)

func migrateNode(name, cluster, flavor, workspace string) *v1.Node {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.ClusterIdLabel:    cluster,
				v1.NodeFlavorIdLabel: flavor,
			},
		},
	}
	if workspace != "" {
		node.Spec.Workspace = pointer.String(workspace)
		node.Labels[v1.WorkspaceIdLabel] = workspace
	}
	return node
}

func migrateWorkspace(name, cluster, flavor string, replica int) *v1.Workspace {
	return &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.WorkspaceSpec{Cluster: cluster, NodeFlavor: flavor, Replica: replica},
	}
}

func withNodesAction(workspace *v1.Workspace, actions map[string]string) *v1.Workspace {
	v1.SetAnnotation(workspace, v1.WorkspaceNodesAction, string(jsonutils.MarshalSilently(actions)))
	return workspace
}

func releasedForMigration(node *v1.Node, from, target string) *v1.Node {
	node.Spec.Workspace = nil
	delete(node.Labels, v1.WorkspaceIdLabel)
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
		From:      from,
		Target:    target,
		StartTime: &metav1.Time{Time: time.Now().UTC()},
	})
	return node
}

// The source workspace gives a node up, so its replica has to come down with it -- otherwise
// the scaling loop reads the workspace as short of a node and takes another one to replace
// the one being migrated away.
func TestMutateNodesActionMigrateReleasesTheReplica(t *testing.T) {
	scheme := newScheme(t)
	node := migrateNode("node1", migrateCluster, migrateFlavor, "ws-a")
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()}

	oldWs := migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 2)
	newWs := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 2),
		map[string]string{"node1": v1.BuildMigrateAction("ws-b")})

	assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, newWs))
	assert.Equal(t, newWs.Spec.Replica, 1)
	assert.Equal(t, v1.GetWorkspaceNodesAction(newWs) != "", true)
}

// The entry has to survive every later update of the source workspace until the node reaches
// the target: it is the only record that a migration is in flight, and the reconciler reads
// it back to carry the node the rest of the way.
func TestMutateNodesActionMigrateInFlightIsKeptAndCountedOnce(t *testing.T) {
	scheme := newScheme(t)
	node := releasedForMigration(migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-b")
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()}

	action := map[string]string{"node1": v1.BuildMigrateAction("ws-b")}
	oldWs := migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1)
	newWs := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1), action)

	assert.NilError(t, m.mutateNodesAction(context.Background(), oldWs, newWs))
	assert.Equal(t, newWs.Spec.Replica, 1)
	actions, err := parseNodesAction(newWs)
	assert.NilError(t, err)
	assert.Equal(t, actions["node1"], v1.BuildMigrateAction("ws-b"))
}

func TestValidateNodesActionMigrate(t *testing.T) {
	action := func(target string, nodes ...string) map[string]string {
		result := make(map[string]string)
		for _, node := range nodes {
			result[node] = v1.BuildMigrateAction(target)
		}
		return result
	}

	cases := []struct {
		name    string
		objects func() []client.Object
		actions map[string]string
		wantErr bool
	}{
		{
			name:    "a bound node moves to a target of the same flavor",
			actions: action("ws-b", "node1"),
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
				}
			},
		},
		{
			name:    "a target with no flavor adopts the node's",
			actions: action("ws-b", "node1"),
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateWorkspace("ws-b", migrateCluster, "", 0),
				}
			},
		},
		{
			name:    "a released node is re-admitted for the same migration",
			actions: action("ws-b", "node1"),
			objects: func() []client.Object {
				return []client.Object{
					releasedForMigration(migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-b"),
					migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
				}
			},
		},
		{
			name:    "the target does not exist",
			actions: action("ws-b", "node1"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{migrateNode("node1", migrateCluster, migrateFlavor, "ws-a")}
			},
		},
		{
			name:    "the target is the workspace itself",
			actions: action("ws-a", "node1"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1),
				}
			},
		},
		{
			name:    "the target is in another cluster",
			actions: action("ws-b", "node1"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateWorkspace("ws-b", "cluster2", migrateFlavor, 1),
				}
			},
		},
		{
			name:    "the target runs another flavor",
			actions: action("ws-b", "node1"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateWorkspace("ws-b", migrateCluster, "flavor2", 1),
				}
			},
		},
		{
			name:    "the target is busy with another node action",
			actions: action("ws-b", "node1"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					withNodesAction(migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
						map[string]string{"node9": v1.NodeActionAdd}),
				}
			},
		},
		{
			name:    "a flavourless target cannot adopt two flavors at once",
			actions: action("ws-b", "node1", "node2"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateNode("node2", migrateCluster, "flavor2", "ws-a"),
					migrateWorkspace("ws-b", migrateCluster, "", 0),
				}
			},
		},
		{
			name:    "the node belongs to a third workspace",
			actions: action("ws-b", "node1"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-c"),
					migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
				}
			},
		},
		{
			name:    "the node is unbound and not migrating anywhere",
			actions: action("ws-b", "node1"),
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, ""),
					migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
				}
			},
		},
		{
			name: "a migration cannot be mixed with another action",
			actions: map[string]string{
				"node1": v1.BuildMigrateAction("ws-b"),
				"node2": v1.NodeActionRemove,
			},
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateNode("node2", migrateCluster, migrateFlavor, "ws-a"),
					migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
				}
			},
		},
		{
			name: "a migration cannot have two targets",
			actions: map[string]string{
				"node1": v1.BuildMigrateAction("ws-b"),
				"node2": v1.BuildMigrateAction("ws-c"),
			},
			wantErr: true,
			objects: func() []client.Object {
				return []client.Object{
					migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
					migrateNode("node2", migrateCluster, migrateFlavor, "ws-a"),
					migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
					migrateWorkspace("ws-c", migrateCluster, migrateFlavor, 1),
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scheme := newScheme(t)
			validator := &WorkspaceValidator{
				Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.objects()...).Build(),
			}
			newWs := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1), tc.actions)
			err := validator.validateNodesAction(context.Background(), newWs, migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1))
			if tc.wantErr {
				assert.Assert(t, err != nil, "expected the migration to be refused")
				return
			}
			assert.NilError(t, err)
		})
	}
}

// The handover writes the add on the target, and a node genuinely released for it reads as
// unbound by then. A node that still reads as bound is refused whatever its annotation says --
// the cost is a retry on a read that has not caught up, and the alternative is letting the
// node vouch for its own release.
func TestValidateNodesActionAddTakesAReleasedNodeAndOnlyAReleasedOne(t *testing.T) {
	scheme := newScheme(t)
	released := releasedForMigration(migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-b")
	stillReadsBound := releasedForMigration(migrateNode("node2", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-b")
	stillReadsBound.Spec.Workspace = pointer.String("ws-a")

	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(released, stillReadsBound).Build(),
	}
	target := migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1)

	accepted := withNodesAction(target.DeepCopy(), map[string]string{"node1": v1.NodeActionAdd})
	assert.NilError(t, validator.validateNodesAction(context.Background(), accepted, target))

	refused := withNodesAction(target.DeepCopy(), map[string]string{"node2": v1.NodeActionAdd})
	assert.Assert(t, validator.validateNodesAction(context.Background(), refused, target) != nil)
}

// A migration takes the node away from whatever is running on it, exactly as a removal does,
// and is gated on the same check.
func TestValidateNodesActionMigrateRefusesABusyNode(t *testing.T) {
	scheme := newScheme(t)
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workload1",
			Labels: map[string]string{
				v1.ClusterIdLabel:   migrateCluster,
				v1.WorkspaceIdLabel: "ws-a",
			},
			Annotations: map[string]string{v1.WorkloadDispatchedAnnotation: v1.TrueStr},
		},
		Status: v1.WorkloadStatus{
			NodeUsage: []v1.NodePodUsage{{Node: "node1", Running: map[string]int{"0": 1}}},
		},
	}
	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"),
			migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
			workload,
		).Build(),
	}

	oldWs := migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1)
	newWs := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1),
		map[string]string{"node1": v1.BuildMigrateAction("ws-b")})
	assert.Assert(t, validator.validateNodesAction(context.Background(), newWs, oldWs) != nil)

	// Forced, it goes through, the same way a forced removal does.
	v1.SetAnnotation(newWs, v1.WorkspaceForcedAction, v1.TrueStr)
	assert.NilError(t, validator.validateNodesAction(context.Background(), newWs, oldWs))
}

// The reservation is only worth as much as the paths that honour it. Scaling up is not the
// only way a node gets claimed -- a user can name it in a workspace's node action, and that
// path used to take a released node without a second thought.
func TestValidateNodesActionAddRefusesSomeoneElsesReservedNode(t *testing.T) {
	scheme := newScheme(t)
	// Released by ws-a, on its way to ws-b, and not bound to anything in the meantime.
	node := releasedForMigration(migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-b")
	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build(),
	}
	thirdParty := migrateWorkspace("ws-c", migrateCluster, migrateFlavor, 1)

	// Refused where the user can see it, so the request does not come back accepted and then
	// fail out of sight for the whole of the migration's timeout.
	assert.Assert(t, validator.validateNodesAction(context.Background(),
		withNodesAction(thirdParty.DeepCopy(), map[string]string{"node1": v1.NodeActionAdd}), thirdParty) != nil)

	nodeValidator := &NodeValidator{}
	bound := node.DeepCopy()
	bound.Spec.Workspace = pointer.String("ws-c")
	err := nodeValidator.validateImmutableFields(bound, node)
	assert.Assert(t, err != nil, "ws-c was allowed to bind a node reserved for ws-b")

	// The workspace it was released for is the one exception.
	toTarget := node.DeepCopy()
	toTarget.Spec.Workspace = pointer.String("ws-b")
	assert.NilError(t, nodeValidator.validateImmutableFields(toTarget, node))
}

// The reservation lives on the node, so it cannot also be what excuses the node: whoever can
// write the annotation could otherwise hand a bound node to another workspace, and the
// workspace still holding it would never have its replica lowered to match.
func TestValidateNodesActionAddRefusesABoundNodeWhateverItsAnnotationSays(t *testing.T) {
	scheme := newScheme(t)
	node := migrateNode("node1", migrateCluster, migrateFlavor, "ws-a")
	v1.SetNodeMigrateInfo(node, &v1.NodeMigrateInfo{
		From:      "ws-a",
		Target:    "ws-b",
		StartTime: &metav1.Time{Time: time.Now().UTC()},
	})
	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build(),
	}
	target := migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1)
	err := validator.validateNodesAction(context.Background(),
		withNodesAction(target.DeepCopy(), map[string]string{"node1": v1.NodeActionAdd}), target)
	assert.Assert(t, err != nil, "a bound node was added on the strength of its own annotation")
}

// A reservation nobody is driving any more must not make the node unbindable for good: the
// workspace that was driving it can be deleted mid-crossing, and a node can leave the cluster
// and come back still carrying the annotation.
func TestNodeMigrationReservationStopsHoldingWhenItExpires(t *testing.T) {
	nodeValidator := &NodeValidator{}
	stale := migrateNode("node1", migrateCluster, migrateFlavor, "")
	v1.SetNodeMigrateInfo(stale, &v1.NodeMigrateInfo{
		From:      "ws-a",
		Target:    "ws-b",
		StartTime: &metav1.Time{Time: time.Now().UTC().Add(-2 * v1.DefaultNodeMigrateTimeout)},
	})
	bound := stale.DeepCopy()
	bound.Spec.Workspace = pointer.String("ws-c")
	assert.NilError(t, nodeValidator.validateImmutableFields(bound, stale))

	// A reservation with no start time cannot be aged, so it is not honoured at all.
	noClock := migrateNode("node2", migrateCluster, migrateFlavor, "")
	v1.SetNodeMigrateInfo(noClock, &v1.NodeMigrateInfo{From: "ws-a", Target: "ws-b"})
	boundNoClock := noClock.DeepCopy()
	boundNoClock.Spec.Workspace = pointer.String("ws-c")
	assert.NilError(t, nodeValidator.validateImmutableFields(boundNoClock, noClock))
}

// A migration's action stays on the source for the whole crossing, and the target is busy for
// most of it carrying out that same migration. Re-judging the standing request on every
// unrelated update refuses edits that have nothing to do with nodes, and names a workspace
// the user never touched.
func TestValidateNodesActionSkipsAnActionThatIsNotBeingChanged(t *testing.T) {
	scheme := newScheme(t)
	node := migrateNode("node1", migrateCluster, migrateFlavor, "ws-a")
	busyTarget := withNodesAction(migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1),
		map[string]string{"node1": v1.NodeActionAdd})
	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, busyTarget).Build(),
	}

	action := map[string]string{"node1": v1.BuildMigrateAction("ws-b")}
	oldWs := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1), action)
	renamed := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1), action)
	v1.SetLabel(renamed, v1.DisplayNameLabel, "renamed")

	assert.NilError(t, validator.validateNodesAction(context.Background(), renamed, oldWs))
}

// The same request arriving twice is one request. The annotation's text is what triggers the
// accounting, and the same action can arrive again spelled differently -- a client retrying,
// or the same JSON with different spacing -- so the entries already carried are left alone.
func TestMutateNodesActionCountsAnActionOnceHoweverOftenItIsSent(t *testing.T) {
	scheme := newScheme(t)
	node := migrateNode("node1", migrateCluster, migrateFlavor, "ws-a")
	mutator := &WorkspaceMutator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build(),
	}
	action := map[string]string{"node1": v1.BuildMigrateAction("ws-b")}

	first := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 3), action)
	assert.NilError(t, mutator.mutateNodesAction(context.Background(),
		migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 3), first))
	assert.Equal(t, first.Spec.Replica, 2)

	resent := withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 2), action)
	assert.NilError(t, mutator.mutateNodesAction(context.Background(),
		withNodesAction(migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 2), action), resent))
	assert.Equal(t, resent.Spec.Replica, 2, "the same request was counted twice")

	// An add resent the same way is the same story, and was before migrations existed.
	free := migrateNode("node2", migrateCluster, migrateFlavor, "")
	addMutator := &WorkspaceMutator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(free).Build(),
	}
	addAction := map[string]string{"node2": v1.NodeActionAdd}
	added := withNodesAction(migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1), addAction)
	assert.NilError(t, addMutator.mutateNodesAction(context.Background(),
		withNodesAction(migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1), addAction), added))
	assert.Equal(t, added.Spec.Replica, 1)
}

// Only the workspace that released the node may go on driving the crossing; matching on the
// target alone lets any workspace take over a migration someone else started.
func TestValidateNodesActionMigrateOnlyByTheWorkspaceThatReleasedIt(t *testing.T) {
	scheme := newScheme(t)
	node := releasedForMigration(migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-b")
	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node,
			migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1)).Build(),
	}
	action := map[string]string{"node1": v1.BuildMigrateAction("ws-b")}

	interloper := migrateWorkspace("ws-c", migrateCluster, migrateFlavor, 1)
	assert.Assert(t, validator.validateNodesAction(context.Background(),
		withNodesAction(interloper.DeepCopy(), action), interloper) != nil)

	owner := migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1)
	assert.NilError(t, validator.validateNodesAction(context.Background(),
		withNodesAction(owner.DeepCopy(), action), owner))
}

// A target on its way out cannot take the nodes, and finding that out after they have been
// released costs a whole timeout and the source's capacity.
func TestValidateMigrateTargetRefusesAWorkspaceBeingDeleted(t *testing.T) {
	scheme := newScheme(t)
	node := migrateNode("node1", migrateCluster, migrateFlavor, "ws-a")
	deleting := migrateWorkspace("ws-b", migrateCluster, migrateFlavor, 1)
	now := metav1.NewTime(time.Now())
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{v1.WorkspaceFinalizer}
	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node, deleting).Build(),
	}
	source := migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 1)
	assert.Assert(t, validator.validateNodesAction(context.Background(),
		withNodesAction(source.DeepCopy(), map[string]string{"node1": v1.BuildMigrateAction("ws-b")}),
		source) != nil)
}

// Which node decides an adopting target's flavor is settled before the batch is walked. Left
// to whichever node is looked at while the flavor is still unset, a node carrying no flavor
// leaves it unset for the next one to fill in, and a batch of mixed flavors goes through --
// to be refused on arrival, one node at a time, after the source has let them all go.
func TestValidateMigrateTargetRefusesAMixedBatchIncludingAFlavourlessNode(t *testing.T) {
	scheme := newScheme(t)
	flavourless := migrateNode("node1", migrateCluster, "", "ws-a")
	delete(flavourless.Labels, v1.NodeFlavorIdLabel)
	other := migrateNode("node2", migrateCluster, migrateFlavor, "ws-a")
	adopting := migrateWorkspace("ws-b", migrateCluster, "", 0)
	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(flavourless, other, adopting).Build(),
	}
	source := migrateWorkspace("ws-a", migrateCluster, migrateFlavor, 2)
	action := map[string]string{
		"node1": v1.BuildMigrateAction("ws-b"),
		"node2": v1.BuildMigrateAction("ws-b"),
	}
	assert.Assert(t, validator.validateNodesAction(context.Background(),
		withNodesAction(source.DeepCopy(), action), source) != nil)
}
