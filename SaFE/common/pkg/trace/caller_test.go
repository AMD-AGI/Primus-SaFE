// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package trace

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGetNearestCaller tests GetNearestCaller function
func TestGetNearestCaller(t *testing.T) {
	// This test depends on being called from the primus-safe codebase
	// We can only verify it returns a string format, not the exact value
	caller := GetNearestCaller(0)
	
	// The result may be empty if not called from primus-safe path
	// or should be in format "package:line"
	if caller != "" {
		assert.Contains(t, caller, ":")
	}
}

// TestGetNearestCaller_WithSkip tests GetNearestCaller with different skip values
func TestGetNearestCaller_WithSkip(t *testing.T) {
	tests := []struct {
		name string
		skip int
	}{
		{"skip 0", 0},
		{"skip 1", 1},
		{"skip 2", 2},
		{"skip 5", 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			assert.NotPanics(t, func() {
				_ = GetNearestCaller(tt.skip)
			})
		})
	}
}

// TestIsCallerIgnored tests isCallerIgnored function with SaFE code paths
func TestIsCallerIgnored(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		expected bool
	}{
		{
			name:     "ignored DAL caller - database client dal",
			caller:   "primus-safe/common/pkg/database/client/dal.QueryImage",
			expected: true,
		},
		{
			name:     "ignored DAL caller - ops job dal",
			caller:   "primus-safe/common/pkg/database/client/dal.InsertOpsJob",
			expected: true,
		},
		{
			name:     "not ignored - apiserver handler",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers/resources.GetNode",
			expected: false,
		},
		{
			name:     "not ignored - database client (not dal)",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/common/pkg/database/client.SelectImages",
			expected: false,
		},
		{
			name:     "empty caller",
			caller:   "",
			expected: false,
		},
		{
			name:     "non-matching caller",
			caller:   "some/other/package.Function",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCallerIgnored(tt.caller)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetPackageName tests getPackageName function with SaFE paths
func TestGetPackageName(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		expected string
	}{
		{
			name:     "SaFE apiserver handler",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers/resources.GetNode",
			expected: "AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers/resources.GetNode",
		},
		{
			name:     "SaFE common trace package",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/common/pkg/trace.GetNearestCaller",
			expected: "AMD-AGI/Primus-SaFE/SaFE/common/pkg/trace.GetNearestCaller",
		},
		{
			name:     "SaFE database dal",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/common/pkg/database/client/dal.QueryImage",
			expected: "AMD-AGI/Primus-SaFE/SaFE/common/pkg/database/client/dal.QueryImage",
		},
		{
			name:     "without github.com prefix",
			caller:   "local/package.Function",
			expected: "local/package.Function",
		},
		{
			name:     "empty caller",
			caller:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPackageName(tt.caller)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTrimPackagePrefixes tests TrimPackagePrefixes function with SaFE paths
func TestTrimPackagePrefixes(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		expected string
	}{
		{
			name:     "SaFE apiserver handler - trim prefix",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers/resources.GetNode",
			expected: "/apiserver/pkg/handlers/resources.GetNode",
		},
		{
			name:     "SaFE common trace - trim prefix",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/common/pkg/trace.GetNearestCaller",
			expected: "/common/pkg/trace.GetNearestCaller",
		},
		{
			name:     "SaFE database dal - trim prefix",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/common/pkg/database/client/dal.QueryImage",
			expected: "/common/pkg/database/client/dal.QueryImage",
		},
		{
			name:     "without prefix - no change",
			caller:   "some/other/package.Function",
			expected: "some/other/package.Function",
		},
		{
			name:     "empty caller",
			caller:   "",
			expected: "",
		},
		{
			name:     "only prefix - returns empty",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimPackagePrefixes(tt.caller)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestCallerKeyword tests the callerKeyword constant
func TestCallerKeyword(t *testing.T) {
	assert.Equal(t, "primus-safe", callerKeyword)
}

// TestCallerIgnoresRegex tests the callerIgnoresRegex patterns
func TestCallerIgnoresRegex(t *testing.T) {
	assert.NotEmpty(t, callerIgnoresRegex)
	assert.Len(t, callerIgnoresRegex, 1)

	// Test the regex pattern
	pattern := callerIgnoresRegex[0]
	assert.NotNil(t, pattern)

	// Test with SaFE-style paths
	tests := []struct {
		input    string
		expected bool
	}{
		// Should match (DAL layer)
		{"primus-safe/common/pkg/database/client/dal.QueryImage", true},
		{"primus-safe/common/pkg/database/client/dal.InsertOpsJob", true},
		{"primus-safe/core/database/user/dal.GetUser", true},
		// Should not match
		{"primus-safe/apiserver/pkg/handlers/resources.GetNode", false},
		{"primus-safe/common/pkg/database/client.SelectImages", false},
		{"github.com/AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers.GetNode", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := pattern.MatchString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPackagePrefixList tests the packagePrefixList
func TestPackagePrefixList(t *testing.T) {
	assert.NotEmpty(t, packagePrefixList)
	assert.Contains(t, packagePrefixList, "github.com/AMD-AGI/Primus-SaFE/SaFE")
}

// TestGetNearestCaller_NestedCalls tests GetNearestCaller with nested function calls
func TestGetNearestCaller_NestedCalls(t *testing.T) {
	var level1, level2, level3 func() string
	
	level1 = func() string {
		return level2()
	}
	level2 = func() string {
		return level3()
	}
	level3 = func() string {
		return GetNearestCaller(0)
	}

	result := level1()
	// Should return a valid result or empty string
	if result != "" {
		assert.Contains(t, result, ":")
	}
}

// TestIsCallerIgnored_WithCustomRegex tests isCallerIgnored with different regex patterns
func TestIsCallerIgnored_WithCustomRegex(t *testing.T) {
	// Save original regex
	originalRegex := callerIgnoresRegex

	// Test with custom regex patterns
	callerIgnoresRegex = []*regexp.Regexp{
		regexp.MustCompile(`^test/.*$`),
		regexp.MustCompile(`/dal\.`), // Match any path containing /dal.
	}

	tests := []struct {
		name     string
		caller   string
		expected bool
	}{
		{"matches first regex", "test/package.Function", true},
		{"matches second regex - dal function", "primus-safe/common/pkg/database/client/dal.QueryImage", true},
		{"matches no regex", "primus-safe/apiserver/pkg/handlers.GetNode", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCallerIgnored(tt.caller)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Restore original regex
	callerIgnoresRegex = originalRegex
}

// TestTrimPackagePrefixes_MultiplePrefixes tests TrimPackagePrefixes with multiple prefixes
func TestTrimPackagePrefixes_MultiplePrefixes(t *testing.T) {
	// Save original prefixes
	originalPrefixes := packagePrefixList

	// Test with multiple prefixes
	packagePrefixList = []string{
		"github.com/AMD-AGI/Primus-SaFE/SaFE",
		"github.com/another/prefix",
	}

	tests := []struct {
		name     string
		caller   string
		expected string
	}{
		{
			name:     "matches first prefix - SaFE",
			caller:   "github.com/AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers.GetNode",
			expected: "/apiserver/pkg/handlers.GetNode",
		},
		{
			name:     "matches second prefix",
			caller:   "github.com/another/prefix/pkg.Function",
			expected: "/pkg.Function",
		},
		{
			name:     "matches no prefix",
			caller:   "github.com/other/repo/pkg.Function",
			expected: "github.com/other/repo/pkg.Function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TrimPackagePrefixes(tt.caller)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Restore original prefixes
	packagePrefixList = originalPrefixes
}

// BenchmarkGetNearestCaller benchmarks GetNearestCaller function
func BenchmarkGetNearestCaller(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = GetNearestCaller(0)
	}
}

// BenchmarkIsCallerIgnored benchmarks isCallerIgnored function
func BenchmarkIsCallerIgnored(b *testing.B) {
	caller := "primus-safe/common/pkg/database/client/dal.QueryImage"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isCallerIgnored(caller)
	}
}

// BenchmarkGetPackageName benchmarks getPackageName function
func BenchmarkGetPackageName(b *testing.B) {
	caller := "github.com/AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers/resources.GetNode"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getPackageName(caller)
	}
}

// BenchmarkTrimPackagePrefixes benchmarks TrimPackagePrefixes function
func BenchmarkTrimPackagePrefixes(b *testing.B) {
	caller := "github.com/AMD-AGI/Primus-SaFE/SaFE/apiserver/pkg/handlers/resources.GetNode"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TrimPackagePrefixes(caller)
	}
}
