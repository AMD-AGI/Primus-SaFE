/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestInitImageRouter verifies all image routes are registered without panicking.
func TestInitImageRouter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	e := gin.New()
	h := &ImageHandler{}
	InitImageRouter(e, h)
	assert.NotEmpty(t, e.Routes())
}

// TestGetPrewarmNodesEmptyName verifies the empty-name branch returns an error.
func TestGetPrewarmNodesEmptyName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	h := &ImageHandler{}
	_, err := h.getPrewarmNodes(c)
	assert.Error(t, err)
}
