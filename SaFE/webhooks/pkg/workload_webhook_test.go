/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package webhooks

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
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
			mutator.mutateResources(context.Background(), tt.workload, tt.workspace)
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

// TestMutateCICDScaleSet_GPURunnerPassesValidation is the regression test for the bug where
// GPU-backed CICD AutoscalingRunnerSet workloads were rejected with a misleading
// "workspace has no GPU resources" error. The UI sends a CPU-only fixed spec.resources[0]
// alongside the user's actual GPU request encoded in env["RESOURCES"]; the mutator must
// inject gpuName into env["RESOURCES"] so the CICD validator's resource check passes.
func TestMutateCICDScaleSet_GPURunnerPassesValidation(t *testing.T) {
	const (
		gpuResourceName = "amd.com/gpu"
		workspaceName   = "ws-1"
	)

	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: workspaceName,
			Annotations: map[string]string{
				v1.GpuResourceNameAnnotation: gpuResourceName,
			},
		},
	}

	userResource := v1.WorkloadResource{
		Replica:          1,
		CPU:              "16",
		GPU:              "8",
		Memory:           "500Gi",
		SharedMemory:     "250Gi",
		EphemeralStorage: "1000Gi",
	}
	resourcesEnvJSON, err := json.Marshal(userResource)
	assert.NilError(t, err)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cicd-1",
		},
		Spec: v1.WorkloadSpec{
			Workspace:        workspaceName,
			GroupVersionKind: v1.GroupVersionKind{Kind: common.CICDScaleRunnerSetKind},
			Resources: []v1.WorkloadResource{
				{Replica: 1, CPU: "1", GPU: "0", Memory: "4Gi", EphemeralStorage: "10Gi"},
			},
			Env: map[string]string{
				ResourcesEnv:           string(resourcesEnvJSON),
				ImageEnv:               "ghcr.io/example/runner:latest",
				EntrypointEnv:          "ZXhlYyAuL3J1bi5zaA==",
				common.GithubConfigUrl: "https://github.com/example/repo",
			},
		},
	}

	var mutator WorkloadMutator
	var validator WorkloadValidator

	mutator.mutateCICDScaleSet(workload, workspace)
	validateErr := validator.validateCICDScalingRunnerSet(workload)

	mutated := &v1.WorkloadResource{}
	assert.NilError(t, json.Unmarshal([]byte(workload.Spec.Env[ResourcesEnv]), mutated),
		"env.RESOURCES should still be valid JSON after mutation")
	assert.Equal(t, gpuResourceName, mutated.GPUName,
		"mutator should inject the workspace's GPU resource name into env.RESOURCES")
	assert.Equal(t, "8", mutated.GPU, "GPU count must be preserved through mutation")
	assert.NilError(t, validateErr,
		"a GPU-backed CICD runner should pass admission after mutation")
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
				v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				v1.NodesAffinityAnnotation:        common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
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
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), "")
}

func TestMutateStickyNodes_UnsupportedKind(t *testing.T) {
	ctx := context.TODO()
	scheme := newTestScheme(t)

	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "w1",
			Annotations: map[string]string{
				v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				v1.NodesAffinityAnnotation:        common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
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
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), "")
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
				v1.RetryOnOriginalNodesAnnotation: v1.TrueStr,
				v1.NodesAffinityAnnotation:        common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
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
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), "")
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
				v1.NodesAffinityAnnotation: common.NodesAffinityRequired,
			},
		},
		Spec: v1.WorkloadSpec{
			MaxRetry:         1,
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
	assert.Equal(t, v1.GetAnnotation(workload, v1.RetryOnOriginalNodesAnnotation), v1.TrueStr)
}

func TestValidateResourceEnough_CpuFlavorWithGpuRequest(t *testing.T) {
	// NodeFlavor: CPU-only (no GPU)
	nf := &v1.NodeFlavor{
		ObjectMeta: metav1.ObjectMeta{Name: "amd-cpu"},
		Spec: v1.NodeFlavorSpec{
			Cpu:    v1.CpuChip{Quantity: resource.MustParse("32")},
			Memory: resource.MustParse("256Gi"),
			ExtendResources: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourceEphemeralStorage: resource.MustParse("990Gi"),
			},
		},
	}

	tests := []struct {
		name    string
		res     *v1.WorkloadResource
		wantErr bool
	}{
		{
			name: "gpu request on cpu-only flavor should fail",
			res: &v1.WorkloadResource{
				CPU:              "1",
				GPU:              "1",
				GPUName:          "amd.com/gpu",
				Memory:           "2Gi",
				SharedMemory:     "1Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: true,
		},
		{
			name: "cpu-only request on cpu flavor should pass",
			res: &v1.WorkloadResource{
				CPU:              "1",
				Memory:           "2Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: false,
		},
		{
			name: "cpu request exceeding flavor should fail",
			res: &v1.WorkloadResource{
				CPU:              "64",
				Memory:           "2Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: true,
		},
		{
			name: "memory request exceeding flavor should fail",
			res: &v1.WorkloadResource{
				CPU:              "1",
				Memory:           "512Gi",
				EphemeralStorage: "3Gi",
				Replica:          1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResourceEnough(nf, tt.res)
			if tt.wantErr {
				assert.Assert(t, err != nil, "expected error but got nil")
			} else {
				assert.NilError(t, err)
			}
		})
	}
}
