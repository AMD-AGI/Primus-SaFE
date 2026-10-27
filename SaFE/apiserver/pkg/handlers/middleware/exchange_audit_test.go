/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange/fake"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// exchangeChain builds a real Authorize + Audit chain with a token exchange
// service installed, and returns an exchanged token for user-alice.
func exchangeChain(t *testing.T) (*gin.Engine, string, chan *dbclient.AuditLog) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	pemBytes, err := identityexchange.GenerateSigningKey("")
	require.NoError(t, err)
	keys, err := identityexchange.NewKeySet(pemBytes, nil)
	require.NoError(t, err)
	store, users := fake.NewStore(), fake.NewUsers()
	svc, err := identityexchange.NewService(identityexchange.Config{Issuer: "https://safe.example.test", Audience: "safe-apiserver"}, keys, store, users)
	require.NoError(t, err)
	require.NoError(t, store.InsertServiceClient(context.Background(), &dbclient.ServiceClient{
		ClientId: "svc-test", SecretHash: identityexchange.HashClientSecret("s"), MaxTtlSeconds: 600,
		AllowedAudiences: `["safe-apiserver"]`, AllowedScopes: `["safe.api"]`, Whitelist: `[]`,
	}))
	users.Add("user-alice", "alice", "alice@example.test")
	require.NoError(t, store.InsertIdentityBinding(context.Background(), &dbclient.IdentityBinding{
		ClientId: "svc-test", Issuer: "idp", Subject: "a", UserId: "user-alice",
	}))
	identityexchange.SetDefault(svc)

	ch := make(chan *dbclient.AuditLog, 10)
	old := auditBuffer
	auditBuffer = &auditLogBuffer{ch: ch}
	t.Cleanup(func() { identityexchange.SetDefault(nil); auditBuffer = old })

	resp, oerr := svc.Exchange(context.Background(), identityexchange.ExchangeRequest{
		ClientId: "svc-test", ClientSecret: "s", GrantType: identityexchange.GrantTypeTokenExchange,
		SubjectToken: "external:idp|a", SubjectTokenType: identityexchange.SubjectTokenTypeUserRef, Audience: "safe-apiserver",
	})
	require.Nil(t, oerr)

	e := gin.New()
	api := e.Group("/api/v1", Authorize())
	api.GET("/workloads", func(c *gin.Context) { c.Status(http.StatusOK) })
	api.POST("/workloads", auditHandler("workload", ""), func(c *gin.Context) { c.Status(http.StatusCreated) })
	api.GET("/secrets", func(c *gin.Context) { c.Status(http.StatusOK) })
	return e, resp.AccessToken, ch
}

func call(e *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	e.ServeHTTP(w, req)
	return w
}

func TestExchangedGetIsAudited(t *testing.T) {
	e, tok, ch := exchangeChain(t)
	w := call(e, http.MethodGet, "/api/v1/workloads", tok)
	require.Equal(t, http.StatusOK, w.Code)
	select {
	case row := <-ch:
		require.Equal(t, "user-alice", row.UserId, "the audited user is the one acted for")
		require.Equal(t, "svc-test", row.ActorClientId.String, "and the client that acted is recorded")
		require.NotEmpty(t, row.TokenJti.String)
		require.Equal(t, identityexchange.UserType, row.UserType.String)
		require.Equal(t, http.MethodGet, row.HttpMethod)
		require.Equal(t, "/api/v1/workloads", row.RequestPath)
		require.Equal(t, 200, row.ResponseStatus)
		require.False(t, row.RequestBody.Valid, "reads are recorded without bodies")
		require.False(t, row.ResponseBody.Valid)
	default:
		t.Fatal("a GET made with an exchanged token left no audit row")
	}
}

func TestExchangedWriteCarriesActor(t *testing.T) {
	e, tok, ch := exchangeChain(t)
	require.Equal(t, http.StatusCreated, call(e, http.MethodPost, "/api/v1/workloads", tok).Code)
	row := <-ch
	require.Equal(t, "user-alice", row.UserId)
	require.Equal(t, "svc-test", row.ActorClientId.String)
	require.NotEmpty(t, row.TokenJti.String)
	require.Equal(t, "create workload", row.Action.String)
	require.Len(t, ch, 0, "one request, one row")
}

func TestRefusedExchangedRequestReachesNoHandler(t *testing.T) {
	e, tok, ch := exchangeChain(t)
	w := call(e, http.MethodGet, "/api/v1/secrets", tok)
	require.Equal(t, http.StatusForbidden, w.Code)
	require.Len(t, ch, 0)
}

func TestDirectRequestsAreNotTaggedAsActed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	old := auditBuffer
	ch := make(chan *dbclient.AuditLog, 2)
	auditBuffer = &auditLogBuffer{ch: ch}
	defer func() { auditBuffer = old }()
	e := gin.New()
	e.POST("/w", func(c *gin.Context) {
		c.Set(common.UserId, "u1")
		c.Set(common.UserType, "sso")
	}, auditHandler("workload", ""), func(c *gin.Context) { c.Status(200) })
	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/w", nil))
	row := <-ch
	require.False(t, row.ActorClientId.Valid)
	require.False(t, row.TokenJti.Valid)
}
