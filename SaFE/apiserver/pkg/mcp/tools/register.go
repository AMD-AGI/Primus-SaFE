// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

// APICall makes an internal HTTP request to the SaFE REST API, forwarding auth headers from the MCP context.
func APICall(ctx context.Context, method, relPath string, body any) (any, error) {
	inReq, ok := mcpserver.HTTPRequestFromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("tools: incoming HTTP request missing from context; use ContextWithIncomingHTTP")
	}
	scheme := "http"
	if inReq.TLS != nil || strings.EqualFold(inReq.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	base := scheme + "://" + inReq.Host
	apiRoot := strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(common.PrimusRouterCustomRootPath, "/")
	if !strings.HasPrefix(relPath, "/") {
		relPath = "/" + relPath
	}
	fullURL := apiRoot + relPath

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", common.JsonContentType)
	}
	for _, key := range []string{"Authorization", "Cookie", common.UserId, common.UserName} {
		if v := inReq.Header.Get(key); v != "" {
			req.Header.Set(key, v)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("api %s %s: %s: %s", method, relPath, resp.Status, strings.TrimSpace(string(respBody)))
	}
	if len(respBody) == 0 {
		return map[string]any{}, nil
	}
	var out any
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

func parseParams(raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func getStr(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func getInt(m map[string]any, key string) (int, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return int(t), true
	case int:
		return t, true
	case int64:
		return int(t), true
	case json.Number:
		i, err := t.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

func getBool(m map[string]any, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return false, false
	}
	switch t := v.(type) {
	case bool:
		return t, true
	default:
		return false, false
	}
}

func getFloat(m map[string]any, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

// buildQuery builds ?k=v using the same names as keys in p (for params whose query name matches the MCP argument name).
func buildQuery(p map[string]any, keys ...string) string {
	q := url.Values{}
	for _, k := range keys {
		if v, ok := p[k]; ok && v != nil {
			if s, ok := scalarToQuery(v); ok {
				q.Set(k, s)
			}
		}
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

func scalarToQuery(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case float64:
		return strconv.FormatInt(int64(t), 10), true
	case int:
		return strconv.Itoa(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case json.Number:
		return t.String(), true
	default:
		return fmt.Sprint(t), true
	}
}

func setQuery(q url.Values, p map[string]any, paramKey, apiKey string) {
	if v, ok := p[paramKey]; ok && v != nil {
		if s, ok := scalarToQuery(v); ok {
			q.Set(apiKey, s)
		}
	}
}

// RegisterAllTools returns all MCP tools for the SaFE API server.
func RegisterAllTools() []*mcpserver.MCPTool {
	var tools []*mcpserver.MCPTool
	tools = append(tools, clusterTools()...)
	tools = append(tools, workspaceTools()...)
	tools = append(tools, flavorTools()...)
	tools = append(tools, nodeTools()...)
	tools = append(tools, workloadTools()...)
	tools = append(tools, opsjobTools()...)
	tools = append(tools, apikeyTools()...)
	return tools
}

func prop(typ, desc string) map[string]any {
	return map[string]any{"type": typ, "description": desc}
}

func propEnum(typ, desc string, enum []string) map[string]any {
	m := prop(typ, desc)
	m["enum"] = enum
	return m
}

// propArray builds an array schema. Call either propArray(items, desc) or propArray(desc, items) for legacy tools.
func propArray(first any, second any) map[string]any {
	switch f := first.(type) {
	case string:
		items, _ := second.(map[string]any)
		if items == nil {
			items = map[string]any{}
		}
		return map[string]any{"type": "array", "items": items, "description": f}
	case map[string]any:
		desc, _ := second.(string)
		return map[string]any{"type": "array", "items": f, "description": desc}
	default:
		desc, _ := second.(string)
		return map[string]any{"type": "array", "items": map[string]any{}, "description": desc}
	}
}

// propObject builds an object schema. Use propObject(props, desc), or propObject(desc) with a single string for empty properties (legacy).
func propObject(args ...any) map[string]any {
	if len(args) == 1 {
		if desc, ok := args[0].(string); ok {
			return map[string]any{"type": "object", "properties": map[string]any{}, "description": desc}
		}
	}
	props, _ := args[0].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	desc := ""
	if len(args) >= 2 {
		desc, _ = args[1].(string)
	}
	return map[string]any{"type": "object", "properties": props, "description": desc}
}
