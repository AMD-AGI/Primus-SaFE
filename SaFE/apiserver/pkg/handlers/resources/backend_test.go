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
	"github.com/spf13/viper"
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

// The console is sent to auth.login_url in external mode, as loginUrl. builtin does not use that URL.
func TestGetEnvsExternalLoginURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	get := func() view.GetEnvResponse {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/envs", nil)
		(&Handler{}).GetEnvs(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
		var resp view.GetEnvResponse
		assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
		return resp
	}
	t.Cleanup(func() { viper.Set("auth.mode", nil); viper.Set("auth.login_url", nil) })

	viper.Set("auth.mode", "external")
	viper.Set("auth.login_url", "https://sign-in.example.com/start")
	got := get()
	assert.Equal(t, "external", got.AuthMode)
	assert.False(t, got.SSOEnable)
	assert.Equal(t, "local", got.Connection)
	assert.Equal(t, "https://sign-in.example.com/start", got.LoginURL)

	viper.Set("auth.mode", "builtin")
	got = get()
	assert.Equal(t, "builtin", got.AuthMode)
	assert.False(t, got.SSOEnable)
	assert.Empty(t, got.LoginURL)
	viper.Set("auth.mode", "external")

	viper.Set("auth.login_url", "")
	got = get()
	assert.False(t, got.SSOEnable)
	assert.Empty(t, got.LoginURL)
}

// The console creates API keys at the identity provider only where the deployment says the provider's route
// exists (auth.api_keys_url), and only in external mode. Unset auth.mode is builtin.
func TestGetEnvsAPIKeysURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	get := func() view.GetEnvResponse {
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/envs", nil)
		(&Handler{}).GetEnvs(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
		var resp view.GetEnvResponse
		assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
		return resp
	}
	t.Cleanup(func() { viper.Set("auth.mode", nil); viper.Set("auth.api_keys_url", nil) })

	for _, tt := range []struct {
		name, mode, url, want string
	}{
		{"external, routed", "external", "/api/v1/apikeys", "/api/v1/apikeys"},
		{"external, trailing slash", "external", "/api/v1/apikeys/", "/api/v1/apikeys"},
		{"external, nothing routed", "external", "", ""},
		{"auth.mode builtin does not report it", "builtin", "/api/v1/apikeys", ""},
		{"unset auth.mode does not report it", "", "/api/v1/apikeys", ""},
		{"external, an absolute URL is not a path on the console's origin", "external", "https://other.example.com/api/v1/apikeys", ""},
		{"external, a protocol relative URL", "external", "//other.example.com/api/v1/apikeys", ""},
		{"external, no leading slash", "external", "api/v1/apikeys", ""},
		{"external, a query or fragment", "external", "/api/v1/apikeys?x=1", ""},
		{"external, only a slash", "external", "/", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			viper.Set("auth.mode", tt.mode)
			viper.Set("auth.api_keys_url", tt.url)
			assert.Equal(t, tt.want, get().APIKeysURL)
		})
	}
}
