/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const (
	RedfishUrl = "https://%s/redfish/v1/Systems/1/Actions/ComputerSystem.Reset"
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

func (h *Handler) RestartNode(c *gin.Context) {
	handle(c, h.restartNode)
}

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
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	query, err := parseListNodeQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	labelSelector, err := buildNodeLabelSelector(query)
	if err != nil {
		return nil, err
	}
	ctx := c.Request.Context()
	nodeList := &v1.NodeList{}
	if err = h.List(ctx, nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		klog.ErrorS(err, "failed to list admin nodes", "labelSelector", labelSelector)
		return nil, err
	}
	result := &types.ListNodeResponse{}
	if len(nodeList.Items) == 0 {
		return result, nil
	}

	allUsedResource, err := h.getAllUsedResourcePerNode(ctx, query)
	if err != nil {
		return nil, err
	}
	roles := h.auth.GetRoles(ctx, requestUser)
	sort.Slice(nodeList.Items, func(i, j int) bool {
		return nodeList.Items[i].Name < nodeList.Items[j].Name
	})
	for _, n := range nodeList.Items {
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
		usedResource, _ := allUsedResource[n.Name]
		item, err := h.cvtToNodeResponseItem(ctx, &n, usedResource)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, item)
		result.TotalCount++
	}
	return result, nil
}
func (h *Handler) getNode(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	node, err := h.getAdminNode(ctx, c.GetString(types.Name))
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
	result, err := h.cvtToNodeResponseItem(c.Request.Context(), node, usedResource)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *Handler) patchNode(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	nodeId := c.GetString(types.Name)
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
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	maxRetry := 3
	if err = backoff.ConflictRetry(func() error {
		isShouldUpdate, innerErr := h.updateNode(ctx, node, req)
		if innerErr != nil || !isShouldUpdate {
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

func (h *Handler) deleteNode(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	node, err := h.getAdminNode(ctx, c.GetString(types.Name))
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

func (h *Handler) getNodePodLog(c *gin.Context) (interface{}, error) {
	node, err := h.getAdminNode(c.Request.Context(), c.GetString(types.Name))
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

func (h *Handler) restartNode(c *gin.Context) (interface{}, error) {
	if err := h.auth.AuthorizeSystemAdmin(authority.Input{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	node, err := h.getAdminNode(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if v1.GetNodeBMCIp(node) == "" || v1.GetNodeBMCPassword(node) == "" {
		return nil, commonerrors.NewInternalError("BMC IP or password is not found")
	}
	req := &types.RebootNodeRequest{}
	if _, err = getBodyFromRequest(c.Request, req); err != nil {
		klog.ErrorS(err, "failed to parse request")
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	url := fmt.Sprintf(RedfishUrl, v1.GetNodeBMCIp(node))
	var body []byte
	if req.Force != nil && *req.Force {
		body = []byte(`{"ResetType": "PowerCycle"}`)
	} else {
		body = []byte(`{"ResetType": "GracefulRestart"}`)
	}

	klog.Infof("restart node, url: %s, body: %s", url, string(body))
	resetReq, err := httpclient.BuildRequest(url, http.MethodPost, body)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	resetReq.SetBasicAuth("ADMIN", v1.GetNodeBMCPassword(node))

	resp, err := h.httpClient.Do(resetReq)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("%s", string(resp.Body))
	}

	return string(resp.Body), nil
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
	ctx := c.Request.Context()
	nf, err := h.getAdminNodeFlavor(ctx, req.FlavorName)
	if err != nil {
		return nil, err
	}
	node.Spec.NodeFlavor = commonutils.GenObjectReference(nf.TypeMeta, nf.ObjectMeta)

	if req.TemplateName != "" {
		nt, err := h.getAdminNodeTemplate(ctx, req.TemplateName)
		if err != nil {
			return nil, err
		}
		node.Spec.NodeTemplate = commonutils.GenObjectReference(nt.TypeMeta, nt.ObjectMeta)
	}

	secret, err := h.getAdminSecret(ctx, req.SSHSecretName)
	if err != nil {
		return nil, err
	}
	node.Spec.SSHSecret = commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta)
	if req.BMCIp != "" {
		v1.SetAnnotation(node, v1.NodeBMCIpAnnotation, req.BMCIp)
	}
	if req.BMCPassword != "" {
		v1.SetAnnotation(node, v1.NodeBMCPasswordAnnotation, req.BMCPassword)
	}
	v1.SetLabel(node, v1.UserIdLabel, c.GetString(common.UserId))
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
	nodesLabelAction := genNodeLabelAction(node, req)
	if len(nodesLabelAction) > 0 {
		isShouldUpdate = true
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
	if req.NodeTemplate != nil && *req.NodeTemplate != "" &&
		(node.Spec.NodeTemplate == nil || *req.NodeTemplate != node.Spec.NodeTemplate.Name) {
		nt, err := h.getAdminNodeTemplate(ctx, *req.NodeTemplate)
		if err != nil {
			return false, err
		}
		node.Spec.NodeTemplate = commonutils.GenObjectReference(nt.TypeMeta, nt.ObjectMeta)
		isShouldUpdate = true
	}
	if req.Port != nil && *req.Port > 0 && *req.Port != node.GetSpecPort() {
		node.Spec.Port = pointer.Int32(*req.Port)
		isShouldUpdate = true
	}
	if req.BMCIp != nil && v1.SetAnnotation(node, v1.NodeBMCIpAnnotation, *req.BMCIp) {
		isShouldUpdate = true
	}
	if req.BMCPassword != nil && v1.SetAnnotation(node, v1.NodeBMCPasswordAnnotation, *req.BMCPassword) {
		isShouldUpdate = true
	}
	if len(nodesLabelAction) > 0 {
		v1.SetAnnotation(node, v1.NodeLabelAction, string(jsonutils.MarshalSilently(nodesLabelAction)))
	}
	return isShouldUpdate, nil
}

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
		faultName := commonfaults.GenerateFaultName(node.Name, id)
		fault, err := h.getAdminFault(ctx, faultName)
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

func genNodeLabelAction(node *v1.Node, req *types.PatchNodeRequest) map[string]string {
	nodesLabelAction := make(map[string]string)
	if req.Labels != nil {
		for key, val := range node.Labels {
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
			if _, ok := node.Labels[key]; !ok {
				nodesLabelAction[key] = v1.NodeActionAdd
				v1.SetLabel(node, key, val)
			}
		}
	}
	return nodesLabelAction
}

func (h *Handler) cvtToNodeResponseItem(ctx context.Context, n *v1.Node, usedResource *resourceInfo) (types.NodeResponseItem, error) {
	avail, message := n.CheckAvailable(false)
	result := types.NodeResponseItem{
		NodeId:            n.Name,
		DisplayName:       v1.GetDisplayName(n),
		Cluster:           v1.GetClusterId(n),
		Phase:             string(n.Status.MachineStatus.Phase),
		InternalIP:        n.Spec.PrivateIP,
		BMCIP:             v1.GetNodeBMCIp(n),
		NodeFlavor:        v1.GetNodeFlavorId(n),
		Available:         avail,
		Message:           message,
		Taints:            getPrimusTaints(n.Status.Taints),
		TotalResources:    cvtToResourceList(n.Status.Resources),
		CustomerLabels:    getNodeCustomerLabels(n.Labels),
		CreationTime:      timeutil.FormatRFC3339(&n.CreationTimestamp.Time),
		IsControlPlane:    v1.IsControlPlane(n),
		IsAddonsInstalled: v1.IsNodeTemplateInstalled(n),
	}
	result.Workspace.Id = v1.GetWorkspaceId(n)
	if result.Workspace.Id != "" {
		workspace := &v1.Workspace{}
		if err := h.Get(ctx, client.ObjectKey{Name: v1.GetWorkspaceId(n)}, workspace); err != nil {
			return types.NodeResponseItem{}, err
		} else {
			result.Workspace.Name = v1.GetDisplayName(workspace)
		}
	}
	if n.Status.ClusterStatus.Phase == v1.NodeManagedFailed || n.Status.ClusterStatus.Phase == v1.NodeUnmanagedFailed ||
		n.Status.ClusterStatus.Phase == v1.NodeManaging || n.Status.ClusterStatus.Phase == v1.NodeUnmanaging {
		result.Phase = string(n.Status.ClusterStatus.Phase)
	}
	if n.Spec.NodeTemplate != nil {
		result.NodeTemplate = n.Spec.NodeTemplate.Name
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
	return result, nil
}

func getNodeCustomerLabels(labels map[string]string) map[string]string {
	result := make(map[string]string)
	for key, val := range labels {
		if strings.HasPrefix(key, v1.PrimusSafePrefix) || key == common.K8sHostNameLabel {
			continue
		}
		result[key] = val
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
