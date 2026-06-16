/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestGetDesiredImagePullSecret(t *testing.T) {
	h := &ImageHandler{}
	// Registries without username are skipped; result is a valid empty-auth secret.
	secret, err := h.getDesiredImagePullSecret([]*model.RegistryInfo{
		{Name: "r1", URL: "harbor.io", Username: ""},
	})
	assert.NoError(t, err)
	assert.Equal(t, ImagePullSecretName, secret.Name)
	assert.Equal(t, corev1.SecretTypeDockerConfigJson, secret.Type)
	assert.Contains(t, secret.StringData, ".dockerconfigjson")
}

func TestGetDesiredImageImportSecret(t *testing.T) {
	h := &ImageHandler{}
	secret, err := h.getDesiredImageImportSecret([]*model.RegistryInfo{
		{Name: "r1", URL: "harbor.io", Username: ""},
	})
	assert.NoError(t, err)
	assert.Equal(t, corev1.SecretTypeOpaque, secret.Type)
	assert.Contains(t, secret.StringData, "config.json")
}

func TestRefreshImagePullSecretsCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return([]*model.RegistryInfo{}, nil)

	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset()}
	assert.NoError(t, h.refreshImagePullSecrets(context.Background()))
}

func TestRefreshImageImportSecretsCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return([]*model.RegistryInfo{}, nil)

	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset()}
	assert.NoError(t, h.refreshImageImportSecrets(context.Background()))
}

func TestRefreshImagePullSecretsDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return(nil, errors.New("db error"))

	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset()}
	assert.Error(t, h.refreshImagePullSecrets(context.Background()))
}

func TestRefreshImagePullSecretsUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListRegistryInfos(gomock.Any(), 1, -1).Return([]*model.RegistryInfo{}, nil)

	// Pre-create the secret so the update path is exercised.
	existing := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{
		Name:      ImagePullSecretName,
		Namespace: "primus-safe",
	}}
	h := &ImageHandler{dbClient: m, clientSet: k8sfake.NewSimpleClientset(existing)}
	assert.NoError(t, h.refreshImagePullSecrets(context.Background()))
}

func TestCvtCreateRegistryRequestToRegistryInfoEmptyCreds(t *testing.T) {
	h := &ImageHandler{}
	info, err := h.cvtCreateRegistryRequestToRegistryInfo(&CreateRegistryRequest{
		Name: "reg", Url: "https://harbor.io", Default: true,
	})
	assert.NoError(t, err)
	assert.Equal(t, "reg", info.Name)
	assert.Equal(t, "https://harbor.io", info.URL)
	assert.Empty(t, info.Password)
	assert.Empty(t, info.Username)
	assert.True(t, info.Default)
}

func TestCvtDBRegistryInfoToViewNoUsername(t *testing.T) {
	h := &ImageHandler{}
	now := time.Now()
	view, err := h.cvtDBRegistryInfoToView(&model.RegistryInfo{
		ID: 1, Name: "reg", URL: "https://harbor.io", Default: true,
		CreatedAt: now, UpdatedAt: now,
	})
	assert.NoError(t, err)
	assert.Equal(t, int32(1), view.Id)
	assert.Equal(t, "reg", view.Name)
	assert.Empty(t, view.Username)
}

func TestListImagePullSecretsName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	dockerSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "docker-1", Namespace: "ns"},
		Type:       corev1.SecretTypeDockerConfigJson,
	}
	opaque := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "opaque-1", Namespace: "ns"},
		Type:       corev1.SecretTypeOpaque,
	}
	cl := ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(dockerSecret, opaque).Build()

	h := &ImageHandler{}
	names, err := h.listImagePullSecretsName(context.Background(), cl, "ns")
	assert.NoError(t, err)
	assert.Equal(t, []string{"docker-1"}, names)
}
