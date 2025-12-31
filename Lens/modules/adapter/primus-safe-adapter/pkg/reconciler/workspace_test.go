/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package reconciler

import (
	"context"
	"testing"

	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestExtractGpuResource(t *testing.T) {
	r := &WorkspaceReconciler{}

	tests := []struct {
		name      string
		workspace *primusSafeV1.Workspace
		expected  int32
	}{
		{
			name: "AMD GPU resource present",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						corev1.ResourceName(AMDGPUResourceName): resource.MustParse("8"),
					},
				},
			},
			expected: 8,
		},
		{
			name: "NVIDIA GPU resource present",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						"nvidia.com/gpu": resource.MustParse("16"),
					},
				},
			},
			expected: 16,
		},
		{
			name: "No GPU resource",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						"cpu":    resource.MustParse("32"),
						"memory": resource.MustParse("128Gi"),
					},
				},
			},
			expected: 0,
		},
		{
			name: "Empty TotalResources",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: nil,
				},
			},
			expected: 0,
		},
		{
			name: "Large GPU count",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						corev1.ResourceName(AMDGPUResourceName): resource.MustParse("512"),
					},
				},
			},
			expected: 512,
		},
		{
			name: "Zero GPU count",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						corev1.ResourceName(AMDGPUResourceName): resource.MustParse("0"),
					},
				},
			},
			expected: 0,
		},
		{
			name: "AMD GPU takes precedence over NVIDIA",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						corev1.ResourceName(AMDGPUResourceName): resource.MustParse("8"),
						"nvidia.com/gpu":                        resource.MustParse("16"),
					},
				},
			},
			expected: 8,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.extractGpuResource(tt.workspace)
			assert.Equal(t, tt.expected, result, "GPU resource extraction mismatch")
		})
	}
}

func TestGetGpuModel(t *testing.T) {
	r := &WorkspaceReconciler{}
	ctx := context.Background()

	tests := []struct {
		name      string
		workspace *primusSafeV1.Workspace
		expected  string
	}{
		{
			name: "Node flavor specified",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: primusSafeV1.WorkspaceSpec{
					NodeFlavor: "MI300X",
				},
			},
			expected: "MI300X",
		},
		{
			name: "No node flavor",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: primusSafeV1.WorkspaceSpec{
					NodeFlavor: "",
				},
			},
			expected: "",
		},
		{
			name: "Different node flavor",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Spec: primusSafeV1.WorkspaceSpec{
					NodeFlavor: "MI250X",
				},
			},
			expected: "MI250X",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.getGpuModel(ctx, tt.workspace, nil)
			assert.Equal(t, tt.expected, result, "GPU model mismatch")
		})
	}
}

func TestCalculateGpuResource(t *testing.T) {
	r := &WorkspaceReconciler{}

	tests := []struct {
		name      string
		workspace *primusSafeV1.Workspace
		expected  int32
	}{
		{
			name: "AMD GPU in status",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						corev1.ResourceName(AMDGPUResourceName): resource.MustParse("64"),
					},
				},
			},
			expected: 64,
		},
		{
			name: "NVIDIA GPU in status",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{
						"nvidia.com/gpu": resource.MustParse("32"),
					},
				},
			},
			expected: 32,
		},
		{
			name: "No GPU resources",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: corev1.ResourceList{},
				},
			},
			expected: 0,
		},
		{
			name: "Nil total resources",
			workspace: &primusSafeV1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-workspace",
				},
				Status: primusSafeV1.WorkspaceStatus{
					TotalResources: nil,
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := r.calculateGpuResource(tt.workspace)
			assert.Equal(t, tt.expected, result, "GPU resource calculation mismatch")
		})
	}
}

func TestParseGpuQuantity(t *testing.T) {
	tests := []struct {
		name     string
		quantity resource.Quantity
		expected int32
	}{
		{
			name:     "Single GPU",
			quantity: resource.MustParse("1"),
			expected: 1,
		},
		{
			name:     "Multiple GPUs",
			quantity: resource.MustParse("8"),
			expected: 8,
		},
		{
			name:     "Large GPU count",
			quantity: resource.MustParse("512"),
			expected: 512,
		},
		{
			name:     "Zero GPUs",
			quantity: resource.MustParse("0"),
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseGpuQuantity(tt.quantity)
			assert.Equal(t, tt.expected, result, "GPU quantity parsing mismatch")
		})
	}
}
