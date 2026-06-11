/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
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

func newAddonTemplate(name, typ string) *v1.AddonTemplate {
	return &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       v1.AddonTemplateSpec{Type: v1.AddonTemplateType(typ), Version: "1.0"},
	}
}

func TestListAddonTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(
		newAddonTemplate("b-tpl", "monitor"),
		newAddonTemplate("a-tpl", "network"),
	)

	// List all (sorted by name).
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListAddonTemplate(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListAddonTemplateResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, "a-tpl", resp.Items[0].AddonTemplateId)

	// List with type filter.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?type=monitor", nil)
	c2.Set(common.UserId, user.Name)
	h.ListAddonTemplate(c2)
	assert.Equal(t, http.StatusOK, rsp2.Code)
	var resp2 view.ListAddonTemplateResponse
	assert.NoError(t, json.Unmarshal(rsp2.Body.Bytes(), &resp2))
	assert.Equal(t, 1, resp2.TotalCount)
}

func TestGetAddonTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(newAddonTemplate("tpl-1", "monitor"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "tpl-1")
	h.GetAddonTemplate(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Missing name -> bad request.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c2.Set(common.UserId, user.Name)
	h.GetAddonTemplate(c2)
	assert.NotEqual(t, http.StatusOK, rsp2.Code)
}

func TestGetAdminAddonTemplate(t *testing.T) {
	h, _ := newAdminHandlerWithObjects(newAddonTemplate("tpl-1", "monitor"))

	_, err := h.getAdminAddonTemplate(context.Background(), "")
	assert.Error(t, err)

	at, err := h.getAdminAddonTemplate(context.Background(), "tpl-1")
	assert.NoError(t, err)
	assert.Equal(t, "tpl-1", at.Name)

	_, err = h.getAdminAddonTemplate(context.Background(), "missing")
	assert.Error(t, err)
}
