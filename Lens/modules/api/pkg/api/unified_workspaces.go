// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
)

func init() {
	// Workspace CRUD endpoints
	unified.Register(&unified.EndpointDef[WorkspacesListRequest, WorkspacesListResponse]{
		Name:        "workspaces_list",
		Description: "List all workspaces with pagination and filtering",
		HTTPMethod:  "GET",
		HTTPPath:    "/workspaces",
		MCPToolName: "lens_workspaces_list",
		Handler:     handleWorkspacesList,
	})

	unified.Register(&unified.EndpointDef[WorkspaceGetRequest, WorkspaceResponse]{
		Name:        "workspaces_get",
		Description: "Get a specific workspace by name",
		HTTPMethod:  "GET",
		HTTPPath:    "/workspaces/:name",
		MCPToolName: "lens_workspaces_get",
		Handler:     handleWorkspaceGet,
	})

	unified.Register(&unified.EndpointDef[WorkspaceCreateRequest, WorkspaceResponse]{
		Name:        "workspaces_create",
		Description: "Create a new workspace",
		HTTPMethod:  "POST",
		HTTPPath:    "/workspaces",
		MCPToolName: "lens_workspaces_create",
		Handler:     handleWorkspaceCreate,
	})

	unified.Register(&unified.EndpointDef[WorkspaceUpdateRequest, WorkspaceResponse]{
		Name:        "workspaces_update",
		Description: "Update an existing workspace",
		HTTPMethod:  "PUT",
		HTTPPath:    "/workspaces/:name",
		MCPToolName: "lens_workspaces_update",
		Handler:     handleWorkspaceUpdate,
	})

	unified.Register(&unified.EndpointDef[WorkspaceGetRequest, MessageResponse]{
		Name:        "workspaces_delete",
		Description: "Delete a workspace by name",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/workspaces/:name",
		MCPToolName: "lens_workspaces_delete",
		Handler:     handleWorkspaceDelete,
	})

	// Workspace-Skill management endpoints
	unified.Register(&unified.EndpointDef[WorkspaceSkillsListRequest, WorkspaceSkillsListResponse]{
		Name:        "workspace_skills_list",
		Description: "List skills in a workspace",
		HTTPMethod:  "GET",
		HTTPPath:    "/workspaces/:name/skills",
		MCPToolName: "lens_workspace_skills_list",
		Handler:     handleWorkspaceSkillsList,
	})

	unified.Register(&unified.EndpointDef[WorkspaceSkillsModifyRequest, MessageResponse]{
		Name:        "workspace_skills_add",
		Description: "Add skills to a workspace",
		HTTPMethod:  "POST",
		HTTPPath:    "/workspaces/:name/skills",
		MCPToolName: "lens_workspace_skills_add",
		Handler:     handleWorkspaceSkillsAdd,
	})

	unified.Register(&unified.EndpointDef[WorkspaceSkillsModifyRequest, MessageResponse]{
		Name:        "workspace_skills_remove",
		Description: "Remove skills from a workspace",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/workspaces/:name/skills",
		MCPToolName: "lens_workspace_skills_remove",
		Handler:     handleWorkspaceSkillsRemove,
	})

	unified.Register(&unified.EndpointDef[WorkspaceSearchRequest, WorkspaceSearchResponse]{
		Name:        "workspace_skills_search",
		Description: "Semantic search for skills within a workspace",
		HTTPMethod:  "POST",
		HTTPPath:    "/workspaces/:name/skills/search",
		MCPToolName: "lens_workspace_skills_search",
		Handler:     handleWorkspaceSkillsSearch,
	})
}

// ======================== Request Types ========================

type WorkspacesListRequest struct {
	Offset int    `json:"offset" query:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit  int    `json:"limit" query:"limit" mcp:"description=Number of items per page (default: 50)"`
	Owner  string `json:"owner" query:"owner" mcp:"description=Filter by owner"`
}

type WorkspaceGetRequest struct {
	Name string `json:"name" param:"name" mcp:"description=Workspace name,required"`
}

type WorkspaceCreateRequest struct {
	Name        string            `json:"name" binding:"required" mcp:"description=Workspace name,required"`
	Description string            `json:"description" mcp:"description=Workspace description"`
	Owner       string            `json:"owner" mcp:"description=Workspace owner"`
	IsDefault   bool              `json:"is_default" mcp:"description=Set as default workspace"`
	Metadata    map[string]string `json:"metadata" mcp:"description=Additional metadata"`
}

type WorkspaceUpdateRequest struct {
	Name        string            `json:"name" param:"name" mcp:"description=Workspace name,required"`
	Description string            `json:"description" mcp:"description=Workspace description"`
	Owner       string            `json:"owner" mcp:"description=Workspace owner"`
	IsDefault   bool              `json:"is_default" mcp:"description=Set as default workspace"`
	Metadata    map[string]string `json:"metadata" mcp:"description=Additional metadata"`
}

type WorkspaceSkillsListRequest struct {
	Name   string `json:"name" param:"name" mcp:"description=Workspace name,required"`
	Offset int    `json:"offset" query:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit  int    `json:"limit" query:"limit" mcp:"description=Number of items per page (default: 50)"`
}

type WorkspaceSkillsModifyRequest struct {
	Name   string   `json:"name" param:"name" mcp:"description=Workspace name,required"`
	Skills []string `json:"skills" binding:"required" mcp:"description=List of skill names to add/remove,required"`
}

type WorkspaceSearchRequest struct {
	Name  string `json:"name" param:"name" mcp:"description=Workspace name,required"`
	Query string `json:"query" binding:"required" mcp:"description=Natural language search query,required"`
	Limit int    `json:"limit" mcp:"description=Maximum number of results (default: 10)"`
}

// ======================== Response Types ========================

type WorkspaceData struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Owner       string            `json:"owner"`
	IsDefault   bool              `json:"is_default"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type WorkspacesListResponse struct {
	Workspaces []*WorkspaceData `json:"workspaces"`
	Total      int64            `json:"total"`
	Offset     int              `json:"offset"`
	Limit      int              `json:"limit"`
}

type WorkspaceResponse struct {
	*WorkspaceData
}

type WorkspaceSkillsListResponse struct {
	Skills []*SkillData `json:"skills"`
	Total  int64        `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
}

type WorkspaceSearchResponse struct {
	Skills    []*SkillSearchResult `json:"skills"`
	Total     int                  `json:"total"`
	Workspace string               `json:"workspace"`
	Hint      string               `json:"hint"`
}

// ======================== Handler Implementations ========================

func handleWorkspacesList(ctx context.Context, req *WorkspacesListRequest) (*WorkspacesListResponse, error) {
	params := url.Values{}
	if req.Offset > 0 {
		params.Set("offset", strconv.Itoa(req.Offset))
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Owner != "" {
		params.Set("owner", req.Owner)
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result WorkspacesListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse workspaces list response", errors.InternalError)
	}

	return &result, nil
}

func handleWorkspaceGet(ctx context.Context, req *WorkspaceGetRequest) (*WorkspaceResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces/" + url.PathEscape(req.Name)

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result WorkspaceData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse workspace response", errors.InternalError)
	}

	return &WorkspaceResponse{WorkspaceData: &result}, nil
}

func handleWorkspaceCreate(ctx context.Context, req *WorkspaceCreateRequest) (*WorkspaceResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces"

	body := map[string]interface{}{
		"name": req.Name,
	}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.Owner != "" {
		body["owner"] = req.Owner
	}
	body["is_default"] = req.IsDefault
	if req.Metadata != nil {
		body["metadata"] = req.Metadata
	}

	resp, err := proxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result WorkspaceData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse create response", errors.InternalError)
	}

	return &WorkspaceResponse{WorkspaceData: &result}, nil
}

func handleWorkspaceUpdate(ctx context.Context, req *WorkspaceUpdateRequest) (*WorkspaceResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces/" + url.PathEscape(req.Name)

	body := map[string]interface{}{}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.Owner != "" {
		body["owner"] = req.Owner
	}
	body["is_default"] = req.IsDefault
	if req.Metadata != nil {
		body["metadata"] = req.Metadata
	}

	resp, err := proxyPut(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result WorkspaceData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse update response", errors.InternalError)
	}

	return &WorkspaceResponse{WorkspaceData: &result}, nil
}

func handleWorkspaceDelete(ctx context.Context, req *WorkspaceGetRequest) (*MessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces/" + url.PathEscape(req.Name)

	_, err := proxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	return &MessageResponse{Message: "workspace deleted successfully"}, nil
}

func handleWorkspaceSkillsList(ctx context.Context, req *WorkspaceSkillsListRequest) (*WorkspaceSkillsListResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}

	params := url.Values{}
	if req.Offset > 0 {
		params.Set("offset", strconv.Itoa(req.Offset))
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces/" + url.PathEscape(req.Name) + "/skills"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result WorkspaceSkillsListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse workspace skills list response", errors.InternalError)
	}

	return &result, nil
}

func handleWorkspaceSkillsAdd(ctx context.Context, req *WorkspaceSkillsModifyRequest) (*MessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}
	if len(req.Skills) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skills list is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces/" + url.PathEscape(req.Name) + "/skills"

	body := map[string]interface{}{
		"skills": req.Skills,
	}

	_, err := proxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	return &MessageResponse{Message: "skills added to workspace"}, nil
}

func handleWorkspaceSkillsRemove(ctx context.Context, req *WorkspaceSkillsModifyRequest) (*MessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}
	if len(req.Skills) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skills list is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces/" + url.PathEscape(req.Name) + "/skills"

	body := map[string]interface{}{
		"skills": req.Skills,
	}

	// Use DELETE with body
	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, errors.WrapError(err, "failed to marshal request body", errors.InternalError)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, errors.WrapError(err, "failed to call skills-repository", errors.InternalError)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, errors.NewError().WithCode(errors.InternalError).WithMessage(fmt.Sprintf("skills-repository error: status %d", resp.StatusCode))
	}

	return &MessageResponse{Message: "skills removed from workspace"}, nil
}

func handleWorkspaceSkillsSearch(ctx context.Context, req *WorkspaceSearchRequest) (*WorkspaceSearchResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("workspace name is required")
	}
	if req.Query == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("search query is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/workspaces/" + url.PathEscape(req.Name) + "/skills/search"

	body := map[string]interface{}{
		"query": req.Query,
	}
	if req.Limit > 0 {
		body["limit"] = req.Limit
	}

	resp, err := proxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result WorkspaceSearchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse search response", errors.InternalError)
	}

	return &result, nil
}

