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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func newHelmAddon(name string) *v1.Addon {
	return &v1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: v1.AddonSpec{
			Cluster: &corev1.ObjectReference{Name: "c1"},
			AddonSource: v1.AddonSource{
				HelmRepository: &v1.HelmRepository{
					ReleaseName: "rel",
					Values:      "old-values",
					Template:    &corev1.ObjectReference{Name: "tpl-1"},
				},
			},
		},
	}
}

func TestGenerateAddon(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	tpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "tpl-1"},
		Spec: v1.AddonTemplateSpec{
			Type:                 v1.AddonTemplateHelm,
			URL:                  "oci://repo/chart",
			Version:              "1.0",
			HelmDefaultNamespace: "addon-ns",
			HelmDefaultValues:    "key: val",
		},
	}
	h, user := newAdminHandlerWithObjects(cluster, tpl)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")

	req := &view.CreateAddonRequestBody{AddonBody: view.AddonBody{ReleaseName: "rel", Template: "tpl-1"}}
	addon, err := h.generateAddon(c, req, []byte(`{}`))
	assert.NoError(t, err)
	assert.NotNil(t, addon.Spec.AddonSource.HelmRepository)
	assert.Equal(t, "addon-ns", addon.Spec.AddonSource.HelmRepository.Namespace)
	assert.Equal(t, "key: val", addon.Spec.AddonSource.HelmRepository.Values)

	// Missing template -> bad request.
	_, err = h.generateAddon(c, &view.CreateAddonRequestBody{}, []byte(`{}`))
	assert.Error(t, err)
}

func TestCreateAddonHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	tpl := &v1.AddonTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "tpl-1"},
		Spec:       v1.AddonTemplateSpec{Type: v1.AddonTemplateHelm, URL: "oci://r/c", Version: "1.0"},
	}
	h, user := newAdminHandlerWithObjects(cluster, tpl)

	body, _ := json.Marshal(view.CreateAddonRequestBody{AddonBody: view.AddonBody{ReleaseName: "rel", Template: "tpl-1", Namespace: "ns"}})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")
	h.CreateAddon(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestListAddonHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(newHelmAddon("b-addon"), newHelmAddon("a-addon"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListAddon(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListAddonResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, "a-addon", resp.Items[0].Name)
}

func TestGetAddonHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(newHelmAddon("addon-1"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Params = gin.Params{{Key: common.AddonName, Value: "addon-1"}}
	h.GetAddon(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetAdminAddon(t *testing.T) {
	h, _ := newAdminHandlerWithObjects(newHelmAddon("addon-1"))

	_, err := h.getAdminAddon(context.Background(), "")
	assert.Error(t, err)

	a, err := h.getAdminAddon(context.Background(), "addon-1")
	assert.NoError(t, err)
	assert.Equal(t, "addon-1", a.Name)

	_, err = h.getAdminAddon(context.Background(), "missing")
	assert.Error(t, err)
}

func TestDeleteAddonHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(newHelmAddon("addon-del"))

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Params = gin.Params{{Key: common.AddonName, Value: "addon-del"}}
	h.DeleteAddon(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestPatchAddonHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(newHelmAddon("addon-1"))

	desc := "new-desc"
	vals := "new: values"
	body, _ := json.Marshal(view.PatchAddonRequest{Description: &desc, Values: &vals})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Params = gin.Params{{Key: common.AddonName, Value: "addon-1"}}
	h.PatchAddon(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestUpdateAddon(t *testing.T) {
	h, _ := newAdminHandlerWithObjects()
	addon := newHelmAddon("addon-1")

	// No-op.
	assert.False(t, h.updateAddon(addon, &view.PatchAddonRequest{}))

	// Change values + template + description.
	desc := "d"
	tpl := "tpl-2"
	vals := "new-values"
	changed := h.updateAddon(addon, &view.PatchAddonRequest{Description: &desc, Template: &tpl, Values: &vals})
	assert.True(t, changed)
	assert.Equal(t, "new-values", addon.Spec.AddonSource.HelmRepository.Values)
	assert.Equal(t, "tpl-2", addon.Spec.AddonSource.HelmRepository.Template.Name)
}
