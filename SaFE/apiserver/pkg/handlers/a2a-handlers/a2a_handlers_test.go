/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2ahandlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// mockDB embeds dbclient.Interface so only the methods exercised by the A2A
// handlers need an implementation; other calls would panic if invoked.
type mockDB struct {
	dbclient.Interface

	callLogs      []*dbclient.A2ACallLog
	callLogsErr   error
	callLogCnt    int
	callLogCntErr error
	apiKeys       []*dbclient.ApiKey

	services      []*dbclient.A2AServiceRegistry
	servicesErr   error
	serviceCnt    int
	serviceCntErr error
	service       *dbclient.A2AServiceRegistry
	serviceErr    error
	activeSvc     []*dbclient.A2AServiceRegistry
	activeErr     error

	upsertErr error
	deleteErr error
}

func (m *mockDB) SelectA2ACallLogs(_ context.Context, _ sqrl.Sqlizer, _ []string, _, _ int) ([]*dbclient.A2ACallLog, error) {
	return m.callLogs, m.callLogsErr
}
func (m *mockDB) CountA2ACallLogs(_ context.Context, _ sqrl.Sqlizer) (int, error) {
	return m.callLogCnt, m.callLogCntErr
}
func (m *mockDB) SelectApiKeys(_ context.Context, _ sqrl.Sqlizer, _ []string, _, _ int) ([]*dbclient.ApiKey, error) {
	return m.apiKeys, nil
}
func (m *mockDB) UpsertA2AService(_ context.Context, _ *dbclient.A2AServiceRegistry) error {
	return m.upsertErr
}
func (m *mockDB) SelectA2AServices(_ context.Context, _ sqrl.Sqlizer, _ []string, _, _ int) ([]*dbclient.A2AServiceRegistry, error) {
	return m.services, m.servicesErr
}
func (m *mockDB) CountA2AServices(_ context.Context, _ sqrl.Sqlizer) (int, error) {
	return m.serviceCnt, m.serviceCntErr
}
func (m *mockDB) GetA2AService(_ context.Context, _ string) (*dbclient.A2AServiceRegistry, error) {
	return m.service, m.serviceErr
}
func (m *mockDB) SetA2AServiceDeleted(_ context.Context, _ string) error {
	return m.deleteErr
}
func (m *mockDB) ListActiveA2AServices(_ context.Context) ([]*dbclient.A2AServiceRegistry, error) {
	return m.activeSvc, m.activeErr
}

func init() {
	gin.SetMode(gin.TestMode)
}

// newCtx builds a gin test context backed by an HTTP request and recorder.
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

func newA2AAccessController(t *testing.T, verbs ...v1.RoleVerb) *authority.AccessController {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	role := &v1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "test-role"},
		Rules: []v1.PolicyRule{{
			Resources:    []string{"a2a"},
			GrantedUsers: []string{authority.GrantedAllUser},
			Verbs:        verbs,
		}},
	}
	user := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user-1"},
		Spec: v1.UserSpec{
			Type:  v1.DefaultUserType,
			Roles: []v1.UserRole{"test-role"},
		},
	}
	return &authority.AccessController{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(role, user).Build()}
}

// newA2AAdminAC seeds a system administrator (admin-1) and a plain user
// (user-1) so audit-read authorization (system-admin only) can be exercised.
func newA2AAdminAC(t *testing.T) *authority.AccessController {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	admin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.SystemAdminRole}},
	}
	nonAdmin := &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: "user-1"},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType},
	}
	return &authority.AccessController{Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(admin, nonAdmin).Build()}
}

func withUser(c *gin.Context) {
	c.Set(common.UserId, "user-1")
	c.Set(common.UserName, "alice")
}

func TestNewHandler(t *testing.T) {
	if NewHandler(&mockDB{}) == nil {
		t.Fatal("expected non-nil handler")
	}
}

func TestToCallLogView(t *testing.T) {
	now := time.Now()
	log := &dbclient.A2ACallLog{
		Id:           1,
		TraceId:      "t1",
		CallerUserId: "u1",
		ErrorMessage: sql.NullString{String: "boom", Valid: true},
		CreatedAt:    pq.NullTime{Time: now, Valid: true},
	}
	v := toCallLogView(log)
	if v.Id != 1 || v.ErrorMessage != "boom" || v.CreatedAt == nil {
		t.Errorf("unexpected view: %+v", v)
	}
}

func TestToServiceView(t *testing.T) {
	now := time.Now()
	svc := &dbclient.A2AServiceRegistry{
		Id:           2,
		ServiceName:  "svc",
		WorkloadId:   sql.NullString{String: "w1", Valid: true},
		A2AAgentCard: sql.NullString{String: "card", Valid: true},
		A2ASkills:    sql.NullString{String: "skills", Valid: true},
		A2ALastSeen:  pq.NullTime{Time: now, Valid: true},
		K8sNamespace: sql.NullString{String: "ns", Valid: true},
		K8sService:   sql.NullString{String: "ksvc", Valid: true},
		CreatedBy:    sql.NullString{String: "admin", Valid: true},
		CreatedAt:    pq.NullTime{Time: now, Valid: true},
		UpdatedAt:    pq.NullTime{Time: now, Valid: true},
	}
	v := toServiceView(svc)
	if v.WorkloadId != "w1" || v.A2AAgentCard != "card" || v.A2ASkills != "skills" ||
		v.K8sNamespace != "ns" || v.K8sService != "ksvc" || v.CreatedBy != "admin" ||
		v.A2ALastSeen == nil || v.CreatedAt == nil || v.UpdatedAt == nil {
		t.Errorf("unexpected view: %+v", v)
	}
}

func TestListCallLogs(t *testing.T) {
	adminAC := newA2AAdminAC(t)
	db := &mockDB{
		callLogs:   []*dbclient.A2ACallLog{{Id: 1, CallerUserId: "u1"}},
		callLogCnt: 1,
		apiKeys:    []*dbclient.ApiKey{{UserId: "u1", UserName: "alice"}},
	}
	h := NewHandler(db, adminAC)
	c, w := newCtx(http.MethodGet, "/?caller=foo&target=bar&limit=10&offset=0", "")
	c.Set(common.UserId, "admin-1")
	h.ListCallLogs(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "alice") {
		t.Errorf("expected caller user name in body: %s", w.Body.String())
	}

	// DB error path (authorized admin).
	dbErr := &mockDB{callLogsErr: errors.New("db down")}
	c2, w2 := newCtx(http.MethodGet, "/", "")
	c2.Set(common.UserId, "admin-1")
	NewHandler(dbErr, adminAC).ListCallLogs(c2)
	if w2.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w2.Code)
	}

	countErr := &mockDB{callLogCntErr: errors.New("count down")}
	c3, w3 := newCtx(http.MethodGet, "/", "")
	c3.Set(common.UserId, "admin-1")
	NewHandler(countErr, adminAC).ListCallLogs(c3)
	if w3.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w3.Code)
	}
}

// TestListCallLogsForbiddenForNonAdmin verifies #10: A2A call-log (audit) reads
// are restricted to system administrators; a plain user receives 403.
func TestListCallLogsForbiddenForNonAdmin(t *testing.T) {
	db := &mockDB{callLogs: []*dbclient.A2ACallLog{{Id: 1}}, callLogCnt: 1}
	h := NewHandler(db, newA2AAdminAC(t))
	c, w := newCtx(http.MethodGet, "/", "")
	c.Set(common.UserId, "user-1") // seeded as a non-admin user
	h.ListCallLogs(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestCreateService(t *testing.T) {
	h := NewHandler(&mockDB{}, newA2AAccessController(t, v1.CreateVerb))
	c, w := newCtx(http.MethodPost, "/", `{"serviceName":"svc","endpoint":"http://x"}`)
	withUser(c)
	h.CreateService(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}

	// Bad request: missing required fields.
	c2, w2 := newCtx(http.MethodPost, "/", `{}`)
	withUser(c2)
	h.CreateService(c2)
	if w2.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w2.Code)
	}
}

func TestListServices(t *testing.T) {
	db := &mockDB{services: []*dbclient.A2AServiceRegistry{{ServiceName: "svc"}}, serviceCnt: 1}
	c, w := newCtx(http.MethodGet, "/?status=active&limit=10", "")
	NewHandler(db).ListServices(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	dbErr := &mockDB{servicesErr: errors.New("db down")}
	c2, w2 := newCtx(http.MethodGet, "/", "")
	NewHandler(dbErr).ListServices(c2)
	if w2.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w2.Code)
	}

	countErr := &mockDB{serviceCntErr: errors.New("count down")}
	c3, w3 := newCtx(http.MethodGet, "/", "")
	NewHandler(countErr).ListServices(c3)
	if w3.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w3.Code)
	}
}

func TestGetService(t *testing.T) {
	db := &mockDB{service: &dbclient.A2AServiceRegistry{ServiceName: "svc"}}
	c, w := newCtx(http.MethodGet, "/", "")
	c.Params = gin.Params{{Key: "serviceName", Value: "svc"}}
	NewHandler(db).GetService(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	dbErr := &mockDB{serviceErr: errors.New("not found")}
	c2, w2 := newCtx(http.MethodGet, "/", "")
	c2.Params = gin.Params{{Key: "serviceName", Value: "missing"}}
	NewHandler(dbErr).GetService(c2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w2.Code)
	}
}

func TestUpdateService(t *testing.T) {
	db := &mockDB{service: &dbclient.A2AServiceRegistry{ServiceName: "svc"}}
	h := NewHandler(db, newA2AAccessController(t, v1.UpdateVerb))
	c, w := newCtx(http.MethodPatch, "/", `{"displayName":"New","description":"d","endpoint":"http://y","a2aPathPrefix":"/a2a","status":"inactive"}`)
	withUser(c)
	c.Params = gin.Params{{Key: "serviceName", Value: "svc"}}
	h.UpdateService(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	// Not found path.
	dbErr := &mockDB{serviceErr: errors.New("not found")}
	c2, w2 := newCtx(http.MethodPatch, "/", `{}`)
	withUser(c2)
	c2.Params = gin.Params{{Key: "serviceName", Value: "missing"}}
	NewHandler(dbErr, newA2AAccessController(t, v1.UpdateVerb)).UpdateService(c2)
	if w2.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w2.Code)
	}
}

func TestDeleteService(t *testing.T) {
	c, w := newCtx(http.MethodDelete, "/", "")
	withUser(c)
	c.Params = gin.Params{{Key: "serviceName", Value: "svc"}}
	NewHandler(&mockDB{}, newA2AAccessController(t, v1.DeleteVerb)).DeleteService(c)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestWriteServiceRequiresA2AAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
		call   func(*Handler, *gin.Context)
	}{
		{
			name:   "create",
			method: http.MethodPost,
			body:   `{"serviceName":"svc","endpoint":"http://x"}`,
			call:   (*Handler).CreateService,
		},
		{
			name:   "update",
			method: http.MethodPatch,
			body:   `{"displayName":"New"}`,
			call:   (*Handler).UpdateService,
		},
		{
			name:   "delete",
			method: http.MethodDelete,
			call:   (*Handler).DeleteService,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := &mockDB{service: &dbclient.A2AServiceRegistry{ServiceName: "svc"}}
			h := NewHandler(db, newA2AAccessController(t, v1.GetVerb))
			c, w := newCtx(tt.method, "/", tt.body)
			withUser(c)
			c.Params = gin.Params{{Key: "serviceName", Value: "svc"}}
			tt.call(h, c)
			if w.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403, body: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetTopology(t *testing.T) {
	adminAC := newA2AAdminAC(t)
	db := &mockDB{
		activeSvc: []*dbclient.A2AServiceRegistry{{ServiceName: "a"}, {ServiceName: "b"}},
		callLogs: []*dbclient.A2ACallLog{
			{CallerServiceName: "a", TargetServiceName: "b"},
			{CallerServiceName: "a", TargetServiceName: "b"},
		},
	}
	c, w := newCtx(http.MethodGet, "/", "")
	c.Set(common.UserId, "admin-1")
	NewHandler(db, adminAC).GetTopology(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	dbErr := &mockDB{activeErr: errors.New("db down")}
	c2, w2 := newCtx(http.MethodGet, "/", "")
	c2.Set(common.UserId, "admin-1")
	NewHandler(dbErr, adminAC).GetTopology(c2)
	if w2.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w2.Code)
	}
}

// TestGetTopologyForbiddenForNonAdmin verifies that the topology view (derived
// from call-log audit data) is restricted to system administrators.
func TestGetTopologyForbiddenForNonAdmin(t *testing.T) {
	db := &mockDB{activeSvc: []*dbclient.A2AServiceRegistry{{ServiceName: "a"}}}
	h := NewHandler(db, newA2AAdminAC(t))
	c, w := newCtx(http.MethodGet, "/", "")
	c.Set(common.UserId, "user-1") // seeded as a non-admin user
	h.GetTopology(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
}

func TestInitA2ARouters(t *testing.T) {
	engine := gin.New()
	InitA2ARouters(engine, NewHandler(&mockDB{}))
	routes := engine.Routes()
	if len(routes) < 7 {
		t.Errorf("expected at least 7 routes registered, got %d", len(routes))
	}
}

// TestA2AReadsOpenToNonAdmin guards #10: A2A service list/get and topology stay
// readable by authenticated non-admins; only call-logs (audit) are restricted.
func TestA2AReadsOpenToNonAdmin(t *testing.T) {
	h := NewHandler(&mockDB{}, newA2AAdminAC(t)) // seeds admin-1 + non-admin user-1

	t.Run("ListServices", func(t *testing.T) {
		c, w := newCtx(http.MethodGet, "/", "")
		c.Set(common.UserId, "user-1")
		h.ListServices(c)
		if w.Code != http.StatusOK {
			t.Fatalf("ListServices status = %d, want 200 (must stay open to non-admin)", w.Code)
		}
	})

	t.Run("GetService", func(t *testing.T) {
		svcDB := &mockDB{service: &dbclient.A2AServiceRegistry{ServiceName: "svc"}}
		hs := NewHandler(svcDB, newA2AAdminAC(t))
		c, w := newCtx(http.MethodGet, "/", "")
		c.Params = gin.Params{{Key: "serviceName", Value: "svc"}}
		c.Set(common.UserId, "user-1")
		hs.GetService(c)
		if w.Code != http.StatusOK {
			t.Fatalf("GetService status = %d, want 200 (must stay open to non-admin)", w.Code)
		}
	})
}
