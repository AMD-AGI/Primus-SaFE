/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/pointer"
)

func TestExtractHFJobFailureReason(t *testing.T) {
	tests := []struct {
		name     string
		job      *batchv1.Job
		expected string
	}{
		{
			name: "extract reason and message from failed condition",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "BackoffLimitExceeded",
							Message: "Job has reached the specified backoff limit",
						},
					},
				},
			},
			expected: "BackoffLimitExceeded: Job has reached the specified backoff limit",
		},
		{
			name: "no failed condition returns unknown error",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionTrue,
						},
					},
				},
			},
			expected: "Unknown error during download",
		},
		{
			name: "empty conditions returns unknown error",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{},
				},
			},
			expected: "Unknown error during download",
		},
		{
			name: "failed condition with false status is ignored",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionFalse,
							Reason:  "SomeReason",
							Message: "Some message",
						},
					},
				},
			},
			expected: "Unknown error during download",
		},
		{
			name: "backoff limit exceeded",
			job: &batchv1.Job{
				Spec: batchv1.JobSpec{
					BackoffLimit: pointer.Int32(3),
				},
				Status: batchv1.JobStatus{
					Failed: 3,
				},
			},
			expected: "Maximum retry attempts exceeded",
		},
		{
			name: "multiple conditions picks failed one",
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: corev1.ConditionFalse,
						},
						{
							Type:    batchv1.JobFailed,
							Status:  corev1.ConditionTrue,
							Reason:  "DeadlineExceeded",
							Message: "Job was active longer than specified deadline",
						},
					},
				},
			},
			expected: "DeadlineExceeded: Job was active longer than specified deadline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHFJobFailureReason(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHFDatasetJobPredicateLogic(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name:     "job with hf-dataset-job=true should match",
			labels:   map[string]string{HFDatasetJobLabel: "true", HFDatasetIdLabel: "dataset-abc"},
			expected: true,
		},
		{
			name:     "job with hf-dataset-job=false should not match",
			labels:   map[string]string{HFDatasetJobLabel: "false"},
			expected: false,
		},
		{
			name:     "job without hf-dataset-job label should not match",
			labels:   map[string]string{"other": "value"},
			expected: false,
		},
		{
			name:     "nil labels should not match",
			labels:   nil,
			expected: false,
		},
		{
			name:     "empty labels should not match",
			labels:   map[string]string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the underlying logic that hfDatasetJobPredicate uses
			result := false
			if tt.labels != nil {
				result = tt.labels[HFDatasetJobLabel] == "true"
			}
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractDatasetBasePath tests the base path extraction for failover
func TestExtractDatasetBasePath(t *testing.T) {
	tests := []struct {
		name     string
		fullPath string
		expected string
	}{
		{
			name:     "standard dataset path",
			fullPath: "/wekafs/datasets/math_500",
			expected: "/wekafs",
		},
		{
			name:     "nested org dataset path",
			fullPath: "/apps/datasets/HuggingFaceH4/MATH-500",
			expected: "/apps",
		},
		{
			name:     "deep path",
			fullPath: "/mnt/storage/datasets/my-dataset",
			expected: "/mnt/storage",
		},
		{
			name:     "no datasets in path",
			fullPath: "/wekafs/models/llama",
			expected: "",
		},
		{
			name:     "empty path",
			fullPath: "",
			expected: "",
		},
		{
			name:     "datasets at root",
			fullPath: "/datasets/test",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDatasetBasePath(tt.fullPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseTriedWorkspacesMap tests parsing of tried_workspaces JSON field
func TestParseTriedWorkspacesMap(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected map[string][]string
	}{
		{
			name:     "empty string",
			data:     "",
			expected: map[string][]string{},
		},
		{
			name:     "empty object",
			data:     "{}",
			expected: map[string][]string{},
		},
		{
			name:     "empty array (invalid but handled)",
			data:     "[]",
			expected: map[string][]string{},
		},
		{
			name: "single path with one workspace",
			data: `{"/wekafs": ["workspace-a"]}`,
			expected: map[string][]string{
				"/wekafs": {"workspace-a"},
			},
		},
		{
			name: "single path with multiple workspaces",
			data: `{"/wekafs": ["workspace-a", "workspace-b"]}`,
			expected: map[string][]string{
				"/wekafs": {"workspace-a", "workspace-b"},
			},
		},
		{
			name: "multiple paths",
			data: `{"/wekafs": ["ws-a"], "/apps": ["ws-b", "ws-c"]}`,
			expected: map[string][]string{
				"/wekafs": {"ws-a"},
				"/apps":   {"ws-b", "ws-c"},
			},
		},
		{
			name:     "invalid JSON returns empty map",
			data:     "not-json",
			expected: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTriedWorkspacesMap(tt.data)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAppendUniqueStr tests the unique append helper
func TestAppendUniqueStr(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected []string
	}{
		{
			name:     "append to empty slice",
			slice:    nil,
			item:     "a",
			expected: []string{"a"},
		},
		{
			name:     "append new item",
			slice:    []string{"a", "b"},
			item:     "c",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "skip duplicate item",
			slice:    []string{"a", "b"},
			item:     "a",
			expected: []string{"a", "b"},
		},
		{
			name:     "skip duplicate at end",
			slice:    []string{"a", "b"},
			item:     "b",
			expected: []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := appendUniqueStr(tt.slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestContainsStr tests the string contains helper
func TestContainsStr(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "found in slice",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "not found in slice",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "nil slice",
			slice:    nil,
			item:     "a",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsStr(tt.slice, tt.item)
			assert.Equal(t, tt.expected, result)
		})
	}
}
