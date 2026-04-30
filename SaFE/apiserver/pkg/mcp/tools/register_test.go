// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

func TestAPICall_MissingHTTPRequest(t *testing.T) {
	t.Parallel()
	_, err := APICall(context.Background(), http.MethodGet, "/clusters", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "incoming HTTP request") {
		t.Fatalf("error %q should mention incoming HTTP request", err.Error())
	}
}

func TestAPICall_SuccessfulGET(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method: got %q want GET", r.Method)
		}
		wantPath := "/" + common.PrimusRouterCustomRootPath + "/clusters"
		if r.URL.Path != wantPath {
			t.Errorf("path: got %q want %q", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[1,2],"ok":true}`))
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	got, err := APICall(ctx, http.MethodGet, "/clusters", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"items": []any{float64(1), float64(2)},
		"ok":    true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestAPICall_SuccessfulPOSTWithBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method: got %q want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != common.JsonContentType {
			t.Errorf("Content-Type: got %q want %q", ct, common.JsonContentType)
		}
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			t.Fatalf("body json: %v", err)
		}
		if body["hello"] != "world" {
			t.Fatalf("body: %#v", body)
		}
		_, _ = w.Write([]byte(`{"created":true}`))
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	got, err := APICall(ctx, http.MethodPost, "/clusters", map[string]any{"hello": "world"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, map[string]any{"created": true}) {
		t.Fatalf("got %#v", got)
	}
}

func TestAPICall_ForwardsAuthorization(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got, want := r.Header.Get("Authorization"), "Bearer secret-token"; got != want {
			t.Errorf("Authorization: got %q want %q", got, want)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	req.Header.Set("Authorization", "Bearer secret-token")
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	if _, err := APICall(ctx, http.MethodGet, "/x", nil); err != nil {
		t.Fatal(err)
	}
}

func TestAPICall_Non2xx(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	_, err := APICall(ctx, http.MethodGet, "/clusters", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("error should contain status: %v", err)
	}
}

// TestAPICall_ErrorBodySanitised verifies that raw REST error bodies are
// NOT echoed back to MCP/LLM callers. Only HTTP status and the apiserver's
// errorCode field should reach the caller; secrets/PII in the body must stay
// in server logs.
func TestAPICall_ErrorBodySanitised(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errorCode":"Primus.00007","errorMessage":"sql: SELECT * FROM users WHERE token='abcd'"}`))
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	_, err := APICall(ctx, http.MethodGet, "/secret", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "403") || !strings.Contains(msg, "Primus.00007") {
		t.Fatalf("error should expose status + errorCode, got: %v", err)
	}
	if strings.Contains(msg, "SELECT") || strings.Contains(msg, "abcd") || strings.Contains(msg, "errorMessage") {
		t.Fatalf("raw body must NOT leak to caller, got: %v", err)
	}
}

// TestAPICall_DropsIdentityHeaders verifies that client-controlled identity
// headers (UserId/UserName) are NOT forwarded to the downstream REST API,
// preventing identity spoofing when token validation is optional.
func TestAPICall_DropsIdentityHeaders(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get(common.UserId); v != "" {
			t.Errorf("%s should NOT be forwarded, got %q", common.UserId, v)
		}
		if v := r.Header.Get(common.UserName); v != "" {
			t.Errorf("%s should NOT be forwarded, got %q", common.UserName, v)
		}
		if v := r.Header.Get("Authorization"); v != "Bearer ok" {
			t.Errorf("Authorization should be forwarded, got %q", v)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	req.Header.Set("Authorization", "Bearer ok")
	req.Header.Set(common.UserId, "attacker-uid")
	req.Header.Set(common.UserName, "root")
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	if _, err := APICall(ctx, http.MethodGet, "/x", nil); err != nil {
		t.Fatal(err)
	}
}

func TestAPICall_Empty2xxBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	got, err := APICall(ctx, http.MethodGet, "/clusters", nil)
	if err != nil {
		t.Fatal(err)
	}
	m, ok := got.(map[string]any)
	if !ok || m == nil {
		t.Fatalf("want non-nil map[string]any, got %#v", got)
	}
	if len(m) != 0 {
		t.Fatalf("want empty map, got %#v", m)
	}
}

func TestAPICall_XForwardedProtoUsesHTTPSURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not receive plain request when client speaks TLS to wrong endpoint")
	}))
	defer srv.Close()

	req := newFakeReq(t, srv.URL)
	req.Header.Set("X-Forwarded-Proto", "https")
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	_, err := APICall(ctx, http.MethodGet, "/clusters", nil)
	if err == nil {
		t.Fatal("expected error connecting with https URL to plain HTTP server")
	}
	if !strings.Contains(err.Error(), "https://") {
		t.Fatalf("error should mention https URL for debugging: %v", err)
	}
}

func TestGetStr(t *testing.T) {
	t.Parallel()
	if got := getStr(nil, "k"); got != "" {
		t.Fatalf("nil map: got %q", got)
	}
	m := map[string]any{"a": "x", "b": nil, "c": 42, "d": true}
	if getStr(m, "missing") != "" {
		t.Fatal()
	}
	if getStr(m, "b") != "" {
		t.Fatal()
	}
	if getStr(m, "a") != "x" {
		t.Fatal()
	}
	if getStr(m, "c") != "42" {
		t.Fatalf("got %q", getStr(m, "c"))
	}
}

func TestGetInt(t *testing.T) {
	t.Parallel()
	if _, ok := getInt(nil, "k"); ok {
		t.Fatal()
	}
	m := map[string]any{
		"f":     float64(3),
		"i":     7,
		"i64":   int64(9),
		"jn":    json.Number("11"),
		"jnBad": json.Number("abc"),
		"jnF":   json.Number("2.5"),
		"bad":   "x",
		"nil":   nil,
	}
	testOk := func(key string, want int) {
		t.Helper()
		got, ok := getInt(m, key)
		if !ok || got != want {
			t.Fatalf("%s: got (%d,%v) want (%d,true)", key, got, ok, want)
		}
	}
	testOk("f", 3)
	testOk("i", 7)
	testOk("i64", 9)
	testOk("jn", 11)
	if _, ok := getInt(m, "jnBad"); ok {
		t.Fatal("jnBad should fail Int64")
	}
	if _, ok := getInt(m, "jnF"); ok {
		t.Fatal("jnF should not be valid int")
	}
	if _, ok := getInt(m, "bad"); ok {
		t.Fatal()
	}
	if _, ok := getInt(m, "nil"); ok {
		t.Fatal()
	}
	if _, ok := getInt(m, "missing"); ok {
		t.Fatal()
	}
}

func TestGetBool(t *testing.T) {
	t.Parallel()
	if _, ok := getBool(nil, "k"); ok {
		t.Fatal()
	}
	m := map[string]any{"t": true, "f": false, "s": "x", "nil": nil}
	if b, ok := getBool(m, "t"); !ok || !b {
		t.Fatal()
	}
	if b, ok := getBool(m, "f"); !ok || b {
		t.Fatal()
	}
	if _, ok := getBool(m, "s"); ok {
		t.Fatal()
	}
	if _, ok := getBool(m, "nil"); ok {
		t.Fatal()
	}
}

func TestGetFloat(t *testing.T) {
	t.Parallel()
	if _, ok := getFloat(nil, "k"); ok {
		t.Fatal()
	}
	m := map[string]any{
		"f":     float64(1.5),
		"i":     2,
		"i64":   int64(3),
		"jn":    json.Number("4.25"),
		"jnBad": json.Number("abc"),
		"bad":   "x",
		"nil":   nil,
	}
	testOk := func(key string, want float64) {
		t.Helper()
		got, ok := getFloat(m, key)
		if !ok || got != want {
			t.Fatalf("%s: got (%v,%v) want (%v,true)", key, got, ok, want)
		}
	}
	testOk("f", 1.5)
	testOk("i", 2)
	testOk("i64", 3)
	testOk("jn", 4.25)
	if _, ok := getFloat(m, "jnBad"); ok {
		t.Fatal()
	}
	if _, ok := getFloat(m, "bad"); ok {
		t.Fatal()
	}
	if _, ok := getFloat(m, "nil"); ok {
		t.Fatal()
	}
}

func TestBuildQuery(t *testing.T) {
	t.Parallel()
	if got := buildQuery(nil, "a"); got != "" {
		t.Fatalf("nil map: %q", got)
	}
	p := map[string]any{
		"a": nil,
		"b": "v2",
		"c": true,
		"d": float64(3),
		"e": 4,
		"f": int64(5),
		"g": json.Number("6"),
		"h": "only-this",
	}
	// keys b,c,d,e,f,g,h — a skipped (nil). Encode sorts: b,c,d,e,f,g,h
	want := "?" + url.Values{
		"b": {"v2"},
		"c": {"true"},
		"d": {"3"},
		"e": {"4"},
		"f": {"5"},
		"g": {"6"},
		"h": {"only-this"},
	}.Encode()
	if got := buildQuery(p, "a", "h", "g", "f", "e", "d", "c", "b"); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if buildQuery(p) != "" {
		t.Fatal()
	}
}

func TestScalarToQueryAndSetQuery(t *testing.T) {
	t.Parallel()
	q := url.Values{}
	setQuery(q, nil, "x", "apiX")
	if len(q) != 0 {
		t.Fatal()
	}
	p := map[string]any{"src": "v", "omit": nil}
	setQuery(q, p, "missing", "api")
	setQuery(q, p, "omit", "api")
	setQuery(q, p, "src", "apiKey")
	if q.Get("apiKey") != "v" || len(q) != 1 {
		t.Fatalf("q: %v", q)
	}
	if s, ok := scalarToQuery("x"); !ok || s != "x" {
		t.Fatal()
	}
}

func TestParseParams(t *testing.T) {
	t.Parallel()
	m, err := parseParams(nil)
	if err != nil || m == nil || len(m) != 0 {
		t.Fatalf("nil raw: %#v %v", m, err)
	}
	m, err = parseParams(json.RawMessage(""))
	if err != nil || m == nil || len(m) != 0 {
		t.Fatalf("empty raw: %#v %v", m, err)
	}
	m, err = parseParams(json.RawMessage(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if a, ok := m["a"].(float64); !ok || a != 1 {
		t.Fatalf("%#v", m)
	}
	if _, err := parseParams(json.RawMessage(`not json`)); err == nil {
		t.Fatal()
	}
	m, err = parseParams(json.RawMessage(`null`))
	if err != nil || m == nil || len(m) != 0 {
		t.Fatalf("null: %#v %v", m, err)
	}
}

func newFakeReq(t *testing.T, serverURL string) *http.Request {
	t.Helper()
	u, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp/rpc", nil)
	req.Host = u.Host
	return req
}
