/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateModelBadBody(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost, "{invalid", "u1", nil))
	assert.Error(t, err)
}

func TestCreateModelURLRequired(t *testing.T) {
	h := &Handler{}
	// local mode without url.
	_, err := h.createModel(sessCtx(t, http.MethodPost, `{"source":{"accessMode":"local"}}`, "u1", nil))
	assert.Error(t, err)
}

func TestCreateModelInvalidAccessMode(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost, `{"source":{"accessMode":"bad","url":"http://x"}}`, "u1", nil))
	assert.Error(t, err)
}

func TestCreateModelRemoteMissingModelName(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"source":{"accessMode":"remote_api","url":"http://x"}}`, "u1", nil))
	assert.Error(t, err)
}

func TestCreateModelRemoteMissingDisplayName(t *testing.T) {
	h := &Handler{}
	_, err := h.createModel(sessCtx(t, http.MethodPost,
		`{"source":{"accessMode":"remote_api","url":"http://x","modelName":"gpt"}}`, "u1", nil))
	assert.Error(t, err)
}
