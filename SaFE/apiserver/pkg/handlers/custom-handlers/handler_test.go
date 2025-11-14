/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// TestCvtToResourceList tests the conversion from Kubernetes ResourceList to custom ResourceList
func TestCvtToResourceList(t *testing.T) {
	tests := []struct {
		name     string
		input    corev1.ResourceList
		expected map[string]int64
	}{
		{
			name: "normal resource list",
			input: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			expected: map[string]int64{
				"cpu":    4,
				"memory": 8 * 1024 * 1024 * 1024,
			},
		},
		{
			name: "resource list with negative value",
			input: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("-2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
			expected: map[string]int64{
				"cpu":    0, // Negative values should be converted to 0
				"memory": 4 * 1024 * 1024 * 1024,
			},
		},
		{
			name: "resource list with GPU",
			input: corev1.ResourceList{
				corev1.ResourceCPU:                    resource.MustParse("8"),
				corev1.ResourceMemory:                 resource.MustParse("16Gi"),
				corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
			},
			expected: map[string]int64{
				"cpu":            8,
				"memory":         16 * 1024 * 1024 * 1024,
				"nvidia.com/gpu": 2,
			},
		},
		{
			name:     "empty resource list",
			input:    corev1.ResourceList{},
			expected: nil,
		},
		{
			name:     "nil resource list",
			input:    nil,
			expected: nil,
		},
		{
			name: "resource list with fractional CPU",
			input: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2000m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
			expected: map[string]int64{
				"cpu":    2, // 2000m = 2 CPU
				"memory": 1 * 1024 * 1024 * 1024,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtToResourceList(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, len(tt.expected), len(result))
				for key, expectedVal := range tt.expected {
					assert.Equal(t, expectedVal, result[key], "Resource %s value mismatch", key)
				}
			}
		})
	}
}
