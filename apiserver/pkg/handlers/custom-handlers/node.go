/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func (h *Handler) CreateNode(c *gin.Context) {
	handle(c, h.createNode)
}

func (h *Handler) ListNode(c *gin.Context) {
	handle(c, h.listNode)
}

func (h *Handler) GetNode(c *gin.Context) {
	handle(c, h.getNode)
}

func (h *Handler) PatchNode(c *gin.Context) {
	handle(c, h.patchNode)
}

func (h *Handler) DeleteNode(c *gin.Context) {
	handle(c, h.deleteNode)
}

func (h *Handler) GetNodePodLog(c *gin.Context) {
	handle(c, h.getNodePodLog)
}

func (h *Handler) createNode(c *gin.Context) (interface{}, error) {
	req := &types.CreateNodeRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request")
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	node, err := h.generateNode(c, req, body)
	if err != nil {
		klog.ErrorS(err, "failed to generate node")
		return nil, err
	}
	if err = h.Create(c.Request.Context(), node); err != nil {
		klog.ErrorS(err, "failed to create node")
		return nil, err
	}
	klog.Infof("created node %s", node.Name)
	return &types.CreateNodeResponse{
		NodeId: node.Name,
	}, nil
}

func (h *Handler) listNode(c *gin.Context) (interface{}, error) {
	query, err := parseListNodeQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	labelSelector, err := buildNodeLabelSelector(query)
	if err != nil {
		return nil, err
	}
	nodeList := &v1.NodeList{}
	if err = h.List(c.Request.Context(), nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		klog.ErrorS(err, "failed to list admin nodes", "labelSelector", labelSelector)
		return nil, err
	}
	result := &types.GetNodeResponse{}
	if len(nodeList.Items) == 0 {
		return result, nil
	}

	allUsedResource, err := h.getAllUsedResourcePerNode(c.Request.Context(), query)
	if err != nil {
		return nil, err
	}
	nodeWrappers := sortAdminNodes(nodeList.Items)
	for _, n := range nodeWrappers {
		usedResource, _ := allUsedResource[n.Node.Name]
		item := cvtToGetNodeResponseItem(n.Node, usedResource)
		result.Items = append(result.Items, item)
		result.TotalCount++
	}
	return result, nil
}

type adminNodeWrapper struct {
	Node     *v1.Node
	NodeRank int64
}

func sortAdminNodes(nodes []v1.Node) []adminNodeWrapper {
	nodeWrappers := make([]adminNodeWrapper, 0, len(nodes))
	for i, n := range nodes {
		nodeWrappers = append(nodeWrappers, adminNodeWrapper{
			Node:     &nodes[i],
			NodeRank: stringutil.ExtractNumber(n.Status.MachineStatus.PrivateIP),
		})
	}
	sort.Slice(nodeWrappers, func(i, j int) bool {
		if nodeWrappers[i].NodeRank == 0 && nodeWrappers[j].NodeRank == 0 {
			return nodeWrappers[i].Node.Name < nodeWrappers[j].Node.Name
		}
		return nodeWrappers[i].NodeRank < nodeWrappers[j].NodeRank
	})
	return nodeWrappers
}

func (h *Handler) getNode(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	usedResource, err := h.getUsedResource(c.Request.Context(), node)
	if err != nil {
		klog.ErrorS(err, "failed to get used resource", "node", node.Name)
		return nil, err
	}
	return cvtToGetNodeResponseItem(node, usedResource), nil
}

func (h *Handler) patchNode(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}

	req := &types.PatchNodeRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	patch := client.MergeFrom(node.DeepCopy())
	isShouldUpdate, err := h.updateNode(c.Request.Context(), node, req)
	if err != nil || !isShouldUpdate {
		return nil, err
	}
	if err = h.Patch(c.Request.Context(), node, patch); err != nil {
		klog.ErrorS(err, "failed to patch node", "name", node.Name)
		return nil, err
	}
	klog.Infof("patch node, name: %s, request: %v", node.Name, *req)
	return nil, nil
}

func (h *Handler) deleteNode(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if v1.GetClusterId(node) != "" {
		cluster, _ := h.getAdminCluster(c.Request.Context(), v1.GetClusterId(node))
		if cluster != nil {
			return nil, commonerrors.NewInternalError(
				fmt.Sprintf("The node is bound to cluster %s and needs to be unmanaged first", v1.GetClusterId(node)))
		}
	}
	if err = h.Delete(c.Request.Context(), node); err != nil {
		klog.ErrorS(err, "failed to delete node")
		return nil, err
	}
	klog.Infof("delete node %s", node.Name)
	return nil, nil
}

func (h *Handler) getNodePodLog(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	clusterName := node.GetSpecCluster()
	if clusterName == "" {
		clusterName = v1.GetClusterId(node)
	}
	if clusterName == "" {
		return nil, commonerrors.NewInternalError("the node is not bound to any cluster")
	}

	labelSelector := labels.SelectorFromSet(map[string]string{
		v1.ClusterManageClusterLabel: clusterName, v1.ClusterManageNodeLabel: node.Name})
	podName, err := h.getLatestPodName(c, labelSelector)
	if err != nil {
		return nil, err
	}
	podLogs, err := h.getPodLog(c, h.clientSet, common.PrimusSafeNamespace, podName, "")
	if err != nil {
		return nil, err
	}
	return &types.GetNodePodLogResponse{
		ClusterId: clusterName,
		NodeId:    node.Name,
		PodId:     podName,
		Logs:      strings.Split(string(podLogs), "\n"),
	}, nil
}

func (h *Handler) getAdminNode(ctx context.Context, name string) (*v1.Node, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the nodeId is empty")
	}
	node := &v1.Node{}
	err := h.Get(ctx, client.ObjectKey{Name: name}, node)
	if err != nil {
		return nil, err
	}
	return node.DeepCopy(), nil
}

type resourceInfo struct {
	resource  corev1.ResourceList
	workloads []types.WorkloadInfo
}

// Retrieves the amount of resources currently in use on each node.
// Returns a map with the node name as the key, and the value containing the resource usage and associated workload name
func (h *Handler) getAllUsedResourcePerNode(ctx context.Context,
	query *types.ListNodeRequest) (map[string]*resourceInfo, error) {
	result := make(map[string]*resourceInfo)
	// Only cluster nodes bound to a workspace are included in the resource usage statistics.
	if (query.ClusterId != nil && *query.ClusterId == "") ||
		(query.WorkspaceId != nil && *query.WorkspaceId == "") ||
		(query.ClusterId == nil && query.WorkspaceId == nil) {
		return result, nil
	}
	var workspaceNames []string
	if query.GetWorkspaceId() != "" {
		workspaceNames = append(workspaceNames, query.GetWorkspaceId())
	}
	workloads, err := h.getRunningWorkloads(ctx, query.GetClusterId(), workspaceNames)
	if err != nil {
		return nil, err
	}

	for _, w := range workloads {
		resourcePerNode, err := commonworkload.GetResourcesPerNode(w, "")
		if err != nil {
			return nil, err
		}
		for nodeName, resList := range resourcePerNode {
			info, ok := result[nodeName]
			if !ok {
				info = &resourceInfo{}
				result[nodeName] = info
			}
			info.resource = quantity.AddResource(info.resource, resList)
			info.workloads = append(info.workloads, types.WorkloadInfo{
				Id:        w.Name,
				User:      v1.GetUserName(w),
				Workspace: w.Spec.Workspace,
			})
		}
	}
	return result, nil
}

// Retrieves the amount of resources currently in use on specified node.
func (h *Handler) getUsedResource(ctx context.Context, node *v1.Node) (*resourceInfo, error) {
	if v1.GetWorkspaceId(node) == "" {
		return nil, nil
	}
	workloads, err := h.getRunningWorkloads(ctx, v1.GetClusterId(node), []string{v1.GetWorkspaceId(node)})
	if err != nil {
		return nil, err
	}
	result := new(resourceInfo)
	for _, w := range workloads {
		resourcePerNode, err := commonworkload.GetResourcesPerNode(w, node.Name)
		if err != nil {
			return nil, err
		}
		resList, ok := resourcePerNode[node.Name]
		if !ok {
			continue
		}
		result.resource = quantity.AddResource(result.resource, resList)
		result.workloads = append(result.workloads, types.WorkloadInfo{
			Id:        w.Name,
			User:      v1.GetUserName(w),
			Workspace: w.Spec.Workspace,
		})
	}
	return result, nil
}

func (h *Handler) generateNode(c *gin.Context, req *types.CreateNodeRequest, body []byte) (*v1.Node, error) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Labels: req.Labels,
		},
	}
	err := json.Unmarshal(body, &node.Spec)
	if err != nil {
		return nil, err
	}
	if err = validateCreateNodeRequest(req); err != nil {
		return nil, err
	}

	nf, err := h.getAdminNodeFlavor(c.Request.Context(), req.FlavorName)
	if err != nil {
		return nil, err
	}
	node.Spec.NodeFlavor = commonutils.GenObjectReference(nf.TypeMeta, nf.ObjectMeta)

	secret, err := h.getSecret(c.Request.Context(), req.SSHSecretName)
	if err != nil {
		return nil, err
	}
	node.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	return node, nil
}

func validateCreateNodeRequest(req *types.CreateNodeRequest) error {
	if req.FlavorName == "" {
		return commonerrors.NewBadRequest("the flavorName of request is empty")
	}
	if req.PrivateIP == "" {
		return commonerrors.NewBadRequest("the privateIP of request is empty")
	}
	if req.SSHSecretName == "" {
		return commonerrors.NewBadRequest("the sshSecretName of request is empty")
	}
	return nil
}

func buildNodeLabelSelector(query *types.ListNodeRequest) (labels.Selector, error) {
	var labelSelector = labels.NewSelector()
	var req1, req2, req3 *labels.Requirement
	if query.ClusterId != nil {
		if *query.ClusterId == "" {
			req1, _ = labels.NewRequirement(v1.ClusterIdLabel, selection.DoesNotExist, nil)
		} else {
			req1, _ = labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{*query.ClusterId})
		}
		labelSelector = labelSelector.Add(*req1)
	}
	if query.WorkspaceId != nil {
		if *query.WorkspaceId == "" {
			req2, _ = labels.NewRequirement(v1.WorkspaceIdLabel, selection.DoesNotExist, nil)
		} else {
			req2, _ = labels.NewRequirement(v1.WorkspaceIdLabel, selection.Equals, []string{*query.WorkspaceId})
		}
		labelSelector = labelSelector.Add(*req2)
	}
	if query.NodeFlavor != nil {
		req3, _ = labels.NewRequirement(v1.NodeFlavorIdLabel, selection.Equals, []string{*query.NodeFlavor})
		labelSelector = labelSelector.Add(*req3)
	}
	return labelSelector, nil
}

func parseListNodeQuery(c *gin.Context) (*types.ListNodeRequest, error) {
	query := &types.ListNodeRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

func (h *Handler) updateNode(ctx context.Context, node *v1.Node, req *types.PatchNodeRequest) (bool, error) {
	isShouldUpdate := false
	nodesLabelAction := make(map[string]string)
	if req.Labels != nil {
		reqLabels := make(map[string]string)
		for key, val := range *req.Labels {
			reqLabels[common.CustomerLabelPrefix+key] = val
		}
		currentLabels := getCustomerLabels(node.Labels, false)
		for key, val := range currentLabels {
			val2, ok := reqLabels[key]
			if !ok {
				nodesLabelAction[key] = v1.NodeActionRemove
				delete(node.Labels, key)
				isShouldUpdate = true
			} else if val != val2 {
				nodesLabelAction[key] = v1.NodeActionAdd
				v1.SetLabel(node, key, val2)
				isShouldUpdate = true
			}
		}
		for key, val := range reqLabels {
			if _, ok := currentLabels[key]; !ok {
				nodesLabelAction[key] = v1.NodeActionAdd
				v1.SetLabel(node, key, val)
				isShouldUpdate = true
			}
		}
	}
	if req.Taints != nil {
		for i, t := range *req.Taints {
			(*req.Taints)[i].Key = commonfaults.GenerateTaintKey(t.Key)
		}
		if !commonfaults.IsTaintsEqualIgnoreOrder(*req.Taints, node.Spec.Taints) {
			node.Spec.Taints = *req.Taints
			isShouldUpdate = true
		}
	}
	if req.NodeFlavor != nil && *req.NodeFlavor != "" &&
		(node.Spec.NodeFlavor == nil || *req.NodeFlavor != node.Spec.NodeFlavor.Name) {
		nf, err := h.getAdminNodeFlavor(ctx, *req.NodeFlavor)
		if err != nil {
			return false, err
		}
		node.Spec.NodeFlavor = commonutils.GenObjectReference(nf.TypeMeta, nf.ObjectMeta)
		nodesLabelAction[v1.NodeFlavorIdLabel] = v1.NodeActionAdd
		isShouldUpdate = true
	}
	if len(nodesLabelAction) > 0 {
		v1.SetAnnotation(node, v1.NodeLabelAction, string(jsonutils.MarshalSilently(nodesLabelAction)))
	}
	return isShouldUpdate, nil
}

func cvtToGetNodeResponseItem(n *v1.Node, usedResource *resourceInfo) types.GetNodeResponseItem {
	result := types.GetNodeResponseItem{
		NodeId:         n.Name,
		DisplayName:    v1.GetDisplayName(n),
		Cluster:        v1.GetClusterId(n),
		Workspace:      v1.GetWorkspaceId(n),
		Phase:          string(n.Status.MachineStatus.Phase),
		InternalIP:     n.Status.MachineStatus.PrivateIP,
		NodeFlavor:     v1.GetNodeFlavorId(n),
		Unschedulable:  n.IsAvailable(false),
		Taints:         getPrimusTaints(n.Status.Taints),
		TotalResources: cvtToResourceList(n.Status.Resources),
		CustomerLabels: getCustomerLabels(n.Labels, true),
		CreatedTime:    timeutil.FormatRFC3339(&n.CreationTimestamp.Time),
		IsControlPlane: v1.IsControlPlane(n),
	}
	var availResource corev1.ResourceList
	if usedResource != nil && len(usedResource.resource) > 0 {
		availResource = quantity.GetAvailableResource(n.Status.Resources)
		availResource = quantity.SubResource(availResource, usedResource.resource)
		result.Workloads = usedResource.workloads
	} else {
		availResource = quantity.GetAvailableResource(n.Status.Resources)
	}
	result.AvailResources = cvtToResourceList(availResource)
	lastStartupTime := timeutil.CvtStrUnixToTime(v1.GetNodeStartupTime(n))
	result.LastStartupTime = timeutil.FormatRFC3339(&lastStartupTime)
	return result
}

func getCustomerLabels(labels map[string]string, removePrefix bool) map[string]string {
	result := make(map[string]string)
	for key, val := range labels {
		if strings.HasPrefix(key, common.CustomerLabelPrefix) {
			if removePrefix {
				result[key[len(common.CustomerLabelPrefix):]] = val
			} else {
				result[key] = val
			}
		}
	}
	return result
}

func getPrimusTaints(taints []corev1.Taint) []corev1.Taint {
	var result []corev1.Taint
	for i, t := range taints {
		if strings.HasPrefix(t.Key, v1.PrimusSafePrefix) {
			taints[i].Key = taints[i].Key[len(v1.PrimusSafePrefix):]
			result = append(result, taints[i])
		}
	}
	return result
}
