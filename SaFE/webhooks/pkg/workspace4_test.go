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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestWorkspaceValidateCommonStepErrors covers validateCommon return-error branches.
func TestWorkspaceValidateCommonStepErrors(t *testing.T) {
	ctx := context.Background()
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// required params error
	assert.Assert(t, v.validateCommon(ctx, &v1.Workspace{}, nil) != nil)

	// bad dns display name
	dns := validWorkspace("ws1")
	v1.SetLabel(dns, v1.DisplayNameLabel, "Bad.Name")
	assert.Assert(t, v.validateCommon(ctx, dns, nil) != nil)

	// related resource missing flavor (replica increase)
	related := validWorkspace("ws1")
	related.Spec.NodeFlavor = "missing"
	related.Spec.Replica = 2
	assert.Assert(t, v.validateCommon(ctx, related, nil) != nil)
}

// TestWorkspaceValidateOnUpdateStepErrors covers validateOnUpdate return-error branches.
func TestWorkspaceValidateOnUpdateStepErrors(t *testing.T) {
	ctx := context.Background()
	scheme := newScheme(t)
	v := &WorkspaceValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}

	// immutable cluster change
	oldWs := validWorkspace("ws1")
	clusterChanged := validWorkspace("ws1")
	clusterChanged.Spec.Cluster = "cluster2"
	assert.Assert(t, v.validateOnUpdate(ctx, clusterChanged, oldWs) != nil)

	// nodes action references missing node
	nodesAction := validWorkspace("ws1")
	v1.SetAnnotation(nodesAction, v1.WorkspaceNodesAction, `{"missing":"add"}`)
	assert.Assert(t, v.validateOnUpdate(ctx, nodesAction, oldWs) != nil)
}

// TestWorkspaceValidateOnUpdateScaleDown covers the scale-down source workload branch.
func TestWorkspaceValidateOnUpdateScaleDown(t *testing.T) {
	scheme := newScheme(t)
	src := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "src"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(src).Build()
	v := &WorkspaceValidator{Client: c}

	oldWs := validWorkspace("ws1")
	oldWs.Spec.Replica = 2
	newWs := validWorkspace("ws1")
	newWs.Spec.Replica = 1
	v1.SetLabel(newWs, v1.SourceWorkloadIdLabel, "src")
	assert.Assert(t, v.validateOnUpdate(context.Background(), newWs, oldWs) != nil)
}

// TestWorkspaceMutateOnUpdatePreempt covers preempt-driven workload mutation routing.
func TestWorkspaceMutateOnUpdatePreempt(t *testing.T) {
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkspaceMutator{Client: c}

	oldWs := validWorkspace("ws1")
	newWs := validWorkspace("ws1")
	newWs.Spec.EnablePreempt = true
	assert.NilError(t, m.mutateOnUpdate(context.Background(), oldWs, newWs))
}

// TestWorkspaceMutatorHandleFullCreate covers the mutator create marshal/patch path.
func TestWorkspaceMutatorHandleFullCreate(t *testing.T) {
	scheme := newScheme(t)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
	m := &WorkspaceMutator{Client: c, decoder: newDecoder(t)}
	resp := m.Handle(context.Background(), newRequest(t, admissionv1.Create, validWorkspace("ws1"), nil))
	assert.Assert(t, resp.Allowed)
}
