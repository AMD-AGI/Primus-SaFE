/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func (h *Handler) CreateWorkspace(c *gin.Context) {
	handle(c, h.createWorkspace)
}

func (h *Handler) ListWorkspace(c *gin.Context) {
	handle(c, h.listWorkspace)
}

func (h *Handler) GetWorkspace(c *gin.Context) {
	handle(c, h.getWorkspace)
}

func (h *Handler) DeleteWorkspace(c *gin.Context) {
	handle(c, h.deleteWorkspace)
}

func (h *Handler) PatchWorkspace(c *gin.Context) {
	handle(c, h.patchWorkspace)
}

func (h *Handler) ProcessWorkspaceNodes(c *gin.Context) {
	handle(c, h.processWorkspaceNodes)
}

func (h *Handler) createWorkspace(c *gin.Context) (interface{}, error) {
	if err := h.auth.Authorize(authority.Input{
		Context:      c.Request.Context(),
		ResourceKind: v1.WorkspaceKind,
		Verb:         v1.CreateVerb,
		UserId:       c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.CreateWorkspaceRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", string(body))
		return nil, err
	}
	workspace := generateWorkspace(c, req)
	err = h.Create(c.Request.Context(), workspace)
	if err != nil {
		klog.ErrorS(err, "failed to create", "workspace", workspace)
		return nil, err
	}
	klog.Infof("create workspace, name: %s", workspace.Name)
	return &types.CreateWorkspaceResponse{
		WorkspaceId: workspace.Name,
	}, nil
}

func (h *Handler) listWorkspace(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	query, err := parseListWorkspaceQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	labelSelector, err := buildListWorkspaceSelector(query)
	if err != nil {
		return nil, err
	}

	ctx := c.Request.Context()
	workspaceList := &v1.WorkspaceList{}
	if err = h.List(ctx, workspaceList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	sort.Slice(workspaceList.Items, func(i, j int) bool {
		return workspaceList.Items[i].Name < workspaceList.Items[j].Name
	})
	roles := h.auth.GetRoles(ctx, requestUser)
	result := &types.ListWorkspaceResponse{}
	for _, w := range workspaceList.Items {
		if err = h.auth.Authorize(authority.Input{
			Context:    ctx,
			Resource:   &w,
			Verb:       v1.ListVerb,
			User:       requestUser,
			Workspaces: []string{w.Name},
			Roles:      roles,
		}); err != nil {
			continue
		}
		item := cvtToWorkspaceResponseItem(&w)
		result.Items = append(result.Items, item)
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getWorkspace(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.GetVerb,
		Workspaces: []string{workspace.Name},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	result, err := h.cvtToGetWorkspaceResponse(ctx, workspace)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *Handler) deleteWorkspace(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.DeleteVerb,
		Workspaces: []string{workspace.Name},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}
	if err = h.Delete(ctx, workspace); err != nil {
		klog.ErrorS(err, "failed to delete workspace")
		return nil, err
	}
	klog.Infof("delete workspace %s", workspace.Name)
	return nil, nil
}

func (h *Handler) patchWorkspace(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.UpdateVerb,
		Workspaces: []string{workspace.Name},
		UserId:     c.GetString(common.UserId),
	}); err != nil {
		return nil, err
	}

	req := &types.PatchWorkspaceRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	patch := client.MergeFrom(workspace.DeepCopy())
	updateWorkspace(workspace, req)
	if err = h.Patch(ctx, workspace, patch); err != nil {
		klog.ErrorS(err, "failed to patch workspace", "data", string(body))
		return nil, err
	}
	klog.Infof("patch workspace, name: %s, request: %s", workspace.Name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

func updateWorkspace(workspace *v1.Workspace, req *types.PatchWorkspaceRequest) {
	if req.Description != nil {
		v1.SetAnnotation(workspace, v1.DescriptionAnnotation, *req.Description)
	}
	if req.NodeFlavor != nil {
		workspace.Spec.NodeFlavor = *req.NodeFlavor
	}
	if req.Replica != nil {
		workspace.Spec.Replica = *req.Replica
	}
	if req.QueuePolicy != nil {
		workspace.Spec.QueuePolicy = *req.QueuePolicy
	}
	if req.Scopes != nil {
		workspace.Spec.Scopes = *req.Scopes
	}
	if req.Volumes != nil {
		workspace.Spec.Volumes = *req.Volumes
	}
	if req.EnablePreempt != nil {
		workspace.Spec.EnablePreempt = *req.EnablePreempt
	}
	if req.Managers != nil {
		workspace.Spec.Managers = *req.Managers
	}
}

func (h *Handler) getAdminWorkspace(ctx context.Context, name string) (*v1.Workspace, error) {
	if name == "" {
		return nil, commonerrors.NewBadRequest("the workspaceId is empty")
	}
	workspace := &v1.Workspace{}
	err := h.Get(ctx, client.ObjectKey{Name: name}, workspace)
	if err != nil {
		klog.ErrorS(err, "failed to get admin workspace")
		return nil, err
	}
	return workspace.DeepCopy(), nil
}

func (h *Handler) processWorkspaceNodes(c *gin.Context) (interface{}, error) {
	req, err := parseProcessNodesRequest(c)
	if err != nil {
		return nil, err
	}
	return nil, h.updateWorkspaceNodesAction(c, c.GetString(types.Name), req.Action, req.NodeIds)
}

func (h *Handler) updateWorkspaceNodesAction(c *gin.Context, workspaceId, action string, nodeIds []string) error {
	nodeAction := commonnodes.BuildAction(action, nodeIds...)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		workspace := &v1.Workspace{}
		if err := h.Get(c.Request.Context(), client.ObjectKey{Name: workspaceId}, workspace); err != nil {
			return err
		}
		if err := h.auth.Authorize(authority.Input{
			Context:    c.Request.Context(),
			Resource:   workspace,
			Verb:       v1.UpdateVerb,
			Workspaces: []string{workspaceId},
			UserId:     c.GetString(common.UserId),
		}); err != nil {
			return err
		}
		v1.SetAnnotation(workspace, v1.WorkspaceNodesAction, nodeAction)
		if err := h.Update(c.Request.Context(), workspace); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func generateWorkspace(c *gin.Context, req *types.CreateWorkspaceRequest) *v1.Workspace {
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: stringutil.NormalizeName(req.Cluster + "-" + req.Name),
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
				v1.UserIdLabel:      c.GetString(common.UserId),
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation: req.Description,
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:       req.Cluster,
			NodeFlavor:    req.NodeFlavor,
			Replica:       req.Replica,
			QueuePolicy:   v1.WorkspaceQueuePolicy(req.QueuePolicy),
			Volumes:       req.Volumes,
			Scopes:        req.Scopes,
			EnablePreempt: req.EnablePreempt,
			Managers:      req.Managers,
		},
	}
	if len(workspace.Spec.Scopes) == 0 {
		workspace.Spec.Scopes = []v1.WorkspaceScope{v1.TrainScope, v1.InferScope, v1.AuthoringScope}
	}
	return workspace
}

func parseListWorkspaceQuery(c *gin.Context) (*types.ListWorkspaceRequest, error) {
	query := &types.ListWorkspaceRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

func buildListWorkspaceSelector(query *types.ListWorkspaceRequest) (labels.Selector, error) {
	var labelSelector = labels.NewSelector()
	if query.ClusterId != "" {
		req, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{query.ClusterId})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector, nil
}

func cvtToWorkspaceResponseItem(w *v1.Workspace) types.WorkspaceResponseItem {
	result := types.WorkspaceResponseItem{
		WorkspaceId:   w.Name,
		WorkspaceName: v1.GetDisplayName(w),
		ClusterId:     w.Spec.Cluster,
		NodeFlavor:    w.Spec.NodeFlavor,
		UserId:        v1.GetUserId(w),
		TotalNode:     w.Spec.Replica,
		Phase:         string(w.Status.Phase),
		CreateTime:    timeutil.FormatRFC3339(&w.CreationTimestamp.Time),
		Description:   v1.GetDescription(w),
		QueuePolicy:   w.Spec.QueuePolicy,
		Scopes:        w.Spec.Scopes,
		Volumes:       w.Spec.Volumes,
		EnablePreempt: w.Spec.EnablePreempt,
		AbnormalNode:  w.Status.AbnormalReplica,
		Managers:      w.Spec.Managers,
	}
	return result
}

func (h *Handler) cvtToGetWorkspaceResponse(ctx context.Context, workspace *v1.Workspace) (*types.GetWorkspaceResponse, error) {
	result := &types.GetWorkspaceResponse{
		WorkspaceResponseItem: cvtToWorkspaceResponseItem(workspace),
	}
	availableNode := workspace.Status.AvailableReplica
	nf, err := h.getAdminNodeFlavor(ctx, workspace.Spec.NodeFlavor)
	if err != nil {
		return nil, err
	}
	nfResource := nf.ToResourceList(commonconfig.GetRdmaName())

	totalQuota := quantity.MultiResource(nfResource, int64(availableNode+result.AbnormalNode))
	abnormalQuota := quantity.MultiResource(nfResource, int64(result.AbnormalNode))
	result.TotalQuota = cvtToResourceList(totalQuota)
	result.AbnormalQuota = cvtToResourceList(abnormalQuota)

	usedQuota, err := h.getWorkspaceUsedQuota(ctx, workspace)
	if err != nil {
		return nil, err
	}
	result.UsedQuota = cvtToResourceList(usedQuota)

	availQuota := quantity.MultiResource(nfResource, int64(availableNode))
	availQuota = quantity.GetAvailableResource(availQuota)
	result.AvailQuota = cvtToResourceList(quantity.SubResource(availQuota, usedQuota))
	return result, nil
}

func (h *Handler) getWorkspaceUsedQuota(ctx context.Context, workspace *v1.Workspace) (corev1.ResourceList, error) {
	filterNode := func(nodeName string) bool {
		n, err := h.getAdminNode(ctx, nodeName)
		if err != nil {
			return true
		}
		if !n.IsAvailable(false) {
			return true
		}
		return false
	}

	workspaceNames := []string{workspace.Name}
	workloads, err := h.getRunningWorkloads(ctx, workspace.Spec.Cluster, workspaceNames)
	if err != nil || len(workloads) == 0 {
		return nil, err
	}
	var usedQuota corev1.ResourceList
	for _, w := range workloads {
		res, err := commonworkload.GetActiveResources(w, filterNode)
		if err != nil {
			return nil, err
		}
		if res != nil {
			usedQuota = quantity.AddResource(usedQuota, res)
		}
	}
	return usedQuota, nil
}
