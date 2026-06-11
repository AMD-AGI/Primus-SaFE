/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestParseGetPodLogQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Defaults applied.
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	q, err := parseGetPodLogQuery(c, "main")
	assert.NoError(t, err)
	assert.Equal(t, "main", q.Container)
	assert.Greater(t, q.TailLines, int64(0))

	// Explicit container/tailLines from query.
	rsp2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rsp2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/?container=side&tailLines=50", nil)
	q2, err := parseGetPodLogQuery(c2, "main")
	assert.NoError(t, err)
	assert.Equal(t, "side", q2.Container)
	assert.Equal(t, int64(50), q2.TailLines)
}

func TestGetClusterPodLog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "kubespray-pod", Namespace: common.PrimusSafeNamespace,
		Labels:            map[string]string{v1.ClusterManageClusterLabel: "c1"},
		CreationTimestamp: metav1.Now(),
	}}
	h, user := newFullHandler([]client.Object{cluster}, pod)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")
	h.GetClusterPodLog(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestGetClusterPodLogNoPod(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cluster := &v1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "c1"}}
	h, user := newFullHandler([]client.Object{cluster})

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "c1")
	h.GetClusterPodLog(c)
	// No pod found -> not-implemented error.
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestGetNodePodLog(t *testing.T) {
	gin.SetMode(gin.TestMode)

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "node-1",
			Labels: map[string]string{v1.ClusterIdLabel: "c1"},
		},
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "node-agent-pod", Namespace: common.PrimusSafeNamespace,
		Labels: map[string]string{
			v1.ClusterManageClusterLabel: "c1",
			v1.ClusterManageNodeLabel:    "node-1",
		},
		CreationTimestamp: metav1.Now(),
	}}
	h, user := newFullHandler([]client.Object{node}, pod)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "node-1")
	h.GetNodePodLog(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
