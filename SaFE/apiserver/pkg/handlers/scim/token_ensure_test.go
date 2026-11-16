/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// A missing Secret is created. An empty token and a read error are refused.
func TestEnsureTokenCreatesTheSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	cli := fake.NewClientBuilder().WithScheme(scheme).Build()
	token, err := EnsureToken(context.Background(), cli)
	require.NoError(t, err)
	require.Len(t, token, 64)

	name := commonconfig.GetSCIMTokenSecret()
	secret := &corev1.Secret{}
	require.NoError(t, cli.Get(context.Background(), client.ObjectKey{Namespace: common.PrimusSafeNamespace, Name: name}, secret))
	secret.Data[tokenKey] = []byte("  ")
	require.NoError(t, cli.Update(context.Background(), secret))
	_, err = EnsureToken(context.Background(), cli)
	require.ErrorContains(t, err, tokenKey)

	broken := interceptor.NewClient(cli, interceptor.Funcs{
		Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
			return apierrors.NewInternalError(context.Canceled)
		},
	})
	_, err = EnsureToken(context.Background(), broken)
	require.Error(t, err)

	require.False(t, bearerMatches("Basic secret", "secret"))
	require.False(t, bearerMatches("Bearer ", ""))
}
