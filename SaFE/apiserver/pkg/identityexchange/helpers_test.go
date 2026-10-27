/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange/fake"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonuser "github.com/AMD-AIG-AIMA/SAFE/common/pkg/user"
)

const (
	testIssuer   = "https://safe.example.test"
	testAudience = "safe-apiserver"
	otherAud     = "other-service"
	clientId     = "svc-test"
	clientSecret = "s3cret-value"
	aliceEmail   = "alice@example.test"
)

var aliceId = commonuser.GenerateUserIdByName(aliceEmail)

type env struct {
	svc   *identityexchange.Service
	store *fake.Store
	users *fake.Users
	keys  *identityexchange.KeySet
	now   *time.Time
}

func (e *env) advance(d time.Duration) { *e.now = e.now.Add(d) }

// newEnv builds a service with one client (allowed for two audiences and both
// scopes), one user "alice" and a confirmed binding external:idp|alice-ext.
func newEnv(t *testing.T, alg string) *env {
	t.Helper()
	pemBytes, err := identityexchange.GenerateSigningKey(alg)
	require.NoError(t, err)
	keys, err := identityexchange.NewKeySet(pemBytes, nil)
	require.NoError(t, err)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	e := &env{store: fake.NewStore(), users: fake.NewUsers(), keys: keys, now: &now}
	e.svc, err = identityexchange.NewService(identityexchange.Config{
		Issuer: testIssuer, Audience: testAudience, Now: func() time.Time { return *e.now },
	}, keys, e.store, e.users)
	require.NoError(t, err)

	require.NoError(t, e.store.InsertServiceClient(context.Background(), &dbclient.ServiceClient{
		ClientId: clientId, Name: "test", SecretHash: identityexchange.HashClientSecret(clientSecret),
		AuthMethod: identityexchange.AuthMethodSecretBasic, MaxTtlSeconds: 600,
		AllowedAudiences: `["safe-apiserver","other-service"]`,
		AllowedScopes:    `["safe.api","safe.read"]`, Whitelist: `[]`,
	}))
	e.users.Add(aliceId, "alice", aliceEmail)
	e.bind(t, aliceId, "idp", "alice-ext")
	return e
}

func (e *env) bind(t *testing.T, userId, issuer, subject string) {
	t.Helper()
	require.NoError(t, e.store.InsertIdentityBinding(context.Background(), &dbclient.IdentityBinding{
		ClientId: clientId, Issuer: issuer, Subject: subject, UserId: userId,
	}))
}

func baseReq() identityexchange.ExchangeRequest {
	return identityexchange.ExchangeRequest{
		ClientId: clientId, ClientSecret: clientSecret, ClientIP: "10.0.0.5",
		GrantType:        identityexchange.GrantTypeTokenExchange,
		SubjectToken:     "external:idp|alice-ext",
		SubjectTokenType: identityexchange.SubjectTokenTypeUserRef,
		Audience:         testAudience, Scope: "safe.api",
	}
}

// exchange runs a successful exchange and returns the access token.
func (e *env) exchange(t *testing.T, mut func(*identityexchange.ExchangeRequest)) string {
	t.Helper()
	req := baseReq()
	if mut != nil {
		mut(&req)
	}
	resp, oerr := e.svc.Exchange(context.Background(), req)
	require.Nil(t, oerr)
	require.NotNil(t, resp)
	return resp.AccessToken
}

func (e *env) lastAudit(t *testing.T) *dbclient.AuditLog {
	t.Helper()
	rows := e.store.AuditRows()
	require.NotEmpty(t, rows)
	return rows[len(rows)-1]
}

func auditBody(t *testing.T, l *dbclient.AuditLog) map[string]string {
	t.Helper()
	var m map[string]string
	require.NoError(t, json.Unmarshal([]byte(l.RequestBody.String), &m))
	return m
}

var _ = sql.ErrNoRows

func bindingFor(client, user string) *dbclient.IdentityBinding {
	return &dbclient.IdentityBinding{ClientId: client, Issuer: "idp", Subject: "x-" + user, UserId: user}
}
