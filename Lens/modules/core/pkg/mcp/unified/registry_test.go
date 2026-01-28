// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package unified

import (
	"context"
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

// Test request/response types
type GetClusterRequest struct {
	Cluster string `query:"cluster" json:"cluster" mcp:"cluster,description=Cluster name"`
}

type GetClusterResponse struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func TestRegistry_Register(t *testing.T) {
	registry := &Registry{
		endpoints: make([]EndpointRegistration, 0),
		mcpTools:  make([]*MCPTool, 0),
	}

	handler := func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
		return &GetClusterResponse{
			Name:   req.Cluster,
			Status: "healthy",
		}, nil
	}

	def := &EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:        "get_cluster",
		Description: "Get cluster information",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters/:name",
		MCPToolName: "lens_get_cluster",
		Handler:     handler,
	}

	registry.RegisterEndpoint(def)

	assert.Equal(t, 1, registry.Count())
	assert.Equal(t, 1, registry.MCPToolCount())
}

func TestRegistry_HTTPOnly(t *testing.T) {
	registry := &Registry{
		endpoints: make([]EndpointRegistration, 0),
		mcpTools:  make([]*MCPTool, 0),
	}

	def := &EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:        "download_file",
		Description: "Download a file",
		HTTPMethod:  "GET",
		HTTPPath:    "/files/:id",
		HTTPOnly:    true,
		RawHTTPHandler: func(c *gin.Context) {
			c.String(http.StatusOK, "file content")
		},
	}

	registry.RegisterEndpoint(def)

	assert.Equal(t, 1, registry.Count())
	assert.Equal(t, 0, registry.MCPToolCount()) // No MCP tool for HTTPOnly
}

func TestRegistry_MCPOnly(t *testing.T) {
	registry := &Registry{
		endpoints: make([]EndpointRegistration, 0),
		mcpTools:  make([]*MCPTool, 0),
	}

	def := &EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:        "mcp_only_tool",
		Description: "An MCP-only tool",
		MCPOnly:     true,
		MCPToolName: "lens_mcp_tool",
		Handler: func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
			return &GetClusterResponse{Name: "test"}, nil
		},
	}

	registry.RegisterEndpoint(def)

	assert.Equal(t, 1, registry.Count())
	assert.Equal(t, 1, registry.MCPToolCount())

	// Should not have HTTP handler
	ep := registry.GetEndpointByName("mcp_only_tool")
	require.NotNil(t, ep)
	assert.Nil(t, ep.GetGinHandler())
}

func TestRegistry_InitGinRoutes(t *testing.T) {
	registry := &Registry{
		endpoints: make([]EndpointRegistration, 0),
		mcpTools:  make([]*MCPTool, 0),
	}

	// Register multiple endpoints with different methods
	registry.RegisterEndpoint(&EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:       "get_cluster",
		HTTPMethod: "GET",
		HTTPPath:   "/clusters",
		Handler: func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
			return &GetClusterResponse{Name: req.Cluster, Status: "ok"}, nil
		},
	})

	registry.RegisterEndpoint(&EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:       "create_cluster",
		HTTPMethod: "POST",
		HTTPPath:   "/clusters",
		Handler: func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
			return &GetClusterResponse{Name: req.Cluster, Status: "created"}, nil
		},
	})

	// Create Gin router and register routes
	r := gin.New()
	group := r.Group("/v1")

	err := registry.InitGinRoutes(group)
	require.NoError(t, err)

	// Test GET endpoint
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/clusters?cluster=prod", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRegistry_GetMCPTools(t *testing.T) {
	registry := &Registry{
		endpoints: make([]EndpointRegistration, 0),
		mcpTools:  make([]*MCPTool, 0),
	}

	// Register an endpoint
	registry.RegisterEndpoint(&EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:        "get_cluster",
		Description: "Get cluster info",
		HTTPMethod:  "GET",
		HTTPPath:    "/clusters",
		MCPToolName: "lens_get_cluster",
		Handler: func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
			return &GetClusterResponse{Name: req.Cluster}, nil
		},
	})

	tools := registry.GetMCPTools()
	require.Len(t, tools, 1)

	tool := tools[0]
	assert.Equal(t, "lens_get_cluster", tool.Name)
	assert.Equal(t, "Get cluster info", tool.Description)

	// Test tool handler
	params := json.RawMessage(`{"cluster": "prod"}`)
	result, err := tool.Handler(context.Background(), params)
	require.NoError(t, err)

	resp, ok := result.(*GetClusterResponse)
	require.True(t, ok)
	assert.Equal(t, "prod", resp.Name)
}

func TestRegistry_GetByName(t *testing.T) {
	registry := &Registry{
		endpoints: make([]EndpointRegistration, 0),
		mcpTools:  make([]*MCPTool, 0),
	}

	registry.RegisterEndpoint(&EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:        "get_cluster",
		MCPToolName: "lens_get_cluster",
		Handler: func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
			return &GetClusterResponse{}, nil
		},
	})

	// Test GetEndpointByName
	ep := registry.GetEndpointByName("get_cluster")
	require.NotNil(t, ep)
	assert.Equal(t, "get_cluster", ep.GetName())

	// Test GetEndpointByName - not found
	ep = registry.GetEndpointByName("nonexistent")
	assert.Nil(t, ep)

	// Test GetMCPToolByName
	tool := registry.GetMCPToolByName("lens_get_cluster")
	require.NotNil(t, tool)
	assert.Equal(t, "lens_get_cluster", tool.Name)

	// Test GetMCPToolByName - not found
	tool = registry.GetMCPToolByName("nonexistent")
	assert.Nil(t, tool)
}

func TestRegistry_Clear(t *testing.T) {
	registry := &Registry{
		endpoints: make([]EndpointRegistration, 0),
		mcpTools:  make([]*MCPTool, 0),
	}

	registry.RegisterEndpoint(&EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name: "test",
		Handler: func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
			return &GetClusterResponse{}, nil
		},
	})

	assert.Equal(t, 1, registry.Count())

	registry.Clear()

	assert.Equal(t, 0, registry.Count())
	assert.Equal(t, 0, registry.MCPToolCount())
}

func TestEndpointDef_GetMCPToolName(t *testing.T) {
	// With explicit MCPToolName
	def1 := &EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name:        "get_cluster",
		MCPToolName: "lens_get_cluster",
	}
	assert.Equal(t, "lens_get_cluster", def1.GetMCPToolName())

	// Without explicit MCPToolName (defaults to Name)
	def2 := &EndpointDef[GetClusterRequest, GetClusterResponse]{
		Name: "get_cluster",
	}
	assert.Equal(t, "get_cluster", def2.GetMCPToolName())
}

func TestRegistry_AllHTTPMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "Any"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			registry := &Registry{
				endpoints: make([]EndpointRegistration, 0),
				mcpTools:  make([]*MCPTool, 0),
			}

			registry.RegisterEndpoint(&EndpointDef[GetClusterRequest, GetClusterResponse]{
				Name:       "test_" + method,
				HTTPMethod: method,
				HTTPPath:   "/test",
				Handler: func(ctx context.Context, req *GetClusterRequest) (*GetClusterResponse, error) {
					return &GetClusterResponse{}, nil
				},
			})

			r := gin.New()
			group := r.Group("/v1")

			err := registry.InitGinRoutes(group)
			assert.NoError(t, err)
		})
	}
}
