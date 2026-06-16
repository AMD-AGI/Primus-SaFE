/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"database/sql"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// TestListExportedImage verifies export jobs are listed and converted.
func TestListExportedImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.OpsJob{{JobId: "j1", Phase: sql.NullString{String: "Succeeded", Valid: true}}}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(1, nil)

	h := importJobHandler(t, m)
	res, err := h.listExportedImage(ginCtx(t, http.MethodGet, "", nil))
	require.NoError(t, err)
	assert.NotNil(t, res)
}

// TestListPrewarmImage verifies prewarm jobs are listed.
func TestListPrewarmImage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectJobs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.OpsJob{{JobId: "p1"}}, nil)
	m.EXPECT().CountJobs(gomock.Any(), gomock.Any()).Return(1, nil)

	h := importJobHandler(t, m)
	res, err := h.listPrewarmImage(ginCtx(t, http.MethodGet, "", nil))
	require.NoError(t, err)
	assert.NotNil(t, res)
}

// TestDeleteExportedImageEmptyID verifies the empty-id branch.
func TestDeleteExportedImageEmptyID(t *testing.T) {
	h := importJobHandler(t, mock_client.NewMockInterface(gomock.NewController(t)))
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", nil))
	assert.Error(t, err)
}

// TestDeleteExportedImageNotFound verifies a missing job yields not-found.
func TestDeleteExportedImageNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "j1").Return(nil, nil)

	h := importJobHandler(t, m)
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "j1"}}))
	assert.Error(t, err)
}

// TestDeleteExportedImageAlreadyDeleted verifies the already-deleted branch.
func TestDeleteExportedImageAlreadyDeleted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "j1").Return(&dbclient.OpsJob{JobId: "j1", IsDeleted: true}, nil)

	h := importJobHandler(t, m)
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "j1"}}))
	assert.Error(t, err)
}

// TestDeleteExportedImageSuccess verifies soft delete succeeds (no image name -> no harbor call).
func TestDeleteExportedImageSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetOpsJob(gomock.Any(), "j1").Return(&dbclient.OpsJob{JobId: "j1"}, nil)
	m.EXPECT().SetOpsJobDeleted(gomock.Any(), "j1").Return(nil)

	h := importJobHandler(t, m)
	_, err := h.deleteExportedImage(ginCtx(t, http.MethodDelete, "", gin.Params{{Key: "jobId", Value: "j1"}}))
	assert.NoError(t, err)
}
