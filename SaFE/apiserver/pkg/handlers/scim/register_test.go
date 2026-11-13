/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scim

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// Registration stops on a configuration error, succeeds against the identity service, and stops when the context ends during a retry.
func TestRegisterLoop(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: commonconfig.GetSCIMTokenSecret(), Namespace: common.PrimusSafeNamespace},
		Data:       map[string][]byte{tokenKey: []byte("tok")},
	}
	cli := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	tok := NewToken("")

	require.False(t, registerLoop(context.Background(), cli, tok))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret"), []byte("s3cret\n"), 0o600))
	idn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(idn.Close)
	viper.Set("auth.scim.identity_url", idn.URL)
	viper.Set("auth.scim.public_url", "https://safe.example.com/scim/v2")
	viper.Set("auth.authenticators", []map[string]any{{
		"type": "introspection", "client_id": "svc", "client_secret_file": filepath.Join(dir, "secret"),
	}})
	require.True(t, registerLoop(context.Background(), cli, tok))
	require.Equal(t, "tok", tok.Load())

	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(refused.Close)
	viper.Set("auth.scim.identity_url", refused.URL)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() { done <- registerLoop(ctx, cli, tok) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case ok := <-done:
		require.False(t, ok)
	case <-time.After(3 * time.Second):
		t.Fatal("register loop did not stop")
	}

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	StartRegistration(ctx, cli, tok)
	time.Sleep(20 * time.Millisecond)
}
