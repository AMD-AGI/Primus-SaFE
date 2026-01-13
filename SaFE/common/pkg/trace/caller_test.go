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
	// This test depends on being called from the primus-lens codebase
	// We can only verify it returns a string format, not the exact value
	caller := GetNearestCaller(0)
	
	// The result may be empty if not called from primus-lens path
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

// TestIsCallerIgnored tests isCallerIgnored function
func TestIsCallerIgnored(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		expected bool
	}{
		{
			name:     "ignored dal caller",
			caller:   "primus-lens/core/database/user/dal.GetUser",
			expected: true,
		},
		{
			name:     "not ignored caller",
			caller:   "primus-lens/core/service/user.GetUser",
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

// TestGetPackageName tests getPackageName function
func TestGetPackageName(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		expected string
	}{
		{
			name:     "with github.com prefix",
			caller:   "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace.GetNearestCaller",
			expected: "AMD-AGI/Primus-SaFE/Lens/core/pkg/trace.GetNearestCaller",
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
		{
			name:     "multiple github.com in path",
			caller:   "github.com/user/repo/github.com/another/path.Function",
			expected: "user/repo/", // Split only takes first part after "github.com/"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPackageName(tt.caller)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestTrimPackagePrefixes tests TrimPackagePrefixes function
func TestTrimPackagePrefixes(t *testing.T) {
	tests := []struct {
		name     string
		caller   string
		expected string
	}{
		{
			name:     "with standard prefix",
			caller:   "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace.GetNearestCaller",
			expected: "/core/pkg/trace.GetNearestCaller",
		},
		{
			name:     "without prefix",
			caller:   "some/other/package.Function",
			expected: "some/other/package.Function",
		},
		{
			name:     "empty caller",
			caller:   "",
			expected: "",
		},
		{
			name:     "only prefix",
			caller:   "github.com/AMD-AGI/Primus-SaFE/Lens",
			expected: "",
		},
		{
			name:     "partial prefix match",
			caller:   "github.com/AMD-AGI/other-project/pkg.Function",
			expected: "github.com/AMD-AGI/other-project/pkg.Function",
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
	assert.Equal(t, "primus-lens", callerKeyword)
}

// TestCallerIgnoresRegex tests the callerIgnoresRegex patterns
func TestCallerIgnoresRegex(t *testing.T) {
	assert.NotEmpty(t, callerIgnoresRegex)
	assert.Len(t, callerIgnoresRegex, 1)

	// Test the regex pattern
	pattern := callerIgnoresRegex[0]
	assert.NotNil(t, pattern)

	// Test some examples
	tests := []struct {
		input    string
		expected bool
	}{
		{"primus-lens/core/database/user/dal.GetUser", true},
		{"primus-lens/core/database/system/dal_impl.SaveConfig", true},
		{"primus-lens/api/service/user.GetUser", false},
		{"primus-lens/core/database/model.User", false},
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
	assert.Contains(t, packagePrefixList, "github.com/AMD-AGI/Primus-SaFE/Lens")
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

	// Test with custom regex
	callerIgnoresRegex = []*regexp.Regexp{
		regexp.MustCompile(`^test/.*$`),
		regexp.MustCompile(`^primus-lens/[^*]+/database/[^*]+/dal[^*]+$`),
	}

	tests := []struct {
		name     string
		caller   string
		expected bool
	}{
		{"matches first regex", "test/package.Function", true},
		{"matches second regex", "primus-lens/core/database/user/dal.GetUser", true},
		{"matches no regex", "other/package.Function", false},
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
		"github.com/AMD-AGI/Primus-SaFE/Lens",
		"github.com/another/prefix",
	}

	tests := []struct {
		name     string
		caller   string
		expected string
	}{
		{
			name:     "matches first prefix",
			caller:   "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace.Function",
			expected: "/core/pkg/trace.Function",
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
	caller := "primus-lens/core/database/user/dal.GetUser"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = isCallerIgnored(caller)
	}
}

// BenchmarkGetPackageName benchmarks getPackageName function
func BenchmarkGetPackageName(b *testing.B) {
	caller := "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace.GetNearestCaller"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = getPackageName(caller)
	}
}

// BenchmarkTrimPackagePrefixes benchmarks TrimPackagePrefixes function
func BenchmarkTrimPackagePrefixes(b *testing.B) {
	caller := "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace.GetNearestCaller"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TrimPackagePrefixes(caller)
	}
}

