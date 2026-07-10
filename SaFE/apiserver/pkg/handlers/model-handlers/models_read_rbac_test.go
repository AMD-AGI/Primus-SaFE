/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newReadRBACAC seeds users used to exercise read-path visibility:
//   - admin-1     : system administrator (sees everything)
//   - readonly-1  : system-admin-readonly (sees everything)
//   - owner-1     : plain user, owns some models, member of no workspace
//   - member-1    : plain user, member of workspace "ws-1"
//   - stranger-1  : plain user, owns some models, member of no workspace
//
// canViewModel does not consult role rules, so no Role objects are required.
func newReadRBACAC(t *testing.T) *authority.AccessController {
	t.Helper()
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	readonly := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "readonly-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminReadonlyRole}},
	}
	owner := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "owner-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType},
	}
	member := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "member-1"},
		Spec: v1.UserSpec{
			Type:      v1.DefaultUserType,
			Resources: map[string][]string{common.UserWorkspaces: {"ws-1"}},
		},
	}
	stranger := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "stranger-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType},
	}
	return &authority.AccessController{
		Client: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).
			WithObjects(admin, readonly, owner, member, stranger).Build(),
	}
}

// newReadModel builds a Ready model owned by owner in the given workspace. An
// empty workspace marks the model public (visible to everyone).
func newReadModel(name, owner, workspace string) *v1.Model {
	m := &v1.Model{ObjectMeta: metav1.ObjectMeta{Name: name}}
	m.Labels = map[string]string{v1.UserIdLabel: owner}
	m.Spec.Workspace = workspace
	m.Spec.Source.AccessMode = v1.AccessModeRemoteAPI
	m.Status.Phase = v1.ModelPhaseReady
	return m
}

func readRBACCtx(userID, rawQuery string, params gin.Params) *gin.Context {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	c.Params = params
	if userID != "" {
		c.Set(common.UserId, userID)
	}
	return c
}

func newReadRBACHandler(t *testing.T) *Handler {
	t.Helper()
	// dbClient is intentionally nil so listModels/getModel take the K8s path,
	// which lets these tests control owner/workspace precisely.
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(
		newReadModel("m-pub", "owner-1", ""),          // public
		newReadModel("m-own", "owner-1", "ws-1"),      // owned by owner-1, in ws-1
		newReadModel("m-wsonly", "nobody", "ws-1"),    // ws-1, owned by a non-participant
		newReadModel("m-other", "stranger-1", "ws-2"), // ws-2, owned by stranger-1
	).Build()
	return &Handler{k8sClient: k8s, accessController: newReadRBACAC(t)}
}

// TestGetModelReadRBAC verifies #9: getModel enforces resource-level read
// visibility and returns 403 for models the caller may not see.
func TestGetModelReadRBAC(t *testing.T) {
	h := newReadRBACHandler(t)
	cases := []struct {
		name    string
		user    string
		modelID string
		denied  bool
	}{
		{"public visible to stranger", "stranger-1", "m-pub", false},
		{"owner sees own private", "owner-1", "m-own", false},
		{"workspace member sees ws model", "member-1", "m-wsonly", false},
		{"owner sees own regardless of ws", "stranger-1", "m-other", false},
		{"stranger denied others private", "stranger-1", "m-own", true},
		{"member denied other workspace", "member-1", "m-other", true},
		{"admin sees any private", "admin-1", "m-other", false},
		{"readonly admin sees any private", "readonly-1", "m-own", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.getModel(readRBACCtx(tc.user, "", gin.Params{{Key: "id", Value: tc.modelID}}))
			if tc.denied {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "not allowed")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestListModelsReadRBAC verifies #9: listModels only returns models visible to
// the caller (public + owned + member workspaces; admins see all).
func TestListModelsReadRBAC(t *testing.T) {
	h := newReadRBACHandler(t)
	listIDs := func(user string) map[string]bool {
		res, err := h.listModels(readRBACCtx(user, "limit=100&offset=0", nil))
		assert.NoError(t, err)
		resp, ok := res.(*ListModelResponse)
		assert.True(t, ok)
		ids := make(map[string]bool, len(resp.Items))
		for _, it := range resp.Items {
			ids[it.ID] = true
		}
		return ids
	}

	cases := []struct {
		name string
		user string
		want []string
	}{
		{"stranger sees public + owned", "stranger-1", []string{"m-pub", "m-other"}},
		{"owner sees public + owned", "owner-1", []string{"m-pub", "m-own"}},
		{"member sees public + ws-1 models", "member-1", []string{"m-pub", "m-own", "m-wsonly"}},
		{"admin sees all", "admin-1", []string{"m-pub", "m-own", "m-wsonly", "m-other"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := listIDs(tc.user)
			assert.Equal(t, len(tc.want), len(got), "unexpected visible set: %v", got)
			for _, id := range tc.want {
				assert.True(t, got[id], "expected %s visible to %s", id, tc.user)
			}
		})
	}
}

// TestListModelsReadRBACUnresolvedUserPublicOnly verifies fail-closed behavior:
// when the requesting user cannot be resolved (empty user id), only public
// models are returned.
func TestListModelsReadRBACUnresolvedUserPublicOnly(t *testing.T) {
	h := newReadRBACHandler(t)
	res, err := h.listModels(readRBACCtx("", "limit=100&offset=0", nil))
	assert.NoError(t, err)
	resp := res.(*ListModelResponse)
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, "m-pub", resp.Items[0].ID)
}
