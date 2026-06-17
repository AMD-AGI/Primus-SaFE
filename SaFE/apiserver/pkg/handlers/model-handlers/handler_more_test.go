/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSetAndGetRobustClient(t *testing.T) {
	// Default/reset state is nil.
	SetRobustClient(nil)
	assert.Nil(t, GetRobustClient())
}

func TestNewHandlerConstructors(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	assert.NotNil(t, h)
	assert.False(t, h.IsDatasetEnabled())

	h2 := NewHandlerWithS3(nil, nil, nil, nil)
	assert.NotNil(t, h2)
	// s3 client is nil -> dataset disabled.
	assert.False(t, h2.IsDatasetEnabled())
}

func TestHandleWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Success path.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handle(c, func(*gin.Context) (interface{}, error) { return gin.H{"ok": true}, nil })
	assert.Equal(t, http.StatusOK, w.Code)

	// Error path -> 500 via getHTTPStatusCode.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handle(c2, func(*gin.Context) (interface{}, error) { return nil, errors.New("boom") })
	assert.Equal(t, http.StatusInternalServerError, w2.Code)
}

func TestHandleDatasetWrapper(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Success with struct response.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handleDataset(c, func(*gin.Context) (interface{}, error) { return gin.H{"ok": true}, nil })
	assert.Equal(t, http.StatusOK, w.Code)

	// Success with []byte response.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handleDataset(c2, func(*gin.Context) (interface{}, error) { return []byte(`{"a":1}`), nil })
	assert.Equal(t, http.StatusOK, w2.Code)

	// Error path.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	handleDataset(c3, func(*gin.Context) (interface{}, error) { return nil, errors.New("boom") })
	assert.NotEqual(t, http.StatusOK, w3.Code)
}
