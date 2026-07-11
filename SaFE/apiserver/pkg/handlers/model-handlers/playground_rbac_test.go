/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newRunningWorkload builds a running inference workload in the given workspace.
func newRunningWorkload(name, workspace string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name}}
	w.Spec.Workspace = workspace
	w.Status.Phase = v1.WorkloadRunning
	return w
}

// newPlaygroundRBACHandler seeds remote_api models and running workloads across
// workspaces so the playground read paths can be exercised against read
// visibility. The DB client is nil, so all lookups take the K8s path.
func newPlaygroundRBACHandler(t *testing.T) *Handler {
	t.Helper()
	k8s := ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).WithObjects(
		newReadModel("rm-pub", "owner-1", ""),          // public remote_api model
		newReadModel("rm-other", "stranger-1", "ws-2"), // private remote_api model in ws-2
		newRunningWorkload("wl-ws1", "ws-1"),
		newRunningWorkload("wl-ws2", "ws-2"),
	).Build()
	return &Handler{k8sClient: k8s, accessController: newReadRBACAC(t)}
}

// TestListPlaygroundServicesReadRBAC verifies the playground service list only
// surfaces remote_api models the caller may see and workloads whose workspace
// the caller may access.
func TestListPlaygroundServicesReadRBAC(t *testing.T) {
	h := newPlaygroundRBACHandler(t)
	listIDs := func(user string) map[string]bool {
		res, err := h.listPlaygroundServices(readRBACCtx(user, "", nil))
		assert.NoError(t, err)
		resp, ok := res.(*ListPlaygroundServicesResponse)
		assert.True(t, ok)
		ids := make(map[string]bool, len(resp.Items))
		for _, it := range resp.Items {
			ids[it.ID] = true
		}
		return ids
	}

	// member-1 is a member of ws-1: sees the public model + ws-1 workload only.
	member := listIDs("member-1")
	assert.True(t, member["rm-pub"], "public model must be visible")
	assert.True(t, member["wl-ws1"], "ws-1 workload must be visible to ws-1 member")
	assert.False(t, member["rm-other"], "must not see private ws-2 model")
	assert.False(t, member["wl-ws2"], "must not see ws-2 workload")

	// admin sees every model and workload.
	admin := listIDs("admin-1")
	assert.True(t, admin["rm-pub"])
	assert.True(t, admin["rm-other"])
	assert.True(t, admin["wl-ws1"])
	assert.True(t, admin["wl-ws2"])
}

// TestGetChatURLReadRBAC verifies getChatURL enforces model read visibility
// before returning the model endpoint / API-key presence.
func TestGetChatURLReadRBAC(t *testing.T) {
	h := newPlaygroundRBACHandler(t)

	// member-1 cannot see the private ws-2 model.
	_, err := h.getChatURL(readRBACCtx("member-1", "", gin.Params{{Key: "id", Value: "rm-other"}}))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")

	// The owner (stranger-1) can.
	_, err = h.getChatURL(readRBACCtx("stranger-1", "", gin.Params{{Key: "id", Value: "rm-other"}}))
	assert.NoError(t, err)
}

// newChatCtx builds a POST context with a JSON body and an optional user id.
func newChatCtx(userID, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if userID != "" {
		c.Set(common.UserId, userID)
	}
	return c, w
}

// TestChatReadRBAC verifies Chat denies callers who cannot see the target model
// (before its endpoint / API key is used) or who cannot access the target
// workload's workspace.
func TestChatReadRBAC(t *testing.T) {
	h := newPlaygroundRBACHandler(t)

	// Private remote_api model: a non-member is denied with 403.
	c, w := newChatCtx("member-1", "{\"serviceId\":\"rm-other\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}")
	h.Chat(c)
	assert.Equal(t, http.StatusForbidden, w.Code)

	// Workload in ws-2: a non-member is denied with 403.
	c2, w2 := newChatCtx("member-1", "{\"serviceId\":\"wl-ws2\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}")
	h.Chat(c2)
	assert.Equal(t, http.StatusForbidden, w2.Code)

	// Admin passes the RBAC gate (not a 403); the empty model URL then yields a
	// 400, confirming authorization succeeded without any network call.
	c3, w3 := newChatCtx("admin-1", "{\"serviceId\":\"rm-other\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}]}")
	h.Chat(c3)
	assert.NotEqual(t, http.StatusForbidden, w3.Code)
}
