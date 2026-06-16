/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestWorkspaceMutateManagersUserNotFound covers removal of non-existent managers.
func TestWorkspaceMutateManagersUserNotFound(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Managers: []string{"missing"}}}
	assert.NilError(t, m.mutateManagers(context.Background(), nil, newWs))
	assert.Equal(t, len(newWs.Spec.Managers), 0)
}

// TestWorkspaceMutateGpuProductError covers the flavor-not-found error branch.
func TestWorkspaceMutateGpuProductError(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{NodeFlavor: "missing"}}
	assert.Assert(t, m.mutateGpuProduct(context.Background(), ws) != nil)
}

// TestWorkspaceValidateRelatedResourceClusterMissing covers cluster-not-found branch.
func TestWorkspaceValidateRelatedResourceClusterMissing(t *testing.T) {
	scheme := newScheme(t)
	nf := &v1.NodeFlavor{ObjectMeta: metav1.ObjectMeta{Name: "flavor1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nf).Build()
	v := &WorkspaceValidator{Client: c}
	ws := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 1, NodeFlavor: "flavor1", Cluster: "missing"}}
	assert.Assert(t, v.validateRelatedResource(context.Background(), ws) != nil)
}

// TestWorkspaceValidateNodesActionProcessing covers the concurrent-job processing branch.
func TestWorkspaceValidateNodesActionProcessing(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(oldWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{Cluster: "cluster1"}}
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n2":"add"}`)
	assert.Assert(t, v.validateNodesAction(context.Background(), newWs, oldWs) != nil)
}

// TestWorkspaceValidatorHandleBranches covers validator decode/deletion/update branches.
func TestWorkspaceValidatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}

	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Create, nil, nil))
	assert.Assert(t, !resp.Allowed)

	now := metav1.Now()
	deleting := validWorkspace("ws1")
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"x"}
	resp = v.Handle(context.Background(), newRequest(t, admissionv1.Update, deleting, deleting))
	assert.Assert(t, resp.Allowed)
}

// TestWorkspaceMutatorHandleUpdateNodesAction covers update routing into node action mutation.
func TestWorkspaceMutatorHandleUpdateNodesAction(t *testing.T) {
	scheme := newScheme(t)
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "n1", Labels: map[string]string{
			v1.ClusterIdLabel: "cluster1", v1.NodeFlavorIdLabel: "flavor1",
		}},
		Spec: v1.NodeSpec{Workspace: pointer.String("")},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
	m := &WorkspaceMutator{Client: c, decoder: newDecoder(t)}

	oldWs := validWorkspace("ws1")
	newWs := validWorkspace("ws1")
	v1.SetAnnotation(newWs, v1.WorkspaceNodesAction, `{"n1":"add"}`)
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, newWs, oldWs))
	assert.Assert(t, resp.Allowed)
}

// TestWorkspaceValidateVolumesSelectorChanged covers the selector immutability branch.
func TestWorkspaceValidateVolumesSelectorChanged(t *testing.T) {
	v := &WorkspaceValidator{}
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/p", Capacity: "10Gi", Selector: map[string]string{"a": "b"}, AccessMode: corev1.ReadWriteMany},
	}}}
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Volumes: []v1.WorkspaceVolume{
		{Id: 1, Type: v1.PFS, MountPath: "/p", Capacity: "10Gi", Selector: map[string]string{"c": "d"}, AccessMode: corev1.ReadWriteMany},
	}}}
	assert.Assert(t, v.validateVolumes(newWs, oldWs) != nil)
}

// TestGetWorkspaceError covers the workspace retrieval error path.
func TestGetWorkspaceError(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	_, err := getWorkspace(context.Background(), c, "missing")
	assert.Assert(t, err != nil)
}

// TestWorkspaceMutateScaleDownNoop covers the no-op scale-down branches.
func TestWorkspaceMutateScaleDownNoop(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	// newCount >= currentReplica -> nil
	oldWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 3}}
	oldWs.Status.AvailableReplica = 1
	newWs := &v1.Workspace{Spec: v1.WorkspaceSpec{Replica: 2}}
	assert.NilError(t, m.mutateScaleDown(context.Background(), oldWs, newWs))
}
