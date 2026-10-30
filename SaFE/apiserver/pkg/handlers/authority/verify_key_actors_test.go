/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	apimetrics "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/metrics"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

const (
	keyActorClient = "svc-bot"
	otherUser      = "bob-id"

	msgDelegatedForbidden = "this operation is not available with a delegated token"
	msgScope              = "the token scope does not allow this operation"
)

// withKeyActors configures auth.key_actors for a test; it must run before newExtEnv, which reads it.
func withKeyActors(t *testing.T, actors ...string) {
	t.Helper()
	viper.Set("auth.key_actors", actors)
	t.Cleanup(func() { viper.Set("auth.key_actors", nil) })
}

// stubPlatformKeys makes the platform key a function of the user it is asked for, and records who was asked.
func stubPlatformKeys(t *testing.T) *[]string {
	t.Helper()
	var asked []string
	old := platformKeyFetcher
	platformKeyFetcher = func(_ context.Context, userId, _ string) (string, error) {
		asked = append(asked, userId)
		return "pk-of-" + userId, nil
	}
	t.Cleanup(func() { platformKeyFetcher = old })
	return &asked
}

// delegatedJWT is a token of the identity provider for audience extAudience that names actor and scope.
func (e *extEnv) delegatedJWT(actor, scope, subject string) string {
	return e.jwt(map[string]any{"act": map[string]any{"sub": actor}, "scope": scope, "sub": subject})
}

func counter(reason string) float64 {
	return testutil.ToFloat64(apimetrics.AuthFailuresTotal.WithLabelValues(reason))
}

func callVerifyRaw(t *testing.T, body string, internalToken string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader([]byte(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	if internalToken != "" {
		c.Request.Header.Set(InternalAuthTokenHeader, internalToken)
	}
	VerifyToken(c)
	return w
}

// A delegated token may call POST /auth/verify only when its actor is named in auth.key_actors, it
// carries safe.api, and the chain accepts it; the answer is about the token's subject and nobody else.
// wantCode 200 means the keys are those of wantSubject; any other code is a refusal that leaves no trace:
// no key looked up, no user provisioned, and the failure counted under wantMetric.
func TestVerifyKeyActors(t *testing.T) {
	defer setupInternalAuth(t)()
	cases := []struct {
		name        string
		actors      []string
		token       func(e *extEnv) string
		wantCode    int
		wantSubject string
		wantMessage string
		wantMetric  string
	}{
		{
			name: "listed actor, safe.api", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.api", extUser) },
			wantCode: http.StatusOK, wantSubject: extUser,
		},
		{
			name: "listed actor among others, safe.api among other scopes", actors: []string{"other-client", keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "profile safe.api safe.read", extUser) },
			wantCode: http.StatusOK, wantSubject: extUser,
		},
		{
			name: "listed actor, a different subject: the keys are that subject's", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.api", otherUser) },
			wantCode: http.StatusOK, wantSubject: otherUser,
		},
		{
			name: "JWT whose act.client_id is listed, act.sub is another", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				return e.jwt(map[string]any{"act": map[string]any{"client_id": keyActorClient, "sub": "someone-else"}, "scope": "safe.api"})
			},
			wantCode: http.StatusOK, wantSubject: extUser,
		},
		{
			name: "introspected answer, act.client_id listed", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				e.revUsers["opaque-delegated"] = map[string]any{"sub": otherUser, "username": "bob", "groups": []string{"platform-admins"},
					"scope": "safe.api", "aud": extAudience, "act": map[string]any{"client_id": keyActorClient, "sub": keyActorClient}}
				return "opaque-delegated"
			},
			wantCode: http.StatusOK, wantSubject: otherUser,
		},

		{
			name: "listed actor, safe.read only", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.read", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgScope, wantMetric: "delegated_scope",
		},
		{
			name: "listed actor, no scope", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.jwt(map[string]any{"act": map[string]any{"sub": keyActorClient}}) },
			wantCode: http.StatusForbidden, wantMessage: msgScope, wantMetric: "delegated_scope",
		},
		{
			name: "listed actor, a scope that is not recognised", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "profile", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgScope, wantMetric: "delegated_scope",
		},
		{
			name: "listed actor, a scope that only contains safe.api as a substring", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.api.admin x-safe.api", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgScope, wantMetric: "delegated_scope",
		},
		{
			name: "introspected answer, listed actor, safe.read only", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				e.revUsers["opaque-read"] = map[string]any{"sub": extUser, "username": "alice", "scope": "safe.read", "aud": extAudience,
					"act": map[string]any{"client_id": keyActorClient}}
				return "opaque-read"
			},
			wantCode: http.StatusForbidden, wantMessage: msgScope, wantMetric: "delegated_scope",
		},

		{
			name: "actor not listed", actors: []string{"other-client"},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.api", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},
		{
			name: "no actor is listed: today's behaviour", actors: nil,
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.api", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},
		{
			name: "the list holds only blanks", actors: []string{"", "  "},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.api", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},
		{
			name: "the actor is a prefix of a listed id", actors: []string{keyActorClient + "-2"},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient, "safe.api", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},
		{
			name: "a listed id is a prefix of the actor", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT(keyActorClient+"-2", "safe.api", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},
		{
			name: "ids are compared exactly, not by case", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.delegatedJWT("SVC-BOT", "safe.api", extUser) },
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},
		{
			name: "JWT whose act.client_id is not listed but act.sub is", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				return e.jwt(map[string]any{"act": map[string]any{"client_id": "someone-else", "sub": keyActorClient}, "scope": "safe.api"})
			},
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},
		{
			name: "introspected answer, act.client_id not listed", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				e.revUsers["opaque-other"] = map[string]any{"sub": extUser, "username": "alice", "scope": "safe.api", "aud": extAudience,
					"act": map[string]any{"client_id": "someone-else", "sub": keyActorClient}}
				return "opaque-other"
			},
			wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden, wantMetric: "delegated_forbidden",
		},

		{
			name: "listed actor, a token for another audience", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				return e.jwt(map[string]any{"act": map[string]any{"sub": keyActorClient}, "scope": "safe.api", "aud": []string{"another-service"}})
			},
			wantCode: http.StatusUnauthorized, wantMetric: "invalid",
		},
		{
			name: "introspected answer, listed actor, another audience", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				e.revUsers["opaque-aud"] = map[string]any{"sub": extUser, "username": "alice", "scope": "safe.api", "aud": []string{"another-service"},
					"act": map[string]any{"client_id": keyActorClient}}
				return "opaque-aud"
			},
			wantCode: http.StatusUnauthorized, wantMetric: "invalid",
		},
		{
			name: "listed actor, an expired token", actors: []string{keyActorClient},
			token: func(e *extEnv) string {
				return e.jwt(map[string]any{"act": map[string]any{"sub": keyActorClient}, "scope": "safe.api", "exp": time.Now().Add(-time.Hour).Unix()})
			},
			wantCode: http.StatusUnauthorized, wantMetric: "invalid",
		},
		{
			name: "listed actor, an act that names nobody", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return e.jwt(map[string]any{"act": map[string]any{}, "scope": "safe.api"}) },
			wantCode: http.StatusUnauthorized, wantMetric: "invalid",
		},
		{
			name: "listed actor, an introspection answer that is not active", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { return "opaque-revoked" },
			wantCode: http.StatusUnauthorized, wantMetric: "invalid",
		},
		{
			name: "listed actor, the introspection endpoint is down", actors: []string{keyActorClient},
			token:    func(e *extEnv) string { e.revMode.Store("503"); return "opaque-never-seen" },
			wantCode: http.StatusServiceUnavailable, wantMetric: "unavailable",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asked := stubPlatformKeys(t)
			withKeyActors(t, tc.actors...)
			e := newExtEnv(t, adminMapping, false)
			tok := tc.token(e)
			before := map[string]float64{}
			for _, r := range []string{"delegated_forbidden", "delegated_scope", "invalid", "unavailable"} {
				before[r] = counter(r)
			}

			w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + tok, IncludePlatformKey: true}, testInternalToken)
			require.Equal(t, tc.wantCode, w.Code, w.Body.String())

			if tc.wantCode != http.StatusOK {
				require.Empty(t, *asked, "no key is looked up for a refused token")
				list := &v1.UserList{}
				require.NoError(t, e.cli.List(context.Background(), list))
				require.Empty(t, list.Items, "a refused token provisions nobody")
				if tc.wantMessage != "" {
					var body struct {
						Message string `json:"message"`
					}
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
					require.Equal(t, tc.wantMessage, body.Message)
				}
				for r, was := range before {
					want := 0.0
					if r == tc.wantMetric {
						want = 1
					}
					require.Equal(t, want, counter(r)-was, "failures counted as %q", r)
				}
				return
			}

			var resp verifyResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, tc.wantSubject, resp.Data.Id, "the verdict is about the token's subject")
			require.Equal(t, UserTypeDelegated, resp.Data.Type)
			require.Equal(t, "pk-of-"+tc.wantSubject, resp.Data.PlatformKey)
			require.Equal(t, []string{tc.wantSubject}, *asked, "keys were looked up for the subject, and only for it")
			require.ElementsMatch(t, []v1.UserRole{v1.SystemAdminRole}, resp.Data.Roles, "the roles are the subject's, as a request of the subject sees them")
			require.Equal(t, v1.SSOUserType, e.user(tc.wantSubject).Spec.Type)
			for _, r := range []string{"delegated_forbidden", "delegated_scope"} {
				require.Equal(t, before[r], counter(r), "an accepted token is not counted as a refusal")
			}
		})
	}
}

// Listing key actors changes nothing for a credential that is not delegated.
func TestVerifyKeyActorsDoNotChangeOrdinaryCredentials(t *testing.T) {
	defer setupInternalAuth(t)()
	asked := stubPlatformKeys(t)
	withKeyActors(t, keyActorClient)
	e := newExtEnv(t, adminMapping, false)
	e.revUsers["ak-ext"] = map[string]any{"sub": extUser, "username": "alice", "groups": []string{"platform-admins"}}
	for name, req := range map[string]VerifyTokenRequest{
		"oidc token":     {Authorization: "Bearer " + e.jwt(nil)},
		"introspected":   {ApiKey: "ak-ext"},
		"unknown opaque": {Authorization: "Bearer opaque-unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			*asked = nil
			req.IncludePlatformKey = true
			w := callVerifyToken(t, req, testInternalToken)
			if name == "unknown opaque" {
				require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
				return
			}
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			var resp verifyResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, UserTypeExternal, resp.Data.Type, "not reported as delegated")
			require.Equal(t, []string{extUser}, *asked)
		})
	}
}

// A key actor still has to hold the internal token of the services behind the apiserver; the delegated
// token alone opens nothing.
func TestVerifyKeyActorStillNeedsTheInternalToken(t *testing.T) {
	defer setupInternalAuth(t)()
	asked := stubPlatformKeys(t)
	withKeyActors(t, keyActorClient)
	e := newExtEnv(t, adminMapping, false)
	req := VerifyTokenRequest{Authorization: "Bearer " + e.delegatedJWT(keyActorClient, "safe.api", extUser), IncludePlatformKey: true}
	for name, internal := range map[string]string{"no internal token": "", "a wrong internal token": "not-the-token"} {
		t.Run(name, func(t *testing.T) {
			w := callVerifyToken(t, req, internal)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), "internal authentication required")
		})
	}
	require.Empty(t, *asked)
	list := &v1.UserList{}
	require.NoError(t, e.cli.List(context.Background(), list))
	require.Empty(t, list.Items)
}

// The request has no way to ask for the keys of anybody but the token's subject: fields that name another
// user are not part of it and change nothing.
func TestVerifyKeyActorCannotNameAnotherUser(t *testing.T) {
	defer setupInternalAuth(t)()
	asked := stubPlatformKeys(t)
	withKeyActors(t, keyActorClient)
	e := newExtEnv(t, adminMapping, false)
	tok := e.delegatedJWT(keyActorClient, "safe.api", extUser)
	body, err := json.Marshal(map[string]any{
		"authorization": "Bearer " + tok, "includePlatformKey": true, "includeVirtualKey": true,
		"id": otherUser, "userId": otherUser, "user": otherUser, "sub": otherUser, "subject": otherUser,
		"name": "bob", "email": "bob@example.com", "actor": "someone-else",
	})
	require.NoError(t, err)

	w := callVerifyRaw(t, string(body), testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp verifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, extUser, resp.Data.Id)
	require.Equal(t, "alice@example.com", resp.Data.Email)
	require.Equal(t, "pk-of-"+extUser, resp.Data.PlatformKey)
	require.Equal(t, []string{extUser}, *asked)
	list := &v1.UserList{}
	require.NoError(t, e.cli.List(context.Background(), list))
	require.Len(t, list.Items, 1, "only the subject was provisioned")
	require.Equal(t, extUser, list.Items[0].Name)
}

// Naming a key actor opens POST /auth/verify and nothing else: the routes a delegated token never calls
// stay refused through the real authentication entry point.
func TestKeyActorsOpenNothingButVerify(t *testing.T) {
	withKeyActors(t, keyActorClient)
	e := newExtEnv(t, adminMapping, false)
	tok := e.delegatedJWT(keyActorClient, "safe.api", extUser)
	for _, tt := range []struct{ name, method, path string }{
		{"verify through the request path", http.MethodPost, "/api/v1/auth/verify"},
		{"anything else under /auth", http.MethodGet, "/api/v1/auth/anything"},
		{"create api key", http.MethodPost, "/api/v1/apikeys"},
		{"list api keys", http.MethodGet, "/api/v1/apikeys"},
		{"read secret", http.MethodGet, "/api/v1/secrets/s1"},
		{"list users", http.MethodGet, "/api/v1/users"},
		{"add an ssh key", http.MethodPost, "/api/v1/publickeys"},
		{"reveal the gateway key", http.MethodGet, "/api/v1/llm-gateway/binding/reveal"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := extParse(t, tt.method, tt.path, tok)
			require.True(t, apierrors.IsForbidden(err), "%v", err)
			require.Contains(t, err.Error(), msgDelegatedForbidden)
		})
	}
	// Its ordinary access is what it was.
	_, err := extParse(t, http.MethodGet, "/api/v1/users/self", tok)
	require.NoError(t, err)
}

// The gateway virtual key of a delegated verdict is found by the email on the subject's own record, not by
// whatever the token says about the subject: here the token claims the email of another user, and only
// the record's email may be looked up (the mock refuses any other).
func TestVerifyKeyActorVirtualKeyIsFoundBySubjectRecord(t *testing.T) {
	defer setupInternalAuth(t)()
	stubPlatformKeys(t)
	withKeyActors(t, keyActorClient)
	e := newExtEnv(t, adminMapping, false)
	const stored, claimed = "alice-local@example.com", "bob@example.com"
	require.NoError(t, e.cli.Create(context.Background(), &v1.User{
		ObjectMeta: metav1.ObjectMeta{Name: extUser, Annotations: map[string]string{v1.UserNameAnnotation: "alice-local", v1.UserEmailAnnotation: stored}},
		Spec:       v1.UserSpec{Type: v1.DefaultUserType, Password: "hashed", Roles: []v1.UserRole{v1.DefaultRole}},
	}))

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), stored).Return(&dbclient.LLMGatewayUserBinding{UserEmail: stored}, nil).Times(1)
	oldKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB, Client: e.cli}
	defer func() { apiKeyTokenInstance = oldKey }()

	tok := e.jwt(map[string]any{"act": map[string]any{"sub": keyActorClient}, "scope": "safe.api", "email": claimed})
	w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + tok, IncludeVirtualKey: true}, testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp verifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, extUser, resp.Data.Id)
	require.Equal(t, stored, resp.Data.Email, "the subject's own email")
}
