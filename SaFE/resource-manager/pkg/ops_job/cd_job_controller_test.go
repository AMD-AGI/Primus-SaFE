/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package ops_job

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

func TestGetParameterValue(t *testing.T) {
	tests := []struct {
		name         string
		job          *v1.OpsJob
		paramName    string
		defaultValue string
		expected     string
	}{
		{
			name: "get existing parameter",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{
						{Name: "test_param", Value: "test_value"},
					},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "test_value",
		},
		{
			name: "get non-existing parameter returns default",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{
						{Name: "other_param", Value: "other_value"},
					},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "default",
		},
		{
			name: "empty inputs returns default",
			job: &v1.OpsJob{
				Spec: v1.OpsJobSpec{
					Inputs: []v1.Parameter{},
				},
			},
			paramName:    "test_param",
			defaultValue: "default",
			expected:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getParameterValue(tt.job, tt.paramName, tt.defaultValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}
