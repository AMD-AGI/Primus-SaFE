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
	// Skillset CRUD endpoints
	unified.Register(&unified.EndpointDef[SkillsetsListRequest, SkillsetsListResponse]{
		Name:        "skillsets_list",
		Description: "List all skillsets with pagination and filtering",
		HTTPMethod:  "GET",
		HTTPPath:    "/skillsets",
		MCPToolName: "lens_skillsets_list",
		Handler:     handleSkillsetsList,
	})

	unified.Register(&unified.EndpointDef[SkillsetGetRequest, SkillsetResponse]{
		Name:        "skillsets_get",
		Description: "Get a specific skillset by name",
		HTTPMethod:  "GET",
		HTTPPath:    "/skillsets/:name",
		MCPToolName: "lens_skillsets_get",
		Handler:     handleSkillsetGet,
	})

	unified.Register(&unified.EndpointDef[SkillsetCreateRequest, SkillsetResponse]{
		Name:        "skillsets_create",
		Description: "Create a new skillset",
		HTTPMethod:  "POST",
		HTTPPath:    "/skillsets",
		MCPToolName: "lens_skillsets_create",
		Handler:     handleSkillsetCreate,
	})

	unified.Register(&unified.EndpointDef[SkillsetUpdateRequest, SkillsetResponse]{
		Name:        "skillsets_update",
		Description: "Update an existing skillset",
		HTTPMethod:  "PUT",
		HTTPPath:    "/skillsets/:name",
		MCPToolName: "lens_skillsets_update",
		Handler:     handleSkillsetUpdate,
	})

	unified.Register(&unified.EndpointDef[SkillsetGetRequest, MessageResponse]{
		Name:        "skillsets_delete",
		Description: "Delete a skillset by name",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/skillsets/:name",
		MCPToolName: "lens_skillsets_delete",
		Handler:     handleSkillsetDelete,
	})

	// Skillset-Skill management endpoints
	unified.Register(&unified.EndpointDef[SkillsetSkillsListRequest, SkillsetSkillsListResponse]{
		Name:        "skillset_skills_list",
		Description: "List skills in a skillset",
		HTTPMethod:  "GET",
		HTTPPath:    "/skillsets/:name/skills",
		MCPToolName: "lens_skillset_skills_list",
		Handler:     handleSkillsetSkillsList,
	})

	unified.Register(&unified.EndpointDef[SkillsetSkillsModifyRequest, MessageResponse]{
		Name:        "skillset_skills_add",
		Description: "Add skills to a skillset",
		HTTPMethod:  "POST",
		HTTPPath:    "/skillsets/:name/skills",
		MCPToolName: "lens_skillset_skills_add",
		Handler:     handleSkillsetSkillsAdd,
	})

	unified.Register(&unified.EndpointDef[SkillsetSkillsModifyRequest, MessageResponse]{
		Name:        "skillset_skills_remove",
		Description: "Remove skills from a skillset",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/skillsets/:name/skills",
		MCPToolName: "lens_skillset_skills_remove",
		Handler:     handleSkillsetSkillsRemove,
	})

	unified.Register(&unified.EndpointDef[SkillsetSearchRequest, SkillsetSearchResponse]{
		Name:        "skillset_skills_search",
		Description: "Semantic search for skills within a skillset",
		HTTPMethod:  "POST",
		HTTPPath:    "/skillsets/:name/skills/search",
		MCPToolName: "lens_skillset_skills_search",
		Handler:     handleSkillsetSkillsSearch,
	})
}

// ======================== Request Types ========================

type SkillsetsListRequest struct {
	Offset int    `json:"offset" query:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit  int    `json:"limit" query:"limit" mcp:"description=Number of items per page (default: 50)"`
	Owner  string `json:"owner" query:"owner" mcp:"description=Filter by owner"`
}

type SkillsetGetRequest struct {
	Name string `json:"name" param:"name" mcp:"description=Skillset name,required"`
}

type SkillsetCreateRequest struct {
	Name        string            `json:"name" binding:"required" mcp:"description=Skillset name,required"`
	Description string            `json:"description" mcp:"description=Skillset description"`
	Owner       string            `json:"owner" mcp:"description=Skillset owner"`
	IsDefault   bool              `json:"is_default" mcp:"description=Set as default skillset"`
	Metadata    map[string]string `json:"metadata" mcp:"description=Additional metadata"`
}

type SkillsetUpdateRequest struct {
	Name        string            `json:"name" param:"name" mcp:"description=Skillset name,required"`
	Description string            `json:"description" mcp:"description=Skillset description"`
	Owner       string            `json:"owner" mcp:"description=Skillset owner"`
	IsDefault   bool              `json:"is_default" mcp:"description=Set as default skillset"`
	Metadata    map[string]string `json:"metadata" mcp:"description=Additional metadata"`
}

type SkillsetSkillsListRequest struct {
	Name   string `json:"name" param:"name" mcp:"description=Skillset name,required"`
	Offset int    `json:"offset" query:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit  int    `json:"limit" query:"limit" mcp:"description=Number of items per page (default: 50)"`
}

type SkillsetSkillsModifyRequest struct {
	Name   string   `json:"name" param:"name" mcp:"description=Skillset name,required"`
	Skills []string `json:"skills" binding:"required" mcp:"description=List of skill names to add/remove,required"`
}

type SkillsetSearchRequest struct {
	Name  string `json:"name" param:"name" mcp:"description=Skillset name,required"`
	Query string `json:"query" binding:"required" mcp:"description=Natural language search query,required"`
	Limit int    `json:"limit" mcp:"description=Maximum number of results (default: 10)"`
}

// ======================== Response Types ========================

type SkillsetData struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Owner       string            `json:"owner"`
	IsDefault   bool              `json:"is_default"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type SkillsetsListResponse struct {
	Skillsets []*SkillsetData `json:"skillsets"`
	Total     int64           `json:"total"`
	Offset    int             `json:"offset"`
	Limit     int             `json:"limit"`
}

type SkillsetResponse struct {
	*SkillsetData
}

type SkillsetSkillsListResponse struct {
	Skills []*SkillData `json:"skills"`
	Total  int64        `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
}

type SkillsetSearchResponse struct {
	Skills   []*SkillSearchResult `json:"skills"`
	Total    int                  `json:"total"`
	Skillset string               `json:"skillset"`
	Hint     string               `json:"hint"`
}

// ======================== Handler Implementations ========================

func handleSkillsetsList(ctx context.Context, req *SkillsetsListRequest) (*SkillsetsListResponse, error) {
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

	reqURL := skillsRepositoryURL + "/api/v1/skillsets"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result SkillsetsListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse skillsets list response", errors.InternalError)
	}

	return &result, nil
}

func handleSkillsetGet(ctx context.Context, req *SkillsetGetRequest) (*SkillsetResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets/" + url.PathEscape(req.Name)

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result SkillsetData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse skillset response", errors.InternalError)
	}

	return &SkillsetResponse{SkillsetData: &result}, nil
}

func handleSkillsetCreate(ctx context.Context, req *SkillsetCreateRequest) (*SkillsetResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets"

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

	var result SkillsetData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse create response", errors.InternalError)
	}

	return &SkillsetResponse{SkillsetData: &result}, nil
}

func handleSkillsetUpdate(ctx context.Context, req *SkillsetUpdateRequest) (*SkillsetResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets/" + url.PathEscape(req.Name)

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

	var result SkillsetData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse update response", errors.InternalError)
	}

	return &SkillsetResponse{SkillsetData: &result}, nil
}

func handleSkillsetDelete(ctx context.Context, req *SkillsetGetRequest) (*MessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets/" + url.PathEscape(req.Name)

	_, err := proxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	return &MessageResponse{Message: "skillset deleted successfully"}, nil
}

func handleSkillsetSkillsList(ctx context.Context, req *SkillsetSkillsListRequest) (*SkillsetSkillsListResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}

	params := url.Values{}
	if req.Offset > 0 {
		params.Set("offset", strconv.Itoa(req.Offset))
	}
	if req.Limit > 0 {
		params.Set("limit", strconv.Itoa(req.Limit))
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets/" + url.PathEscape(req.Name) + "/skills"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result SkillsetSkillsListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse skillset skills list response", errors.InternalError)
	}

	return &result, nil
}

func handleSkillsetSkillsAdd(ctx context.Context, req *SkillsetSkillsModifyRequest) (*MessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}
	if len(req.Skills) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skills list is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets/" + url.PathEscape(req.Name) + "/skills"

	body := map[string]interface{}{
		"skills": req.Skills,
	}

	_, err := proxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	return &MessageResponse{Message: "skills added to skillset"}, nil
}

func handleSkillsetSkillsRemove(ctx context.Context, req *SkillsetSkillsModifyRequest) (*MessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}
	if len(req.Skills) == 0 {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skills list is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets/" + url.PathEscape(req.Name) + "/skills"

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

	return &MessageResponse{Message: "skills removed from skillset"}, nil
}

func handleSkillsetSkillsSearch(ctx context.Context, req *SkillsetSearchRequest) (*SkillsetSearchResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skillset name is required")
	}
	if req.Query == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("search query is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skillsets/" + url.PathEscape(req.Name) + "/skills/search"

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

	var result SkillsetSearchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse search response", errors.InternalError)
	}

	return &result, nil
}
