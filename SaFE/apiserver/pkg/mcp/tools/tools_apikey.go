// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

func apikeyTools() []*mcpserver.MCPTool {
	return []*mcpserver.MCPTool{
		apikeyCurrent(), apikeyList(), apikeyCreate(), apikeyDelete(),
	}
}

func apikeyCurrent() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "apikey_current",
		Description: "Get information about the API Key currently being used for authentication",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			_, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			return APICall(ctx, http.MethodGet, "/apikeys/current", nil)
		},
	}
}

func apikeyList() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "apikey_list",
		Description: "List all API Keys for the authenticated user",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  prop("string", "Filter by name (partial match, case-insensitive)"),
				"limit": prop("integer", "Records per page, default 100"),
			},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			q := url.Values{}
			setQuery(q, p, "name", "name")
			setQuery(q, p, "limit", "limit")
			suffix := ""
			if len(q) > 0 {
				suffix = "?" + q.Encode()
			}
			return APICall(ctx, http.MethodGet, "/apikeys"+suffix, nil)
		},
	}
}

func apikeyCreate() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "apikey_create",
		Description: "Create a new API Key. The key value is only returned once during creation - store it securely!",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":      prop("string", "Display name for the API Key (max 100 characters)"),
				"ttl_days":  prop("integer", "Validity period in days (1-366)"),
				"whitelist": propArray(prop("string", ""), "List of allowed IP addresses or CIDR ranges (optional)"),
			},
			"required": []string{"name", "ttl_days"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			if getStr(p, "name") == "" {
				return nil, fmt.Errorf("name is required")
			}
			ttl, ok := getInt(p, "ttl_days")
			if !ok {
				return nil, fmt.Errorf("ttl_days is required")
			}
			body := map[string]any{
				"name":    getStr(p, "name"),
				"ttlDays": ttl,
			}
			if wl, ok := p["whitelist"]; ok && wl != nil {
				body["whitelist"] = wl
			}
			return APICall(ctx, http.MethodPost, "/apikeys", body)
		},
	}
}

func apikeyDelete() *mcpserver.MCPTool {
	return &mcpserver.MCPTool{
		Name:        "apikey_delete",
		Description: "Delete an API Key (soft deletion). The key cannot be recovered after deletion.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"apikey_id": prop("integer", "API Key ID (numeric)"),
			},
			"required": []string{"apikey_id"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (any, error) {
			p, err := parseParams(raw)
			if err != nil {
				return nil, err
			}
			id, ok := getInt(p, "apikey_id")
			if !ok {
				return nil, fmt.Errorf("apikey_id is required")
			}
			path := "/apikeys/" + url.PathEscape(strconv.Itoa(id))
			return APICall(ctx, http.MethodDelete, path, nil)
		},
	}
}
