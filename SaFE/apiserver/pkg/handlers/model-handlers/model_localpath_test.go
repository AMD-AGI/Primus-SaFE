/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// TestCreateModelFromLocalPathSuccess verifies a Ready model CR is created from a local path.
func TestCreateModelFromLocalPathSuccess(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()
	h := newMockModelHandler(cl)

	req := &CreateModelRequest{
		DisplayName: "My Model",
		Source: ModelSourceReq{
			AccessMode: "local_path",
			LocalPath:  "/wekafs/models/my-model",
		},
	}
	res, err := h.createModelFromLocalPath(context.Background(), req, "uid-1", "user-1")
	require.NoError(t, err)
	resp, ok := res.(*CreateResponse)
	require.True(t, ok)
	assert.NotEmpty(t, resp.ID)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	assert.Equal(t, v1.AccessModeLocalPath, created.Spec.Source.AccessMode)
	assert.Equal(t, v1.ModelPhaseReady, created.Status.Phase)
	assert.Equal(t, "external", created.Spec.Origin)
}

// TestCreateModelFromLocalPathFineTuned verifies origin defaults to fine_tuned when sftJobId is set.
func TestCreateModelFromLocalPathFineTuned(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()
	h := newMockModelHandler(cl)

	req := &CreateModelRequest{
		DisplayName: "Tuned Model",
		SftJobId:    "sft-123",
		Source: ModelSourceReq{
			AccessMode: "local_path",
			LocalPath:  "/wekafs/models/tuned",
		},
	}
	res, err := h.createModelFromLocalPath(context.Background(), req, "uid-1", "user-1")
	require.NoError(t, err)
	resp := res.(*CreateResponse)

	created := &v1.Model{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{Name: resp.ID}, created))
	assert.Equal(t, "fine_tuned", created.Spec.Origin)
	assert.Equal(t, "sft-123", created.Spec.SftJobId)
}

// TestCreateModelFromLocalPathMissingLocalPath verifies validation of the local path.
func TestCreateModelFromLocalPathMissingLocalPath(t *testing.T) {
	h := newMockModelHandler(ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build())
	req := &CreateModelRequest{DisplayName: "M"}
	_, err := h.createModelFromLocalPath(context.Background(), req, "", "")
	assert.Error(t, err)
}

// TestCreateModelFromLocalPathMissingDisplayName verifies validation of the display name.
func TestCreateModelFromLocalPathMissingDisplayName(t *testing.T) {
	h := newMockModelHandler(ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build())
	req := &CreateModelRequest{Source: ModelSourceReq{LocalPath: "/x"}}
	_, err := h.createModelFromLocalPath(context.Background(), req, "", "")
	assert.Error(t, err)
}
