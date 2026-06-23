/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// TestGetImportingDetailSuccess verifies layer details are returned.
func TestGetImportingDetailSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(3)).Return(&model.Image{ID: 3}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(3)).Return(&model.ImageImportJob{ID: 9, ImageID: 3}, nil)

	h := importJobHandler(t, m)
	res, err := h.getImportingDetail(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "3"}}))
	require.NoError(t, err)
	assert.NotNil(t, res)
}

// TestGetImportingLogsImportNotFound verifies missing import record yields not-found.
func TestGetImportingLogsImportNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImage(gomock.Any(), int32(3)).Return(&model.Image{ID: 3}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(3)).Return(nil, nil)

	h := importJobHandler(t, m)
	_, err := h.getImportingLogs(ginCtx(t, http.MethodGet, "", gin.Params{{Key: "id", Value: "3"}}))
	assert.Error(t, err)
}

// TestConvertOpsJobToPrewarmImageList verifies prewarm job conversion across all branches.
func TestConvertOpsJobToPrewarmImageList(t *testing.T) {
	m := mock_client.NewMockInterface(gomock.NewController(t))
	h := importJobHandler(t, m)

	jobs := []*dbclient.OpsJob{
		{
			JobId:      "p1",
			Phase:      sql.NullString{String: "Running", Valid: true},
			Inputs:     []byte("{image:img:tag,workspace:ws-1}"),
			Outputs:    sql.NullString{String: `[{"name":"status","value":"InProgress"},{"name":"prewarm_progress","value":"50"},{"name":"nodes_ready","value":"1"},{"name":"nodes_total","value":"2"}]`, Valid: true},
			Conditions: sql.NullString{String: `[{"type":"X","status":"True","message":"warn"}]`, Valid: true},
		},
	}
	result := h.convertOpsJobToPrewarmImageList(context.Background(), jobs)
	require.Len(t, result, 1)
	assert.Equal(t, "img:tag", result[0].ImageName)
	assert.Equal(t, "ws-1", result[0].WorkspaceId)
	assert.Equal(t, "InProgress", result[0].Status)
	assert.Equal(t, "50", result[0].PrewarmProgress)
	assert.Equal(t, "warn", result[0].ErrorMessage)
}
