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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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

// TestFilterDeletingNode tests the FilterDeletingNode function
func TestFilterDeletingNode(t *testing.T) {
	now := metav1.NewTime(time.Now())

	tests := []struct {
		name     string
		node     v1.Node
		expected bool
	}{
		{
			name: "node with deletion timestamp",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "deleting-node",
					DeletionTimestamp: &now,
				},
			},
			expected: true,
		},
		{
			name: "normal node without deletion timestamp",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "normal-node",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterDeletingNode(tt.node)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestIsPodRunning tests the IsPodRunning function
func TestIsPodRunning(t *testing.T) {
	now := metav1.NewTime(time.Now())

	tests := []struct {
		name     string
		pod      corev1.Pod
		expected bool
	}{
		{
			name: "running pod",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod1"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			expected: true,
		},
		{
			name: "succeeded pod",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod2"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			},
			expected: false,
		},
		{
			name: "failed pod",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod3"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodFailed},
			},
			expected: false,
		},
		{
			name: "pod with deletion timestamp",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "pod4",
					DeletionTimestamp: &now,
				},
				Spec:   corev1.PodSpec{NodeName: "node1"},
				Status: corev1.PodStatus{Phase: corev1.PodRunning},
			},
			expected: false,
		},
		{
			name: "pod without node assignment",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod5"},
				Spec:       corev1.PodSpec{},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			expected: false,
		},
		{
			name: "pending pod with node",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod6"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodPending},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPodRunning(tt.pod)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestGetInternalIp tests the GetInternalIp function
func TestGetInternalIp(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		expected string
	}{
		{
			name: "node with internal IP",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{Type: corev1.NodeHostName, Address: "node1"},
						{Type: corev1.NodeInternalIP, Address: "192.168.1.100"},
						{Type: corev1.NodeExternalIP, Address: "8.8.8.8"},
					},
				},
			},
			expected: "192.168.1.100",
		},
		{
			name: "node without internal IP",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{Type: corev1.NodeHostName, Address: "node2"},
						{Type: corev1.NodeExternalIP, Address: "8.8.4.4"},
					},
				},
			},
			expected: "",
		},
		{
			name: "node with multiple IPs, internal IP first",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{Type: corev1.NodeInternalIP, Address: "10.0.0.1"},
						{Type: corev1.NodeExternalIP, Address: "1.2.3.4"},
					},
				},
			},
			expected: "10.0.0.1",
		},
		{
			name: "node with no addresses",
			node: &corev1.Node{
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{},
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetInternalIp(tt.node)
			assert.Equal(t, result, tt.expected)
		})
	}
}

// TestBuildAction tests the BuildAction function
func TestBuildAction(t *testing.T) {
	tests := []struct {
		name     string
		action   string
		keys     []string
		validate func(t *testing.T, result string)
	}{
		{
			name:   "single key",
			action: "delete",
			keys:   []string{"node1"},
			validate: func(t *testing.T, result string) {
				assert.Assert(t, len(result) > 0)
				assert.Assert(t, result != "")
				// Should be valid JSON containing the action
				assert.Assert(t, len(result) > len("{}"))
			},
		},
		{
			name:   "multiple keys",
			action: "scale",
			keys:   []string{"node1", "node2", "node3"},
			validate: func(t *testing.T, result string) {
				assert.Assert(t, len(result) > 0)
				// Result should be longer for more keys
				assert.Assert(t, len(result) > len("{}"))
			},
		},
		{
			name:   "empty keys",
			action: "test",
			keys:   []string{},
			validate: func(t *testing.T, result string) {
				assert.Equal(t, result, "{}")
			},
		},
		{
			name:   "action with special characters",
			action: "scale-down",
			keys:   []string{"node-1", "node-2"},
			validate: func(t *testing.T, result string) {
				assert.Assert(t, len(result) > 0)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildAction(tt.action, tt.keys...)
			tt.validate(t, result)
		})
	}
}

// TestNodes2PointerSlice tests the Nodes2PointerSlice function
func TestNodes2PointerSlice(t *testing.T) {
	tests := []struct {
		name  string
		nodes []v1.Node
	}{
		{
			name: "single node",
			nodes: []v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
			},
		},
		{
			name: "multiple nodes",
			nodes: []v1.Node{
				{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "node2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "node3"}},
			},
		},
		{
			name:  "empty slice",
			nodes: []v1.Node{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Nodes2PointerSlice(tt.nodes)
			assert.Equal(t, len(result), len(tt.nodes))

			// Verify each pointer points to the correct node
			for i, nodePtr := range result {
				assert.Assert(t, nodePtr != nil)
				assert.Equal(t, nodePtr.Name, tt.nodes[i].Name)
			}
		})
	}
}

// TestListPods tests the ListPods function
// Note: fake clientset has limited FieldSelector support, so we primarily test the nil nodeNames case
func TestListPods(t *testing.T) {
	t.Run("list all running pods in namespace", func(t *testing.T) {
		pods := []*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "running-pod", Namespace: "default"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "succeeded-pod", Namespace: "default"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pending-pod", Namespace: "default"},
				Spec:       corev1.PodSpec{NodeName: "node2"},
				Status:     corev1.PodStatus{Phase: corev1.PodPending},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "failed-pod", Namespace: "default"},
				Spec:       corev1.PodSpec{NodeName: "node2"},
				Status:     corev1.PodStatus{Phase: corev1.PodFailed},
			},
		}

		clientSet := fake.NewSimpleClientset()
		for _, pod := range pods {
			_, _ = clientSet.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
		}

		result, err := ListPods(context.Background(), clientSet, nil, "default")
		assert.NilError(t, err)
		assert.Equal(t, len(result), 2) // Only running and pending pods

		resultNames := make(map[string]bool)
		for _, pod := range result {
			resultNames[pod.Name] = true
		}
		assert.Assert(t, resultNames["running-pod"])
		assert.Assert(t, resultNames["pending-pod"])
		assert.Assert(t, !resultNames["succeeded-pod"])
		assert.Assert(t, !resultNames["failed-pod"])
	})

	t.Run("list pods with empty result", func(t *testing.T) {
		pods := []*corev1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "succeeded-pod", Namespace: "default"},
				Spec:       corev1.PodSpec{NodeName: "node1"},
				Status:     corev1.PodStatus{Phase: corev1.PodSucceeded},
			},
		}

		clientSet := fake.NewSimpleClientset()
		for _, pod := range pods {
			_, _ = clientSet.CoreV1().Pods(pod.Namespace).Create(context.Background(), pod, metav1.CreateOptions{})
		}

		result, err := ListPods(context.Background(), clientSet, nil, "default")
		assert.NilError(t, err)
		assert.Equal(t, len(result), 0) // No running pods
	})
}
