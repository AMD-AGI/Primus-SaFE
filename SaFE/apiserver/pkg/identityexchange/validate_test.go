/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
)

func TestValidateAccessToken(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(t *testing.T, e *env) (token, audience string)
		wantErr string
	}{
		{"valid", func(t *testing.T, e *env) (string, string) { return e.exchange(t, nil), "" }, ""},
		{"expired", func(t *testing.T, e *env) (string, string) {
			tok := e.exchange(t, nil)
			e.advance(10*time.Minute + time.Minute) // past exp and the clock leeway
			return tok, ""
		}, "exp"},
		{"not yet valid", func(t *testing.T, e *env) (string, string) {
			tok := e.exchange(t, nil)
			e.advance(-time.Hour)
			return tok, ""
		}, "not valid yet"},
		{"audience of another service is refused by the apiserver", func(t *testing.T, e *env) (string, string) {
			return e.exchange(t, func(r *identityexchange.ExchangeRequest) { r.Audience = otherAud }), ""
		}, "audience"},
		{"the same token is valid for the audience it was issued for", func(t *testing.T, e *env) (string, string) {
			return e.exchange(t, func(r *identityexchange.ExchangeRequest) { r.Audience = otherAud }), otherAud
		}, ""},
		{"wrong audience asked of a token", func(t *testing.T, e *env) (string, string) { return e.exchange(t, nil), otherAud }, "audience"},
		{"signature tampered", func(t *testing.T, e *env) (string, string) {
			tok := e.exchange(t, nil)
			parts := strings.Split(tok, ".")
			payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var m map[string]any
			require.NoError(t, json.Unmarshal(payload, &m))
			m["sub"] = "someone-else"
			payload, _ = json.Marshal(m)
			parts[1] = base64.RawURLEncoding.EncodeToString(payload)
			return strings.Join(parts, "."), ""
		}, "signature"},
		{"signed by another key", func(t *testing.T, e *env) (string, string) {
			other := newEnv(t, identityexchange.AlgES256)
			return other.exchange(t, nil), ""
		}, "key id"},
		{"client disabled after issue", func(t *testing.T, e *env) (string, string) {
			tok := e.exchange(t, nil)
			_, _ = e.store.SetServiceClientDisabled(context.Background(), clientId, true)
			return tok, ""
		}, "client disabled"},
		{"binding revoked after issue", func(t *testing.T, e *env) (string, string) {
			tok := e.exchange(t, nil)
			ok, err := e.store.DeleteIdentityBinding(context.Background(), e.store.Bindings[0].Id, aliceId)
			require.NoError(t, err)
			require.True(t, ok)
			return tok, ""
		}, "binding revoked"},
		{"user deleted after issue", func(t *testing.T, e *env) (string, string) {
			tok := e.exchange(t, nil)
			delete(e.users.M, aliceId)
			return tok, ""
		}, "user lookup"},
		{"not a token", func(t *testing.T, e *env) (string, string) { return "garbage", "" }, "parse"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, identityexchange.AlgES256)
			tok, aud := tt.prepare(t, e)
			p, err := e.svc.ValidateAccessToken(context.Background(), tok, aud)
			if tt.wantErr == "" {
				require.NoError(t, err)
				require.Equal(t, aliceId, p.UserId)
				return
			}
			require.Error(t, err)
			require.Contains(t, strings.ToLower(err.Error()), tt.wantErr)
		})
	}
}

func TestKeyRotationKeepsPreviousKeyValid(t *testing.T) {
	oldPEM, err := identityexchange.GenerateSigningKey("")
	require.NoError(t, err)
	newPEM, err := identityexchange.GenerateSigningKey("")
	require.NoError(t, err)
	oldKeys, err := identityexchange.NewKeySet(oldPEM, nil)
	require.NoError(t, err)
	rotated, err := identityexchange.NewKeySet(newPEM, oldPEM)
	require.NoError(t, err)
	require.Len(t, rotated.JWKS().Keys, 2)
	require.Equal(t, oldKeys.KeyID(), rotated.JWKS().Keys[1].KeyID)
	for _, k := range rotated.JWKS().Keys {
		require.True(t, k.IsPublic(), "the JWKS must hold public keys only")
	}
}

func TestIsExchangeTokenDecidesByTypeAndIssuer(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	tok := e.exchange(t, nil)
	require.True(t, e.svc.IsExchangeToken(tok))
	for name, raw := range map[string]string{
		"api key":       "ak-abcdef",
		"session id":    "0123456789abcdef",
		"empty":         "",
		"two segments":  "a.b",
		"junk segments": "a.b.c",
	} {
		require.False(t, e.svc.IsExchangeToken(raw), name)
	}
	// A bare JWT of another issuer (what an SSO login yields without a database) is not ours.
	foreign := newEnv(t, identityexchange.AlgES256)
	svc2, err := identityexchange.NewService(identityexchange.Config{Issuer: "https://elsewhere.example.test", Audience: testAudience},
		foreign.keys, foreign.store, foreign.users)
	require.NoError(t, err)
	require.False(t, svc2.IsExchangeToken(tok))
}

func TestNewServiceRequiresIssuerAndAudience(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	_, err := identityexchange.NewService(identityexchange.Config{Audience: "a"}, e.keys, e.store, e.users)
	require.Error(t, err)
	_, err = identityexchange.NewService(identityexchange.Config{Issuer: "https://x"}, e.keys, e.store, e.users)
	require.Error(t, err)
}
