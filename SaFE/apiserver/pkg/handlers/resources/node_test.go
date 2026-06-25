/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

func genMockNodeFlavor() *v1.NodeFlavor {
	memQuantity, _ := resource.ParseQuantity("1024Gi")
	return &v1.NodeFlavor{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.NodeFlavorKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("nodeFlavor"),
		},
		Spec: v1.NodeFlavorSpec{
			Cpu: v1.CpuChip{
				Product:  "AMD 9554",
				Quantity: *resource.NewQuantity(256, resource.DecimalSI),
			},
			Memory: memQuantity,
			Gpu: &v1.GpuChip{
				ResourceName: common.AmdGpu,
				Product:      "MI300X",
				Quantity:     *resource.NewQuantity(8, resource.DecimalSI),
			},
		},
	}
}

func genMockAdminNode(clusterId, workspaceId, nodeFlavorId string) *v1.Node {
	result := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.NodeKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("node"),
			Labels: map[string]string{
				v1.DisplayNameLabel:  "node",
				v1.NodeFlavorIdLabel: nodeFlavorId,
			},
		},
		Spec: v1.NodeSpec{
			Cluster: pointer.String(clusterId),
			NodeFlavor: &corev1.ObjectReference{
				Name:      nodeFlavorId,
				Namespace: common.PrimusSafeNamespace,
			},
		},
	}
	if clusterId != "" {
		result.Spec.Cluster = pointer.String(clusterId)
		metav1.SetMetaDataLabel(&result.ObjectMeta, v1.ClusterIdLabel, clusterId)
	}
	if workspaceId != "" {
		result.Spec.Workspace = pointer.String(workspaceId)
		metav1.SetMetaDataLabel(&result.ObjectMeta, v1.WorkspaceIdLabel, workspaceId)
	}
	return result
}

func genMockNodeResource(cpu, mem, gpu int64) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    *resource.NewQuantity(cpu, resource.DecimalSI),
		corev1.ResourceMemory: *resource.NewQuantity(mem, resource.BinarySI),
		common.AmdGpu:         *resource.NewQuantity(gpu, resource.DecimalSI),
	}
}

func TestListNodes(t *testing.T) {
	clusterId := "test-cluster"
	nodeFlavorId := "test-nodeflavor"
	workspace := genMockWorkspace(clusterId, nodeFlavorId)
	adminNode1 := genMockAdminNode(clusterId, workspace.Name, nodeFlavorId)
	adminNode2 := genMockAdminNode(clusterId, workspace.Name, nodeFlavorId)
	adminNode1.Name = "node1"
	adminNode1.Status.Resources = genMockNodeResource(64, 2*1024*1024*1024, 8)
	adminNode2.Name = "node2"
	adminNode2.Status.Resources = adminNode1.Status.Resources
	// Mark both nodes ready/managed so they are available and contribute their
	// available resources (an abnormal node reports zero available).
	for _, n := range []*v1.Node{adminNode1, adminNode2} {
		n.Status.MachineStatus.Phase = v1.NodeReady
		n.Status.ClusterStatus.Phase = v1.NodeManaged
	}
	workload1 := genMockWorkload(clusterId, workspace.Name)
	workload2 := genMockWorkload(clusterId, workspace.Name)
	workload1.Status.Pods = []v1.WorkloadPod{{
		AdminNodeName: adminNode1.Name,
	}}
	workload2.Status.Pods = []v1.WorkloadPod{{
		AdminNodeName: adminNode1.Name,
	}, {
		AdminNodeName: adminNode2.Name,
	}}
	user := genMockUser()
	role := genMockRole()
	fakeClient := fake.NewClientBuilder().WithObjects(workspace, workload1, workload2, adminNode1, adminNode2, user, role).
		WithStatusSubresource(workload1, workload2).WithScheme(scheme.Scheme).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/nodes?clusterId=%s&workspaceId=%s", clusterId, workspace.Name), nil)
	h.ListNode(c)
	assert.Equal(t, rsp.Code, http.StatusOK)

	result := &view.ListNodeResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 2)
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].NodeId < result.Items[j].NodeId
	})

	assert.Equal(t, result.Items[0].NodeId, adminNode1.Name)
	assert.Equal(t, result.Items[0].ClusterId, clusterId)
	assert.Equal(t, result.Items[0].Workspace.Id, workspace.Name)
	assert.Equal(t, result.Items[0].TotalResources["cpu"], int64(64))
	assert.Equal(t, result.Items[0].TotalResources["memory"], int64(2*1024*1024*1024))
	assert.Equal(t, result.Items[0].TotalResources[common.AmdGpu], int64(8))
	assert.Equal(t, result.Items[0].AvailResources["cpu"], int64(32))
	assert.Equal(t, result.Items[0].AvailResources["memory"], int64(0))
	assert.Equal(t, result.Items[0].AvailResources[common.AmdGpu], int64(0))
	assert.Equal(t, len(result.Items[0].Workloads), 2)

	assert.Equal(t, result.Items[1].NodeId, adminNode2.Name)
	assert.Equal(t, result.Items[1].ClusterId, clusterId)
	assert.Equal(t, result.Items[1].Workspace.Id, workspace.Name)
	assert.Equal(t, result.Items[1].TotalResources["cpu"], int64(64))
	assert.Equal(t, result.Items[1].TotalResources["memory"], int64(2*1024*1024*1024))
	assert.Equal(t, result.Items[1].TotalResources[common.AmdGpu], int64(8))
	assert.Equal(t, result.Items[1].AvailResources["cpu"], int64(48))
	assert.Equal(t, result.Items[1].AvailResources["memory"], int64(1*1024*1024*1024))
	assert.Equal(t, result.Items[1].AvailResources[common.AmdGpu], int64(4))
	assert.Equal(t, len(result.Items[1].Workloads), 1)
	assert.Equal(t, result.Items[1].Workloads[0].Id, workload2.Name)
}

func TestPatchNode(t *testing.T) {
	clusterId := "test-cluster"
	nodeFlavor := genMockNodeFlavor()
	user := genMockUser()
	role := genMockRole()
	adminNode := genMockAdminNode(clusterId, "", "test-node-flavor")
	adminNode.Labels["key1"] = "val1"
	fakeClient := fake.NewClientBuilder().WithObjects(nodeFlavor, adminNode, user, role).WithScheme(scheme.Scheme).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)
	body := &view.PatchNodeRequest{
		Labels: &map[string]string{
			"key2": "val2",
		},
		Taints: &[]corev1.Taint{{
			Key:    "key1",
			Effect: corev1.TaintEffectNoSchedule,
		}},
		FlavorId: &nodeFlavor.Name,
	}
	c.Request = httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/nodes/%s", adminNode.Name),
		strings.NewReader(string(jsonutils.MarshalSilently(body))))
	c.Set(common.Name, adminNode.Name)
	h.PatchNode(c)
	assert.Equal(t, rsp.Code, http.StatusOK)
	time.Sleep(time.Millisecond * 200)

	node2, err := h.getAdminNode(context.Background(), adminNode.Name)
	assert.NilError(t, err)
	assert.Equal(t, node2.Labels["key1"], "")
	assert.Equal(t, node2.Labels["key2"], "val2")
	assert.Equal(t, node2.Spec.NodeFlavor.Name, nodeFlavor.Name)
	assert.Equal(t, len(node2.Spec.Taints), 1)
	assert.Equal(t, node2.Spec.Taints[0].Key, commonfaults.GenerateTaintKey("key1"))

	actions := v1.GetNodeLabelAction(node2)
	actionMap := make(map[string]string)
	err = json.Unmarshal([]byte(actions), &actionMap)
	assert.NilError(t, err)
	val, ok := actionMap["key1"]
	assert.Equal(t, ok, true)
	assert.Equal(t, val, v1.NodeActionRemove)
	val, ok = actionMap["key2"]
	assert.Equal(t, ok, true)
	assert.Equal(t, val, v1.NodeActionAdd)
	val, ok = actionMap[v1.NodeFlavorIdLabel]
	assert.Equal(t, ok, true)
	assert.Equal(t, val, v1.NodeActionAdd)
}

func TestParseListNodeQuery(t *testing.T) {
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?workspaceId=", nil)
	query, err := parseListNodeQuery(c)
	assert.NilError(t, err)
	assert.Equal(t, query.WorkspaceId == nil, false)
	assert.Equal(t, *query.WorkspaceId, "")

	c, _ = gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	query, err = parseListNodeQuery(c)
	assert.NilError(t, err)
	assert.Equal(t, query.WorkspaceId == nil, true)
}

func TestBuildNodeLabelSelector(t *testing.T) {
	query := view.ListNodeRequest{}
	selector, _ := buildNodeLabelSelector(&query)
	assert.Equal(t, selector.Empty(), true)

	cluster := "cl"
	query.ClusterId = &cluster
	selector, _ = buildNodeLabelSelector(&query)
	assert.Equal(t, selector.Matches(labels.Set{v1.ClusterIdLabel: "cl"}), true)

	query.WorkspaceId = pointer.String("workspace")
	selector, _ = buildNodeLabelSelector(&query)
	assert.Equal(t, selector.Matches(labels.Set{v1.WorkspaceIdLabel: "workspace", v1.ClusterIdLabel: "cl"}), true)

	query.WorkspaceId = pointer.String("")
	selector, _ = buildNodeLabelSelector(&query)
	assert.Equal(t, selector.String(), fmt.Sprintf("%s=%s,!%s", v1.ClusterIdLabel, "cl", v1.WorkspaceIdLabel))
}

func genMockNodeWithPhase(clusterID string, phase v1.NodePhase, isMachineReady bool) *v1.Node {
	node := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.NodeKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName("node"),
			Labels: map[string]string{
				v1.ClusterIdLabel: clusterID,
			},
		},
		Spec: v1.NodeSpec{
			Cluster: pointer.String(clusterID),
		},
		Status: v1.NodeStatus{
			ClusterStatus: v1.NodeClusterStatus{
				Phase: phase,
			},
		},
	}
	if isMachineReady {
		node.Status.MachineStatus.Phase = v1.NodeReady
	}
	return node
}

func getTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = scheme.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

func TestRetryNodes_SingleNode(t *testing.T) {
	clusterID := "test-cluster"
	user := genMockUser()
	role := genMockRole()
	node := genMockNodeWithPhase(clusterID, v1.NodeManagedFailed, true)

	fakeClient := fake.NewClientBuilder().
		WithObjects(node, user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)

	reqBody := fmt.Sprintf(`{"nodeIds": ["%s"]}`, node.Name)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/nodes/retry", strings.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RetryNodes(c)

	assert.Equal(t, rsp.Code, http.StatusOK)

	result := &view.RetryNodesResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 1)
	assert.Equal(t, result.SuccessCount, 1)
	assert.Equal(t, len(result.SuccessNodes), 1)
	assert.Equal(t, result.SuccessNodes[0].NodeId, node.Name)
}

func TestRetryNodes_BatchNodes(t *testing.T) {
	clusterID := "test-cluster"
	user := genMockUser()
	role := genMockRole()
	node1 := genMockNodeWithPhase(clusterID, v1.NodeManagedFailed, true)
	node1.Name = "node-1"
	node2 := genMockNodeWithPhase(clusterID, v1.NodeUnmanagedFailed, true)
	node2.Name = "node-2"

	fakeClient := fake.NewClientBuilder().
		WithObjects(node1, node2, user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)

	reqBody := `{"nodeIds": ["node-1", "node-2"]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/nodes/retry", strings.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RetryNodes(c)

	assert.Equal(t, rsp.Code, http.StatusOK)

	result := &view.RetryNodesResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 2)
	assert.Equal(t, result.SuccessCount, 2)
	assert.Equal(t, len(result.SuccessNodes), 2)
}

func TestRetryNodes_EmptyNodeIds(t *testing.T) {
	user := genMockUser()
	role := genMockRole()

	fakeClient := fake.NewClientBuilder().
		WithObjects(user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)

	reqBody := `{"nodeIds": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/nodes/retry", strings.NewReader(reqBody))
	c.Request.Header.Set("Content-Type", "application/json")

	h.RetryNodes(c)

	assert.Equal(t, rsp.Code, http.StatusBadRequest)
	assert.Assert(t, strings.Contains(rsp.Body.String(), "nodeIds cannot be empty"))
}

func genMockAdminNodeWithIP(name, clusterId, workspaceId, privateIP string) *v1.Node {
	result := &v1.Node{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.NodeKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.DisplayNameLabel: name,
			},
		},
		Spec: v1.NodeSpec{
			PrivateIP: privateIP,
		},
	}
	if clusterId != "" {
		result.Spec.Cluster = pointer.String(clusterId)
		metav1.SetMetaDataLabel(&result.ObjectMeta, v1.ClusterIdLabel, clusterId)
	}
	if workspaceId != "" {
		result.Spec.Workspace = pointer.String(workspaceId)
		metav1.SetMetaDataLabel(&result.ObjectMeta, v1.WorkspaceIdLabel, workspaceId)
	}
	return result
}

func TestListNodes_SearchByName(t *testing.T) {
	clusterId := "test-cluster"
	user := genMockUser()
	role := genMockRole()

	// Create nodes with different names
	node1 := genMockAdminNodeWithIP("gpu-node-001", clusterId, "", "192.168.1.100")
	node2 := genMockAdminNodeWithIP("gpu-node-002", clusterId, "", "192.168.1.101")
	node3 := genMockAdminNodeWithIP("cpu-server-001", clusterId, "", "10.0.0.50")

	fakeClient := fake.NewClientBuilder().
		WithObjects(node1, node2, node3, user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

	// Test search by name (partial match)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?search=gpu-node", nil)

	h.ListNode(c)

	assert.Equal(t, rsp.Code, http.StatusOK)
	result := &view.ListNodeResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 2) // Should match gpu-node-001 and gpu-node-002
}

func TestListNodes_SearchByIP(t *testing.T) {
	clusterId := "test-cluster"
	user := genMockUser()
	role := genMockRole()

	// Create nodes with different IPs
	node1 := genMockAdminNodeWithIP("node-1", clusterId, "", "192.168.1.100")
	node2 := genMockAdminNodeWithIP("node-2", clusterId, "", "192.168.1.101")
	node3 := genMockAdminNodeWithIP("node-3", clusterId, "", "10.0.0.50")

	fakeClient := fake.NewClientBuilder().
		WithObjects(node1, node2, node3, user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

	// Test search by IP (partial match)
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?search=192.168", nil)

	h.ListNode(c)

	assert.Equal(t, rsp.Code, http.StatusOK)
	result := &view.ListNodeResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 2) // Should match 192.168.1.100 and 192.168.1.101
}

func TestListNodes_SearchCaseInsensitive(t *testing.T) {
	clusterId := "test-cluster"
	user := genMockUser()
	role := genMockRole()

	node1 := genMockAdminNodeWithIP("GPU-Node-Upper", clusterId, "", "192.168.1.100")
	node2 := genMockAdminNodeWithIP("gpu-node-lower", clusterId, "", "192.168.1.101")

	fakeClient := fake.NewClientBuilder().
		WithObjects(node1, node2, user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

	// Test case-insensitive search
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?search=GPU-NODE", nil)

	h.ListNode(c)

	assert.Equal(t, rsp.Code, http.StatusOK)
	result := &view.ListNodeResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 2) // Should match both (case-insensitive)
}

func TestListNodes_SearchNoMatch(t *testing.T) {
	clusterId := "test-cluster"
	user := genMockUser()
	role := genMockRole()

	node1 := genMockAdminNodeWithIP("node-1", clusterId, "", "192.168.1.100")

	fakeClient := fake.NewClientBuilder().
		WithObjects(node1, user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

	// Test search with no matches
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?search=nonexistent", nil)

	h.ListNode(c)

	assert.Equal(t, rsp.Code, http.StatusOK)
	result := &view.ListNodeResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 0)
}

func TestListNodes_SearchCombinedWithOtherFilters(t *testing.T) {
	cluster1 := "cluster-1"
	cluster2 := "cluster-2"
	user := genMockUser()
	role := genMockRole()

	// Nodes in cluster-1
	node1 := genMockAdminNodeWithIP("gpu-node-001", cluster1, "", "192.168.1.100")
	node2 := genMockAdminNodeWithIP("gpu-node-002", cluster1, "", "192.168.1.101")
	// Node in cluster-2
	node3 := genMockAdminNodeWithIP("gpu-node-003", cluster2, "", "192.168.1.102")

	fakeClient := fake.NewClientBuilder().
		WithObjects(node1, node2, node3, user, role).
		WithScheme(getTestScheme()).Build()

	h := Handler{Client: fakeClient, accessController: authority.NewAccessController(fakeClient)}

	// Test search combined with clusterId filter
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Set(common.UserId, user.Name)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/nodes?search=gpu-node&clusterId=cluster-1", nil)

	h.ListNode(c)

	assert.Equal(t, rsp.Code, http.StatusOK)
	result := &view.ListNodeResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 2) // Only gpu-node-001 and gpu-node-002 in cluster-1
}

// genAvailableNode builds a ready + managed (i.e. available) node bound to the
// given workspace/flavor with the provided status resources.
func genAvailableNode(name, clusterId, workspaceId, flavorId string, res corev1.ResourceList) *v1.Node {
	n := genMockAdminNode(clusterId, workspaceId, flavorId)
	n.Name = name
	n.Status.Resources = res
	n.Status.MachineStatus.Phase = v1.NodeReady
	n.Status.ClusterStatus.Phase = v1.NodeManaged
	return n
}

func TestNodeContributesAvailable(t *testing.T) {
	clusterId := "c1"
	wsId := "ws1"
	flavor := "f1"
	available := genAvailableNode("n1", clusterId, wsId, flavor, genMockNodeResource(8, 1<<30, 4))

	// Available node whose flavor matches the workspace flavor contributes.
	assert.Equal(t, true, nodeContributesAvailable(available, flavor))
	// Available node whose flavor differs from the workspace flavor does not.
	assert.Equal(t, false, nodeContributesAvailable(available, "other-flavor"))
	// Unknown workspace flavor falls back to availability only.
	assert.Equal(t, true, nodeContributesAvailable(available, ""))

	// Abnormal node (machine not ready) does not contribute even if flavor matches.
	notReady := genAvailableNode("n2", clusterId, wsId, flavor, genMockNodeResource(8, 1<<30, 4))
	notReady.Status.MachineStatus.Phase = ""
	assert.Equal(t, false, nodeContributesAvailable(notReady, flavor))

	// Abnormal node (not managed) does not contribute.
	notManaged := genAvailableNode("n3", clusterId, wsId, flavor, genMockNodeResource(8, 1<<30, 4))
	notManaged.Status.ClusterStatus.Phase = ""
	assert.Equal(t, false, nodeContributesAvailable(notManaged, flavor))
}

func TestCvtToNodeResponseItemZerosWhenNotContributing(t *testing.T) {
	node := genAvailableNode("n1", "c1", "ws1", "f1", genMockNodeResource(64, 2*1024*1024*1024, 8))

	// Contributing node reports allocatable minus in-use (reserve is 0 in tests).
	item := cvtToNodeResponseItem(node, nil, true)
	assert.Equal(t, item.AvailResources["cpu"], int64(64))
	assert.Equal(t, item.AvailResources[common.AmdGpu], int64(8))

	// Non-contributing (abnormal / flavor-mismatched) node reports zero, never
	// negative, while totalResources is unchanged.
	item = cvtToNodeResponseItem(node, nil, false)
	assert.Equal(t, item.TotalResources["cpu"], int64(64))
	assert.Equal(t, item.AvailResources["cpu"], int64(0))
	assert.Equal(t, item.AvailResources[common.AmdGpu], int64(0))
}

func TestGetWorkspaceAvailQuota(t *testing.T) {
	clusterId := "test-cluster"
	flavor := "test-flavor"
	workspace := genMockWorkspace(clusterId, flavor)
	wsName := workspace.Name

	// nodeA: available + flavor match, lightly used -> contributes positive avail.
	nodeA := genAvailableNode("node-a", clusterId, wsName, flavor, genMockNodeResource(64, 2*1024*1024*1024, 8))
	// nodeB: available + flavor match, over-committed -> clamps to 0 (not negative).
	nodeB := genAvailableNode("node-b", clusterId, wsName, flavor, genMockNodeResource(8, 1*1024*1024*1024, 4))
	// nodeC: flavor match but abnormal (machine not ready) -> excluded.
	nodeC := genAvailableNode("node-c", clusterId, wsName, flavor, genMockNodeResource(64, 2*1024*1024*1024, 8))
	nodeC.Status.MachineStatus.Phase = ""
	// nodeD: available but flavor mismatch -> excluded.
	nodeD := genAvailableNode("node-d", clusterId, wsName, "other-flavor", genMockNodeResource(64, 2*1024*1024*1024, 8))

	// One running pod per used node (per-pod request = cpu 16, gpu 4, mem 1Gi).
	wlA := genMockWorkload(clusterId, wsName)
	wlA.Status.Pods = []v1.WorkloadPod{{AdminNodeName: nodeA.Name}}
	wlB := genMockWorkload(clusterId, wsName)
	wlB.Status.Pods = []v1.WorkloadPod{{AdminNodeName: nodeB.Name}}

	fakeClient := fake.NewClientBuilder().
		WithObjects(workspace, nodeA, nodeB, nodeC, nodeD, wlA, wlB).
		WithStatusSubresource(wlA, wlB).
		WithScheme(scheme.Scheme).Build()
	h := Handler{Client: fakeClient}

	avail, err := h.getWorkspaceAvailQuota(context.Background(), workspace)
	assert.NilError(t, err)

	// nodeA: cpu 64-16=48, gpu 8-4=4, mem 2Gi-1Gi=1Gi.
	// nodeB: cpu 8-16<0 -> 0, gpu 4-4=0, mem 1Gi-1Gi=0.
	// nodeC (abnormal) and nodeD (flavor mismatch) excluded.
	cpu := avail[corev1.ResourceCPU]
	gpu := avail[common.AmdGpu]
	mem := avail[corev1.ResourceMemory]
	assert.Equal(t, int64(48), cpu.Value())
	assert.Equal(t, int64(4), gpu.Value())
	assert.Equal(t, int64(1*1024*1024*1024), mem.Value())
}
