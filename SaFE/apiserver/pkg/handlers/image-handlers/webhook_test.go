/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func progressCtx(t *testing.T, name, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
	} else {
		r = httptest.NewRequest(http.MethodPut, "/", nil)
	}
	r.Header.Set("Content-Type", "application/json")
	c.Request = r
	c.Params = gin.Params{{Key: "name", Value: name}}
	return c, w
}

// TestUpdateImportProgressBadName verifies invalid base64 names are rejected.
func TestUpdateImportProgressBadName(t *testing.T) {
	h := &ImageHandler{}
	c, _ := progressCtx(t, "!!!not-base64!!!", "")
	_, err := h.updateImportProgress(c)
	assert.Error(t, err)
}

// TestUpdateImportProgressImageNotFound verifies a missing image yields a not-found error.
func TestUpdateImportProgressImageNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), "reg/app:tag").Return(nil, nil)

	h := &ImageHandler{dbClient: m}
	name := base64.URLEncoding.EncodeToString([]byte("reg/app:tag"))
	c, _ := progressCtx(t, name, "")
	_, err := h.updateImportProgress(c)
	assert.Error(t, err)
}

// TestUpdateImportProgressImportNotFound verifies a missing import record yields an error.
func TestUpdateImportProgressImportNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), "reg/app:tag").Return(&model.Image{ID: 1}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).Return(nil, nil)

	h := &ImageHandler{dbClient: m}
	name := base64.URLEncoding.EncodeToString([]byte("reg/app:tag"))
	c, _ := progressCtx(t, name, "")
	_, err := h.updateImportProgress(c)
	assert.Error(t, err)
}

// TestUpdateImportProgressSuccess verifies the happy path updates the import job.
func TestUpdateImportProgressSuccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetImageByTag(gomock.Any(), "reg/app:tag").Return(&model.Image{ID: 1}, nil)
	m.EXPECT().GetImportImageByImageID(gomock.Any(), int32(1)).Return(&model.ImageImportJob{ID: 5, ImageID: 1}, nil)
	m.EXPECT().UpdateImageImportJob(gomock.Any(), gomock.Any()).Return(nil)

	h := &ImageHandler{dbClient: m}
	name := base64.URLEncoding.EncodeToString([]byte("reg/app:tag"))
	c, _ := progressCtx(t, name, `{}`)
	_, err := h.updateImportProgress(c)
	assert.NoError(t, err)
}
