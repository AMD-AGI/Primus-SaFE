/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func utilsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRemoveFinalizer(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", Finalizers: []string{"fz"}}}
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).WithObjects(node).Build()
	err := RemoveFinalizer(context.Background(), cl, node, "fz")
	assert.NoError(t, err)

	// No-op when finalizer absent.
	node2 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}
	cl2 := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).WithObjects(node2).Build()
	assert.NoError(t, RemoveFinalizer(context.Background(), cl2, node2, "missing"))
}

func TestIncRetryCount(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).WithObjects(node).Build()

	// First increment -> 1.
	count, err := IncRetryCount(context.Background(), cl, node, 5)
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	// Over the max -> returns count without patching.
	node.Annotations = map[string]string{v1.RetryCountAnnotation: "5"}
	count, err = IncRetryCount(context.Background(), cl, node, 5)
	assert.NoError(t, err)
	assert.Equal(t, 6, count)
}

func TestGetK8sClientFactory(t *testing.T) {
	// Nil manager -> error.
	_, err := GetK8sClientFactory(nil, "c")
	assert.Error(t, err)

	// Missing cluster -> not-found error.
	mgr := commonutils.NewObjectManager()
	_, err = GetK8sClientFactory(mgr, "missing")
	assert.Error(t, err)
}

func TestGetSSHConfigNoSecret(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).Build()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	// No SSHSecret reference -> error.
	_, err := GetSSHConfig(context.Background(), cl, node)
	assert.Error(t, err)
}

func TestGetSSHConfigWithPassword(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ssh", Namespace: "ns"},
		Data:       map[string][]byte{Username: []byte("admin"), Password: []byte("pw")},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).WithObjects(secret).Build()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	node.Spec.SSHSecret = &corev1.ObjectReference{Name: "ssh", Namespace: "ns"}

	cfg, err := GetSSHConfig(context.Background(), cl, node)
	assert.NoError(t, err)
	assert.Equal(t, "admin", cfg.User)
	assert.Equal(t, 1, len(cfg.Auth))
}

func TestGetSSHConfigNoAuth(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ssh", Namespace: "ns"},
		Data:       map[string][]byte{},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).WithObjects(secret).Build()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	node.Spec.SSHSecret = &corev1.ObjectReference{Name: "ssh", Namespace: "ns"}

	// Neither key nor password -> error.
	_, err := GetSSHConfig(context.Background(), cl, node)
	assert.Error(t, err)
}

func TestGetSSHConfigSecretNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).Build()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	node.Spec.SSHSecret = &corev1.ObjectReference{Name: "missing", Namespace: "ns"}
	_, err := GetSSHConfig(context.Background(), cl, node)
	assert.Error(t, err)
}
