/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package nodes

import (
	"context"
	"sort"
	"testing"
	"time"

	testifyassert "github.com/stretchr/testify/assert"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/pointer"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

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

// --- merged from nodes_extra_test.go ---

func nodesScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	testifyassert.NoError(t, v1.AddToScheme(s))
	return s
}

// nodeWith builds a node the way a settled binding leaves one: the claim in spec.workspace
// and the label that mirrors it back from the data plane, both naming the same workspace.
// The two are separate fields on purpose -- see nodeWithClaim for what happens when they
// disagree, which is the state every scale-down has to survive.
func nodeWith(name, cluster, workspace string) *v1.Node {
	n := &v1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if cluster != "" {
		n.Labels[v1.ClusterIdLabel] = cluster
	}
	if workspace != "" {
		n.Labels[v1.WorkspaceIdLabel] = workspace
		n.Spec.Workspace = pointer.String(workspace)
	}
	return n
}

// nodeWithClaim builds a node mid-flight: labelled for one workspace, claimed by another (or
// by nobody). This is what the admin plane looks like for the length of a binding's round
// trip through the data plane, not a corruption.
func nodeWithClaim(name, cluster, labelled, claimed string) *v1.Node {
	n := nodeWith(name, cluster, labelled)
	if claimed == "" {
		n.Spec.Workspace = nil
		return n
	}
	n.Spec.Workspace = pointer.String(claimed)
	return n
}

func runningWorkload(name, cluster, workspace, adminNode string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if cluster != "" {
		w.Labels[v1.ClusterIdLabel] = cluster
	}
	if workspace != "" {
		w.Labels[v1.WorkspaceIdLabel] = workspace
	}
	w.Status.Pods = []v1.WorkloadPod{{AdminNodeName: adminNode, Phase: corev1PodRunningPhase}}
	return w
}

// corev1PodRunningPhase keeps the import surface small; v1.IsPodRunning checks
// the pod phase is not a terminal one.
const corev1PodRunningPhase = "Running"

// offloadedWorkload mirrors runningWorkload but expresses the pod placement via
// the etcd NodeUsage aggregate (Status.Pods empty), as offloaded workloads do.
// The manual aggregate matches what BuildNodeUsage produces for one running,
// scheduled pod on adminNode (its node-set equivalence is locked separately by
// workload.TestNodeUsageNodeSetEquivalence).
func offloadedWorkload(name, cluster, workspace, adminNode string) *v1.Workload {
	w := &v1.Workload{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: map[string]string{}}}
	if cluster != "" {
		w.Labels[v1.ClusterIdLabel] = cluster
	}
	if workspace != "" {
		w.Labels[v1.WorkspaceIdLabel] = workspace
	}
	w.Status.NodeUsage = []v1.NodePodUsage{{
		Node:    adminNode,
		Active:  map[string]int{"0": 1},
		Running: map[string]int{"0": 1},
	}}
	return w
}

// TestGetIdleNodesOfWorkspaceOffloadedDualRead verifies the NodeUsage read path
// yields the same idle-node result as the Status.Pods path: an offloaded workload
// (NodeUsage only, no Status.Pods) still marks its node as used.
func TestGetIdleNodesOfWorkspaceOffloadedDualRead(t *testing.T) {
	ctx := context.Background()
	idle := nodeWith("idle", "c1", "ws1")
	used := nodeWith("used", "c1", "ws1")
	wl := offloadedWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(idle, used, wl).Build()

	idleNodes, err := GetIdleNodesOfWorkspace(ctx, cl, "ws1")
	testifyassert.NoError(t, err)
	testifyassert.Len(t, idleNodes, 1)
	assert.Equal(t, "idle", idleNodes[0].Name)
}

// TestGetUsingNodesOfClusterOffloaded verifies GetUsingNodesOfCluster reads the
// node from the NodeUsage aggregate for an offloaded workload.
func TestGetUsingNodesOfClusterOffloaded(t *testing.T) {
	ctx := context.Background()
	wl := offloadedWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(wl).Build()
	set, err := GetUsingNodesOfCluster(ctx, cl, "c1")
	testifyassert.NoError(t, err)
	testifyassert.True(t, set.Has("used"))
}

// TestOccupiedNodes locks the contract both read sources have to satisfy: a
// terminated pod has released its node, a pod not yet scheduled holds none, and
// a node is reported once however many pods it carries. The inline pairs this
// replaced disagreed on the first point, which let a workload whose pods had
// finished still read as holding their nodes.
func TestOccupiedNodes(t *testing.T) {
	testifyassert.Nil(t, OccupiedNodes(nil))

	// The aggregate answers whenever it carries entries. The unscheduled bucket
	// names no node, and a node split across resourceIds is still one node.
	agg := &v1.Workload{Status: v1.WorkloadStatus{NodeUsage: []v1.NodePodUsage{
		{Node: "n1", Active: map[string]int{"0": 1}},
		{Node: "n1", Active: map[string]int{"1": 1}},
		{Node: "", Active: map[string]int{"0": 1}},
	}}}
	testifyassert.Equal(t, []string{"n1"}, OccupiedNodes(agg))

	// Without an aggregate the pod array answers, under the same rules.
	pods := &v1.Workload{Status: v1.WorkloadStatus{Pods: []v1.WorkloadPod{
		{PodId: "p1", AdminNodeName: "n1", Phase: corev1PodRunningPhase},
		{PodId: "p2", AdminNodeName: "n1", Phase: corev1PodRunningPhase},
		{PodId: "p3", AdminNodeName: "n2", Phase: "Succeeded"},
		{PodId: "p4", AdminNodeName: "n3", Phase: "Failed"},
		{PodId: "p5", AdminNodeName: "n4", Phase: corev1.PodPhase(v1.WorkloadStopped)},
		{PodId: "p6", AdminNodeName: "", Phase: "Pending"},
	}}}
	testifyassert.Equal(t, []string{"n1"}, OccupiedNodes(pods))

	// A workload whose pods have all finished holds nothing, which is what makes
	// an empty aggregate the correct answer at the end of a run.
	done := &v1.Workload{Status: v1.WorkloadStatus{Pods: []v1.WorkloadPod{
		{PodId: "p1", AdminNodeName: "n1", Phase: "Succeeded"},
	}}}
	testifyassert.Empty(t, OccupiedNodes(done))
}

func TestGetNodesOfWorkspacesAndCluster(t *testing.T) {
	ctx := context.Background()
	n1 := nodeWith("n1", "c1", "ws1")
	n2 := nodeWith("n2", "c1", "ws2")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(n1, n2).Build()

	ws, err := GetNodesOfWorkspaces(ctx, cl, []string{"ws1"}, nil)
	testifyassert.NoError(t, err)
	testifyassert.Len(t, ws, 1)

	// filter that drops everything
	wsNone, err := GetNodesOfWorkspaces(ctx, cl, []string{"ws1", "ws2"}, func(v1.Node) bool { return true })
	testifyassert.NoError(t, err)
	testifyassert.Empty(t, wsNone)

	cnodes, err := GetNodesOfCluster(ctx, cl, "c1", nil)
	testifyassert.NoError(t, err)
	testifyassert.Len(t, cnodes, 2)
}

func TestGetIdleNodesAndScalingDown(t *testing.T) {
	ctx := context.Background()
	idle := nodeWith("idle", "c1", "ws1")
	used := nodeWith("used", "c1", "ws1")
	wl := runningWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(idle, used, wl).Build()

	idleNodes, err := GetIdleNodesOfWorkspace(ctx, cl, "ws1")
	testifyassert.NoError(t, err)
	testifyassert.Len(t, idleNodes, 1)
	assert.Equal(t, "idle", idleNodes[0].Name)

	// count <= 0 -> error
	_, err = GetNodesForScalingDown(ctx, cl, "ws1", 0)
	testifyassert.Error(t, err)

	down, err := GetNodesForScalingDown(ctx, cl, "ws1", 1)
	testifyassert.NoError(t, err)
	testifyassert.Len(t, down, 1)
}

func TestGetUsingNodesOfCluster(t *testing.T) {
	ctx := context.Background()
	wl := runningWorkload("w1", "c1", "ws1", "used")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).WithObjects(wl).Build()
	set, err := GetUsingNodesOfCluster(ctx, cl, "c1")
	testifyassert.NoError(t, err)
	testifyassert.True(t, set.Has("used"))
}

// The number three parties have to agree on: the controller writes it, and both webhooks
// recompute it to decide whether the write in front of them is a withdrawal at all.
func TestWithdrawnReplica(t *testing.T) {
	cases := []struct {
		name       string
		replica    int
		oldActions map[string]string
		newActions map[string]string
		want       int
	}{
		{
			name:       "one add of two withdrawn",
			replica:    3,
			oldActions: map[string]string{"n1": v1.NodeActionAdd, "n2": v1.NodeActionAdd},
			newActions: map[string]string{"n2": v1.NodeActionAdd},
			want:       2,
		},
		{
			name:       "the whole request withdrawn",
			replica:    3,
			oldActions: map[string]string{"n1": v1.NodeActionAdd, "n2": v1.NodeActionAdd},
			newActions: nil,
			want:       1,
		},
		{
			// The node was refused because somebody else already has it, so this workspace
			// lost it either way. The decrement that already applied describes where it ended
			// up, and adding one back would ask for a replacement it never released.
			name:       "a withdrawn remove is not given back",
			replica:    2,
			oldActions: map[string]string{"n1": v1.NodeActionRemove},
			newActions: nil,
			want:       2,
		},
		{
			name:       "adds and removes withdrawn together",
			replica:    5,
			oldActions: map[string]string{"n1": v1.NodeActionAdd, "n2": v1.NodeActionRemove, "n3": v1.NodeActionAdd},
			newActions: nil,
			want:       3,
		},
		{
			name:       "nothing withdrawn is nothing given back",
			replica:    2,
			oldActions: map[string]string{"n1": v1.NodeActionAdd},
			newActions: map[string]string{"n1": v1.NodeActionAdd},
			want:       2,
		},
		{
			// Cannot happen from a count the webhook itself moved, but the result of this is
			// written to a spec field, so it clamps rather than going negative.
			name:       "never below zero",
			replica:    1,
			oldActions: map[string]string{"n1": v1.NodeActionAdd, "n2": v1.NodeActionAdd},
			newActions: nil,
			want:       0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, WithdrawnReplica(c.replica, c.oldActions, c.newActions), c.want)
		})
	}
}

// Scale-down candidates are the nodes the workspace can actually release, which is the nodes
// it still holds the claim on -- not the nodes still carrying its label. The two say different
// things for the length of a binding's round trip, and this is the function that has to pick
// the right one: judgeNodeBinding decides the unbind on spec.workspace, so a candidate chosen
// on the label alone comes back refused, and its slot in the batch is spent on a node the
// workspace did hold and did not need to give back.
func TestGetIdleNodesOfWorkspaceFollowsTheClaimNotTheLabel(t *testing.T) {
	ctx := context.Background()
	held := nodeWith("held", "c1", "ws1")
	// Released by ws1; its label has not caught up yet. An unbind here is a no-op that still
	// consumes one of the count nodes asked for.
	released := nodeWithClaim("released", "c1", "ws1", "")
	// Taken by ws2 while ws1's label lingered. An unbind here is refused outright.
	stolen := nodeWithClaim("stolen", "c1", "ws1", "ws2")
	cl := ctrlfake.NewClientBuilder().WithScheme(nodesScheme(t)).
		WithObjects(held, released, stolen).Build()

	idle, err := GetIdleNodesOfWorkspace(ctx, cl, "ws1")
	testifyassert.NoError(t, err)
	testifyassert.Len(t, idle, 1)
	assert.Equal(t, "held", idle[0].Name)

	// And the shortfall is reported rather than papered over with the wrong node: asking for
	// two back when only one can be given hands back one, and the callers refuse to build a
	// request out of a short list.
	down, err := GetNodesForScalingDown(ctx, cl, "ws1", 2)
	testifyassert.NoError(t, err)
	testifyassert.Len(t, down, 1)
	assert.Equal(t, "held", down[0].Name)
}
