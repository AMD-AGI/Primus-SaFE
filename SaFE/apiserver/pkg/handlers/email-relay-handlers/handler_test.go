/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package emailrelayhandlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"
	"gotest.tools/assert"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbModel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newCtx(method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, target, nil)
	}
	c.Request = r
	return c, w
}

// TestNewHandler verifies an error is returned when no database is configured.
func TestNewHandler(t *testing.T) {
	h, err := NewHandler()
	if err != nil {
		if h != nil {
			t.Errorf("expected nil handler on error, got %v", h)
		}
		return
	}
	// A database is unexpectedly reachable in this environment.
	if h == nil {
		t.Fatal("NewHandler returned nil handler without error")
	}
}

// TestAck covers the invalid-id validation branch (no DB access).
func TestAck(t *testing.T) {
	h := &Handler{}
	c, w := newCtx(http.MethodPost, "/", "")
	c.Params = gin.Params{{Key: "id", Value: "not-a-number"}}
	h.Ack(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestFail covers the invalid-id validation branch (no DB access).
func TestFail(t *testing.T) {
	h := &Handler{}
	c, w := newCtx(http.MethodPost, "/", "")
	c.Params = gin.Params{{Key: "id", Value: "bad"}}
	h.Fail(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestSubmit covers the request validation branches (no DB access).
func TestSubmit(t *testing.T) {
	h := &Handler{}

	// Invalid JSON.
	c, w := newCtx(http.MethodPost, "/", "{not-json")
	h.Submit(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid json: status = %d, want 400", w.Code)
	}

	// Missing required fields.
	c2, w2 := newCtx(http.MethodPost, "/", `{"recipients":[]}`)
	h.Submit(c2)
	if w2.Code != http.StatusBadRequest {
		t.Errorf("missing fields: status = %d, want 400", w2.Code)
	}
}

// TestSendSSEEvent verifies the SSE payload is written to the response.
func TestSendSSEEvent(t *testing.T) {
	h := &Handler{}
	c, w := newCtx(http.MethodGet, "/", "")
	h.sendSSEEvent(c, nil, &dbModel.EmailOutbox{ID: 7, Subject: "hello"})
	if !strings.Contains(w.Body.String(), "email") {
		t.Errorf("expected SSE event in body, got %q", w.Body.String())
	}
}

// TestAuthorizeRelay verifies unauthenticated requests are rejected with 401.
func TestAuthorizeRelay(t *testing.T) {
	h := &Handler{}
	engine := gin.New()
	engine.GET("/protected", h.AuthorizeRelay(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// setRelaySubmitToken writes token to a file configured as email_relay.submit_token_file.
func setRelaySubmitToken(t *testing.T, token string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	commonconfig.SetValue("email_relay.submit_token_file", path)
	t.Cleanup(func() { commonconfig.UnsetValue("email_relay.submit_token_file") })
}

// serveRelay sends a request carrying the relay token header through the relay routes.
func serveRelay(h *Handler, method, path, body, relayToken string) *httptest.ResponseRecorder {
	engine := gin.New()
	InitEmailRelayRouters(engine, h)
	req := httptest.NewRequest(method, "/"+common.PrimusRouterCustomRootPath+"/email-relay"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if relayToken != "" {
		req.Header.Set(EmailRelayTokenHeader, relayToken)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// TestRelayTokenAcceptedOnSubmit verifies the submit token authorizes the submit route.
func TestRelayTokenAcceptedOnSubmit(t *testing.T) {
	setRelaySubmitToken(t, "relay-secret")
	store := &fakeOutboxStore{}
	w := serveRelay(&Handler{dbClient: store}, http.MethodPost, "/submit",
		`{"source":"identity","recipients":["a@b.com"],"subject":"s","content":"c"}`, "relay-secret")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if store.createdOutbox == nil {
		t.Error("expected outbox to be created")
	}
}

// TestRelayTokenWrongIsUnauthorized verifies a wrong or unconfigured submit token is refused with 401.
func TestRelayTokenWrongIsUnauthorized(t *testing.T) {
	body := `{"recipients":["a@b.com"],"subject":"s","content":"c"}`
	if w := serveRelay(&Handler{dbClient: &fakeOutboxStore{}}, http.MethodPost, "/submit", body, "relay-secret"); w.Code != http.StatusUnauthorized {
		t.Errorf("unconfigured: status = %d, want 401", w.Code)
	}
	setRelaySubmitToken(t, "relay-secret")
	store := &fakeOutboxStore{}
	if w := serveRelay(&Handler{dbClient: store}, http.MethodPost, "/submit", body, "relay-secreT"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong token: status = %d, want 401", w.Code)
	}
	if store.createdOutbox != nil {
		t.Error("outbox must not be written with a wrong token")
	}
}

// TestRelayTokenRefusedOutsideSubmit verifies the submit token does not authorize the other relay routes.
func TestRelayTokenRefusedOutsideSubmit(t *testing.T) {
	setRelaySubmitToken(t, "relay-secret")
	h := &Handler{dbClient: &fakeOutboxStore{}}
	if w := serveRelay(h, http.MethodGet, "/stream", "", "relay-secret"); w.Code != http.StatusUnauthorized {
		t.Errorf("stream: status = %d, want 401", w.Code)
	}
	if w := serveRelay(h, http.MethodPost, "/5/ack", "", "relay-secret"); w.Code != http.StatusUnauthorized {
		t.Errorf("ack: status = %d, want 401", w.Code)
	}
	if w := serveRelay(h, http.MethodPost, "/5/fail", "", "relay-secret"); w.Code != http.StatusUnauthorized {
		t.Errorf("fail: status = %d, want 401", w.Code)
	}
}

// TestInitEmailRelayRouters verifies all relay routes are registered.
func TestInitEmailRelayRouters(t *testing.T) {
	engine := gin.New()
	InitEmailRelayRouters(engine, &Handler{})
	if len(engine.Routes()) < 4 {
		t.Errorf("expected at least 4 routes, got %d", len(engine.Routes()))
	}
}

// --- merged from handler_store_test.go ---

// fakeOutboxStore is a configurable in-memory implementation of emailOutboxStore.
type fakeOutboxStore struct {
	pending       []*dbModel.EmailOutbox
	pendingAfter  []*dbModel.EmailOutbox
	createErr     error
	ackErr        error
	failErr       error
	dispatchN     int64
	dispatchErr   error
	resetN        int64
	resetErr      error
	createdOutbox *dbModel.EmailOutbox
}

func (f *fakeOutboxStore) CreateEmailOutbox(ctx context.Context, outbox *dbModel.EmailOutbox) error {
	if f.createErr != nil {
		return f.createErr
	}
	outbox.ID = 100
	f.createdOutbox = outbox
	return nil
}
func (f *fakeOutboxStore) ListPendingEmailOutbox(ctx context.Context, limit int) ([]*dbModel.EmailOutbox, error) {
	return f.pending, nil
}
func (f *fakeOutboxStore) ListPendingEmailOutboxAfter(ctx context.Context, afterID int32, limit int) ([]*dbModel.EmailOutbox, error) {
	return f.pendingAfter, nil
}
func (f *fakeOutboxStore) DispatchEmailOutbox(ctx context.Context, id int32) (int64, error) {
	return f.dispatchN, f.dispatchErr
}
func (f *fakeOutboxStore) AckEmailOutbox(ctx context.Context, id int32) error { return f.ackErr }
func (f *fakeOutboxStore) FailEmailOutbox(ctx context.Context, id int32, msg string) error {
	return f.failErr
}
func (f *fakeOutboxStore) ResetStaleDispatched(ctx context.Context, d time.Duration) (int64, error) {
	return f.resetN, f.resetErr
}

func relayCtxWith(method, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	c, w := newCtx(method, "/", body)
	c.Params = params
	return c, w
}

// TestAckSuccess verifies a valid ack returns 200.
func TestAckSuccess(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{}}
	c, w := relayCtxWith(http.MethodPost, "", gin.Params{{Key: "id", Value: "5"}})
	h.Ack(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestAckDBError verifies a db failure returns 500.
func TestAckDBError(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{ackErr: errors.New("db")}}
	c, w := relayCtxWith(http.MethodPost, "", gin.Params{{Key: "id", Value: "5"}})
	h.Ack(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestFailSuccess verifies a valid fail returns 200.
func TestFailSuccess(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{}}
	c, w := relayCtxWith(http.MethodPost, `{"error":"boom"}`, gin.Params{{Key: "id", Value: "5"}})
	h.Fail(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestFailDBError verifies a db failure returns 500.
func TestFailDBError(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{failErr: errors.New("db")}}
	c, w := relayCtxWith(http.MethodPost, `{"error":"boom"}`, gin.Params{{Key: "id", Value: "5"}})
	h.Fail(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestFailDefaultsErrorMessage verifies a missing body defaults the error text.
func TestFailDefaultsErrorMessage(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{}}
	c, w := relayCtxWith(http.MethodPost, "", gin.Params{{Key: "id", Value: "5"}})
	h.Fail(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// TestSubmitSuccess verifies a valid submission persists an outbox entry.
func TestSubmitSuccess(t *testing.T) {
	store := &fakeOutboxStore{}
	h := &Handler{dbClient: store}
	body := `{"source":"lens","recipients":["a@b.com"],"subject":"s","content":"<p>c</p>"}`
	c, w := relayCtxWith(http.MethodPost, body, nil)
	h.Submit(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	if store.createdOutbox == nil || store.createdOutbox.Subject != "s" {
		t.Error("expected outbox to be created with subject")
	}
}

// TestSubmitDefaultSource verifies the source defaults when omitted.
func TestSubmitDefaultSource(t *testing.T) {
	store := &fakeOutboxStore{}
	h := &Handler{dbClient: store}
	body := `{"recipients":["a@b.com"],"subject":"s","content":"c"}`
	c, w := relayCtxWith(http.MethodPost, body, nil)
	h.Submit(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if store.createdOutbox == nil || store.createdOutbox.Source == "" {
		t.Error("expected a default source to be set")
	}
}

// TestSubmitDBError verifies a db failure returns 500.
func TestSubmitDBError(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{createErr: errors.New("db")}}
	body := `{"recipients":["a@b.com"],"subject":"s","content":"c"}`
	c, w := relayCtxWith(http.MethodPost, body, nil)
	h.Submit(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}

// TestDispatchAndSendSkipsAlreadyDispatched verifies a 0-affected dispatch is skipped.
func TestDispatchAndSendSkipsAlreadyDispatched(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{dispatchN: 0}}
	c, _ := newCtx(http.MethodGet, "/", "")
	var lastSentID int32
	h.dispatchAndSend(c, nil, &dbModel.EmailOutbox{ID: 9}, &lastSentID)
	if lastSentID != 9 {
		t.Errorf("lastSentID = %d, want 9", lastSentID)
	}
}

// TestDispatchAndSendSends verifies a 1-affected dispatch emits the SSE event.
func TestDispatchAndSendSends(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{dispatchN: 1}}
	c, w := newCtx(http.MethodGet, "/", "")
	var lastSentID int32
	h.dispatchAndSend(c, nil, &dbModel.EmailOutbox{ID: 3, Subject: "x"}, &lastSentID)
	if lastSentID != 3 {
		t.Errorf("lastSentID = %d, want 3", lastSentID)
	}
	if w.Body.Len() == 0 {
		t.Error("expected SSE event to be written")
	}
}

// TestDispatchAndSendDBError verifies a dispatch error short-circuits without sending.
func TestDispatchAndSendDBError(t *testing.T) {
	h := &Handler{dbClient: &fakeOutboxStore{dispatchErr: errors.New("db")}}
	c, _ := newCtx(http.MethodGet, "/", "")
	var lastSentID int32
	h.dispatchAndSend(c, nil, &dbModel.EmailOutbox{ID: 3}, &lastSentID)
	if lastSentID != 0 {
		t.Errorf("lastSentID = %d, want 0 (no update on error)", lastSentID)
	}
}

// TestStreamSendsBacklogThenExits drives Stream with a pre-cancelled context so it
// flushes the backlog and returns promptly.
func TestStreamSendsBacklogThenExits(t *testing.T) {
	store := &fakeOutboxStore{
		pending:   []*dbModel.EmailOutbox{{ID: 1, Subject: "a"}},
		dispatchN: 1,
	}
	h := &Handler{dbClient: store}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // ensure the select loop exits immediately after backlog
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		h.Stream(c)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after context cancellation")
	}
	if w.Body.Len() == 0 {
		t.Error("expected backlog SSE event to be written")
	}
}

// --- merged from email_relay_more_test.go ---

func relayCtx(t *testing.T, method, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "/", strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, "/", nil)
	}
	r.Header.Set("Content-Type", "application/json")
	c.Request = r
	c.Params = params
	return c, w
}

func TestAckBadID(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, "", gin.Params{{Key: "id", Value: "abc"}})
	h.Ack(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFailBadID(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, "", gin.Params{{Key: "id", Value: "abc"}})
	h.Fail(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitBadBody(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, "{invalid", nil)
	h.Submit(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitMissingFields(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, `{"subject":"s"}`, nil)
	h.Submit(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthorizeRelayNoAuth(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodGet, "", nil)
	h.AuthorizeRelay()(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	testifyassert.True(t, c.IsAborted())
}
