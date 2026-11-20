/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// logoutThroughJar posts to the apiserver's logout from a browser cookie jar that holds the identity
// provider's session (the cookies its login sets: path /, host-only), and returns the Token the jar
// would send to the provider's logout that follows.
func logoutThroughJar(t *testing.T) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/logout", (&Handler{}).Logout)
	server := httptest.NewServer(router)
	defer server.Close()

	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	origin, err := url.Parse(server.URL + "/")
	require.NoError(t, err)
	jar.SetCookies(origin, []*http.Cookie{
		{Name: authority.CookieToken, Value: "provider-session", Path: "/", HttpOnly: true},
		{Name: common.UserId, Value: "u-1", Path: "/", HttpOnly: true},
		{Name: common.UserType, Value: "sso", Path: "/", HttpOnly: true},
	})
	rsp, err := (&http.Client{Jar: jar}).Post(server.URL+"/api/v1/logout", "application/json", nil)
	require.NoError(t, err)
	rsp.Body.Close()
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	providerLogout, err := url.Parse(server.URL + "/identity/api/v1/logout")
	require.NoError(t, err)
	for _, cookie := range jar.Cookies(providerLogout) {
		if cookie.Name == authority.CookieToken {
			return cookie.Value
		}
	}
	return ""
}

// In external mode the session cookie is the identity provider's, so the apiserver's logout leaves it to
// the provider's logout, which needs it to find and revoke the session.
func TestLogoutKeepsTheProviderSessionInExternalMode(t *testing.T) {
	t.Cleanup(func() { viper.Set("auth.mode", nil) })
	viper.Set("auth.mode", "external")
	assert.Equal(t, "provider-session", logoutThroughJar(t))
}

// With local accounts the cookie is the apiserver's own, and its logout clears it.
func TestLogoutClearsItsOwnCookieInBuiltinMode(t *testing.T) {
	t.Cleanup(func() { viper.Set("auth.mode", nil) })
	viper.Set("auth.mode", "builtin")
	assert.Equal(t, "", logoutThroughJar(t))
}
