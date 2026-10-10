/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package exportimage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// refusingRegistry answers every write with the registry's own error, the way a registry
// refuses an immutable tag or a full quota.
func refusingRegistry(t *testing.T) (*registryClient, name.Repository) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":[{"code":"DENIED","message":"the tag is immutable"}]}`))
	}))
	t.Cleanup(srv.Close)
	repo, err := name.NewRepository(strings.TrimPrefix(srv.URL, "http://")+"/custom/img", name.Insecure)
	require.NoError(t, err)
	return &registryClient{http: srv.Client()}, repo
}

func TestPutManifestReportsTheRegistrysError(t *testing.T) {
	c, repo := refusingRegistry(t)
	_, err := c.PutManifest(context.Background(), repo.Tag("v1"), types.OCIManifestSchema1, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the tag is immutable")
}

func TestUploadReportsTheRegistrysError(t *testing.T) {
	c, repo := refusingRegistry(t)
	d, err := v1.NewHash("sha256:" + strings.Repeat("a", 64))
	require.NoError(t, err)
	err = c.Upload(context.Background(), repo, d, []byte(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "the tag is immutable")
}
