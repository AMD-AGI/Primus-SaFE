// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestCalculateGpuRequest(t *testing.T) {
	r := &WorkloadReconciler{}
	ctx := context.Background()

	tests := []struct {
		name     string
		workload *primusSafeV1.Workload
		expected int32
	}{
		{
			name: "normal calculation - single replica single GPU",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "1",
						Replica: 1,
					},
				},
			},
			expected: 1,
		},
		{
			name: "normal calculation - multiple replicas multiple GPUs",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "4",
						Replica: 8,
					},
				},
			},
			expected: 32,
		},
		{
			name: "zero replicas",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "2",
						Replica: 0,
					},
				},
			},
			expected: 0,
		},
		{
			name: "GPU count is 0",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "0",
						Replica: 5,
					},
				},
			},
			expected: 0,
		},
		{
			name: "invalid GPU string - empty string",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "",
						Replica: 3,
					},
				},
			},
			expected: 0,
		},
		{
			name: "invalid GPU string - non-numeric",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "invalid",
						Replica: 3,
					},
				},
			},
			expected: 0,
		},
		{
			name: "large value calculation",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "8",
						Replica: 100,
					},
				},
			},
			expected: 800,
		},
		{
			name: "negative GPU string",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "-2",
						Replica: 4,
					},
				},
			},
			expected: -8,
		},
		{
			name: "decimal GPU string - should fail",
			workload: &primusSafeV1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "default",
				},
				Spec: primusSafeV1.WorkloadSpec{
					Resource: primusSafeV1.WorkloadResource{
						GPU:     "2.5",
						Replica: 4,
					},
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.calculateGpuRequest(ctx, tt.workload)
			assert.Equal(t, tt.expected, result, "GPU request calculation mismatch")
		})
	}
}

