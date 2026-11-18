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
		_, _ = w.Write([]byte(`{"userId":"u1","apiKey":"ak-platform"}`))
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
	key, err := FetchPlatformAPIKey(context.Background(), "u1", "sso-access-token", "", "sso")
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
	key, err := FetchPlatformAPIKey(context.Background(), "u1", "", "cookie-token", "sso")
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
	_, err := FetchPlatformAPIKey(context.Background(), "u1", "", "", "")
	require.True(t, apierrors.IsUnauthorized(err))
}

func TestFetchPlatformAPIKeyPrefersBearerAndRefusesAnotherUser(t *testing.T) {
	seen := fakePlatformIdentity(t)
	_, err := FetchPlatformAPIKey(context.Background(), "u1", "bearer-token", "cookie-token", "sso")
	require.NoError(t, err)
	r := (*seen)[0]
	require.Equal(t, "Bearer bearer-token", r.Header.Get("Authorization"))
	require.Empty(t, r.Header.Get("Cookie"))

	_, err = FetchPlatformAPIKey(context.Background(), "u2", "bearer-token", "", "")
	require.Error(t, err)
}

// platformIdentityAnswering serves every platform key request with status and body.
func platformIdentityAnswering(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	resetIdentityConfig(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	viper.Set("auth.scim.identity_url", srv.URL)
	viper.Set("auth.authenticators", []map[string]any{{
		"type": "introspection", "client_id": "svc", "client_secret_file": writeFile(t, "client_secret", "secret\n"),
	}})
	return srv
}

func TestFetchPlatformAPIKeyRefusesBadAnswers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		code   int32
	}{
		{"unauthorized", http.StatusUnauthorized, `{}`, http.StatusBadGateway},
		{"no primary key", http.StatusNotFound, `{}`, http.StatusBadGateway},
		{"identity error", http.StatusInternalServerError, `{}`, http.StatusServiceUnavailable},
		{"invalid body", http.StatusOK, `not json`, http.StatusBadGateway},
		{"empty key", http.StatusOK, `{"userId":"u1","apiKey":" "}`, http.StatusBadGateway},
	} {
		t.Run(tc.name, func(t *testing.T) {
			platformIdentityAnswering(t, tc.status, tc.body)
			_, err := FetchPlatformAPIKey(context.Background(), "u1", "sso-access-token", "", "")
			var status apierrors.APIStatus
			require.ErrorAs(t, err, &status)
			require.Equal(t, tc.code, status.Status().Code, err.Error())
		})
	}
}

func TestFetchPlatformAPIKeyUnreachableIdentity(t *testing.T) {
	srv := platformIdentityAnswering(t, http.StatusOK, `{}`)
	srv.Close()
	_, err := FetchPlatformAPIKey(context.Background(), "u1", "sso-access-token", "", "")
	require.Error(t, err)
}

func TestFetchPlatformAPIKeyWithoutIdentityConfig(t *testing.T) {
	resetIdentityConfig(t)
	_, err := FetchPlatformAPIKey(context.Background(), "u1", "sso-access-token", "", "")
	var cfg *ConfigError
	require.ErrorAs(t, err, &cfg)

	viper.Set("auth.scim.identity_url", "https://identity.example.com")
	_, err = FetchPlatformAPIKey(context.Background(), "u1", "sso-access-token", "", "")
	require.Error(t, err, "no introspection client secret")
}
