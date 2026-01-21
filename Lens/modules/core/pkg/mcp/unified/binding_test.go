// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type TestRequest struct {
	Name      string   `query:"name" json:"name"`
	ID        int      `param:"id" json:"id"`
	Limit     int      `query:"limit" json:"limit"`
	Active    bool     `query:"active" json:"active"`
	Tags      []string `query:"tags" json:"tags"`
	AuthToken string   `header:"Authorization" json:"auth_token"`
}

func TestBindGinRequest_QueryParams(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/test?name=foo&limit=10&active=true&tags=a,b,c", nil)

	var req TestRequest
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, "foo", req.Name)
	assert.Equal(t, 10, req.Limit)
	assert.True(t, req.Active)
	assert.Equal(t, []string{"a", "b", "c"}, req.Tags)
}

func TestBindGinRequest_PathParams(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/test/123", nil)
	c.Params = gin.Params{
		{Key: "id", Value: "123"},
	}

	var req TestRequest
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, 123, req.ID)
}

func TestBindGinRequest_Headers(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/test", nil)
	c.Request.Header.Set("Authorization", "Bearer token123")

	var req TestRequest
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, "Bearer token123", req.AuthToken)
}

func TestBindGinRequest_JSONBody(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := `{"name": "bar", "limit": 20}`
	c.Request = httptest.NewRequest("POST", "/test", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")

	var req TestRequest
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, "bar", req.Name)
	assert.Equal(t, 20, req.Limit)
}

func TestBindGinRequest_Combined(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	body := `{"name": "json_name"}`
	c.Request = httptest.NewRequest("POST", "/test/42?limit=50", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Authorization", "Bearer xyz")
	c.Params = gin.Params{
		{Key: "id", Value: "42"},
	}

	var req TestRequest
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, "json_name", req.Name)
	assert.Equal(t, 42, req.ID)
	assert.Equal(t, 50, req.Limit)
	assert.Equal(t, "Bearer xyz", req.AuthToken)
}

func TestBindMCPRequest(t *testing.T) {
	params := json.RawMessage(`{"name": "mcp_test", "limit": 100, "active": true}`)

	var req TestRequest
	err := BindMCPRequest(params, &req)

	require.NoError(t, err)
	assert.Equal(t, "mcp_test", req.Name)
	assert.Equal(t, 100, req.Limit)
	assert.True(t, req.Active)
}

func TestBindMCPRequest_EmptyParams(t *testing.T) {
	params := json.RawMessage(``)

	var req TestRequest
	err := BindMCPRequest(params, &req)

	require.NoError(t, err)
	assert.Equal(t, "", req.Name)
}

func TestParseMCPTag(t *testing.T) {
	tests := []struct {
		tag      string
		expected MCPTagOptions
	}{
		{
			tag: "cluster",
			expected: MCPTagOptions{
				Name: "cluster",
			},
		},
		{
			tag: "cluster,required",
			expected: MCPTagOptions{
				Name:     "cluster",
				Required: true,
			},
		},
		{
			tag: "cluster,description=The cluster name",
			expected: MCPTagOptions{
				Name:        "cluster",
				Description: "The cluster name",
			},
		},
		{
			tag: "cluster,description=The cluster name,required",
			expected: MCPTagOptions{
				Name:        "cluster",
				Description: "The cluster name",
				Required:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			opts := ParseMCPTag(tt.tag)
			assert.Equal(t, tt.expected, opts)
		})
	}
}

// TestEmbeddedStruct tests binding with embedded structs
type PaginationParams struct {
	Page     int `query:"page" json:"page"`
	PageSize int `query:"page_size" json:"page_size"`
}

type ListRequest struct {
	PaginationParams
	Filter string `query:"filter" json:"filter"`
}

func TestBindGinRequest_EmbeddedStruct(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/list?page=2&page_size=20&filter=active", nil)

	var req ListRequest
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, 2, req.Page)
	assert.Equal(t, 20, req.PageSize)
	assert.Equal(t, "active", req.Filter)
}

// TestPointerField tests binding with pointer fields
type RequestWithPointer struct {
	Name  string  `query:"name" json:"name"`
	Count *int    `query:"count" json:"count"`
	Rate  *string `query:"rate" json:"rate"`
}

func TestBindGinRequest_PointerFields(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	c.Request = httptest.NewRequest("GET", "/test?name=foo&count=5&rate=high", nil)

	var req RequestWithPointer
	err := BindGinRequest(c, &req)

	require.NoError(t, err)
	assert.Equal(t, "foo", req.Name)
	require.NotNil(t, req.Count)
	assert.Equal(t, 5, *req.Count)
	require.NotNil(t, req.Rate)
	assert.Equal(t, "high", *req.Rate)
}

// BenchmarkBindGinRequest benchmarks the request binding
func BenchmarkBindGinRequest(b *testing.B) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest("GET", "/test?name=foo&limit=10&active=true", nil)
	c.Params = gin.Params{{Key: "id", Value: "123"}}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req TestRequest
		_ = BindGinRequest(c, &req)
	}
}
