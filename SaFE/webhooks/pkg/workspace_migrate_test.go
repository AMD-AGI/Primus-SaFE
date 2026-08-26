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

// The handover writes the add on the target while the node may still read as bound to the
// source. That one node is the exception; a node bound to anyone else is not.
func TestValidateNodesActionAddAcceptsOnlyItsOwnMigration(t *testing.T) {
	scheme := newScheme(t)
	arriving := releasedForMigration(migrateNode("node1", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-b")
	arriving.Spec.Workspace = pointer.String("ws-a") // the release has not been read back yet
	elsewhere := releasedForMigration(migrateNode("node2", migrateCluster, migrateFlavor, "ws-a"), "ws-a", "ws-c")
	elsewhere.Spec.Workspace = pointer.String("ws-a")

	validator := &WorkspaceValidator{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(arriving, elsewhere).Build(),
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
