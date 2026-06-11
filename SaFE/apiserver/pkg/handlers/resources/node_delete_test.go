/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
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

func TestExportNodeToCSV(t *testing.T) {
	nodes := &view.ListNodeResponse{
		Items: []view.NodeResponseItem{
			{
				NodeBriefResponseItem: view.NodeBriefResponseItem{
					NodeId:     "node-1",
					InternalIP: "10.0.0.1",
					Available:  true,
				},
				ClusterId:      "c1",
				Phase:          "Ready",
				AvailResources: view.ResourceList{"amd.com/gpu": 4, "cpu": 64},
				TotalResources: view.ResourceList{"amd.com/gpu": 8, "cpu": 128},
				IsControlPlane: false,
			},
		},
	}
	var buf bytes.Buffer
	err := ExportNodeToCSV(nodes, &buf)
	assert.NoError(t, err)
	out := buf.String()
	assert.Contains(t, out, "node-1")
	assert.Contains(t, out, "internalIP")
	assert.Contains(t, out, "4/8")
}

func TestDeleteNodeHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("node not found", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects()
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "missing")
		h.DeleteNode(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful delete of unbound node", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-free"}})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "node-free")
		h.DeleteNode(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})

	t.Run("node bound to cluster cannot be deleted without force", func(t *testing.T) {
		cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
		node := &v1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   "node-bound",
			Labels: map[string]string{v1.ClusterIdLabel: "c1"},
		}}
		h, user := newAdminHandlerWithObjects(cluster, node)
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "node-bound")
		h.DeleteNode(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})
}

func TestCreateNodeHandlerValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	// Missing required fields -> bad request via validateCreateNodeRequest.
	body, _ := json.Marshal(view.CreateNodeRequest{})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user.Name)
	h.CreateNode(c)
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}
