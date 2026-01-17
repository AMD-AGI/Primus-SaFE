/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractResourceInfo(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		expectedType string
		expectedName string
		description  string
	}{
		// Basic resource routes
		{
			name:         "simple_resource_list",
			path:         "/api/v1/workloads",
			expectedType: "workloads",
			expectedName: "",
			description:  "POST /api/v1/workloads - create workload",
		},
		{
			name:         "resource_with_name",
			path:         "/api/v1/workloads/my-job",
			expectedType: "workloads",
			expectedName: "my-job",
			description:  "GET/DELETE /api/v1/workloads/:name",
		},
		{
			name:         "secrets_with_name",
			path:         "/api/v1/secrets/my-secret",
			expectedType: "secrets",
			expectedName: "my-secret",
			description:  "DELETE /api/v1/secrets/:name",
		},

		// Batch operation routes (operation keyword should be skipped)
		{
			name:         "batch_delete_workloads",
			path:         "/api/v1/workloads/delete",
			expectedType: "workloads",
			expectedName: "",
			description:  "POST /api/v1/workloads/delete - batch delete",
		},
		{
			name:         "batch_stop_workloads",
			path:         "/api/v1/workloads/stop",
			expectedType: "workloads",
			expectedName: "",
			description:  "POST /api/v1/workloads/stop - batch stop",
		},
		{
			name:         "batch_clone_workloads",
			path:         "/api/v1/workloads/clone",
			expectedType: "workloads",
			expectedName: "",
			description:  "POST /api/v1/workloads/clone - batch clone",
		},
		{
			name:         "batch_delete_nodes",
			path:         "/api/v1/nodes/delete",
			expectedType: "nodes",
			expectedName: "",
			description:  "POST /api/v1/nodes/delete - batch delete nodes",
		},
		{
			name:         "batch_retry_nodes",
			path:         "/api/v1/nodes/retry",
			expectedType: "nodes",
			expectedName: "",
			description:  "POST /api/v1/nodes/retry - batch retry nodes",
		},

		// Single resource operation routes
		{
			name:         "single_workload_stop",
			path:         "/api/v1/workloads/my-job/stop",
			expectedType: "workloads",
			expectedName: "my-job",
			description:  "POST /api/v1/workloads/:name/stop",
		},
		{
			name:         "single_fault_stop",
			path:         "/api/v1/faults/fault-123/stop",
			expectedType: "faults",
			expectedName: "fault-123",
			description:  "POST /api/v1/faults/:name/stop",
		},
		{
			name:         "single_opsjob_stop",
			path:         "/api/v1/opsjobs/job-456/stop",
			expectedType: "opsjobs",
			expectedName: "job-456",
			description:  "POST /api/v1/opsjobs/:name/stop",
		},

		// Routes with numeric ID
		{
			name:         "publickey_with_id",
			path:         "/api/v1/publickeys/123",
			expectedType: "publickeys",
			expectedName: "123",
			description:  "DELETE /api/v1/publickeys/:id",
		},
		{
			name:         "publickey_status",
			path:         "/api/v1/publickeys/123/status",
			expectedType: "publickeys",
			expectedName: "123",
			description:  "PATCH /api/v1/publickeys/:id/status",
		},
		{
			name:         "publickey_description",
			path:         "/api/v1/publickeys/456/description",
			expectedType: "publickeys",
			expectedName: "456",
			description:  "PATCH /api/v1/publickeys/:id/description",
		},
		{
			name:         "apikey_with_id",
			path:         "/api/v1/apikeys/789",
			expectedType: "apikeys",
			expectedName: "789",
			description:  "DELETE /api/v1/apikeys/:id",
		},

		// CD module routes (with module prefix)
		{
			name:         "cd_deployments_list",
			path:         "/api/v1/cd/deployments",
			expectedType: "deployments",
			expectedName: "",
			description:  "GET/POST /api/v1/cd/deployments",
		},
		{
			name:         "cd_deployment_by_id",
			path:         "/api/v1/cd/deployments/33",
			expectedType: "deployments",
			expectedName: "33",
			description:  "GET /api/v1/cd/deployments/:id",
		},
		{
			name:         "cd_deployment_approve",
			path:         "/api/v1/cd/deployments/33/approve",
			expectedType: "deployments",
			expectedName: "33",
			description:  "POST /api/v1/cd/deployments/:id/approve",
		},
		{
			name:         "cd_deployment_rollback",
			path:         "/api/v1/cd/deployments/10/rollback",
			expectedType: "deployments",
			expectedName: "10",
			description:  "POST /api/v1/cd/deployments/:id/rollback",
		},
		{
			name:         "cd_env_config",
			path:         "/api/v1/cd/env-config",
			expectedType: "env-config",
			expectedName: "",
			description:  "GET /api/v1/cd/env-config",
		},

		// Nested resource routes
		{
			name:         "cluster_addons_list",
			path:         "/api/v1/clusters/my-cluster/addons",
			expectedType: "clusters",
			expectedName: "my-cluster",
			description:  "GET/POST /api/v1/clusters/:name/addons",
		},
		{
			name:         "cluster_addon_specific",
			path:         "/api/v1/clusters/my-cluster/addons/my-addon",
			expectedType: "clusters",
			expectedName: "my-cluster",
			description:  "DELETE /api/v1/clusters/:name/addons/:addon",
		},
		{
			name:         "workspace_nodes",
			path:         "/api/v1/workspaces/ws-001/nodes",
			expectedType: "workspaces",
			expectedName: "ws-001",
			description:  "POST /api/v1/workspaces/:name/nodes",
		},
		{
			name:         "cluster_nodes",
			path:         "/api/v1/clusters/cluster-001/nodes",
			expectedType: "clusters",
			expectedName: "cluster-001",
			description:  "POST /api/v1/clusters/:name/nodes",
		},

		// Log routes
		{
			name:         "node_logs",
			path:         "/api/v1/nodes/node-1/logs",
			expectedType: "nodes",
			expectedName: "node-1",
			description:  "GET /api/v1/nodes/:name/logs",
		},
		{
			name:         "cluster_logs",
			path:         "/api/v1/clusters/cluster-1/logs",
			expectedType: "clusters",
			expectedName: "cluster-1",
			description:  "GET /api/v1/clusters/:name/logs",
		},
		{
			name:         "workload_pod_logs",
			path:         "/api/v1/workloads/job-1/pods/pod-abc/logs",
			expectedType: "workloads",
			expectedName: "job-1",
			description:  "GET /api/v1/workloads/:name/pods/:podId/logs",
		},

		// Export routes
		{
			name:         "nodes_export",
			path:         "/api/v1/nodes/export",
			expectedType: "nodes",
			expectedName: "",
			description:  "GET /api/v1/nodes/export",
		},

		// Login/Logout routes
		{
			name:         "login",
			path:         "/api/v1/login",
			expectedType: "login",
			expectedName: "",
			description:  "POST /api/v1/login",
		},
		{
			name:         "logout",
			path:         "/api/v1/logout",
			expectedType: "logout",
			expectedName: "",
			description:  "POST /api/v1/logout",
		},

		// Auth verify route
		{
			name:         "auth_verify",
			path:         "/api/v1/auth/verify",
			expectedType: "auth",
			expectedName: "",
			description:  "POST /api/v1/auth/verify - verify is an operation keyword, skipped",
		},

		// Edge cases
		{
			name:         "empty_path",
			path:         "",
			expectedType: "",
			expectedName: "",
			description:  "empty path",
		},
		{
			name:         "root_path",
			path:         "/",
			expectedType: "",
			expectedName: "",
			description:  "root path only",
		},
		{
			name:         "api_only",
			path:         "/api",
			expectedType: "",
			expectedName: "",
			description:  "api prefix only",
		},
		{
			name:         "api_v1_only",
			path:         "/api/v1",
			expectedType: "",
			expectedName: "",
			description:  "api/v1 prefix only",
		},
		{
			name:         "api_v2_resource",
			path:         "/api/v2/workloads/test",
			expectedType: "workloads",
			expectedName: "test",
			description:  "v2 API version support",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resourceType, resourceName := extractResourceInfo(tt.path)
			assert.Equal(t, tt.expectedType, resourceType, "resourceType mismatch for: %s", tt.description)
			assert.Equal(t, tt.expectedName, resourceName, "resourceName mismatch for: %s", tt.description)
		})
	}
}

func TestIsOperationKeyword(t *testing.T) {
	tests := []struct {
		keyword  string
		expected bool
	}{
		// True cases - operation keywords
		{"delete", true},
		{"DELETE", true},
		{"Delete", true},
		{"stop", true},
		{"clone", true},
		{"retry", true},
		{"logs", true},
		{"export", true},
		{"verify", true},
		{"status", true},
		{"approve", true},
		{"rollback", true},
		{"description", true},

		// False cases - not operation keywords (resource names)
		{"my-workload", false},
		{"node-123", false},
		{"123", false},
		{"my-secret", false},
		{"cluster-001", false},
		{"addon-name", false},
		{"env-config", false},
		{"components", false},
		{"deployments", false},
		{"workloads", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.keyword, func(t *testing.T) {
			result := isOperationKeyword(tt.keyword)
			assert.Equal(t, tt.expected, result, "isOperationKeyword(%q) should be %v", tt.keyword, tt.expected)
		})
	}
}

func TestIsModulePrefix(t *testing.T) {
	tests := []struct {
		prefix   string
		expected bool
	}{
		// True cases - module prefixes
		{"cd", true},
		{"CD", true},
		{"Cd", true},

		// False cases - not module prefixes
		{"workloads", false},
		{"api", false},
		{"v1", false},
		{"nodes", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			result := isModulePrefix(tt.prefix)
			assert.Equal(t, tt.expected, result, "isModulePrefix(%q) should be %v", tt.prefix, tt.expected)
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
