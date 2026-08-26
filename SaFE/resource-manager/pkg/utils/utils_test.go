/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	testifyassert "github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestRemoveOwnerReferences tests the removal of owner references by UID
func TestRemoveOwnerReferences(t *testing.T) {
	uid1 := types.UID("uid-1")
	uid2 := types.UID("uid-2")
	uid3 := types.UID("uid-3")

	tests := []struct {
		name        string
		references  []metav1.OwnerReference
		uidToRemove types.UID
		expected    []metav1.OwnerReference
	}{
		{
			name: "remove single reference",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid2, Name: "owner2"},
			},
			uidToRemove: uid1,
			expected: []metav1.OwnerReference{
				{UID: uid2, Name: "owner2"},
			},
		},
		{
			name: "remove non-existent UID",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid2, Name: "owner2"},
			},
			uidToRemove: uid3,
			expected: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid2, Name: "owner2"},
			},
		},
		{
			name:        "remove from empty list",
			references:  []metav1.OwnerReference{},
			uidToRemove: uid1,
			expected:    []metav1.OwnerReference{},
		},
		{
			name: "remove all references with same UID",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
				{UID: uid1, Name: "owner1-duplicate"},
				{UID: uid2, Name: "owner2"},
			},
			uidToRemove: uid1,
			expected: []metav1.OwnerReference{
				{UID: uid2, Name: "owner2"},
			},
		},
		{
			name: "remove last reference",
			references: []metav1.OwnerReference{
				{UID: uid1, Name: "owner1"},
			},
			uidToRemove: uid1,
			expected:    []metav1.OwnerReference{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RemoveOwnerReferences(tt.references, tt.uidToRemove)
			assert.Equal(t, len(tt.expected), len(result))
			for i, ref := range result {
				assert.Equal(t, tt.expected[i].UID, ref.UID)
				assert.Equal(t, tt.expected[i].Name, ref.Name)
			}
		})
	}
}

// --- merged from utils_extra_test.go ---

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
	testifyassert.NoError(t, err)

	// No-op when finalizer absent.
	node2 := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n2"}}
	cl2 := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).WithObjects(node2).Build()
	testifyassert.NoError(t, RemoveFinalizer(context.Background(), cl2, node2, "missing"))
}

func TestIncRetryCount(t *testing.T) {
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).WithObjects(node).Build()

	// First increment -> 1.
	count, err := IncRetryCount(context.Background(), cl, node, 5)
	testifyassert.NoError(t, err)
	assert.Equal(t, 1, count)

	// Over the max -> returns count without patching.
	node.Annotations = map[string]string{v1.RetryCountAnnotation: "5"}
	count, err = IncRetryCount(context.Background(), cl, node, 5)
	testifyassert.NoError(t, err)
	assert.Equal(t, 6, count)
}

func TestGetK8sClientFactory(t *testing.T) {
	// Nil manager -> error.
	_, err := GetK8sClientFactory(nil, "c")
	testifyassert.Error(t, err)

	// Missing cluster -> not-found error.
	mgr := commonutils.NewObjectManager()
	_, err = GetK8sClientFactory(mgr, "missing")
	testifyassert.Error(t, err)
}

func TestGetSSHConfigNoSecret(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).Build()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	// No SSHSecret reference -> error.
	_, err := GetSSHConfig(context.Background(), cl, node)
	testifyassert.Error(t, err)
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
	testifyassert.NoError(t, err)
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
	testifyassert.Error(t, err)
}

func TestGetSSHConfigSecretNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(utilsScheme(t)).Build()
	node := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n"}}
	node.Spec.SSHSecret = &corev1.ObjectReference{Name: "missing", Namespace: "ns"}
	_, err := GetSSHConfig(context.Background(), cl, node)
	testifyassert.Error(t, err)
}
