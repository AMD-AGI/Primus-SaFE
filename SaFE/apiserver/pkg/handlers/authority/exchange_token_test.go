/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange/fake"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

const (
	exIssuer   = "https://safe.example.test"
	exAudience = "safe-apiserver"
	exClient   = "svc-test"
	exSecret   = "client-secret"
	exUser     = "user-alice"
)

type exchangeEnv struct {
	svc   *identityexchange.Service
	store *fake.Store
	users *fake.Users
}

// setupExchange installs a token exchange service as the process default, with
// one client, one user and one confirmed binding.
func setupExchange(t *testing.T) *exchangeEnv {
	t.Helper()
	pemBytes, err := identityexchange.GenerateSigningKey("")
	require.NoError(t, err)
	keys, err := identityexchange.NewKeySet(pemBytes, nil)
	require.NoError(t, err)
	e := &exchangeEnv{store: fake.NewStore(), users: fake.NewUsers()}
	e.svc, err = identityexchange.NewService(identityexchange.Config{Issuer: exIssuer, Audience: exAudience}, keys, e.store, e.users)
	require.NoError(t, err)
	require.NoError(t, e.store.InsertServiceClient(context.Background(), &dbclient.ServiceClient{
		ClientId: exClient, Name: "t", SecretHash: identityexchange.HashClientSecret(exSecret), MaxTtlSeconds: 600,
		AllowedAudiences: `["safe-apiserver"]`, AllowedScopes: `["safe.api","safe.read"]`, Whitelist: `[]`,
	}))
	e.users.Add(exUser, "alice", "alice@example.test", v1.DefaultRole)
	require.NoError(t, e.store.InsertIdentityBinding(context.Background(), &dbclient.IdentityBinding{
		ClientId: exClient, Issuer: "idp", Subject: "alice-ext", UserId: exUser,
	}))
	identityexchange.SetDefault(e.svc)
	t.Cleanup(func() { identityexchange.SetDefault(nil) })
	return e
}

func (e *exchangeEnv) token(t *testing.T, scope string) string {
	t.Helper()
	resp, oerr := e.svc.Exchange(context.Background(), identityexchange.ExchangeRequest{
		ClientId: exClient, ClientSecret: exSecret, GrantType: identityexchange.GrantTypeTokenExchange,
		SubjectToken: "external:idp|alice-ext", SubjectTokenType: identityexchange.SubjectTokenTypeUserRef,
		Audience: exAudience, Scope: scope,
	})
	require.Nil(t, oerr)
	return resp.AccessToken
}

func parseWith(t *testing.T, method, path, token string) (*gin.Context, error) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, path, nil)
	c.Request.Header.Set("Authorization", "Bearer "+token)
	return c, ParseToken(c)
}

func TestParseTokenAcceptsExchangedToken(t *testing.T) {
	e := setupExchange(t)
	c, err := parseWith(t, http.MethodPost, "/api/v1/workloads", e.token(t, "safe.api"))
	require.NoError(t, err)
	require.Equal(t, exUser, c.GetString(common.UserId), "the subject user is the identity")
	require.Equal(t, "alice", c.GetString(common.UserName))
	require.Equal(t, identityexchange.UserType, c.GetString(common.UserType))
	require.Equal(t, exClient, c.GetString(common.ActorClientId), "the actor is recorded")
	require.NotEmpty(t, c.GetString(common.TokenJti))
}

// Each capability an exchanged token must never have, through the real auth chain.
func TestParseTokenRefusesForbiddenCapabilities(t *testing.T) {
	e := setupExchange(t)
	tok := e.token(t, "safe.api")
	for _, tt := range []struct{ name, method, path string }{
		{"create api key", http.MethodPost, "/api/v1/apikeys"},
		{"delete api key", http.MethodDelete, "/api/v1/apikeys/1"},
		{"read secret", http.MethodGet, "/api/v1/secrets/s1"},
		{"create secret", http.MethodPost, "/api/v1/secrets"},
		{"list users", http.MethodGet, "/api/v1/users"},
		{"change a user's roles", http.MethodPatch, "/api/v1/users/u1"},
		{"create a binding", http.MethodPost, "/api/v1/identity-bindings"},
		{"administer service clients", http.MethodPost, "/api/v1/service-clients"},
		{"add an ssh key", http.MethodPost, "/api/v1/publickeys"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, err := parseWith(t, tt.method, tt.path, tok)
			require.Error(t, err)
			require.Contains(t, err.Error(), "not available with an exchanged token")
			require.Empty(t, c.GetString(common.UserId), "no identity is established for a refused request")
		})
	}
	// The same token works for the user's ordinary access.
	_, err := parseWith(t, http.MethodGet, "/api/v1/users/self", tok)
	require.NoError(t, err)
}

func TestParseTokenScopeAndValidity(t *testing.T) {
	e := setupExchange(t)
	t.Run("read scope refuses writes", func(t *testing.T) {
		tok := e.token(t, "safe.read")
		_, err := parseWith(t, http.MethodGet, "/api/v1/workloads", tok)
		require.NoError(t, err)
		_, err = parseWith(t, http.MethodPost, "/api/v1/workloads", tok)
		require.Error(t, err)
		require.Contains(t, err.Error(), "scope")
	})
	t.Run("revoked binding refuses at once", func(t *testing.T) {
		tok := e.token(t, "safe.api")
		_, err := parseWith(t, http.MethodGet, "/api/v1/workloads", tok)
		require.NoError(t, err)
		_, err = e.store.DeleteIdentityBinding(context.Background(), e.store.Bindings[0].Id, exUser)
		require.NoError(t, err)
		_, err = parseWith(t, http.MethodGet, "/api/v1/workloads", tok)
		require.Error(t, err)
		require.Equal(t, ErrInvalidToken, err.Error())
	})
}

func TestParseTokenLeavesOtherCredentialsAlone(t *testing.T) {
	setupExchange(t)
	// A bare JWT that is not ours still goes down the old path and fails there.
	_, err := parseWith(t, http.MethodGet, "/api/v1/workloads", "aaa.bbb.ccc")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "exchanged")
}

func TestVerifyTokenWithExchangedToken(t *testing.T) {
	cleanup := setupInternalAuth(t)
	defer cleanup()
	e := setupExchange(t)
	tok := e.token(t, "safe.api")

	called := false
	old := platformKeyFetcher
	platformKeyFetcher = func(context.Context, string, string) (string, error) { called = true; return "ak-platform", nil }
	t.Cleanup(func() { platformKeyFetcher = old })

	w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + tok, IncludePlatformKey: true}, testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp verifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, exUser, resp.Data.Id)
	require.Equal(t, identityexchange.UserType, resp.Data.Type)
	require.Equal(t, exClient, resp.Data.Actor)
	require.Equal(t, "safe.api", resp.Data.Scope)
	require.Equal(t, []v1.UserRole{v1.DefaultRole}, resp.Data.Roles)
	// A short-lived delegation must not turn into the user's permanent platform key.
	require.False(t, called, "the platform key is not even looked up")
	require.Empty(t, resp.Data.PlatformKey)

	t.Run("a token for another audience", func(t *testing.T) {
		w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + tok, Audience: "someone-else"}, testInternalToken)
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
	t.Run("without the internal token", func(t *testing.T) {
		w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + tok}, "")
		require.Equal(t, http.StatusForbidden, w.Code)
	})
}
