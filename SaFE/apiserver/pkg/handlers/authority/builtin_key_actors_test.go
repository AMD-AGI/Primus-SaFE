/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package authority

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/golang/mock/gomock"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

const (
	bkaUser   = "alice-id"
	bkaOther  = "bob-id"
	bkaEmail  = "alice-local@example.com"
	bkaChat   = "svc-chat"
	bkaFrozen = "frozen-id"
)

// bkaEnv is builtin mode with auth.key_actors: a fake issuer (JWKS), the chain of auth.authenticators (an oidc
// authenticator, an introspection one that must never be asked, and a builtin entry), and a cluster that holds
// the users alice and bob and a frozen user.
type bkaEnv struct {
	*extEnv
	introspected atomic.Int64
	jwksDown     atomic.Bool
}

func newBuiltinKeyActorEnv(t *testing.T, actors []string, authenticators func(e *bkaEnv) []map[string]any) *bkaEnv {
	t.Helper()
	e := &bkaEnv{extEnv: &extEnv{t: t}}
	var err error
	e.key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		if e.jwksDown.Load() {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &e.key.PublicKey, KeyID: "k1", Algorithm: "ES256"}}})
	})
	mux.HandleFunc("/introspect", func(w http.ResponseWriter, r *http.Request) {
		e.introspected.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"active": true, "sub": bkaOther, "act": map[string]any{"client_id": keyActorClient}, "scope": "safe.api", "aud": extAudience})
	})
	e.issuer = httptest.NewServer(mux)
	t.Cleanup(e.issuer.Close)

	secretFile := filepath.Join(t.TempDir(), "client-secret")
	require.NoError(t, os.WriteFile(secretFile, []byte("x\n"), 0o600))
	chain := []map[string]any{
		{"type": "oidc", "issuer": e.issuer.URL, "audience": extAudience, "jwks_url": e.issuer.URL + "/jwks"},
		{"type": "introspection", "url": e.issuer.URL + "/introspect", "client_id": "c", "client_secret_file": secretFile, "audience": extAudience},
		{"type": "builtin"},
	}
	if authenticators != nil {
		chain = authenticators(e)
	}
	viper.Set("auth.mode", "builtin")
	viper.Set("auth.authenticators", chain)
	viper.Set("auth.key_actors", actors)

	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	users := []*v1.User{
		{ObjectMeta: metav1.ObjectMeta{Name: bkaUser, Annotations: map[string]string{v1.UserNameAnnotation: "alice", v1.UserEmailAnnotation: bkaEmail}},
			Spec: v1.UserSpec{Type: v1.DefaultUserType, Roles: []v1.UserRole{v1.DefaultRole}}},
		{ObjectMeta: metav1.ObjectMeta{Name: bkaOther, Annotations: map[string]string{v1.UserNameAnnotation: "bob", v1.UserEmailAnnotation: "bob@example.com"}},
			Spec: v1.UserSpec{Type: v1.SSOUserType, Roles: []v1.UserRole{v1.DefaultRole}}},
		{ObjectMeta: metav1.ObjectMeta{Name: bkaFrozen, Annotations: map[string]string{v1.UserNameAnnotation: "frozen", v1.UserEmailAnnotation: "frozen@example.com"}},
			Spec: v1.UserSpec{Type: v1.SSOUserType, RestrictedType: v1.UserFrozen, Roles: []v1.UserRole{v1.DefaultRole}}},
	}
	b := fake.NewClientBuilder().WithScheme(scheme)
	for _, u := range users {
		b = b.WithObjects(u)
	}
	e.cli = b.Build()
	require.NoError(t, InitExternalAuth(e.cli))
	t.Cleanup(func() {
		viper.Set("auth.mode", nil)
		viper.Set("auth.authenticators", nil)
		viper.Set("auth.key_actors", nil)
		require.NoError(t, InitExternalAuth(nil))
	})
	return e
}

// tok is what Claw shows SaFE for a chat user: the identity provider's token for audience extAudience, issued to the
// key actor (client_id) acting now, with the chat service that asked it earlier in the act chain.
func (e *bkaEnv) tok(over map[string]any) string {
	claims := map[string]any{
		"sub": bkaUser, "client_id": keyActorClient, "scope": "safe.api",
		"act":    map[string]any{"sub": keyActorClient, "act": map[string]any{"sub": bkaChat}},
		"groups": []string{"platform-admins"}, "email": "someone-else@example.com",
	}
	for k, v := range over {
		claims[k] = v
	}
	return e.jwt(claims)
}

func (e *bkaEnv) userCount() int {
	list := &v1.UserList{}
	require.NoError(e.t, e.cli.List(context.Background(), list))
	return len(list.Items)
}

// In builtin mode, with auth.key_actors and an oidc authenticator configured, POST /auth/verify answers a delegated
// token of a listed client that carries safe.api and the configured audience, signed by the issuer, with the keys of
// the existing user the token is for and of nobody else. Everything else is refused, with no key looked up and no
// user created.
func TestBuiltinKeyActors(t *testing.T) {
	defer setupInternalAuth(t)()
	defer setupDefaultToken(t)()
	otherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cases := []struct {
		name        string
		token       func(e *bkaEnv) string
		wantCode    int
		wantSubject string
		wantMessage string
	}{
		{name: "listed actor, safe.api, chat service earlier in the chain", token: func(e *bkaEnv) string { return e.tok(nil) },
			wantCode: http.StatusOK, wantSubject: bkaUser},
		{name: "listed actor, safe.api among other scopes", token: func(e *bkaEnv) string { return e.tok(map[string]any{"scope": "profile safe.api safe.read"}) },
			wantCode: http.StatusOK, wantSubject: bkaUser},
		{name: "another existing subject: the keys are that subject's", token: func(e *bkaEnv) string { return e.tok(map[string]any{"sub": bkaOther}) },
			wantCode: http.StatusOK, wantSubject: bkaOther},
		{name: "act.client_id is listed", token: func(e *bkaEnv) string {
			return e.tok(map[string]any{"act": map[string]any{"client_id": keyActorClient, "sub": "x"}, "client_id": nil})
		}, wantCode: http.StatusOK, wantSubject: bkaUser},

		{name: "wrong scope", token: func(e *bkaEnv) string { return e.tok(map[string]any{"scope": "safe.read"}) },
			wantCode: http.StatusForbidden, wantMessage: msgScope},
		{name: "no scope", token: func(e *bkaEnv) string { return e.tok(map[string]any{"scope": nil}) },
			wantCode: http.StatusForbidden, wantMessage: msgScope},
		{name: "a scope that only contains safe.api as a substring", token: func(e *bkaEnv) string { return e.tok(map[string]any{"scope": "safe.api.admin"}) },
			wantCode: http.StatusForbidden, wantMessage: msgScope},
		{name: "actor not listed", token: func(e *bkaEnv) string {
			return e.tok(map[string]any{"act": map[string]any{"sub": "svc-other"}, "client_id": "svc-other"})
		}, wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden},
		{name: "the chat service itself, not listed", token: func(e *bkaEnv) string {
			return e.tok(map[string]any{"act": map[string]any{"sub": bkaChat}, "client_id": bkaChat})
		}, wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden},
		{name: "only an earlier party of the chain is listed", token: func(e *bkaEnv) string {
			return e.tok(map[string]any{"act": map[string]any{"sub": "svc-other", "act": map[string]any{"sub": keyActorClient}}, "client_id": "svc-other"})
		}, wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden},
		{name: "act.sub is listed but the token names no client", token: func(e *bkaEnv) string {
			return e.tok(map[string]any{"act": map[string]any{"sub": keyActorClient}, "client_id": nil})
		}, wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden},
		{name: "ids are compared exactly", token: func(e *bkaEnv) string {
			return e.tok(map[string]any{"act": map[string]any{"sub": "SVC-BOT"}, "client_id": "SVC-BOT"})
		}, wantCode: http.StatusForbidden, wantMessage: msgDelegatedForbidden},
		{name: "a frozen user has no keys", token: func(e *bkaEnv) string { return e.tok(map[string]any{"sub": bkaFrozen}) },
			wantCode: http.StatusForbidden, wantMessage: "the user is restricted"},
		{name: "the provider restricts the user", token: func(e *bkaEnv) string { return e.tok(map[string]any{"restricted": true}) },
			wantCode: http.StatusForbidden, wantMessage: "the user is restricted"},

		{name: "a subject with no user here: none is created", token: func(e *bkaEnv) string { return e.tok(map[string]any{"sub": "nobody-id"}) },
			wantCode: http.StatusUnauthorized, wantMessage: ErrInvalidToken},
		{name: "a subject that is not a user id", token: func(e *bkaEnv) string { return e.tok(map[string]any{"sub": "Not/A_Name"}) },
			wantCode: http.StatusUnauthorized, wantMessage: ErrInvalidToken},
		{name: "wrong audience", token: func(e *bkaEnv) string { return e.tok(map[string]any{"aud": []string{"claw-api"}}) },
			wantCode: http.StatusUnauthorized},
		{name: "expired", token: func(e *bkaEnv) string { return e.tok(map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}) },
			wantCode: http.StatusUnauthorized},
		{name: "not signed by the issuer", token: func(e *bkaEnv) string {
			claims := map[string]any{"iss": e.issuer.URL, "sub": bkaUser, "aud": []string{extAudience}, "exp": time.Now().Add(time.Minute).Unix(),
				"client_id": keyActorClient, "scope": "safe.api", "act": map[string]any{"sub": keyActorClient}}
			sg, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: otherKey}, (&jose.SignerOptions{}).WithHeader(jose.HeaderKey("kid"), "k1"))
			require.NoError(t, err)
			raw, err := jwt.Signed(sg).Claims(claims).Serialize()
			require.NoError(t, err)
			return raw
		}, wantCode: http.StatusUnauthorized},
		{name: "another issuer", token: func(e *bkaEnv) string { return e.tok(map[string]any{"iss": "https://elsewhere.example.com"}) },
			wantCode: http.StatusUnauthorized},
		{name: "a token of the issuer that is not delegated is still unknown here", token: func(e *bkaEnv) string {
			return e.tok(map[string]any{"act": nil, "client_id": nil})
		}, wantCode: http.StatusUnauthorized},
		{name: "the issuer's keys cannot be fetched", token: func(e *bkaEnv) string { e.jwksDown.Store(true); return e.tok(nil) },
			wantCode: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			asked := stubPlatformKeys(t)
			e := newBuiltinKeyActorEnv(t, []string{"svc-another", keyActorClient}, nil)
			users := e.userCount()

			w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + tc.token(e), IncludePlatformKey: true}, testInternalToken)
			require.Equal(t, tc.wantCode, w.Code, w.Body.String())
			require.Equal(t, users, e.userCount(), "builtin mode creates no user")
			require.Zero(t, e.introspected.Load(), "an introspection authenticator is never asked in builtin mode")
			if tc.wantCode != http.StatusOK {
				require.Empty(t, *asked, "no key is looked up for a refused token")
				if tc.wantMessage != "" {
					var body struct {
						Message string `json:"message"`
					}
					require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
					require.Equal(t, tc.wantMessage, body.Message)
				}
				return
			}
			var resp verifyResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.Equal(t, tc.wantSubject, resp.Data.Id, "the verdict is about the token's subject")
			require.Equal(t, UserTypeDelegated, resp.Data.Type)
			require.Equal(t, "pk-of-"+tc.wantSubject, resp.Data.PlatformKey)
			require.Equal(t, []string{tc.wantSubject}, *asked, "keys were looked up for the subject, and only for it")
			require.Equal(t, []v1.UserRole{v1.DefaultRole}, resp.Data.Roles, "the stored roles, never the token's groups")
		})
	}
}

// The gateway virtual key is found by the email on the subject's own record, never by the token's email claim.
func TestBuiltinKeyActorVirtualKeyIsFoundBySubjectRecord(t *testing.T) {
	defer setupInternalAuth(t)()
	stubPlatformKeys(t)
	e := newBuiltinKeyActorEnv(t, []string{keyActorClient}, nil)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockDB := mock_client.NewMockInterface(ctrl)
	mockDB.EXPECT().GetLLMBindingByEmail(gomock.Any(), bkaEmail).Return(nil, nil).Times(1)
	oldKey := apiKeyTokenInstance
	apiKeyTokenInstance = &ApiKeyToken{dbClient: mockDB, Client: e.cli}
	defer func() { apiKeyTokenInstance = oldKey }()

	w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + e.tok(nil), IncludeVirtualKey: true}, testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp verifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, bkaUser, resp.Data.Id)
	require.Equal(t, bkaEmail, resp.Data.Email)
	require.Equal(t, "alice", resp.Data.Name)
}

// Off unless both auth.key_actors and an oidc authenticator are configured: a delegated token is then the unknown
// credential it has always been in builtin mode (401), and no key is looked up.
func TestBuiltinKeyActorsOffByDefault(t *testing.T) {
	defer setupInternalAuth(t)()
	defer setupDefaultToken(t)()
	cases := map[string]struct {
		actors []string
		chain  func(e *bkaEnv) []map[string]any
	}{
		"no key actor":         {actors: nil},
		"only blank key actor": {actors: []string{" ", ""}},
		"no authenticator":     {actors: []string{keyActorClient}, chain: func(*bkaEnv) []map[string]any { return nil }},
		"only introspection and builtin": {actors: []string{keyActorClient}, chain: func(e *bkaEnv) []map[string]any {
			return []map[string]any{{"type": "introspection", "url": e.issuer.URL + "/introspect", "client_id": "c", "client_secret_file": "/nonexistent", "audience": extAudience}, {"type": "builtin"}}
		}},
		"an oidc authenticator that cannot be built": {actors: []string{keyActorClient}, chain: func(e *bkaEnv) []map[string]any {
			return []map[string]any{{"type": "oidc", "issuer": e.issuer.URL}}
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			asked := stubPlatformKeys(t)
			e := newBuiltinKeyActorEnv(t, tc.actors, tc.chain)
			require.Nil(t, builtinKeyActorState.Load())
			w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + e.tok(nil), IncludePlatformKey: true}, testInternalToken)
			require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
			require.Empty(t, *asked)
			require.Zero(t, e.introspected.Load())
		})
	}
}

// The apiserver's own credentials are answered by the builtin path exactly as before when key actors are configured,
// and no request other than POST /auth/verify accepts a delegated token.
func TestBuiltinKeyActorsLeaveBuiltinCredentialsAlone(t *testing.T) {
	defer setupInternalAuth(t)()
	defer setupDefaultToken(t)()
	stubPlatformKeys(t)
	e := newBuiltinKeyActorEnv(t, []string{keyActorClient}, nil)
	require.NotNil(t, builtinKeyActorState.Load())
	require.Nil(t, externalState.Load(), "the mode is still builtin")

	w := callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + generateTestToken(t, "user-300", "eve")}, testInternalToken)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp verifyResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, "user-300", resp.Data.Id)
	require.Equal(t, string(v1.DefaultUserType), resp.Data.Type)

	// A request to the API with the same delegated token is refused: only /auth/verify knows it.
	_, err := extParse(t, http.MethodGet, "/api/v1/users/self", e.tok(nil))
	require.Error(t, err)

	// The internal token is still required.
	w = callVerifyToken(t, VerifyTokenRequest{Authorization: "Bearer " + e.tok(nil), IncludePlatformKey: true}, "wrong")
	require.Equal(t, http.StatusForbidden, w.Code)
}

// Switching to external mode turns the builtin-mode path off: the external chain is then the only one.
func TestExternalModeTurnsBuiltinKeyActorsOff(t *testing.T) {
	newBuiltinKeyActorEnv(t, []string{keyActorClient}, nil)
	require.NotNil(t, builtinKeyActorState.Load())
	withKeyActors(t, keyActorClient)
	newExtEnv(t, adminMapping, false)
	require.Nil(t, builtinKeyActorState.Load())
}
