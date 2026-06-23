/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

// TestWorkspaceValidateVolumesImmutable covers volume immutable-field error branches.
func TestWorkspaceValidateVolumesImmutable(t *testing.T) {
	v := &WorkspaceValidator{}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc1", Capacity: "100Gi", AccessMode: corev1.ReadWriteMany},
	}}}

	scChanged := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc2", Capacity: "100Gi", AccessMode: corev1.ReadWriteMany},
	}}}
	assert.Assert(t, v.validateVolumes(scChanged, oldWs) != nil)

	capChanged := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc1", Capacity: "200Gi", AccessMode: corev1.ReadWriteMany},
	}}}
	assert.Assert(t, v.validateVolumes(capChanged, oldWs) != nil)

	zeroCap := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 2, Type: v1.PFS, MountPath: "/p", StorageClass: "sc", Capacity: "0"},
	}}}
	assert.Assert(t, v.validateVolumes(zeroCap, nil) != nil)

	badAccess := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 3, Type: v1.PFS, MountPath: "/p", StorageClass: "sc", Capacity: "10Gi", AccessMode: "Bad"},
	}}}
	assert.Assert(t, v.validateVolumes(badAccess, nil) != nil)
}

// TestWorkspaceValidateNodesActionErrors covers node action validation error branches.
func TestWorkspaceValidateNodesActionErrors(t *testing.T) {
	scheme := newScheme(t)

	// node bound elsewhere cannot be added
	bound := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{v1.ClusterIdLabel: "cluster1"}},
		Spec:       v1.NodeSpec{Workspace: pointer.String("other")},
	}
	c1 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bound).Build()
	v1v := &WorkspaceValidator{Client: c1}
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	assert.Assert(t, v1v.validateNodesAction(context.Background(), ws, &v1.Workspace{}) != nil)

	// node not found
	c2 := fake.NewClientBuilder().WithScheme(scheme).Build()
	v2 := &WorkspaceValidator{Client: c2}
	ws2 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws2, v1.WorkspaceNodesAction, `{"missing":"add"}`)
	assert.Assert(t, v2.validateNodesAction(context.Background(), ws2, &v1.Workspace{}) != nil)

	// cluster mismatch
	wrongCluster := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n2", Labels: map[string]string{v1.ClusterIdLabel: "other"}},
	}
	c3 := fake.NewClientBuilder().WithScheme(scheme).WithObjects(wrongCluster).Build()
	v3 := &WorkspaceValidator{Client: c3}
	ws3 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(ws3, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	assert.Assert(t, v3.validateNodesAction(context.Background(), ws3, &v1.Workspace{}) != nil)
}

// TestWorkspaceMutateNodesActionErrors covers node action mutation error branches.
func TestWorkspaceMutateNodesActionErrors(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// replica change with action set -> error
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", Replica: 1}}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", Replica: 2}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	assert.Assert(t, m.mutateNodesAction(context.Background(), oldWs, newWs) != nil)

	// node not found -> error
	oldWs2 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	newWs2 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(newWs2, v1.WorkspaceNodesAction, `{"missing":"add"}`)
	assert.Assert(t, m.mutateNodesAction(context.Background(), oldWs2, newWs2) != nil)

	// flavor mismatch -> error
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavorX",
		}},
		Spec: v1.NodeSpec{Workspace: pointer.String("")},
	}
	mc := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()}
	oldWs3 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 1}}
	newWs3 := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1", NodeFlavor: "flavor1", Replica: 1}}
	v1.SetAnnotation(newWs3, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	assert.Assert(t, mc.mutateNodesAction(context.Background(), oldWs3, newWs3) != nil)
}

// TestWorkspaceMutateManagersRemove covers manager removal mutation.
func TestWorkspaceMutateManagersRemove(t *testing.T) {
	scheme := newScheme(t)
	u := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}}
	commonuser.AssignWorkspace(u, "ws1")
	commonuser.AssignManagedWorkspace(u, "ws1")
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(u).Build()
	m := &WorkspaceMutator{Client: k8sClient}

	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Managers: []string{"u1"}}}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Managers: []string{}}}
	assert.NilError(t, m.mutateManagers(context.Background(), oldWs, newWs))

	updated := &v1.User{}
	assert.NilError(t, k8sClient.Get(context.Background(), client.ObjectKey{Name: "u1"}, updated))
	assert.Assert(t, !commonuser.HasWorkspaceManagedRight(updated, "ws1"))
}

// TestWorkspaceValidateRequiredParamsBranches covers required-param error branches.
func TestWorkspaceValidateRequiredParamsBranches(t *testing.T) {
	v := &WorkspaceValidator{}
	// bad queue policy + empty displayName
	w := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster:     "cluster1",
		QueuePolicy: "bad",
	}}
	v1.SetLabel(w, v1.ClusterIdLabel, "cluster1")
	assert.Assert(t, v.validateRequiredParams(w) != nil)
}
