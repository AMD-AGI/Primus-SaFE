/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
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

func (h *Handler) createWorkspace(c *gin.Context) (interface{}, error) {
	req := &types.CreateWorkspaceRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", string(body))
		return nil, err
	}

	workspace := generateWorkspace(req)
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

func generateWorkspace(req *types.CreateWorkspaceRequest) *v1.Workspace {
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Cluster + "-" + req.Name,
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation:         req.Description,
				v1.QueueBalanceTimeoutAnnotation: strconv.Itoa(req.QueueBalanceTimeout),
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:     req.Cluster,
			NodeFlavor:  req.NodeFlavor,
			Replica:     req.Replica,
			QueuePolicy: v1.WorkspaceQueuePolicy(req.QueuePolicy),
			Volumes:     req.Volumes,
			Scopes:      req.Scopes,
		},
	}
	if len(workspace.Spec.Scopes) == 0 {
		workspace.Spec.Scopes = []v1.WorkspaceScope{v1.TrainScope, v1.InferScope, v1.AuthoringScope}
	}
	return workspace
}

func (h *Handler) listWorkspace(c *gin.Context) (interface{}, error) {
	query, err := parseListWorkspaceQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}
	labelSelector, err := buildListWorkspaceSelector(query)
	if err != nil {
		return nil, err
	}
	workspaceList := &v1.WorkspaceList{}
	if err = h.List(c.Request.Context(), workspaceList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return nil, err
	}
	sort.Sort(types.WorkspaceSlice(workspaceList.Items))

	result := &types.GetWorkspaceResponse{}
	for _, w := range workspaceList.Items {
		item, err := h.cvtToWorkspaceResItem(c.Request.Context(), &w, false)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *item)
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

func (h *Handler) getWorkspace(c *gin.Context) (interface{}, error) {
	workspace, err := h.getAdminWorkspace(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	result, err := h.cvtToWorkspaceResItem(c.Request.Context(), workspace, true)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *Handler) deleteWorkspace(c *gin.Context) (interface{}, error) {
	workspace, err := h.getAdminWorkspace(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
		return nil, err
	}
	if err = h.Delete(c.Request.Context(), workspace); err != nil {
		klog.ErrorS(err, "failed to delete workspace")
		return nil, err
	}
	klog.Infof("delete workspace, name: %s", workspace.Name)
	return nil, nil
}

func (h *Handler) patchWorkspace(c *gin.Context) (interface{}, error) {
	workspace, err := h.getAdminWorkspace(c.Request.Context(), c.GetString(types.Name))
	if err != nil {
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
	if err = h.Patch(c.Request.Context(), workspace, patch); err != nil {
		klog.ErrorS(err, "failed to patch workspace", "data", string(body))
		return nil, err
	}
	klog.Infof("patch workspace, name: %s, request: %s", workspace.Name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

func updateWorkspace(workspace *v1.Workspace, req *types.PatchWorkspaceRequest) {
	if req.Description != nil {
		metav1.SetMetaDataAnnotation(&workspace.ObjectMeta, v1.DescriptionAnnotation, *req.Description)
	}
	if req.QueueBalanceTimeout != nil {
		metav1.SetMetaDataAnnotation(&workspace.ObjectMeta,
			v1.QueueBalanceTimeoutAnnotation, strconv.Itoa(*req.QueueBalanceTimeout))
	}
	if req.NodeFlavor != nil {
		workspace.Spec.NodeFlavor = *req.NodeFlavor
	}
	if req.TotalNodes != nil {
		workspace.Spec.Replica = *req.TotalNodes
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

func parseListWorkspaceQuery(c *gin.Context) (*types.GetWorkspaceRequest, error) {
	query := &types.GetWorkspaceRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

func buildListWorkspaceSelector(query *types.GetWorkspaceRequest) (labels.Selector, error) {
	var labelSelector = labels.NewSelector()
	if query.ClusterId != "" {
		req, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{query.ClusterId})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector, nil
}

func (h *Handler) cvtToWorkspaceResItem(ctx context.Context,
	w *v1.Workspace, isNeedDetail bool) (*types.GetWorkspaceResponseItem, error) {
	result := &types.GetWorkspaceResponseItem{
		WorkspaceId:         w.Name,
		WorkspaceName:       v1.GetDisplayName(w),
		ClusterId:           w.Spec.Cluster,
		NodeFlavor:          w.Spec.NodeFlavor,
		TotalReplica:        w.Spec.Replica,
		Phase:               string(w.Status.Phase),
		CreatedTime:         timeutil.FormatRFC3339(&w.CreationTimestamp.Time),
		Description:         v1.GetDescription(w),
		QueuePolicy:         w.Spec.QueuePolicy,
		QueueBalanceTimeout: v1.GetQueueBalanceTimeout(w),
		Scopes:              w.Spec.Scopes,
		Volumes:             w.Spec.Volumes,
	}
	if isNeedDetail {
		if err := h.buildWorkspaceDetail(ctx, w, result); err != nil {
			klog.ErrorS(err, "failed to buildWorkspaceDetail")
			return nil, err
		}
	}
	return result, nil
}

func (h *Handler) buildWorkspaceDetail(ctx context.Context, workspace *v1.Workspace, result *types.GetWorkspaceResponseItem) error {
	result.AvailableReplica = workspace.Status.AvailableReplica
	result.AbnormalReplica = workspace.Status.AbnormalReplica

	nf, err := h.getAdminNodeFlavor(ctx, workspace.Spec.NodeFlavor)
	if err != nil {
		return err
	}
	nfResource := nf.ToResourceList()

	totalQuota := quantity.MultiResource(nfResource, int64(result.AvailableReplica+result.AbnormalReplica))
	availQuota := quantity.MultiResource(nfResource, int64(result.AvailableReplica))
	availQuota = quantity.GetAvailResource(availQuota)
	abnormalQuota := quantity.MultiResource(nfResource, int64(result.AbnormalReplica))
	result.TotalQuota = cvtToResourceList(totalQuota)
	result.AvailQuota = cvtToResourceList(availQuota)
	result.AbnormalQuota = cvtToResourceList(abnormalQuota)

	filterNode := func(nodeName string) bool {
		n, err := h.getAdminNode(ctx, nodeName)
		if err != nil {
			return true
		}
		if !n.IsAvailable() {
			return true
		}
		return false
	}

	workspaceNames := []string{workspace.Name}
	workloads, err := h.getRunningWorkloads(ctx, workspace.Spec.Cluster, workspaceNames)
	if err != nil || len(workloads) == 0 {
		return err
	}
	var usedQuota corev1.ResourceList
	for _, w := range workloads {
		res, err := commonworkload.GetActiveResource(w, filterNode)
		if err != nil {
			return err
		}
		if res != nil {
			usedQuota = quantity.AddResource(usedQuota, res)
		}
	}
	result.UsedQuota = cvtToResourceList(usedQuota)
	return nil
}
