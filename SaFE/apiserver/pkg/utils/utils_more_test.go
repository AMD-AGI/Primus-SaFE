/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func TestPrimusApiErrorError(t *testing.T) {
	e := &PrimusApiError{ErrorMessage: "boom"}
	assert.Equal(t, "boom", e.Error())
}

func TestReadBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`))
	data, err := ReadBody(req)
	assert.NoError(t, err)
	assert.Equal(t, `{"a":1}`, string(data))
}

func TestReadBodyTooLarge(t *testing.T) {
	big := strings.Repeat("x", int(DefaultMaxRequestBodyBytes)+10)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(big))
	_, err := ReadBody(req)
	assert.Error(t, err)
}

func TestParseRequestBody(t *testing.T) {
	var dst struct {
		A int `json:"a"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":5}`))
	_, err := ParseRequestBody(req, &dst)
	assert.NoError(t, err)
	assert.Equal(t, 5, dst.A)
}

func TestParseRequestBodyEmpty(t *testing.T) {
	var dst struct{}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
	body, err := ParseRequestBody(req, &dst)
	assert.NoError(t, err)
	assert.Nil(t, body)
}

func TestParseRequestBodyInvalidJSON(t *testing.T) {
	var dst struct {
		A int `json:"a"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{invalid"))
	_, err := ParseRequestBody(req, &dst)
	assert.Error(t, err)
}

func TestGetK8sClientFactoryNotFound(t *testing.T) {
	mgr := commonutils.NewObjectManagerSingleton()
	_, err := GetK8sClientFactory(mgr, "nonexistent-cluster")
	assert.Error(t, err)
}
