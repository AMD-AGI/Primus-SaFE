/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

// stubHTTPClient is a minimal httpclient.Interface used to drive token fetching.
type stubHTTPClient struct {
	result *httpclient.Result
	err    error
}

func (s *stubHTTPClient) Get(string, ...string) (*httpclient.Result, error) { return s.result, s.err }
func (s *stubHTTPClient) Post(string, interface{}, ...string) (*httpclient.Result, error) {
	return s.result, s.err
}
func (s *stubHTTPClient) Put(string, interface{}, ...string) (*httpclient.Result, error) {
	return s.result, s.err
}
func (s *stubHTTPClient) Delete(string, ...string) (*httpclient.Result, error) {
	return s.result, s.err
}
func (s *stubHTTPClient) Do(*http.Request) (*httpclient.Result, error) { return s.result, s.err }
func (s *stubHTTPClient) GetBaseClient() *http.Client                  { return nil }

// TestDecryptRegistryAuthEmpty verifies empty credentials yield empty auth.
func TestDecryptRegistryAuthEmpty(t *testing.T) {
	h := &ImageHandler{}
	auth, err := h.decryptRegistryAuth(&model.RegistryInfo{})
	require.NoError(t, err)
	assert.Empty(t, auth.Username)
	assert.Empty(t, auth.Password)
}

// TestFetchDockerTokenSuccess verifies a token is parsed from the auth response.
func TestFetchDockerTokenSuccess(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusOK, Body: []byte(`{"token":"abc123"}`)},
	}}
	token, err := h.fetchDockerToken(context.Background(), "library/alpine")
	require.NoError(t, err)
	assert.Equal(t, "abc123", token)
}

// TestFetchDockerTokenNon200 verifies a non-200 response yields an error.
func TestFetchDockerTokenNon200(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusUnauthorized},
	}}
	_, err := h.fetchDockerToken(context.Background(), "library/alpine")
	assert.Error(t, err)
}

// TestFetchDockerTokenBadJSON verifies an unparseable body yields an error.
func TestFetchDockerTokenBadJSON(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusOK, Body: []byte("not-json")},
	}}
	_, err := h.fetchDockerToken(context.Background(), "library/alpine")
	assert.Error(t, err)
}

// TestGetDockerHubSystemCtx verifies a system context is built with the fetched token.
func TestGetDockerHubSystemCtx(t *testing.T) {
	h := &ImageHandler{httpClient: &stubHTTPClient{
		result: &httpclient.Result{StatusCode: http.StatusOK, Body: []byte(`{"token":"tok"}`)},
	}}
	ctx, err := h.getDockerHubSystemCtx(context.Background(), "docker.io/library/alpine:latest")
	require.NoError(t, err)
	assert.NotNil(t, ctx)
}
