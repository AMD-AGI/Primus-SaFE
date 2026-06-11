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

func readyCluster(name string) *v1.Cluster {
	c := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: name}}
	c.Status.ControlPlaneStatus.Phase = v1.ReadyPhase
	return c
}

func TestProcessClusterNode(t *testing.T) {
	cluster := readyCluster("c1")
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	h, _ := newAdminHandlerWithObjects(cluster, node)

	// Add node to cluster.
	err := h.processClusterNode(context.Background(), cluster, "node-1", v1.NodeActionAdd)
	assert.NoError(t, err)

	// Node not found -> error.
	err = h.processClusterNode(context.Background(), cluster, "missing", v1.NodeActionAdd)
	assert.Error(t, err)
}

func TestRemoveNodesFromWorkspaceNoWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	h, user := newAdminHandlerWithObjects(node)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)

	// Node has no workspace -> no-op, no error.
	err := h.removeNodesFromWorkspace(c, []string{"node-1"}, false)
	assert.NoError(t, err)
}

func TestProcessClusterNodesHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("cluster not ready", func(t *testing.T) {
		cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c-notready"}}
		h, user := newAdminHandlerWithObjects(cluster)
		body, _ := json.Marshal(view.ProcessNodesRequest{NodeIds: []string{"node-1"}, Action: v1.NodeActionAdd})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c-notready")
		h.ProcessClusterNodes(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("successful add", func(t *testing.T) {
		cluster := readyCluster("c1")
		node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
		h, user := newAdminHandlerWithObjects(cluster, node)
		body, _ := json.Marshal(view.ProcessNodesRequest{NodeIds: []string{"node-1"}, Action: v1.NodeActionAdd})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		c.Set(common.Name, "c1")
		h.ProcessClusterNodes(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}
