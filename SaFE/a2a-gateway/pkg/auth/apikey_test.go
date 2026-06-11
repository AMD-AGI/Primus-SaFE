/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// mockDB embeds the dbclient.Interface so only the methods used by the auth
// middleware need to be implemented; any other call would panic.
type mockDB struct {
	dbclient.Interface
	apiKey *dbclient.ApiKey
	err    error
}

func (m *mockDB) GetApiKeyByKey(_ context.Context, _ string) (*dbclient.ApiKey, error) {
	return m.apiKey, m.err
}

func init() {
	gin.SetMode(gin.TestMode)
}

// TestApiKeyMiddleware verifies the auth outcomes for each request path.
func TestApiKeyMiddleware(t *testing.T) {
	cases := []struct {
		name       string
		authHeader string
		db         *mockDB
		wantStatus int
		wantNext   bool
	}{
		{
			name:       "missing header",
			authHeader: "",
			db:         &mockDB{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid format",
			authHeader: "Bearer token-without-prefix",
			db:         &mockDB{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "key lookup fails",
			authHeader: "Bearer ak-bad",
			db:         &mockDB{apiKey: nil, err: context.Canceled},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "revoked key",
			authHeader: "Bearer ak-revoked",
			db:         &mockDB{apiKey: &dbclient.ApiKey{Deleted: true}},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid key",
			authHeader: "Bearer ak-good",
			db:         &mockDB{apiKey: &dbclient.ApiKey{UserId: "u1", UserName: "alice"}},
			wantStatus: http.StatusOK,
			wantNext:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			engine := gin.New()
			engine.GET("/", ApiKeyMiddleware(tc.db), func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if nextCalled != tc.wantNext {
				t.Errorf("next called = %v, want %v", nextCalled, tc.wantNext)
			}
		})
	}
}

// TestGetUserID covers presence and absence of the user id in the context.
func TestGetUserID(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := GetUserID(c); got != "" {
		t.Errorf("expected empty user id, got %q", got)
	}
	c.Set(contextKeyUserID, "user-123")
	if got := GetUserID(c); got != "user-123" {
		t.Errorf("expected user-123, got %q", got)
	}
}

// TestGetUserName covers presence and absence of the user name in the context.
func TestGetUserName(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := GetUserName(c); got != "" {
		t.Errorf("expected empty user name, got %q", got)
	}
	c.Set(contextKeyUserName, "bob")
	if got := GetUserName(c); got != "bob" {
		t.Errorf("expected bob, got %q", got)
	}
}
