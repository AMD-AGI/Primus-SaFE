/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// hostFromServer strips the scheme so it can be passed as harborHost (handlers
// build the URL as http://{host}{path}).
func hostFromServer(ts *httptest.Server) string {
	return strings.TrimPrefix(ts.URL, "http://")
}

// TestHarborRequestSuccess verifies a GET request decodes JSON into the result.
func TestHarborRequestSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v2.0/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	}))
	defer ts.Close()

	h := &ImageHandler{}
	var health HarborHealth
	err := h.harborRequest(context.Background(), hostFromServer(ts), "/api/v2.0/health", "admin", "pw", &health)
	require.NoError(t, err)
	assert.Equal(t, "healthy", health.Status)
}

// TestHarborRequestNon200 verifies a non-200 response yields an error.
func TestHarborRequestNon200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	h := &ImageHandler{}
	var out map[string]any
	err := h.harborRequest(context.Background(), hostFromServer(ts), "/x", "", "", &out)
	assert.Error(t, err)
}

// TestGetHarborStats verifies health+statistics are fetched and combined.
func TestGetHarborStats(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/api/v2.0/health":
			_, _ = w.Write([]byte(`{"status":"healthy"}`))
		case "/api/v2.0/statistics":
			_, _ = w.Write([]byte(`{"private_repo_count":3,"public_repo_count":2}`))
		}
	}))
	defer ts.Close()

	h := &ImageHandler{}
	stats, err := h.getHarborStats(context.Background(), hostFromServer(ts), "admin", "pw")
	require.NoError(t, err)
	assert.Equal(t, "healthy", stats.Status)
	assert.Equal(t, 3, stats.PrivateRepoCount)
}

// TestHarborPostSuccess verifies a POST accepts a 201 status.
func TestHarborPostSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	h := &ImageHandler{}
	err := h.harborPost(context.Background(), hostFromServer(ts), "/api/v2.0/projects", "admin", "pw", map[string]any{"x": 1})
	assert.NoError(t, err)
}

// TestHarborPutUnexpectedStatus verifies an unexpected status yields an error.
func TestHarborPutUnexpectedStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	h := &ImageHandler{}
	err := h.harborPut(context.Background(), hostFromServer(ts), "/x", "admin", "pw", map[string]any{})
	assert.Error(t, err)
}

// TestEnsureHarborProjectExistsPublic verifies a public project needs no further calls.
func TestEnsureHarborProjectExistsPublic(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"name":"p","public":true,"project_id":1}`))
	}))
	defer ts.Close()

	h := &ImageHandler{}
	err := h.ensureHarborProject(context.Background(), hostFromServer(ts), "admin", "pw", "p")
	assert.NoError(t, err)
}

// TestEnsureHarborProjectCreatesWhenMissing verifies a 404 triggers project creation.
func TestEnsureHarborProjectCreatesWhenMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()

	h := &ImageHandler{}
	err := h.ensureHarborProject(context.Background(), hostFromServer(ts), "admin", "pw", "p")
	assert.NoError(t, err)
}

// TestGetHarborCredentialsSuccess verifies credentials are read from configmap+secret.
func TestGetHarborCredentialsSuccess(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "harbor", Name: "harbor-core"},
		Data:       map[string]string{"EXT_ENDPOINT": "https://harbor.example.com"},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "harbor", Name: "harbor-core"},
		Data:       map[string][]byte{"HARBOR_ADMIN_PASSWORD": []byte("secret-pw")},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, sec).Build()
	h := &ImageHandler{Client: cl}

	domain, endpoint, password, err := h.GetHarborCredentials(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "harbor.example.com", domain)
	assert.Equal(t, "harbor-core.harbor.svc.cluster.local", endpoint)
	assert.Equal(t, "secret-pw", password)
}

// TestGetHarborCredentialsNoConfigMap verifies a missing configmap returns empty values without error.
func TestGetHarborCredentialsNoConfigMap(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).Build()
	h := &ImageHandler{Client: cl}

	domain, _, _, err := h.GetHarborCredentials(context.Background())
	require.NoError(t, err)
	assert.Empty(t, domain)
}

// TestGetHarborCredentialsMissingSecretKey verifies a missing password key yields an error.
func TestGetHarborCredentialsMissingSecretKey(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "harbor", Name: "harbor-core"},
		Data:       map[string]string{"EXT_ENDPOINT": "http://harbor.example.com"},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "harbor", Name: "harbor-core"},
		Data:       map[string][]byte{"OTHER": []byte("x")},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, sec).Build()
	h := &ImageHandler{Client: cl}

	_, _, _, err := h.GetHarborCredentials(context.Background())
	assert.Error(t, err)
}

// TestHarborDeleteArtifactBadName verifies an unparseable image name yields an error
// before any network call is attempted.
func TestHarborDeleteArtifactBadName(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "harbor", Name: "harbor-core"},
		Data:       map[string]string{"EXT_ENDPOINT": "http://harbor.example.com"},
	}
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "harbor", Name: "harbor-core"},
		Data:       map[string][]byte{"HARBOR_ADMIN_PASSWORD": []byte("pw")},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(cm, sec).Build()
	h := &ImageHandler{Client: cl}

	// Missing tag -> parseHarborImageName fails.
	err := h.harborDeleteArtifact(context.Background(), "host/project/repo")
	assert.Error(t, err)
}
