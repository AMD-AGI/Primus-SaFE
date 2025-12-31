/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"testing"
	"time"

	"gotest.tools/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func TestValidateCronJobs(t *testing.T) {
	nowTime := time.Now().UTC()
	tests := []struct {
		name   string
		t      time.Time
		result bool
	}{
		{"Past time", nowTime.Add(-time.Hour), false},
		{"Future 1 minute", nowTime.Add(time.Minute), true},
		{"Future 6 months", nowTime.AddDate(0, 6, 0), true},
		{"Almost 1 year but less 1 minute", nowTime.AddDate(1, 0, 0).Add(-time.Minute), true},
		{"Exactly 1 year", nowTime.AddDate(1, 0, 0), false},
		{"Over 1 year", nowTime.AddDate(1, 0, 0).Add(time.Minute), false},
		{"now", nowTime, false},
	}

	var validator WorkloadValidator
	for _, tt := range tests {
		workload := &v1.Workload{
			Spec: v1.WorkloadSpec{
				CronJobs: []v1.CronJob{{
					Schedule: tt.t.Format(timeutil.TimeRFC3339Milli),
					Action:   v1.CronStart,
				}},
			},
		}
		err := validator.validateCronJobs(workload)
		assert.Equal(t, tt.result, err == nil)
	}
}

func TestMutateResources(t *testing.T) {
	gpuResourceName := "amd.com/gpu"
	workspaceWithGpu := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				v1.GpuResourceNameAnnotation: gpuResourceName,
			},
		},
	}

	tests := []struct {
		name              string
		workload          *v1.Workload
		workspace         *v1.Workspace
		expectedChanged   bool
		expectedResources []v1.WorkloadResource
	}{
		{
			name: "Replica 0 is filtered out",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 0, CPU: "8", Memory: "64Gi"},
						{Replica: 1, CPU: "16", Memory: "128Gi"},
					},
				},
			},
			workspace:       nil,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "16", Memory: "128Gi", SharedMemory: "64Gi", EphemeralStorage: DefaultEphemeralStorage},
			},
		},
		{
			name: "GPU '0' cleared and GPUName set from workspace",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 1, CPU: "8", GPU: "0", Memory: "64Gi"},
						{Replica: 1, CPU: "8", GPU: "4", Memory: "64Gi"},
					},
				},
			},
			workspace:       workspaceWithGpu,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "8", GPU: "", Memory: "64Gi", SharedMemory: "32Gi", EphemeralStorage: DefaultEphemeralStorage},
				{Replica: 1, CPU: "8", GPU: "4", GPUName: gpuResourceName, Memory: "64Gi", SharedMemory: "32Gi", EphemeralStorage: DefaultEphemeralStorage},
			},
		},
		{
			name: "SharedMemory and EphemeralStorage get defaults",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 1, CPU: "8", Memory: "100Gi"},
					},
				},
			},
			workspace:       nil,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "8", Memory: "100Gi", SharedMemory: "50Gi", EphemeralStorage: DefaultEphemeralStorage},
			},
		},
		{
			name: "SharedMemory and EphemeralStorage not overwritten if set",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 1, CPU: "8", Memory: "64Gi", SharedMemory: "16Gi", EphemeralStorage: "200Gi"},
					},
				},
			},
			workspace:       nil,
			expectedChanged: false,
			expectedResources: []v1.WorkloadResource{
				{Replica: 1, CPU: "8", Memory: "64Gi", SharedMemory: "16Gi", EphemeralStorage: "200Gi"},
			},
		},
		{
			name: "Multiple resources with mixed scenarios",
			workload: &v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{
						{Replica: 0, CPU: "4", Memory: "32Gi"},                                                            // filtered out
						{Replica: 2, CPU: "8", GPU: "4", Memory: "64Gi"},                                                  // GPU + defaults
						{Replica: 1, CPU: "16", GPU: "0", Memory: "128Gi", SharedMemory: "64Gi", EphemeralStorage: "1Ti"}, // GPU=0 cleared
					},
				},
			},
			workspace:       workspaceWithGpu,
			expectedChanged: true,
			expectedResources: []v1.WorkloadResource{
				{Replica: 2, CPU: "8", GPU: "4", GPUName: gpuResourceName, Memory: "64Gi", SharedMemory: "32Gi", EphemeralStorage: DefaultEphemeralStorage},
				{Replica: 1, CPU: "16", GPU: "", Memory: "128Gi", SharedMemory: "64Gi", EphemeralStorage: "1Ti"},
			},
		},
	}

	var mutator WorkloadMutator
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := mutator.mutateResources(tt.workload, tt.workspace)
			assert.Equal(t, tt.expectedChanged, changed, "isChanged mismatch")
			assert.Equal(t, len(tt.expectedResources), len(tt.workload.Spec.Resources), "resources count mismatch")
			for i, expected := range tt.expectedResources {
				actual := tt.workload.Spec.Resources[i]
				assert.Equal(t, expected.Replica, actual.Replica, "Replica mismatch at index %d", i)
				assert.Equal(t, expected.CPU, actual.CPU, "CPU mismatch at index %d", i)
				assert.Equal(t, expected.GPU, actual.GPU, "GPU mismatch at index %d", i)
				assert.Equal(t, expected.GPUName, actual.GPUName, "GPUName mismatch at index %d", i)
				assert.Equal(t, expected.Memory, actual.Memory, "Memory mismatch at index %d", i)
				assert.Equal(t, expected.SharedMemory, actual.SharedMemory, "SharedMemory mismatch at index %d", i)
				assert.Equal(t, expected.EphemeralStorage, actual.EphemeralStorage, "EphemeralStorage mismatch at index %d", i)
			}
		})
	}
}
