// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// mockJob is a mock implementation of Job interface for testing
type mockJob struct {
	schedule string
}

func (m *mockJob) Schedule() string {
	return m.schedule
}

func TestGetJobName(t *testing.T) {
	tests := []struct {
		name     string
		job      Job
		expected string
	}{
		{
			name:     "get name from mockJob",
			job:      &mockJob{schedule: "@every 1m"},
			expected: "mockJob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJobName(tt.job)
			assert.Equal(t, tt.expected, result, "Job name should match expected")
		})
	}
}

func TestGetJobNameWithNonPointer(t *testing.T) {
	// Test with non-pointer type (need to use pointer since Schedule has pointer receiver)
	job := mockJob{schedule: "@every 1m"}
	result := getJobName(&job)
	assert.Equal(t, "mockJob", result, "Should handle non-pointer types")
}

func TestGetJobType(t *testing.T) {
	tests := []struct {
		name         string
		job          Job
		expectedName string
	}{
		{
			name:         "get type from mockJob pointer",
			job:          &mockJob{schedule: "@every 1m"},
			expectedName: "mockJob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJobType(tt.job)

			// The result should contain the package path and type name
			assert.Contains(t, result, tt.expectedName, "Job type should contain type name")
			assert.Contains(t, result, "common", "Job type should contain package name")
		})
	}
}

func TestGetJobTypeWithNonPointer(t *testing.T) {
	// Test with non-pointer type (need to use pointer since Schedule has pointer receiver)
	job := mockJob{schedule: "@every 1m"}
	result := getJobType(&job)

	assert.Contains(t, result, "mockJob", "Should contain type name")
	assert.Contains(t, result, "common", "Should contain package name")
}

// Additional mock job types for testing
type anotherMockJob struct {
	data string
}

func (a *anotherMockJob) Schedule() string {
	return "@every 5m"
}

func TestGetJobNameDifferentTypes(t *testing.T) {
	tests := []struct {
		name     string
		job      Job
		expected string
	}{
		{
			name:     "mockJob type",
			job:      &mockJob{schedule: "@every 1m"},
			expected: "mockJob",
		},
		{
			name:     "anotherMockJob type",
			job:      &anotherMockJob{data: "test"},
			expected: "anotherMockJob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJobName(tt.job)
			assert.Equal(t, tt.expected, result, "Job name should match expected")
		})
	}
}

func TestGetJobTypeDifferentTypes(t *testing.T) {
	tests := []struct {
		name         string
		job          Job
		expectedName string
	}{
		{
			name:         "mockJob type",
			job:          &mockJob{schedule: "@every 1m"},
			expectedName: "mockJob",
		},
		{
			name:         "anotherMockJob type",
			job:          &anotherMockJob{data: "test"},
			expectedName: "anotherMockJob",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJobType(tt.job)
			assert.Contains(t, result, tt.expectedName, "Job type should contain type name")
		})
	}
}
