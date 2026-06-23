/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestGetAdminWorkspace(t *testing.T) {
	h, _ := newAdminHandlerWithObjects(&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-1"}})

	_, err := h.getAdminWorkspace(context.Background(), "")
	assert.Error(t, err)

	ws, err := h.getAdminWorkspace(context.Background(), "ws-1")
	assert.NoError(t, err)
	assert.Equal(t, "ws-1", ws.Name)

	_, err = h.getAdminWorkspace(context.Background(), "missing")
	assert.Error(t, err)
}

func TestGenerateWorkspace(t *testing.T) {
	h, user := newAdminHandlerWithObjects()
	req := &view.CreateWorkspaceRequest{
		Name:        "myws",
		ClusterId:   "c1",
		Description: "desc",
		QueuePolicy: "FIFO",
	}
	ws, err := h.generateWorkspace(context.Background(), user, req)
	assert.NoError(t, err)
	assert.Contains(t, ws.Name, "c1")
	assert.Equal(t, "c1", ws.Spec.Cluster)
	// Default scopes applied.
	assert.NotEmpty(t, ws.Spec.Scopes)
}

func TestCreateWorkspaceHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body, _ := json.Marshal(view.CreateWorkspaceRequest{Name: "myws", ClusterId: "c1"})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	h.CreateWorkspace(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestListWorkspaceHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-b"}},
		&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-a"}},
	)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListWorkspace(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListWorkspaceResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalCount)
}

func TestDeleteWorkspaceHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-del"}})

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "ws-del")
	h.DeleteWorkspace(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestPatchWorkspaceHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-1"}})

	desc := "updated"
	replica := 5
	body, _ := json.Marshal(view.PatchWorkspaceRequest{Description: &desc, Replica: &replica})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "ws-1")
	h.PatchWorkspace(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestApplyWorkspacePatch(t *testing.T) {
	h, user := newAdminHandlerWithObjects()
	ws := &v1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: "ws-1"}}

	desc := "d"
	replica := 3
	preempt := true
	isDefault := true
	err := h.applyWorkspacePatch(context.Background(), ws, user, &view.PatchWorkspaceRequest{
		Description:   &desc,
		Replica:       &replica,
		EnablePreempt: &preempt,
		IsDefault:     &isDefault,
	})
	assert.NoError(t, err)
	assert.Equal(t, 3, ws.Spec.Replica)
	assert.True(t, ws.Spec.EnablePreempt)
	assert.True(t, ws.Spec.IsDefault)
}

func TestParseListWorkspaceQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/?clusterId=c1", nil)
	q, err := parseListWorkspaceQuery(c)
	assert.NoError(t, err)
	assert.Equal(t, "c1", q.ClusterId)
}

func TestBuildListWorkspaceSelector(t *testing.T) {
	sel, err := buildListWorkspaceSelector(&view.ListWorkspaceRequest{})
	assert.NoError(t, err)
	assert.True(t, sel.Empty())

	sel2, err := buildListWorkspaceSelector(&view.ListWorkspaceRequest{ClusterId: "c1"})
	assert.NoError(t, err)
	assert.False(t, sel2.Empty())
}
