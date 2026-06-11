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

func newOpsJobCtx(user string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(common.UserId, user)
	return c, rsp
}

func TestGenerateExportImageJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	wl := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{Name: "wl-1"},
		Spec:       v1.WorkloadSpec{Workspace: "ws-1", Images: []string{"repo/img:tag"}},
	}
	h, user := newAdminHandlerWithObjects(wl)

	body := `{"name":"export","type":"exportImage","inputs":[{"name":"workload","value":"wl-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generateExportImageJob(c, []byte(body))
	assert.NoError(t, err)
	assert.Equal(t, "wl-1", job.Labels[v1.WorkloadIdLabel])

	// Missing workload id in inputs -> bad request.
	body2 := `{"name":"export","type":"exportImage","inputs":[]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generateExportImageJob(c2, []byte(body2))
	assert.Error(t, err)
}

func TestGeneratePrewarmImageJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-1"},
		Spec:       v1.WorkspaceSpec{Cluster: "c1"},
	}
	h, user := newAdminHandlerWithObjects(ws)

	body := `{"name":"prewarm","type":"prewarm","inputs":[{"name":"image","value":"repo/img:tag"},{"name":"workspace","value":"ws-1"}]}`
	c, _ := newOpsJobCtx(user.Name, body)
	job, err := h.generatePrewarmImageJob(c, []byte(body))
	assert.NoError(t, err)
	assert.Equal(t, "ws-1", job.Labels[v1.WorkspaceIdLabel])
	assert.Equal(t, "c1", job.Labels[v1.ClusterIdLabel])

	// Missing image -> bad request.
	body2 := `{"name":"prewarm","type":"prewarm","inputs":[{"name":"workspace","value":"ws-1"}]}`
	c2, _ := newOpsJobCtx(user.Name, body2)
	_, err = h.generatePrewarmImageJob(c2, []byte(body2))
	assert.Error(t, err)
}

func TestCreateOpsJobRebootHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	body, _ := json.Marshal(view.BaseOpsJobRequest{
		Name:   "reboot-job",
		Type:   v1.OpsJobRebootType,
		Inputs: []v1.Parameter{{Name: "node", Value: "node-1"}},
	})
	c, rsp := newOpsJobCtx(user.Name, string(body))
	h.CreateOpsJob(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	// Unsupported type -> error.
	body2, _ := json.Marshal(view.BaseOpsJobRequest{
		Name:   "weird-job",
		Type:   v1.OpsJobType("weird"),
		Inputs: []v1.Parameter{{Name: "node", Value: "node-1"}},
	})
	c2, rsp2 := newOpsJobCtx(user.Name, string(body2))
	h.CreateOpsJob(c2)
	assert.NotEqual(t, http.StatusOK, rsp2.Code)
}

func TestDeleteOpsJobHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	opsJob := &v1.OpsJob{ObjectMeta: metav1.ObjectMeta{Name: "job-del"}}
	h, user := newAdminHandlerWithObjects(opsJob)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "job-del")
	h.DeleteOpsJob(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
