/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInferAction(t *testing.T) {
	tests := []struct {
		method   string
		expected string
	}{
		{"POST", "create"},
		{"DELETE", "delete"},
		{"PATCH", "update"},
		{"PUT", "replace"},
		{"GET", "get"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			result := inferAction(tt.method)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsInvalidTraceId(t *testing.T) {
	tests := []struct {
		traceId  string
		expected bool
	}{
		{"", true},
		{"00000000000000000000000000000000", true},
		{"0000", true},
		{"abc123def456", false},
		{"00000000000000000000000000000001", false},
	}

	for _, tt := range tests {
		t.Run(tt.traceId, func(t *testing.T) {
			result := isInvalidTraceId(tt.traceId)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeBody(t *testing.T) {
	// Note: sanitizeBody replaces the entire "field": "value" with "[REDACTED]"
	// It uses regex patterns: "password"\s*:\s*"[^"]*" -> "[REDACTED]"
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty_body",
			input:    "",
			expected: "",
		},
		{
			name:     "no_sensitive_data",
			input:    `{"name": "test", "value": 123}`,
			expected: `{"name": "test", "value": 123}`,
		},
		{
			name:     "password_field",
			input:    `{"username": "admin", "password": "secret123"}`,
			expected: `{"username": "admin", "[REDACTED]"}`,
		},
		{
			name:     "apiKey_field",
			input:    `{"name": "test", "apiKey": "ak-xxxxx"}`,
			expected: `{"name": "test", "[REDACTED]"}`,
		},
		{
			name:     "api_key_field",
			input:    `{"name": "test", "api_key": "ak-xxxxx"}`,
			expected: `{"name": "test", "[REDACTED]"}`,
		},
		{
			name:     "token_field",
			input:    `{"userId": "123", "token": "jwt-token-here"}`,
			expected: `{"userId": "123", "[REDACTED]"}`,
		},
		{
			name:     "secret_field",
			input:    `{"name": "mysecret", "secret": "super-secret"}`,
			expected: `{"name": "mysecret", "[REDACTED]"}`,
		},
		{
			name:     "multiple_sensitive_fields",
			input:    `{"password": "pass1", "token": "tok1", "apiKey": "key1"}`,
			expected: `{"[REDACTED]", "[REDACTED]", "[REDACTED]"}`,
		},
		{
			name:     "password_with_spaces",
			input:    `{"password" : "secret"}`,
			expected: `{"[REDACTED]"}`,
		},
		{
			name:     "case_sensitive_password_lowercase",
			input:    `{"password": "secret"}`,
			expected: `{"[REDACTED]"}`,
		},
		{
			name:     "case_sensitive_PASSWORD_uppercase_not_matched",
			input:    `{"PASSWORD": "secret"}`,
			expected: `{"PASSWORD": "secret"}`, // regex is case-sensitive
		},
		{
			name:     "form_data_not_matched",
			input:    `name=admin&password=secret123&type=default`,
			expected: `name=admin&password=secret123&type=default`, // only JSON format matched
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeBody(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short_string",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact_length",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "truncated",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...(truncated)",
		},
		{
			name:     "empty_string",
			input:    "",
			maxLen:   10,
			expected: "",
		},
		{
			name:     "zero_max_length",
			input:    "hello",
			maxLen:   0,
			expected: "...(truncated)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}
