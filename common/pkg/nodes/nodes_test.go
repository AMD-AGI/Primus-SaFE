/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package nodes

import (
	"context"
	"sort"
	"testing"
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func TestNodeDeleteSort(t *testing.T) {
	tests := []struct {
		name   string
		n1     v1.Node
		n2     v1.Node
		result string
	}{
		{
			name: "test deleteTime",
			n1: v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			},
			n2: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "n2",
					DeletionTimestamp: &metav1.Time{Time: time.Now().UTC()},
				},
			},
			result: "n2",
		},
		{
			name: "test taint",
			n1: v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n2"},
				Status: v1.NodeStatus{
					Taints: []corev1.Taint{{
						Key: v1.PrimusSafePrefix + "001",
					}},
				},
			},
			n2: v1.Node{
				ObjectMeta: metav1.ObjectMeta{Name: "n1"},
			},
			result: "n1",
		},
		{
			name: "test creation time",
			n1: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "n1",
					CreationTimestamp: metav1.Time{Time: time.Now().UTC().Add(-time.Minute)},
				},
			},
			n2: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "n2",
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
			},
			result: "n2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nodes := []v1.Node{test.n1, test.n2}
			sort.Sort(NodeSlice(nodes))
			assert.Equal(t, nodes[0].Name, test.result)
		})
	}

	nodes := []v1.Node{{
		ObjectMeta: metav1.ObjectMeta{Name: "n1"},
	}, {
		ObjectMeta: metav1.ObjectMeta{Name: "n2"},
	}, {
		ObjectMeta: metav1.ObjectMeta{Name: "n3"},
	}}
	nodes = nodes[0:2]
	assert.Equal(t, len(nodes), 2)
	assert.Equal(t, nodes[0].Name, "n1")
	assert.Equal(t, nodes[1].Name, "n2")
}

func genPods() []*corev1.Pod {
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "10.10.0.0",
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: *resource.NewQuantity(16, resource.BinarySI),
						common.NvidiaGpu:   *resource.NewQuantity(8, resource.DecimalSI),
					},
				},
			}},
		},
	}
	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p2",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "10.10.0.1",
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: *resource.NewQuantity(32, resource.BinarySI),
						common.NvidiaGpu:   *resource.NewQuantity(16, resource.DecimalSI),
					},
				},
			}},
		},
	}
	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "p3",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "10.10.0.0",
			Containers: []corev1.Container{{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewQuantity(4096, resource.BinarySI),
						common.NvidiaGpu:      *resource.NewQuantity(8, resource.DecimalSI),
					},
				},
			}},
		},
	}
	return []*corev1.Pod{pod1, pod2, pod3}
}

func TestGetNodesLoad(t *testing.T) {
	podList := genPods()
	clientSet := fake.NewSimpleClientset(podList[0], podList[1], podList[2])
	loads, err := GetPodResources(context.Background(), clientSet, nil, corev1.NamespaceAll)
	assert.NilError(t, err)
	assert.Equal(t, len(loads), 2)
	q := loads["10.10.0.0"]
	assert.Equal(t, q.Cpu().Value(), int64(16))
	assert.Equal(t, q.Memory().Value(), int64(4096))
	gpu := q[corev1.ResourceName(common.NvidiaGpu)]
	assert.Equal(t, gpu.Value(), int64(16))

	q = loads["10.10.0.1"]
	assert.Equal(t, q.Cpu().Value(), int64(32))
	gpu = q[corev1.ResourceName(common.NvidiaGpu)]
	assert.Equal(t, gpu.Value(), int64(16))
}
