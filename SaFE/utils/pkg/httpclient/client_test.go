/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package httpclient

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gotest.tools/assert"
)

// newTestServer returns a TLS test server that echoes the request method.
func newTestServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.Method))
	})
	return httptest.NewTLSServer(mux)
}

// TestNewClient verifies NewClient returns a usable singleton instance.
func TestNewClient(t *testing.T) {
	c1 := NewClient()
	c2 := NewClient()
	assert.Equal(t, c1, c2)
	assert.Assert(t, c1.GetBaseClient() != nil)
}

// TestClientMethods exercises Get, Post, Put and Delete against a test server.
func TestClientMethods(t *testing.T) {
	server := newTestServer()
	defer server.Close()

	c := NewClient()

	res, err := c.Get(server.URL)
	assert.NilError(t, err)
	assert.Equal(t, res.IsSuccess(), true)
	assert.Equal(t, string(res.Body), http.MethodGet)

	res, err = c.Post(server.URL, map[string]string{"a": "b"})
	assert.NilError(t, err)
	assert.Equal(t, string(res.Body), http.MethodPost)

	res, err = c.Put(server.URL, "body")
	assert.NilError(t, err)
	assert.Equal(t, string(res.Body), http.MethodPut)

	res, err = c.Delete(server.URL)
	assert.NilError(t, err)
	assert.Equal(t, string(res.Body), http.MethodDelete)
}

// TestDoError verifies Do returns an error when the host is unreachable.
func TestDoError(t *testing.T) {
	c := NewClient()
	req, err := http.NewRequest(http.MethodGet, "https://127.0.0.1:0", nil)
	assert.NilError(t, err)
	_, err = c.Do(req)
	assert.Assert(t, err != nil)
}

// TestBuildRequest verifies request construction for various inputs.
func TestBuildRequest(t *testing.T) {
	// scheme is added when missing and headers are applied in pairs
	req, err := BuildRequest("example.com", http.MethodGet, nil, "X-Key", "val")
	assert.NilError(t, err)
	assert.Equal(t, req.URL.Scheme, "https")
	assert.Equal(t, req.Header.Get("X-Key"), "val")
	assert.Equal(t, req.Header.Get("Content-Type"), "application/json")

	// odd header count drops the trailing key
	req, err = BuildRequest("https://example.com", http.MethodPost, []byte("x"), "only")
	assert.NilError(t, err)
	assert.Equal(t, req.Header.Get("only"), "")

	// string and io.Reader bodies are accepted
	_, err = BuildRequest("https://example.com", http.MethodPost, "str")
	assert.NilError(t, err)
	_, err = BuildRequest("https://example.com", http.MethodPost, strings.NewReader("r"))
	assert.NilError(t, err)

	// unmarshalable body returns an error
	_, err = BuildRequest("https://example.com", http.MethodPost, make(chan int))
	assert.Assert(t, err != nil)

	// invalid method returns an error
	_, err = BuildRequest("https://example.com", "bad method", nil)
	assert.Assert(t, err != nil)
}

// TestResult verifies the helpers on the Result type.
func TestResult(t *testing.T) {
	var r *Result
	assert.Equal(t, r.IsSuccess(), false)

	r = &Result{StatusCode: http.StatusOK, Body: []byte("ok")}
	assert.Equal(t, r.IsSuccess(), true)
	assert.Equal(t, r.String(), "http code: 200, body: ok")

	r = &Result{StatusCode: http.StatusInternalServerError}
	assert.Equal(t, r.IsSuccess(), false)
}
