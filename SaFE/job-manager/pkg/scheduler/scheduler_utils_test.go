/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// TestFormatResourceName tests formatting of resource names
func TestFormatResourceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "NVIDIA GPU",
			input:    common.NvidiaGpu,
			expected: "gpu",
		},
		{
			name:     "AMD GPU",
			input:    common.AmdGpu,
			expected: "gpu",
		},
		{
			name:     "CPU",
			input:    "cpu",
			expected: "cpu",
		},
		{
			name:     "Memory",
			input:    "memory",
			expected: "memory",
		},
		{
			name:     "Custom resource",
			input:    "custom-resource",
			expected: "custom-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatResourceName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsReScheduledForFailover tests failover rescheduling detection
func TestIsReScheduledForFailover(t *testing.T) {
	tests := []struct {
		name     string
		workload *v1.Workload
		expected bool
	}{
		{
			name: "rescheduled for failover",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.WorkloadReScheduledAnnotation: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "not rescheduled",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{},
				},
			},
			expected: false,
		},
		{
			name: "preempted (not failover)",
			workload: &v1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						v1.WorkloadReScheduledAnnotation: "true",
						v1.WorkloadPreemptedAnnotation:   "true",
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isReScheduledForFailover(tt.workload)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsHaveDependencies tests dependency checking
func TestIsHaveDependencies(t *testing.T) {
	tests := []struct {
		name     string
		workload *v1.Workload
		expected bool
	}{
		{
			name: "no dependencies",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{},
				},
			},
			expected: false,
		},
		{
			name: "dependency not met",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{"dep-1"},
				},
				Status: v1.WorkloadStatus{
					DependenciesPhase: map[string]v1.WorkloadPhase{
						"dep-1": v1.WorkloadRunning,
					},
				},
			},
			expected: true,
		},
		{
			name: "all dependencies succeeded",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{"dep-1", "dep-2"},
				},
				Status: v1.WorkloadStatus{
					DependenciesPhase: map[string]v1.WorkloadPhase{
						"dep-1": v1.WorkloadSucceeded,
						"dep-2": v1.WorkloadSucceeded,
					},
				},
			},
			expected: false,
		},
		{
			name: "missing dependency status",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Dependencies: []string{"dep-1"},
				},
				Status: v1.WorkloadStatus{
					DependenciesPhase: map[string]v1.WorkloadPhase{},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isHaveDependencies(tt.workload)
			assert.Equal(t, tt.expected, result)
		})
	}
}
