/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// delegatedChain builds a real Authorize + Audit chain in external mode. The
// authenticator is a fake RFC 7662 introspection server that answers "alice, acted for by
// svc-bot" for the token "delegated" and "alice" for "direct".
func delegatedChain(t *testing.T) (*gin.Engine, chan *dbclient.AuditLog) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rev := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, r.ParseForm())
		token := r.PostForm.Get("token")
		answer := map[string]any{"active": token != "", "sub": "alice-id", "username": "alice"}
		if token == "delegated" {
			answer["act"] = map[string]any{"client_id": "svc-bot"}
			answer["scope"] = "safe.api"
			answer["jti"] = "jti-7"
		}
		_ = json.NewEncoder(w).Encode(answer)
	}))
	t.Cleanup(rev.Close)
	viper.Set("auth.mode", "external")
	secretFile := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("secret"), 0o600))
	viper.Set("auth.authenticators", []map[string]any{{"type": "introspection", "url": rev.URL, "client_id": "svc-safe", "client_secret_file": secretFile}})
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	require.NoError(t, authority.InitExternalAuth(fake.NewClientBuilder().WithScheme(scheme).Build()))

	ch := make(chan *dbclient.AuditLog, 10)
	old := auditBuffer
	auditBuffer = &auditLogBuffer{ch: ch}
	t.Cleanup(func() {
		viper.Set("auth.mode", nil)
		viper.Set("auth.authenticators", nil)
		require.NoError(t, authority.InitExternalAuth(nil))
		auditBuffer = old
	})

	e := gin.New()
	api := e.Group("/api/v1", Authorize())
	api.GET("/workloads", func(c *gin.Context) { c.Status(http.StatusOK) })
	api.POST("/workloads", auditHandler("workload", ""), func(c *gin.Context) { c.Status(http.StatusCreated) })
	api.GET("/secrets", func(c *gin.Context) { c.Status(http.StatusOK) })
	return e, ch
}

func call(e *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	e.ServeHTTP(w, req)
	return w
}

func TestDelegatedGetIsAudited(t *testing.T) {
	e, ch := delegatedChain(t)
	require.Equal(t, http.StatusOK, call(e, http.MethodGet, "/api/v1/workloads", "delegated").Code)
	select {
	case row := <-ch:
		require.Equal(t, "alice-id", row.UserId, "the audited user is the one acted for")
		require.Equal(t, "svc-bot", row.ActorClientId.String, "and the party that acted is recorded")
		require.Equal(t, "jti-7", row.TokenJti.String)
		require.Equal(t, authority.UserTypeDelegated, row.UserType.String)
		require.Equal(t, http.MethodGet, row.HttpMethod)
		require.Equal(t, "/api/v1/workloads", row.RequestPath)
		require.Equal(t, 200, row.ResponseStatus)
		require.False(t, row.RequestBody.Valid, "reads are recorded without bodies")
		require.False(t, row.ResponseBody.Valid)
	default:
		t.Fatal("a GET made with a delegated token left no audit row")
	}
}

func TestDelegatedWriteCarriesActor(t *testing.T) {
	e, ch := delegatedChain(t)
	require.Equal(t, http.StatusCreated, call(e, http.MethodPost, "/api/v1/workloads", "delegated").Code)
	row := <-ch
	require.Equal(t, "alice-id", row.UserId)
	require.Equal(t, "svc-bot", row.ActorClientId.String)
	require.Equal(t, "jti-7", row.TokenJti.String)
	require.Len(t, ch, 0, "one request, one row")
}

func TestRefusedDelegatedRequestReachesNoHandler(t *testing.T) {
	e, ch := delegatedChain(t)
	require.Equal(t, http.StatusForbidden, call(e, http.MethodGet, "/api/v1/secrets", "delegated").Code)
	require.Len(t, ch, 0)
}

func TestDirectRequestsAreNotTaggedAsActed(t *testing.T) {
	e, ch := delegatedChain(t)
	// A direct read is not audited; a direct write is, without an actor.
	require.Equal(t, http.StatusOK, call(e, http.MethodGet, "/api/v1/workloads", "direct").Code)
	require.Len(t, ch, 0)
	require.Equal(t, http.StatusCreated, call(e, http.MethodPost, "/api/v1/workloads", "direct").Code)
	row := <-ch
	require.Equal(t, "alice-id", row.UserId)
	require.False(t, row.ActorClientId.Valid)
	require.False(t, row.TokenJti.Valid)
	require.Equal(t, authority.UserTypeExternal, row.UserType.String)
}
