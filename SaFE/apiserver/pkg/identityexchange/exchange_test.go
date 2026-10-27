/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/stretchr/testify/require"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
)

func TestExchangeSuccess(t *testing.T) {
	for _, alg := range []string{identityexchange.AlgES256, identityexchange.AlgRS256} {
		t.Run(alg, func(t *testing.T) {
			e := newEnv(t, alg)
			resp, oerr := e.svc.Exchange(context.Background(), baseReq())
			require.Nil(t, oerr)
			require.Equal(t, "Bearer", resp.TokenType)
			require.Equal(t, identityexchange.TokenTypeAccessToken, resp.IssuedTokenType)
			require.Equal(t, int64(600), resp.ExpiresIn)
			require.Equal(t, "safe.api", resp.Scope)

			tok, err := jwt.ParseSigned(resp.AccessToken, []jose.SignatureAlgorithm{jose.ES256, jose.RS256})
			require.NoError(t, err)
			require.Equal(t, alg, tok.Headers[0].Algorithm)
			require.Equal(t, "at+jwt", tok.Headers[0].ExtraHeaders[jose.HeaderType])
			var c map[string]any
			require.NoError(t, tok.Claims(e.keys.JWKS().Keys[0].Key, &c))
			require.Equal(t, testIssuer, c["iss"])
			require.Equal(t, aliceId, c["sub"])
			require.Equal(t, testAudience, c["aud"])
			require.Equal(t, clientId, c["client_id"])
			require.Equal(t, map[string]any{"sub": clientId}, c["act"])
			require.Equal(t, "safe.api", c["scope"])
			require.NotEmpty(t, c["jti"])
			require.Equal(t, float64(e.now.Add(10*time.Minute).Unix()), c["exp"])
			// Roles and permissions are never carried in the token.
			for _, k := range []string{"roles", "role", "permissions", "groups"} {
				require.NotContains(t, c, k)
			}

			// The token validates for the subject user with the client as actor.
			p, err := e.svc.ValidateAccessToken(context.Background(), resp.AccessToken, "")
			require.NoError(t, err)
			require.Equal(t, aliceId, p.UserId)
			require.Equal(t, "alice", p.UserName)
			require.Equal(t, clientId, p.ClientId)
			require.Equal(t, []string{"safe.api"}, p.Scopes)
		})
	}
}

func TestExchangeSubjectForms(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		wantOK  bool
		reason  string
	}{
		{"external bound", "external:idp|alice-ext", true, ""},
		{"external unbound subject", "external:idp|someone-else", false, "no_binding"},
		{"external other issuer", "external:other-idp|alice-ext", false, "no_binding"},
		{"email bound user", "email:" + aliceEmail, true, ""},
		{"email is case insensitive", "email:ALICE@Example.test", true, ""},
		{"email of unbound user", "email:bob@example.test", false, "email_mismatch"},
		{"safe_user_id bound user", "safe_user_id:" + aliceId, true, ""},
		{"safe_user_id of unbound user", "safe_user_id:bob-id", false, "no_binding"},
		{"malformed", "alice", false, "malformed_subject"},
		{"unknown kind", "name:alice", false, "malformed_subject"},
		{"external without separator", "external:idp", false, "malformed_subject"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, identityexchange.AlgES256)
			// A user that exists but never confirmed a binding to this client.
			e.users.Add("bob-id", "bob", "bob@example.test")
			req := baseReq()
			req.SubjectToken = tt.subject
			resp, oerr := e.svc.Exchange(context.Background(), req)
			if tt.wantOK {
				require.Nil(t, oerr)
				require.NotEmpty(t, resp.AccessToken)
				return
			}
			require.Nil(t, resp)
			require.Equal(t, identityexchange.ErrInvalidGrant, oerr.Code)
			require.Equal(t, 400, oerr.Status)
			// The caller learns nothing about why; the audit row records the cause.
			require.Empty(t, oerr.Description)
			require.Equal(t, tt.reason, auditBody(t, e.lastAudit(t))["reason"])
		})
	}
}

// A user bound to a different client is not reachable by this one, whatever the
// subject form: the binding is per client.
func TestExchangeBindingIsPerClient(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	e.users.Add("carol-id", "carol", "carol@example.test")
	require.NoError(t, e.store.InsertIdentityBinding(context.Background(), bindingFor("another-client", "carol-id")))
	for _, subj := range []string{"safe_user_id:carol-id", "email:carol@example.test"} {
		req := baseReq()
		req.SubjectToken = subj
		_, oerr := e.svc.Exchange(context.Background(), req)
		require.NotNil(t, oerr, subj)
		require.Equal(t, identityexchange.ErrInvalidGrant, oerr.Code)
	}
}

func TestExchangeRefusals(t *testing.T) {
	tests := []struct {
		name     string
		mut      func(e *env, r *identityexchange.ExchangeRequest)
		wantCode string
		status   int
		reason   string
	}{
		{"wrong secret", func(_ *env, r *identityexchange.ExchangeRequest) { r.ClientSecret = "nope" }, identityexchange.ErrInvalidClient, 401, "bad_secret"},
		{"unknown client", func(_ *env, r *identityexchange.ExchangeRequest) { r.ClientId = "svc-unknown" }, identityexchange.ErrInvalidClient, 401, "unknown_client"},
		{"missing credentials", func(_ *env, r *identityexchange.ExchangeRequest) { r.ClientSecret = "" }, identityexchange.ErrInvalidClient, 401, "missing_credentials"},
		{"disabled client", func(e *env, _ *identityexchange.ExchangeRequest) {
			_, _ = e.store.SetServiceClientDisabled(context.Background(), clientId, true)
		}, identityexchange.ErrInvalidClient, 401, "client_disabled"},
		{"reserved auth method is not implemented", func(e *env, _ *identityexchange.ExchangeRequest) {
			e.store.Clients[clientId].AuthMethod = identityexchange.AuthMethodPrivateKey
		}, identityexchange.ErrInvalidClient, 401, "unsupported_auth_method"},
		{"source ip outside the allow list", func(e *env, r *identityexchange.ExchangeRequest) {
			e.store.Clients[clientId].Whitelist = `["10.1.0.0/16","192.0.2.7"]`
			r.ClientIP = "10.0.0.5"
		}, identityexchange.ErrInvalidClient, 401, "ip_not_allowed"},
		{"unsupported grant", func(_ *env, r *identityexchange.ExchangeRequest) { r.GrantType = "client_credentials" }, identityexchange.ErrUnsupportedGrant, 400, "grant_type"},
		{"unsupported subject token type", func(_ *env, r *identityexchange.ExchangeRequest) {
			r.SubjectTokenType = "urn:ietf:params:oauth:token-type:id_token"
		}, identityexchange.ErrInvalidRequest, 400, "subject_token_type"},
		{"unsupported requested token type", func(_ *env, r *identityexchange.ExchangeRequest) {
			r.RequestedTokenType = "urn:ietf:params:oauth:token-type:refresh_token"
		}, identityexchange.ErrInvalidRequest, 400, "requested_token_type"},
		{"audience not allowed", func(_ *env, r *identityexchange.ExchangeRequest) { r.Audience = "admin-console" }, identityexchange.ErrInvalidTarget, 400, "audience"},
		{"audience missing", func(_ *env, r *identityexchange.ExchangeRequest) { r.Audience = "" }, identityexchange.ErrInvalidTarget, 400, "audience"},
		{"scope beyond the client", func(_ *env, r *identityexchange.ExchangeRequest) { r.Scope = "safe.api admin" }, identityexchange.ErrInvalidScope, 400, "scope"},
		{"frozen user", func(e *env, _ *identityexchange.ExchangeRequest) {
			u, _ := e.users.GetUser(context.Background(), aliceId)
			u.Spec.RestrictedType = 1
			e.users.M[aliceId] = u
		}, identityexchange.ErrInvalidGrant, 400, "user_restricted"},
		{"bound user no longer exists", func(e *env, _ *identityexchange.ExchangeRequest) { delete(e.users.M, aliceId) }, identityexchange.ErrInvalidGrant, 400, "user_not_found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, identityexchange.AlgES256)
			req := baseReq()
			tt.mut(e, &req)
			resp, oerr := e.svc.Exchange(context.Background(), req)
			require.Nil(t, resp)
			require.NotNil(t, oerr)
			require.Equal(t, tt.wantCode, oerr.Code)
			require.Equal(t, tt.status, oerr.Status)
			row := e.lastAudit(t)
			require.Equal(t, tt.status, row.ResponseStatus)
			require.Equal(t, tt.reason, auditBody(t, row)["reason"])
			require.False(t, row.TokenJti.Valid, "no token was issued")
		})
	}
}

func TestExchangeTTL(t *testing.T) {
	tests := []struct {
		name      string
		clientMax int
		want      int64
	}{
		{"default", 600, 600},
		{"unset falls back to default", 0, 600},
		{"client max below default", 120, 120},
		{"client max above the default never lengthens the token", 7200, 600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, identityexchange.AlgES256)
			e.store.Clients[clientId].MaxTtlSeconds = tt.clientMax
			resp, oerr := e.svc.Exchange(context.Background(), baseReq())
			require.Nil(t, oerr)
			require.Equal(t, tt.want, resp.ExpiresIn)
		})
	}
}

func TestExchangeScopeDefaultsToClientScopes(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	req := baseReq()
	req.Scope = ""
	resp, oerr := e.svc.Exchange(context.Background(), req)
	require.Nil(t, oerr)
	require.Equal(t, "safe.api safe.read", resp.Scope)
	req.Scope = "safe.read safe.read"
	resp, oerr = e.svc.Exchange(context.Background(), req)
	require.Nil(t, oerr)
	require.Equal(t, "safe.read", resp.Scope)
}

func TestExchangeAuditRowOnSuccess(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	tok := e.exchange(t, nil)
	row := e.lastAudit(t)
	require.Equal(t, aliceId, row.UserId)
	require.Equal(t, identityexchange.UserType, row.UserType.String)
	require.Equal(t, clientId, row.ActorClientId.String)
	require.Equal(t, "POST", row.HttpMethod)
	require.Equal(t, "/oauth/token", row.RequestPath)
	require.Equal(t, 200, row.ResponseStatus)
	require.Equal(t, "10.0.0.5", row.ClientIP.String)
	require.Equal(t, "exchange", row.Action.String)
	body := auditBody(t, row)
	require.Equal(t, "ok", body["result"])
	require.Equal(t, "external:idp|alice-ext", body["subject"])
	require.Equal(t, testAudience, body["audience"])
	// The jti in the audit row is the one in the token.
	p, err := e.svc.ValidateAccessToken(context.Background(), tok, "")
	require.NoError(t, err)
	require.Equal(t, p.Jti, row.TokenJti.String)
	// Neither the token nor the secret ends up in the audit trail.
	for _, s := range []string{row.RequestBody.String, row.ResponseBody.String} {
		require.NotContains(t, s, tok)
		require.NotContains(t, s, clientSecret)
	}
}

func TestExchangeAuditRowOnRefusal(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	req := baseReq()
	req.SubjectToken = "email:" + aliceEmail
	e.users.M[aliceId].Annotations[v1.UserEmailAnnotation] = "someone-else@example.test"
	_, oerr := e.svc.Exchange(context.Background(), req)
	require.NotNil(t, oerr)
	row := e.lastAudit(t)
	require.Equal(t, clientId, row.ActorClientId.String)
	require.Equal(t, 400, row.ResponseStatus)
	require.Equal(t, "refused", auditBody(t, row)["result"])
}

func TestExchangeAuditTruncatesHostileInput(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	req := baseReq()
	req.ClientId = strings.Repeat("x", 5000)
	_, oerr := e.svc.Exchange(context.Background(), req)
	require.NotNil(t, oerr)
	require.LessOrEqual(t, len(e.lastAudit(t).ActorClientId.String), 256)
}
