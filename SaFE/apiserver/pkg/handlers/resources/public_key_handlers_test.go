/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

func TestCreatePublicKeyHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("db disabled returns error", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects()
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{}`)))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		h.CreatePublicKey(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("success", func(t *testing.T) {
		withDBEnabled(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		h, user := newAdminHandlerWithObjects()
		mockDB := mock_client.NewMockInterface(ctrl)
		h.dbClient = mockDB
		mockDB.EXPECT().InsertPublicKey(gomock.Any(), gomock.Any()).Return(nil)

		body, _ := json.Marshal(view.CreatePublicKeyRequest{Name: "k1", Description: "d", PublicKey: "ssh-rsa AAA"})
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set(common.UserId, user.Name)
		h.CreatePublicKey(c)
		assert.Equal(t, http.StatusOK, rsp.Code)
	})
}

func TestListPublicKeysHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("db disabled returns error", func(t *testing.T) {
		h, user := newAdminHandlerWithObjects()
		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(common.UserId, user.Name)
		h.ListPublicKeys(c)
		assert.NotEqual(t, http.StatusOK, rsp.Code)
	})

	t.Run("success", func(t *testing.T) {
		withDBEnabled(t)
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		h, user := newAdminHandlerWithObjects()
		mockDB := mock_client.NewMockInterface(ctrl)
		h.dbClient = mockDB
		mockDB.EXPECT().SelectPublicKeys(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]*dbclient.PublicKey{{Id: 1, UserId: user.Name, PublicKey: "ssh-rsa AAA", Status: true}}, nil)
		mockDB.EXPECT().CountPublicKeys(gomock.Any(), gomock.Any()).Return(1, nil)

		rsp := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rsp)
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Set(common.UserId, user.Name)
		h.ListPublicKeys(c)
		assert.Equal(t, http.StatusOK, rsp.Code)

		var resp view.ListPublicKeysResponse
		assert.NoError(t, json.Unmarshal(rsp.Body.Bytes(), &resp))
		assert.Equal(t, 1, resp.TotalCount)
	})
}

func TestDeletePublicKeyHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withDBEnabled(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user := newAdminHandlerWithObjects()
	mockDB := mock_client.NewMockInterface(ctrl)
	h.dbClient = mockDB
	mockDB.EXPECT().DeletePublicKey(gomock.Any(), user.Name, int64(5)).Return(nil)

	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodDelete, "/", nil)
	c.Params = gin.Params{{Key: "id", Value: "5"}}
	c.Set(common.UserId, user.Name)
	h.DeletePublicKey(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestSetPublicKeyStatusHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withDBEnabled(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user := newAdminHandlerWithObjects()
	mockDB := mock_client.NewMockInterface(ctrl)
	h.dbClient = mockDB
	mockDB.EXPECT().SetPublicKeyStatus(gomock.Any(), user.Name, int64(5), false).Return(nil)

	body, _ := json.Marshal(view.SetPublicKeyStatusRequest{Status: false})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "5"}}
	c.Set(common.UserId, user.Name)
	h.SetPublicKeyStatus(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}

func TestSetPublicKeyDescriptionHandlerSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	withDBEnabled(t)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	h, user := newAdminHandlerWithObjects()
	mockDB := mock_client.NewMockInterface(ctrl)
	h.dbClient = mockDB
	mockDB.EXPECT().SetPublicKeyDescription(gomock.Any(), user.Name, int64(5), "new-desc").Return(nil)

	body, _ := json.Marshal(view.SetPublicKeyDescriptionRequest{Description: "new-desc"})
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodPut, "/", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "5"}}
	c.Set(common.UserId, user.Name)
	h.SetPublicKeyDescription(c)
	assert.Equal(t, http.StatusOK, rsp.Code)
}
