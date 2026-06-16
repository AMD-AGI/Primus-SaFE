/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// TestGetImportImageInfoNoDefaultRegistry verifies missing default registry yields bad request.
func TestGetImportImageInfoNoDefaultRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(nil, nil)

	h := &ImageHandler{dbClient: m}
	_, err := h.getImportImageInfo(context.Background(),
		&ImportImageServiceRequest{Source: "docker.io/library/alpine:latest"}, nil)
	assert.Error(t, err)
}

// TestGetImportImageInfoDBError verifies a registry lookup error is surfaced.
func TestGetImportImageInfoDBError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(nil, errors.New("db down"))

	h := &ImageHandler{dbClient: m}
	_, err := h.getImportImageInfo(context.Background(),
		&ImportImageServiceRequest{Source: "docker.io/library/alpine:latest"}, nil)
	assert.Error(t, err)
}

// TestGetImportImageInfoBadSource verifies an invalid source image name is rejected.
func TestGetImportImageInfoBadSource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(&model.RegistryInfo{URL: "harbor.io"}, nil)

	h := &ImageHandler{dbClient: m}
	_, err := h.getImportImageInfo(context.Background(),
		&ImportImageServiceRequest{Source: "noslash"}, nil)
	assert.Error(t, err)
}

// TestImportImageBadBody verifies invalid JSON is rejected at the entry point.
func TestImportImageBadBody(t *testing.T) {
	h := importJobHandler(t, mock_client.NewMockInterface(gomock.NewController(t)))
	_, err := h.importImage(ginCtx(t, http.MethodPost, "{bad", nil))
	assert.Error(t, err)
}

// TestImportImageNoDefaultRegistry drives the entry point through authorization into
// getImportImageInfo, which fails fast when no default push registry is configured.
func TestImportImageNoDefaultRegistry(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDefaultRegistryInfo(gomock.Any()).Return(nil, nil)

	h := importJobHandler(t, m)
	_, err := h.importImage(ginCtx(t, http.MethodPost,
		`{"source":"docker.io/library/alpine:latest"}`, nil))
	assert.Error(t, err)
}
