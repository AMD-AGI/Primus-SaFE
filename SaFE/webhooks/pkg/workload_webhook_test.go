/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"testing"
	"time"

	"gotest.tools/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
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

func newTestScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	err := v1.AddToScheme(s)
	assert.NilError(t, err)
	return s
}

func TestMutateStickyNodes_EnablePreempt(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Annotations: map[string]string{
				v1.WorkloadStickyNodesAnnotation: v1.TrueStr,
			},
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: true,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should remove sticky nodes annotation when preempt is enabled
	assert.Equal(t, v1.GetAnnotation(workload, v1.WorkloadStickyNodesAnnotation), "")
}

func TestMutateStickyNodes_UnsupportedKind(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Annotations: map[string]string{
				v1.WorkloadStickyNodesAnnotation: v1.TrueStr,
			},
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: "Deployment"}, // unsupported kind
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: false,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should remove sticky nodes annotation for unsupported kind
	assert.Equal(t, v1.GetAnnotation(workload, v1.WorkloadStickyNodesAnnotation), "")
}

func TestMutateStickyNodes_GpuCountMismatch(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	nodeFlavor := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nf1",
		},
		Spec: v1.NodeFlavorSpec{
			Gpu: &v1.GpuChip{
				Quantity: resource.MustParse("8"),
			},
		},
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Labels: map[string]string{
				v1.NodeFlavorIdLabel: "nf1",
			},
			Annotations: map[string]string{
				v1.WorkloadStickyNodesAnnotation: v1.TrueStr,
			},
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
			Resources: []v1.WorkloadResource{
				{GPU: "4"}, // mismatch: 4 != 8
			},
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: false,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeFlavor).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should remove sticky nodes annotation when GPU count mismatch
	assert.Equal(t, v1.GetAnnotation(workload, v1.WorkloadStickyNodesAnnotation), "")
}

func TestMutateStickyNodes_AllConditionsPass(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	nodeFlavor := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{
			Name: "nf1",
		},
		Spec: v1.NodeFlavorSpec{
			Gpu: &v1.GpuChip{
				Quantity: resource.MustParse("8"),
			},
		},
	}

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Labels: map[string]string{
				v1.NodeFlavorIdLabel: "nf1",
			},
			Annotations: map[string]string{
				v1.WorkloadStickyNodesAnnotation: v1.TrueStr,
			},
		},
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{Kind: common.PytorchJobKind},
			Resources: []v1.WorkloadResource{
				{GPU: "8"}, // matches node flavor GPU count
			},
		},
	}

	workspace := &v1.Workspace{
		Spec: v1.WorkspaceSpec{
			EnablePreempt: false,
		},
	}

	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(nodeFlavor).Build()
	m := &WorkloadMutator{Client: k8sClient}

	m.mutateStickNodes(ctx, workload, workspace)

	// Should keep sticky nodes annotation when all conditions pass
	assert.Equal(t, v1.GetAnnotation(workload, v1.WorkloadStickyNodesAnnotation), v1.TrueStr)
}
