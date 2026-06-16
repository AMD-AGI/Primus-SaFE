/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestClient(handler http.HandlerFunc) (*GitHubClient, *httptest.Server) {
	server := httptest.NewServer(handler)
	c := NewGitHubClient("test-token")
	c.baseURL = server.URL
	c.httpClient = server.Client()
	return c, server
}

func TestNewGitHubClient(t *testing.T) {
	c := NewGitHubClient("tok")
	assert.Equal(t, "tok", c.token)
	assert.Equal(t, "https://api.github.com", c.baseURL)
}

func TestGetWorkflowRun(t *testing.T) {
	c, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"id":10,"name":"ci","status":"completed"}`))
	})
	defer server.Close()

	run, err := c.GetWorkflowRun(context.Background(), "o", "r", 10)
	assert.NoError(t, err)
	assert.Equal(t, int64(10), run.ID)
	assert.Equal(t, "ci", run.Name)
}

func TestGetAllJobs(t *testing.T) {
	c, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"total_count":1,"jobs":[{"id":1,"name":"build"}]}`))
	})
	defer server.Close()

	jobs, err := c.GetAllJobs(context.Background(), "o", "r", 10)
	assert.NoError(t, err)
	assert.Len(t, jobs, 1)
	assert.Equal(t, "build", jobs[0].Name)
}

func TestGetCommit(t *testing.T) {
	c, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"abc","stats":{"additions":1,"deletions":2}}`))
	})
	defer server.Close()

	commit, err := c.GetCommit(context.Background(), "o", "r", "abc")
	assert.NoError(t, err)
	assert.Equal(t, "abc", commit.SHA)
	assert.Equal(t, 1, commit.Stats.Additions)
}

func TestGetWorkflowRunNotFound(t *testing.T) {
	c, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	defer server.Close()

	_, err := c.GetWorkflowRun(context.Background(), "o", "r", 10)
	assert.Error(t, err)
}

func TestDoWithRetryServerErrorThenSuccess(t *testing.T) {
	var calls int
	c, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"id":7}`))
	})
	defer server.Close()

	run, err := c.GetWorkflowRun(context.Background(), "o", "r", 7)
	assert.NoError(t, err)
	assert.Equal(t, int64(7), run.ID)
	assert.GreaterOrEqual(t, calls, 2)
}

func TestDoWithRetryRateLimited(t *testing.T) {
	var calls int
	c, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"id":8}`))
	})
	defer server.Close()

	run, err := c.GetWorkflowRun(context.Background(), "o", "r", 8)
	assert.NoError(t, err)
	assert.Equal(t, int64(8), run.ID)
}

func TestGetWorkflowRunClientError(t *testing.T) {
	c, server := newTestClient(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad"))
	})
	defer server.Close()

	_, err := c.GetWorkflowRun(context.Background(), "o", "r", 10)
	assert.Error(t, err)
}
