// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/gin-gonic/gin"
)

var toolsRepositoryURL string
var toolsHTTPClient = &http.Client{Timeout: 30 * time.Second}

func init() {
	// Get skills-repository service URL from environment or use default
	toolsRepositoryURL = os.Getenv("SKILLS_REPOSITORY_URL")
	if toolsRepositoryURL == "" {
		toolsRepositoryURL = "http://skills-repository:8092"
	}

	// Tools Marketplace endpoints - proxy to skills-repository service
	unified.Register(&unified.EndpointDef[ToolsListRequest, ToolsListResponse]{
		Name:        "tools_list",
		Description: "List all tools with pagination, filtering and sorting",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools",
		MCPToolName: "lens_tools_list",
		Handler:     handleToolsList,
	})

	unified.Register(&unified.EndpointDef[ToolGetRequest, ToolResponse]{
		Name:        "tools_get",
		Description: "Get a specific tool by ID",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools/:id",
		MCPToolName: "lens_tools_get",
		Handler:     handleToolGet,
	})

	unified.Register(&unified.EndpointDef[ToolUpdateRequest, ToolResponse]{
		Name:        "tools_update",
		Description: "Update an existing tool",
		HTTPMethod:  "PUT",
		HTTPPath:    "/tools/:id",
		MCPToolName: "lens_tools_update",
		Handler:     handleToolUpdate,
	})

	unified.Register(&unified.EndpointDef[ToolGetRequest, ToolsMessageResponse]{
		Name:        "tools_delete",
		Description: "Delete a tool by ID",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/tools/:id",
		MCPToolName: "lens_tools_delete",
		Handler:     handleToolDelete,
	})

	unified.Register(&unified.EndpointDef[ToolsSearchRequest, ToolsSearchResponse]{
		Name:        "tools_search",
		Description: "Search tools using keyword or semantic search",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools/search",
		MCPToolName: "lens_tools_search",
		Handler:     handleToolsSearch,
	})

	unified.Register(&unified.EndpointDef[ToolsEmptyRequest, ToolsHealthResponse]{
		Name:        "tools_health",
		Description: "Health check for tools service",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools/health",
		MCPToolName: "lens_tools_health",
		Handler:     handleToolsHealth,
	})

	unified.Register(&unified.EndpointDef[MCPCreateRequest, ToolResponse]{
		Name:        "tools_create_mcp",
		Description: "Create a new MCP Server",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/mcp",
		MCPToolName: "lens_tools_create_mcp",
		Handler:     handleToolsCreateMCP,
	})

	unified.Register(&unified.EndpointDef[ToolsRunRequest, ToolsRunResponse]{
		Name:        "tools_run",
		Description: "Run tools and get redirect URL",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/run",
		MCPToolName: "lens_tools_run",
		Handler:     handleToolsRun,
	})

	unified.Register(&unified.EndpointDef[ToolGetRequest, ToolsDownloadResponse]{
		Name:        "tools_download",
		Description: "Download a tool",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools/:id/download",
		MCPToolName: "lens_tools_download",
		Handler:     handleToolDownload,
	})

	// Import discover - multipart/form-data, needs special handling
	unified.Register(&unified.EndpointDef[ToolsEmptyRequest, ToolsImportDiscoverResponse]{
		Name:        "tools_import_discover",
		Description: "Discover skills from uploaded file or GitHub URL",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/import/discover",
		MCPToolName: "lens_tools_import_discover",
		Handler:     handleToolsImportDiscover,
	})

	unified.Register(&unified.EndpointDef[ToolsImportCommitRequest, ToolsImportCommitResponse]{
		Name:        "tools_import_commit",
		Description: "Commit selected skills from discovery",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/import/commit",
		MCPToolName: "lens_tools_import_commit",
		Handler:     handleToolsImportCommit,
	})

	// Like endpoints
	unified.Register(&unified.EndpointDef[ToolLikeRequest, ToolLikeResponse]{
		Name:        "tools_like",
		Description: "Like a tool",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/:id/like",
		MCPToolName: "lens_tools_like",
		Handler:     handleToolLike,
	})

	unified.Register(&unified.EndpointDef[ToolLikeRequest, ToolLikeResponse]{
		Name:        "tools_unlike",
		Description: "Unlike a tool",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/tools/:id/like",
		MCPToolName: "lens_tools_unlike",
		Handler:     handleToolUnlike,
	})

	// Clone endpoint
	unified.Register(&unified.EndpointDef[ToolCloneRequest, ToolCloneResponse]{
		Name:        "tools_clone",
		Description: "Clone a tool to create a copy owned by current user",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/:id/clone",
		MCPToolName: "lens_tools_clone",
		Handler:     handleToolClone,
	})

	// Toolset endpoints
	unified.Register(&unified.EndpointDef[ToolsetListRequest, ToolsetListResponse]{
		Name:        "toolsets_list",
		Description: "List all toolsets with pagination and sorting",
		HTTPMethod:  "GET",
		HTTPPath:    "/toolsets",
		MCPToolName: "lens_toolsets_list",
		Handler:     handleToolsetsList,
	})

	unified.Register(&unified.EndpointDef[ToolsetCreateRequest, ToolsetResponse]{
		Name:        "toolsets_create",
		Description: "Create a new toolset",
		HTTPMethod:  "POST",
		HTTPPath:    "/toolsets",
		MCPToolName: "lens_toolsets_create",
		Handler:     handleToolsetCreate,
	})

	unified.Register(&unified.EndpointDef[ToolsetGetRequest, ToolsetDetailResponse]{
		Name:        "toolsets_get",
		Description: "Get a toolset by ID with its tools",
		HTTPMethod:  "GET",
		HTTPPath:    "/toolsets/:id",
		MCPToolName: "lens_toolsets_get",
		Handler:     handleToolsetGet,
	})

	unified.Register(&unified.EndpointDef[ToolsetUpdateRequest, ToolsetResponse]{
		Name:        "toolsets_update",
		Description: "Update a toolset",
		HTTPMethod:  "PUT",
		HTTPPath:    "/toolsets/:id",
		MCPToolName: "lens_toolsets_update",
		Handler:     handleToolsetUpdate,
	})

	unified.Register(&unified.EndpointDef[ToolsetGetRequest, ToolsMessageResponse]{
		Name:        "toolsets_delete",
		Description: "Delete a toolset",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/toolsets/:id",
		MCPToolName: "lens_toolsets_delete",
		Handler:     handleToolsetDelete,
	})

	unified.Register(&unified.EndpointDef[ToolsetAddToolsRequest, ToolsetAddToolsResponse]{
		Name:        "toolsets_add_tools",
		Description: "Add tools to a toolset",
		HTTPMethod:  "POST",
		HTTPPath:    "/toolsets/:id/tools",
		MCPToolName: "lens_toolsets_add_tools",
		Handler:     handleToolsetAddTools,
	})

	unified.Register(&unified.EndpointDef[ToolsetRemoveToolRequest, ToolsMessageResponse]{
		Name:        "toolsets_remove_tool",
		Description: "Remove a tool from a toolset",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/toolsets/:id/tools/:toolId",
		MCPToolName: "lens_toolsets_remove_tool",
		Handler:     handleToolsetRemoveTool,
	})

	unified.Register(&unified.EndpointDef[ToolsetSearchRequest, ToolsetSearchResponse]{
		Name:        "toolsets_search",
		Description: "Search toolsets",
		HTTPMethod:  "GET",
		HTTPPath:    "/toolsets/search",
		MCPToolName: "lens_toolsets_search",
		Handler:     handleToolsetSearch,
	})
}

// ======================== Request Types ========================

type ToolsListRequest struct {
	Offset int    `json:"offset" query:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit  int    `json:"limit" query:"limit" mcp:"description=Number of items per page (default: 50)"`
	Type   string `json:"type" query:"type" mcp:"description=Filter by type (skill/mcp)"`
	Status string `json:"status" query:"status" mcp:"description=Filter by status (default: active)"`
	Sort   string `json:"sort" query:"sort" mcp:"description=Sort field (created_at/updated_at/run_count/download_count)"`
	Order  string `json:"order" query:"order" mcp:"description=Sort order (asc/desc)"`
}

type ToolGetRequest struct {
	ID string `json:"id" param:"id" mcp:"description=Tool ID,required"`
}

type ToolUpdateRequest struct {
	ID          string   `json:"id" param:"id" mcp:"description=Tool ID,required"`
	Name        string   `json:"name" mcp:"description=Tool name"`
	DisplayName string   `json:"display_name" mcp:"description=Display name"`
	Description string   `json:"description" mcp:"description=Description"`
	Tags        []string `json:"tags" mcp:"description=Tags"`
	IconURL     string   `json:"icon_url" mcp:"description=Icon URL"`
	Author      string   `json:"author" mcp:"description=Author"`
	IsPublic    *bool    `json:"is_public" mcp:"description=Is public"`
}

type ToolsSearchRequest struct {
	Query string `json:"q" query:"q" mcp:"description=Search query,required"`
	Mode  string `json:"mode" query:"mode" mcp:"description=Search mode (keyword/semantic/hybrid)"`
	Type  string `json:"type" query:"type" mcp:"description=Filter by type (skill/mcp)"`
	Limit int    `json:"limit" query:"limit" mcp:"description=Maximum number of results (default: 20)"`
}

type MCPCreateRequest struct {
	Name        string                 `json:"name" binding:"required" mcp:"description=MCP server name,required"`
	DisplayName string                 `json:"display_name" mcp:"description=Display name"`
	Description string                 `json:"description" binding:"required" mcp:"description=Description,required"`
	Tags        []string               `json:"tags" mcp:"description=Tags"`
	IconURL     string                 `json:"icon_url" mcp:"description=Icon URL"`
	Author      string                 `json:"author" mcp:"description=Author"`
	Config      map[string]interface{} `json:"config" binding:"required" mcp:"description=MCP server config (mcpServers format),required"`
	IsPublic    *bool                  `json:"is_public" mcp:"description=Is public"`
}

type ToolRef struct {
	ID   *int64 `json:"id" mcp:"description=Tool ID (preferred)"`
	Type string `json:"type" mcp:"description=Tool type (skill/mcp)"`
	Name string `json:"name" mcp:"description=Tool name"`
}

type ToolsRunRequest struct {
	Tools []ToolRef `json:"tools" binding:"required" mcp:"description=Tools to run,required"`
}

type ToolsImportCommitRequest struct {
	ArchiveKey string                       `json:"archive_key" binding:"required" mcp:"description=Archive key from discover,required"`
	Selections []ToolsImportCommitSelection `json:"selections" binding:"required" mcp:"description=Selected skills to import,required"`
}

type ToolLikeRequest struct {
	ID string `json:"id" param:"id" mcp:"description=Tool ID,required"`
}

type ToolsImportCommitSelection struct {
	RelativePath string  `json:"relative_path" mcp:"description=Relative path of the skill"`
	NameOverride *string `json:"name_override" mcp:"description=Override skill name"`
}

type ToolsEmptyRequest struct{}

// ======================== Response Types ========================

type ToolsListResponse struct {
	Tools  []ToolData `json:"tools"`
	Total  int64      `json:"total"`
	Offset int        `json:"offset"`
	Limit  int        `json:"limit"`
}

type ToolResponse struct {
	ToolData
}

type ToolData struct {
	ID             int64                  `json:"id"`
	Type           string                 `json:"type"`
	Name           string                 `json:"name"`
	DisplayName    string                 `json:"display_name"`
	Description    string                 `json:"description"`
	Tags           []string               `json:"tags"`
	IconURL        string                 `json:"icon_url"`
	Author         string                 `json:"author"`
	Config         map[string]interface{} `json:"config"`
	SkillSource    string                 `json:"skill_source,omitempty"`
	SkillSourceURL string                 `json:"skill_source_url,omitempty"`
	OwnerUserID    string                 `json:"owner_user_id"`
	IsPublic       bool                   `json:"is_public"`
	Status         string                 `json:"status"`
	RunCount       int                    `json:"run_count"`
	DownloadCount  int                    `json:"download_count"`
	LikeCount      int                    `json:"like_count"`
	IsLiked        bool                   `json:"is_liked"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
}

type ToolsSearchResponse struct {
	Tools []ToolDataWithScore `json:"tools"`
	Total int                 `json:"total"`
	Mode  string              `json:"mode"`
}

type ToolDataWithScore struct {
	ToolData
	Score float64 `json:"score,omitempty"`
}

type ToolsHealthResponse struct {
	Status string `json:"status"`
}

type ToolsMessageResponse struct {
	Message string `json:"message"`
}

type ToolsRunResponse struct {
	RedirectURL string `json:"redirect_url"`
	SessionID   string `json:"session_id,omitempty"`
}

type ToolsDownloadResponse struct {
	// Binary content - handled by proxy
}

type ToolsImportDiscoverResponse struct {
	ArchiveKey string                 `json:"archive_key"`
	Candidates []ToolsImportCandidate `json:"candidates"`
}

type ToolsImportCandidate struct {
	RelativePath     string `json:"relative_path"`
	SkillName        string `json:"skill_name"`
	SkillDescription string `json:"skill_description"`
	RequiresName     bool   `json:"requires_name"`
	WillOverwrite    bool   `json:"will_overwrite"`
}

type ToolsImportCommitResponse struct {
	Items []ToolsImportCommitResult `json:"items"`
}

type ToolsImportCommitResult struct {
	RelativePath string `json:"relative_path"`
	SkillName    string `json:"skill_name"`
	Status       string `json:"status"`
	ToolID       int64  `json:"tool_id,omitempty"`
	Error        string `json:"error,omitempty"`
}

type ToolLikeResponse struct {
	Message   string `json:"message"`
	LikeCount int    `json:"like_count"`
}

type ToolCloneRequest struct {
	ID string `json:"id" param:"id" mcp:"description=Tool ID to clone,required"`
}

type ToolCloneResponse struct {
	Message string   `json:"message"`
	Tool    ToolData `json:"tool"`
}

// ======================== Handlers ========================

func handleToolsList(ctx context.Context, req *ToolsListRequest) (*ToolsListResponse, error) {
	params := url.Values{}
	if req.Offset > 0 {
		params.Set("offset", strconv.Itoa(req.Offset))
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Type != "" {
		params.Set("type", req.Type)
	}
	if req.Status != "" {
		params.Set("status", req.Status)
	}
	if req.Sort != "" {
		params.Set("sort", req.Sort)
	}
	if req.Order != "" {
		params.Set("order", req.Order)
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolsListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse tools list response", errors.InternalError)
	}
	return &result, nil
}

func handleToolGet(ctx context.Context, req *ToolGetRequest) (*ToolResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool ID is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.ID)
	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse tool response", errors.InternalError)
	}
	return &ToolResponse{ToolData: result}, nil
}

func handleToolUpdate(ctx context.Context, req *ToolUpdateRequest) (*ToolResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool ID is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.ID)
	resp, err := toolsProxyPut(ctx, reqURL, req)
	if err != nil {
		return nil, err
	}

	var result ToolData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse tool response", errors.InternalError)
	}
	return &ToolResponse{ToolData: result}, nil
}

func handleToolDelete(ctx context.Context, req *ToolGetRequest) (*ToolsMessageResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool ID is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.ID)
	_, err := toolsProxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	return &ToolsMessageResponse{Message: "Tool deleted successfully"}, nil
}

func handleToolsSearch(ctx context.Context, req *ToolsSearchRequest) (*ToolsSearchResponse, error) {
	params := url.Values{}
	if req.Query != "" {
		params.Set("q", req.Query)
	}
	if req.Mode != "" {
		params.Set("mode", req.Mode)
	}
	if req.Type != "" {
		params.Set("type", req.Type)
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/search"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolsSearchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse search response", errors.InternalError)
	}
	return &result, nil
}

func handleToolsHealth(ctx context.Context, req *ToolsEmptyRequest) (*ToolsHealthResponse, error) {
	reqURL := toolsRepositoryURL + "/api/v1/tools/health"
	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolsHealthResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse health response", errors.InternalError)
	}
	return &result, nil
}

func handleToolsCreateMCP(ctx context.Context, req *MCPCreateRequest) (*ToolResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("MCP name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/mcp"
	resp, err := toolsProxyPost(ctx, reqURL, req)
	if err != nil {
		return nil, err
	}

	var result ToolData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse tool response", errors.InternalError)
	}
	return &ToolResponse{ToolData: result}, nil
}

func handleToolsRun(ctx context.Context, req *ToolsRunRequest) (*ToolsRunResponse, error) {
	if len(req.Tools) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("at least one tool is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/run"
	resp, err := toolsProxyPost(ctx, reqURL, req)
	if err != nil {
		return nil, err
	}

	var result ToolsRunResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse run response", errors.InternalError)
	}
	return &result, nil
}

func handleToolDownload(ctx context.Context, req *ToolGetRequest) (*ToolsDownloadResponse, error) {
	// Download is handled by direct proxy in router for binary content
	return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("download should be handled by direct proxy")
}

// ToolsDownloadProxyHandler returns a Gin handler for proxying download requests
// This handles binary content (ZIP files)
func ToolsDownloadProxyHandler() func(c *gin.Context) {
	return func(c *gin.Context) {
		id := c.Param("id")
		targetURL := toolsRepositoryURL + "/api/v1/tools/" + id + "/download"

		// Create a new request to the target
		proxyReq, err := http.NewRequestWithContext(c.Request.Context(), "GET", targetURL, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
			return
		}

		// Execute the request
		resp, err := toolsHTTPClient.Do(proxyReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reach skills-repository"})
			return
		}
		defer resp.Body.Close()

		// Copy response headers (Content-Type, Content-Disposition, etc.)
		for key, values := range resp.Header {
			for _, value := range values {
				c.Header(key, value)
			}
		}

		// Stream response body
		c.Status(resp.StatusCode)
		io.Copy(c.Writer, resp.Body)
	}
}

func handleToolsImportDiscover(ctx context.Context, req *ToolsEmptyRequest) (*ToolsImportDiscoverResponse, error) {
	// Import discover is multipart/form-data, handled by direct proxy in router
	return nil, errors.NewError().WithCode(errors.InternalError).WithMessage("import discover should be handled by direct proxy")
}

// ToolsImportDiscoverProxyHandler returns a Gin handler for proxying import/discover requests
// This handles multipart/form-data file uploads
func ToolsImportDiscoverProxyHandler() func(c *gin.Context) {
	return func(c *gin.Context) {
		targetURL := toolsRepositoryURL + "/api/v1/tools/import/discover"

		// Create a new request to the target
		proxyReq, err := http.NewRequestWithContext(c.Request.Context(), "POST", targetURL, c.Request.Body)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
			return
		}

		// Copy headers (especially Content-Type for multipart)
		for key, values := range c.Request.Header {
			for _, value := range values {
				proxyReq.Header.Add(key, value)
			}
		}

		// Execute the request
		resp, err := toolsHTTPClient.Do(proxyReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to reach skills-repository"})
			return
		}
		defer resp.Body.Close()

		// Copy response headers
		for key, values := range resp.Header {
			for _, value := range values {
				c.Header(key, value)
			}
		}

		// Copy response body
		body, _ := io.ReadAll(resp.Body)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}

func handleToolsImportCommit(ctx context.Context, req *ToolsImportCommitRequest) (*ToolsImportCommitResponse, error) {
	if req.ArchiveKey == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("archive_key is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/import/commit"
	resp, err := toolsProxyPost(ctx, reqURL, req)
	if err != nil {
		return nil, err
	}

	var result ToolsImportCommitResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse commit response", errors.InternalError)
	}
	return &result, nil
}

func handleToolLike(ctx context.Context, req *ToolLikeRequest) (*ToolLikeResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool id is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + req.ID + "/like"
	resp, err := toolsProxyPost(ctx, reqURL, nil)
	if err != nil {
		return nil, err
	}

	var result ToolLikeResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse like response", errors.InternalError)
	}
	return &result, nil
}

func handleToolUnlike(ctx context.Context, req *ToolLikeRequest) (*ToolLikeResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool id is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + req.ID + "/like"
	resp, err := toolsProxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolLikeResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse unlike response", errors.InternalError)
	}
	return &result, nil
}

func handleToolClone(ctx context.Context, req *ToolCloneRequest) (*ToolCloneResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool id is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + req.ID + "/clone"
	resp, err := toolsProxyPost(ctx, reqURL, nil)
	if err != nil {
		return nil, err
	}

	var result ToolCloneResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse clone response", errors.InternalError)
	}
	return &result, nil
}

// ======================== Toolset Request Types ========================

type ToolsetListRequest struct {
	Offset int    `json:"offset" query:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit  int    `json:"limit" query:"limit" mcp:"description=Number of items per page (default: 50)"`
	Sort   string `json:"sort" query:"sort" mcp:"description=Sort field (created_at/updated_at/tool_count)"`
	Order  string `json:"order" query:"order" mcp:"description=Sort order (asc/desc)"`
	Owner  string `json:"owner" query:"owner" mcp:"description=Filter by owner (me for own toolsets)"`
}

type ToolsetGetRequest struct {
	ID string `json:"id" param:"id" mcp:"description=Toolset ID,required"`
}

type ToolsetCreateRequest struct {
	Name        string   `json:"name" binding:"required" mcp:"description=Toolset name,required"`
	DisplayName string   `json:"display_name" mcp:"description=Display name"`
	Description string   `json:"description" mcp:"description=Description"`
	Tags        []string `json:"tags" mcp:"description=Tags"`
	IconURL     string   `json:"icon_url" mcp:"description=Icon URL"`
	IsPublic    *bool    `json:"is_public" mcp:"description=Is public"`
	ToolIDs     []int64  `json:"tool_ids" mcp:"description=Initial tool IDs to add"`
}

type ToolsetUpdateRequest struct {
	ID          string   `json:"id" param:"id" mcp:"description=Toolset ID,required"`
	DisplayName string   `json:"display_name" mcp:"description=Display name"`
	Description string   `json:"description" mcp:"description=Description"`
	Tags        []string `json:"tags" mcp:"description=Tags"`
	IconURL     string   `json:"icon_url" mcp:"description=Icon URL"`
	IsPublic    *bool    `json:"is_public" mcp:"description=Is public"`
}

type ToolsetAddToolsRequest struct {
	ID      string  `json:"id" param:"id" mcp:"description=Toolset ID,required"`
	ToolIDs []int64 `json:"tool_ids" binding:"required" mcp:"description=Tool IDs to add,required"`
}

type ToolsetRemoveToolRequest struct {
	ID     string `json:"id" param:"id" mcp:"description=Toolset ID,required"`
	ToolID string `json:"toolId" param:"toolId" mcp:"description=Tool ID to remove,required"`
}

type ToolsetSearchRequest struct {
	Query string `json:"q" query:"q" mcp:"description=Search query,required"`
	Mode  string `json:"mode" query:"mode" mcp:"description=Search mode (keyword/semantic)"`
	Limit int    `json:"limit" query:"limit" mcp:"description=Maximum number of results (default: 20)"`
}

// ======================== Toolset Response Types ========================

type ToolsetData struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	DisplayName   string    `json:"display_name"`
	Description   string    `json:"description"`
	Tags          []string  `json:"tags"`
	IconURL       string    `json:"icon_url"`
	OwnerUserID   string    `json:"owner_user_id"`
	OwnerUserName string    `json:"owner_user_name"`
	IsPublic      bool      `json:"is_public"`
	ToolCount     int       `json:"tool_count"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ToolsetResponse struct {
	ToolsetData
}

type ToolsetDetailResponse struct {
	ToolsetData
	Tools []ToolData `json:"tools"`
}

type ToolsetListResponse struct {
	Toolsets []ToolsetData `json:"toolsets"`
	Total    int64         `json:"total"`
	Offset   int           `json:"offset"`
	Limit    int           `json:"limit"`
}

type ToolsetSearchResponse struct {
	Toolsets interface{} `json:"toolsets"`
	Total    int         `json:"total"`
	Mode     string      `json:"mode"`
}

type ToolsetAddToolsResponse struct {
	Message string `json:"message"`
	Added   int    `json:"added"`
}

// ======================== Toolset Handlers ========================

func handleToolsetsList(ctx context.Context, req *ToolsetListRequest) (*ToolsetListResponse, error) {
	params := url.Values{}
	if req.Offset > 0 {
		params.Set("offset", strconv.Itoa(req.Offset))
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Sort != "" {
		params.Set("sort", req.Sort)
	}
	if req.Order != "" {
		params.Set("order", req.Order)
	}
	if req.Owner != "" {
		params.Set("owner", req.Owner)
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolsetListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse toolsets list response", errors.InternalError)
	}
	return &result, nil
}

func handleToolsetCreate(ctx context.Context, req *ToolsetCreateRequest) (*ToolsetResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("toolset name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets"
	resp, err := toolsProxyPost(ctx, reqURL, req)
	if err != nil {
		return nil, err
	}

	var result ToolsetData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse toolset response", errors.InternalError)
	}
	return &ToolsetResponse{ToolsetData: result}, nil
}

func handleToolsetGet(ctx context.Context, req *ToolsetGetRequest) (*ToolsetDetailResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("toolset ID is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets/" + url.PathEscape(req.ID)
	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolsetDetailResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse toolset response", errors.InternalError)
	}
	return &result, nil
}

func handleToolsetUpdate(ctx context.Context, req *ToolsetUpdateRequest) (*ToolsetResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("toolset ID is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets/" + url.PathEscape(req.ID)
	resp, err := toolsProxyPut(ctx, reqURL, req)
	if err != nil {
		return nil, err
	}

	var result ToolsetData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse toolset response", errors.InternalError)
	}
	return &ToolsetResponse{ToolsetData: result}, nil
}

func handleToolsetDelete(ctx context.Context, req *ToolsetGetRequest) (*ToolsMessageResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("toolset ID is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets/" + url.PathEscape(req.ID)
	_, err := toolsProxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	return &ToolsMessageResponse{Message: "Toolset deleted successfully"}, nil
}

func handleToolsetAddTools(ctx context.Context, req *ToolsetAddToolsRequest) (*ToolsetAddToolsResponse, error) {
	if req.ID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("toolset ID is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets/" + url.PathEscape(req.ID) + "/tools"
	resp, err := toolsProxyPost(ctx, reqURL, req)
	if err != nil {
		return nil, err
	}

	var result ToolsetAddToolsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse add tools response", errors.InternalError)
	}
	return &result, nil
}

func handleToolsetRemoveTool(ctx context.Context, req *ToolsetRemoveToolRequest) (*ToolsMessageResponse, error) {
	if req.ID == "" || req.ToolID == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("toolset ID and tool ID are required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets/" + url.PathEscape(req.ID) + "/tools/" + url.PathEscape(req.ToolID)
	_, err := toolsProxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	return &ToolsMessageResponse{Message: "Tool removed from toolset"}, nil
}

func handleToolsetSearch(ctx context.Context, req *ToolsetSearchRequest) (*ToolsetSearchResponse, error) {
	params := url.Values{}
	if req.Query != "" {
		params.Set("q", req.Query)
	}
	if req.Mode != "" {
		params.Set("mode", req.Mode)
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}

	reqURL := toolsRepositoryURL + "/api/v1/toolsets/search"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolsetSearchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse toolset search response", errors.InternalError)
	}
	return &result, nil
}

// ======================== Proxy Helpers ========================

func toolsProxyGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}

	resp, err := toolsHTTPClient.Do(req)
	if err != nil {
		return nil, errors.WrapError(err, "failed to call skills-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("skills-repository error: %s", string(body)))
	}

	return body, nil
}

func toolsProxyPost(ctx context.Context, reqURL string, data interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, errors.WrapError(err, "failed to marshal request body", errors.InternalError)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := toolsHTTPClient.Do(req)
	if err != nil {
		return nil, errors.WrapError(err, "failed to call skills-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("skills-repository error: %s", string(body)))
	}

	return body, nil
}

func toolsProxyPut(ctx context.Context, reqURL string, data interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, errors.WrapError(err, "failed to marshal request body", errors.InternalError)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := toolsHTTPClient.Do(req)
	if err != nil {
		return nil, errors.WrapError(err, "failed to call skills-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("skills-repository error: %s", string(body)))
	}

	return body, nil
}

func toolsProxyDelete(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}

	resp, err := toolsHTTPClient.Do(req)
	if err != nil {
		return nil, errors.WrapError(err, "failed to call skills-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("skills-repository error: %s", string(body)))
	}

	return body, nil
}
