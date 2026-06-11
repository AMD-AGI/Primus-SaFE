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

func TestGenerateCluster(t *testing.T) {
	h, user := newAdminHandlerWithObjects()
	req := &view.CreateClusterRequest{
		Name:           "cluster-1",
		Description:    "desc",
		Labels:         map[string]string{"team": "infra", v1.PrimusSafePrefix + "x": "skip"},
		IsProtected:    true,
		IsControlPlane: true,
	}
	cluster, err := h.generateCluster(context.Background(), user, req, []byte(`{}`))
	assert.NoError(t, err)
	assert.Equal(t, "cluster-1", cluster.Name)
	assert.Equal(t, "infra", cluster.Labels["team"])
	assert.True(t, v1.IsProtected(cluster))
	assert.True(t, v1.HasLabel(cluster, v1.ClusterControlPlaneLabel))
}

func TestCreateClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body, _ := json.Marshal(view.CreateClusterRequest{Name: "cluster-new"})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	h.CreateCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestListClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "b-cluster"}},
		&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "a-cluster"}},
	)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.ListClusterResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, 2, resp.TotalCount)
	assert.Equal(t, "a-cluster", resp.Items[0].ClusterId)
}

func TestGetClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}})

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")
	h.GetCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestDeleteClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("protected cluster cannot be deleted", func(t *testing.T) {
		protected := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
			Name:   "c-prot",
			Labels: map[string]string{v1.ProtectLabel: ""},
		}}
		h, user := newAdminHandlerWithObjects(protected)
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c-prot")
		h.DeleteCluster(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful delete (no running workloads)", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c-del"}})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c-del")
		h.DeleteCluster(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

func TestPatchClusterHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}})

	protected := true
	body, _ := json.Marshal(view.PatchClusterRequest{IsProtected: &protected})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")
	h.PatchCluster(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestApplyClusterPatch(t *testing.T) {
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "c1",
		Labels: map[string]string{"old": "v"},
	}}

	// No-op patch.
	changed, err := applyClusterPatch(cluster, &view.PatchClusterRequest{})
	assert.NoError(t, err)
	assert.False(t, changed)

	// Set protected + control plane + label changes.
	protected := true
	cp := true
	newLabels := map[string]string{"team": "infra"}
	changed, err = applyClusterPatch(cluster, &view.PatchClusterRequest{
		IsProtected:    &protected,
		IsControlPlane: &cp,
		Labels:         &newLabels,
	})
	assert.NoError(t, err)
	assert.True(t, changed)
	assert.True(t, v1.IsProtected(cluster))
	assert.Equal(t, "infra", cluster.Labels["team"])
	// Old custom label removed.
	_, ok := cluster.Labels["old"]
	assert.False(t, ok)
}
