/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// createReadOnlyStorage creates a PyTorchJob in hpWorkspace as a workspace member with the given
// extra request fields and returns the stored workload.
func (f *hostPathFixture) createReadOnlyStorage(t *testing.T, extra map[string]any) *v1.Workload {
	t.Helper()
	fields := map[string]any{
		"displayName": "ro",
		"workspaceId": hpWorkspace,
		"images":      []string{"img"},
		"entryPoints": []string{"sleep"},
		"groupVersionKind": map[string]string{
			"version": "v1", "kind": common.PytorchJobKind,
		},
		"resources": []map[string]any{{"replica": 1, "cpu": "1", "memory": "1Gi"}},
	}
	for k, val := range extra {
		fields[k] = val
	}
	body, err := json.Marshal(fields)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, hpMember)
	res, err := f.h.createWorkload(c)
	require.NoError(t, err)
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(),
		client.ObjectKey{Name: res.(*view.CreateWorkloadResponse).WorkloadId}, stored))
	return stored
}

// Any member may ask for read-only workspace storage, since it only takes write access away; the
// workload then carries the annotation, and still uses workspace storage. Without the field, or
// with it false, there is no annotation, and a request cannot set the annotation directly.
func TestCreateWorkloadWorkspaceStorageReadOnly(t *testing.T) {
	f := newHostPathFixture(t)

	stored := f.createReadOnlyStorage(t, map[string]any{"workspaceStorageReadOnly": true})
	require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkspaceStorageReadOnlyAnnotation))
	require.True(t, v1.IsEnableWorkspaceStorage(stored))

	for _, extra := range []map[string]any{
		nil,
		{"workspaceStorageReadOnly": false},
		{"annotations": map[string]string{v1.WorkspaceStorageReadOnlyAnnotation: v1.TrueStr}},
	} {
		stored = f.createReadOnlyStorage(t, extra)
		require.False(t, v1.HasAnnotation(stored, v1.WorkspaceStorageReadOnlyAnnotation), "%v", extra)
	}
}
