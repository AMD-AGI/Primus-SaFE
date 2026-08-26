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
	"time"

	"github.com/gin-gonic/gin"
	testifyassert "github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestCvtToAddonResponseBody tests the conversion from v1.Addon to types.AddonResponseBody
func TestCvtToAddonResponseBody(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		addon    *v1.Addon
		validate func(*testing.T, interface{})
	}{
		{
			name: "basic addon without status",
			addon: &v1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-cluster-default-prometheus",
					CreationTimestamp: metav1.NewTime(now),
					Annotations: map[string]string{
						v1.DescriptionAnnotation: "Prometheus monitoring addon",
					},
				},
				Spec: v1.AddonSpec{
					Cluster: &corev1.ObjectReference{
						APIVersion: "amd.com/v1",
						Kind:       "Cluster",
						Name:       "test-cluster",
					},
					AddonSource: v1.AddonSource{
						HelmRepository: &v1.HelmRepository{
							ReleaseName:  "prometheus",
							URL:          "https://prometheus-community.github.io/helm-charts",
							ChartVersion: "15.0.0",
							Namespace:    "monitoring",
							Values:       "replicas: 2",
							Template: &corev1.ObjectReference{
								APIVersion: "amd.com/v1",
								Kind:       v1.AddOnTemplateKind,
								Name:       "prometheus-template",
							},
						},
					},
				},
				Status: v1.AddonStatus{},
			},
			validate: func(t *testing.T, result interface{}) {
				resp := result.(view.AddonResponseBody)
				assert.Equal(t, "test-cluster-default-prometheus", resp.Name)
				assert.Equal(t, "test-cluster", resp.Cluster)
				assert.Equal(t, "prometheus", resp.ReleaseName)
				assert.Equal(t, "prometheus-template", resp.Template)
				assert.Equal(t, "monitoring", resp.Namespace)
				assert.Equal(t, "replicas: 2", resp.Values)
				assert.Equal(t, "Prometheus monitoring addon", resp.Description)
				assert.Equal(t, v1.AddonCreating, resp.Phase) // Empty phase defaults to Creating
				assert.NotEmpty(t, resp.CreationTime)
			},
		},
		{
			name: "addon with deployed status",
			addon: &v1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "prod-cluster-kube-system-metrics",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1.AddonSpec{
					Cluster: &corev1.ObjectReference{
						Name: "prod-cluster",
					},
					AddonSource: v1.AddonSource{
						HelmRepository: &v1.HelmRepository{
							ReleaseName: "metrics-server",
							Namespace:   "kube-system",
						},
					},
				},
				Status: v1.AddonStatus{
					Phase: v1.AddonDeployed,
					AddonSourceStatus: v1.AddonSourceStatus{
						HelmRepositoryStatus: &v1.HelmRepositoryStatus{
							FirstDeployed: metav1.NewTime(now.Add(-1 * time.Hour)),
							LastDeployed:  metav1.NewTime(now),
							Description:   "Deployed successfully",
							Status:        "deployed",
							Version:       1,
							ChartVersion:  "3.8.0",
						},
					},
				},
			},
			validate: func(t *testing.T, result interface{}) {
				resp := result.(view.AddonResponseBody)
				assert.Equal(t, "prod-cluster-kube-system-metrics", resp.Name)
				assert.Equal(t, v1.AddonPhaseType(v1.AddonDeployed), resp.Phase)
				assert.Equal(t, "deployed", resp.Status.Status)
				assert.Equal(t, 1, resp.Status.Version)
				assert.Equal(t, "3.8.0", resp.Status.ChartVersion)
				assert.Equal(t, "Deployed successfully", resp.Status.Description)
			},
		},
		{
			name: "addon with empty helm repository",
			addon: &v1.Addon{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "simple-addon",
					CreationTimestamp: metav1.NewTime(now),
				},
				Spec: v1.AddonSpec{
					Cluster: &corev1.ObjectReference{
						Name: "test",
					},
				},
				Status: v1.AddonStatus{
					Phase: v1.AddonFailed,
				},
			},
			validate: func(t *testing.T, result interface{}) {
				resp := result.(view.AddonResponseBody)
				assert.Equal(t, "simple-addon", resp.Name)
				assert.Equal(t, v1.AddonPhaseType(v1.AddonFailed), resp.Phase)
				assert.Empty(t, resp.ReleaseName)
				assert.Empty(t, resp.Template)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToAddonResponseBody(tt.addon)
			tt.validate(t, result)
		})
	}
}

// TestGenAddonName tests addon name generation
func TestGenAddonName(t *testing.T) {
	tests := []struct {
		name        string
		cluster     string
		namespace   string
		releaseName string
		expected    string
	}{
		{
			name:        "with namespace",
			cluster:     "prod",
			namespace:   "monitoring",
			releaseName: "prometheus",
			expected:    "prod-monitoring-prometheus",
		},
		{
			name:        "empty namespace defaults to default",
			cluster:     "test",
			namespace:   "",
			releaseName: "grafana",
			expected:    "test-default-grafana",
		},
		{
			name:        "complex names",
			cluster:     "cluster-01",
			namespace:   "kube-system",
			releaseName: "metrics-server-ha",
			expected:    "cluster-01-kube-system-metrics-server-ha",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := genAddonName(tt.cluster, tt.namespace, tt.releaseName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- merged from addon_handlers_test.go ---

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
	testifyassert.NoError(t, err)
	testifyassert.NotNil(t, addon.Spec.AddonSource.HelmRepository)
	assert.Equal(t, "addon-ns", addon.Spec.AddonSource.HelmRepository.Namespace)
	assert.Equal(t, "key: val", addon.Spec.AddonSource.HelmRepository.Values)

	// Missing template -> bad request.
	_, err = h.generateAddon(c, &view.CreateAddonRequestBody{}, []byte(`{}`))
	testifyassert.Error(t, err)
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
	testifyassert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
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
	testifyassert.Error(t, err)

	a, err := h.getAdminAddon(context.Background(), "addon-1")
	testifyassert.NoError(t, err)
	assert.Equal(t, "addon-1", a.Name)

	_, err = h.getAdminAddon(context.Background(), "missing")
	testifyassert.Error(t, err)
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
	testifyassert.False(t, h.updateAddon(addon, &view.PatchAddonRequest{}))

	// Change values + template + description.
	desc := "d"
	tpl := "tpl-2"
	vals := "new-values"
	changed := h.updateAddon(addon, &view.PatchAddonRequest{Description: &desc, Template: &tpl, Values: &vals})
	testifyassert.True(t, changed)
	assert.Equal(t, "new-values", addon.Spec.AddonSource.HelmRepository.Values)
	assert.Equal(t, "tpl-2", addon.Spec.AddonSource.HelmRepository.Template.Name)
}
