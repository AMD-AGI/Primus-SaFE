/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

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
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apis/pkg/client/clientset/versioned/scheme"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
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
				Product:      "AMD MI300X",
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
	clusterId := "cluster"
	nodeFlavorId := "nodeflavor"
	workspace := genMockWorkspace(clusterId, nodeFlavorId)
	adminNode1 := genMockAdminNode(clusterId, workspace.Name, nodeFlavorId)
	adminNode2 := genMockAdminNode(clusterId, workspace.Name, nodeFlavorId)
	adminNode1.Name = "node1"
	adminNode1.Status.Resources = genMockNodeResource(64, 2*1024*1024*1024, 8)
	adminNode2.Name = "node2"
	adminNode2.Status.Resources = adminNode1.Status.Resources
	workload1 := genMockWorkload(clusterId, workspace.Name)
	workload2 := genMockWorkload(clusterId, workspace.Name)
	workload1.Status.Pods = []v1.WorkloadPod{{
		AdminNodeName: adminNode1.Name,
		K8sNodeName:   adminNode1.Name,
	}}
	workload2.Status.Pods = []v1.WorkloadPod{{
		AdminNodeName: adminNode1.Name,
		K8sNodeName:   adminNode1.Name,
	}, {
		AdminNodeName: adminNode2.Name,
		K8sNodeName:   adminNode2.Name,
	}}
	adminClient := fake.NewClientBuilder().WithObjects(workspace, workload1, workload2, adminNode1, adminNode2).
		WithStatusSubresource(workload1, workload2).WithScheme(scheme.Scheme).Build()

	h := Handler{Client: adminClient}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	c.Request = httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/nodes?clusterId=%s&workspaceId=%s", clusterId, workspace.Name), nil)
	h.ListNode(c)
	assert.Equal(t, rsp.Code, http.StatusOK)

	result := &types.ListNodeResponse{}
	err := json.Unmarshal(rsp.Body.Bytes(), &result)
	assert.NilError(t, err)
	assert.Equal(t, result.TotalCount, 2)
	sort.Slice(result.Items, func(i, j int) bool {
		return result.Items[i].NodeId < result.Items[j].NodeId
	})

	assert.Equal(t, result.Items[0].NodeId, adminNode1.Name)
	assert.Equal(t, result.Items[0].Cluster, clusterId)
	assert.Equal(t, result.Items[0].Workspace.Id, workspace.Name)
	assert.Equal(t, result.Items[0].TotalResources["cpu"], int64(64))
	assert.Equal(t, result.Items[0].TotalResources["memory"], int64(2*1024*1024*1024))
	assert.Equal(t, result.Items[0].TotalResources[common.AmdGpu], int64(8))
	assert.Equal(t, result.Items[0].AvailResources["cpu"], int64(32))
	assert.Equal(t, result.Items[0].AvailResources["memory"], int64(0))
	assert.Equal(t, result.Items[0].AvailResources[common.AmdGpu], int64(0))
	assert.Equal(t, len(result.Items[0].Workloads), 2)

	assert.Equal(t, result.Items[1].NodeId, adminNode2.Name)
	assert.Equal(t, result.Items[1].Cluster, clusterId)
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
	clusterId := "cluster"
	nodeFlavor := genMockNodeFlavor()
	adminNode := genMockAdminNode(clusterId, "", "test-node-flavor")
	adminNode.Labels[common.CustomerLabelPrefix+"key1"] = "val1"
	adminClient := fake.NewClientBuilder().WithObjects(nodeFlavor, adminNode).WithScheme(scheme.Scheme).Build()

	h := Handler{Client: adminClient}
	rsp := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rsp)
	body := &types.PatchNodeRequest{
		Labels: &map[string]string{
			"key2": "val2",
		},
		Taints: &[]corev1.Taint{{
			Key:    "key1",
			Effect: corev1.TaintEffectNoSchedule,
		}},
		NodeFlavor: &nodeFlavor.Name,
	}
	c.Request = httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/nodes/%s", adminNode.Name),
		strings.NewReader(string(jsonutils.MarshalSilently(body))))
	c.Set(types.Name, adminNode.Name)
	h.PatchNode(c)
	assert.Equal(t, rsp.Code, http.StatusOK)
	time.Sleep(time.Millisecond * 200)

	node2, err := h.getAdminNode(context.Background(), adminNode.Name)
	assert.NilError(t, err)
	assert.Equal(t, node2.Labels[common.CustomerLabelPrefix+"key1"], "")
	assert.Equal(t, node2.Labels[common.CustomerLabelPrefix+"key2"], "val2")
	assert.Equal(t, node2.Spec.NodeFlavor.Name, nodeFlavor.Name)
	assert.Equal(t, len(node2.Spec.Taints), 1)
	assert.Equal(t, node2.Spec.Taints[0].Key, commonfaults.GenerateTaintKey("key1"))

	actions := v1.GetNodeLabelAction(node2)
	actionMap := make(map[string]string)
	err = json.Unmarshal([]byte(actions), &actionMap)
	assert.NilError(t, err)
	val, ok := actionMap[common.CustomerLabelPrefix+"key1"]
	assert.Equal(t, ok, true)
	assert.Equal(t, val, v1.NodeActionRemove)
	val, ok = actionMap[common.CustomerLabelPrefix+"key2"]
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
	query := types.ListNodeRequest{}
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
