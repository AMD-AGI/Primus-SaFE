/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCleanDatasetRepoID(t *testing.T) {
	assert.Equal(t, "owner/name", cleanDatasetRepoID("owner/name"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("https://huggingface.co/datasets/owner/name"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("http://huggingface.co/datasets/owner/name/"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("api/datasets/owner/name"))
	assert.Equal(t, "owner/name", cleanDatasetRepoID("  huggingface.co/owner/name  "))
}

func TestNormalizeHFDatasetURL(t *testing.T) {
	// Full URL returned as-is (trailing slash trimmed).
	assert.Equal(t, "https://huggingface.co/datasets/owner/name",
		normalizeHFDatasetURL("https://huggingface.co/datasets/owner/name"))
	// Repo ID constructed into full URL.
	assert.Equal(t, "https://huggingface.co/datasets/owner/name",
		normalizeHFDatasetURL("owner/name"))
}

func TestParseStringOrArray(t *testing.T) {
	assert.Nil(t, parseStringOrArray(nil))
	assert.Nil(t, parseStringOrArray(json.RawMessage(`""`)))
	assert.Equal(t, []string{"mit"}, parseStringOrArray(json.RawMessage(`"mit"`)))
	assert.Equal(t, []string{"a", "b"}, parseStringOrArray(json.RawMessage(`["a","b"]`)))
	assert.Nil(t, parseStringOrArray(json.RawMessage(`{bad`)))
}

func TestGetHTTPStatusCode(t *testing.T) {
	assert.Equal(t, http.StatusOK, getHTTPStatusCode(nil))
	assert.Equal(t, http.StatusInternalServerError, getHTTPStatusCode(errors.New("x")))
}

func TestSupportedModelNames(t *testing.T) {
	// Should return a comma-joined, non-empty list of recipe names.
	names := supportedModelNames()
	assert.NotEmpty(t, names)
}

func TestCategorizeTagString(t *testing.T) {
	assert.Empty(t, CategorizeTagString("", true))
	got := CategorizeTagString("llama-3, text-generation", true)
	assert.NotEmpty(t, got)
}

func TestCategorizeTags(t *testing.T) {
	got := CategorizeTags([]string{"llama-3", "text-generation"}, true)
	assert.NotEmpty(t, got)
	// Unmatched excluded in local mode may yield fewer entries but should not panic.
	_ = CategorizeTags([]string{"some-unknown-tag-xyz"}, false)
}
