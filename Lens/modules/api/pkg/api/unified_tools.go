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
)

var toolsRepositoryURL string

func init() {
	// Get tools-repository service URL from environment or use default
	toolsRepositoryURL = os.Getenv("TOOLS_REPOSITORY_URL")
	if toolsRepositoryURL == "" {
		toolsRepositoryURL = "http://tools-repository:8093"
	}

	// Tools Repository endpoints - proxy to tools-repository service
	unified.Register(&unified.EndpointDef[ToolsListRequest, ToolsListResponse]{
		Name:        "tools_list",
		Description: "List all tools with pagination and filtering",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools",
		MCPToolName: "lens_tools_list",
		Handler:     handleToolsList,
	})

	unified.Register(&unified.EndpointDef[ToolGetRequest, ToolResponse]{
		Name:        "tools_get",
		Description: "Get a specific tool by name",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools/:name",
		MCPToolName: "lens_tools_get",
		Handler:     handleToolGet,
	})

	unified.Register(&unified.EndpointDef[ToolGetRequest, ToolSchemaResponse]{
		Name:        "tools_get_schema",
		Description: "Get the input/output schema for a tool",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools/:name/schema",
		MCPToolName: "lens_tools_get_schema",
		Handler:     handleToolGetSchema,
	})

	unified.Register(&unified.EndpointDef[ToolsSearchRequest, ToolsSearchResponse]{
		Name:        "tools_search",
		Description: "Semantic search for tools using natural language query",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/search",
		MCPToolName: "lens_tools_search",
		Handler:     handleToolsSearch,
	})

	unified.Register(&unified.EndpointDef[ToolsDiscoverRequest, ToolsDiscoverResponse]{
		Name:        "tools_discover",
		Description: "Discover tools relevant to a specific task",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/discover",
		MCPToolName: "lens_tools_discover",
		Handler:     handleToolsDiscover,
	})

	unified.Register(&unified.EndpointDef[ToolCreateRequest, ToolResponse]{
		Name:        "tools_create",
		Description: "Register a new tool",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools",
		MCPToolName: "lens_tools_create",
		Handler:     handleToolCreate,
	})

	unified.Register(&unified.EndpointDef[ToolUpdateRequest, ToolResponse]{
		Name:        "tools_update",
		Description: "Update an existing tool",
		HTTPMethod:  "PUT",
		HTTPPath:    "/tools/:name",
		MCPToolName: "lens_tools_update",
		Handler:     handleToolUpdate,
	})

	unified.Register(&unified.EndpointDef[ToolGetRequest, ToolsMessageResponse]{
		Name:        "tools_delete",
		Description: "Unregister a tool by name",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/tools/:name",
		MCPToolName: "lens_tools_delete",
		Handler:     handleToolDelete,
	})

	unified.Register(&unified.EndpointDef[ToolInvokeRequest, ToolInvokeResponse]{
		Name:        "tools_invoke",
		Description: "Invoke a tool with the given input",
		HTTPMethod:  "POST",
		HTTPPath:    "/tools/:name/invoke",
		MCPToolName: "lens_tools_invoke",
		Handler:     handleToolInvoke,
	})

	unified.Register(&unified.EndpointDef[ToolGetRequest, ToolAnalyticsResponse]{
		Name:        "tools_analytics",
		Description: "Get usage analytics for a tool",
		HTTPMethod:  "GET",
		HTTPPath:    "/tools/:name/analytics",
		MCPToolName: "lens_tools_analytics",
		Handler:     handleToolAnalytics,
	})

	unified.Register(&unified.EndpointDef[DomainsListRequest, DomainsListResponse]{
		Name:        "domains_list",
		Description: "List all tool domains",
		HTTPMethod:  "GET",
		HTTPPath:    "/domains",
		MCPToolName: "lens_domains_list",
		Handler:     handleDomainsList,
	})

	unified.Register(&unified.EndpointDef[DomainGetRequest, DomainToolsResponse]{
		Name:        "domains_tools",
		Description: "List tools in a specific domain",
		HTTPMethod:  "GET",
		HTTPPath:    "/domains/:domain/tools",
		MCPToolName: "lens_domains_tools",
		Handler:     handleDomainTools,
	})
}

// ======================== Request Types ========================

type ToolsListRequest struct {
	Offset   int    `json:"offset" form:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit    int    `json:"limit" form:"limit" mcp:"description=Number of items per page (default: 50)"`
	Category string `json:"category" form:"category" mcp:"description=Filter by category (observability/diagnosis/management/data/workflow)"`
	Domain   string `json:"domain" form:"domain" mcp:"description=Filter by domain (training/cluster/workflow/logging)"`
	Scope    string `json:"scope" form:"scope" mcp:"description=Filter by access scope (platform/team/user)"`
	Status   string `json:"status" form:"status" mcp:"description=Filter by status (active/inactive/disabled)"`
}

type ToolGetRequest struct {
	Name string `json:"name" param:"name" mcp:"description=Tool name,required"`
}

type ToolsSearchRequest struct {
	Query string `json:"query" binding:"required" mcp:"description=Natural language search query,required"`
	Limit int    `json:"limit" mcp:"description=Maximum number of results (default: 10)"`
}

type ToolsDiscoverRequest struct {
	Task   string `json:"task" binding:"required" mcp:"description=Task description to find relevant tools,required"`
	Limit  int    `json:"limit" mcp:"description=Maximum number of results (default: 10)"`
	Domain string `json:"domain" mcp:"description=Optional domain to filter results"`
}

type ToolCreateRequest struct {
	Name              string                 `json:"name" binding:"required" mcp:"description=Tool name,required"`
	Version           string                 `json:"version" binding:"required" mcp:"description=Tool version,required"`
	Description       string                 `json:"description" binding:"required" mcp:"description=Tool description,required"`
	Category          string                 `json:"category" mcp:"description=Tool category"`
	Domain            string                 `json:"domain" mcp:"description=Tool domain"`
	Tags              []string               `json:"tags" mcp:"description=Tool tags"`
	ProviderType      string                 `json:"provider_type" binding:"required" mcp:"description=Provider type (mcp/http/grpc),required"`
	ProviderEndpoint  string                 `json:"provider_endpoint" binding:"required" mcp:"description=Provider endpoint URL,required"`
	ProviderTimeoutMs int                    `json:"provider_timeout_ms" mcp:"description=Provider timeout in milliseconds"`
	InputSchema       map[string]interface{} `json:"input_schema" mcp:"description=JSON Schema for tool input"`
	OutputSchema      map[string]interface{} `json:"output_schema" mcp:"description=JSON Schema for tool output"`
	ReadOnlyHint      *bool                  `json:"read_only_hint" mcp:"description=Whether tool only reads data"`
	DestructiveHint   *bool                  `json:"destructive_hint" mcp:"description=Whether tool can be destructive"`
	IdempotentHint    *bool                  `json:"idempotent_hint" mcp:"description=Whether tool is idempotent"`
	OpenWorldHint     *bool                  `json:"open_world_hint" mcp:"description=Whether tool accesses external resources"`
	AccessScope       string                 `json:"access_scope" mcp:"description=Access scope (platform/team/user)"`
	AccessRoles       []string               `json:"access_roles" mcp:"description=Roles allowed to use this tool"`
	AccessTeams       []string               `json:"access_teams" mcp:"description=Teams allowed to use this tool"`
	AccessUsers       []string               `json:"access_users" mcp:"description=Users allowed to use this tool"`
	Examples          []ToolExampleData      `json:"examples" mcp:"description=Usage examples"`
	OwnerType         string                 `json:"owner_type" mcp:"description=Owner type (platform/team/user)"`
	OwnerID           string                 `json:"owner_id" mcp:"description=Owner ID"`
}

type ToolUpdateRequest struct {
	Name              string                 `json:"name" param:"name" mcp:"description=Tool name,required"`
	Version           string                 `json:"version" mcp:"description=Tool version"`
	Description       string                 `json:"description" mcp:"description=Tool description"`
	Category          string                 `json:"category" mcp:"description=Tool category"`
	Domain            string                 `json:"domain" mcp:"description=Tool domain"`
	Tags              []string               `json:"tags" mcp:"description=Tool tags"`
	ProviderType      string                 `json:"provider_type" mcp:"description=Provider type"`
	ProviderEndpoint  string                 `json:"provider_endpoint" mcp:"description=Provider endpoint URL"`
	ProviderTimeoutMs int                    `json:"provider_timeout_ms" mcp:"description=Provider timeout in milliseconds"`
	InputSchema       map[string]interface{} `json:"input_schema" mcp:"description=JSON Schema for tool input"`
	OutputSchema      map[string]interface{} `json:"output_schema" mcp:"description=JSON Schema for tool output"`
	Status            string                 `json:"status" mcp:"description=Tool status"`
}

type ToolInvokeRequest struct {
	Name   string                 `json:"name" param:"name" mcp:"description=Tool name,required"`
	Input  map[string]interface{} `json:"input" mcp:"description=Tool input parameters"`
	UserID string                 `json:"user_id" mcp:"description=User ID for tracking"`
}

type DomainsListRequest struct{}

type DomainGetRequest struct {
	Domain string `json:"domain" param:"domain" mcp:"description=Domain name,required"`
}

// ======================== Response Types ========================

type ToolProviderData struct {
	Type      string `json:"type"`
	Endpoint  string `json:"endpoint"`
	TimeoutMs int    `json:"timeout_ms"`
}

type ToolAnnotationsData struct {
	ReadOnlyHint    bool `json:"read_only_hint"`
	DestructiveHint bool `json:"destructive_hint"`
	IdempotentHint  bool `json:"idempotent_hint"`
	OpenWorldHint   bool `json:"open_world_hint"`
}

type ToolAccessData struct {
	Scope string   `json:"scope"`
	Roles []string `json:"roles"`
	Teams []string `json:"teams,omitempty"`
	Users []string `json:"users,omitempty"`
}

type ToolExampleData struct {
	Description string                 `json:"description"`
	Input       map[string]interface{} `json:"input"`
	Output      map[string]interface{} `json:"output"`
}

type ToolData struct {
	ID           int64                  `json:"id"`
	Name         string                 `json:"name"`
	Version      string                 `json:"version"`
	Description  string                 `json:"description"`
	Category     string                 `json:"category"`
	Domain       string                 `json:"domain"`
	Tags         []string               `json:"tags"`
	Provider     ToolProviderData       `json:"provider"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	OutputSchema map[string]interface{} `json:"output_schema"`
	Annotations  ToolAnnotationsData    `json:"annotations"`
	Access       ToolAccessData         `json:"access"`
	Examples     []ToolExampleData      `json:"examples"`
	OwnerType    string                 `json:"owner_type"`
	OwnerID      string                 `json:"owner_id"`
	Status       string                 `json:"status"`
	RegisteredAt time.Time              `json:"registered_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

type ToolsListResponse struct {
	Tools  []*ToolData `json:"tools"`
	Total  int64       `json:"total"`
	Offset int         `json:"offset"`
	Limit  int         `json:"limit"`
}

type ToolResponse struct {
	*ToolData
}

type ToolSchemaResponse struct {
	Name         string                 `json:"name"`
	InputSchema  map[string]interface{} `json:"input_schema"`
	OutputSchema map[string]interface{} `json:"output_schema"`
}

type ToolSearchResultData struct {
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Category       string  `json:"category"`
	Domain         string  `json:"domain"`
	RelevanceScore float64 `json:"relevance_score"`
}

type ToolsSearchResponse struct {
	Tools []*ToolSearchResultData `json:"tools"`
	Total int                     `json:"total"`
	Hint  string                  `json:"hint"`
}

type ToolsDiscoverResponse struct {
	Tools   []*ToolSearchResultData            `json:"tools"`
	Domains map[string][]*ToolSearchResultData `json:"domains"`
	Query   string                             `json:"query"`
	Hint    string                             `json:"hint"`
}

type ToolsMessageResponse struct {
	Message string `json:"message"`
}

type ToolInvokeResponse struct {
	ToolName   string                 `json:"tool_name"`
	Output     map[string]interface{} `json:"output"`
	Error      string                 `json:"error,omitempty"`
	DurationMs int64                  `json:"duration_ms"`
}

type ToolAnalyticsResponse struct {
	ToolName         string     `json:"tool_name"`
	TotalInvocations int64      `json:"total_invocations"`
	SuccessCount     int64      `json:"success_count"`
	FailureCount     int64      `json:"failure_count"`
	AvgDurationMs    int        `json:"avg_duration_ms"`
	P50DurationMs    int        `json:"p50_duration_ms"`
	P99DurationMs    int        `json:"p99_duration_ms"`
	ErrorRate        float64    `json:"error_rate"`
	LastInvokedAt    *time.Time `json:"last_invoked_at"`
}

type DomainData struct {
	Domain      string   `json:"domain"`
	Description string   `json:"description"`
	ToolCount   int      `json:"tool_count"`
	ToolNames   []string `json:"tool_names"`
}

type DomainsListResponse struct {
	Domains []*DomainData `json:"domains"`
	Total   int           `json:"total"`
}

type DomainToolsResponse struct {
	Domain string      `json:"domain"`
	Tools  []*ToolData `json:"tools"`
	Total  int         `json:"total"`
}

// ======================== Handler Implementations ========================

// HTTP client with timeout for tools repository
var toolsHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

func handleToolsList(ctx context.Context, req *ToolsListRequest) (*ToolsListResponse, error) {
	// Build query parameters
	params := url.Values{}
	if req.Offset > 0 {
		params.Set("offset", strconv.Itoa(req.Offset))
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Category != "" {
		params.Set("category", req.Category)
	}
	if req.Domain != "" {
		params.Set("domain", req.Domain)
	}
	if req.Scope != "" {
		params.Set("scope", req.Scope)
	}
	if req.Status != "" {
		params.Set("status", req.Status)
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
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.Name)

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse tool response", errors.InternalError)
	}

	return &ToolResponse{ToolData: &result}, nil
}

func handleToolGetSchema(ctx context.Context, req *ToolGetRequest) (*ToolSchemaResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.Name) + "/schema"

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolSchemaResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse schema response", errors.InternalError)
	}

	return &result, nil
}

func handleToolsSearch(ctx context.Context, req *ToolsSearchRequest) (*ToolsSearchResponse, error) {
	if req.Query == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("search query is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/search"

	body := map[string]interface{}{
		"query": req.Query,
	}
	if req.Limit > 0 {
		body["limit"] = req.Limit
	}

	resp, err := toolsProxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result ToolsSearchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse search response", errors.InternalError)
	}

	return &result, nil
}

func handleToolsDiscover(ctx context.Context, req *ToolsDiscoverRequest) (*ToolsDiscoverResponse, error) {
	if req.Task == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("task description is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/discover"

	body := map[string]interface{}{
		"task": req.Task,
	}
	if req.Limit > 0 {
		body["limit"] = req.Limit
	}
	if req.Domain != "" {
		body["domain"] = req.Domain
	}

	resp, err := toolsProxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result ToolsDiscoverResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse discover response", errors.InternalError)
	}

	return &result, nil
}

func handleToolCreate(ctx context.Context, req *ToolCreateRequest) (*ToolResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool name is required")
	}
	if req.Version == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool version is required")
	}
	if req.Description == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool description is required")
	}
	if req.ProviderType == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("provider type is required")
	}
	if req.ProviderEndpoint == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("provider endpoint is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools"

	body := map[string]interface{}{
		"name":              req.Name,
		"version":           req.Version,
		"description":       req.Description,
		"provider_type":     req.ProviderType,
		"provider_endpoint": req.ProviderEndpoint,
	}
	if req.Category != "" {
		body["category"] = req.Category
	}
	if req.Domain != "" {
		body["domain"] = req.Domain
	}
	if req.Tags != nil {
		body["tags"] = req.Tags
	}
	if req.ProviderTimeoutMs > 0 {
		body["provider_timeout_ms"] = req.ProviderTimeoutMs
	}
	if req.InputSchema != nil {
		body["input_schema"] = req.InputSchema
	}
	if req.OutputSchema != nil {
		body["output_schema"] = req.OutputSchema
	}
	if req.ReadOnlyHint != nil {
		body["read_only_hint"] = *req.ReadOnlyHint
	}
	if req.DestructiveHint != nil {
		body["destructive_hint"] = *req.DestructiveHint
	}
	if req.IdempotentHint != nil {
		body["idempotent_hint"] = *req.IdempotentHint
	}
	if req.OpenWorldHint != nil {
		body["open_world_hint"] = *req.OpenWorldHint
	}
	if req.AccessScope != "" {
		body["access_scope"] = req.AccessScope
	}
	if req.AccessRoles != nil {
		body["access_roles"] = req.AccessRoles
	}
	if req.AccessTeams != nil {
		body["access_teams"] = req.AccessTeams
	}
	if req.AccessUsers != nil {
		body["access_users"] = req.AccessUsers
	}
	if req.Examples != nil {
		body["examples"] = req.Examples
	}
	if req.OwnerType != "" {
		body["owner_type"] = req.OwnerType
	}
	if req.OwnerID != "" {
		body["owner_id"] = req.OwnerID
	}

	resp, err := toolsProxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result ToolData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse create response", errors.InternalError)
	}

	return &ToolResponse{ToolData: &result}, nil
}

func handleToolUpdate(ctx context.Context, req *ToolUpdateRequest) (*ToolResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.Name)

	body := map[string]interface{}{}
	if req.Version != "" {
		body["version"] = req.Version
	}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.Category != "" {
		body["category"] = req.Category
	}
	if req.Domain != "" {
		body["domain"] = req.Domain
	}
	if req.Tags != nil {
		body["tags"] = req.Tags
	}
	if req.ProviderType != "" {
		body["provider_type"] = req.ProviderType
	}
	if req.ProviderEndpoint != "" {
		body["provider_endpoint"] = req.ProviderEndpoint
	}
	if req.ProviderTimeoutMs > 0 {
		body["provider_timeout_ms"] = req.ProviderTimeoutMs
	}
	if req.InputSchema != nil {
		body["input_schema"] = req.InputSchema
	}
	if req.OutputSchema != nil {
		body["output_schema"] = req.OutputSchema
	}
	if req.Status != "" {
		body["status"] = req.Status
	}

	resp, err := toolsProxyPut(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result ToolData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse update response", errors.InternalError)
	}

	return &ToolResponse{ToolData: &result}, nil
}

func handleToolDelete(ctx context.Context, req *ToolGetRequest) (*ToolsMessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.Name)

	_, err := toolsProxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	return &ToolsMessageResponse{Message: "tool unregistered successfully"}, nil
}

func handleToolInvoke(ctx context.Context, req *ToolInvokeRequest) (*ToolInvokeResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.Name) + "/invoke"

	body := map[string]interface{}{}
	if req.Input != nil {
		body["input"] = req.Input
	}
	if req.UserID != "" {
		body["user_id"] = req.UserID
	}

	resp, err := toolsProxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result ToolInvokeResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse invoke response", errors.InternalError)
	}

	return &result, nil
}

func handleToolAnalytics(ctx context.Context, req *ToolGetRequest) (*ToolAnalyticsResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("tool name is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/tools/" + url.PathEscape(req.Name) + "/analytics"

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result ToolAnalyticsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse analytics response", errors.InternalError)
	}

	return &result, nil
}

func handleDomainsList(ctx context.Context, req *DomainsListRequest) (*DomainsListResponse, error) {
	reqURL := toolsRepositoryURL + "/api/v1/domains"

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result DomainsListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse domains list response", errors.InternalError)
	}

	return &result, nil
}

func handleDomainTools(ctx context.Context, req *DomainGetRequest) (*DomainToolsResponse, error) {
	if req.Domain == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("domain is required")
	}

	reqURL := toolsRepositoryURL + "/api/v1/domains/" + url.PathEscape(req.Domain) + "/tools"

	resp, err := toolsProxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result DomainToolsResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse domain tools response", errors.InternalError)
	}

	return &result, nil
}

// ======================== HTTP Proxy Helpers ========================

func toolsProxyGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}

	resp, err := toolsHTTPClient.Do(req)
	if err != nil {
		return nil, errors.WrapError(err, "failed to call tools-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("tools-repository error: %s", string(body)))
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
		return nil, errors.WrapError(err, "failed to call tools-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("tools-repository error: %s", string(body)))
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
		return nil, errors.WrapError(err, "failed to call tools-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("tools-repository error: %s", string(body)))
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
		return nil, errors.WrapError(err, "failed to call tools-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to read response", errors.InternalError)
	}

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("tools-repository error: %s", string(body)))
	}

	return body, nil
}
