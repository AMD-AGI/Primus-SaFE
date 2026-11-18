/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// fakePlatformIdentity serves GET /api/v1/apikeys/platform and records each request.
func fakePlatformIdentity(t *testing.T) *[]*http.Request {
	t.Helper()
	resetIdentityConfig(t)
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Clone(context.Background()))
		if r.URL.Path != "/api/v1/apikeys/platform" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"apiKey":"ak-platform"}`))
	}))
	t.Cleanup(srv.Close)
	viper.Set("auth.scim.identity_url", srv.URL)
	viper.Set("auth.authenticators", []map[string]any{{
		"type": "introspection", "client_id": "svc", "client_secret_file": writeFile(t, "client_secret", "secret\n"),
	}})
	return &seen
}

func TestFetchPlatformAPIKeySSOBearer(t *testing.T) {
	seen := fakePlatformIdentity(t)
	key, err := FetchPlatformAPIKey(context.Background(), "sso-access-token", "", "sso")
	require.NoError(t, err)
	require.Equal(t, "ak-platform", key)
	require.Len(t, *seen, 1)
	r := (*seen)[0]
	require.Equal(t, "Bearer sso-access-token", r.Header.Get("Authorization"))
	require.Equal(t, "sso", r.Header.Get("UserType"))
	require.Equal(t, "svc:secret", r.Header.Get("X-Internal-Token"))
}

func TestFetchPlatformAPIKeyCookie(t *testing.T) {
	seen := fakePlatformIdentity(t)
	key, err := FetchPlatformAPIKey(context.Background(), "", "cookie-token", "sso")
	require.NoError(t, err)
	require.Equal(t, "ak-platform", key)
	r := (*seen)[0]
	c, err := r.Cookie("Token")
	require.NoError(t, err)
	require.Equal(t, "cookie-token", c.Value)
	ut, err := r.Cookie("userType")
	require.NoError(t, err)
	require.Equal(t, "sso", ut.Value)
	require.Empty(t, r.Header.Get("Authorization"))
}

func TestFetchPlatformAPIKeyWithoutSession(t *testing.T) {
	_, err := FetchPlatformAPIKey(context.Background(), "", "", "")
	require.True(t, apierrors.IsUnauthorized(err))
}
