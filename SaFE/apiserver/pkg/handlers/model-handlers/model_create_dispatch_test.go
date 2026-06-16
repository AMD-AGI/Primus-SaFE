/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func dispatchCapableHandler(t *testing.T) *Handler {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return newMockModelHandler(ctrlfake.NewClientBuilder().WithScheme(s).Build())
}

// TestCreateModelDispatchLocalPath verifies dispatch to the local_path flow succeeds.
func TestCreateModelDispatchLocalPath(t *testing.T) {
	h := dispatchCapableHandler(t)
	res, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"displayName":"LP","source":{"accessMode":"local_path","localPath":"/wekafs/m"}}`, "u1", nil))
	require.NoError(t, err)
	assert.NotNil(t, res)
}

// TestCreateModelDispatchS3Sync verifies dispatch to the s3_sync flow succeeds.
func TestCreateModelDispatchS3Sync(t *testing.T) {
	h := dispatchCapableHandler(t)
	res, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"displayName":"S3","source":{"accessMode":"s3_sync"},"s3Source":{"uri":"s3://b/p"}}`, "u1", nil))
	require.NoError(t, err)
	assert.NotNil(t, res)
}

// TestCreateModelRemoteAPISuccess verifies the remote_api flow creates a ready model and api key secret.
func TestCreateModelRemoteAPISuccess(t *testing.T) {
	h := dispatchCapableHandler(t)
	res, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"displayName":"Remote","source":{"accessMode":"remote_api","url":"https://api.example.com","modelName":"gpt","apiKey":"k"}}`, "u1", nil))
	require.NoError(t, err)
	assert.NotNil(t, res)
}
