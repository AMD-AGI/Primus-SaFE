/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

func TestMapOpsJobPhaseToDatasetStatus(t *testing.T) {
	tests := []struct {
		name     string
		phase    v1.OpsJobPhase
		expected dbclient.DatasetStatus
	}{
		{
			name:     "pending phase",
			phase:    v1.OpsJobPending,
			expected: dbclient.DatasetStatusPending,
		},
		{
			name:     "running phase",
			phase:    v1.OpsJobRunning,
			expected: dbclient.DatasetStatusDownloading,
		},
		{
			name:     "succeeded phase",
			phase:    v1.OpsJobSucceeded,
			expected: dbclient.DatasetStatusReady,
		},
		{
			name:     "failed phase",
			phase:    v1.OpsJobFailed,
			expected: dbclient.DatasetStatusFailed,
		},
		{
			name:     "unknown phase returns pending",
			phase:    v1.OpsJobPhase("Unknown"),
			expected: dbclient.DatasetStatusPending,
		},
		{
			name:     "empty phase returns pending",
			phase:    v1.OpsJobPhase(""),
			expected: dbclient.DatasetStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapOpsJobPhaseToDatasetStatus(tt.phase)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractOpsJobFailureMessage(t *testing.T) {
	tests := []struct {
		name     string
		job      *v1.OpsJob
		expected string
	}{
		{
			name: "extract failure message from conditions",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Failed",
							Message: "Download failed: connection timeout",
						},
					},
				},
			},
			expected: "Download failed: connection timeout",
		},
		{
			name: "no failure condition returns default message",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Running",
							Message: "Job is running",
						},
					},
				},
			},
			expected: "Download failed",
		},
		{
			name: "empty conditions returns default message",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expected: "Download failed",
		},
		{
			name: "failed condition with empty message returns default",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Failed",
							Message: "",
						},
					},
				},
			},
			expected: "Download failed",
		},
		{
			name: "multiple conditions picks failed one",
			job: &v1.OpsJob{
				Status: v1.OpsJobStatus{
					Conditions: []metav1.Condition{
						{
							Type:    "Running",
							Message: "Was running",
						},
						{
							Type:    "Failed",
							Message: "S3 bucket not accessible",
						},
					},
				},
			},
			expected: "S3 bucket not accessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOpsJobFailureMessage(tt.job)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDatasetOpsJobPredicateLogic(t *testing.T) {
	// Test the underlying logic used in datasetOpsJobPredicate
	tests := []struct {
		name     string
		job      *v1.OpsJob
		expected bool
	}{
		{
			name: "job with dataset-id label should match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
					Labels: map[string]string{
						dbclient.DatasetIdLabel: "dataset-abc123",
					},
				},
			},
			expected: true,
		},
		{
			name: "job without dataset-id label should not match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
					Labels: map[string]string{
						"other-label": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "job with nil labels should not match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-job",
					Labels: nil,
				},
			},
			expected: false,
		},
		{
			name: "job with empty labels should not match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-job",
					Labels: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "job with dataset-id and other labels should match",
			job: &v1.OpsJob{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-job",
					Labels: map[string]string{
						dbclient.DatasetIdLabel: "dataset-xyz789",
						v1.WorkspaceIdLabel:     "ws-1",
						"other-label":           "value",
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the underlying logic that datasetOpsJobPredicate uses
			labels := tt.job.GetLabels()
			var hasDatasetId bool
			if labels != nil {
				_, hasDatasetId = labels[dbclient.DatasetIdLabel]
			}
			assert.Equal(t, tt.expected, hasDatasetId)
		})
	}
}
