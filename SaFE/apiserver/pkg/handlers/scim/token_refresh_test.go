/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

func TestRefreshTokenFollowsTheSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: commonconfig.GetSCIMTokenSecret(), Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{tokenKey: []byte("first")},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	ctx := context.Background()
	tok := NewToken("first")

	changed, err := refreshToken(ctx, cli, tok)
	require.NoError(t, err)
	require.False(t, changed)

	// A rotated Secret replaces the token the endpoint accepts.
	secret.Data[tokenKey] = []byte("second")
	require.NoError(t, cli.Update(ctx, secret))
	changed, err = refreshToken(ctx, cli, tok)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "second", tok.Load())

	s := &server{token: tok}
	accepted := func(bearer string) bool {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/scim/v2/Users", nil)
		c.Request.Header.Set("Authorization", "Bearer "+bearer)
		s.authenticate(c)
		return !c.IsAborted()
	}
	require.True(t, accepted("second"))
	require.False(t, accepted("first"))

	// A deleted Secret is created again with a new token.
	require.NoError(t, cli.Delete(ctx, secret))
	changed, err = refreshToken(ctx, cli, tok)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEqual(t, "second", tok.Load())
	stored := &corev1.Secret{}
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(secret), stored))
	require.Equal(t, tok.Load(), string(stored.Data[tokenKey]))
}
