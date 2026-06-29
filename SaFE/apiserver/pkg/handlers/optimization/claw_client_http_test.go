/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newClawTestServer routes Claw control-plane requests to canned responses.
func newClawTestServer(t *testing.T) (*ClawClient, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()

	// POST /sessions -> create session.
	// GET/DELETE /sessions/{id} -> status / delete.
	// GET /sessions/{id}/files -> list artifacts.
	mux.HandleFunc("/sessions", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"session_id":"sess-1","agent_status":"running","message":{"message_id":"msg-1","dispatched":true}}}`))
	})
	mux.HandleFunc("/sessions/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/files"):
			_, _ = w.Write([]byte(`{"data":[{"path":"claw-1/optimization_report.md","size":100}]}`))
		case strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`{"code":0}`))
		case strings.Contains(r.URL.Path, "/files/") && strings.HasSuffix(r.URL.Path, "/stream"):
			_, _ = w.Write([]byte("file-content"))
		case strings.Contains(r.URL.Path, "/files/") && strings.HasSuffix(r.URL.Path, "/download"):
			_, _ = w.Write([]byte(`{"data":{"api_path":"/proxy/download/x"}}`))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
		default: // GET /sessions/{id}
			_, _ = w.Write([]byte(`{"data":{"session_id":"sess-1","status":"completed","agent_status":"idle"}}`))
		}
	})
	mux.HandleFunc("/chat/sessions/", func(w http.ResponseWriter, r *http.Request) {
		// interrupt
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return NewClawClient(srv.URL, "test-key"), srv
}

func TestClawCreateSession(t *testing.T) {
	c, _ := newClawTestServer(t)
	id, err := c.CreateSession(context.Background(), &SessionRequest{Name: "opt"})
	assert.NoError(t, err)
	assert.Equal(t, "sess-1", id)
}

func TestClawCreateSessionWithMessage(t *testing.T) {
	c, _ := newClawTestServer(t)
	res, err := c.CreateSessionWithMessage(context.Background(), &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "sess-1", res.SessionID)
	assert.Equal(t, "running", res.AgentStatus)
	assert.Equal(t, "msg-1", res.MessageID)
	assert.True(t, res.Dispatched != nil && *res.Dispatched)
}

func TestClawCreateSessionWithMessageTopLevelEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"session_id":"sess-1","agent_status":"running","message":{"message_id":"msg-1","dispatched":true}}`))
	}))
	defer srv.Close()
	c := NewClawClient(srv.URL, "test-key")
	res, err := c.CreateSessionWithMessage(context.Background(), &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "sess-1", res.SessionID)
	assert.Equal(t, "msg-1", res.MessageID)
	assert.True(t, res.Dispatched != nil && *res.Dispatched)
}

// TestClawCreateSessionWithMessageNoDispatchedField verifies that an older
// Claw build which omits the `dispatched` field is accepted (we cannot prove
// non-dispatch, and a session was created), rather than failing every submit.
func TestClawCreateSessionWithMessageNoDispatchedField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"session_id":"sess-1","agent_status":"running","message":{"message_id":"msg-1"}}}`))
	}))
	defer srv.Close()
	c := NewClawClient(srv.URL, "test-key")
	res, err := c.CreateSessionWithMessage(context.Background(), &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "sess-1", res.SessionID)
	assert.Nil(t, res.Dispatched)
}

// TestClawCreateSessionWithMessageExplicitNotDispatched verifies that an
// explicit dispatched:false is still treated as a hard failure.
func TestClawCreateSessionWithMessageExplicitNotDispatched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"session_id":"sess-1","agent_status":"idle","message":{"message_id":"msg-1","dispatched":false}}}`))
	}))
	defer srv.Close()
	c := NewClawClient(srv.URL, "test-key")
	_, err := c.CreateSessionWithMessage(context.Background(), &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.Error(t, err)
}

func TestCreateClawSessionWithRetryTransientFailures(t *testing.T) {
	oldAttemptTimeout := clawCreateSessionAttemptTimeout
	oldRetryInterval := clawCreateSessionRetryInterval
	oldTotalTimeout := clawCreateSessionTotalTimeout
	clawCreateSessionAttemptTimeout = 100 * time.Millisecond
	clawCreateSessionRetryInterval = time.Millisecond
	clawCreateSessionTotalTimeout = time.Second
	t.Cleanup(func() {
		clawCreateSessionAttemptTimeout = oldAttemptTimeout
		clawCreateSessionRetryInterval = oldRetryInterval
		clawCreateSessionTotalTimeout = oldTotalTimeout
	})

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`{"data":[]}`))
			return
		}
		var body struct {
			Name string `json:"name"`
		}
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "opt", body.Name)

		n := calls.Add(1)
		if n < 3 {
			w.WriteHeader(499)
			_, _ = w.Write([]byte(`{"ok":false,"error":"client_closed_request"}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"session_id":"sess-1","agent_status":"running","message":{"message_id":"msg-1","dispatched":true}}}`))
	}))
	defer srv.Close()

	h := &Handler{clawClient: NewClawClient(srv.URL, "test-key")}
	res, err := h.createClawSessionWithRetry(context.Background(), "bearer", "task-1", &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "sess-1", res.SessionID)
	assert.Equal(t, int32(3), calls.Load())
}

func TestCreateClawSessionWithRetryReusesExistingSessionByName(t *testing.T) {
	oldRetryInterval := clawCreateSessionRetryInterval
	oldTotalTimeout := clawCreateSessionTotalTimeout
	oldLookupTimeout := clawCreateSessionLookupTimeout
	clawCreateSessionRetryInterval = time.Millisecond
	clawCreateSessionTotalTimeout = time.Second
	clawCreateSessionLookupTimeout = time.Second
	t.Cleanup(func() {
		clawCreateSessionRetryInterval = oldRetryInterval
		clawCreateSessionTotalTimeout = oldTotalTimeout
		clawCreateSessionLookupTimeout = oldLookupTimeout
	})

	var postCalls atomic.Int32
	var lookupCalls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			lookupCalls.Add(1)
			assert.Equal(t, "opt", r.URL.Query().Get("name"))
			_, _ = w.Write([]byte(`{"data":{"items":[{"session_id":"sess-existing","name":"opt","agent_status":"running"}]}}`))
		case http.MethodPost:
			postCalls.Add(1)
			w.WriteHeader(499)
			_, _ = w.Write([]byte(`{"ok":false,"error":"client_closed_request"}`))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	h := &Handler{clawClient: NewClawClient(srv.URL, "test-key")}
	res, err := h.createClawSessionWithRetry(context.Background(), "bearer", "task-1", &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.NoError(t, err)
	assert.Equal(t, "sess-existing", res.SessionID)
	assert.Equal(t, "running", res.AgentStatus)
	assert.Equal(t, int32(1), postCalls.Load())
	assert.Equal(t, int32(1), lookupCalls.Load())
}

func TestCreateClawSessionWithRetryDoesNotRetryBusinessFailure(t *testing.T) {
	oldRetryInterval := clawCreateSessionRetryInterval
	oldTotalTimeout := clawCreateSessionTotalTimeout
	clawCreateSessionRetryInterval = time.Millisecond
	clawCreateSessionTotalTimeout = time.Second
	t.Cleanup(func() {
		clawCreateSessionRetryInterval = oldRetryInterval
		clawCreateSessionTotalTimeout = oldTotalTimeout
	})

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"code":0,"data":{"session_id":"sess-1","agent_status":"idle","message":{"message_id":"msg-1","dispatched":false}}}`))
	}))
	defer srv.Close()

	h := &Handler{clawClient: NewClawClient(srv.URL, "test-key")}
	_, err := h.createClawSessionWithRetry(context.Background(), "bearer", "task-1", &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestCreateClawSessionWithRetryDoesNotRetryServerError(t *testing.T) {
	oldRetryInterval := clawCreateSessionRetryInterval
	oldTotalTimeout := clawCreateSessionTotalTimeout
	clawCreateSessionRetryInterval = time.Millisecond
	clawCreateSessionTotalTimeout = time.Second
	t.Cleanup(func() {
		clawCreateSessionRetryInterval = oldRetryInterval
		clawCreateSessionTotalTimeout = oldTotalTimeout
	})

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"error":"internal"}`))
	}))
	defer srv.Close()

	h := &Handler{clawClient: NewClawClient(srv.URL, "test-key")}
	_, err := h.createClawSessionWithRetry(context.Background(), "bearer", "task-1", &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.Error(t, err)
	assert.Equal(t, int32(1), calls.Load())
}

func TestCreateClawSessionWithRetryStopsOnParentCancellation(t *testing.T) {
	oldRetryInterval := clawCreateSessionRetryInterval
	oldTotalTimeout := clawCreateSessionTotalTimeout
	clawCreateSessionRetryInterval = 50 * time.Millisecond
	clawCreateSessionTotalTimeout = time.Second
	t.Cleanup(func() {
		clawCreateSessionRetryInterval = oldRetryInterval
		clawCreateSessionTotalTimeout = oldTotalTimeout
	})

	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		cancel()
		w.WriteHeader(499)
		_, _ = w.Write([]byte(`{"ok":false,"error":"client_closed_request"}`))
	}))
	defer srv.Close()

	h := &Handler{clawClient: NewClawClient(srv.URL, "test-key")}
	_, err := h.createClawSessionWithRetry(ctx, "bearer", "task-1", &SessionRequest{
		Name:    "opt",
		Message: &MessageRequest{Content: "hi"},
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), calls.Load())
}

func TestClawSendMessage(t *testing.T) {
	c, _ := newClawTestServer(t)
	err := c.SendMessage(context.Background(), "sess-1", &MessageRequest{Content: "hi"})
	assert.NoError(t, err)

	// Empty session id -> error.
	assert.Error(t, c.SendMessage(context.Background(), "", &MessageRequest{}))
}

func TestClawGetSession(t *testing.T) {
	c, _ := newClawTestServer(t)
	ss, err := c.GetSession(context.Background(), "sess-1")
	assert.NoError(t, err)
	assert.Equal(t, "completed", ss.Status)
	assert.True(t, ss.IsTerminal())

	assert.Error(t, func() error { _, e := c.GetSession(context.Background(), ""); return e }())
}

func TestClawDeleteSession(t *testing.T) {
	c, _ := newClawTestServer(t)
	assert.NoError(t, c.DeleteSession(context.Background(), "sess-1"))
	// Empty id is a no-op (no error).
	assert.NoError(t, c.DeleteSession(context.Background(), ""))
}

func TestClawInterruptSession(t *testing.T) {
	c, _ := newClawTestServer(t)
	assert.NoError(t, c.InterruptSession(context.Background(), "sess-1"))
	assert.Error(t, c.InterruptSession(context.Background(), ""))
}

func TestClawListSessionFiles(t *testing.T) {
	c, _ := newClawTestServer(t)
	files, err := c.ListSessionFiles(context.Background(), "sess-1")
	assert.NoError(t, err)
	assert.Len(t, files, 1)
	assert.Contains(t, files[0].Path, "optimization_report.md")

	_, err = c.ListSessionFiles(context.Background(), "")
	assert.Error(t, err)
}

func TestClawReadSessionFile(t *testing.T) {
	c, _ := newClawTestServer(t)
	data, err := c.ReadSessionFile(context.Background(), "sess-1", "claw-1/report.md")
	assert.NoError(t, err)
	assert.Equal(t, "file-content", string(data))

	_, err = c.ReadSessionFile(context.Background(), "sess-1", "")
	assert.Error(t, err)
}

func TestClawDownloadProxyPath(t *testing.T) {
	c, _ := newClawTestServer(t)
	path, err := c.DownloadProxyPath(context.Background(), "sess-1", "claw-1/report.md")
	assert.NoError(t, err)
	assert.Equal(t, "/proxy/download/x", path)

	_, err = c.DownloadProxyPath(context.Background(), "", "x")
	assert.Error(t, err)
}

func TestClawStream(t *testing.T) {
	// Dedicated server returning an SSE stream.
	mux := http.NewServeMux()
	var gotAfterEventID string
	mux.HandleFunc("/chat/sessions/sess-1/messages", func(w http.ResponseWriter, r *http.Request) {
		gotAfterEventID = r.URL.Query().Get("after_event_id")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("id: 1\nevent: phase\ndata: {\"phase\":1}\n\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := NewClawClient(srv.URL, "k")

	var events []ClawSSEEvent
	err := c.Stream(context.Background(), "sess-1", "", func(e ClawSSEEvent) error {
		events = append(events, e)
		return nil
	})
	assert.NoError(t, err)
	assert.Len(t, events, 1)
	assert.Equal(t, "phase", events[0].Event)

	events = nil
	err = c.Stream(context.Background(), "sess-1", "event+1/2", func(e ClawSSEEvent) error {
		events = append(events, e)
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "event+1/2", gotAfterEventID)

	// Empty session id -> error.
	assert.Error(t, c.Stream(context.Background(), "", "", func(ClawSSEEvent) error { return nil }))
}
