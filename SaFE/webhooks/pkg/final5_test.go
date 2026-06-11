/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"

	"gotest.tools/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestWorkspaceMutateOnUpdateScaleDownRoute covers the scale-down routing branch.
func TestWorkspaceMutateOnUpdateScaleDownRoute(t *testing.T) {
	scheme := newScheme(t)
	m := &WorkspaceMutator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldWs := validWorkspace("ws1")
	oldWs.Spec.Replica = 1
	newWs := validWorkspace("ws1")
	newWs.Spec.Replica = 1
	assert.NilError(t, m.mutateOnUpdate(context.Background(), oldWs, newWs))
}

// TestNodeValidateOnCreationFlavorMissing covers node creation with a missing flavor.
func TestNodeValidateOnCreationFlavorMissing(t *testing.T) {
	scheme := newScheme(t)
	v := &NodeValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	node := validNode()
	assert.Assert(t, v.validateOnCreation(context.Background(), node) != nil)
}

// TestOpsJobValidateOnUpdateRequiredParams covers ops update required-param validation.
func TestOpsJobValidateOnUpdateRequiredParams(t *testing.T) {
	scheme := newScheme(t)
	v := &OpsJobValidator{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	oldJob := opsJobWithDisplayName("job1", v1.OpsJobCDType)
	newJob := &v1.OpsJob{} // missing required params
	assert.Assert(t, v.validateOnUpdate(context.Background(), newJob, oldJob) != nil)
}
