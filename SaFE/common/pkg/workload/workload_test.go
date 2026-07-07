/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
	"reflect"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
)

func genMockWorkload(clusterName, workspace string) *v1.Workload {
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("workload"),
			Labels: map[string]string{
				v1.ClusterIdLabel:   clusterName,
				v1.WorkspaceIdLabel: workspace,
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
			GroupVersionKind: v1.GroupVersionKind{
				Group:   "kubeflow.org",
				Kind:    common.PytorchJobKind,
				Version: "v1",
			},
			Resources: []v1.WorkloadResource{{
				Replica: 2,
				CPU:     "64",
				Memory:  "1024Gi",
				GPU:     "8",
				GPUName: common.AmdGpu,
			}},
		},
	}
	return workload
}

func TestGetWorkloadsOfWorkspace(t *testing.T) {
	workload1 := genMockWorkload("cluster1", "workspace1")
	workload1.Labels["key"] = "val"
	workload2 := genMockWorkload("cluster1", "workspace2")
	workload3 := genMockWorkload("cluster2", "workspace1")
	cli := fake.NewClientBuilder().WithObjects(workload1, workload2, workload3).WithScheme(scheme.Scheme).Build()

	result, err := GetWorkloadsOfWorkspace(context.Background(), cli, "cluster1", []string{"workspace1"}, nil)
	assert.NilError(t, err)
	assert.Equal(t, len(result) == 1, true)
	assert.Equal(t, result[0].Name == workload1.Name, true)

	filter := func(w *v1.Workload) bool {
		if w.GetLabels()["key"] == "val" {
			return true
		}
		return false
	}
	result, err = GetWorkloadsOfWorkspace(context.Background(), cli, "cluster1", []string{"workspace1"}, filter)
	assert.NilError(t, err)
	assert.Assert(t, len(result) == 0)
}

func TestConvertToPodResource(t *testing.T) {
	tests := []struct {
		name     string
		workload *v1.Workload
		gotError bool
	}{
		{
			"success",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{{
						CPU:     "64",
						Memory:  "100Mi",
						GPU:     "1",
						GPUName: common.AmdGpu,
					}},
				},
			},
			false,
		},
		{
			"Invalid cpu",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{{
						Replica: 1,
						CPU:     "-64",
						Memory:  "100Ki",
					}},
				},
			},
			true,
		},
		{
			"Invalid memory",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{{
						Replica: 1,
						CPU:     "64",
						Memory:  "1000abc",
					}},
				},
			},
			true,
		},
		{
			"Invalid gpu",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resources: []v1.WorkloadResource{{
						Replica: 2,
						CPU:     "10",
						Memory:  "10Mi",
						GPU:     "-1",
						GPUName: common.AmdGpu,
					}},
				},
			},
			true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := GetPodResourceList(&test.workload.Spec.Resources[0])
			assert.Equal(t, err != nil, test.gotError)
		})
	}
}

func TestGetWorkloadTemplate(t *testing.T) {
	mockScheme := runtime.NewScheme()
	err := v1.AddToScheme(mockScheme)
	assert.NilError(t, err)
	err = corev1.AddToScheme(mockScheme)
	assert.NilError(t, err)

	configmap1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configmap1",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.WorkloadVersionLabel: "v1", v1.WorkloadKindLabel: "kind1"},
		},
	}
	configmap2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "configmap2",
			Namespace: common.PrimusSafeNamespace,
			Labels:    map[string]string{v1.WorkloadVersionLabel: "v1", v1.WorkloadKindLabel: "kind2"},
		},
	}
	cli := fake.NewClientBuilder().WithObjects(configmap1, configmap2).WithScheme(mockScheme).Build()
	workload := &v1.Workload{
		Spec: v1.WorkloadSpec{
			GroupVersionKind: v1.GroupVersionKind{
				Kind:    "kind2",
				Version: "v1",
			},
		},
	}
	resp, err := GetWorkloadTemplate(context.Background(), cli, workload.ToSchemaGVK())
	assert.NilError(t, err)
	assert.Equal(t, resp.Name, configmap2.Name)
}

func TestGetResourcePerNode(t *testing.T) {
	workload := &v1.Workload{
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{{
				CPU:     "8",
				Memory:  "128",
				Replica: 3,
			}},
		},
		Status: v1.WorkloadStatus{
			Pods: []v1.WorkloadPod{
				{AdminNodeName: "n1"},
				{AdminNodeName: "n2"},
				{AdminNodeName: "n1"}},
		},
	}
	allResourcePerNode, err := GetResourcesPerNode(workload, "")
	assert.NilError(t, err)
	res, ok := allResourcePerNode["n1"]
	assert.Equal(t, ok, true)
	assert.Equal(t, quantity.Equal(res, corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(16, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(256, resource.BinarySI),
	}), true)

	res, ok = allResourcePerNode["n2"]
	assert.Equal(t, ok, true)
	assert.Equal(t, quantity.Equal(res, corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(8, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(128, resource.BinarySI),
	}), true)

	_, ok = allResourcePerNode["n3"]
	assert.Equal(t, ok, false)
}

func TestGetWorkloadResourceUsage(t *testing.T) {
	n1 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "n1",
			Labels: map[string]string{
				v1.ClusterIdLabel: "c1",
			},
		},
		Status: v1.NodeStatus{
			MachineStatus: v1.MachineStatus{
				Phase: v1.NodeReady,
			},
			ClusterStatus: v1.NodeClusterStatus{
				Phase: v1.NodeManaged,
			},
		},
	}
	n2 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "n2",
			Labels: map[string]string{
				v1.ClusterIdLabel: "c1",
			},
		},
		Status: v1.NodeStatus{
			MachineStatus: v1.MachineStatus{
				Phase: v1.NodeReady,
			},
			ClusterStatus: v1.NodeClusterStatus{
				Phase: v1.NodeManaged,
			},
		},
	}
	n3 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "n3",
		},
	}
	workload1 := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workload1",
			Labels: map[string]string{
				v1.ClusterIdLabel: "c1",
			},
		},
		Spec: v1.WorkloadSpec{
			Resources: []v1.WorkloadResource{{
				CPU:     "8",
				Memory:  "10",
				Replica: 3,
			}},
		},
		Status: v1.WorkloadStatus{
			Pods: []v1.WorkloadPod{
				{AdminNodeName: "n1", ResourceId: 0},
				{AdminNodeName: "n2", Phase: corev1.PodSucceeded, ResourceId: 0},
				{AdminNodeName: "n3", ResourceId: 0}},
		},
	}

	mockClient := fake.NewClientBuilder().WithObjects(n1, n2, n3, workload1).WithScheme(scheme.Scheme).Build()
	filterFunc := func(nodeName string) bool {
		n := &v1.Node{}
		if err := mockClient.Get(
			context.Background(), client.ObjectKey{Name: nodeName}, n); err != nil {
			return true
		}
		if !n.IsAvailable(false) {
			return true
		}
		return false
	}
	totalResource, availableResource, availableNodes, err := GetWorkloadResourceUsage(workload1, filterFunc)
	assert.NilError(t, err)
	assert.Equal(t, totalResource.Cpu().Value(), int64(16))
	assert.Equal(t, totalResource.Memory().Value(), int64(20))
	assert.Equal(t, availableResource.Cpu().Value(), int64(8))
	assert.Equal(t, availableResource.Memory().Value(), int64(10))
	assert.Equal(t, len(availableNodes), 1)
	assert.Equal(t, availableNodes[0], "n1")
}

func TestIsResourceEqual(t *testing.T) {
	workload1 := genMockWorkload("cluster1", "workspace1")
	workload2 := genMockWorkload("cluster1", "workspace2")
	resp := IsResourceEqual(workload1, workload2)
	assert.Equal(t, resp, true)

	workload2.Spec.Resources[0].CPU = "256"
	resp = IsResourceEqual(workload1, workload2)
	assert.Equal(t, resp, false)
}

func TestGeneral(t *testing.T) {
	workload := genMockWorkload("cluster1", "workspace1")
	assert.Equal(t, GetScope(workload), v1.TrainScope)
	assert.Equal(t, IsApplication(workload), false)
}

func TestMigrateResourceToResources(t *testing.T) {
	tests := []struct {
		name             string
		resource         v1.WorkloadResource
		kind             string
		expectedLen      int
		expectedReplicas []int
	}{
		{
			name: "PyTorchJob with Replica=1 creates single element array",
			resource: v1.WorkloadResource{
				Replica: 1,
				CPU:     "8",
				GPU:     "2",
				Memory:  "64Gi",
			},
			kind:             common.PytorchJobKind,
			expectedLen:      1,
			expectedReplicas: []int{1},
		},
		{
			name: "PyTorchJob with Replica=2 creates two element array",
			resource: v1.WorkloadResource{
				Replica: 2,
				CPU:     "8",
				GPU:     "2",
				Memory:  "64Gi",
			},
			kind:             common.PytorchJobKind,
			expectedLen:      2,
			expectedReplicas: []int{1, 1},
		},
		{
			name: "PyTorchJob with Replica=3 creates two element array with correct replicas",
			resource: v1.WorkloadResource{
				Replica: 3,
				CPU:     "8",
				GPU:     "2",
				Memory:  "64Gi",
			},
			kind:             common.PytorchJobKind,
			expectedLen:      2,
			expectedReplicas: []int{1, 2},
		},
		{
			name: "Deployment keeps original Replica (single element)",
			resource: v1.WorkloadResource{
				Replica: 3,
				CPU:     "8",
				GPU:     "2",
				Memory:  "64Gi",
			},
			kind:             common.DeploymentKind,
			expectedLen:      1,
			expectedReplicas: []int{3},
		},
		{
			name: "PyTorchJob with Replica=0 creates single element array",
			resource: v1.WorkloadResource{
				Replica: 0,
				CPU:     "8",
				Memory:  "64Gi",
			},
			kind:        common.PytorchJobKind,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertResourceToList(tt.resource, tt.kind)

			// Check length
			assert.Equal(t, len(result), tt.expectedLen, "resources count mismatch")

			// Check replicas
			for i, expectedReplica := range tt.expectedReplicas {
				assert.Equal(t, result[i].Replica, expectedReplica, "Replica mismatch at index %d", i)
			}

			// Check other fields are preserved
			for i := range result {
				assert.Equal(t, result[i].CPU, tt.resource.CPU, "CPU mismatch at index %d", i)
				assert.Equal(t, result[i].GPU, tt.resource.GPU, "GPU mismatch at index %d", i)
				assert.Equal(t, result[i].Memory, tt.resource.Memory, "Memory mismatch at index %d", i)
			}
		})
	}
}

func wlKind(kind string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: "w", Annotations: map[string]string{}, Labels: map[string]string{}}}
	w.Spec.GroupVersionKind.Kind = kind
	return w
}

func TestGetScope(t *testing.T) {
	assert.Equal(t, GetScope(wlKind(common.PytorchJobKind)), v1.TrainScope)
	assert.Equal(t, GetScope(wlKind(common.DeploymentKind)), v1.InferScope)
	assert.Equal(t, GetScope(wlKind(common.AuthoringKind)), v1.AuthoringScope)
	assert.Equal(t, GetScope(wlKind(common.CICDScaleRunnerSetKind)), v1.CICDScope)
	assert.Equal(t, GetScope(wlKind(common.RayJobKind)), v1.RayScope)
	assert.Equal(t, GetScope(wlKind(common.SandboxKind)), v1.SandboxScope)
	assert.Equal(t, GetScope(wlKind("Unknown")), v1.WorkspaceScope(""))
}

func TestKindPredicates(t *testing.T) {
	assert.Assert(t, IsApplication(wlKind(common.DeploymentKind)))
	assert.Assert(t, IsApplication(wlKind(common.StatefulSetKind)))
	assert.Assert(t, !IsApplication(wlKind(common.JobKind)))
	assert.Assert(t, IsAuthoring(wlKind(common.AuthoringKind)))
	assert.Assert(t, !IsAuthoring(wlKind(common.JobKind)))
	assert.Assert(t, IsCICD(wlKind(common.CICDScaleRunnerSetKind)))
	assert.Assert(t, IsCICD(wlKind(common.CICDEphemeralRunnerKind)))
	assert.Assert(t, !IsCICD(wlKind(common.JobKind)))
	assert.Assert(t, IsCICDScalingRunnerSet(wlKind(common.CICDScaleRunnerSetKind)))
	assert.Assert(t, !IsCICDScalingRunnerSet(wlKind(common.JobKind)))
	assert.Assert(t, IsCICDEphemeralRunner(wlKind(common.CICDEphemeralRunnerKind)))
	assert.Assert(t, !IsCICDEphemeralRunner(wlKind(common.JobKind)))
	assert.Assert(t, IsTorchFT(wlKind(common.TorchFTKind)))
	assert.Assert(t, !IsTorchFT(wlKind(common.JobKind)))
	assert.Assert(t, IsRayJob(wlKind(common.RayJobKind)))
	assert.Assert(t, !IsRayJob(wlKind(common.JobKind)))
	assert.Assert(t, IsDynamoDeployment(wlKind(common.DynamoDeploymentKind)))
	assert.Assert(t, IsInferaDeployment(wlKind(common.InferaDeploymentKind)))
	assert.Assert(t, IsMonarchJob(wlKind(common.MonarchJob)))
	assert.Assert(t, !IsMonarchJob(wlKind(common.JobKind)))
	assert.Assert(t, IsMonarchMesh(wlKind(common.MonarchMesh)))
	assert.Assert(t, !IsMonarchMesh(wlKind(common.JobKind)))
	assert.Assert(t, IsSandBox(wlKind(common.SandboxKind)))
	assert.Assert(t, !IsSandBox(wlKind(common.JobKind)))
}

func TestIsOpsJob(t *testing.T) {
	w := wlKind(common.JobKind)
	assert.Assert(t, !IsOpsJob(w))
	v1.SetLabel(w, v1.OpsJobIdLabel, "job-1")
	assert.Assert(t, IsOpsJob(w))
}

func TestDynamoHelpers(t *testing.T) {
	// non-dynamo -> nil roles
	assert.Assert(t, GetDynamoServiceRoles(wlKind(common.JobKind)) == nil)

	d := wlKind(common.DynamoDeploymentKind)
	// annotation-driven roles
	v1.SetAnnotation(d, v1.DynamoServiceRolesAnnotation, "frontend, worker , planner")
	assert.Assert(t, reflect.DeepEqual(GetDynamoServiceRoles(d), []string{"frontend", "worker", "planner"}))

	// fallback by resource count
	d2 := wlKind(common.DynamoDeploymentKind)
	d2.Spec.Resources = []v1.WorkloadResource{{}, {}}
	assert.Equal(t, len(GetDynamoServiceRoles(d2)), 2)
	d3 := wlKind(common.DynamoDeploymentKind)
	d3.Spec.Resources = []v1.WorkloadResource{{}, {}, {}}
	assert.Equal(t, len(GetDynamoServiceRoles(d3)), 3)
	d4 := wlKind(common.DynamoDeploymentKind)
	d4.Spec.Resources = []v1.WorkloadResource{{}}
	assert.Assert(t, GetDynamoServiceRoles(d4) == nil)

	// backends / frameworks default + set
	assert.Equal(t, GetDynamoKVTransferBackend(d2), common.DynamoDefaultKVBackend)
	v1.SetAnnotation(d, v1.DynamoKVTransferBackendAnnotation, "mori")
	assert.Equal(t, GetDynamoKVTransferBackend(d), "mori")
	assert.Equal(t, GetDynamoBackendFramework(d2), common.DynamoDefaultBackendFramework)
	v1.SetAnnotation(d, v1.DynamoBackendFrameworkAnnotation, "vllm")
	assert.Equal(t, GetDynamoBackendFramework(d), "vllm")

	// multinode roles
	assert.Assert(t, GetDynamoMultinodeRoles(d2) == nil)
	v1.SetAnnotation(d, v1.DynamoMultinodeRolesAnnotation, "worker")
	assert.Assert(t, reflect.DeepEqual(GetDynamoMultinodeRoles(d), []string{"worker"}))
	assert.Assert(t, IsDynamoMultinodeRole(d, "worker"))
	assert.Assert(t, !IsDynamoMultinodeRole(d, "frontend"))
}

func TestInferaHelpers(t *testing.T) {
	assert.Assert(t, GetInferaServiceRoles(wlKind(common.JobKind)) == nil)
	o := wlKind(common.InferaDeploymentKind)
	v1.SetAnnotation(o, v1.InferaServiceRolesAnnotation, "frontend,prefill,decode")
	assert.Equal(t, len(GetInferaServiceRoles(o)), 3)

	o2 := wlKind(common.InferaDeploymentKind)
	o2.Spec.Resources = []v1.WorkloadResource{{}, {}}
	assert.Equal(t, len(GetInferaServiceRoles(o2)), 2)
	o3 := wlKind(common.InferaDeploymentKind)
	o3.Spec.Resources = []v1.WorkloadResource{{}, {}, {}}
	assert.Equal(t, len(GetInferaServiceRoles(o3)), 3)

	assert.Equal(t, GetInferaKVTransferBackend(o2), common.InferaDefaultKVBackend)
	v1.SetAnnotation(o, v1.InferaKVTransferBackendAnnotation, "nixl")
	assert.Equal(t, GetInferaKVTransferBackend(o), "nixl")
	assert.Equal(t, GetInferaBackendFramework(o2), common.InferaDefaultBackendFramework)
	v1.SetAnnotation(o, v1.InferaBackendFrameworkAnnotation, "sglang")
	assert.Equal(t, GetInferaBackendFramework(o), "sglang")

	assert.Assert(t, GetInferaMultinodeRoles(o2) == nil)
	v1.SetAnnotation(o, v1.InferaMultinodeRolesAnnotation, "decode")
	assert.Assert(t, IsInferaMultinodeRole(o, "decode"))
	assert.Assert(t, !IsInferaMultinodeRole(o, "frontend"))
}

func TestGeneratePriorityAndReason(t *testing.T) {
	assert.Equal(t, GenerateDispatchReason(3), "run_3_times")
	assert.Equal(t, GeneratePriority(common.HighPriorityInt), common.HighPriority)
	assert.Equal(t, GeneratePriority(common.MedPriorityInt), common.MedPriority)
	assert.Equal(t, GeneratePriority(-100), common.LowPriority)
	assert.Assert(t, GeneratePriorityClass(wlKind(common.JobKind)) != "")
}
