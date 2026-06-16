/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"strings"
	"testing"
)

// TestTagsToDB verifies tag slice serialization to a comma-joined string.
func TestTagsToDB(t *testing.T) {
	if tagsToDB(nil) != "" {
		t.Error("expected empty string for nil tags")
	}
	if got := tagsToDB([]string{"a", "b", "c"}); got != "a,b,c" {
		t.Errorf("unexpected joined tags: %s", got)
	}
}

// TestExtractTarget verifies target normalization and subpath validation.
func TestExtractTarget(t *testing.T) {
	vol, sub, err := extractTarget(nil)
	if err != nil || vol != "" || sub != "" {
		t.Errorf("nil target should return empty values, got vol=%q sub=%q err=%v", vol, sub, err)
	}

	vol, sub, err = extractTarget(&ModelTargetReq{Volume: " data ", Subpath: "/models/foo/"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vol != "data" || sub != "models/foo" {
		t.Errorf("unexpected normalization: vol=%q sub=%q", vol, sub)
	}

	if _, _, err := extractTarget(&ModelTargetReq{Subpath: "../etc"}); err == nil {
		t.Error("expected error for path-traversal subpath")
	}
}

// TestIsSafeSubpath verifies allowed and rejected subpaths.
func TestIsSafeSubpath(t *testing.T) {
	if !isSafeSubpath("") {
		t.Error("empty subpath should be safe")
	}
	if !isSafeSubpath("models/foo-bar_1.2/baz") {
		t.Error("valid subpath should be safe")
	}
	if isSafeSubpath("foo/../bar") {
		t.Error("path traversal should be rejected")
	}
	if isSafeSubpath("foo bar") {
		t.Error("space should be rejected")
	}
}

// TestIsSafeS3URI verifies allowed and rejected S3 URIs.
func TestIsSafeS3URI(t *testing.T) {
	if isSafeS3URI("") {
		t.Error("empty S3 URI should be unsafe")
	}
	if !isSafeS3URI("s3://bucket/key-1_2.bin") {
		t.Error("valid S3 URI should be safe")
	}
	if isSafeS3URI("s3://bucket/$(rm -rf)") {
		t.Error("shell metacharacters should be rejected")
	}
}

// TestIsSafeURL verifies allowed and rejected endpoint URLs.
func TestIsSafeURL(t *testing.T) {
	if !isSafeURL("") {
		t.Error("empty URL should be considered safe (optional)")
	}
	if !isSafeURL("https://s3.example.com:9000") {
		t.Error("valid URL should be safe")
	}
	if isSafeURL("https://x?a=b") {
		t.Error("query characters should be rejected")
	}
}

// TestModelNameSortKey verifies the sort key strips prefix and lowercases.
func TestModelNameSortKey(t *testing.T) {
	if got := modelNameSortKey("Qwen/Qwen3-8B"); got != "qwen3-8b" {
		t.Errorf("unexpected sort key: %s", got)
	}
	if got := modelNameSortKey("  Plain  "); got != "plain" {
		t.Errorf("unexpected sort key: %s", got)
	}
}

// TestMatchModelOrigin verifies origin matching semantics.
func TestMatchModelOrigin(t *testing.T) {
	if !matchModelOrigin("custom", "custom") {
		t.Error("custom origin should match custom query")
	}
	if matchModelOrigin("external", "custom") {
		t.Error("external origin should not match custom query")
	}
	if !matchModelOrigin("external", "external") {
		t.Error("exact origin should match")
	}
}

// TestEnrichInferenceXInfo verifies InferenceX availability is marked by display name.
func TestEnrichInferenceXInfo(t *testing.T) {
	items := []ModelInfo{
		{DisplayName: "deepseek-ai/DeepSeek-R1-0528"},
		{DisplayName: "unknown-org/Unknown-Model"},
	}
	enrichInferenceXInfo(items)

	if !items[0].HasInferenceX || items[0].InferenceXModel != "DeepSeek-R1-0528" {
		t.Errorf("expected first model to be marked InferenceX, got %+v", items[0])
	}
	if items[1].HasInferenceX {
		t.Error("unknown model should not be marked InferenceX")
	}
}

// TestSanitizeLabelValue verifies invalid chars are replaced and length is bounded.
func TestSanitizeLabelValue(t *testing.T) {
	if sanitizeLabelValue("") != "" {
		t.Error("empty stays empty")
	}
	if got := sanitizeLabelValue("Qwen/Qwen3-8B"); got != "Qwen_Qwen3-8B" {
		t.Errorf("unexpected sanitized label: %s", got)
	}
	long := strings.Repeat("a", 100)
	if len(sanitizeLabelValue(long)) > 63 {
		t.Error("label value must be bounded to 63 chars")
	}
}