/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/lib/pq"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestNullTimeToISO(t *testing.T) {
	assert.Equal(t, "", nullTimeToISO(pq.NullTime{Valid: false}))
	out := nullTimeToISO(pq.NullTime{Time: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC), Valid: true})
	assert.Equal(t, "2025-01-02T03:04:05Z", out)
}

func TestMustJSON(t *testing.T) {
	assert.Equal(t, `{"a":1}`, mustJSON(map[string]int{"a": 1}))
	// Unmarshalable value falls back to "[]".
	assert.Equal(t, "[]", mustJSON(make(chan int)))
}

func TestParseAfterSeqAndEventID(t *testing.T) {
	assert.Equal(t, int64(0), parseAfterSeq(""))
	assert.Equal(t, int64(42), parseAfterSeq("task-1-42"))
	assert.Equal(t, int64(0), parseAfterSeq("task-abc")) // non-numeric tail
	assert.Equal(t, int64(7), parseSeqFromEventID("t-7"))
}

func TestDbEventToAPI(t *testing.T) {
	ev := dbEventToAPI(&dbclient.OptimizationEvent{
		EventID:   "t-1",
		TaskID:    "t",
		Type:      "log",
		Timestamp: 123,
		Payload:   `{"k":"v"}`,
	})
	assert.Equal(t, "t-1", ev.ID)
	assert.Equal(t, EventType("log"), ev.Type)
	assert.Equal(t, int64(123), ev.Timestamp)
}

func TestMakeDoneEvent(t *testing.T) {
	ev := makeDoneEvent("task-1", 9, dbclient.OptimizationTaskStatusSucceeded, "ok")
	assert.Equal(t, "task-1-9", ev.ID)
	assert.Equal(t, EventTypeDone, ev.Type)
	assert.Contains(t, string(ev.Payload), "ok")
}

func TestWriteSSEEvent(t *testing.T) {
	w := httptest.NewRecorder()
	ev := Event{ID: "id-1", TaskID: "t", Type: EventTypeLog}
	err := writeSSEEvent(w, w, ev)
	assert.NoError(t, err)
	body := w.Body.String()
	assert.Contains(t, body, "id: id-1")
	assert.Contains(t, body, "event: log")
	assert.Contains(t, body, "data: ")
}

func TestExtractPortFromCommand(t *testing.T) {
	// Space-separated and '='-separated port flags are both parsed.
	assert.Equal(t, 8888, extractPortFromCommand("python -m sglang.launch_server --port 8888"))
	assert.Equal(t, 8000, extractPortFromCommand("vllm serve model --port=8000"))
	// No port flag -> 0 (no-match branch).
	assert.Equal(t, 0, extractPortFromCommand("vllm serve model"))
}

func TestSafePositive(t *testing.T) {
	assert.Equal(t, 5, safePositive(5, 1))
	assert.Equal(t, 1, safePositive(0, 1))
	assert.Equal(t, 1, safePositive(-3, 1))
}

func TestBuildDefaultLaunchCommand(t *testing.T) {
	sglang := buildDefaultLaunchCommand(FrameworkSGLang, "/models/m", "m", 4)
	assert.Contains(t, sglang, "sglang.launch_server")
	assert.Contains(t, sglang, "--tp 4")

	vllm := buildDefaultLaunchCommand("vllm", "/models/m", "my-model", 0)
	assert.Contains(t, vllm, "vllm serve")
	assert.Contains(t, vllm, "--tensor-parallel-size 1") // 0 -> default 1
}

func TestExtractLaunchCommandFromReport(t *testing.T) {
	report := "intro\n```bash\npython3 -m sglang.launch_server --model-path /m --tp 2\n```\nmore"
	cmd := extractLaunchCommandFromReport(report)
	assert.Contains(t, cmd, "sglang.launch_server")

	// Line-based fallback.
	cmd2 := extractLaunchCommandFromReport("run this: vllm serve foo --port 8000")
	assert.Contains(t, cmd2, "vllm serve")

	// Nothing found.
	assert.Equal(t, "", extractLaunchCommandFromReport("no command here"))
}

func TestLooksLikeOptimizationReport(t *testing.T) {
	assert.True(t, looksLikeOptimizationReport("/a/b/optimization_report.md"))
	assert.True(t, looksLikeOptimizationReport("Optimization-Report.md"))
	assert.True(t, looksLikeOptimizationReport("/x/my_optimization_report_v2.txt"))
	assert.False(t, looksLikeOptimizationReport("/a/readme.md"))
}

func TestTaskToCreateRequest(t *testing.T) {
	task := &dbclient.OptimizationTask{
		DisplayName:    "opt-1",
		ModelID:        "m1",
		Framework:      "vllm",
		KernelBackends: `["triton","cuda"]`,
	}
	req := taskToCreateRequest(task)
	assert.Equal(t, "opt-1", req.DisplayName)
	assert.Equal(t, []string{"triton", "cuda"}, req.KernelBackends)
}

func TestClawClientPureHelpers(t *testing.T) {
	c := NewClawClient("https://claw.example.com/", "api-key")
	assert.Equal(t, "https://claw.example.com", c.baseURL)
	assert.Equal(t, "api-key", c.apiKey)

	// applyHeaders: static apiKey used when no per-request bearer.
	req, _ := http.NewRequest(http.MethodGet, "https://x", nil)
	c.applyHeaders(req)
	assert.Equal(t, "Bearer api-key", req.Header.Get("Authorization"))

	// Per-request bearer overrides the static key.
	req2, _ := http.NewRequestWithContext(WithClawBearer(context.Background(), "ctx-tok"), http.MethodGet, "https://x", nil)
	c.applyHeaders(req2)
	assert.Equal(t, "Bearer ctx-tok", req2.Header.Get("Authorization"))
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "abc", truncate("abc", 5))
	assert.Equal(t, "ab...", truncate("abcdef", 2))
}

func TestEscapeClawFilePath(t *testing.T) {
	assert.Equal(t, "a/b%20c", escapeClawFilePath("/a/b c/"))
	assert.Equal(t, "x/y", escapeClawFilePath("x\\y"))
}

func TestSessionStatusTerminalSucceeded(t *testing.T) {
	assert.True(t, (&SessionStatus{AgentStatus: "failed"}).IsTerminal())
	assert.True(t, (&SessionStatus{Status: "completed"}).IsTerminal())
	assert.False(t, (&SessionStatus{AgentStatus: "running"}).IsTerminal())

	assert.True(t, (&SessionStatus{AgentStatus: "idle"}).IsSucceeded())
	assert.True(t, (&SessionStatus{Status: "completed"}).IsSucceeded())
	assert.False(t, (&SessionStatus{Status: "failed"}).IsSucceeded())
}

func TestParseSSEStream(t *testing.T) {
	stream := "id: 1\nevent: phase\ndata: {\"a\":1}\n\n: keepalive\nid: 2\nevent: log\ndata: hello\n\n"
	var events []ClawSSEEvent
	err := parseSSEStream(context.Background(), strings.NewReader(stream), func(e ClawSSEEvent) error {
		events = append(events, e)
		return nil
	})
	assert.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, "phase", events[0].Event)
	assert.Equal(t, `{"a":1}`, events[0].Data)
	assert.Equal(t, "hello", events[1].Data)
}
