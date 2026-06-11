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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// newAdminHandlerWithObjects builds a Handler backed by a fake client seeded
// with the admin user/role plus any extra objects.
func newAdminHandlerWithObjects(objs ...client.Object) (*Handler, *v1.User) {
	mockUser := genMockUser()
	mockRole := genMockRole()
	scheme := runtime.NewScheme()
	_ = v1.AddToScheme(scheme)

	all := append([]client.Object{mockUser, mockRole}, objs...)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(all...).Build()
	h := &Handler{
		Client:           fakeClient,
		accessController: authority.NewAccessController(fakeClient),
	}
	return h, mockUser
}

func TestGenerateNodeTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, "user-1")

	req := &view.CreateNodeTemplateRequest{Name: "My Template", AddOnTemplates: []string{"a", "b"}}
	nt := generateNodeTemplate(c, req)
	assert.Equal(t, "My Template", nt.Labels[v1.DisplayNameLabel])
	assert.Equal(t, "user-1", nt.Labels[v1.UserIdLabel])
	assert.Equal(t, []string{"a", "b"}, nt.Spec.AddOnTemplates)
	assert.NotEmpty(t, nt.Name)
}

func TestGetAdminNodeTemplate(t *testing.T) {
	h, _ := newAdminHandlerWithObjects(&v1.NodeTemplate{ObjectMeta: metav1.ObjectMeta{Name: "tpl-1"}})

	// Empty name -> bad request.
	_, err := h.getAdminNodeTemplate(context.Background(), "")
	assert.Error(t, err)

	// Existing template.
	nt, err := h.getAdminNodeTemplate(context.Background(), "tpl-1")
	assert.NoError(t, err)
	assert.Equal(t, "tpl-1", nt.Name)

	// Missing template -> error.
	_, err = h.getAdminNodeTemplate(context.Background(), "missing")
	assert.Error(t, err)
}

func TestCreateNodeTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body, _ := json.Marshal(view.CreateNodeTemplateRequest{Name: "tpl-new", AddOnTemplates: []string{"a"}})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)

	h.CreateNodeTemplate(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestListNodeTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.NodeTemplate{ObjectMeta: metav1.ObjectMeta{Name: "tpl-1"}})

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)

	h.ListNodeTemplate(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListNodeTemplateResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 1, resp.TotalCount)
}

func TestDeleteNodeTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.NodeTemplate{ObjectMeta: metav1.ObjectMeta{Name: "tpl-del"}})

	// Missing name -> bad request.
	rsp0 := httptest.NewRecorder()
	c0, _ := gin.CreateTestContext(rsp0)
	c0.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c0.Set(common.UserId, user.Name)
	h.DeleteNodeTemplate(c0)
	assert.NotEqual(t, http.StatusOK, rsp0.Code)

	// Successful delete.
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "tpl-del")
	h.DeleteNodeTemplate(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
