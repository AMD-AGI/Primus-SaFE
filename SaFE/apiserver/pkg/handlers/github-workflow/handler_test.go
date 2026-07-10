/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package githubworkflow

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newCtx(method, body string) (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	r := httptest.NewRequest(method, "/", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	c.Request = r
	return c, w
}

func TestRegisterRoutes(t *testing.T) {
	engine := gin.New()
	RegisterRoutes(&engine.RouterGroup)
	if len(engine.Routes()) < 14 {
		t.Errorf("expected at least 14 routes, got %d", len(engine.Routes()))
	}
}

func TestRegisterRoutesRequiresAuthorization(t *testing.T) {
	oldGetDB := getDB
	dbCalled := false
	getDB = func() *sql.DB {
		dbCalled = true
		return nil
	}
	defer func() { getDB = oldGetDB }()

	engine := gin.New()
	RegisterRoutes(&engine.RouterGroup)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/github-workflow/stats", nil)
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("status = %d, want authorization failure", w.Code)
	}
	if dbCalled {
		t.Fatal("unauthorized request reached DB handler")
	}
}

// TestHandleCreateConfig covers the request-validation branch (no DB access).
func TestHandleCreateConfig(t *testing.T) {
	c, w := newCtx(http.MethodPost, `{}`)
	handleCreateConfig(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// TestHandleUpdateConfig covers the malformed-JSON branch (no DB access).
func TestHandleUpdateConfig(t *testing.T) {
	c, w := newCtx(http.MethodPut, `{not-json`)
	handleUpdateConfig(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestNs(t *testing.T) {
	if got := ns(sql.NullString{String: "x", Valid: true}); got != "x" {
		t.Errorf("valid: got %q", got)
	}
	if got := ns(sql.NullString{Valid: false}); got != "" {
		t.Errorf("invalid: got %q", got)
	}
}

func TestNi(t *testing.T) {
	if got := ni(sql.NullInt64{Int64: 42, Valid: true}); got != 42 {
		t.Errorf("valid: got %d", got)
	}
	if got := ni(sql.NullInt64{Valid: false}); got != 0 {
		t.Errorf("invalid: got %d", got)
	}
}

func TestPgArr(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"{a,b,c}", []string{"a", "b", "c"}},
		{`{"a","b"}`, []string{"a", "b"}},
		{"{}", []string{}},
		{"", []string{}},
	}
	for _, tc := range cases {
		if got := pgArr(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("pgArr(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestSliceToArr(t *testing.T) {
	if got := sliceToArr(nil); got != "{}" {
		t.Errorf("empty: got %q", got)
	}
	if got := sliceToArr([]string{"a", "b"}); got != "{a,b}" {
		t.Errorf("got %q, want {a,b}", got)
	}
}
