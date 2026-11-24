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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// createUserDirMounts creates a PyTorchJob in hpWorkspace as userId with the userDirMounts field of the
// request; a nil userDirMounts leaves the field out.
func (f *hostPathFixture) createUserDirMounts(t *testing.T, userId string,
	annotations map[string]string, userDirMounts *bool) (*v1.Workload, error) {
	t.Helper()
	fields := map[string]any{
		"displayName": "udm",
		"workspaceId": hpWorkspace,
		"images":      []string{"img"},
		"entryPoints": []string{"sleep"},
		"groupVersionKind": map[string]string{
			"version": "v1", "kind": common.PytorchJobKind,
		},
		"resources":   []map[string]any{{"replica": 1, "cpu": "1", "memory": "1Gi"}},
		"annotations": annotations,
	}
	if userDirMounts != nil {
		fields["userDirMounts"] = *userDirMounts
	}
	body, err := json.Marshal(fields)
	require.NoError(t, err)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/workloads", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, userId)
	res, err := f.h.createWorkload(c)
	if err != nil {
		return nil, err
	}
	stored := &v1.Workload{}
	require.NoError(t, f.cli.Get(t.Context(),
		client.ObjectKey{Name: res.(*view.CreateWorkloadResponse).WorkloadId}, stored))
	return stored, nil
}

// Only a system administrator may set userDirMounts to false; the request is refused for anyone else
// and no workload is created.
func TestUserDirMountsOffRequiresSystemAdmin(t *testing.T) {
	off := false
	for _, who := range []string{hpMember, hpManager} {
		f := newHostPathFixture(t)
		before := f.workloadCount(t)
		_, err := f.createUserDirMounts(t, who, nil, &off)
		require.Error(t, err, who)
		require.True(t, apierrors.IsForbidden(err), "%s: %v", who, err)
		require.Equal(t, before, f.workloadCount(t), who)
	}
}

// A system administrator's workload carries the opt-out mark when userDirMounts is false, and no mark
// when the field is unset or true.
func TestSystemAdminSetsUserDirMountsOff(t *testing.T) {
	off, on := false, true
	f := newHostPathFixture(t)

	stored, err := f.createUserDirMounts(t, hpAdmin, nil, &off)
	require.NoError(t, err)
	require.Equal(t, v1.TrueStr, v1.GetAnnotation(stored, v1.WorkloadUserDirMountsOffAnnotation))

	for _, value := range []*bool{nil, &on} {
		stored, err = f.createUserDirMounts(t, hpAdmin, nil, value)
		require.NoError(t, err)
		require.False(t, v1.HasAnnotation(stored, v1.WorkloadUserDirMountsOffAnnotation))
	}
}

// The mark is not what the request sends: a request that carries it, with or without the field, gets
// no mark unless the apiserver set it for a system administrator.
func TestUserDirMountsOffMarkCannotBeForged(t *testing.T) {
	forged := map[string]string{v1.WorkloadUserDirMountsOffAnnotation: v1.TrueStr}
	f := newHostPathFixture(t)

	stored, err := f.createUserDirMounts(t, hpMember, forged, nil)
	require.NoError(t, err)
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadUserDirMountsOffAnnotation))

	stored, err = f.createUserDirMounts(t, hpAdmin, forged, nil)
	require.NoError(t, err)
	require.False(t, v1.HasAnnotation(stored, v1.WorkloadUserDirMountsOffAnnotation))
}

// A patch of a workload that carries the opt-out mark needs a system administrator, so a workspace
// manager cannot change what runs with whole volumes; a workload without the mark is unchanged.
func TestUserDirMountsOffWorkloadPatchRequiresSystemAdmin(t *testing.T) {
	off := false
	f := newHostPathFixture(t)
	marked, err := f.createUserDirMounts(t, hpAdmin, nil, &off)
	require.NoError(t, err)
	plain, err := f.createUserDirMounts(t, hpManager, nil, nil)
	require.NoError(t, err)

	patch := func(userId string, workload *v1.Workload) error {
		requestUser := &v1.User{}
		require.NoError(t, f.cli.Get(t.Context(), client.ObjectKey{Name: userId}, requestUser))
		roles := f.h.accessController.GetRoles(t.Context(), requestUser)
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPatch, "/", nil)
		c.Set(common.UserId, userId)
		_, err := f.h.authWorkloadUpdate(c, workload, requestUser, roles, &view.PatchWorkloadRequest{})
		return err
	}
	err = patch(hpManager, marked)
	require.Error(t, err)
	require.True(t, apierrors.IsForbidden(err), "%v", err)
	require.NoError(t, patch(hpAdmin, marked))
	require.NoError(t, patch(hpManager, plain))
}
