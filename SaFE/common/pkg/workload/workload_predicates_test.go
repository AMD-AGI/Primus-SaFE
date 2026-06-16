/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func wlKind(kind string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w", Annotations: map[string]string{}, Labels: map[string]string{}}}
	w.Spec.GroupVersionKind.Kind = kind
	return w
}

func TestGetScope(t *testing.T) {
	assert.Equal(t, v1.TrainScope, GetScope(wlKind(common.PytorchJobKind)))
	assert.Equal(t, v1.InferScope, GetScope(wlKind(common.DeploymentKind)))
	assert.Equal(t, v1.AuthoringScope, GetScope(wlKind(common.AuthoringKind)))
	assert.Equal(t, v1.CICDScope, GetScope(wlKind(common.CICDScaleRunnerSetKind)))
	assert.Equal(t, v1.RayScope, GetScope(wlKind(common.RayJobKind)))
	assert.Equal(t, v1.SandboxScope, GetScope(wlKind(common.SandboxKind)))
	assert.Equal(t, v1.WorkspaceScope(""), GetScope(wlKind("Unknown")))
}

func TestKindPredicates(t *testing.T) {
	assert.True(t, IsApplication(wlKind(common.DeploymentKind)))
	assert.True(t, IsApplication(wlKind(common.StatefulSetKind)))
	assert.False(t, IsApplication(wlKind(common.JobKind)))
	assert.True(t, IsAuthoring(wlKind(common.AuthoringKind)))
	assert.False(t, IsAuthoring(wlKind(common.JobKind)))
	assert.True(t, IsCICD(wlKind(common.CICDScaleRunnerSetKind)))
	assert.True(t, IsCICD(wlKind(common.CICDEphemeralRunnerKind)))
	assert.False(t, IsCICD(wlKind(common.JobKind)))
	assert.True(t, IsCICDScalingRunnerSet(wlKind(common.CICDScaleRunnerSetKind)))
	assert.False(t, IsCICDScalingRunnerSet(wlKind(common.JobKind)))
	assert.True(t, IsCICDEphemeralRunner(wlKind(common.CICDEphemeralRunnerKind)))
	assert.False(t, IsCICDEphemeralRunner(wlKind(common.JobKind)))
	assert.True(t, IsTorchFT(wlKind(common.TorchFTKind)))
	assert.False(t, IsTorchFT(wlKind(common.JobKind)))
	assert.True(t, IsRayJob(wlKind(common.RayJobKind)))
	assert.False(t, IsRayJob(wlKind(common.JobKind)))
	assert.True(t, IsDynamoDeployment(wlKind(common.DynamoDeploymentKind)))
	assert.True(t, IsOptimusDeployment(wlKind(common.OptimusDeploymentKind)))
	assert.True(t, IsMonarchJob(wlKind(common.MonarchJob)))
	assert.False(t, IsMonarchJob(wlKind(common.JobKind)))
	assert.True(t, IsMonarchMesh(wlKind(common.MonarchMesh)))
	assert.False(t, IsMonarchMesh(wlKind(common.JobKind)))
	assert.True(t, IsSandBox(wlKind(common.SandboxKind)))
	assert.False(t, IsSandBox(wlKind(common.JobKind)))
}

func TestIsOpsJob(t *testing.T) {
	w := wlKind(common.JobKind)
	assert.False(t, IsOpsJob(w))
	v1.SetLabel(w, v1.OpsJobIdLabel, "job-1")
	assert.True(t, IsOpsJob(w))
}

func TestDynamoHelpers(t *testing.T) {
	// non-dynamo -> nil roles
	assert.Nil(t, GetDynamoServiceRoles(wlKind(common.JobKind)))

	d := wlKind(common.DynamoDeploymentKind)
	// annotation-driven roles
	v1.SetAnnotation(d, v1.DynamoServiceRolesAnnotation, "frontend, worker , planner")
	assert.Equal(t, []string{"frontend", "worker", "planner"}, GetDynamoServiceRoles(d))

	// fallback by resource count
	d2 := wlKind(common.DynamoDeploymentKind)
	d2.Spec.Resources = []v1.WorkloadResource{{}, {}}
	assert.Len(t, GetDynamoServiceRoles(d2), 2)
	d3 := wlKind(common.DynamoDeploymentKind)
	d3.Spec.Resources = []v1.WorkloadResource{{}, {}, {}}
	assert.Len(t, GetDynamoServiceRoles(d3), 3)
	d4 := wlKind(common.DynamoDeploymentKind)
	d4.Spec.Resources = []v1.WorkloadResource{{}}
	assert.Nil(t, GetDynamoServiceRoles(d4))

	// backends / frameworks default + set
	assert.Equal(t, common.DynamoDefaultKVBackend, GetDynamoKVTransferBackend(d2))
	v1.SetAnnotation(d, v1.DynamoKVTransferBackendAnnotation, "mori")
	assert.Equal(t, "mori", GetDynamoKVTransferBackend(d))
	assert.Equal(t, common.DynamoDefaultBackendFramework, GetDynamoBackendFramework(d2))
	v1.SetAnnotation(d, v1.DynamoBackendFrameworkAnnotation, "vllm")
	assert.Equal(t, "vllm", GetDynamoBackendFramework(d))

	// multinode roles
	assert.Nil(t, GetDynamoMultinodeRoles(d2))
	v1.SetAnnotation(d, v1.DynamoMultinodeRolesAnnotation, "worker")
	assert.Equal(t, []string{"worker"}, GetDynamoMultinodeRoles(d))
	assert.True(t, IsDynamoMultinodeRole(d, "worker"))
	assert.False(t, IsDynamoMultinodeRole(d, "frontend"))
}

func TestOptimusHelpers(t *testing.T) {
	assert.Nil(t, GetOptimusServiceRoles(wlKind(common.JobKind)))
	o := wlKind(common.OptimusDeploymentKind)
	v1.SetAnnotation(o, v1.OptimusServiceRolesAnnotation, "frontend,prefill,decode")
	assert.Len(t, GetOptimusServiceRoles(o), 3)

	o2 := wlKind(common.OptimusDeploymentKind)
	o2.Spec.Resources = []v1.WorkloadResource{{}, {}}
	assert.Len(t, GetOptimusServiceRoles(o2), 2)
	o3 := wlKind(common.OptimusDeploymentKind)
	o3.Spec.Resources = []v1.WorkloadResource{{}, {}, {}}
	assert.Len(t, GetOptimusServiceRoles(o3), 3)

	assert.Equal(t, common.OptimusDefaultKVBackend, GetOptimusKVTransferBackend(o2))
	v1.SetAnnotation(o, v1.OptimusKVTransferBackendAnnotation, "nixl")
	assert.Equal(t, "nixl", GetOptimusKVTransferBackend(o))
	assert.Equal(t, common.OptimusDefaultBackendFramework, GetOptimusBackendFramework(o2))
	v1.SetAnnotation(o, v1.OptimusBackendFrameworkAnnotation, "sglang")
	assert.Equal(t, "sglang", GetOptimusBackendFramework(o))

	assert.Nil(t, GetOptimusMultinodeRoles(o2))
	v1.SetAnnotation(o, v1.OptimusMultinodeRolesAnnotation, "decode")
	assert.True(t, IsOptimusMultinodeRole(o, "decode"))
	assert.False(t, IsOptimusMultinodeRole(o, "frontend"))
}

func TestGeneratePriorityAndReason(t *testing.T) {
	assert.Equal(t, "run_3_times", GenerateDispatchReason(3))
	assert.Equal(t, common.HighPriority, GeneratePriority(common.HighPriorityInt))
	assert.Equal(t, common.MedPriority, GeneratePriority(common.MedPriorityInt))
	assert.Equal(t, common.LowPriority, GeneratePriority(-100))
	assert.NotEmpty(t, GeneratePriorityClass(wlKind(common.JobKind)))
}
