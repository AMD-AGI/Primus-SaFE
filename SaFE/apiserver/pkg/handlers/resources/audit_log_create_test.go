/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

const testAuditInternalToken = "internal-token-for-audit-tests"

// withInternalToken makes testAuditInternalToken the internal service token for one test.
func withInternalToken(t *testing.T) {
	old := internalTokenValid
	internalTokenValid = func(token string) bool { return token == testAuditInternalToken }
	t.Cleanup(func() { internalTokenValid = old })
}

// apiKeyCreation is the report the identity service sends for an API key created from the console.
func apiKeyCreation(at time.Time) map[string]any {
	return map[string]any{
		"userId":         "be24fda013465da928ffc6756959c4ce",
		"userName":       "claw-canary-user",
		"userType":       "default",
		"clientIp":       "10.255.36.101",
		"httpMethod":     "POST",
		"requestPath":    "/api/v1/apikeys",
		"resourceType":   "apikey",
		"action":         "create apikey",
		"requestBody":    `{"name":"ci","ttlDays":1}`,
		"responseStatus": 200,
		"responseBody":   `{"id":211,"name":"ci","apiKey":"ak-0123456789abcdef","expirationTime":"2026-11-07T10:00:00"}`,
		"latencyMs":      12,
		"createTime":     at.Format(time.RFC3339Nano),
	}
}

func postAuditLog(t *testing.T, engine *gin.Engine, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	switch b := body.(type) {
	case string:
		raw = []byte(b)
	default:
		var err error
		raw, err = json.Marshal(b)
		require.NoError(t, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/"+common.PrimusRouterCustomRootPath+"/auditlogs", strings.NewReader(string(raw)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set(authority.InternalAuthTokenHeader, token)
	}
	rsp := httptest.NewRecorder()
	engine.ServeHTTP(rsp, req)
	return rsp
}

func auditEngine(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	InitCustomRouters(engine, h)
	return engine
}

func TestCreateAuditLogStoresTheReportedOperation(t *testing.T) {
	withInternalToken(t)
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)
	at := time.Date(2026, 11, 6, 10, 0, 0, 123000000, time.FixedZone("CST", 8*3600))

	var stored *dbclient.AuditLog
	mockDB.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, l *dbclient.AuditLog) error {
		stored = l
		return nil
	})
	rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), testAuditInternalToken, apiKeyCreation(at))
	require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	require.NotNil(t, stored)

	assert.Equal(t, "be24fda013465da928ffc6756959c4ce", stored.UserId)
	assert.Equal(t, "claw-canary-user", stored.UserName.String)
	assert.Equal(t, "default", stored.UserType.String)
	assert.Equal(t, "10.255.36.101", stored.ClientIP.String)
	assert.Equal(t, "POST", stored.HttpMethod)
	assert.Equal(t, "/api/v1/apikeys", stored.RequestPath)
	assert.Equal(t, "apikey", stored.ResourceType.String)
	assert.Equal(t, "create apikey", stored.Action.String)
	assert.Equal(t, `{"name":"ci","ttlDays":1}`, stored.RequestBody.String)
	assert.Equal(t, 200, stored.ResponseStatus)
	assert.Equal(t, int64(12), stored.LatencyMs.Int64)
	assert.True(t, stored.LatencyMs.Valid)
	assert.False(t, stored.TraceId.Valid, "no trace id was reported")
	// The time of the operation, not of the report, in UTC.
	assert.True(t, stored.CreateTime.Valid)
	assert.True(t, stored.CreateTime.Time.Equal(at), "stored %v, reported %v", stored.CreateTime.Time, at)
	assert.Equal(t, time.UTC, stored.CreateTime.Time.Location())
	// A credential in a reported body is redacted as in the rows SaFE records itself.
	assert.NotContains(t, stored.ResponseBody.String, "ak-0123456789abcdef")
	assert.Contains(t, stored.ResponseBody.String, `"[REDACTED]"`)
	assert.Contains(t, stored.ResponseBody.String, `"id":211`)
}

func TestCreateAuditLogNeedsTheInternalToken(t *testing.T) {
	withInternalToken(t)
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl) // no call expected: nothing is stored
	engine := auditEngine(&Handler{dbClient: mockDB})
	now := time.Now()

	for name, token := range map[string]string{"none": "", "wrong": "not-the-token"} {
		t.Run(name, func(t *testing.T) {
			rsp := postAuditLog(t, engine, token, apiKeyCreation(now))
			assert.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())
		})
	}
	t.Run("a user session is not an internal service", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/"+common.PrimusRouterCustomRootPath+"/auditlogs", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: authority.CookieToken, Value: "some-session"})
		rsp := httptest.NewRecorder()
		engine.ServeHTTP(rsp, req)
		assert.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())
	})
}

func TestCreateAuditLogWithoutTheTokenSecret(t *testing.T) {
	// Before the internal auth is initialised there is no token to match: every report is refused.
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)
	rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), "anything", apiKeyCreation(time.Now()))
	assert.Equal(t, http.StatusForbidden, rsp.Code, rsp.Body.String())
}

func TestCreateAuditLogRefusesWhatARowCannotHold(t *testing.T) {
	withInternalToken(t)
	now := time.Now()
	cases := map[string]func(m map[string]any){
		"no user":            func(m map[string]any) { delete(m, "userId") },
		"blank user":         func(m map[string]any) { m["userId"] = "  " },
		"no method":          func(m map[string]any) { delete(m, "httpMethod") },
		"lower-case method":  func(m map[string]any) { m["httpMethod"] = "post" },
		"no path":            func(m map[string]any) { delete(m, "requestPath") },
		"relative path":      func(m map[string]any) { m["requestPath"] = "api/v1/apikeys" },
		"no status":          func(m map[string]any) { delete(m, "responseStatus") },
		"status too large":   func(m map[string]any) { m["responseStatus"] = 600 },
		"negative latency":   func(m map[string]any) { m["latencyMs"] = -1 },
		"latency too large":  func(m map[string]any) { m["latencyMs"] = int64(1) << 31 },
		"no time":            func(m map[string]any) { delete(m, "createTime") },
		"time not RFC3339":   func(m map[string]any) { m["createTime"] = "2026-11-06 10:00:00" },
		"time in the future": func(m map[string]any) { m["createTime"] = now.Add(time.Hour).Format(time.RFC3339) },
		"user id too long":   func(m map[string]any) { m["userId"] = strings.Repeat("a", 65) },
		"action too long":    func(m map[string]any) { m["action"] = strings.Repeat("é", 65) },
		"path too long":      func(m map[string]any) { m["requestPath"] = "/" + strings.Repeat("a", 512) },
		"status not number":  func(m map[string]any) { m["responseStatus"] = "200" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockDB := mock_client.NewMockInterface(ctrl) // no call expected
			body := apiKeyCreation(now)
			mutate(body)
			rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), testAuditInternalToken, body)
			assert.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
		})
	}
	t.Run("not JSON", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := mock_client.NewMockInterface(ctrl)
		rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), testAuditInternalToken, "{")
		assert.Equal(t, http.StatusBadRequest, rsp.Code, rsp.Body.String())
	})
	t.Run("a value at the limit fits", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockDB := mock_client.NewMockInterface(ctrl)
		mockDB.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).Return(nil)
		body := apiKeyCreation(now)
		body["action"] = strings.Repeat("é", 64)
		body["createTime"] = now.Add(time.Minute).Format(time.RFC3339) // within the clock skew
		rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), testAuditInternalToken, body)
		assert.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
	})
}

func TestCreateAuditLogBodyLimit(t *testing.T) {
	withInternalToken(t)
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)
	body := apiKeyCreation(time.Now())
	body["requestBody"] = strings.Repeat("x", maxCreateAuditLogBody)
	rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), testAuditInternalToken, body)
	assert.Equal(t, http.StatusRequestEntityTooLarge, rsp.Code, rsp.Body.String())
}

func TestCreateAuditLogDatabaseFailureIsNotSuccess(t *testing.T) {
	withInternalToken(t)
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).Return(errors.New("connection refused"))
	rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), testAuditInternalToken, apiKeyCreation(time.Now()))
	assert.Equal(t, http.StatusInternalServerError, rsp.Code, rsp.Body.String())
}

func TestCreateAuditLogLeavesTheListRouteAuthenticated(t *testing.T) {
	withInternalToken(t)
	ctrl := gomock.NewController(t)
	mockDB := mock_client.NewMockInterface(ctrl)
	req := httptest.NewRequest(http.MethodGet, "/"+common.PrimusRouterCustomRootPath+"/auditlogs", nil)
	req.Header.Set(authority.InternalAuthTokenHeader, testAuditInternalToken)
	rsp := httptest.NewRecorder()
	auditEngine(&Handler{dbClient: mockDB}).ServeHTTP(rsp, req)
	assert.Equal(t, http.StatusUnauthorized, rsp.Code, "reading the audit log still needs a user who may: %s", rsp.Body.String())
}

func TestCreateAuditLogLatency(t *testing.T) {
	withInternalToken(t)
	for name, c := range map[string]struct {
		set   func(m map[string]any)
		valid bool
		value int64
	}{
		"absent: not recorded":  {func(m map[string]any) { delete(m, "latencyMs") }, false, 0},
		"zero is a latency":     {func(m map[string]any) { m["latencyMs"] = 0 }, true, 0},
		"the largest INT value": {func(m map[string]any) { m["latencyMs"] = int64(1)<<31 - 1 }, true, 1<<31 - 1},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockDB := mock_client.NewMockInterface(ctrl)
			var stored *dbclient.AuditLog
			mockDB.EXPECT().InsertAuditLog(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, l *dbclient.AuditLog) error {
				stored = l
				return nil
			})
			body := apiKeyCreation(time.Now())
			c.set(body)
			rsp := postAuditLog(t, auditEngine(&Handler{dbClient: mockDB}), testAuditInternalToken, body)
			require.Equal(t, http.StatusOK, rsp.Code, rsp.Body.String())
			assert.Equal(t, c.valid, stored.LatencyMs.Valid)
			assert.Equal(t, c.value, stored.LatencyMs.Int64)
		})
	}
}
