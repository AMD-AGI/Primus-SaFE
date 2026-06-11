/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestListOpsJobDBDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	h.ListOpsJob(c)
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestGetOpsJobDBDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "job-1")
	h.GetOpsJob(c)
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestStopOpsJobNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	// Empty name -> bad request.
	rsp0 := httptest.NewRecorder()
	c0, _ := gin.CreateTestContext(rsp0)
	c0.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c0.Set(common.UserId, user.Name)
	h.StopOpsJob(c0)
	assert.NotEqual(t, http.StatusOK, rsp0.Code)

	// Name set but job not in cluster -> not found.
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)
	c.Set(common.Name, "missing-job")
	h.StopOpsJob(c)
	assert.NotEqual(t, http.StatusOK, rsp.Code)
}

func TestGenDefaultOpsJob(t *testing.T) {
	user := genMockUser()
	req := &view.BaseOpsJobRequest{
		Name:              "my-job",
		Type:              v1.OpsJobRebootType,
		SecurityOperation: true,
	}
	job := genDefaultOpsJob(req, user)
	assert.Equal(t, "my-job", job.Labels[v1.DisplayNameLabel])
	assert.Equal(t, user.Name, job.Labels[v1.UserIdLabel])
	assert.Equal(t, v1.OpsJobRebootType, job.Spec.Type)
}

func TestGenerateRebootJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	c.Set(common.UserId, user.Name)

	body, _ := json.Marshal(view.BaseOpsJobRequest{Name: "reboot-job", Type: v1.OpsJobRebootType})
	job, err := h.generateRebootJob(c, body)
	assert.NoError(t, err)
	assert.NotNil(t, job)
	assert.Equal(t, v1.OpsJobRebootType, job.Spec.Type)
}

func TestDeleteAdminOpsJob(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, user := newAdminHandlerWithObjects()

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Set(common.UserId, user.Name)

	// Empty id -> bad request error.
	_, err := h.deleteAdminOpsJob(c, "")
	assert.Error(t, err)

	// Missing job -> not found is ignored, returns (false, nil).
	found, err := h.deleteAdminOpsJob(c, "missing")
	assert.NoError(t, err)
	assert.False(t, found)
}
