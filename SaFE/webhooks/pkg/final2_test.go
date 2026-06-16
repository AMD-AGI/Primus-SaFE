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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestWorkspaceMutatorHandleBranches covers workspace mutator deletion and error branches.
func TestWorkspaceMutatorHandleBranches(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}

	// deletion timestamp -> allowed
	now := metav1.Now()
	deleting := validWorkspace("ws1")
	deleting.DeletionTimestamp = &now
	deleting.Finalizers = []string{"x"}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, deleting, nil))
	assert.Assert(t, resp.Allowed)

	// creation error: cluster lookup fails
	ws := validWorkspace("ws1")
	ws.Spec.Cluster = "missing"
	resp = m.Handle(context.Background(), newRequest(t, admissionv1.Create, ws, nil))
	assert.Assert(t, !resp.Allowed)
}

// TestWorkspaceValidateVolumeRemovedNoWorkload covers volume removal with no running workloads.
func TestWorkspaceValidateVolumeRemovedNoWorkload(t *testing.T) {
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster: "cluster1",
		Volumes: []v1.WorkspaceVolume{
			{Id: 1, Type: v1.HOSTPATH, MountPath: "/h", HostPath: "/h"},
			{Id: 2, Type: v1.PFS, MountPath: "/pfs", StorageClass: "sc", Capacity: "100Gi"},
		},
	}}
	v1.SetLabel(oldWs, v1.ClusterIdLabel, "cluster1")
	newWs := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}, Spec: v1.WorkspaceSpec{
		Cluster: "cluster1",
		Volumes: []v1.WorkspaceVolume{{Id: 1, Type: v1.HOSTPATH, MountPath: "/h", HostPath: "/h"}},
	}}
	v1.SetLabel(newWs, v1.ClusterIdLabel, "cluster1")
	assert.NilError(t, v.validateVolumeRemoved(context.Background(), newWs, oldWs))
}

// TestNodeMutatorHandleDeletion covers the node mutator deletion-timestamp branch.
func TestNodeMutatorHandleDeletion(t *testing.T) {
	scheme := newScheme(t)
	m := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), decoder: newDecoder(t)}
	now := metav1.Now()
	node := validNode()
	node.DeletionTimestamp = &now
	node.Finalizers = []string{"x"}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Update, node, node))
	assert.Assert(t, resp.Allowed)
}

// TestNodeMutateLabelsRemoveEmpty covers empty workspace/cluster label removal.
func TestNodeMutateLabelsRemoveEmpty(t *testing.T) {
	scheme := newScheme(t)
	m := &NodeMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
		v1.WorkspaceIdLabel: "",
		v1.ClusterIdLabel:   "",
	}}, Spec: v1.NodeSpec{Hostname: pointer.String("h1")}}
	assert.Assert(t, m.mutateLabels(context.Background(), node))
}

// TestNodeValidateNodeSpecWorkspaceMissing covers node workspace existence error in spec validation.
func TestNodeValidateNodeSpecWorkspaceMissing(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := validNode()
	node.Spec.Workspace = pointer.String("missing")
	assert.Assert(t, v.validateNodeSpec(context.Background(), node) != nil)
}

// TestNodeValidateOnUpdateImmutable covers node update immutable validation.
func TestNodeValidateOnUpdateImmutable(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldNode := validNode()
	newNode := validNode()
	newNode.Spec.Hostname = pointer.String("changed")
	assert.Assert(t, v.validateOnUpdate(context.Background(), newNode, oldNode) != nil)
}

// TestUserValidateOnUpdateImmutable covers user update immutable validation.
func TestUserValidateOnUpdateImmutable(t *testing.T) {
	scheme := newScheme(t)
	v := &UserValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldUser := validUser("u1")
	newUser := validUser("u1")
	newUser.Spec.Type = v1.SSOUserType
	assert.Assert(t, v.validateOnUpdate(context.Background(), newUser, oldUser) != nil)
}

// TestFaultValidatorHandleUpdateDeleting covers fault validator update deletion branch.
func TestFaultValidatorHandleUpdateDeleting(t *testing.T) {
	v := &FaultValidator{decoder: newDecoder(t)}
	now := metav1.Now()
	fault := &v1.Fault{ObjectMeta: metav1.ObjectMeta{Name: "f1"}, Spec: v1.FaultSpec{MonitorId: "m1"}}
	fault.DeletionTimestamp = &now
	fault.Finalizers = []string{"x"}
	resp := v.Handle(context.Background(), newRequest(t, admissionv1.Update, fault, fault))
	assert.Assert(t, resp.Allowed)
}
