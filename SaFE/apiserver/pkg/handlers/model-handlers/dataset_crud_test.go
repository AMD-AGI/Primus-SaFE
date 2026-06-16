/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestListDatasetTypes(t *testing.T) {
	h := &Handler{}
	res, err := h.listDatasetTypes(modelGinCtx(t, nil, ""))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestListDatasetsHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().CountDatasets(gomock.Any(), gomock.Any()).Return(1, nil)
	m.EXPECT().SelectDatasets(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Dataset{{DatasetId: "ds-1", DisplayName: "D"}}, nil)

	h := &Handler{dbClient: m}
	res, err := h.listDatasets(modelGinCtx(t, nil, ""))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetDatasetHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDataset(gomock.Any(), "ds-1").
		Return(&dbclient.Dataset{DatasetId: "ds-1", DisplayName: "D", S3Path: "p/"}, nil)

	// s3Client nil -> file listing skipped.
	h := &Handler{dbClient: m}
	res, err := h.getDataset(modelGinCtx(t, gin.Params{{Key: "id", Value: "ds-1"}}, ""))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetDatasetHandlerBadID(t *testing.T) {
	h := &Handler{}
	_, err := h.getDataset(modelGinCtx(t, nil, ""))
	assert.Error(t, err)
}

func TestDeleteDatasetHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDataset(gomock.Any(), "ds-1").
		Return(&dbclient.Dataset{DatasetId: "ds-1", UserId: "u1"}, nil)
	m.EXPECT().SetDatasetDeleted(gomock.Any(), "ds-1").Return(nil)

	h := &Handler{dbClient: m}
	res, err := h.deleteDataset(modelGinCtx(t, gin.Params{{Key: "id", Value: "ds-1"}}, ""))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestDeleteDatasetHandlerSystemProtected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetDataset(gomock.Any(), "ds-1").
		Return(&dbclient.Dataset{DatasetId: "ds-1", UserId: common.UserSystem}, nil)

	h := &Handler{dbClient: m}
	_, err := h.deleteDataset(modelGinCtx(t, gin.Params{{Key: "id", Value: "ds-1"}}, ""))
	assert.Error(t, err)
}

func TestDeleteDatasetHandlerBadID(t *testing.T) {
	h := &Handler{}
	_, err := h.deleteDataset(modelGinCtx(t, nil, ""))
	assert.Error(t, err)
}
