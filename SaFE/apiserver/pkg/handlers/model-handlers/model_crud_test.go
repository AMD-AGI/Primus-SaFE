/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	mock_client "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/mock"
)

// modelScheme returns a scheme with the project API types registered.
func modelScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	if err := v1.AddToScheme(s); err != nil {
		t.Fatal(err)
	}
	return s
}

// modelGinCtx builds a gin context with optional params and query.
func modelGinCtx(t *testing.T, params gin.Params, rawQuery string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	r := httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil)
	c.Request = r
	c.Params = params
	return c
}

func TestGetModelFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().GetModelByID(gomock.Any(), "m1").
		Return(&dbclient.Model{ID: "m1", AccessMode: "local", DisplayName: "My Model"}, nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	res, err := h.getModel(modelGinCtx(t, gin.Params{{Key: "id", Value: "m1"}}, ""))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestGetModelEmptyID(t *testing.T) {
	h := &Handler{k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	_, err := h.getModel(modelGinCtx(t, nil, ""))
	assert.Error(t, err)
}

func TestGetModelNotFound(t *testing.T) {
	// No db client, empty k8s -> not found.
	h := &Handler{k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	_, err := h.getModel(modelGinCtx(t, gin.Params{{Key: "id", Value: "missing"}}, ""))
	assert.Error(t, err)
}

func TestListModelsFromDB(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Model{
			{ID: "m1", AccessMode: "local", DisplayName: "A"},
			{ID: "m2", AccessMode: "local", DisplayName: "B"},
		}, nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	res, err := h.listModels(modelGinCtx(t, nil, ""))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}

func TestListModelsEmptyDBFallsBackToK8s(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := mock_client.NewMockInterface(ctrl)
	m.EXPECT().ListModels(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*dbclient.Model{}, nil)

	h := &Handler{dbClient: m, k8sClient: ctrlfake.NewClientBuilder().WithScheme(modelScheme(t)).Build()}
	res, err := h.listModels(modelGinCtx(t, nil, ""))
	assert.NoError(t, err)
	assert.NotNil(t, res)
}
