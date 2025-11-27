/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
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

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"

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

// CreateNode handles the creation of a new node resource.
// It authorizes the request, parses the request body, generates a node object,
// and creates it in the system. Returns the created node ID on success.
func (h *Handler) CreateNode(c *gin.Context) {
	handle(c, h.createNode)
}

// ListNode handles listing nodes based on query parameters.
// Supports filtering, pagination, and brief response formats.
// Returns a list of nodes that match the query criteria.
func (h *Handler) ListNode(c *gin.Context) {
	handle(c, h.listNode)
}

// ExportNode handles exporting nodes based on query parameters.
// Supports filtering and exporting in various formats.
// Returns an exported file containing the nodes that match the query criteria.
func (h *Handler) ExportNode(c *gin.Context) {
	h.ExportNodeByQuery(c)
}

// GetNode retrieves detailed information about a specific node.
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

// DeleteNode handles deletion of a node resource.
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

// ListNodeRebootLog retrieves reboot logs from the node's management operations.
func (h *Handler) ListNodeRebootLog(c *gin.Context) {
	handle(c, h.listNodeRebootLog)
}

// DeleteNodes handles batch deleting of multiple nodes.
func (h *Handler) DeleteNodes(c *gin.Context) {
	handle(c, h.deleteNodes)
}

// createNode implements the node creation logic.
// Validates the request, generates a node object with specified parameters,
// and persists it in the system.
func (h *Handler) createNode(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.NodeKind,
		Verb:         v1.CreateVerb,
		User:         requestUser,
	}); err != nil {
		return nil, err
	}

	req := &types.CreateNodeRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request")
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	node, err := h.generateNode(c.Request.Context(), requestUser, req, body)
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

// ExportNodeToCSV writes the node information to a CSV file using the provided writer (file or response stream).
func ExportNodeToCSV(nodes *types.ListNodeResponse, writer io.Writer) error {
	w := csv.NewWriter(writer)

	if err := w.Write([]string{
		"id", "internalIP", "workspace", "cluster", "available", "status",
		"gpu(available/total)", "cpu(available/total)", "controlPlane",
	}); err != nil {
		klog.ErrorS(err, "failed to write csv header")
		return err
	}

	for _, node := range nodes.Items {
		var gpuAvail, gpuTotal int64
		var cpuAvail, cpuTotal int64

		if node.AvailResources != nil {
			gpuAvail = node.AvailResources["amd.com/gpu"]
			cpuAvail = node.AvailResources["cpu"]
		}
		if node.TotalResources != nil {
			gpuTotal = node.TotalResources["amd.com/gpu"]
			cpuTotal = node.TotalResources["cpu"]
		}

		record := []string{
			node.NodeId,
			node.InternalIP,
			node.Workspace.Name,
			node.ClusterId,
			fmt.Sprintf("%t", node.Available),
			node.Phase,
			fmt.Sprintf("\t%d/%d", gpuAvail, gpuTotal),
			fmt.Sprintf("\t%d/%d", cpuAvail, cpuTotal),
			fmt.Sprintf("%t", node.IsControlPlane),
		}

		if err := w.Write(record); err != nil {
			klog.ErrorS(err, "failed to write csv record", "node", node.NodeId)
			return err
		}
	}
	w.Flush()

	if err := w.Error(); err != nil {
		klog.ErrorS(err, "csv writer flush error")
		return err
	}

	return nil
}

// ExportNodeByQuery can export nodes based on the provided query parameteres.
func (h *Handler) ExportNodeByQuery(c *gin.Context) {
	query, err := parseListNodeQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		apiutils.AbortWithApiError(c, err)
		return
	}
	ctx := c.Request.Context()
	query.Limit = -1 // Don't need limit.
	totalCount, nodes, err := h.listNodeByQuery(c, query)
	if err != nil {
		klog.ErrorS(err, "failed to query node")
		apiutils.AbortWithApiError(c, err)
		return
	}
	result, err := h.buildListNodeResponse(ctx, query, totalCount, nodes)
	if err != nil {
		klog.ErrorS(err, "failed to build node list")
		apiutils.AbortWithApiError(c, err)
		return
	}
	res, _ := result.(*types.ListNodeResponse) //Don't use brief, so struct is ListNodeResponse
	filename := fmt.Sprintf("node_list_%s.csv", time.Now().Format("20060102_150405"))
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "text/csv; charset=utf-8")
	if err := ExportNodeToCSV(res, c.Writer); err != nil {
		klog.ErrorS(err, "failed to export node to CSV")
		apiutils.AbortWithApiError(c, err)
		return
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

	roles := h.accessController.GetRoles(ctx, requestUser)
	nodes := make([]*v1.Node, 0, len(nodeList.Items))
	var phases []string
	if query.Phase != nil {
		phases = strings.Split(string(*query.Phase), ",")
	}

	for i, n := range nodeList.Items {
		if err = h.accessController.Authorize(authority.AccessInput{
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

// buildListNodeBriefResponse constructs a simplified response for node listings.
// Provides basic node information to improve performance when full details are not needed.
func buildListNodeBriefResponse(totalCount int, nodes []*v1.Node) (interface{}, error) {
	result := &types.ListNodeBriefResponse{
		TotalCount: totalCount,
	}
	for _, n := range nodes {
		result.Items = append(result.Items, convertToNodeBriefResponse(n))
	}
	return result, nil
}

// buildListNodeResponse constructs a detailed response for node listings.
// Includes comprehensive node information with resource usage and workspace details.
func (h *Handler) buildListNodeResponse(ctx context.Context,
	query *types.ListNodeRequest, totalCount int, nodes []*v1.Node) (interface{}, error) {
	allUsedResource, err := h.getAllUsedResourcePerNode(ctx, query)
	if err != nil {
		return nil, err
	}

	// Get GPU utilization from node_statistic table
	clusterId := query.GetClusterId()
	klog.V(4).Infof("Fetching GPU utilization for cluster: %q, node count: %d", clusterId, len(nodes))
	nodeGpuUtilization, err := h.getNodeGpuUtilization(ctx, clusterId, nodes)
	if err != nil {
		klog.ErrorS(err, "failed to get node GPU utilization from node_statistic", "clusterId", clusterId)
		// Don't fail the entire request, just log the error
		nodeGpuUtilization = make(map[string]float64)
	}
	klog.V(4).Infof("Retrieved GPU utilization for %d nodes", len(nodeGpuUtilization))

	result := &types.ListNodeResponse{
		TotalCount: totalCount,
	}
	for i, n := range nodes {
		var item types.NodeResponseItem
		usedResource, _ := allUsedResource[n.Name]
		item = cvtToNodeResponseItem(n, usedResource)

		// Add GPU utilization from node_statistic if available
		if gpuUtil, ok := nodeGpuUtilization[n.Name]; ok {
			item.GpuUtilization = &gpuUtil
			klog.V(5).Infof("Node %s: GPU utilization = %.2f%%", n.Name, gpuUtil)
		} else {
			klog.V(5).Infof("Node %s: GPU utilization not found in map (map size: %d)", n.Name, len(nodeGpuUtilization))
		}

		if item.Workspace.Id != "" {
			if i > 0 && item.Workspace.Id == result.Items[i-1].Workspace.Id {
				item.Workspace.Name = result.Items[i-1].Workspace.Name
			} else if item.Workspace.Name, err = h.getWorkspaceDisplayName(ctx, item.Workspace.Id); err != nil {
				klog.ErrorS(err, "failed to get workspace display name", "workspaceId", item.Workspace.Id)
			}
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

// getNode implements the logic for retrieving a single node's detailed information.
// Authorizes access, retrieves the node, and includes resource usage data.
func (h *Handler) getNode(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	node, err := h.getAdminNode(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
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
			klog.ErrorS(err, "failed to get workspace display name", "workspaceId", result.Workspace.Id)
		}
	}
	return result, nil
}

// patchNode implements partial update logic for a node.
// Applies specified changes with conflict resolution and retry mechanisms.
func (h *Handler) patchNode(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	ctx := c.Request.Context()
	nodeId := c.GetString(common.Name)
	node, err := h.getAdminNode(ctx, nodeId)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   node,
		Verb:       v1.UpdateVerb,
		Workspaces: []string{v1.GetWorkspaceId(node)},
		User:       requestUser,
	}); err != nil {
		return nil, err
	}

	req := &types.PatchNodeRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}
	
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
	}, defaultRetryCount, defaultRetryDelay); err != nil {
		klog.ErrorS(err, "failed to update node", "name", node.Name)
		return nil, err
	}
	klog.Infof("update node, name: %s, request: %v. user: %s/%s",
		node.Name, *req, c.GetString(common.UserName), c.GetString(common.UserId))
	return nil, nil
}

// deleteNode implements node deletion logic.
// Ensures the node is not bound to a cluster and removes it from the system.
func (h *Handler) deleteNode(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)
	name := c.GetString(common.Name)
	return h.deleteNodeImpl(c, name, requestUser, roles)
}

func (h *Handler) deleteNodeImpl(c *gin.Context, name string, requestUser *v1.User, roles []*v1.Role) (interface{}, error) {
	ctx := c.Request.Context()
	node, err := h.getAdminNode(ctx, name)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   node,
		Verb:       v1.DeleteVerb,
		Workspaces: []string{v1.GetWorkspaceId(node)},
		User:       requestUser,
		Roles:      roles,
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

// getNodePodLog implements the logic for retrieving node management pod logs.
// Finds the relevant pod and returns its logs in a structured format.
func (h *Handler) getNodePodLog(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
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

// listNodeRebootLog implements the logic for retrieving node reboot logs.
func (h *Handler) listNodeRebootLog(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: node,
		Verb:     v1.GetVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		if commonerrors.IsForbidden(err) {
			return nil, commonerrors.NewForbidden("The user is not allowed to get node's log")
		}
		return nil, err
	}

	req := &types.ListNodeRebootLogRequest{}
	if err = c.ShouldBindWith(req, binding.Query); err != nil {
		klog.Errorf("failed to parse query err: %v", err)
		return nil, err
	}

	dbSql, orderBy := cvtToListNodeRebootSql(req, node)
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, orderBy, req.Limit, req.Offset)
	if err != nil {
		return nil, err
	}
	count, err := h.dbClient.CountJobs(c.Request.Context(), dbSql)
	if err != nil {
		return nil, err
	}
	result := &types.ListNodeRebootLogResponse{
		TotalCount: count,
	}
	for _, job := range jobs {
		result.Items = append(result.Items, types.NodeRebootLogResponseItem{
			UserId:       dbutils.ParseNullString(job.UserId),
			UserName:     dbutils.ParseNullString(job.UserName),
			CreationTime: dbutils.ParseNullTimeToString(job.CreationTime),
		})
	}

	return result, nil
}

// getAdminNode retrieves a node resource by name from the k8s cluster.
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

// getAllUsedResourcePerNode retrieves the amount of resources currently in use on each node.
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
			info.workloads = append(info.workloads, generateWorkloadInfo(w))
		}
	}
	return result, nil
}

// getUsedResource retrieves resource usage information for a specific node.
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
		result.workloads = append(result.workloads, generateWorkloadInfo(w))
	}
	return result, nil
}

// generateNode creates a new node object based on the creation request.
// Validates the request parameters and create References for the flavors and templates used internally.
func (h *Handler) generateNode(ctx context.Context, requestUser *v1.User, req *types.CreateNodeRequest, body []byte) (*v1.Node, error) {
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

	secret, err := h.getAndAuthorizeSecret(ctx, req.SSHSecretId, "", requestUser, v1.GetVerb)
	if err != nil {
		return nil, err
	}
	node.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	v1.SetLabel(node, v1.UserIdLabel, requestUser.Name)
	return node, nil
}

// validateCreateNodeRequest validates CreateNodeRequest and returns an error if validation fails.
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

// buildNodeLabelSelector constructs a label selector based on query parameters.
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

// parseListNodeQuery parses and validates the query parameters for node listing.
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

// updateNode applies updates to a node based on the patch request.
// Handles label updates, taint modifications, flavor/template changes, and port updates.
func (h *Handler) updateNode(ctx context.Context, node *v1.Node, req *types.PatchNodeRequest) (bool, error) {
	shouldUpdate := false
	nodesLabelAction := generateNodeLabelAction(node, req)
	if len(nodesLabelAction) > 0 {
		shouldUpdate = true
	}
	if req.Taints != nil {
		for i, t := range *req.Taints {
			key := t.Key
			(*req.Taints)[i].Key = commonfaults.GenerateTaintKey(key)
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

// deleteRelatedFaults removes fault resources associated with removed taints.
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

// deleteNodes implements batch node deleting logic.
// Processes multiple nodes deleting concurrently with error handling.
func (h *Handler) deleteNodes(c *gin.Context) (interface{}, error) {
	return h.handleBatchNodes(c, BatchDelete)
}

// handleBatchNodes processes batch operations on multiple nodes.
// Supports delete actions with concurrent execution.
func (h *Handler) handleBatchNodes(c *gin.Context, action WorkloadBatchAction) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	req := &types.BatchNodesRequest{}
	if _, err = apiutils.ParseRequestBody(c.Request, req); err != nil {
		return nil, err
	}
	count := len(req.NodeIds)
	ch := make(chan string, count)
	defer close(ch)
	for _, id := range req.NodeIds {
		ch <- id
	}

	success, err := concurrent.Exec(count, func() error {
		nodeId := <-ch
		var innerErr error
		switch action {
		case BatchDelete:
			_, innerErr = h.deleteNodeImpl(c, nodeId, requestUser, roles)
		default:
			return commonerrors.NewInternalError("invalid action")
		}
		return innerErr
	})
	if success == 0 {
		return nil, commonerrors.NewInternalError(err.Error())
	}
	return nil, nil
}

// generateNodeLabelAction determines label changes needed for a node update.
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

// cvtToNodeResponseItem converts a node object to a response item format.
// Includes resource availability, phase information, and workload details.
func cvtToNodeResponseItem(node *v1.Node, usedResource *resourceInfo) types.NodeResponseItem {
	result := types.NodeResponseItem{
		NodeBriefResponseItem: convertToNodeBriefResponse(node),
		ClusterId:             v1.GetClusterId(node),
		Phase:                 string(node.GetPhase()),
		TotalResources:        cvtToResourceList(node.Status.Resources),
		CreationTime:          timeutil.FormatRFC3339(node.CreationTimestamp.Time),
		IsControlPlane:        v1.IsControlPlane(node),
		IsAddonsInstalled:     v1.IsNodeTemplateInstalled(node),
	}
	result.Workspace.Id = v1.GetWorkspaceId(node)
	var availResource corev1.ResourceList
	if usedResource != nil && len(usedResource.resource) > 0 {
		availResource = quantity.GetAvailableResource(node.Status.Resources)
		availResource = quantity.SubResource(availResource, usedResource.resource)
		result.Workloads = usedResource.workloads
	} else {
		availResource = quantity.GetAvailableResource(node.Status.Resources)
	}
	result.AvailResources = cvtToResourceList(availResource)
	return result
}

// cvtToGetNodeResponse converts a node object to a detailed response format.
// Includes all node details, taints, labels, and template information.
func cvtToGetNodeResponse(n *v1.Node, usedResource *resourceInfo) types.GetNodeResponse {
	result := types.GetNodeResponse{
		NodeResponseItem: cvtToNodeResponseItem(n, usedResource),
	}
	result.FlavorId = v1.GetNodeFlavorId(n)
	result.Taints = getPrimusTaints(n.Status.Taints)
	result.Labels = getNodeCustomerLabels(n.Labels)
	if n.Spec.NodeTemplate != nil {
		result.TemplateId = n.Spec.NodeTemplate.Name
	}
	lastStartupTime := timeutil.CvtStrUnixToTime(v1.GetNodeStartupTime(n))
	result.LastStartupTime = timeutil.FormatRFC3339(lastStartupTime)
	return result
}

// convertToNodeBriefResponse converts a node object to a brief response format.
// Returns basic node information including ID, name, internal IP, availability status, and message.
func convertToNodeBriefResponse(node *v1.Node) types.NodeBriefResponseItem {
	isAvailable, message := node.CheckAvailable(false)
	return types.NodeBriefResponseItem{
		NodeId:     node.Name,
		NodeName:   v1.GetDisplayName(node),
		InternalIP: node.Spec.PrivateIP,
		Available:  isAvailable,
		Message:    message,
	}
}

// getNodeCustomerLabels extracts customer-defined labels from a node's label set.
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

// getPrimusTaints extracts Primus-specific taints from a list of taints.
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

// cvtToListNodeRebootSql converts the reboot log query parameters into SQL conditions and order by clauses.
// It filters jobs that are not deleted, related to the given node, and of reboot type.
// Time range filters and sorting options are applied if specified in the query.
func cvtToListNodeRebootSql(query *types.ListNodeRebootLogRequest, node *v1.Node) (sqrl.Sqlizer, []string) {
	dbTags := dbclient.GetOpsJobFieldTags()
	creationTime := dbclient.GetFieldTag(dbTags, "CreationTime")
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Expr("outputs::jsonb @> ?", fmt.Sprintf(`[{"value": "%s"}]`, node.Name)),
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "type"): v1.OpsJobRebootType},
	}
	if !query.SinceTime.IsZero() {
		dbSql = append(dbSql, sqrl.GtOrEq{creationTime: query.SinceTime})
	}
	if !query.UntilTime.IsZero() {
		dbSql = append(dbSql, sqrl.LtOrEq{creationTime: query.UntilTime})
	}
	orderBy := buildOrderBy(query.SortBy, query.Order, dbTags)
	return dbSql, orderBy
}

// generateWorkloadInfo creates a WorkloadInfo struct from a workload object.
func generateWorkloadInfo(workload *v1.Workload) types.WorkloadInfo {
	return types.WorkloadInfo{
		Id:          workload.Name,
		Kind:        workload.Spec.Kind,
		UserId:      v1.GetUserName(workload),
		WorkspaceId: workload.Spec.Workspace,
	}
}

// getNodeGpuUtilization retrieves GPU utilization for nodes from node_statistic table
// Returns a map with node name as key and GPU utilization as value
func (h *Handler) getNodeGpuUtilization(ctx context.Context, clusterId string, nodes []*v1.Node) (map[string]float64, error) {
	if len(nodes) == 0 {
		return make(map[string]float64), nil
	}

	// If dbClient is not initialized, return empty map
	// This can happen in test scenarios or when database is not configured
	if h.dbClient == nil {
		klog.V(4).Info("dbClient is nil, returning empty GPU utilization map")
		return make(map[string]float64), nil
	}

	// Build node names list
	nodeNames := make([]string, 0, len(nodes))
	for _, node := range nodes {
		nodeNames = append(nodeNames, node.Name)
	}

	klog.V(4).Infof("Querying GPU utilization from DB - cluster: %q, nodes: %v", clusterId, nodeNames)
	// Use database client to query node_statistic table
	result, err := h.dbClient.GetNodeGpuUtilizationMap(ctx, clusterId, nodeNames)
	if err != nil {
		return nil, err
	}
	klog.V(4).Infof("DB query returned %d entries: %v", len(result), result)
	return result, nil
}
