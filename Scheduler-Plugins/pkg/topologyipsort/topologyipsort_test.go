package topologyipsort

import (
	"math"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/scheduler-plugins/apis/scheduling/v1alpha1"
)

func TestTopologyIPSort_Name(t *testing.T) {
	plugin := &TopologyIPSort{}
	if got := plugin.Name(); got != Name {
		t.Errorf("TopologyIPSort.Name() = %v, want %v", got, Name)
	}
}

func TestGetIPIndex(t *testing.T) {
	tests := []struct {
		name     string
		nodeInfo *framework.NodeInfo
		want     int
	}{
		{
			name: "node with valid internal IP",
			nodeInfo: func() *framework.NodeInfo {
				nodeInfo := framework.NewNodeInfo()
				node := &corev1.Node{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "192.168.1.1",
							},
						},
					},
				}
				nodeInfo.SetNode(node)
				return nodeInfo
			}(),
			want: 3232235777, // 192.168.1.1 converted to int
		},
		{
			name: "node without internal IP",
			nodeInfo: func() *framework.NodeInfo {
				nodeInfo := framework.NewNodeInfo()
				node := &corev1.Node{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeExternalIP,
								Address: "10.0.0.1",
							},
						},
					},
				}
				nodeInfo.SetNode(node)
				return nodeInfo
			}(),
			want: 0,
		},
		{
			name: "node with invalid IP format",
			nodeInfo: func() *framework.NodeInfo {
				nodeInfo := framework.NewNodeInfo()
				node := &corev1.Node{
					Status: corev1.NodeStatus{
						Addresses: []corev1.NodeAddress{
							{
								Type:    corev1.NodeInternalIP,
								Address: "invalid-ip",
							},
						},
					},
				}
				nodeInfo.SetNode(node)
				return nodeInfo
			}(),
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getIPIndex(tt.nodeInfo); got != tt.want {
				t.Errorf("getIPIndex() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLess(t *testing.T) {
	tests := []struct {
		name string
		pod1 *corev1.Pod
		pod2 *corev1.Pod
		want *bool
	}{
		{
			name: "pods with different priorities",
			pod1: &corev1.Pod{
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(10),
				},
			},
			pod2: &corev1.Pod{
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(5),
				},
			},
			want: pointer.Bool(true), // higher priority should come first
		},
		{
			name: "pods with same priority, different pod groups",
			pod1: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1alpha1.PodGroupLabel: "group1",
					},
				},
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(5),
				},
			},
			pod2: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1alpha1.PodGroupLabel: "group2",
					},
				},
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(5),
				},
			},
			want: nil, // should return nil when pod groups are different
		},
		{
			name: "pods with same priority and pod group, different replica types",
			pod1: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1alpha1.PodGroupLabel: "group1",
						ReplicaTypeLabel:       ReplicaMaster,
					},
				},
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(5),
				},
			},
			pod2: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						v1alpha1.PodGroupLabel: "group1",
						ReplicaTypeLabel:       "worker",
					},
				},
				Spec: corev1.PodSpec{
					Priority: pointer.Int32(5),
				},
			},
			want: pointer.Bool(true), // master should come before worker
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := less(tt.pod1, tt.pod2)
			if tt.want == nil {
				if got != nil {
					t.Errorf("less() = %v, want nil", got)
				}
			} else {
				if got == nil || *got != *tt.want {
					t.Errorf("less() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestGetUnit(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want int
	}{
		{
			name: "pod without annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			want: 1, // default value when no annotations
		},
		{
			name: "pod with topology annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						TPCountAnnotation: "2",
						EPCountAnnotation: "3",
						CPCountAnnotation: "4",
						PPCountAnnotation: "5",
					},
				},
			},
			want: (2*3*4*5 + 7) / 8, // calculated unit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getUnit(tt.pod); got != tt.want {
				t.Errorf("getUnit() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPCount(t *testing.T) {
	tests := []struct {
		name string
		obj  metav1.Object
		key  string
		want int
	}{
		{
			name: "object without annotations",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			key:  "test-key",
			want: 1, // default value
		},
		{
			name: "object with valid annotation",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						"test-key": "5",
					},
				},
			},
			key:  "test-key",
			want: 5,
		},
		{
			name: "object with invalid annotation",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						"test-key": "invalid",
					},
				},
			},
			key:  "test-key",
			want: 1, // default value for invalid number
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPCount(tt.obj, tt.key); got != tt.want {
				t.Errorf("getPCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAtoi(t *testing.T) {
	tests := []struct {
		name string
		str  string
		want int
	}{
		{
			name: "valid number",
			str:  "123",
			want: 123,
		},
		{
			name: "empty string",
			str:  "",
			want: 0,
		},
		{
			name: "invalid number",
			str:  "abc",
			want: 0,
		},
		{
			name: "zero",
			str:  "0",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := atoi(tt.str); got != tt.want {
				t.Errorf("atoi() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAnnotation(t *testing.T) {
	tests := []struct {
		name string
		obj  metav1.Object
		key  string
		want string
	}{
		{
			name: "object without annotations",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
				},
			},
			key:  "test-key",
			want: "",
		},
		{
			name: "object with matching annotation",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						"test-key": "test-value",
					},
				},
			},
			key:  "test-key",
			want: "test-value",
		},
		{
			name: "object with non-matching annotation",
			obj: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-pod",
					Annotations: map[string]string{
						"other-key": "other-value",
					},
				},
			},
			key:  "test-key",
			want: "",
		},
		{
			name: "nil object",
			obj:  nil,
			key:  "test-key",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getAnnotation(tt.obj, tt.key); got != tt.want {
				t.Errorf("getAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test constants
func TestConstants(t *testing.T) {
	if Name != "TopologyIPSort" {
		t.Errorf("Name constant = %v, want TopologyIPSort", Name)
	}

	if TPCountAnnotation != "scheduling.x-k8s.io.tp" {
		t.Errorf("TPCountAnnotation = %v, want scheduling.x-k8s.io.tp", TPCountAnnotation)
	}

	if EPCountAnnotation != "scheduling.x-k8s.io.ep" {
		t.Errorf("EPCountAnnotation = %v, want scheduling.x-k8s.io.ep", EPCountAnnotation)
	}

	if CPCountAnnotation != "scheduling.x-k8s.io.cp" {
		t.Errorf("CPCountAnnotation = %v, want scheduling.x-k8s.io.cp", CPCountAnnotation)
	}

	if PPCountAnnotation != "scheduling.x-k8s.io.pp" {
		t.Errorf("PPCountAnnotation = %v, want scheduling.x-k8s.io.pp", PPCountAnnotation)
	}

	if ReplicaTypeLabel != "training.kubeflow.org/replica-type" {
		t.Errorf("ReplicaTypeLabel = %v, want training.kubeflow.org/replica-type", ReplicaTypeLabel)
	}

	if ReplicaIndexLabel != "training.kubeflow.org/replica-index" {
		t.Errorf("ReplicaIndexLabel = %v, want training.kubeflow.org/replica-index", ReplicaIndexLabel)
	}

	if ReplicaMaster != "master" {
		t.Errorf("ReplicaMaster = %v, want master", ReplicaMaster)
	}
}

func TestMinimizeVariance(t *testing.T) {
	tests := []struct {
		name      string
		rackNodes []rackNode
		count     int
		want      []node
	}{
		{
			name:      "empty rackNodes",
			rackNodes: []rackNode{},
			count:     5,
			want:      []node{},
		},
		{
			name: "zero count",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
					},
				},
			},
			count: 0,
			want:  []node{},
		},
		{
			name: "single rack allocation",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
						{name: "node3", ip: 3},
					},
				},
			},
			count: 2,
			want: []node{
				{name: "node1", ip: 1},
				{name: "node2", ip: 2},
			},
		},
		{
			name: "two racks equal allocation",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node3", ip: 3},
						{name: "node4", ip: 4},
					},
				},
			},
			count: 4,
			want: []node{
				{name: "node1", ip: 1},
				{name: "node2", ip: 2},
				{name: "node3", ip: 3},
				{name: "node4", ip: 4},
			},
		},
		{
			name: "three racks with variance minimization",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node3", ip: 3},
						{name: "node4", ip: 4},
						{name: "node5", ip: 5},
					},
				},
				{
					rack: "rack3",
					nodes: []node{
						{name: "node6", ip: 6},
					},
				},
			},
			count: 6,
			want: []node{
				{name: "node1", ip: 1},
				{name: "node2", ip: 2},
				{name: "node3", ip: 3},
				{name: "node4", ip: 4},
				{name: "node5", ip: 5},
				{name: "node6", ip: 6},
			},
		},
		{
			name: "insufficient nodes - should return empty",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node2", ip: 2},
					},
				},
			},
			count: 5,
			want:  []node{}, // Return empty when cannot allocate requested count
		},
		{
			name: "large count with multiple racks",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
						{name: "node3", ip: 3},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node4", ip: 4},
						{name: "node5", ip: 5},
					},
				},
				{
					rack: "rack3",
					nodes: []node{
						{name: "node6", ip: 6},
						{name: "node7", ip: 7},
						{name: "node8", ip: 8},
						{name: "node9", ip: 9},
					},
				},
			},
			count: 7,
			want: []node{
				{name: "node1", ip: 1},
				{name: "node2", ip: 2},
				{name: "node3", ip: 3},
				{name: "node4", ip: 4},
				{name: "node5", ip: 5},
				{name: "node6", ip: 6},
				{name: "node7", ip: 7},
			},
		},
		{
			name: "exact allocation possible",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node3", ip: 3},
					},
				},
			},
			count: 3,
			want: []node{
				{name: "node1", ip: 1},
				{name: "node2", ip: 2},
				{name: "node3", ip: 3},
			},
		},
		{
			name: "partial allocation not possible - should return empty",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node2", ip: 2},
					},
				},
			},
			count: 3,
			want:  []node{}, // Return empty when cannot allocate requested count
		},
		{
			name: "two racks with three nodes each, count 4",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
						{name: "node3", ip: 3},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node4", ip: 4},
						{name: "node5", ip: 5},
						{name: "node6", ip: 6},
					},
				},
			},
			count: 4,
			want: []node{
				{name: "node1", ip: 1},
				{name: "node2", ip: 2},
				{name: "node4", ip: 4},
				{name: "node5", ip: 5},
			},
		},
		{
			name: "three racks with two nodes each, count 4",
			rackNodes: []rackNode{
				{
					rack: "rack1",
					nodes: []node{
						{name: "node1", ip: 1},
						{name: "node2", ip: 2},
					},
				},
				{
					rack: "rack2",
					nodes: []node{
						{name: "node3", ip: 3},
						{name: "node4", ip: 4},
					},
				},
				{
					rack: "rack3",
					nodes: []node{
						{name: "node5", ip: 5},
						{name: "node6", ip: 6},
					},
				},
			},
			count: 4,
			want: []node{
				{name: "node1", ip: 1},
				{name: "node2", ip: 2},
				{name: "node3", ip: 3},
				{name: "node4", ip: 4},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := minimizeVariance(tt.rackNodes, tt.count)

			// Check if we got the expected number of nodes
			if len(got) != len(tt.want) {
				t.Errorf("minimizeVariance() returned %d nodes, want %d", len(got), len(tt.want))
			}

			// For cases where we expect specific nodes, verify them
			if len(tt.want) > 0 {
				for i, expectedNode := range tt.want {
					if i >= len(got) {
						t.Errorf("minimizeVariance() returned fewer nodes than expected")
						break
					}
					if got[i].name != expectedNode.name {
						t.Errorf("minimizeVariance() node[%d].name = %v, want %v", i, got[i].name, expectedNode.name)
					}
					if got[i].ip != expectedNode.ip {
						t.Errorf("minimizeVariance() node[%d].ip = %v, want %v", i, got[i].ip, expectedNode.ip)
					}
				}
			}
		})
	}
}

func TestFindMinVarianceAllocation(t *testing.T) {
	tests := []struct {
		name       string
		rackCounts []int
		totalCount int
		want       []int
	}{
		{
			name:       "empty rack counts",
			rackCounts: []int{},
			totalCount: 5,
			want:       []int{},
		},
		{
			name:       "zero total count",
			rackCounts: []int{3, 2, 4},
			totalCount: 0,
			want:       []int{0, 0, 0},
		},
		{
			name:       "single rack",
			rackCounts: []int{5},
			totalCount: 3,
			want:       []int{3},
		},
		{
			name:       "two racks equal allocation",
			rackCounts: []int{3, 3},
			totalCount: 4,
			want:       []int{2, 2},
		},
		{
			name:       "three racks with remainder",
			rackCounts: []int{4, 4, 4},
			totalCount: 7,
			want:       []int{4, 3, 0}, // New algorithm behavior
		},
		{
			name:       "limited capacity",
			rackCounts: []int{2, 1, 3},
			totalCount: 5,
			want:       []int{2, 1, 2},
		},
		{
			name:       "insufficient capacity",
			rackCounts: []int{1, 1},
			totalCount: 5,
			want:       []int{1, 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findMinVarianceAllocation(tt.rackCounts, tt.totalCount)

			// Check if we got the expected number of allocations
			if len(got) != len(tt.want) {
				t.Errorf("findMinVarianceAllocation() returned %d allocations, want %d", len(got), len(tt.want))
				return
			}

			// Check if the sum matches the total count (or is limited by capacity)
			sum := 0
			for _, alloc := range got {
				sum += alloc
			}

			// For empty rack counts, we expect 0 sum
			if len(tt.rackCounts) == 0 {
				if sum != 0 {
					t.Errorf("findMinVarianceAllocation() sum = %d, want 0 for empty rack counts", sum)
				}
				return
			}

			// For insufficient capacity, we expect the sum to be limited by available capacity
			if tt.name == "insufficient capacity" {
				maxPossible := 0
				for _, count := range tt.rackCounts {
					maxPossible += count
				}
				if sum != maxPossible {
					t.Errorf("findMinVarianceAllocation() sum = %d, want %d for insufficient capacity", sum, maxPossible)
				}
				return
			}

			// For other cases, check if the sum matches the total count
			if sum != tt.totalCount {
				t.Errorf("findMinVarianceAllocation() sum = %d, want %d", sum, tt.totalCount)
			}

			// Check individual allocations for non-empty cases
			if len(tt.rackCounts) > 0 {
				for i, expectedAlloc := range tt.want {
					if i < len(got) && got[i] != expectedAlloc {
						t.Errorf("findMinVarianceAllocation()[%d] = %d, want %d", i, got[i], expectedAlloc)
					}
				}
			}
		})
	}
}

func TestCalculateVariance(t *testing.T) {
	tests := []struct {
		name       string
		allocation []int
		want       float64
	}{
		{
			name:       "empty allocation",
			allocation: []int{},
			want:       0.0,
		},
		{
			name:       "all zeros",
			allocation: []int{0, 0, 0},
			want:       0.0,
		},
		{
			name:       "single value",
			allocation: []int{5},
			want:       0.0,
		},
		{
			name:       "two equal values",
			allocation: []int{3, 3},
			want:       0.0,
		},
		{
			name:       "two different values",
			allocation: []int{2, 4},
			want:       1.0, // variance of [2, 4] = ((2-3)² + (4-3)²) / 2 = (1 + 1) / 2 = 1
		},
		{
			name:       "three values with zeros",
			allocation: []int{0, 3, 0, 3},
			want:       0.0, // variance of [3, 3] = 0
		},
		{
			name:       "multiple different values",
			allocation: []int{1, 3, 5},
			want:       2.666666666666666, // variance of [1, 3, 5] = ((1-3)² + (3-3)² + (5-3)²) / 3 = (4 + 0 + 4) / 3 = 8/3
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateVariance(tt.allocation)
			// Use approximate comparison for floating point values
			if math.Abs(got-tt.want) > 1e-10 {
				t.Errorf("calculateVariance() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		want int
	}{
		{"positive numbers", 5, 3, 3},
		{"negative numbers", -5, -3, -5},
		{"mixed numbers", 5, -3, -3},
		{"equal numbers", 5, 5, 5},
		{"zero", 0, 5, 0},
		{"zero", 5, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := min(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
