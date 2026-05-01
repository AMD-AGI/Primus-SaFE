// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

var toolNameRegexp = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func findTool(t *testing.T, name string) *mcpserver.MCPTool {
	t.Helper()
	for _, tl := range RegisterAllTools() {
		if tl.Name == name {
			return tl
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}

func TestRegisterAllTools_Count(t *testing.T) {
	t.Parallel()
	if n := len(RegisterAllTools()); n != 47 {
		t.Fatalf("RegisterAllTools: got %d tools want 47", n)
	}
}

func TestRegisterAllTools_NamesUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]int)
	for _, tl := range RegisterAllTools() {
		seen[tl.Name]++
	}
	for name, c := range seen {
		if c != 1 {
			t.Fatalf("duplicate tool name %q count %d", name, c)
		}
	}
}

func TestRegisterAllTools_SchemaValid(t *testing.T) {
	t.Parallel()
	for _, tl := range RegisterAllTools() {
		if tl.Name == "" {
			t.Fatal("empty name")
		}
		if !toolNameRegexp.MatchString(tl.Name) {
			t.Errorf("tool %q name does not match ^[a-z][a-z0-9_]*$", tl.Name)
		}
		if strings.TrimSpace(tl.Description) == "" {
			t.Errorf("tool %q: empty description", tl.Name)
		}
		if tl.Handler == nil {
			t.Errorf("tool %q: nil handler", tl.Name)
		}
		schema := tl.InputSchema
		if schema == nil {
			t.Fatalf("tool %q: nil InputSchema", tl.Name)
		}
		typ, _ := schema["type"].(string)
		if typ != "object" {
			t.Errorf("tool %q: InputSchema type=%q want object", tl.Name, typ)
		}
		propsAny, ok := schema["properties"]
		if !ok {
			t.Errorf("tool %q: missing properties", tl.Name)
			continue
		}
		props, ok := propsAny.(map[string]any)
		if !ok {
			t.Errorf("tool %q: properties not map[string]any", tl.Name)
			continue
		}
		if reqAny, has := schema["required"]; has {
			reqNames := extractRequiredStrings(reqAny)
			if reqNames == nil {
				t.Errorf("tool %q: required has wrong type %T", tl.Name, reqAny)
				continue
			}
			for _, r := range reqNames {
				if _, ok := props[r]; !ok {
					t.Errorf("tool %q: required field %q not in properties", tl.Name, r)
				}
			}
		}
		_ = props
	}
}

func extractRequiredStrings(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
}

func TestRegisterAllTools_GroupCounts(t *testing.T) {
	t.Parallel()
	counts := map[string]int{
		"cluster_":   0,
		"workspace_": 0,
		"flavor_":    0,
		"node_":      0,
		"workload_":  0,
		"opsjob_":    0,
		"apikey_":    0,
	}
	want := map[string]int{
		"cluster_":   7,
		"workspace_": 7,
		"flavor_":    2,
		"node_":      12,
		"workload_":  10,
		"opsjob_":    5,
		"apikey_":    4,
	}
	for _, tl := range RegisterAllTools() {
		for prefix := range counts {
			if strings.HasPrefix(tl.Name, prefix) {
				counts[prefix]++
				break
			}
		}
	}
	for prefix, w := range want {
		if got := counts[prefix]; got != w {
			t.Errorf("prefix %s: got %d want %d", prefix, got, w)
		}
	}
}

func TestTool_HappyPath_GET(t *testing.T) {
	t.Parallel()
	sample := `{"clusters":[{"id":"c1"}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		if want := "/" + common.PrimusRouterCustomRootPath + "/clusters"; r.URL.Path != want {
			t.Errorf("path got %q want %q", r.URL.Path, want)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sample))
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	tool := findTool(t, "cluster_list")
	got, err := tool.Handler(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var exp any
	if err := json.Unmarshal([]byte(sample), &exp); err != nil {
		t.Fatal(err)
	}
	if !fmtDeepEqualJSON(got, exp) {
		t.Fatalf("got %#v want %#v", got, exp)
	}
}

func fmtDeepEqualJSON(a, b any) bool {
	// Normalize via JSON round-trip for stable comparison.
	ja, err1 := json.Marshal(a)
	jb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ja) == string(jb)
}

func TestTool_HappyPath_GETWithPath(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		want := "/" + common.PrimusRouterCustomRootPath + "/clusters/test-cluster"
		if r.URL.Path != want {
			t.Errorf("path got %q want %q", r.URL.Path, want)
		}
		_, _ = w.Write([]byte(`{"id":"test-cluster"}`))
	}))
	defer srv.Close()

	raw := json.RawMessage(`{"cluster_id":"test-cluster"}`)
	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	tool := findTool(t, "cluster_get")
	got, err := tool.Handler(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	exp := map[string]any{"id": "test-cluster"}
	if !fmtDeepEqualJSON(got, exp) {
		t.Fatalf("got %#v", got)
	}
}

func TestTool_HappyPath_POSTWithBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		if want := "/" + common.PrimusRouterCustomRootPath + "/workspaces"; r.URL.Path != want {
			t.Errorf("path got %q want %q", r.URL.Path, want)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"name", "cluster_id", "flavor_id"} {
			if _, ok := body[k]; !ok {
				t.Errorf("body missing %q: %#v", k, body)
			}
		}
		_, _ = w.Write([]byte(`{"workspace_id":"ws-1"}`))
	}))
	defer srv.Close()

	raw := json.RawMessage(`{"name":"w1","cluster_id":"c1","flavor_id":"f1"}`)
	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	tool := findTool(t, "workspace_create")
	got, err := tool.Handler(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	exp := map[string]any{"workspace_id": "ws-1"}
	if !fmtDeepEqualJSON(got, exp) {
		t.Fatalf("got %#v", got)
	}
}

func TestTool_QueryParamMapping(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method %s", r.Method)
		}
		q := r.URL.Query()
		if got := q.Get("clusterId"); got != "my-cluster" {
			t.Fatalf("query clusterId=%q full %q", got, r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	raw := json.RawMessage(`{"cluster_id":"my-cluster"}`)
	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	tool := findTool(t, "node_list")
	if _, err := tool.Handler(ctx, raw); err != nil {
		t.Fatal(err)
	}
}

// minimalArgsForTool returns minimum required JSON args for a tool name so the
// Handler can be exercised end-to-end. Empty raw is fine for tools without
// required fields.
func minimalArgsForTool(name string) json.RawMessage {
	args := map[string]any{}
	// Cover every "required" field referenced by InputSchema across the 47 tools.
	defaults := map[string]any{
		"cluster_id":   "test-cluster",
		"workspace_id": "ws-1",
		"node_id":      "node-1",
		"workload_id":  "wl-1",
		"job_id":       "job-1",
		"flavor_id":    "flavor-1",
		"pod_id":       "pod-1",
		"name":         "name-1",
		"action":       "add",
		"node_ids":     []string{"node-1"},
		"workload_ids": []string{"wl-1"},
		"images":       []string{"img:latest"},
		"resources":    []map[string]any{{"cpu": "1", "memory": "1Gi", "replica": 1}},
		"kind":         "Deployment",
		"display_name": "wl-display",
		"replica":      1,
		"ttl_days":     7,
		"apikey_id":    1,
		"type":         "preflight",
		"inputs":       []map[string]any{{"name": "node", "value": "node-1"}},
		"ssh_secret_id":         "ssh-1",
		"kube_spray_image":      "spray:1",
		"kube_pods_subnet":      "10.0.0.0/16",
		"kube_service_address":  "10.96.0.0/16",
		"kube_version":          "1.32.5",
		"nodes":                 []string{"node-1"},
		"private_ip":            "10.0.0.1",
		"template_id":           "tmpl-1",
	}
	for k, v := range defaults {
		args[k] = v
	}
	raw, _ := json.Marshal(args)
	return raw
}

// TestAllTools_ReachableViaHandler runs every registered tool's Handler against
// a permissive loopback server. The goal is coverage and a basic smoke-check
// that no Handler panics, mis-builds a URL, or fails to parse params, NOT to
// validate per-tool behavior. Per-tool semantics are covered elsewhere.
func TestAllTools_ReachableViaHandler(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)

	for _, tl := range RegisterAllTools() {
		tl := tl
		t.Run(tl.Name, func(t *testing.T) {
			args := minimalArgsForTool(tl.Name)
			if _, err := tl.Handler(ctx, args); err != nil {
				t.Fatalf("tool %q handler error: %v", tl.Name, err)
			}
		})
	}
}

func TestTool_RequiresContext(t *testing.T) {
	t.Parallel()
	tool := findTool(t, "cluster_list")
	_, err := tool.Handler(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error without HTTP request in context")
	}
	if !strings.Contains(err.Error(), "incoming HTTP request") {
		t.Fatalf("unexpected error: %v", err)
	}
}
