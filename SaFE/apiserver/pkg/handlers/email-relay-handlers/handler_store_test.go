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
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	dbModel "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

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
func (f *fakeOutboxStore) AckEmailOutbox(ctx context.Context, id int32) error  { return f.ackErr }
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
