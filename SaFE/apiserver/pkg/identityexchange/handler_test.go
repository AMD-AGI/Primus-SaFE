/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

func newEngine(e *env, asUser string, admin bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	e.svc.RegisterPublicRoutes(r)
	e.svc.SetAdminCheck(func(c *gin.Context) error {
		if !admin {
			return commonerrors.NewForbidden("system administrator required")
		}
		return nil
	})
	api := r.Group("/api/v1", func(c *gin.Context) { c.Set(common.UserId, asUser) })
	api.POST("/identity-bindings", e.svc.CreateBinding)
	api.GET("/identity-bindings", e.svc.ListBindings)
	api.DELETE("/identity-bindings/:id", e.svc.DeleteBinding)
	api.POST("/service-clients", e.svc.CreateClient)
	api.GET("/service-clients", e.svc.ListClients)
	api.POST("/service-clients/:clientId/disable", e.svc.DisableClient)
	return r
}

func tokenForm(mut func(url.Values)) url.Values {
	v := url.Values{
		"grant_type":         {identityexchange.GrantTypeTokenExchange},
		"subject_token":      {"external:idp|alice-ext"},
		"subject_token_type": {identityexchange.SubjectTokenTypeUserRef},
		"audience":           {testAudience},
		"scope":              {"safe.api"},
	}
	if mut != nil {
		mut(v)
	}
	return v
}

func post(r http.Handler, path string, form url.Values, basic bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basic {
		req.SetBasicAuth(clientId, clientSecret)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestTokenEndpoint(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	r := newEngine(e, aliceId, false)

	t.Run("basic auth", func(t *testing.T) {
		w := post(r, "/oauth/token", tokenForm(nil), true)
		require.Equal(t, 200, w.Code, w.Body.String())
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		var resp identityexchange.TokenResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		require.Equal(t, "Bearer", resp.TokenType)
		require.NotEmpty(t, resp.AccessToken)
	})
	t.Run("credentials in the form", func(t *testing.T) {
		w := post(r, "/oauth/token", tokenForm(func(v url.Values) {
			v.Set("client_id", clientId)
			v.Set("client_secret", clientSecret)
		}), false)
		require.Equal(t, 200, w.Code, w.Body.String())
	})
	t.Run("both methods at once", func(t *testing.T) {
		w := post(r, "/oauth/token", tokenForm(func(v url.Values) { v.Set("client_id", clientId) }), true)
		require.Equal(t, 400, w.Code)
		require.Contains(t, w.Body.String(), identityexchange.ErrInvalidRequest)
	})
	t.Run("no credentials", func(t *testing.T) {
		w := post(r, "/oauth/token", tokenForm(nil), false)
		require.Equal(t, 401, w.Code)
		require.Contains(t, w.Header().Get("WWW-Authenticate"), "Basic")
		require.Contains(t, w.Body.String(), identityexchange.ErrInvalidClient)
	})
	t.Run("unbound subject does not say why", func(t *testing.T) {
		w := post(r, "/oauth/token", tokenForm(func(v url.Values) { v.Set("subject_token", "external:idp|stranger") }), true)
		require.Equal(t, 400, w.Code)
		require.JSONEq(t, `{"error":"invalid_grant"}`, w.Body.String())
	})
	t.Run("a GET is not the token endpoint", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/oauth/token", nil))
		require.Equal(t, 404, w.Code)
	})
}

func TestDiscoveryAndJWKS(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	r := newEngine(e, aliceId, false)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/.well-known/openid-configuration", nil))
	require.Equal(t, 200, w.Code)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &doc))
	require.Equal(t, testIssuer, doc["issuer"])
	require.Equal(t, testIssuer+"/oauth/token", doc["token_endpoint"])
	require.Equal(t, testIssuer+"/.well-known/jwks.json", doc["jwks_uri"])
	require.Equal(t, []any{identityexchange.GrantTypeTokenExchange}, doc["grant_types_supported"])
	require.Equal(t, []any{"ES256"}, doc["id_token_signing_alg_values_supported"])

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), `"kty":"EC"`)
	require.Contains(t, w.Body.String(), `"use":"sig"`)
	require.Contains(t, w.Body.String(), e.keys.KeyID())
	// The private scalar never appears.
	require.NotContains(t, w.Body.String(), `"d":`)
}

func TestBindingEndpoints(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	bob := "bob-id"
	e.users.Add(bob, "bob", "bob@example.test")
	do := func(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}
	rb := newEngine(e, bob, false)

	w := do(rb, http.MethodPost, "/api/v1/identity-bindings", `{"clientId":"`+clientId+`","issuer":"idp","subject":"bob-ext"}`)
	require.Equal(t, 200, w.Code, w.Body.String())
	has, _ := e.store.HasIdentityBindingForUser(context.Background(), clientId, bob)
	require.True(t, has, "the binding belongs to the authenticated user")

	// Bob can now be exchanged for; before he confirmed it he could not.
	req := baseReq()
	req.SubjectToken = "external:idp|bob-ext"
	_, oerr := e.svc.Exchange(context.Background(), req)
	require.Nil(t, oerr)

	// The same identity cannot be claimed by someone else.
	w = do(newEngine(e, aliceId, false), http.MethodPost, "/api/v1/identity-bindings", `{"clientId":"`+clientId+`","issuer":"idp","subject":"bob-ext"}`)
	require.Equal(t, 409, w.Code, w.Body.String())

	for name, body := range map[string]string{
		"unknown client": `{"clientId":"nope","issuer":"idp","subject":"x"}`,
		"missing fields": `{"clientId":"` + clientId + `"}`,
		"bad issuer":     `{"clientId":"` + clientId + `","issuer":"a|b","subject":"x"}`,
		"not json":       `nope`,
	} {
		w = do(rb, http.MethodPost, "/api/v1/identity-bindings", body)
		require.GreaterOrEqual(t, w.Code, 400, name)
	}

	// Listing shows only one's own; deleting someone else's binding is a 404.
	w = do(rb, http.MethodGet, "/api/v1/identity-bindings", "")
	var list []identityexchange.BindingView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)
	require.Equal(t, "bob-ext", list[0].Subject)
	w = do(newEngine(e, aliceId, false), http.MethodDelete, "/api/v1/identity-bindings/"+itoa(list[0].Id), "")
	require.Equal(t, 404, w.Code)
	w = do(rb, http.MethodDelete, "/api/v1/identity-bindings/"+itoa(list[0].Id), "")
	require.Equal(t, 200, w.Code)
	_, oerr = e.svc.Exchange(context.Background(), req)
	require.NotNil(t, oerr, "a revoked binding stops the exchange")
}

func itoa(i int64) string { b, _ := json.Marshal(i); return string(b) }

func TestServiceClientAdministration(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	body := `{"name":"worker","allowedAudiences":["safe-apiserver"],"allowedScopes":["safe.api"],"whitelist":["10.0.0.0/8"]}`
	do := func(r http.Handler, method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w
	}

	// Only system administrators.
	for _, call := range [][3]string{
		{http.MethodPost, "/api/v1/service-clients", body},
		{http.MethodGet, "/api/v1/service-clients", ""},
		{http.MethodPost, "/api/v1/service-clients/" + clientId + "/disable", ""},
	} {
		w := do(newEngine(e, aliceId, false), call[0], call[1], call[2])
		require.Equal(t, 403, w.Code, call[1])
	}
	require.False(t, e.store.Clients[clientId].Disabled)

	r := newEngine(e, "admin-id", true)
	w := do(r, http.MethodPost, "/api/v1/service-clients", body)
	require.Equal(t, 200, w.Code, w.Body.String())
	var created identityexchange.ClientView
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	require.True(t, strings.HasPrefix(created.ClientId, identityexchange.ClientIdPrefix))
	require.NotEmpty(t, created.Secret)
	require.Equal(t, 600, created.MaxTtlSeconds)
	stored := e.store.Clients[created.ClientId]
	require.Equal(t, identityexchange.HashClientSecret(created.Secret), stored.SecretHash)
	require.NotContains(t, stored.SecretHash, created.Secret, "only the hash is stored")

	// Listing never repeats the secret or its hash.
	w = do(r, http.MethodGet, "/api/v1/service-clients", "")
	require.Equal(t, 200, w.Code)
	require.NotContains(t, w.Body.String(), created.Secret)
	require.NotContains(t, w.Body.String(), stored.SecretHash)

	for name, bad := range map[string]string{
		"no audiences":  `{"name":"x","allowedScopes":["safe.api"]}`,
		"ttl too large": `{"name":"x","allowedAudiences":["a"],"allowedScopes":["s"],"maxTtlSeconds":3600}`,
		"bad whitelist": `{"name":"x","allowedAudiences":["a"],"allowedScopes":["s"],"whitelist":["not-an-ip"]}`,
	} {
		require.Equal(t, 400, do(r, http.MethodPost, "/api/v1/service-clients", bad).Code, name)
	}

	require.Equal(t, 200, do(r, http.MethodPost, "/api/v1/service-clients/"+clientId+"/disable", "").Code)
	_, oerr := e.svc.Exchange(context.Background(), baseReq())
	require.NotNil(t, oerr)
	require.Equal(t, identityexchange.ErrInvalidClient, oerr.Code)
	require.Equal(t, 404, do(r, http.MethodPost, "/api/v1/service-clients/missing/disable", "").Code)
}
