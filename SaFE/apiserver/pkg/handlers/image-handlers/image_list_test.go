/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	dbClient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestCreateImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().UpsertImage(gomock.Any(), gomock.Any()).Return(nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodPost, `{"imageTag":"nginx:latest","description":"d"}`, nil)
	_, err := h.createImage(c)
	assert.NoError(t, err)
}

func TestCreateImageHandlerInvalid(t *testing.T) {
	h := registryTestHandler(t, nil)
	c := ginCtx(t, http.MethodPost, `{"description":"no tag"}`, nil)
	_, err := h.createImage(c)
	assert.Error(t, err)
}

func TestDeleteImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(&model.Image{ID: 7}, nil)
	m.EXPECT().DeleteImage(gomock.Any(), int32(7), gomock.Any()).Return(nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	_, err := h.deleteImage(c)
	assert.NoError(t, err)
}

func TestDeleteImageHandlerNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(7)).Return(nil, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "7"}})
	_, err := h.deleteImage(c)
	assert.NoError(t, err)
}

func TestDeleteImageHandlerBadID(t *testing.T) {
	h := registryTestHandler(t, nil)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "id", Value: "x"}})
	_, err := h.deleteImage(c)
	assert.Error(t, err)
}

func TestListImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectImages(gomock.Any(), gomock.Any()).Return(
		[]*model.Image{{ID: 1, Tag: "harbor.io/proj/app:v1", CreatedAt: time.Now()}}, 1, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodGet, "", nil)
	res, err := h.listImage(c)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestListExportedImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbClient.OpsJob{}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(0, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodGet, "", nil)
	res, err := h.listExportedImage(c)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestListPrewarmImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbClient.OpsJob{}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(0, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodGet, "", nil)
	res, err := h.listPrewarmImage(c)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestDeleteExportedImageHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	// Job has no outputs -> no harbor deletion attempted.
	m.EXPECT().GetOpsJob(gomock.Any(), "job-1").Return(&dbClient.OpsJob{JobId: "job-1"}, nil)
	m.EXPECT().SetOpsJobDeleted(gomock.Any(), "job-1").Return(nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "job-1"}})
	_, err := h.deleteExportedImage(c)
	assert.NoError(t, err)
}

func TestDeleteExportedImageHandlerNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "job-1").Return(nil, nil)

	h := registryTestHandler(t, m)
	c := ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "job-1"}})
	_, err := h.deleteExportedImage(c)
	assert.Error(t, err)
}
