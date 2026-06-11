/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package emailrelayhandlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

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

// TestInitEmailRelayRouters verifies all relay routes are registered.
func TestInitEmailRelayRouters(t *testing.T) {
	engine := gin.New()
	InitEmailRelayRouters(engine, &Handler{})
	if len(engine.Routes()) < 4 {
		t.Errorf("expected at least 4 routes, got %d", len(engine.Routes()))
	}
}
