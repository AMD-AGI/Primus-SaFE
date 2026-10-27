/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package identityexchange_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/identityexchange"
)

func TestLoadOrCreateKeySetSharesTheSecretAcrossReplicas(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()

	first, err := identityexchange.LoadOrCreateKeySet(context.Background(), cli, "ns", identityexchange.AlgRS256)
	require.NoError(t, err)
	require.Equal(t, identityexchange.AlgRS256, first.Algorithm())
	// A second replica reads the key the first one created instead of making its own.
	second, err := identityexchange.LoadOrCreateKeySet(context.Background(), cli, "ns", identityexchange.AlgES256)
	require.NoError(t, err)
	require.Equal(t, first.KeyID(), second.KeyID())

	secret := &corev1.Secret{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: identityexchange.SigningSecretName}, secret))
	require.NotEmpty(t, secret.Data[identityexchange.SigningSecretCurrent])

	// A secret without a key is a misconfiguration, not a reason to sign with nothing.
	delete(secret.Data, identityexchange.SigningSecretCurrent)
	require.NoError(t, cli.Update(context.Background(), secret))
	_, err = identityexchange.LoadOrCreateKeySet(context.Background(), cli, "ns", "")
	require.Error(t, err)
}

func TestSigningKeyValidation(t *testing.T) {
	_, err := identityexchange.GenerateSigningKey("HS256")
	require.Error(t, err)
	_, err = identityexchange.NewKeySet([]byte("not pem"), nil)
	require.Error(t, err)
	good, err := identityexchange.GenerateSigningKey("")
	require.NoError(t, err)
	_, err = identityexchange.NewKeySet(good, []byte("not pem"))
	require.Error(t, err)
}

func TestUserStore(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1.AddToScheme(scheme))
	user := &v1.User{}
	user.Name = "u1"
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(user).Build()
	store := identityexchange.NewUserStore(cli)
	got, err := store.GetUser(context.Background(), "u1")
	require.NoError(t, err)
	require.Equal(t, "u1", got.Name)
	_, err = store.GetUser(context.Background(), "missing")
	require.ErrorIs(t, err, identityexchange.ErrUserNotFound)
}

func TestClientEnableAndMisc(t *testing.T) {
	e := newEnv(t, identityexchange.AlgES256)
	e.store.Clients[clientId].Disabled = true
	require.Equal(t, testAudience, e.svc.Audience())
	_, oerr := e.svc.Exchange(context.Background(), baseReq())
	require.NotNil(t, oerr)
	ok, err := e.store.SetServiceClientDisabled(context.Background(), clientId, false)
	require.NoError(t, err)
	require.True(t, ok)
	_, oerr = e.svc.Exchange(context.Background(), baseReq())
	require.Nil(t, oerr)
	require.Contains(t, (&identityexchange.OAuthError{Code: "x", Description: "y"}).Error(), "x")

	// Source allow-list: single IP, CIDR, IPv6 and an unparsable client address.
	for _, tt := range []struct {
		list, ip string
		want     bool
	}{
		{`["192.0.2.7"]`, "192.0.2.7", true},
		{`["192.0.2.7"]`, "192.0.2.8", false},
		{`["10.0.0.0/8"]`, "10.9.9.9", true},
		{`["2001:db8::/32"]`, "2001:db8::1", true},
		{`["10.0.0.0/8"]`, "not-an-ip", false},
		{`not json`, "10.0.0.1", true},
	} {
		e.store.Clients[clientId].Whitelist = tt.list
		req := baseReq()
		req.ClientIP = tt.ip
		_, oerr := e.svc.Exchange(context.Background(), req)
		require.Equal(t, tt.want, oerr == nil, "%s from %s", tt.list, tt.ip)
	}
}
