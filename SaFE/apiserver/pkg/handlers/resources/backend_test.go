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

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
)

// TestGetEnvs verifies the backend env handler returns the config-derived view.
func TestGetEnvs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	h := &Handler{}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/envs", nil)

	h.GetEnvs(c)
	assert.Equal(t, http.StatusOK, rsp.Code)

	var resp view.GetEnvResponse
	assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
	assert.Equal(t, map[string]string{
		"primussafe/kubespray:20200530": "1.32.5",
		"primussafe/kubespray:v2.31.0":  "1.35.4",
		"primussafe/kubespray:v2.29.1":  "1.33.7",
		"primussafe/kubespray:v2.30.0":  "1.34.3",
	}, resp.KubeSprayK8sVersions)
}
