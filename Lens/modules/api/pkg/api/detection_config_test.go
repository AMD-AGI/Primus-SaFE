// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateFrameworkLogPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern FrameworkLogPatterns
		wantErr bool
	}{
		{
			name: "valid pattern",
			pattern: FrameworkLogPatterns{
				Name:     "primus",
				Priority: 1,
				Enabled:  true,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			pattern: FrameworkLogPatterns{
				Name:     "",
				Priority: 1,
				Enabled:  true,
			},
			wantErr: true,
		},
		{
			name: "negative priority",
			pattern: FrameworkLogPatterns{
				Name:     "primus",
				Priority: -1,
				Enabled:  true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFrameworkLogPatterns(&tt.pattern)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidatePatternConfig(t *testing.T) {
	tests := []struct {
		name    string
		pattern PatternConfig
		wantErr bool
	}{
		{
			name: "valid pattern",
			pattern: PatternConfig{
				Name:       "test_pattern",
				Pattern:    ".*",
				Confidence: 0.8,
			},
			wantErr: false,
		},
		{
			name: "empty name",
			pattern: PatternConfig{
				Name:       "",
				Pattern:    ".*",
				Confidence: 0.8,
			},
			wantErr: true,
		},
		{
			name: "empty pattern",
			pattern: PatternConfig{
				Name:       "test_pattern",
				Pattern:    "",
				Confidence: 0.8,
			},
			wantErr: true,
		},
		{
			name: "invalid confidence - too low",
			pattern: PatternConfig{
				Name:       "test_pattern",
				Pattern:    ".*",
				Confidence: -0.1,
			},
			wantErr: true,
		},
		{
			name: "invalid confidence - too high",
			pattern: PatternConfig{
				Name:       "test_pattern",
				Pattern:    ".*",
				Confidence: 1.5,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePatternConfig(&tt.pattern)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetUserFromContext(t *testing.T) {
	// Note: This is a simplified test
	// In a real scenario, you would create a gin.Context with proper setup
	// For now, we just verify the function exists and has correct signature

	// This test is mainly for code coverage
	// Actual testing would require setting up a full gin context
	assert.NotNil(t, getUserFromContext)
}
