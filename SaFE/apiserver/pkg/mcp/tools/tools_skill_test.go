// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	mcpserver "github.com/AMD-AIG-AIMA/SAFE/common/pkg/mcp/server"
)

// The bounds below are written out here on purpose: a test that reads them from the
// implementation would move with it.
const (
	wantMaxLogLimit      = 200
	wantDefaultLogLimit  = 100
	wantDefaultEvtLimit  = 50
	wantMaxTailLines     = 1000
	wantDefaultTailLines = 200
	wantMaxResponseBytes = 64 * 1024
	wantMaxFieldChars    = 1000
)

type recordedRequest struct {
	Method string
	Path   string // escaped path, without the /api/v1 root
	Query  url.Values
	Body   map[string]any
	Auth   string
}

// backend records every request it receives and answers each with reply.
type backend struct {
	mu   sync.Mutex
	reqs []recordedRequest
	srv  *httptest.Server
}

func newBackend(t *testing.T, reply string) *backend {
	t.Helper()
	b := &backend{}
	b.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		rec := recordedRequest{
			Method: r.Method,
			Path:   strings.TrimPrefix(r.URL.EscapedPath(), "/"+common.PrimusRouterCustomRootPath),
			Query:  r.URL.Query(),
			Auth:   r.Header.Get("Authorization"),
		}
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &rec.Body)
		}
		b.mu.Lock()
		b.reqs = append(b.reqs, rec)
		b.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(b.srv.Close)
	return b
}

func (b *backend) requests() []recordedRequest {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]recordedRequest(nil), b.reqs...)
}

// call runs one tool as a caller whose API key is "caller-key" and returns the result the
// way a client receives it: after a round trip through JSON.
func (b *backend) call(t *testing.T, name, args string) (any, error) {
	t.Helper()
	req := newFakeReq(t, b.srv.URL)
	req.Header.Set("Authorization", "Bearer caller-key")
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	res, err := findTool(t, name).Handler(ctx, json.RawMessage(args))
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var wire any
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	return wire, nil
}

// serve runs one tool through the MCP server as a client does and returns the text the
// client receives, which is what the size bounds are about.
func (b *backend) serve(t *testing.T, name, args string) string {
	t.Helper()
	srv := mcpserver.New()
	srv.RegisterTools(RegisterAllTools())
	req := newFakeReq(t, b.srv.URL)
	req.Header.Set("Authorization", "Bearer caller-key")
	ctx := mcpserver.ContextWithHTTPRequest(context.Background(), req)
	msg, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": json.RawMessage(args)},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := srv.HandleMessage(ctx, msg)
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Error != nil || resp.Result.IsError || len(resp.Result.Content) != 1 {
		t.Fatalf("tools/call %s failed: %s", name, raw)
	}
	return resp.Result.Content[0].Text
}

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("want a JSON object, got %T: %v", v, v)
	}
	return m
}

func asInt(t *testing.T, v any) int {
	t.Helper()
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("want a number, got %T: %v", v, v)
	}
	return int(f)
}

func TestSkillTools_RequestShape(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		tool      string
		args      string
		method    string
		path      string
		query     map[string]string // exact expected query
		body      map[string]any    // exact expected body; nil means no body
		wantNoReq bool
		wantErr   string
	}{
		{name: "user self", tool: "user_self", args: `{}`, method: "GET", path: "/users/self"},
		{name: "flavor avail", tool: "flavor_avail", args: `{"flavor_id":"f1"}`, method: "GET", path: "/nodeflavors/f1/avail"},
		{name: "flavor avail escapes the id", tool: "flavor_avail", args: `{"flavor_id":"a/b"}`, method: "GET", path: "/nodeflavors/a%2Fb/avail"},
		{name: "flavor avail needs an id", tool: "flavor_avail", args: `{}`, wantNoReq: true, wantErr: "flavor_id"},
		{name: "events default window", tool: "workload_events", args: `{"workload_id":"w1"}`, method: "POST", path: "/workloads/w1/events",
			body: map[string]any{"limit": float64(wantDefaultEvtLimit), "order": "desc"}},
		{name: "events limit is clamped", tool: "workload_events", args: `{"workload_id":"w1","limit":99999}`, method: "POST", path: "/workloads/w1/events",
			body: map[string]any{"limit": float64(wantMaxLogLimit), "order": "desc"}},
		{name: "events non-positive limit falls back", tool: "workload_events", args: `{"workload_id":"w1","limit":-3}`, method: "POST", path: "/workloads/w1/events",
			body: map[string]any{"limit": float64(wantDefaultEvtLimit), "order": "desc"}},
		{name: "events pass the window through", tool: "workload_events", args: `{"workload_id":"w1","since":"2026-11-03T00:00:00.000Z","until":"2026-11-03T01:00:00.000Z","offset":5,"limit":10,"order":"asc"}`,
			method: "POST", path: "/workloads/w1/events",
			body: map[string]any{"since": "2026-11-03T00:00:00.000Z", "until": "2026-11-03T01:00:00.000Z", "offset": float64(5), "limit": float64(10), "order": "asc"}},
		{name: "events need an id", tool: "workload_events", args: `{}`, wantNoReq: true, wantErr: "workload_id"},
		{name: "events reject an unknown order", tool: "workload_events", args: `{"workload_id":"w1","order":"sideways"}`, wantNoReq: true, wantErr: "order"},
		{name: "logs default window", tool: "workload_logs", args: `{"workload_id":"w1"}`, method: "POST", path: "/workloads/w1/logs",
			body: map[string]any{"limit": float64(wantDefaultLogLimit), "order": "desc"}},
		{name: "logs limit is clamped", tool: "workload_logs", args: `{"workload_id":"w1","limit":100000}`, method: "POST", path: "/workloads/w1/logs",
			body: map[string]any{"limit": float64(wantMaxLogLimit), "order": "desc"}},
		{name: "logs map every filter to the REST field", tool: "workload_logs",
			args:   `{"workload_id":"w1","keywords":["error","oom"],"pod_names":"p1,p2","node_names":"n1","dispatch_count":2,"order":"asc"}`,
			method: "POST", path: "/workloads/w1/logs",
			body: map[string]any{"limit": float64(wantDefaultLogLimit), "order": "asc", "keywords": []any{"error", "oom"},
				"podNames": "p1,p2", "nodeNames": "n1", "dispatchCount": float64(2)}},
		{name: "logs need an id", tool: "workload_logs", args: `{}`, wantNoReq: true, wantErr: "workload_id"},
		{name: "resume", tool: "workload_resume", args: `{"workload_id":"w1"}`, method: "POST", path: "/workloads/w1/resume"},
		{name: "resume escapes the id", tool: "workload_resume", args: `{"workload_id":"w/1"}`, method: "POST", path: "/workloads/w%2F1/resume"},
		{name: "resume needs an id", tool: "workload_resume", args: `{}`, wantNoReq: true, wantErr: "workload_id"},
		{name: "pod containers", tool: "workload_pod_containers", args: `{"workload_id":"w1","pod_id":"p1"}`, method: "GET", path: "/workloads/w1/pods/p1/containers"},
		{name: "pod containers need both ids", tool: "workload_pod_containers", args: `{"workload_id":"w1"}`, wantNoReq: true, wantErr: "pod_id"},
		{name: "node pod logs default tail", tool: "node_pod_logs", args: `{"node_id":"n1"}`, method: "GET", path: "/nodes/n1/logs",
			query: map[string]string{"tailLines": fmt.Sprint(wantDefaultTailLines)}},
		{name: "node pod logs tail is clamped", tool: "node_pod_logs", args: `{"node_id":"n1","tail_lines":99999}`, method: "GET", path: "/nodes/n1/logs",
			query: map[string]string{"tailLines": fmt.Sprint(wantMaxTailLines)}},
		{name: "node pod logs container and window", tool: "node_pod_logs", args: `{"node_id":"n1","tail_lines":50,"container":"c","since_seconds":600}`, method: "GET", path: "/nodes/n1/logs",
			query: map[string]string{"tailLines": "50", "container": "c", "sinceSeconds": "600"}},
		{name: "node pod logs need an id", tool: "node_pod_logs", args: `{}`, wantNoReq: true, wantErr: "node_id"},
		{name: "secret list", tool: "secret_list", args: `{}`, method: "GET", path: "/secrets"},
		{name: "secret list filters", tool: "secret_list", args: `{"type":"image","workspace_id":"ws1"}`, method: "GET", path: "/secrets",
			query: map[string]string{"type": "image", "workspaceId": "ws1"}},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend(t, `{}`)
			_, err := b.call(t, tc.tool, tc.args)
			reqs := b.requests()
			if tc.wantNoReq {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want an error naming %q, got %v", tc.wantErr, err)
				}
				if len(reqs) != 0 {
					t.Fatalf("a rejected call must not reach the API, got %+v", reqs)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(reqs) != 1 {
				t.Fatalf("want exactly one API request, got %+v", reqs)
			}
			r := reqs[0]
			if r.Method != tc.method || r.Path != tc.path {
				t.Fatalf("got %s %s want %s %s", r.Method, r.Path, tc.method, tc.path)
			}
			if r.Auth != "Bearer caller-key" {
				t.Fatalf("the caller's own credential must be forwarded, got %q", r.Auth)
			}
			gotQuery := map[string]string{}
			for k, v := range r.Query {
				gotQuery[k] = v[0]
			}
			if fmt.Sprint(gotQuery) != fmt.Sprint(tc.query) && !(len(gotQuery) == 0 && len(tc.query) == 0) {
				t.Fatalf("query: got %v want %v", gotQuery, tc.query)
			}
			gotBody, _ := json.Marshal(r.Body)
			wantBody, _ := json.Marshal(tc.body)
			if tc.body == nil {
				wantBody = []byte("null")
			}
			if string(gotBody) != string(wantBody) {
				t.Fatalf("body: got %s want %s", gotBody, wantBody)
			}
		})
	}
}

// openSearchReply builds the shape the logs and events endpoints return. The _source holds
// what cicdlog.BuildOutput asks the search for on a workload: no container name.
func openSearchReply(n int, msg func(i int) string) string {
	var hits []string
	for i := 0; i < n; i++ {
		m, _ := json.Marshal(msg(i))
		hits = append(hits, fmt.Sprintf(
			`{"_id":"d%d","_source":{"@timestamp":"2026-11-03T08:%02d:00.000Z","stream":"stderr","message":%s,`+
				`"kubernetes":{"pod_name":"pod-%d","host":"node-a",`+
				`"labels":{"primus-safe_workload_dispatch_count":"2"}}}}`, i, i%60, m, i))
	}
	return fmt.Sprintf(`{"took":3,"hits":{"total":{"value":%d},"hits":[%s]}}`, n, strings.Join(hits, ","))
}

func TestWorkloadLogs_SmallReplyIsReshapedIntoLines(t *testing.T) {
	t.Parallel()
	b := newBackend(t, openSearchReply(3, func(i int) string { return fmt.Sprintf("line %d", i) }))
	got, err := b.call(t, "workload_logs", `{"workload_id":"w1"}`)
	if err != nil {
		t.Fatal(err)
	}
	m := asMap(t, got)
	if asInt(t, m["total"]) != 3 || asInt(t, m["returned"]) != 3 {
		t.Fatalf("total/returned: %v", m)
	}
	if tr, _ := m["truncated"].(bool); tr {
		t.Fatalf("a small reply must not be marked truncated: %v", m)
	}
	lines, _ := m["lines"].([]any)
	if len(lines) != 3 {
		t.Fatalf("lines: %v", m["lines"])
	}
	first := asMap(t, lines[0])
	want := map[string]any{"time": "2026-11-03T08:00:00.000Z", "message": "line 0", "pod": "pod-0",
		"node": "node-a", "stream": "stderr", "dispatch_count": "2"}
	if fmt.Sprint(first) != fmt.Sprint(want) {
		t.Fatalf("first line: got %v, want %v", first, want)
	}
	for _, banned := range []string{"labels", "kubernetes", "_id"} {
		if _, ok := first[banned]; ok {
			t.Fatalf("per-line metadata %q should not be repeated on every line: %v", banned, first)
		}
	}
}

func TestWorkloadLogs_LongReplyIsBounded(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 5000)
	b := newBackend(t, openSearchReply(wantMaxLogLimit, func(int) string { return long }))
	got, err := b.call(t, "workload_logs", `{"workload_id":"w1","limit":200}`)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(b.serve(t, "workload_logs", `{"workload_id":"w1","limit":200}`)); n > wantMaxResponseBytes {
		t.Fatalf("the client receives %d bytes, the budget is %d", n, wantMaxResponseBytes)
	}
	m := asMap(t, got)
	if tr, _ := m["truncated"].(bool); !tr {
		t.Fatalf("a cut result must say so: total=%v returned=%v", m["total"], m["returned"])
	}
	if asInt(t, m["total"]) != wantMaxLogLimit {
		t.Fatalf("total must still report what matched: %v", m["total"])
	}
	lines, _ := m["lines"].([]any)
	if len(lines) == 0 || len(lines) >= wantMaxLogLimit || asInt(t, m["returned"]) != len(lines) {
		t.Fatalf("returned %v but holds %d lines", m["returned"], len(lines))
	}
	for _, l := range lines {
		msg, _ := asMap(t, l)["message"].(string)
		if n := len([]rune(msg)); n > wantMaxFieldChars+64 {
			t.Fatalf("a log line of %d chars got through", n)
		}
	}
}

func TestWorkloadLogs_RepliesThatAreNotSearchHits(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{`{}`, `{"hits":{}}`, `{"hits":{"hits":null}}`, `[]`} {
		b := newBackend(t, reply)
		if _, err := b.call(t, "workload_logs", `{"workload_id":"w1"}`); err != nil {
			t.Fatalf("reply %s: %v", reply, err)
		}
	}
}

func TestWorkloadEvents_LongReplyIsBounded(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("e", 5000)
	b := newBackend(t, openSearchReply(wantMaxLogLimit, func(int) string { return long }))
	got, err := b.call(t, "workload_events", `{"workload_id":"w1","limit":200}`)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(b.serve(t, "workload_events", `{"workload_id":"w1","limit":200}`)); n > wantMaxResponseBytes {
		t.Fatalf("the client receives %d bytes, the budget is %d", n, wantMaxResponseBytes)
	}
	m := asMap(t, got)
	if tr, _ := m["truncated"].(bool); !tr {
		t.Fatalf("a cut result must say so: %v", m["returned"])
	}
	hits := asMap(t, m["hits"])
	list, _ := hits["hits"].([]any)
	if len(list) == 0 || len(list) >= wantMaxLogLimit || asInt(t, m["returned"]) != len(list) {
		t.Fatalf("returned %v but kept %d of %d events", m["returned"], len(list), wantMaxLogLimit)
	}
	src := asMap(t, asMap(t, list[0])["_source"])
	if msg, _ := src["message"].(string); len([]rune(msg)) > wantMaxFieldChars+64 {
		t.Fatalf("an event message of %d chars got through", len([]rune(msg)))
	}
}

func TestWorkloadEvents_SmallReplyIsUntouched(t *testing.T) {
	t.Parallel()
	reply := openSearchReply(2, func(i int) string { return fmt.Sprintf("Back-off pulling image %d", i) })
	b := newBackend(t, reply)
	got, err := b.call(t, "workload_events", `{"workload_id":"w1"}`)
	if err != nil {
		t.Fatal(err)
	}
	m := asMap(t, got)
	if tr, _ := m["truncated"].(bool); tr {
		t.Fatalf("a small complete reply must not be marked truncated")
	}
	list, _ := asMap(t, m["hits"])["hits"].([]any)
	if len(list) != 2 {
		t.Fatalf("kept %d events", len(list))
	}
	if src := asMap(t, asMap(t, list[1])["_source"]); src["message"] != "Back-off pulling image 1" {
		t.Fatalf("event 1: %v", src)
	}
}

// A search that matched more than it returned is not complete, whatever the byte budget
// says. The reply says how many matched (hits.total.value); offset moves the window.
func TestSearchTools_TruncatedMeansMoreMatchedThanReturned(t *testing.T) {
	t.Parallel()
	withTotal := func(n int, total string) string {
		r := openSearchReply(n, func(i int) string { return fmt.Sprintf("m%d", i) })
		return strings.Replace(r, fmt.Sprintf(`"total":{"value":%d}`, n), `"total":{"value":`+total+`}`, 1)
	}
	noTotal := strings.Replace(openSearchReply(2, func(int) string { return "m" }), `"total":{"value":2},`, "", 1)
	tests := []struct {
		name  string
		reply string
		args  string
		want  bool
	}{
		{"all of them came back", withTotal(3, "3"), `{"workload_id":"w1"}`, false},
		{"more matched than came back", withTotal(3, "350"), `{"workload_id":"w1","limit":3}`, true},
		{"the last page is complete", withTotal(100, "200"), `{"workload_id":"w1","offset":100}`, false},
		{"a middle page is not", withTotal(100, "350"), `{"workload_id":"w1","offset":100}`, true},
		{"one short of the end", withTotal(100, "201"), `{"workload_id":"w1","offset":100}`, true},
		{"no total in the reply", noTotal, `{"workload_id":"w1"}`, false},
		{"a lower-bound total is never complete", withTotal(100, `10000,"relation":"gte"`), `{"workload_id":"w1","offset":9900}`, true},
		{"an exact total is", withTotal(100, `200,"relation":"eq"`), `{"workload_id":"w1","offset":100}`, false},
	}
	for _, tool := range []string{"workload_logs", "workload_events"} {
		for _, tc := range tests {
			tool, tc := tool, tc
			t.Run(tool+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				b := newBackend(t, tc.reply)
				got, err := b.call(t, tool, tc.args)
				if err != nil {
					t.Fatal(err)
				}
				if tr, _ := asMap(t, got)["truncated"].(bool); tr != tc.want {
					t.Fatalf("truncated = %v, want %v", tr, tc.want)
				}
			})
		}
	}
}

// The budget is about the text the client receives, so characters that JSON escapes
// (< > & become \u003c, \u003e, \u0026) and the indentation count.
func TestBoundedReplies_BudgetIsMeasuredOnTheEncodedText(t *testing.T) {
	t.Parallel()
	escaped := strings.Repeat("<&>", 330) // 990 characters, 5940 bytes once encoded
	var nodeLines []string
	for i := 0; i < 1000; i++ {
		nodeLines = append(nodeLines, escaped)
	}
	nodeReply, _ := json.Marshal(map[string]any{"nodeId": "n1", "logs": nodeLines})
	tests := []struct {
		tool, args, reply string
	}{
		{"workload_logs", `{"workload_id":"w1","limit":200}`, openSearchReply(wantMaxLogLimit, func(int) string { return escaped })},
		{"workload_events", `{"workload_id":"w1","limit":200}`, openSearchReply(wantMaxLogLimit, func(int) string { return escaped })},
		{"node_pod_logs", `{"node_id":"n1","tail_lines":1000}`, string(nodeReply)},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.tool, func(t *testing.T) {
			t.Parallel()
			b := newBackend(t, tc.reply)
			text := b.serve(t, tc.tool, tc.args)
			if len(text) > wantMaxResponseBytes {
				t.Fatalf("the client receives %d bytes, the budget is %d", len(text), wantMaxResponseBytes)
			}
			if !strings.Contains(text, `"truncated": true`) {
				t.Fatalf("a cut result must say so")
			}
		})
	}
}

func TestNodePodLogs_ReplyKeepsTheNewestLines(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := 0; i < 5000; i++ {
		lines = append(lines, fmt.Sprintf("log line %d %s", i, strings.Repeat("y", 3000)))
	}
	raw, _ := json.Marshal(map[string]any{"clusterId": "c1", "nodeId": "n1", "podId": "p1", "logs": lines})
	b := newBackend(t, string(raw))
	got, err := b.call(t, "node_pod_logs", `{"node_id":"n1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(b.serve(t, "node_pod_logs", `{"node_id":"n1"}`)); n > wantMaxResponseBytes {
		t.Fatalf("the client receives %d bytes, the budget is %d", n, wantMaxResponseBytes)
	}
	m := asMap(t, got)
	if m["nodeId"] != "n1" || m["podId"] != "p1" {
		t.Fatalf("identity fields were lost: %v", m)
	}
	if tr, _ := m["truncated"].(bool); !tr {
		t.Fatalf("a cut result must say so")
	}
	kept, _ := m["logs"].([]any)
	if len(kept) == 0 || len(kept) > wantMaxTailLines {
		t.Fatalf("kept %d lines", len(kept))
	}
	last, _ := kept[len(kept)-1].(string)
	if !strings.HasPrefix(last, "log line 4999 ") {
		t.Fatalf("the newest line must survive and stay last, last kept = %.20q", last)
	}
	prev, _ := kept[len(kept)-2].(string)
	if !strings.HasPrefix(prev, "log line 4998 ") {
		t.Fatalf("lines must keep their order, second to last = %.20q", prev)
	}
	for _, l := range kept {
		if n := len([]rune(l.(string))); n > wantMaxFieldChars+64 {
			t.Fatalf("a line of %d chars got through", n)
		}
	}
}

// The backend splits the log text on newlines and applies tail_lines itself, so a full
// reply to tail_lines=200 holds 200 lines and the empty string after the final newline.
func TestNodePodLogs_TrailingNewlineIsNotALine(t *testing.T) {
	t.Parallel()
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	raw, _ := json.Marshal(map[string]any{"nodeId": "n1", "logs": append(lines, "")})
	b := newBackend(t, string(raw))
	got, err := b.call(t, "node_pod_logs", `{"node_id":"n1","tail_lines":200}`)
	if err != nil {
		t.Fatal(err)
	}
	m := asMap(t, got)
	kept, _ := m["logs"].([]any)
	if len(kept) != 200 || kept[0] != "line 0" || kept[199] != "line 199" {
		t.Fatalf("want the 200 lines the backend returned, got %d lines from %v to %v", len(kept), kept[0], kept[len(kept)-1])
	}
	if tr, _ := m["truncated"].(bool); tr {
		t.Fatalf("the backend returned exactly what was asked for and the reply says lines were left out")
	}
}

func TestNodePodLogs_ShortReplyIsUntouched(t *testing.T) {
	t.Parallel()
	b := newBackend(t, `{"clusterId":"c1","nodeId":"n1","podId":"p1","logs":["a","b","c"]}`)
	got, err := b.call(t, "node_pod_logs", `{"node_id":"n1"}`)
	if err != nil {
		t.Fatal(err)
	}
	m := asMap(t, got)
	if tr, _ := m["truncated"].(bool); tr {
		t.Fatalf("a short complete reply must not be marked truncated")
	}
	if fmt.Sprint(m["logs"]) != "[a b c]" {
		t.Fatalf("logs: %v", m["logs"])
	}
}

// ---------------------------------------------------------------------------------------
// Which tools only read. The approval gate in front of the MCP server is meant to hold
// every tool except a named read-only set, so this set has to be exactly right.

var wantReadOnlyTools = []string{
	"cluster_get", "cluster_list", "cluster_logs",
	"flavor_avail", "flavor_get", "flavor_list",
	"node_export", "node_get", "node_list", "node_pod_logs", "node_reboot_logs", "node_status",
	"opsjob_get", "opsjob_list",
	"secret_list",
	"user_self",
	"workload_events", "workload_get", "workload_list", "workload_logs", "workload_pod_containers", "workload_pod_logs",
	"workspace_get", "workspace_list",
}

var wantStateChangingTools = []string{
	"cluster_create", "cluster_delete", "cluster_manage_nodes", "cluster_update",
	"node_cordon", "node_create", "node_delete", "node_drain", "node_reboot", "node_uncordon", "node_update",
	"opsjob_create", "opsjob_delete", "opsjob_stop",
	"workload_batch_delete", "workload_batch_stop", "workload_create", "workload_delete",
	"workload_logs_download", "workload_resume", "workload_stop", "workload_update",
	"workspace_create", "workspace_delete", "workspace_manage_nodes", "workspace_scale", "workspace_update",
}

// listedTools returns what a client sees from tools/list: name -> annotations.
func listedTools(t *testing.T) map[string]map[string]any {
	t.Helper()
	srv := mcpserver.New()
	srv.RegisterTools(RegisterAllTools())
	raw, err := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	var resp struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				Annotations map[string]any `json:"annotations"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]any{}
	for _, tl := range resp.Result.Tools {
		out[tl.Name] = tl.Annotations
	}
	return out
}

func TestToolsList_ReadOnlyHintMatchesTheClassification(t *testing.T) {
	t.Parallel()
	listed := listedTools(t)
	readOnly := map[string]bool{}
	for _, n := range wantReadOnlyTools {
		readOnly[n] = true
	}
	for _, n := range wantStateChangingTools {
		if readOnly[n] {
			t.Fatalf("%s is in both lists", n)
		}
	}
	if len(listed) != len(wantReadOnlyTools)+len(wantStateChangingTools) {
		var got []string
		for n := range listed {
			got = append(got, n)
		}
		sort.Strings(got)
		t.Fatalf("every tool must be classified exactly once: %d tools listed, %d classified:\n%v",
			len(listed), len(wantReadOnlyTools)+len(wantStateChangingTools), got)
	}
	for _, n := range wantReadOnlyTools {
		ann, ok := listed[n]
		if !ok {
			t.Errorf("read-only tool %s is not served", n)
			continue
		}
		if v, _ := ann["readOnlyHint"].(bool); !v {
			t.Errorf("%s must carry readOnlyHint=true, annotations=%v", n, ann)
		}
	}
	for _, n := range wantStateChangingTools {
		ann, ok := listed[n]
		if !ok {
			t.Errorf("state-changing tool %s is not served", n)
			continue
		}
		if v, _ := ann["readOnlyHint"].(bool); v {
			t.Errorf("%s changes state and must not claim readOnlyHint, annotations=%v", n, ann)
		}
	}
}

// isReadRequest says whether an API request cannot change state. Two POST routes are
// reads: the log and event searches take their query as a body.
func isReadRequest(r recordedRequest) bool {
	if r.Method == http.MethodGet {
		return true
	}
	return r.Method == http.MethodPost &&
		(strings.HasSuffix(r.Path, "/events") || (strings.HasPrefix(r.Path, "/workloads/") && strings.HasSuffix(r.Path, "/logs")))
}

// TestReadOnlyHint_IsTrueOfWhatTheToolsDo runs every tool against a recording backend.
// A tool that claims readOnlyHint must only issue read requests; a tool that does not
// must issue at least one request that is not a read, or it would be held for nothing.
func TestReadOnlyHint_IsTrueOfWhatTheToolsDo(t *testing.T) {
	t.Parallel()
	listed := listedTools(t)
	for _, tl := range RegisterAllTools() {
		tl := tl
		t.Run(tl.Name, func(t *testing.T) {
			t.Parallel()
			b := newBackend(t, `{}`)
			if _, err := b.call(t, tl.Name, string(minimalArgsForTool(tl.Name))); err != nil {
				t.Fatal(err)
			}
			reqs := b.requests()
			if len(reqs) == 0 {
				t.Fatal("the tool made no API request")
			}
			allReads := true
			for _, r := range reqs {
				if !isReadRequest(r) {
					allReads = false
				}
			}
			claimsReadOnly, _ := listed[tl.Name]["readOnlyHint"].(bool)
			if claimsReadOnly && !allReads {
				t.Fatalf("%s claims readOnlyHint but sent %+v", tl.Name, reqs)
			}
			if !claimsReadOnly && allReads {
				t.Fatalf("%s only reads (%+v) but is not marked read-only", tl.Name, reqs)
			}
		})
	}
}

// A cut result must be an unbroken run from the start of what the API returned (from the
// end for the node log): a short entry further on must not slip in after longer ones were
// dropped, or the reader sees a gap it cannot tell from the real log.
func TestBoundedReplies_KeepAnUnbrokenRun(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("z", 5000)
	mixed := func(i int) string {
		if i == wantMaxLogLimit-1 {
			return "SHORT-LAST-ENTRY"
		}
		return long
	}
	for _, tool := range []string{"workload_logs", "workload_events"} {
		b := newBackend(t, openSearchReply(wantMaxLogLimit, mixed))
		got, err := b.call(t, tool, `{"workload_id":"w1","limit":200}`)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(got)
		if tr, _ := asMap(t, got)["truncated"].(bool); !tr {
			t.Fatalf("%s: the reply was cut and must say so", tool)
		}
		if strings.Contains(string(raw), "SHORT-LAST-ENTRY") {
			t.Fatalf("%s: an entry after the cut slipped in", tool)
		}
	}

	var lines []string
	for i := 0; i < 2000; i++ {
		if i == 0 {
			lines = append(lines, "SHORT-FIRST-LINE")
			continue
		}
		lines = append(lines, strings.Repeat("z", 3000))
	}
	raw, _ := json.Marshal(map[string]any{"nodeId": "n1", "logs": lines})
	b := newBackend(t, string(raw))
	got, err := b.call(t, "node_pod_logs", `{"node_id":"n1","tail_lines":1000}`)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(got)
	if strings.Contains(string(out), "SHORT-FIRST-LINE") {
		t.Fatalf("a line before the cut slipped in")
	}
}

// ---------------------------------------------------------------------------------------
// The skill is the other half of this change: what it names has to exist.

func skillFiles(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "..", "skills", "primus-safe")
	out := map[string]string{}
	for _, f := range []string{"SKILL.md", "references/api-cheatsheet.md", "references/troubleshooting.md"} {
		raw, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Fatal(err)
		}
		out[f] = string(raw)
	}
	return out
}

// TestSkill_NamesOnlyToolsAndArgumentsThatExist checks every code span in the skill that
// is shaped like a SaFE tool or argument name (workload_..., node_id, ...) against what the
// server registers, so a renamed or invented tool cannot linger in the text.
func TestSkill_NamesOnlyToolsAndArgumentsThatExist(t *testing.T) {
	t.Parallel()
	known := map[string]bool{}
	for _, tl := range RegisterAllTools() {
		known[tl.Name] = true
		props, _ := tl.InputSchema["properties"].(map[string]any)
		for arg := range props {
			known[arg] = true
		}
	}
	span := regexp.MustCompile("`((?:cluster|workspace|flavor|node|workload|opsjob|apikey|secret|user|pod|tail|since|dispatch|keywords)_?[a-z_]*)`")
	for name, text := range skillFiles(t) {
		matched := 0
		for _, m := range span.FindAllStringSubmatch(text, -1) {
			matched++
			tok := m[1]
			if !strings.Contains(tok, "_") && tok != "keywords" {
				continue // a plain word such as `node`, not a name
			}
			if !known[tok] {
				t.Errorf("%s names %q, which is neither a tool nor a tool argument", name, tok)
			}
		}
		if name == "SKILL.md" && matched < 30 {
			t.Errorf("the check found only %d names in %s; the pattern no longer matches the text", matched, name)
		}
	}
}

// Every tool the server registers that the skill can use should be reachable from the skill
// text, or the model will never learn of it.
func TestSkill_MentionsEveryTool(t *testing.T) {
	t.Parallel()
	all := ""
	for _, text := range skillFiles(t) {
		all += text
	}
	for _, tl := range RegisterAllTools() {
		if !strings.Contains(all, "`"+tl.Name+"`") {
			t.Errorf("tool %s is not mentioned anywhere in the skill", tl.Name)
		}
	}
}

// curl is for one thing only: saving a whole log to a file.
func TestSkill_CurlOnlyForTheLogDownload(t *testing.T) {
	t.Parallel()
	for name, text := range skillFiles(t) {
		for i, line := range strings.Split(text, "\n") {
			if strings.Contains(line, "curl") && !strings.Contains(line, "logs/download") && !strings.Contains(line, "do not fall back to curl") {
				t.Errorf("%s:%d tells the reader to use curl: %q", name, i+1, line)
			}
		}
	}
}

// Cutting text, or applying the tail, is a reason to say truncated even when nothing was
// dropped for size.
func TestBoundedReplies_CutTextAndTailAreReported(t *testing.T) {
	t.Parallel()
	one := openSearchReply(1, func(int) string { return strings.Repeat("q", wantMaxFieldChars+500) })
	for _, tool := range []string{"workload_logs", "workload_events"} {
		b := newBackend(t, one)
		got, err := b.call(t, tool, `{"workload_id":"w1"}`)
		if err != nil {
			t.Fatal(err)
		}
		if tr, _ := asMap(t, got)["truncated"].(bool); !tr {
			t.Fatalf("%s: a message was cut and the reply says it is complete", tool)
		}
	}

	var lines []string
	for i := 0; i < 300; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	raw, _ := json.Marshal(map[string]any{"nodeId": "n1", "logs": lines})
	b := newBackend(t, string(raw))
	got, err := b.call(t, "node_pod_logs", `{"node_id":"n1","tail_lines":200}`)
	if err != nil {
		t.Fatal(err)
	}
	m := asMap(t, got)
	kept, _ := m["logs"].([]any)
	if len(kept) != 200 || kept[0] != "line 100" || kept[199] != "line 299" {
		t.Fatalf("the last 200 lines were not kept in order: %d lines, first %v", len(kept), kept[0])
	}
	if tr, _ := m["truncated"].(bool); !tr {
		t.Fatalf("older lines were left out and the reply says it is complete")
	}

	long, _ := json.Marshal(map[string]any{"nodeId": "n1", "logs": []string{strings.Repeat("q", wantMaxFieldChars+500)}})
	b = newBackend(t, string(long))
	got, err = b.call(t, "node_pod_logs", `{"node_id":"n1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if tr, _ := asMap(t, got)["truncated"].(bool); !tr {
		t.Fatalf("a line was cut and the reply says it is complete")
	}
}

func TestWorkloadLogs_SaysWhenTotalIsOnlyALowerBound(t *testing.T) {
	t.Parallel()
	reply := strings.Replace(openSearchReply(2, func(int) string { return "m" }), `"total":{"value":2}`, `"total":{"value":10000,"relation":"gte"}`, 1)
	b := newBackend(t, reply)
	got, err := b.call(t, "workload_logs", `{"workload_id":"w1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if lb, _ := asMap(t, got)["totalIsLowerBound"].(bool); !lb {
		t.Fatalf("total 10000 with relation gte must be flagged as a lower bound: %v", got)
	}
	b = newBackend(t, openSearchReply(2, func(int) string { return "m" }))
	got, err = b.call(t, "workload_logs", `{"workload_id":"w1"}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := asMap(t, got)["totalIsLowerBound"]; ok {
		t.Fatalf("an exact total must not be flagged: %v", got)
	}
}

// A table row of the skill says which tool answers a question; the filters named in the
// question cell must be arguments of a tool in the answer cell, and the arguments the
// answer cell names must belong to the tool they follow.
func TestSkill_TableRowsNameArgumentsOfTheirTools(t *testing.T) {
	t.Parallel()
	args := map[string]map[string]bool{} // tool -> its arguments
	anyArg := map[string]bool{}
	for _, tl := range RegisterAllTools() {
		args[tl.Name] = map[string]bool{}
		props, _ := tl.InputSchema["properties"].(map[string]any)
		for a := range props {
			args[tl.Name][a] = true
			anyArg[a] = true
		}
	}
	span := regexp.MustCompile("`([a-z_]+)`")
	rows := 0
	for _, line := range strings.Split(skillFiles(t)["SKILL.md"], "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if !strings.HasPrefix(line, "|") || len(cells) != 2 {
			continue
		}
		var tools []string
		for _, m := range span.FindAllStringSubmatch(cells[1], -1) {
			if _, ok := args[m[1]]; ok {
				tools = append(tools, m[1])
			}
		}
		if len(tools) == 0 {
			continue
		}
		rows++
		for _, cell := range cells {
			for _, m := range span.FindAllStringSubmatch(cell, -1) {
				tok := m[1]
				if _, isTool := args[tok]; isTool || !anyArg[tok] {
					continue
				}
				accepted := false
				for _, tool := range tools {
					accepted = accepted || args[tool][tok]
				}
				if !accepted {
					t.Errorf("row %q names %q, which none of %v accepts", strings.TrimSpace(line), tok, tools)
				}
			}
		}
	}
	if rows < 10 {
		t.Errorf("the check found only %d table rows; the pattern no longer matches the table", rows)
	}
}
