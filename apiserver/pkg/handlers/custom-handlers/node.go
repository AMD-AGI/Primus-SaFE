/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/slice"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CreateNode: handles the creation of a new node resource.
// It authorizes the request, parses the request body, generates a node object,
// and creates it in the system. Returns the created node ID on success.
func (h *Handler) CreateNode(c *gin.Context) {
	handle(c, h.createNode)
}

// ListNode: handles listing nodes based on query parameters.
// Supports filtering, pagination, and brief response formats.
// Returns a list of nodes that match the query criteria.
func (h *Handler) ListNode(c *gin.Context) {
	handle(c, h.listNode)
}

// GetNode: retrieves detailed information about a specific node.
// Authorizes access to the node and returns comprehensive node details
// including resource usage and workload information.
func (h *Handler) GetNode(c *gin.Context) {
	handle(c, h.getNode)
}

// PatchNode handles partial updates to a node resource.
// Authorizes the request, parses update parameters, and applies changes
// to the specified node with conflict retry logic.
func (h *Handler) PatchNode(c *gin.Context) {
	handle(c, h.patchNode)
}

// DeleteNode: handles deletion of a node resource.
// Ensures the node is not bound to a cluster and authorizes the deletion
// before removing the node from the system.
func (h *Handler) DeleteNode(c *gin.Context) {
	handle(c, h.deleteNode)
}

// GetNodePodLog retrieves logs from the pod associated with a node's management operations.
// Authorizes access and fetches logs from the most recent management pod for the node.
func (h *Handler) GetNodePodLog(c *gin.Context) {
	handle(c, h.getNodePodLog)
}

// createNode implements the node creation logic.
// Validates the request, generates a node object with specified parameters,
// and persists it in the system.
func (h *Handler) createNode(c *gin.Context) (interface{}, error) {
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: v1.NodeKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.CreateNodeRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
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

// listNode implements the node listing logic.
// Parses query parameters, retrieves matching nodes, and builds
// either a brief or detailed response based on the query.
func (h *Handler) listNode(c *gin.Context) (interface{}, error) {
	query, err := parseListNodeQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	ctx := c.Request.Context()
	totalCount, nodes, err := h.listNodeByQuery(c, query)
	if err != nil {
		return nil, err
	}
	if query.Brief {
		return buildListNodeBriefResponse(totalCount, nodes)
	} else {
		return h.buildListNodeResponse(ctx, query, totalCount, nodes)
	}
}

// listNodeByQuery retrieves nodes based on the provided query parameters.
// Applies filtering, authorization checks, and pagination to return
// a list of nodes that match the criteria.
func (h *Handler) listNodeByQuery(c *gin.Context, query *types.ListNodeRequest) (int, []*v1.Node, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return 0, nil, err
	}

	labelSelector, err := buildNodeLabelSelector(query)
	if err != nil {
		return 0, nil, err
	}
	nodeList := &v1.NodeList{}
	ctx := c.Request.Context()
	if query.NodeId == nil {
		if err = h.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
			return 0, nil, err
		}
	} else {
		// If a nodeId is provided, you can directly get it to save time.
		node, err := h.getAdminNode(ctx, *query.NodeId)
		if err != nil {
			return 0, nil, err
		}
		nodeLabels := labels.Set(node.Labels)
		if !labelSelector.Matches(nodeLabels) {
			return 0, nil, nil
		}
		nodeList.Items = append(nodeList.Items, *node)
	}

	roles := h.auth.GetRoles(ctx, requestUser)
	nodes := make([]*v1.Node, 0, len(nodeList.Items))
	var phases []string
	if query.Phase != nil {
		phases = strings.Split(string(*query.Phase), ",")
	}

	for i, n := range nodeList.Items {
		if err = h.auth.Authorize(authority.Input{
			Context:    ctx,
			Resource:   &n,
			Verb:       v1.ListVerb,
			Workspaces: []string{query.GetWorkspaceId()},
			User:       requestUser,
			Roles:      roles,
		}); err != nil {
			continue
		}
		if query.Available != nil {
			isAvailable, _ := n.CheckAvailable(false)
			if *query.Available != isAvailable {
				continue
			}
		}
		if query.IsAddonsInstalled != nil {
			if *query.IsAddonsInstalled != v1.IsNodeTemplateInstalled(&n) {
				continue
			}
		}
		if query.Phase != nil {
			if !slice.Contains(phases, string(n.GetPhase())) {
				continue
			}
		}
		nodes = append(nodes, &nodeList.Items[i])
	}
	totalCount := len(nodes)
	if totalCount == 0 {
		return 0, nil, nil
	} else if totalCount > 1 {
		sort.Slice(nodes, func(i, j int) bool {
			return nodes[i].Name < nodes[j].Name
		})
	}

	if query.Limit >= 0 {
		start := query.Offset
		end := start + query.Limit
		if start > totalCount {
			return totalCount, nil, nil
		}
		if end > totalCount {
			end = totalCount
		}
		return totalCount, nodes[start:end], nil
	}
	return totalCount, nodes, nil
}

// buildListNodeBriefResponse: constructs a simplified response for node listings.
// Provides basic node information to improve performance when full details are not needed.
func buildListNodeBriefResponse(totalCount int, nodes []*v1.Node) (interface{}, error) {
	result := &types.ListNodeBriefResponse{
		TotalCount: totalCount,
	}
	for _, n := range nodes {
		item := types.NodeBriefResponseItem{
			NodeId:     n.Name,
			NodeName:   v1.GetDisplayName(n),
			InternalIP: n.Spec.PrivateIP,
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// buildListNodeResponse: constructs a detailed response for node listings.
// Includes comprehensive node information with resource usage and workspace details.
func (h *Handler) buildListNodeResponse(ctx context.Context,
	query *types.ListNodeRequest, totalCount int, nodes []*v1.Node) (interface{}, error) {
	allUsedResource, err := h.getAllUsedResourcePerNode(ctx, query)
	if err != nil {
		return nil, err
	}
	result := &types.ListNodeResponse{
		TotalCount: totalCount,
	}
	for i, n := range nodes {
		var item types.NodeResponseItem
		usedResource, _ := allUsedResource[n.Name]
		item = cvtToNodeResponseItem(n, usedResource)
		if item.Workspace.Id != "" {
			if i > 0 && item.Workspace.Id == result.Items[i-1].Workspace.Id {
				item.Workspace.Name = result.Items[i-1].Workspace.Name
			} else if item.Workspace.Name, err = h.getWorkspaceDisplayName(ctx, item.Workspace.Id); err != nil {
				return nil, err
			}
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// getNode: implements the logic for retrieving a single node's detailed information.
// Authorizes access, retrieves the node, and includes resource usage data.
func (h *Handler) getNode(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	node, err := h.getAdminNode(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    ctx,
		Resource:   node,
		Verb:       v1.GetVerb,
		Workspaces: []string{v1.GetWorkspaceId(node)},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	usedResource, err := h.getUsedResource(ctx, node)
	if err != nil {
		klog.ErrorS(err, "failed to get used resource", "node", node.Name)
		return nil, err
	}
	result := cvtToGetNodeResponse(node, usedResource)
	if result.Workspace.Id != "" {
		if result.Workspace.Name, err = h.getWorkspaceDisplayName(ctx, result.Workspace.Id); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// patchNode: implements partial update logic for a node.
// Applies specified changes with conflict resolution and retry mechanisms.
func (h *Handler) patchNode(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	nodeId := c.GetString(common.Name)
	node, err := h.getAdminNode(ctx, nodeId)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    ctx,
		Resource:   node,
		Verb:       v1.UpdateVerb,
		Workspaces: []string{v1.GetWorkspaceId(node)},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.PatchNodeRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	maxRetry := 3
	if err = backoff.ConflictRetry(func() error {
		shouldUpdate, innerErr := h.updateNode(ctx, node, req)
		if innerErr != nil || !shouldUpdate {
			return innerErr
		}
		innerErr = h.Update(ctx, node)
		if apierrors.IsConflict(innerErr) {
			h.getAdminNode(ctx, nodeId)
		}
		return innerErr
	}, maxRetry, time.Millisecond*200); err != nil {
		klog.ErrorS(err, "failed to update node", "name", node.Name)
		return nil, err
	}
	klog.Infof("update node, name: %s, request: %v", node.Name, *req)
	return nil, nil
}

// deleteNode: implements node deletion logic.
// Ensures the node is not bound to a cluster and removes it from the system.
func (h *Handler) deleteNode(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	node, err := h.getAdminNode(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    ctx,
		Resource:   node,
		Verb:       v1.DeleteVerb,
		Workspaces: []string{v1.GetWorkspaceId(node)},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	if v1.GetClusterId(node) != "" {
		cluster, _ := h.getAdminCluster(ctx, v1.GetClusterId(node))
		if cluster != nil {
			return nil, commonerrors.NewInternalError(
				fmt.Sprintf("The node is bound to cluster %s and needs to be unmanaged first", v1.GetClusterId(node)))
		}
	}
	if err = h.Delete(ctx, node); err != nil {
		klog.ErrorS(err, "failed to delete node")
		return nil, err
	}
	klog.Infof("delete node %s", node.Name)
	return nil, nil
}

// getNodePodLog: implements the logic for retrieving node management pod logs.
// Finds the relevant pod and returns its logs in a structured format.
func (h *Handler) getNodePodLog(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:  c.Request.Context(),
		Resource: node,
		Verb:     v1.CreateVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		if commonerrors.IsForbidden(err) {
			return nil, commonerrors.NewForbidden("The user is not allowed to get node's log")
		}
		return nil, err
	}

	clusterName := node.GetSpecCluster()
	if clusterName == "" {
		clusterName = v1.GetClusterId(node)
	}
	if clusterName == "" {
		return nil, commonerrors.NewBadRequest("the node is not bound to any cluster")
	}

	labelSelector := labels.SelectorFromSet(map[string]string{
		v1.ClusterManageClusterLabel: clusterName, v1.ClusterManageNodeLabel: node.Name})
	podName, err := h.getLatestPodName(c, labelSelector)
	if err != nil {
		return nil, commonerrors.NewNotImplemented("Logging service is only available during node managing or unmanaging processes")
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

// getAdminNode: retrieves a node resource by name from the k8s cluster.
// Returns an error if the node doesn't exist or the name is empty.
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

// getAllUsedResourcePerNode: retrieves the amount of resources currently in use on each node.
// Returns a map with the node name as the key, and the value containing the resource usage and associated workload name
func (h *Handler) getAllUsedResourcePerNode(ctx context.Context,
	query *types.ListNodeRequest) (map[string]*resourceInfo, error) {
	result := make(map[string]*resourceInfo)
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
				Id:          w.Name,
				UserId:      v1.GetUserName(w),
				WorkspaceId: w.Spec.Workspace,
			})
		}
	}
	return result, nil
}

// getUsedResource: retrieves resource usage information for a specific node.
// Calculates the resources currently consumed by workloads on the specified node.
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
			Id:          w.Name,
			UserId:      v1.GetUserName(w),
			WorkspaceId: w.Spec.Workspace,
		})
	}
	return result, nil
}

// generateNode: creates a new node object based on the creation request.
// Validates the request parameters and create References for the flavors and templates used internally.
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
	ctx := c.Request.Context()
	nf, err := h.getAdminNodeFlavor(ctx, req.FlavorId)
	if err != nil {
		return nil, err
	}
	node.Spec.NodeFlavor = commonutils.GenObjectReference(nf.TypeMeta, nf.ObjectMeta)

	if req.TemplateId != "" {
		nt, err := h.getAdminNodeTemplate(ctx, req.TemplateId)
		if err != nil {
			return nil, err
		}
		node.Spec.NodeTemplate = commonutils.GenObjectReference(nt.TypeMeta, nt.ObjectMeta)
	}

	secret, err := h.getAdminSecret(ctx, req.SSHSecretId)
	if err != nil {
		return nil, err
	}
	node.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	v1.SetLabel(node, v1.UserIdLabel, c.GetString(common.UserId))
	return node, nil
}

// validateCreateNodeRequest: validates the parameters in a node creation request.
// Ensures required fields like flavorId, privateIP, and SSHSecretId are provided.
func validateCreateNodeRequest(req *types.CreateNodeRequest) error {
	if req.FlavorId == "" {
		return commonerrors.NewBadRequest("the flavorId of request is empty")
	}
	if req.PrivateIP == "" {
		return commonerrors.NewBadRequest("the privateIP of request is empty")
	}
	if req.SSHSecretId == "" {
		return commonerrors.NewBadRequest("the sshSecretId of request is empty")
	}
	return nil
}

// buildNodeLabelSelector: constructs a label selector based on query parameters.
// Used to filter nodes by cluster, workspace, or flavor criteria.
func buildNodeLabelSelector(query *types.ListNodeRequest) (labels.Selector, error) {
	var labelSelector = labels.NewSelector()
	if query.ClusterId != nil {
		var req *labels.Requirement
		if *query.ClusterId == "" {
			req, _ = labels.NewRequirement(v1.ClusterIdLabel, selection.DoesNotExist, nil)
		} else {
			req, _ = labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{*query.ClusterId})
		}
		labelSelector = labelSelector.Add(*req)
	}
	if query.WorkspaceId != nil {
		var req *labels.Requirement
		if *query.WorkspaceId == "" {
			req, _ = labels.NewRequirement(v1.WorkspaceIdLabel, selection.DoesNotExist, nil)
		} else {
			req, _ = labels.NewRequirement(v1.WorkspaceIdLabel, selection.Equals, []string{*query.WorkspaceId})
		}
		labelSelector = labelSelector.Add(*req)
	}
	if query.FlavorId != nil {
		var req *labels.Requirement
		req, _ = labels.NewRequirement(v1.NodeFlavorIdLabel, selection.Equals, []string{*query.FlavorId})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector, nil
}

// parseListNodeQuery: parses and validates the query parameters for node listing.
// Sets default values for pagination and ensures query parameters are valid.
func parseListNodeQuery(c *gin.Context) (*types.ListNodeRequest, error) {
	query := &types.ListNodeRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Limit == 0 {
		query.Limit = types.DefaultQueryLimit
	}
	return query, nil
}

// updateNode: applies updates to a node based on the patch request.
// Handles label updates, taint modifications, flavor/template changes, and port updates.
func (h *Handler) updateNode(ctx context.Context, node *v1.Node, req *types.PatchNodeRequest) (bool, error) {
	shouldUpdate := false
	nodesLabelAction := generateNodeLabelAction(node, req)
	if len(nodesLabelAction) > 0 {
		shouldUpdate = true
	}
	if req.Taints != nil {
		for i, t := range *req.Taints {
			(*req.Taints)[i].Key = commonfaults.GenerateTaintKey(t.Key)
		}
		if err := h.deleteRelatedFaults(ctx, node, *req.Taints); err != nil {
			return false, err
		}
		if !commonfaults.IsTaintsEqualIgnoreOrder(*req.Taints, node.Spec.Taints) {
			node.Spec.Taints = *req.Taints
			shouldUpdate = true
		}
	}
	if req.FlavorId != nil && *req.FlavorId != "" &&
		(node.Spec.NodeFlavor == nil || *req.FlavorId != node.Spec.NodeFlavor.Name) {
		nf, err := h.getAdminNodeFlavor(ctx, *req.FlavorId)
		if err != nil {
			return false, err
		}
		node.Spec.NodeFlavor = commonutils.GenObjectReference(nf.TypeMeta, nf.ObjectMeta)
		nodesLabelAction[v1.NodeFlavorIdLabel] = v1.NodeActionAdd
		shouldUpdate = true
	}
	if req.TemplateId != nil && *req.TemplateId != "" &&
		(node.Spec.NodeTemplate == nil || *req.TemplateId != node.Spec.NodeTemplate.Name) {
		nt, err := h.getAdminNodeTemplate(ctx, *req.TemplateId)
		if err != nil {
			return false, err
		}
		node.Spec.NodeTemplate = commonutils.GenObjectReference(nt.TypeMeta, nt.ObjectMeta)
		shouldUpdate = true
	}
	if req.Port != nil && *req.Port > 0 && *req.Port != node.GetSpecPort() {
		node.Spec.Port = pointer.Int32(*req.Port)
		shouldUpdate = true
	}
	if req.PrivateIP != nil && *req.PrivateIP != node.Spec.PrivateIP {
		node.Spec.PrivateIP = *req.PrivateIP
		shouldUpdate = true
	}
	if len(nodesLabelAction) > 0 {
		v1.SetAnnotation(node, v1.NodeLabelAction, string(jsonutils.MarshalSilently(nodesLabelAction)))
	}
	return shouldUpdate, nil
}

// deleteRelatedFaults: removes fault resources associated with removed taints.
// Ensures that faults corresponding to removed taints are cleaned up.
func (h *Handler) deleteRelatedFaults(ctx context.Context, node *v1.Node, newTaints []corev1.Taint) error {
	if node.GetSpecCluster() == "" {
		return nil
	}
	newTaintKeys := sets.NewSet()
	for _, t := range newTaints {
		newTaintKeys.Insert(t.Key)
	}
	for _, t := range node.Spec.Taints {
		if newTaintKeys.Has(t.Key) {
			continue
		}
		id := commonfaults.GetIdByTaintKey(t.Key)
		faultId := commonfaults.GenerateFaultId(node.Name, id)
		fault, err := h.getAdminFault(ctx, faultId)
		if err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return err
		}
		if fault.GetDeletionTimestamp().IsZero() {
			if err = h.Delete(ctx, fault); err != nil {
				return err
			}
		}
	}
	return nil
}

// generateNodeLabelAction: determines label changes needed for a node update.
// Compares current and requested labels to generate add/remove actions.
func generateNodeLabelAction(node *v1.Node, req *types.PatchNodeRequest) map[string]string {
	nodesLabelAction := make(map[string]string)
	if req.Labels != nil {
		customerLabels := getNodeCustomerLabels(node.Labels)
		for key, val := range customerLabels {
			val2, ok := (*req.Labels)[key]
			if !ok {
				nodesLabelAction[key] = v1.NodeActionRemove
				delete(node.Labels, key)
			} else if val != val2 {
				nodesLabelAction[key] = v1.NodeActionAdd
				v1.SetLabel(node, key, val2)
			}
		}
		for key, val := range *req.Labels {
			if _, ok := customerLabels[key]; !ok {
				nodesLabelAction[key] = v1.NodeActionAdd
				v1.SetLabel(node, key, val)
			}
		}
	}
	return nodesLabelAction
}

// cvtToNodeResponseItem: converts a node object to a response item format.
// Includes resource availability, phase information, and workload details.
func cvtToNodeResponseItem(n *v1.Node, usedResource *resourceInfo) types.NodeResponseItem {
	isAvailable, message := n.CheckAvailable(false)
	result := types.NodeResponseItem{
		NodeBriefResponseItem: types.NodeBriefResponseItem{
			NodeId:     n.Name,
			NodeName:   v1.GetDisplayName(n),
			InternalIP: n.Spec.PrivateIP,
		},
		ClusterId:         v1.GetClusterId(n),
		Phase:             string(n.GetPhase()),
		Available:         isAvailable,
		Message:           message,
		TotalResources:    cvtToResourceList(n.Status.Resources),
		CreationTime:      timeutil.FormatRFC3339(n.CreationTimestamp.Time),
		IsControlPlane:    v1.IsControlPlane(n),
		IsAddonsInstalled: v1.IsNodeTemplateInstalled(n),
	}
	result.Workspace.Id = v1.GetWorkspaceId(n)
	var availResource corev1.ResourceList
	if usedResource != nil && len(usedResource.resource) > 0 {
		availResource = quantity.GetAvailableResource(n.Status.Resources)
		availResource = quantity.SubResource(availResource, usedResource.resource)
		result.Workloads = usedResource.workloads
	} else {
		availResource = quantity.GetAvailableResource(n.Status.Resources)
	}
	result.AvailResources = cvtToResourceList(availResource)
	return result
}

// cvtToGetNodeResponse: converts a node object to a detailed response format.
// Includes all node details, taints, labels, and template information.
func cvtToGetNodeResponse(n *v1.Node, usedResource *resourceInfo) types.GetNodeResponse {
	result := types.GetNodeResponse{
		NodeResponseItem: cvtToNodeResponseItem(n, usedResource),
	}
	result.FlavorId = v1.GetNodeFlavorId(n)
	result.Taints = getPrimusTaints(n.Status.Taints)
	result.CustomerLabels = getNodeCustomerLabels(n.Labels)
	if n.Spec.NodeTemplate != nil {
		result.TemplateId = n.Spec.NodeTemplate.Name
	}
	lastStartupTime := timeutil.CvtStrUnixToTime(v1.GetNodeStartupTime(n))
	result.LastStartupTime = timeutil.FormatRFC3339(lastStartupTime)
	return result
}

// getNodeCustomerLabels: extracts customer-defined labels from a node's label set.
// Filters out system labels to return only user-defined labels.
func getNodeCustomerLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)
	for key, val := range labels {
		if strings.HasPrefix(key, v1.PrimusSafePrefix) || key == v1.KubernetesControlPlane {
			continue
		}
		result[key] = val
	}
	return result
}

// getPrimusTaints: extracts Primus-specific taints from a list of taints.
// Removes the Primus prefix and returns only the relevant taints.
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
