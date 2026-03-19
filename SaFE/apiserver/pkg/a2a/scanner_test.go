/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2a

import (
	"testing"
)

func TestParseLabelSelector(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]string
	}{
		{
			name:     "single label",
			input:    "a2a.primus.io/enabled=true",
			expected: map[string]string{"a2a.primus.io/enabled": "true"},
		},
		{
			name:     "multiple labels",
			input:    "a2a.primus.io/enabled=true, app=test",
			expected: map[string]string{"a2a.primus.io/enabled": "true", "app": "test"},
		},
		{
			name:     "empty",
			input:    "",
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLabelSelector(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d labels, got %d", len(tt.expected), len(result))
			}
			for k, v := range tt.expected {
				if result[k] != v {
					t.Errorf("expected %s=%s, got %s=%s", k, v, k, result[k])
				}
			}
		})
	}
}

func TestSplitTrim(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		sep      string
		expected []string
	}{
		{"normal", "a,b,c", ",", []string{"a", "b", "c"}},
		{"with spaces", " a , b , c ", ",", []string{"a", "b", "c"}},
		{"empty", "", ",", []string{}},
		{"single", "abc", ",", []string{"abc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitTrim(tt.input, tt.sep)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d parts, got %d: %v", len(tt.expected), len(result), result)
			}
			for i, v := range tt.expected {
				if i < len(result) && result[i] != v {
					t.Errorf("expected %s at index %d, got %s", v, i, result[i])
				}
			}
		})
	}
}
