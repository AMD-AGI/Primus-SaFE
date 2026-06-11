/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestGetAdminNode(t *testing.T) {
	h, _ := newAdminHandlerWithObjects(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}})

	_, err := h.getAdminNode(context.Background(), "")
	assert.Error(t, err)

	n, err := h.getAdminNode(context.Background(), "node-1")
	assert.NoError(t, err)
	assert.Equal(t, "node-1", n.Name)

	_, err = h.getAdminNode(context.Background(), "missing")
	assert.Error(t, err)
}

func TestGetUsedResourceNoWorkspace(t *testing.T) {
	h, _ := newAdminHandlerWithObjects()
	// Node without a workspace label -> early return (nil, nil).
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}}
	res, err := h.getUsedResource(context.Background(), node)
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestBuildListNodeBriefResponse(t *testing.T) {
	nodes := []*v1.Node{
		{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}, Spec: v1.NodeSpec{PrivateIP: "10.0.0.2"}},
	}
	resp, err := buildListNodeBriefResponse(2, nodes)
	assert.NoError(t, err)
	briefResp, ok := resp.(*view.ListNodeBriefResponse)
	assert.True(t, ok)
	assert.Equal(t, 2, briefResp.TotalCount)
	assert.Len(t, briefResp.Items, 2)
}

func TestGetNodeHandlerNoWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects(&v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}})

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "node-1")
	h.GetNode(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetNodeFlavorAvailHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "flavor-1"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Product: "AMD_EPYC", Quantity: resource.MustParse("128")},
			Memory: resource.MustParse("256Gi"),
			Gpu:    &v1.GpuChip{Product: "MI300X", ResourceName: "amd.com/gpu", Quantity: resource.MustParse("8")},
		},
	}
	h, user := newAdminHandlerWithObjects(nf)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "flavor-1")
	h.GetNodeFlavorAvail(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetWorkspaceDisplayName(t *testing.T) {
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "ws-1",
			Labels: map[string]string{v1.DisplayNameLabel: "My Workspace"},
		},
	}
	h, _ := newAdminHandlerWithObjects(ws)

	name, err := h.getWorkspaceDisplayName(context.Background(), "ws-1")
	assert.NoError(t, err)
	assert.Equal(t, "My Workspace", name)

	_, err = h.getWorkspaceDisplayName(context.Background(), "missing")
	assert.Error(t, err)
}
