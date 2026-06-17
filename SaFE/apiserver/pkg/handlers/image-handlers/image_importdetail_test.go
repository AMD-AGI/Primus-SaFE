/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestGetImportingDetailHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(&model.Image{ID: 1}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).
		Return(&model.ImageImportJob{ID: 1, ImageID: 1}, nil)

	h := registryTestHandler(t, m)
	res, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetImportingDetailImageNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(nil, nil)

	h := registryTestHandler(t, m)
	_, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	assert.Error(t, err)
}

func TestGetImportingDetailImportNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(&model.Image{ID: 1}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).Return(nil, nil)

	h := registryTestHandler(t, m)
	_, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	assert.Error(t, err)
}

func TestGetImportingDetailBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	_, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "x"}}))
	assert.Error(t, err)
}

func TestGetImportingLogsDBFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(1)).Return(&model.Image{ID: 1}, nil)
	// JobName empty -> returns DB log without touching OpenSearch.
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).
		Return(&model.ImageImportJob{ID: 1, ImageID: 1, Log: "log line", Os: "linux", Arch: "amd64"}, nil)

	h := registryTestHandler(t, m)
	res, err := h.getImportingLogs(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "1"}}))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetImportingLogsBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	_, err := h.getImportingLogs(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "x"}}))
	assert.Error(t, err)
}
