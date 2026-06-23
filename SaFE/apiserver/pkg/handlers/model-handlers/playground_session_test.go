/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// sessCtx builds a gin context with body, user id, and params.
func sessCtx(t *testing.T, method, body, userId string, params gin.Params) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, "/", strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, "/", nil)
	}
	r.Header.Set("Content-Type", "application/json")
	c.Request = r
	if userId != "" {
		c.Set(common.UserId, userId)
	}
	c.Params = params
	return c
}

func TestSaveSessionCreate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().InsertPlaygroundSession(gomock.Any(), gomock.Any()).Return(nil)

	h := &Handler{dbClient: m}
	c := sessCtx(t, http.MethodPost, `{"modelName":"gpt","displayName":"d","messages":[]}`, "u1", nil)
	res, err := h.saveSession(c)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestSaveSessionUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetPlaygroundSession(gomock.Any(), int64(3)).
		Return(&dbclient.PlaygroundSession{Id: 3, UserId: "u1"}, nil)
	m.EXPECT().UpdatePlaygroundSession(gomock.Any(), gomock.Any()).Return(nil)

	h := &Handler{dbClient: m}
	c := sessCtx(t, http.MethodPost, `{"id":3,"modelName":"gpt","displayName":"d","messages":[]}`, "u1", nil)
	res, err := h.saveSession(c)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestSaveSessionMissingModelName(t *testing.T) {
	h := &Handler{dbClient: mock_client.NewMockInterface(gomock.NewController(t))}
	c := sessCtx(t, http.MethodPost, `{"messages":[]}`, "u1", nil)
	_, err := h.saveSession(c)
	assert.Error(t, err)
}

func TestListPlaygroundSessionHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().SelectPlaygroundSessions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.PlaygroundSession{{Id: 1, UserId: "u1", ModelName: "gpt"}}, nil)
	m.EXPECT().CountPlaygroundSessions(gomock.Any(), gomock.Any()).Return(1, nil)

	h := &Handler{dbClient: m}
	c := sessCtx(t, http.MethodGet, "", "u1", nil)
	res, err := h.listPlaygroundSession(c)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetPlaygroundSessionHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetPlaygroundSession(gomock.Any(), int64(1)).
		Return(&dbclient.PlaygroundSession{Id: 1, UserId: "u1"}, nil)

	h := &Handler{dbClient: m}
	c := sessCtx(t, http.MethodGet, "", "u1", gin.Params{{Key: "id", Value: "1"}})
	res, err := h.getPlaygroundSession(c)
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetPlaygroundSessionForbidden(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetPlaygroundSession(gomock.Any(), int64(1)).
		Return(&dbclient.PlaygroundSession{Id: 1, UserId: "other"}, nil)

	h := &Handler{dbClient: m}
	c := sessCtx(t, http.MethodGet, "", "u1", gin.Params{{Key: "id", Value: "1"}})
	_, err := h.getPlaygroundSession(c)
	assert.Error(t, err)
}

func TestDeletePlaygroundSessionHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetPlaygroundSession(gomock.Any(), int64(1)).
		Return(&dbclient.PlaygroundSession{Id: 1, UserId: "u1"}, nil)
	m.EXPECT().SetPlaygroundSessionDeleted(gomock.Any(), int64(1)).Return(nil)

	h := &Handler{dbClient: m}
	c := sessCtx(t, http.MethodDelete, "", "u1", gin.Params{{Key: "id", Value: "1"}})
	_, err := h.deletePlaygroundSession(c)
	assert.NoError(t, err)
}

func TestDeletePlaygroundSessionInvalidID(t *testing.T) {
	h := &Handler{dbClient: mock_client.NewMockInterface(gomock.NewController(t))}
	c := sessCtx(t, http.MethodDelete, "", "u1", gin.Params{{Key: "id", Value: "abc"}})
	_, err := h.deletePlaygroundSession(c)
	assert.Error(t, err)
}
