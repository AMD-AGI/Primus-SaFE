/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package proxy

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// mockDB embeds dbclient.Interface and only implements the methods used by the
// proxy handler; unimplemented methods would panic if called.
type mockDB struct {
	dbclient.Interface
	svc        *dbclient.A2AServiceRegistry
	svcErr     error
	activeList []*dbclient.A2AServiceRegistry
	activeErr  error
	insertErr  error
}

func (m *mockDB) GetA2AService(_ context.Context, _ string) (*dbclient.A2AServiceRegistry, error) {
	return m.svc, m.svcErr
}

func (m *mockDB) ListActiveA2AServices(_ context.Context) ([]*dbclient.A2AServiceRegistry, error) {
	return m.activeList, m.activeErr
}

func (m *mockDB) InsertA2ACallLog(_ context.Context, _ *dbclient.A2ACallLog) error {
	return m.insertErr
}

func init() {
	gin.SetMode(gin.TestMode)
}

func newEngine(h *Handler) *gin.Engine {
	engine := gin.New()
	engine.POST("/a2a/invoke/:target", h.Invoke)
	engine.POST("/a2a/invoke/:target/:skill", h.Invoke)
	engine.GET("/a2a/agents", h.ListAgents)
	return engine
}

// TestInvoke covers not-found, inactive and the successful proxy path.
func TestInvoke(t *testing.T) {
	// not found
	t.Run("not found", func(t *testing.T) {
		h := NewHandler(&mockDB{svc: nil, svcErr: errors.New("missing")})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/a2a/invoke/agentx", strings.NewReader("{}"))
		newEngine(h).ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	// inactive
	t.Run("inactive", func(t *testing.T) {
		h := NewHandler(&mockDB{svc: &dbclient.A2AServiceRegistry{Status: "inactive"}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/a2a/invoke/agentx", strings.NewReader("{}"))
		newEngine(h).ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", w.Code)
		}
	})

	// success: proxy to a backend test server
	t.Run("success", func(t *testing.T) {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"ok":true}`))
		}))
		defer backend.Close()

		h := NewHandler(&mockDB{svc: &dbclient.A2AServiceRegistry{
			Status:        "active",
			Endpoint:      backend.URL,
			A2APathPrefix: "",
		}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/a2a/invoke/agentx/chat", strings.NewReader("{}"))
		newEngine(h).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "ok") {
			t.Errorf("unexpected body: %s", w.Body.String())
		}
	})
}

// TestListAgents covers the success and DB-error paths.
func TestListAgents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h := NewHandler(&mockDB{activeList: []*dbclient.A2AServiceRegistry{
			{ServiceName: "svc1", DisplayName: "Service 1", Endpoint: "http://svc1", A2AHealth: "healthy"},
		}})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/a2a/agents", nil)
		newEngine(h).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("status = %d, want 200", w.Code)
		}
		if !strings.Contains(w.Body.String(), "svc1") {
			t.Errorf("unexpected body: %s", w.Body.String())
		}
	})

	t.Run("db error", func(t *testing.T) {
		h := NewHandler(&mockDB{activeErr: errors.New("db down")})
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/a2a/agents", nil)
		newEngine(h).ServeHTTP(w, req)
		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}
