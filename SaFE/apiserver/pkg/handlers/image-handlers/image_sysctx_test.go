/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// TestExistImageValidFound verifies an existing image returns its id.
func TestExistImageValidFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), "reg/app:tag").
		Return(&model.Image{ID: 7, Tag: "reg/app:tag"}, nil)

	h := &ImageHandler{dbClient: m}
	id, err := h.existImageValid(context.Background(), "reg/app:tag")
	require.NoError(t, err)
	assert.Equal(t, int32(7), id)
}

// TestExistImageValidNotFound verifies a nil image returns zero id.
func TestExistImageValidNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), "reg/app:tag").Return(nil, nil)

	h := &ImageHandler{dbClient: m}
	id, err := h.existImageValid(context.Background(), "reg/app:tag")
	require.NoError(t, err)
	assert.Equal(t, int32(0), id)
}

// TestExistImageValidDBError verifies a db error is surfaced.
func TestExistImageValidDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), gomock.Any()).Return(nil, errors.New("db down"))

	h := &ImageHandler{dbClient: m}
	_, err := h.existImageValid(context.Background(), "reg/app:tag")
	assert.Error(t, err)
}

// TestGetImageSystemCtxUserSecret verifies a matching user secret short-circuits auth resolution.
func TestGetImageSystemCtxUserSecret(t *testing.T) {
	h := &ImageHandler{}
	userSecret := &corev1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"harbor.example.com":{"username":"u","password":"p"}}}`),
		},
	}
	sysCtx, err := h.getImageSystemCtx(context.Background(), "harbor.example.com", "harbor.example.com/p/i:t", userSecret)
	require.NoError(t, err)
	require.NotNil(t, sysCtx)
	require.NotNil(t, sysCtx.DockerAuthConfig)
	assert.Equal(t, "u", sysCtx.DockerAuthConfig.Username)
}

// TestGetImageSystemCtxFromDB verifies registry info from the database is used when no user secret is given.
func TestGetImageSystemCtxFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// Empty credentials avoid crypto dependency (decrypt is skipped for empty fields).
	m.EXPECT().GetRegistryInfoByUrl(gomock.Any(), "reg.example.com").
		Return(&model.RegistryInfo{URL: "reg.example.com"}, nil)

	h := &ImageHandler{dbClient: m}
	sysCtx, err := h.getImageSystemCtx(context.Background(), "reg.example.com", "reg.example.com/p/i:t", nil)
	require.NoError(t, err)
	require.NotNil(t, sysCtx)
}
