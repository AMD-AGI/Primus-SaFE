/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package emailrelayhandlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func relayCtx(t *testing.T, method, body string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
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
	c.Params = params
	return c, w
}

func TestAckBadID(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, "", gin.Params{{Key: "id", Value: "abc"}})
	h.Ack(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestFailBadID(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, "", gin.Params{{Key: "id", Value: "abc"}})
	h.Fail(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitBadBody(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, "{invalid", nil)
	h.Submit(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSubmitMissingFields(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodPost, `{"subject":"s"}`, nil)
	h.Submit(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthorizeRelayNoAuth(t *testing.T) {
	h := &Handler{}
	c, w := relayCtx(t, http.MethodGet, "", nil)
	h.AuthorizeRelay()(c)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.True(t, c.IsAborted())
}
