/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// deletingK8sModel is a local model whose deletion is still removing path.
func deletingK8sModel(name, workspace, path string) *v1.Model {
	m := genMockLocalK8sModel(name, workspace)
	now := metav1.Now()
	m.DeletionTimestamp = &now
	m.Finalizers = []string{"model.amd.com/finalizer"}
	m.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: workspace, Path: path, Status: v1.LocalPathStatusReady}}
	return m
}

// TestRejectTargetBeingDeleted: a new local model may not download into a directory
// that the deletion of another model is still cleaning up.
func TestRejectTargetBeingDeleted(t *testing.T) {
	ctx := context.Background()
	ws := genMockWorkspace("ws1", "/data")
	candidate := &v1.Model{Spec: v1.ModelSpec{
		Workspace:     "ws1",
		TargetSubpath: "team/models/hf",
		Source:        v1.ModelSource{URL: "https://huggingface.co/org/repo", AccessMode: v1.AccessModeLocal},
	}}

	old := deletingK8sModel("old", "ws1", "/data/team/models/hf/org--repo")
	h := newMockModelHandler(fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws, old).Build())
	err := h.rejectTargetBeingDeleted(ctx, candidate)
	require.Error(t, err)
	assert.Equal(t, commonerrors.ResourceProcessing, string(apierrors.ReasonForError(err)), "got %v", err)
	assert.Contains(t, err.Error(), "old")

	// Public model: every workspace's directory is checked.
	public := candidate.DeepCopy()
	public.Spec.Workspace = ""
	assert.Error(t, h.rejectTargetBeingDeleted(ctx, public))

	// Another directory, or a live model on the same directory, is not a deletion race.
	other := candidate.DeepCopy()
	other.Spec.Source.URL = "https://huggingface.co/org/other"
	assert.NoError(t, h.rejectTargetBeingDeleted(ctx, other))
	live := genMockLocalK8sModel("live", "ws1")
	live.Status.LocalPaths = []v1.ModelLocalPath{{Workspace: "ws1", Path: "/data/team/models/hf/org--repo"}}
	h = newMockModelHandler(fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(ws, live).Build())
	assert.NoError(t, h.rejectTargetBeingDeleted(ctx, candidate))
}

// TestListModelsIncludeDeleting: a model whose files are still being removed is hidden
// by default and listed, marked deleted, on request.
func TestListModelsIncludeDeleting(t *testing.T) {
	live := genMockLocalK8sModel("live", "ws1")
	gone := deletingK8sModel("gone", "ws1", "/data/models/org--repo")
	gone.Status.LocalPaths[0].SizeBytes = 42
	h := newMockModelHandler(fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(live, gone).Build())

	list := func(query string) *ListModelResponse {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request, _ = http.NewRequest("GET", "/models?"+query, nil)
		c.Set(common.UserId, adminModelUserID)
		res, err := h.listModels(c)
		require.NoError(t, err)
		return res.(*ListModelResponse)
	}

	resp := list("")
	require.Equal(t, int64(1), resp.Total)
	assert.Equal(t, "live", resp.Items[0].ID)

	resp = list("includeDeleting=true")
	require.Equal(t, int64(2), resp.Total)
	var deleting *ModelInfo
	for i := range resp.Items {
		if resp.Items[i].ID == "gone" {
			deleting = &resp.Items[i]
		}
	}
	require.NotNil(t, deleting)
	assert.True(t, deleting.IsDeleted)
	assert.NotEmpty(t, deleting.DeletionTime)
	require.Len(t, deleting.LocalPaths, 1)
	assert.Equal(t, "/data/models/org--repo", deleting.LocalPaths[0].Path)
	assert.Equal(t, int64(42), deleting.LocalPaths[0].SizeBytes)
}
