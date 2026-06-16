/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func coreScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(s))
	require.NoError(t, v1.AddToScheme(s))
	return s
}

// TestGetAndValidateImageSecretSuccess verifies an image-type secret is returned.
func TestGetAndValidateImageSecretSuccess(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sec1",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.SecretTypeLabel: string(v1.SecretImage)},
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithObjects(secret).Build()
	h := &ImageHandler{Client: cl}

	got, err := h.getAndValidateImageSecret(context.Background(), "sec1")
	require.NoError(t, err)
	assert.Equal(t, "sec1", got.Name)
}

// TestGetAndValidateImageSecretNotFound verifies a missing secret yields an error.
func TestGetAndValidateImageSecretNotFound(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	_, err := h.getAndValidateImageSecret(context.Background(), "missing")
	assert.Error(t, err)
}

// TestGetAndValidateImageSecretWrongType verifies a non-image secret yields an error.
func TestGetAndValidateImageSecretWrongType(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sec2",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.SecretTypeLabel: "other"},
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithObjects(secret).Build()
	h := &ImageHandler{Client: cl}
	_, err := h.getAndValidateImageSecret(context.Background(), "sec2")
	assert.Error(t, err)
}

// TestCreateMergedAuthConfigMap verifies system and user auths are merged into a ConfigMap.
func TestCreateMergedAuthConfigMap(t *testing.T) {
	systemSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.ImageImportSecretName,
			Namespace: common.PrimusSafeNamespace,
		},
		Data: map[string][]byte{
			"config.json": []byte(`{"auths":{"sys.io":{"auth":"a"}}}`),
		},
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).WithObjects(systemSecret).Build()
	h := &ImageHandler{Client: cl}

	userSecret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"user.io":{"auth":"b"}}}`),
		},
	}
	cm, err := h.createMergedAuthConfigMap(context.Background(), "job1", userSecret)
	require.NoError(t, err)
	assert.Contains(t, cm.Data["config.json"], "sys.io")
	assert.Contains(t, cm.Data["config.json"], "user.io")

	created := &corev1.ConfigMap{}
	require.NoError(t, cl.Get(context.Background(), ctrlclient.ObjectKey{
		Name: "job1-auth", Namespace: common.PrimusSafeNamespace,
	}, created))
}

// TestCreateMergedAuthConfigMapMissingSystemSecret verifies an error when system secret is absent.
func TestCreateMergedAuthConfigMapMissingSystemSecret(t *testing.T) {
	cl := ctrlfake.NewClientBuilder().WithScheme(coreScheme(t)).Build()
	h := &ImageHandler{Client: cl}
	_, err := h.createMergedAuthConfigMap(context.Background(), "job1", &corev1.Secret{})
	assert.Error(t, err)
}