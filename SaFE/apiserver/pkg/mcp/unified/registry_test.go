// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type emptyReq struct{}
type emptyResp struct{ OK bool `json:"ok"` }

func makeEndpoint(name, method, path string, mcpOnly, httpOnly bool) *EndpointDef[emptyReq, emptyResp] {
	return &EndpointDef[emptyReq, emptyResp]{
		Name: name, HTTPMethod: method, HTTPPath: path,
		MCPOnly: mcpOnly, HTTPOnly: httpOnly,
		Handler: func(ctx context.Context, req *emptyReq) (*emptyResp, error) {
			return &emptyResp{OK: true}, nil
		},
	}
}

func TestRegistry_RegisterAndCount(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	Register(makeEndpoint("a", "GET", "/a", false, false))
	Register(makeEndpoint("b", "GET", "/b", false, false))
	Register(makeEndpoint("c", "GET", "/c", false, false))

	assert.Equal(t, 3, GetRegistry().Count())
}

func TestRegistry_GetMCPTools(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	Register(makeEndpoint("with-http-mcp", "GET", "/h1", false, false))
	Register(makeEndpoint("http-only-skip", "GET", "/h2", false, true))
	Register(makeEndpoint("mcp-only", "GET", "/h3", true, false))

	assert.Equal(t, 3, GetRegistry().Count())
	assert.Equal(t, 2, GetRegistry().MCPToolCount())

	tools := GetRegistry().GetMCPTools()
	require.Len(t, tools, 2)
	names := make(map[string]bool)
	for _, tool := range tools {
		require.NotNil(t, tool)
		names[tool.Name] = true
	}
	assert.True(t, names["with-http-mcp"])
	assert.True(t, names["mcp-only"])
	assert.False(t, names["http-only-skip"])
}

func TestRegistry_GetMCPToolsByGroup(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	d1 := makeEndpoint("d1", "GET", "/d1", false, false)
	d1.Group = "diagnostic"
	Register(d1)

	d2 := makeEndpoint("d2", "GET", "/d2", true, false)
	d2.Group = "diagnostic"
	Register(d2)

	other := makeEndpoint("other", "GET", "/o", false, false)
	other.Group = "ops"
	Register(other)

	diag := GetRegistry().GetMCPToolsByGroup("diagnostic")
	require.Len(t, diag, 2)
	got := map[string]struct{}{}
	for _, tool := range diag {
		got[tool.Name] = struct{}{}
	}
	assert.Contains(t, got, "d1")
	assert.Contains(t, got, "d2")

	ops := GetRegistry().GetMCPToolsByGroup("ops")
	require.Len(t, ops, 1)
	assert.Equal(t, "other", ops[0].Name)
}

func TestRegistry_GetEndpointByName(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	Register(makeEndpoint("lookup-me", "GET", "/x", false, false))

	ep := GetRegistry().GetEndpointByName("lookup-me")
	require.NotNil(t, ep)
	assert.Equal(t, "lookup-me", ep.GetName())

	assert.Nil(t, GetRegistry().GetEndpointByName("missing"))
}

func TestRegistry_GetEndpointByMethodAndPath(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	getEp := makeEndpoint("get-same", "GET", "/resource", false, false)
	postEp := makeEndpoint("post-same", "POST", "/resource", false, false)
	Register(getEp)
	Register(postEp)

	g := GetRegistry().GetEndpointByMethodAndPath("GET", "/resource")
	require.NotNil(t, g)
	assert.Equal(t, "get-same", g.GetName())

	p := GetRegistry().GetEndpointByMethodAndPath("POST", "/resource")
	require.NotNil(t, p)
	assert.Equal(t, "post-same", p.GetName())

	// Fallback: method mismatch → first registration with same path
	fallback := GetRegistry().GetEndpointByMethodAndPath("PUT", "/resource")
	require.NotNil(t, fallback)
	assert.Equal(t, "get-same", fallback.GetName())
}

func TestRegistry_GetEndpointByPath(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	Register(makeEndpoint("p1", "GET", "/only-path", false, false))

	ep := GetRegistry().GetEndpointByPath("/only-path")
	require.NotNil(t, ep)
	assert.Equal(t, "p1", ep.GetName())
	assert.Nil(t, GetRegistry().GetEndpointByPath("/nope"))
}

func TestRegistry_GetMCPToolByName(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	custom := makeEndpoint("base-name", "GET", "/m", false, false)
	custom.MCPToolName = "override-tool"
	Register(custom)

	tool := GetRegistry().GetMCPToolByName("override-tool")
	require.NotNil(t, tool)
	assert.Equal(t, "override-tool", tool.Name)

	assert.Nil(t, GetRegistry().GetMCPToolByName("nope"))
}

func TestRegistry_Clear(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	Register(makeEndpoint("x", "GET", "/x", false, false))
	Register(makeEndpoint("y", "GET", "/y", true, false))
	require.NotZero(t, GetRegistry().Count())
	require.NotZero(t, GetRegistry().MCPToolCount())

	GetRegistry().Clear()
	assert.Zero(t, GetRegistry().Count())
	assert.Zero(t, GetRegistry().MCPToolCount())
}

func TestRegistry_InitGinRoutes_Methods(t *testing.T) {
	GetRegistry().Clear()
	defer GetRegistry().Clear()

	gin.SetMode(gin.TestMode)

	type row struct {
		method   string
		path     string
		httpTest string // method for httptest (HEAD/OPTIONS need exact match)
	}
	tests := []row{
		{"GET", "/tm/get", "GET"},
		{"POST", "/tm/post", "POST"},
		{"PUT", "/tm/put", "PUT"},
		{"DELETE", "/tm/delete", "DELETE"},
		{"PATCH", "/tm/patch", "PATCH"},
		{"HEAD", "/tm/head", "HEAD"},
		{"OPTIONS", "/tm/options", "OPTIONS"},
		{"Any", "/tm/any", "GET"},      // Any registers all methods; probe with GET
		{"ANY", "/tm/any_upper", "GET"},
		{"any", "/tm/any_lower", "GET"},
		{"UNKNOWN", "/tm/unknown", ""}, // no route expected
	}

	for _, tc := range tests {
		tc := tc
		name := tc.method + "_" + tc.path
		t.Run(name, func(t *testing.T) {
			GetRegistry().Clear()
			defer GetRegistry().Clear()

			ep := makeEndpoint("ep-"+tc.method, tc.method, tc.path, false, false)
			Register(ep)

			r := gin.New()
			grp := r.Group("/api")
			err := GetRegistry().InitGinRoutes(grp)
			require.NoError(t, err)

			if tc.httpTest == "" {
				req := httptest.NewRequest(http.MethodGet, "/api"+tc.path, nil)
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				assert.Equal(t, http.StatusNotFound, w.Code)
				return
			}

			req := httptest.NewRequest(tc.httpTest, "/api"+tc.path, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		})
	}
}
