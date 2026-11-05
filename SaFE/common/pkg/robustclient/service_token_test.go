/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// guardedServer answers like the Robust API's guard: 401 unless the request
// carries want in X-Robust-Service-Token. It records what each path received.
func guardedServer(t *testing.T, want string) (*httptest.Server, func(path string) (string, bool)) {
	t.Helper()
	var mu sync.Mutex
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.Method+" "+r.URL.Path] = r.Header.Get(ServiceTokenHeader)
		mu.Unlock()
		if r.Header.Get(ServiceTokenHeader) != want {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"a SaFE session or API key is required"}`))
			return
		}
		_, _ = w.Write([]byte(`{"meta":{"code":2000},"data":{"value":"ok"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func(path string) (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		v, ok := seen[path]
		return v, ok
	}
}

func clusterWithToken(t *testing.T, endpoint string, token func() string) *ClusterClient {
	t.Helper()
	cfg := DefaultClientConfig()
	cfg.ServiceToken = token
	c := NewClient(cfg)
	c.RegisterCluster("c1", endpoint)
	return c.ForCluster("c1")
}

// callEveryMethod exercises each request-building method of ClusterClient, so a
// method added without the token fails here rather than in production.
func callEveryMethod(cc *ClusterClient) map[string]error {
	ctx := context.Background()
	var out map[string]string
	errs := map[string]error{}
	errs["GET /get"] = cc.Get(ctx, "/get", nil, &out)
	errs["POST /post"] = cc.Post(ctx, "/post", map[string]string{"a": "b"}, &out)
	errs["DELETE /delete"] = cc.Delete(ctx, "/delete", nil)
	_, errs["POST /rawpost"] = cc.RawPost(ctx, "/rawpost", map[string]string{"a": "b"})
	_, errs["GET /rawget"] = cc.RawGet(ctx, "/rawget", nil)
	return errs
}

func TestEveryMethodSendsTheServiceToken(t *testing.T) {
	srv, seen := guardedServer(t, "tok-1")
	cc := clusterWithToken(t, srv.URL, func() string { return "tok-1" })

	for path, err := range callEveryMethod(cc) {
		assert.NoError(t, err, path)
		got, ok := seen(path)
		require.True(t, ok, "%s never reached the server", path)
		assert.Equal(t, "tok-1", got, path)
	}
}

func TestWithoutATokenTheGuardRefusesEveryMethod(t *testing.T) {
	srv, seen := guardedServer(t, "tok-1")
	for name, token := range map[string]func() string{
		"nil func":     nil,
		"empty string": func() string { return "" },
	} {
		cc := clusterWithToken(t, srv.URL, token)
		for path, err := range callEveryMethod(cc) {
			require.Error(t, err, "%s/%s", name, path)
			assert.Contains(t, err.Error(), "401", "%s/%s", name, path)
			got, ok := seen(path)
			require.True(t, ok)
			assert.Empty(t, got, "%s/%s sent a header without a token", name, path)
		}
	}
}

func TestTheTokenIsReadPerRequest(t *testing.T) {
	srv, _ := guardedServer(t, "rotated")
	current := "old"
	cc := clusterWithToken(t, srv.URL, func() string { return current })

	var out map[string]string
	require.Error(t, cc.Get(context.Background(), "/get", nil, &out))
	current = "rotated"
	require.NoError(t, cc.Get(context.Background(), "/get", nil, &out))
	assert.Equal(t, "ok", out["value"])
}

// TestDefaultConfigReadsTheMountedSecret drives the token from the file the
// chart mounts at robust.secret_path, the way the components run.
func TestDefaultConfigReadsTheMountedSecret(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, commonconfig.RobustServiceTokenKey), []byte("from-secret\n"), 0o600))
	viper.Set("robust.secret_path", dir)

	srv, seen := guardedServer(t, "from-secret")
	c := NewClient(DefaultClientConfig())
	c.RegisterCluster("c1", srv.URL)

	var out map[string]string
	require.NoError(t, c.ForCluster("c1").Get(context.Background(), "/get", nil, &out))
	got, _ := seen("GET /get")
	assert.Equal(t, "from-secret", got)

	// A cluster registered by discovery after start-up carries it too.
	c.RegisterCluster("c2", srv.URL)
	require.NoError(t, c.ForCluster("c2").Delete(context.Background(), "/later", nil))
	got, _ = seen("DELETE /later")
	assert.Equal(t, "from-secret", got)
}
