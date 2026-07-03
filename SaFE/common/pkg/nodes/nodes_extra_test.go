/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package nodes

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func nodesScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	assert.NoError(t, v1.AddToScheme(s))
	return s
}

func nodeWith(name, cluster, workspace string) *v1.Node {
	n := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if cluster != "" {
		n.Labels[v1.ClusterIdLabel] = cluster
	}
	if workspace != "" {
		n.Labels[v1.WorkspaceIdLabel] = workspace
	}
	return n
}

func runningWorkload(name, cluster, workspace, adminNode string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if cluster != "" {
		w.Labels[v1.ClusterIdLabel] = cluster
	}
	if workspace != "" {
		w.Labels[v1.WorkspaceIdLabel] = workspace
	}
	w.Status.Pods = []v1.WorkloadPod{{AdminNodeName: adminNode, Phase: corev1PodRunningPhase}}
	return w
}

// corev1PodRunningPhase keeps the import surface small; v1.IsPodRunning checks
// the pod phase is not a terminal one.
const corev1PodRunningPhase = "Running"

// offloadedWorkload mirrors runningWorkload but expresses the pod placement via
// the etcd NodeUsage aggregate (Status.Pods empty), as offloaded workloads do.
// The manual aggregate matches what BuildNodeUsage produces for one running,
// scheduled pod on adminNode (its node-set equivalence is locked separately by
// workload.TestNodeUsageNodeSetEquivalence).
func offloadedWorkload(name, cluster, workspace, adminNode string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if cluster != "" {
		w.Labels[v1.ClusterIdLabel] = cluster
	}
	if workspace != "" {
		w.Labels[v1.WorkspaceIdLabel] = workspace
	}
	w.Status.NodeUsage = []v1.NodePodUsage{{
		Node:    adminNode,
		Active:  map[string]int{"0": 1},
		Running: map[string]int{"0": 1},
	}}
	return w
}

// TestGetIdleNodesOfWorkspaceOffloadedDualRead verifies the NodeUsage read path
// yields the same idle-node result as the Status.Pods path: an offloaded workload
// (NodeUsage only, no Status.Pods) still marks its node as used.
func TestGetIdleNodesOfWorkspaceOffloadedDualRead(t *testing.T) {
	ctx := context.Background()
	idle := nodeWith("idle", "c1", "ws1")
	used := nodeWith("used", "c1", "ws1")
	wl := offloadedWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(idle, used, wl).Build()

	idleNodes, err := GetIdleNodesOfWorkspace(ctx, cl, "ws1")
	assert.NoError(t, err)
	assert.Len(t, idleNodes, 1)
	assert.Equal(t, "idle", idleNodes[0].Name)
}

// TestGetUsingNodesOfClusterOffloaded verifies GetUsingNodesOfCluster reads the
// node from the NodeUsage aggregate for an offloaded workload.
func TestGetUsingNodesOfClusterOffloaded(t *testing.T) {
	ctx := context.Background()
	wl := offloadedWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(wl).Build()
	set, err := GetUsingNodesOfCluster(ctx, cl, "c1")
	assert.NoError(t, err)
	assert.True(t, set.Has("used"))
}

func TestGetNodesOfWorkspacesAndCluster(t *testing.T) {
	ctx := context.Background()
	n1 := nodeWith("n1", "c1", "ws1")
	n2 := nodeWith("n2", "c1", "ws2")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(n1, n2).Build()

	ws, err := GetNodesOfWorkspaces(ctx, cl, []string{"ws1"}, nil)
	assert.NoError(t, err)
	assert.Len(t, ws, 1)

	// filter that drops everything
	wsNone, err := GetNodesOfWorkspaces(ctx, cl, []string{"ws1", "ws2"}, func(v1.Node) bool { return true })
	assert.NoError(t, err)
	assert.Empty(t, wsNone)

	cnodes, err := GetNodesOfCluster(ctx, cl, "c1", nil)
	assert.NoError(t, err)
	assert.Len(t, cnodes, 2)
}

func TestGetIdleNodesAndScalingDown(t *testing.T) {
	ctx := context.Background()
	idle := nodeWith("idle", "c1", "ws1")
	used := nodeWith("used", "c1", "ws1")
	wl := runningWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(idle, used, wl).Build()

	idleNodes, err := GetIdleNodesOfWorkspace(ctx, cl, "ws1")
	assert.NoError(t, err)
	assert.Len(t, idleNodes, 1)
	assert.Equal(t, "idle", idleNodes[0].Name)

	// count <= 0 -> error
	_, err = GetNodesForScalingDown(ctx, cl, "ws1", 0)
	assert.Error(t, err)

	down, err := GetNodesForScalingDown(ctx, cl, "ws1", 1)
	assert.NoError(t, err)
	assert.Len(t, down, 1)
}

func TestGetUsingNodesOfCluster(t *testing.T) {
	ctx := context.Background()
	wl := runningWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(wl).Build()
	set, err := GetUsingNodesOfCluster(ctx, cl, "c1")
	assert.NoError(t, err)
	assert.True(t, set.Has("used"))
}
