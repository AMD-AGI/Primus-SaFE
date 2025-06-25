/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"context"
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
			Resource: v1.WorkloadResource{
				Replica: 2,
				CPU:     "64",
				Memory:  "1024Gi",
				GPU:     "8",
				GPUName: common.AmdGpu,
			},
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

func TestCvtToResourceList(t *testing.T) {
	tests := []struct {
		name     string
		workload *v1.Workload
		gotError bool
	}{
		{
			"success",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resource: v1.WorkloadResource{
						CPU:     "64",
						Memory:  "100Mi",
						GPU:     "1",
						GPUName: common.AmdGpu,
					},
				},
			},
			false,
		},
		{
			"Invalid cpu",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resource: v1.WorkloadResource{
						Replica: 1,
						CPU:     "-64",
						Memory:  "100Ki",
					},
				},
			},
			true,
		},
		{
			"Invalid memory",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resource: v1.WorkloadResource{
						Replica: 1,
						CPU:     "64",
						Memory:  "1000abc",
					},
				},
			},
			true,
		},
		{
			"Invalid gpu",
			&v1.Workload{
				Spec: v1.WorkloadSpec{
					Resource: v1.WorkloadResource{
						Replica: 2,
						CPU:     "10",
						Memory:  "10Mi",
						GPU:     "-1",
						GPUName: common.AmdGpu,
					},
				},
			},
			true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CvtToResourceList(test.workload)
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
	resp, err := GetWorkloadTemplate(context.Background(), cli, workload)
	assert.NilError(t, err)
	assert.Equal(t, resp.Name, configmap2.Name)
}

func TestGetResourcePerNode(t *testing.T) {
	workload := &v1.Workload{
		Spec: v1.WorkloadSpec{
			Resource: v1.WorkloadResource{
				CPU:     "8",
				Memory:  "128",
				Replica: 3,
			},
		},
		Status: v1.WorkloadStatus{
			Pods: []v1.WorkloadPod{
				{AdminNodeName: "n1", K8sNodeName: "n1"},
				{AdminNodeName: "n2", K8sNodeName: "n2"},
				{AdminNodeName: "n1", K8sNodeName: "n1"}},
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

func TestGetActiveResource(t *testing.T) {
	n1 := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "n1",
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
			Resource: v1.WorkloadResource{
				CPU:     "8",
				Memory:  "10",
				Replica: 3,
			},
		},
		Status: v1.WorkloadStatus{
			Pods: []v1.WorkloadPod{
				{AdminNodeName: "n1", K8sNodeName: "n1"},
				{AdminNodeName: "n2", K8sNodeName: "n2", Phase: corev1.PodSucceeded},
				{AdminNodeName: "n3", K8sNodeName: "n3"}},
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
	res, err := GetActiveResources(workload1, filterFunc)
	assert.NilError(t, err)
	assert.Equal(t, res.Cpu().Value(), int64(8))
	assert.Equal(t, res.Memory().Value(), int64(10))
}

func TestIsResourceEqual(t *testing.T) {
	workload1 := genMockWorkload("cluster1", "workspace1")
	workload2 := genMockWorkload("cluster1", "workspace2")
	resp := IsResourceEqual(workload1, workload2)
	assert.Equal(t, resp, true)

	workload2.Spec.Resource.CPU = "256"
	resp = IsResourceEqual(workload1, workload2)
	assert.Equal(t, resp, false)
}

func TestGeneral(t *testing.T) {
	workload := genMockWorkload("cluster1", "workspace1")
	assert.Equal(t, GetScope(workload), v1.TrainScope)
	assert.Equal(t, IsApplication(workload), false)
}
