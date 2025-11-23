/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/kyaml/sliceutil"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/quantity"
	commonsecret "github.com/AMD-AIG-AIMA/SAFE/common/pkg/secret"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CreateWorkspace handles the creation of a new workspace resource.
// It authorizes the request, parses the creation request, generates a workspace object,
// and creates it in the system. Returns the created workspace ID on success.
func (h *Handler) CreateWorkspace(c *gin.Context) {
	handle(c, h.createWorkspace)
}

// ListWorkspace handles listing workspace resources with filtering capabilities.
// It retrieves workspaces based on query parameters, applies authorization filtering,
// and returns them in a sorted list with detailed information.
func (h *Handler) ListWorkspace(c *gin.Context) {
	handle(c, h.listWorkspace)
}

// GetWorkspace retrieves detailed information about a specific workspace.
// It authorizes the request and returns comprehensive workspace details
// including resource quotas and manager information.
func (h *Handler) GetWorkspace(c *gin.Context) {
	handle(c, h.getWorkspace)
}

// DeleteWorkspace handles deletion of a workspace resource.
// It authorizes the request and removes the specified workspace from the system.
func (h *Handler) DeleteWorkspace(c *gin.Context) {
	handle(c, h.deleteWorkspace)
}

// PatchWorkspace handles partial updates to a workspace resource.
// It authorizes the request, parses update parameters, and applies changes
// to the specified workspace.
func (h *Handler) PatchWorkspace(c *gin.Context) {
	handle(c, h.patchWorkspace)
}

// ProcessWorkspaceNodes handles adding or removing nodes from a workspace.
// It parses the node processing request and updates the workspace with the specified nodes and action.
func (h *Handler) ProcessWorkspaceNodes(c *gin.Context) {
	handle(c, h.processWorkspaceNodes)
}

// createWorkspace implements the workspace creation logic.
// Parses the request, generates a workspace object with specified parameters,
// and persists it in the system.
func (h *Handler) createWorkspace(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.WorkspaceKind,
		Verb:         v1.CreateVerb,
		User:         requestUser,
	}); err != nil {
		return nil, err
	}

	req := &types.CreateWorkspaceRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", string(body))
		return nil, err
	}
	workspace, err := h.generateWorkspace(c.Request.Context(), requestUser, req)
	if err != nil {
		return nil, err
	}
	err = h.Create(c.Request.Context(), workspace)
	if err != nil {
		klog.ErrorS(err, "failed to create", "workspace", req.Name)
		return nil, err
	}
	klog.Infof("create workspace, name: %s", workspace.Name)
	return &types.CreateWorkspaceResponse{
		WorkspaceId: workspace.Name,
	}, nil
}

// listWorkspace implements the workspace listing logic.
// Parses query parameters, builds label selectors, retrieves matching workspaces,
// applies authorization filtering, sorts them, and converts to response format.
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
	if len(workspaceList.Items) > 1 {
		sort.Slice(workspaceList.Items, func(i, j int) bool {
			return workspaceList.Items[i].Name < workspaceList.Items[j].Name
		})
	}
	roles := h.accessController.GetRoles(ctx, requestUser)
	result := &types.ListWorkspaceResponse{}
	for _, w := range workspaceList.Items {
		if err = h.accessController.Authorize(authority.AccessInput{
			Context:    ctx,
			Resource:   &w,
			Verb:       v1.ListVerb,
			Workspaces: []string{w.Name},
			User:       requestUser,
			Roles:      roles,
		}); err != nil {
			continue
		}
		item := h.cvtToWorkspaceResponseItem(ctx, &w)
		result.Items = append(result.Items, item)
	}
	result.TotalCount = len(result.Items)
	return result, nil
}

// getWorkspace implements the logic for retrieving a single workspace's detailed information.
// Authorizes access to the workspace and returns comprehensive workspace details
// including resource quotas and configuration.
func (h *Handler) getWorkspace(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
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

// deleteWorkspace implements workspace deletion logic.
// Authorizes the request and removes the specified workspace from the system.
func (h *Handler) deleteWorkspace(c *gin.Context) (interface{}, error) {
	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
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

// patchWorkspace implements partial update logic for a workspace.
// Parses the patch request and applies specified changes to the workspace.
func (h *Handler) patchWorkspace(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	ctx := c.Request.Context()
	workspace, err := h.getAdminWorkspace(ctx, c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workspace,
		Verb:       v1.UpdateVerb,
		Workspaces: []string{workspace.Name},
		User:       requestUser,
	}); err != nil {
		return nil, err
	}

	req := &types.PatchWorkspaceRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse request", "body", string(body))
		return nil, err
	}

	originalWorkspace := client.MergeFrom(workspace.DeepCopy())
	if err = h.updateWorkspace(ctx, workspace, requestUser, req); err != nil {
		return nil, err
	}
	if err = h.Patch(ctx, workspace, originalWorkspace); err != nil {
		klog.ErrorS(err, "failed to patch workspace", "data", string(body))
		return nil, err
	}
	klog.Infof("patch workspace, name: %s, request: %s", workspace.Name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

// updateWorkspace applies updates to a workspace based on the patch request.
// Handles changes to description, flavor, replica count, queue policy, scopes,
// volumes, preemption settings, managers, default status, and image secrets.
func (h *Handler) updateWorkspace(ctx context.Context, workspace *v1.Workspace, requestUser *v1.User, req *types.PatchWorkspaceRequest) error {
	if req.Description != nil {
		v1.SetAnnotation(workspace, v1.DescriptionAnnotation, *req.Description)
	}
	if req.FlavorId != nil {
		workspace.Spec.NodeFlavor = *req.FlavorId
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
	if req.IsDefault != nil {
		workspace.Spec.IsDefault = *req.IsDefault
	}
	if req.ImageSecretIds != nil {
		if err := h.updateWorkspaceImageSecrets(ctx, workspace, requestUser, *req.ImageSecretIds); err != nil {
			return err
		}
	}
	return nil
}

// updateWorkspaceImageSecrets updates the image secrets associated with a workspace.
// Retrieves secret objects by ID and updates the workspace's image secret references.
func (h *Handler) updateWorkspaceImageSecrets(ctx context.Context, workspace *v1.Workspace, requestUser *v1.User, secretIds []string) error {
	var imageSecrets []corev1.ObjectReference
	for _, id := range secretIds {
		secret, err := h.getAndAuthorizeSecret(ctx, id, workspace.Name, requestUser, v1.ListVerb)
		if err != nil {
			return err
		}
		if v1.GetSecretType(secret) != string(v1.SecretImage) {
			return commonerrors.NewBadRequest("the secret type is not image")
		}
		if !v1.IsSecretSharable(secret) {
			return commonerrors.NewBadRequest("the secret is not sharable")
		}
		workspaceIds := commonsecret.GetSecretWorkspaces(secret)
		if !sliceutil.Contains(workspaceIds, workspace.Name) {
			return commonerrors.NewBadRequest("the secret is not associated with the workspace")
		}
		imageSecrets = append(imageSecrets, *commonutils.GenObjectReference(secret.TypeMeta, secret.ObjectMeta))
	}
	workspace.Spec.ImageSecrets = imageSecrets
	return nil
}

// getAdminWorkspace retrieves a workspace resource by ID from the system.
// Returns an error if the workspace doesn't exist or the ID is empty.
func (h *Handler) getAdminWorkspace(ctx context.Context, workspaceId string) (*v1.Workspace, error) {
	if workspaceId == "" {
		return nil, commonerrors.NewBadRequest("the workspaceId is empty")
	}
	workspace := &v1.Workspace{}
	err := h.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace)
	if err != nil {
		klog.ErrorS(err, "failed to get admin workspace")
		return nil, err
	}
	return workspace.DeepCopy(), nil
}

// processWorkspaceNodes handles the processing of nodes for a workspace.
// Parses the request and updates the workspace with the specified node action.
func (h *Handler) processWorkspaceNodes(c *gin.Context) (interface{}, error) {
	req, err := parseProcessNodesRequest(c)
	if err != nil {
		return nil, err
	}
	return nil, h.updateWorkspaceNodesAction(c, c.GetString(common.Name), req.Action, req.NodeIds)
}

// updateWorkspaceNodesAction converts requested nodes and action into a node action
// to update the workspace
func (h *Handler) updateWorkspaceNodesAction(c *gin.Context, workspaceId, action string, nodeIds []string) error {
	nodeAction := commonnodes.BuildAction(action, nodeIds...)
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		workspace := &v1.Workspace{}
		if err := h.Get(c.Request.Context(), client.ObjectKey{Name: workspaceId}, workspace); err != nil {
			return err
		}
		if err := h.accessController.Authorize(authority.AccessInput{
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

// getWorkspaceDisplayName retrieves the display name of a workspace by ID.
func (h *Handler) getWorkspaceDisplayName(ctx context.Context, workspaceId string) (string, error) {
	workspace := &v1.Workspace{}
	if err := h.Get(ctx, client.ObjectKey{Name: workspaceId}, workspace); err != nil {
		return "", err
	}
	return v1.GetDisplayName(workspace), nil
}

// removeWorkspaceManager removes a user from the workspace's manager list.
// If the user is not in the manager list, no change is made.
func (h *Handler) removeWorkspaceManager(ctx context.Context, workspaceId, userId string) error {
	workspace, err := h.getAdminWorkspace(ctx, workspaceId)
	if err != nil {
		return err
	}
	newManagers := sliceutil.Remove(workspace.Spec.Managers, userId)
	if len(newManagers) == len(workspace.Spec.Managers) {
		return nil
	}
	workspace.Spec.Managers = newManagers
	if err = h.Update(ctx, workspace); err != nil {
		return err
	}
	return nil
}

// generateWorkspace creates a new workspace object based on the creation request.
// Validates the request parameters and populates the workspace specification.
func (h *Handler) generateWorkspace(ctx context.Context,
	requestUser *v1.User, req *types.CreateWorkspaceRequest) (*v1.Workspace, error) {
	workspace := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: stringutil.NormalizeName(req.ClusterId + "-" + req.Name),
			Labels: map[string]string{
				v1.DisplayNameLabel: req.Name,
				v1.UserIdLabel:      requestUser.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation:    v1.GetUserName(requestUser),
				v1.DescriptionAnnotation: req.Description,
			},
		},
		Spec: v1.WorkspaceSpec{
			Cluster:       req.ClusterId,
			NodeFlavor:    req.FlavorId,
			Replica:       req.Replica,
			QueuePolicy:   v1.WorkspaceQueuePolicy(strings.ToLower(req.QueuePolicy)),
			Volumes:       req.Volumes,
			Scopes:        req.Scopes,
			EnablePreempt: req.EnablePreempt,
			Managers:      req.Managers,
			IsDefault:     req.IsDefault,
		},
	}
	if len(workspace.Spec.Scopes) == 0 {
		workspace.Spec.Scopes = []v1.WorkspaceScope{v1.TrainScope, v1.InferScope, v1.AuthoringScope}
	}
	err := h.updateWorkspaceImageSecrets(ctx, workspace, requestUser, req.ImageSecretIds)
	if err != nil {
		return nil, err
	}
	return workspace, nil
}

// parseListWorkspaceQuery parses and validates the query parameters for workspace listing.
func parseListWorkspaceQuery(c *gin.Context) (*types.ListWorkspaceRequest, error) {
	query := &types.ListWorkspaceRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	return query, nil
}

// buildListWorkspaceSelector constructs a label selector based on workspace list query parameters.
// Used to filter workspaces by cluster ID criteria.
func buildListWorkspaceSelector(query *types.ListWorkspaceRequest) (labels.Selector, error) {
	var labelSelector = labels.NewSelector()
	if query.ClusterId != "" {
		req, _ := labels.NewRequirement(v1.ClusterIdLabel, selection.Equals, []string{query.ClusterId})
		labelSelector = labelSelector.Add(*req)
	}
	return labelSelector, nil
}

// cvtToWorkspaceResponseItem converts a workspace object to a response item format.
// Includes basic workspace information like ID, name, cluster, flavor, and status.
func (h *Handler) cvtToWorkspaceResponseItem(ctx context.Context, w *v1.Workspace) types.WorkspaceResponseItem {
	result := types.WorkspaceResponseItem{
		WorkspaceId:       w.Name,
		WorkspaceName:     v1.GetDisplayName(w),
		ClusterId:         w.Spec.Cluster,
		FlavorId:          w.Spec.NodeFlavor,
		UserId:            v1.GetUserId(w),
		TargetNodeCount:   w.Spec.Replica,
		CurrentNodeCount:  w.CurrentReplica(),
		AbnormalNodeCount: w.Status.AbnormalReplica,
		Phase:             string(w.Status.Phase),
		CreationTime:      timeutil.FormatRFC3339(w.CreationTimestamp.Time),
		Description:       v1.GetDescription(w),
		QueuePolicy:       w.Spec.QueuePolicy,
		Scopes:            w.Spec.Scopes,
		Volumes:           w.Spec.Volumes,
		EnablePreempt:     w.Spec.EnablePreempt,
		IsDefault:         w.Spec.IsDefault,
	}
	for _, m := range w.Spec.Managers {
		user, err := h.getAdminUser(ctx, m)
		if err == nil {
			result.Managers = append(result.Managers, types.UserEntity{
				Id: m, Name: v1.GetUserName(user),
			})
		}
	}
	return result
}

// cvtToGetWorkspaceResponse converts a workspace object to a detailed response format.
// Includes comprehensive workspace information with resource quotas and secret IDs.
func (h *Handler) cvtToGetWorkspaceResponse(ctx context.Context, workspace *v1.Workspace) (*types.GetWorkspaceResponse, error) {
	result := &types.GetWorkspaceResponse{
		WorkspaceResponseItem: h.cvtToWorkspaceResponseItem(ctx, workspace),
	}
	nf, err := h.getAdminNodeFlavor(ctx, workspace.Spec.NodeFlavor)
	if err != nil {
		return nil, err
	}
	nfResource := nf.ToResourceList(commonconfig.GetRdmaName())

	abnormalQuota := quantity.MultiResource(nfResource, int64(result.AbnormalNodeCount))
	result.TotalQuota = cvtToResourceList(workspace.Status.TotalResources)
	result.AbnormalQuota = cvtToResourceList(abnormalQuota)

	usedQuota, usedNodeCount, err := h.getWorkspaceUsedQuota(ctx, workspace)
	if err != nil {
		return nil, err
	}
	result.UsedQuota = cvtToResourceList(usedQuota)
	result.UsedNodeCount = usedNodeCount

	availQuota := workspace.Status.AvailableResources
	result.AvailQuota = cvtToResourceList(quantity.SubResource(availQuota, usedQuota))
	for _, s := range workspace.Spec.ImageSecrets {
		result.ImageSecretIds = append(result.ImageSecretIds, s.Name)
	}
	return result, nil
}

// getWorkspaceUsedQuota calculates the quota that has been used by the workspace.
// Aggregates resource usage from running workloads in the workspace. The number of associated nodes are also returned.
func (h *Handler) getWorkspaceUsedQuota(ctx context.Context, workspace *v1.Workspace) (corev1.ResourceList, int, error) {
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
		return nil, 0, err
	}
	var usedQuota corev1.ResourceList
	nodeSet := sets.NewSet()
	for _, w := range workloads {
		res, nodes, err := commonworkload.GetActiveResources(w, filterNode)
		if err != nil {
			return nil, 0, err
		}
		if res != nil {
			usedQuota = quantity.AddResource(usedQuota, res)
			for _, n := range nodes {
				nodeSet.Insert(n)
			}
		}
	}
	return usedQuota, len(nodeSet), nil
}
