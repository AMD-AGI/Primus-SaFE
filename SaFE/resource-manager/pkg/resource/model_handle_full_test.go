/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestHandlePendingLocalModelFull(t *testing.T) {
	model := genMockLocalModel("local-pending", "")
	model.Status.Phase = v1.ModelPhasePending

	mockScheme, err := genMockScheme()
	assert.NoError(t, err)
	assert.NoError(t, batchv1.AddToScheme(mockScheme))
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)

	_, err = r.handlePending(context.Background(), model)
	assert.NoError(t, err)
	// Local model either starts uploading or fails when the download job can't be built.
	assert.Contains(t, []v1.ModelPhase{v1.ModelPhaseUploading, v1.ModelPhaseFailed}, model.Status.Phase)
}

func TestHandleDeleteLocalModelFull(t *testing.T) {
	model := genMockLocalModel("local-delete", "")
	controllerutil.AddFinalizer(model, ModelFinalizer)

	mockScheme, err := genMockScheme()
	assert.NoError(t, err)
	assert.NoError(t, batchv1.AddToScheme(mockScheme))
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)

	_, err = r.handleDelete(context.Background(), model)
	assert.NoError(t, err)
}

func TestModelFailoverHelpers(t *testing.T) {
	r := &ModelReconciler{}
	assert.Equal(t, "/wekafs", r.extractBasePath("/wekafs/models/llama"))
	assert.Equal(t, "", r.extractBasePath("/no-models-here"))

	model := genMockLocalModel("m-tried", "")
	// initially empty
	assert.Empty(t, r.getTriedWorkspaces(model, "/wekafs"))
	// set then read back
	r.setTriedWorkspaces(model, "/wekafs", []string{"ws1", "ws2"})
	got := r.getTriedWorkspaces(model, "/wekafs")
	assert.Equal(t, []string{"ws1", "ws2"}, got)
}

func TestTryFailoverFull(t *testing.T) {
	model := genMockLocalModel("failover", "")
	mockScheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)
	ctx := context.Background()

	// empty path -> cannot determine base path
	assert.False(t, r.tryFailover(ctx, model, &v1.ModelLocalPath{Workspace: "ws1", Path: ""}))

	// valid path but no other workspaces sharing it -> no candidates
	assert.False(t, r.tryFailover(ctx, model, &v1.ModelLocalPath{Workspace: "ws1", Path: "/wekafs/models/failover"}))
}

func TestHandleDeleteNoFinalizer(t *testing.T) {
	model := genMockLocalModel("local-nofin", "")
	mockScheme, err := genMockScheme()
	assert.NoError(t, err)
	cl := fake.NewClientBuilder().WithObjects(model).WithStatusSubresource(model).WithScheme(mockScheme).Build()
	r := newMockModelReconciler(cl)

	_, err = r.handleDelete(context.Background(), model)
	assert.NoError(t, err)
}
