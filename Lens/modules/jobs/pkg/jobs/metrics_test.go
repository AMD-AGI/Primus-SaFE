// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package jobs

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"github.com/stretchr/testify/assert"
)

// mockJobForMetrics is a mock implementation for testing metrics
type mockJobForMetrics struct {
	name string
}

func (m *mockJobForMetrics) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	return &common.ExecutionStats{}, nil
}

func (m *mockJobForMetrics) Schedule() string {
	return "@every 1m"
}

func TestGetJobName(t *testing.T) {
	tests := []struct {
		name     string
		job      Job
		expected string
	}{
		{
			name:     "get name from mock job",
			job:      &mockJobForMetrics{name: "test"},
			expected: "mockJobForMetrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJobName(tt.job)
			assert.Equal(t, tt.expected, result, "Job name should match expected")
		})
	}
}

func TestGetJobNameWithPointerType(t *testing.T) {
	job := &mockJobForMetrics{name: "test"}
	result := getJobName(job)

	assert.Equal(t, "mockJobForMetrics", result, "Should extract type name from pointer")
}

func TestGetJobNameWithNonPointerType(t *testing.T) {
	job := mockJobForMetrics{name: "test"}
	result := getJobName(&job)

	assert.Equal(t, "mockJobForMetrics", result, "Should extract type name from non-pointer")
}

// Additional mock types for comprehensive testing
type firstJob struct{}

func (f *firstJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	return &common.ExecutionStats{}, nil
}

func (f *firstJob) Schedule() string {
	return "@every 10s"
}

type secondJob struct{}

func (s *secondJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	return &common.ExecutionStats{}, nil
}

func (s *secondJob) Schedule() string {
	return "@every 30s"
}

func TestGetJobNameMultipleTypes(t *testing.T) {
	tests := []struct {
		name     string
		job      Job
		expected string
	}{
		{
			name:     "firstJob",
			job:      &firstJob{},
			expected: "firstJob",
		},
		{
			name:     "secondJob",
			job:      &secondJob{},
			expected: "secondJob",
		},
		{
			name:     "mockJobForMetrics",
			job:      &mockJobForMetrics{name: "test"},
			expected: "mockJobForMetrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getJobName(tt.job)
			assert.Equal(t, tt.expected, result, "Job name should match expected")
		})
	}
}
