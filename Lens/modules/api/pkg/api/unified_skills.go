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

var skillsRepositoryURL string

func init() {
	// Get skills-repository service URL from environment or use default
	skillsRepositoryURL = os.Getenv("SKILLS_REPOSITORY_URL")
	if skillsRepositoryURL == "" {
		skillsRepositoryURL = "http://skills-repository:8092"
	}

	// Skills Repository endpoints - proxy to skills-repository service
	unified.Register(&unified.EndpointDef[SkillsListRequest, SkillsListResponse]{
		Name:        "skills_list",
		Description: "List all skills with pagination and filtering",
		HTTPMethod:  "GET",
		HTTPPath:    "/skills",
		MCPToolName: "lens_skills_list",
		Handler:     handleSkillsList,
	})

	unified.Register(&unified.EndpointDef[SkillGetRequest, SkillResponse]{
		Name:        "skills_get",
		Description: "Get a specific skill by name",
		HTTPMethod:  "GET",
		HTTPPath:    "/skills/:name",
		MCPToolName: "lens_skills_get",
		Handler:     handleSkillGet,
	})

	unified.Register(&unified.EndpointDef[SkillGetRequest, SkillContentResponse]{
		Name:        "skills_get_content",
		Description: "Get the full SKILL.md content for a skill",
		HTTPMethod:  "GET",
		HTTPPath:    "/skills/:name/content",
		MCPToolName: "lens_skills_get_content",
		Handler:     handleSkillGetContent,
	})

	unified.Register(&unified.EndpointDef[SkillsSearchRequest, SkillsSearchResponse]{
		Name:        "skills_search",
		Description: "Semantic search for skills using natural language query",
		HTTPMethod:  "POST",
		HTTPPath:    "/skills/search",
		MCPToolName: "lens_skills_search",
		Handler:     handleSkillsSearch,
	})

	unified.Register(&unified.EndpointDef[SkillCreateRequest, SkillResponse]{
		Name:        "skills_create",
		Description: "Create a new skill",
		HTTPMethod:  "POST",
		HTTPPath:    "/skills",
		MCPToolName: "lens_skills_create",
		Handler:     handleSkillCreate,
	})

	unified.Register(&unified.EndpointDef[SkillUpdateRequest, SkillResponse]{
		Name:        "skills_update",
		Description: "Update an existing skill",
		HTTPMethod:  "PUT",
		HTTPPath:    "/skills/:name",
		MCPToolName: "lens_skills_update",
		Handler:     handleSkillUpdate,
	})

	unified.Register(&unified.EndpointDef[SkillGetRequest, MessageResponse]{
		Name:        "skills_delete",
		Description: "Delete a skill by name",
		HTTPMethod:  "DELETE",
		HTTPPath:    "/skills/:name",
		MCPToolName: "lens_skills_delete",
		Handler:     handleSkillDelete,
	})

	unified.Register(&unified.EndpointDef[SkillImportGitHubRequest, SkillImportResponse]{
		Name:        "skills_import_github",
		Description: "Import skills from a GitHub repository",
		HTTPMethod:  "POST",
		HTTPPath:    "/skills/import/github",
		MCPToolName: "lens_skills_import_github",
		Handler:     handleSkillImportGitHub,
	})
}

// ======================== Request Types ========================

type SkillsListRequest struct {
	Offset   int    `json:"offset" query:"offset" mcp:"description=Pagination offset (default: 0)"`
	Limit    int    `json:"limit" query:"limit" mcp:"description=Number of items per page (default: 50)"`
	Category string `json:"category" query:"category" mcp:"description=Filter by category"`
	Source   string `json:"source" query:"source" mcp:"description=Filter by source (manual/git/local)"`
}

type SkillGetRequest struct {
	Name string `json:"name" param:"name" mcp:"description=Skill name,required"`
}

type SkillsSearchRequest struct {
	Query string `json:"query" binding:"required" mcp:"description=Natural language search query,required"`
	Limit int    `json:"limit" mcp:"description=Maximum number of results (default: 10)"`
}

type SkillCreateRequest struct {
	Name        string            `json:"name" binding:"required" mcp:"description=Skill name,required"`
	Description string            `json:"description" binding:"required" mcp:"description=Skill description,required"`
	Category    string            `json:"category" mcp:"description=Skill category"`
	Version     string            `json:"version" mcp:"description=Skill version"`
	Source      string            `json:"source" mcp:"description=Skill source (manual/git/local)"`
	License     string            `json:"license" mcp:"description=License information"`
	Content     string            `json:"content" mcp:"description=Full SKILL.md content"`
	FilePath    string            `json:"file_path" mcp:"description=File path for local skills"`
	Metadata    map[string]string `json:"metadata" mcp:"description=Additional metadata"`
}

type SkillUpdateRequest struct {
	Name        string            `json:"name" param:"name" mcp:"description=Skill name,required"`
	Description string            `json:"description" mcp:"description=Skill description"`
	Category    string            `json:"category" mcp:"description=Skill category"`
	Version     string            `json:"version" mcp:"description=Skill version"`
	License     string            `json:"license" mcp:"description=License information"`
	Content     string            `json:"content" mcp:"description=Full SKILL.md content"`
	Metadata    map[string]string `json:"metadata" mcp:"description=Additional metadata"`
}

type SkillImportGitHubRequest struct {
	URL         string `json:"url" binding:"required" mcp:"description=GitHub repository URL (e.g. https://github.com/owner/repo or https://github.com/owner/repo/tree/branch/path),required"`
	GitHubToken string `json:"github_token" mcp:"description=GitHub personal access token for private repositories"`
}

type SkillImportResponse struct {
	Message  string   `json:"message"`
	Imported []string `json:"imported"`
	Skipped  []string `json:"skipped"`
	Errors   []string `json:"errors"`
}

// ======================== Response Types ========================

type SkillData struct {
	ID          int64             `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Category    string            `json:"category"`
	Version     string            `json:"version"`
	Source      string            `json:"source"`
	License     string            `json:"license"`
	Content     string            `json:"content"`
	FilePath    string            `json:"file_path"`
	Metadata    map[string]string `json:"metadata"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type SkillsListResponse struct {
	Skills []*SkillData `json:"skills"`
	Total  int64        `json:"total"`
	Offset int          `json:"offset"`
	Limit  int          `json:"limit"`
}

type SkillResponse struct {
	*SkillData
}

type SkillContentResponse struct {
	Content string `json:"content"`
}

type SkillSearchResult struct {
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	Category       string  `json:"category"`
	RelevanceScore float64 `json:"relevance_score"`
}

type SkillsSearchResponse struct {
	Skills []*SkillSearchResult `json:"skills"`
	Total  int                  `json:"total"`
	Hint   string               `json:"hint"`
}

type MessageResponse struct {
	Message string `json:"message"`
}

// ======================== Handler Implementations ========================

// HTTP client with timeout
var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

func handleSkillsList(ctx context.Context, req *SkillsListRequest) (*SkillsListResponse, error) {
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
	if req.Source != "" {
		params.Set("source", req.Source)
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills"
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result SkillsListResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse skills list response", errors.InternalError)
	}

	return &result, nil
}

func handleSkillGet(ctx context.Context, req *SkillGetRequest) (*SkillResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skill name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills/" + url.PathEscape(req.Name)

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var result SkillData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse skill response", errors.InternalError)
	}

	return &SkillResponse{SkillData: &result}, nil
}

func handleSkillGetContent(ctx context.Context, req *SkillGetRequest) (*SkillContentResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skill name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills/" + url.PathEscape(req.Name) + "/content"

	resp, err := proxyGet(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	return &SkillContentResponse{Content: string(resp)}, nil
}

func handleSkillsSearch(ctx context.Context, req *SkillsSearchRequest) (*SkillsSearchResponse, error) {
	if req.Query == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("search query is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills/search"

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

	var result SkillsSearchResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse search response", errors.InternalError)
	}

	return &result, nil
}

func handleSkillCreate(ctx context.Context, req *SkillCreateRequest) (*SkillResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skill name is required")
	}
	if req.Description == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skill description is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills"

	body := map[string]interface{}{
		"name":        req.Name,
		"description": req.Description,
	}
	if req.Category != "" {
		body["category"] = req.Category
	}
	if req.Version != "" {
		body["version"] = req.Version
	}
	if req.Source != "" {
		body["source"] = req.Source
	} else {
		body["source"] = "manual"
	}
	if req.License != "" {
		body["license"] = req.License
	}
	if req.Content != "" {
		body["content"] = req.Content
	}
	if req.FilePath != "" {
		body["file_path"] = req.FilePath
	}
	if req.Metadata != nil {
		body["metadata"] = req.Metadata
	}

	resp, err := proxyPost(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result SkillData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse create response", errors.InternalError)
	}

	return &SkillResponse{SkillData: &result}, nil
}

func handleSkillUpdate(ctx context.Context, req *SkillUpdateRequest) (*SkillResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skill name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills/" + url.PathEscape(req.Name)

	body := map[string]interface{}{}
	if req.Description != "" {
		body["description"] = req.Description
	}
	if req.Category != "" {
		body["category"] = req.Category
	}
	if req.Version != "" {
		body["version"] = req.Version
	}
	if req.License != "" {
		body["license"] = req.License
	}
	if req.Content != "" {
		body["content"] = req.Content
	}
	if req.Metadata != nil {
		body["metadata"] = req.Metadata
	}

	resp, err := proxyPut(ctx, reqURL, body)
	if err != nil {
		return nil, err
	}

	var result SkillData
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse update response", errors.InternalError)
	}

	return &SkillResponse{SkillData: &result}, nil
}

func handleSkillDelete(ctx context.Context, req *SkillGetRequest) (*MessageResponse, error) {
	if req.Name == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("skill name is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills/" + url.PathEscape(req.Name)

	_, err := proxyDelete(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	return &MessageResponse{Message: "skill deleted successfully"}, nil
}

// ======================== HTTP Proxy Helpers ========================

func proxyGet(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}

	resp, err := httpClient.Do(req)
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

func proxyPost(ctx context.Context, reqURL string, data interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, errors.WrapError(err, "failed to marshal request body", errors.InternalError)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
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

func proxyPut(ctx context.Context, reqURL string, data interface{}) ([]byte, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, errors.WrapError(err, "failed to marshal request body", errors.InternalError)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL, bytes.NewReader(jsonData))
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
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

func proxyDelete(ctx context.Context, reqURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL, nil)
	if err != nil {
		return nil, errors.WrapError(err, "failed to create request", errors.InternalError)
	}

	resp, err := httpClient.Do(req)
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

// ======================== Import Handlers ========================

func handleSkillImportGitHub(ctx context.Context, req *SkillImportGitHubRequest) (*SkillImportResponse, error) {
	if req.URL == "" {
		return nil, errors.NewError().WithCode(errors.RequestParameterInvalid).WithMessage("url is required")
	}

	reqURL := skillsRepositoryURL + "/api/v1/skills/import/github"

	reqBody := map[string]string{
		"url": req.URL,
	}
	if req.GitHubToken != "" {
		reqBody["github_token"] = req.GitHubToken
	}

	resp, err := proxyPost(ctx, reqURL, reqBody)
	if err != nil {
		return nil, err
	}

	var result SkillImportResponse
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, errors.WrapError(err, "failed to parse import response", errors.InternalError)
	}

	return &result, nil
}
