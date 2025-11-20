/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

type WorkloadBatchAction string

const (
	DefaultLogTailLine int64 = 1000

	BatchDelete WorkloadBatchAction = "delete"
	BatchStop   WorkloadBatchAction = "stop"
	BatchClone  WorkloadBatchAction = "clone"
)

// CreateWorkload handles the creation of a new workload resource.
// It parses the creation request, generates a workload object,
// and creates it in the system. Returns the created workload ID on success.
func (h *Handler) CreateWorkload(c *gin.Context) {
	handle(c, h.createWorkload)
}

// ListWorkload handles listing workload resources with filtering and pagination.
// Supports both database and etcd backends for workload storage.
// Returns a list of workloads matching the query criteria.
func (h *Handler) ListWorkload(c *gin.Context) {
	handle(c, h.listWorkload)
}

// GetWorkload retrieves detailed information about a specific workload.
// Supports both database and etcd backends for workload storage.
// Returns complete workload information including status and configuration.
func (h *Handler) GetWorkload(c *gin.Context) {
	handle(c, h.getWorkload)
}

// DeleteWorkload handles deletion of a single workload resource.
// Performs both administrative deletion and database marking as deleted.
func (h *Handler) DeleteWorkload(c *gin.Context) {
	handle(c, h.deleteWorkload)
}

// DeleteWorkloads handles batch deletion of multiple workload resources.
// Processes multiple workload deletions concurrently.
func (h *Handler) DeleteWorkloads(c *gin.Context) {
	handle(c, h.deleteWorkloads)
}

// StopWorkload handles stopping a single workload resource.
// Marks the workload as stopped in both etcd and database.
func (h *Handler) StopWorkload(c *gin.Context) {
	handle(c, h.stopWorkload)
}

// StopWorkloads handles batch stopping of multiple workload resources.
// Processes multiple workload stops concurrently.
func (h *Handler) StopWorkloads(c *gin.Context) {
	handle(c, h.stopWorkloads)
}

// PatchWorkload handles partial updates to a workload resource.
// Supports updating specific fields like priority, resources, and configuration.
func (h *Handler) PatchWorkload(c *gin.Context) {
	handle(c, h.patchWorkload)
}

// CloneWorkloads handles batch cloning of multiple workload resources.
// Creates new workloads based on existing workload configurations.
func (h *Handler) CloneWorkloads(c *gin.Context) {
	handle(c, h.cloneWorkloads)
}

// GetWorkloadPodLog retrieves logs from a specific pod associated with a workload.
// Fetches pod logs from the Kubernetes cluster and returns them in a structured format.
func (h *Handler) GetWorkloadPodLog(c *gin.Context) {
	handle(c, h.getWorkloadPodLog)
}

// GetWorkloadPodContainers retrieves the container list and available shells for a specific pod of a workload.
// It authorizes access to the workload, fetches the pod details from Kubernetes,
// and returns the container names along with available shell options (bash, sh, zsh).
func (h *Handler) GetWorkloadPodContainers(c *gin.Context) {
	handle(c, h.getWorkloadPodContainers)
}

// createWorkload implements the workload creation logic.
// Parses the request, generates a workload object, and creates it in the system.
func (h *Handler) createWorkload(c *gin.Context) (interface{}, error) {
	req := &types.CreateWorkloadRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		return nil, err
	}
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	workload, err := h.generateWorkload(c.Request.Context(), req, body)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	return h.createWorkloadImpl(c, workload, requestUser, roles)
}

// createWorkloadImpl performs the actual workload creation in the system.
// Handles authorization checks, workload creation in etcd, and initial phase setting.
func (h *Handler) createWorkloadImpl(c *gin.Context, workload *v1.Workload, requestUser *v1.User, roles []*v1.Role) (interface{}, error) {
	var err error
	if err = h.authWorkloadAction(c, workload, v1.CreateVerb, requestUser, roles); err != nil {
		klog.ErrorS(err, "failed to auth workload", "workload", workload.Name,
			"workspace", workload.Spec.Workspace, "user", c.GetString(common.UserName))
		return nil, err
	}
	if err = h.authWorkloadPriority(c, workload, v1.CreateVerb, workload.Spec.Priority, requestUser, roles); err != nil {
		klog.ErrorS(err, "failed to auth workload priority", "workload", workload.Name,
			"priority", workload.Spec.Priority, "user", c.GetString(common.UserName))
		return nil, err
	}
	v1.SetLabel(workload, v1.UserIdLabel, requestUser.Name)
	v1.SetAnnotation(workload, v1.UserNameAnnotation, v1.GetUserName(requestUser))
	if err = h.Create(c.Request.Context(), workload); err != nil {
		return nil, err
	}
	if err = h.patchPhase(c.Request.Context(), workload, v1.WorkloadPending, nil); err != nil {
		return nil, err
	}
	klog.Infof("create workload, name: %s, user: %s/%s, priority: %d, timeout: %d",
		workload.Name, c.GetString(common.UserName), c.GetString(common.UserId), workload.Spec.Priority, workload.GetTimeout())
	return &types.CreateWorkloadResponse{WorkloadId: workload.Name}, nil
}

// listWorkload implements the workload listing logic.
// Parses query parameters, builds database or etcd queries,
// and returns workloads matching the criteria.
func (h *Handler) listWorkload(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	query, err := parseListWorkloadQuery(c)
	if err != nil {
		return nil, err
	}
	adminWorkload := generateWorkloadForAuth("", "", query.WorkspaceId, query.ClusterId)
	if err = h.authWorkloadAction(c, adminWorkload, v1.ListVerb, requestUser, roles); err != nil {
		return nil, err
	}

	dbSql, orderBy := cvtToListWorkloadSql(query)
	ctx := c.Request.Context()
	workloads, err := h.dbClient.SelectWorkloads(ctx, dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}

	result := &types.ListWorkloadResponse{}
	if result.TotalCount, err = h.dbClient.CountWorkloads(ctx, dbSql); err != nil {
		return nil, err
	}
	for _, w := range workloads {
		workload := h.cvtDBWorkloadToResponseItem(ctx, w)

		// Query workload statistics to get GPU usage
		stat, err := h.dbClient.GetWorkloadStatisticByWorkloadID(ctx, w.WorkloadId)
		if err != nil {
			klog.V(4).InfoS("failed to get workload statistic", "workloadId", w.WorkloadId, "error", err)
			workload.AvgGpuUsage = -1
		} else if stat == nil {
			// No statistics available
			workload.AvgGpuUsage = -1
		} else {
			// Use the average GPU usage from statistics
			workload.AvgGpuUsage = stat.AvgGpuUsage3H
		}

		result.Items = append(result.Items, workload)
	}
	return result, nil
}

// getWorkload implements the logic for retrieving a single workload's detailed information.
// Supports both database and etcd backends and includes authorization checks.
func (h *Handler) getWorkload(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}

	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	name := c.GetString(common.Name)
	ctx := c.Request.Context()
	dbWorkload, err := h.dbClient.GetWorkload(ctx, name)
	if err != nil {
		return nil, err
	}
	adminWorkload := generateWorkloadForAuth(name, dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
	if err = h.authWorkloadAction(c, adminWorkload, v1.GetVerb, requestUser, roles); err != nil {
		return nil, err
	}
	return h.cvtDBWorkloadToGetResponse(ctx, requestUser, roles, dbWorkload), nil
}

// deleteWorkload implements single workload deletion logic.
// Handles deletion from both etcd and database with proper authorization.
func (h *Handler) deleteWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	name := c.GetString(common.Name)
	return h.deleteWorkloadImpl(c, name, requestUser, roles)
}

// deleteWorkloads implements batch workload deletion logic.
// Processes multiple workload deletions concurrently with error handling.
func (h *Handler) deleteWorkloads(c *gin.Context) (interface{}, error) {
	return h.handleBatchWorkloads(c, BatchDelete)
}

// deleteWorkloadImpl performs the actual deletion of a workload.
// Handles deletion from both etcd and database based on system configuration.
func (h *Handler) deleteWorkloadImpl(c *gin.Context, name string, requestUser *v1.User, roles []*v1.Role) (interface{}, error) {
	adminWorkload, err := h.getAdminWorkload(c.Request.Context(), name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
			return nil, err
		}
		message := fmt.Sprintf("the workload is deleted by %s", c.GetString(common.UserName))
		if err = h.deleteAdminWorkload(c.Request.Context(), adminWorkload, message); err != nil {
			return nil, err
		}
	}

	if commonconfig.IsDBEnable() {
		dbWorkload, err := h.dbClient.GetWorkload(c.Request.Context(), name)
		if err != nil {
			return nil, commonerrors.IgnoreFound(err)
		}
		adminWorkload = generateWorkloadForAuth(name, dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
			return nil, err
		}
		if err = h.dbClient.SetWorkloadDeleted(c.Request.Context(), name); err != nil {
			return nil, err
		}
	}
	klog.Infof("delete workload %s by user %s/%s",
		name, c.GetString(common.UserName), c.GetString(common.UserId))
	return nil, nil
}

// deleteAdminWorkload removes a workload from the Kubernetes cluster.
// Sets the workload phase to stopped and deletes the resource from etcd.
func (h *Handler) deleteAdminWorkload(ctx context.Context, adminWorkload *v1.Workload, message string) error {
	cond := &metav1.Condition{
		Type:    string(v1.AdminStopped),
		Status:  metav1.ConditionTrue,
		Message: message,
	}

	if err := h.patchPhase(ctx, adminWorkload, v1.WorkloadStopped, cond); err != nil {
		return err
	}
	if err := h.Delete(ctx, adminWorkload); err != nil {
		return err
	}
	return nil
}

// stopWorkload implements single workload stopping logic.
// Marks the workload as stopped in both etcd and database.
func (h *Handler) stopWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)
	name := c.GetString(common.Name)
	return h.stopWorkloadImpl(c, name, requestUser, roles)
}

// stopWorkloads implements batch workload stopping logic.
// Processes multiple workload stops concurrently with error handling.
func (h *Handler) stopWorkloads(c *gin.Context) (interface{}, error) {
	return h.handleBatchWorkloads(c, BatchStop)
}

// stopWorkloadImpl performs the actual stopping of a workload.
// Handles stopping in both etcd and database based on system configuration.
func (h *Handler) stopWorkloadImpl(c *gin.Context, name string, requestUser *v1.User, roles []*v1.Role) (interface{}, error) {
	adminWorkload, err := h.getAdminWorkload(c.Request.Context(), name)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, err
		}
		if !commonconfig.IsDBEnable() {
			return nil, nil
		}
		dbWorkload, err := h.dbClient.GetWorkload(c.Request.Context(), name)
		if err != nil {
			return nil, commonerrors.IgnoreFound(err)
		}
		if dbutils.ParseNullString(dbWorkload.Phase) != string(v1.WorkloadStopped) {
			adminWorkload = generateWorkloadForAuth(name,
				dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
			if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
				return nil, err
			}
			if err = h.dbClient.SetWorkloadStopped(c.Request.Context(), name); err != nil {
				return nil, err
			}
		}
	} else {
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, requestUser, roles); err != nil {
			return nil, err
		}
		message := fmt.Sprintf("the workload is stopped by %s", c.GetString(common.UserName))
		if err = h.deleteAdminWorkload(c.Request.Context(), adminWorkload, message); err != nil {
			return nil, err
		}
	}
	klog.Infof("stop workload %s by user %s/%s",
		name, c.GetString(common.UserName), c.GetString(common.UserId))
	return nil, nil
}

// patchWorkload implements partial update logic for a workload.
// Parses the patch request, validates authorization, and applies changes.
func (h *Handler) patchWorkload(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	name := c.GetString(common.Name)
	adminWorkload, err := h.getAdminWorkload(c.Request.Context(), name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewInternalError("The workload can only be edited when it is running.")
		}
		return nil, err
	}

	req := &types.PatchWorkloadRequest{}
	if _, err = apiutils.ParseRequestBody(c.Request, req); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if err = h.authWorkloadAction(c, adminWorkload, v1.UpdateVerb, requestUser, roles); err != nil {
		return nil, err
	}
	if req.Priority != nil {
		if err = h.authWorkloadPriority(c, adminWorkload, v1.UpdateVerb, *req.Priority, requestUser, roles); err != nil {
			return nil, err
		}
	}

	originalWorkload := client.MergeFrom(adminWorkload.DeepCopy())
	if err = updateWorkload(adminWorkload, req); err != nil {
		return nil, err
	}
	if err = h.Patch(c.Request.Context(), adminWorkload, originalWorkload); err != nil {
		return nil, err
	}

	klog.Infof("patch workload, name: %s, request: %s", name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

// cloneWorkloads implements batch workload cloning logic.
// Processes multiple workload clones concurrently with error handling.
func (h *Handler) cloneWorkloads(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	return h.handleBatchWorkloads(c, BatchClone)
}

// cloneWorkloadImpl performs the actual cloning of a workload.
// Creates a new workload based on an existing workload's database record.
func (h *Handler) cloneWorkloadImpl(c *gin.Context, name string, requestUser *v1.User, roles []*v1.Role) (interface{}, error) {
	dbWorkload, err := h.dbClient.GetWorkload(c.Request.Context(), name)
	if err != nil {
		return nil, err
	}
	workload := cvtDBWorkloadToAdminWorkload(dbWorkload)
	// Only the user themselves or an administrator can get this info.
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: workload,
		Verb:     v1.GetVerb,
		User:     requestUser,
		Roles:    roles,
	}); err != nil {
		return nil, err
	}
	klog.Infof("cloning workload from %s to %s", name, workload.Name)
	return h.createWorkloadImpl(c, workload, requestUser, roles)
}

// getWorkloadPodLog implements the logic for retrieving pod logs for a workload.
// Fetches logs from the Kubernetes cluster and formats them for the response.
func (h *Handler) getWorkloadPodLog(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	workload, err := h.getAdminWorkload(c.Request.Context(), c.GetString(common.Name))
	if err != nil {
		return nil, err
	}
	if err = h.authWorkloadAction(c, workload, v1.GetVerb, requestUser, roles); err != nil {
		return nil, err
	}

	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, v1.GetClusterId(workload))
	if err != nil {
		return nil, err
	}
	podName := strings.TrimSpace(c.Param(common.PodId))
	podLogs, err := h.getPodLog(c, k8sClients.ClientSet(),
		workload.Spec.Workspace, podName, v1.GetMainContainer(workload))
	if err != nil {
		return nil, err
	}
	return &types.GetWorkloadPodLogResponse{
		WorkloadId: workload.Name,
		PodId:      podName,
		Namespace:  workload.Spec.Workspace,
		Logs:       strings.Split(string(podLogs), "\n"),
	}, nil
}

// patchPhase updates the phase of a workload and optionally adds a condition.
// Handles status updates including setting end time for stopped workloads.
func (h *Handler) patchPhase(ctx context.Context,
	workload *v1.Workload, phase v1.WorkloadPhase, cond *metav1.Condition) error {
	originalWorkload := client.MergeFrom(workload.DeepCopy())
	if phase != "" {
		workload.Status.Phase = phase
		if phase == v1.WorkloadStopped && workload.Status.EndTime == nil {
			workload.Status.EndTime = &metav1.Time{Time: time.Now().UTC()}
		}
	}

	if cond != nil {
		cond.LastTransitionTime = metav1.NewTime(time.Now())
		cond.Reason = commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(workload))
		if cond2 := workload.GetLastCondition(); cond2 != nil && cond2.Type == cond.Type {
			meta.SetStatusCondition(&workload.Status.Conditions, *cond)
		} else {
			workload.Status.Conditions = append(workload.Status.Conditions, *cond)
		}
	}
	if err := h.Status().Patch(ctx, workload, originalWorkload); err != nil {
		return err
	}
	return nil
}

// getAdminWorkload retrieves a workload resource by ID from etcd.
// Returns an error if the workload doesn't exist or the ID is empty.
func (h *Handler) getAdminWorkload(ctx context.Context, workloadId string) (*v1.Workload, error) {
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("the workloadId is empty")
	}
	workload := &v1.Workload{}
	if err := h.Get(ctx, client.ObjectKey{Name: workloadId}, workload); err != nil {
		return nil, err
	}
	return workload.DeepCopy(), nil
}

// getWorkloadForAuth retrieves a workload for authorization purposes.
// Supports both database and etcd backends and includes minimal information for auth checks.
func (h *Handler) getWorkloadForAuth(ctx context.Context, workloadId string) (*v1.Workload, error) {
	if !commonconfig.IsDBEnable() {
		return h.getAdminWorkload(ctx, workloadId)
	}
	dbWorkload, err := h.dbClient.GetWorkload(ctx, workloadId)
	if err != nil {
		return nil, err
	}
	adminWorkload := generateWorkloadForAuth(workloadId, dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
	adminWorkload.CreationTimestamp = metav1.NewTime(dbutils.ParseNullTime(dbWorkload.CreationTime))
	endTime := dbutils.ParseNullTime(dbWorkload.EndTime)
	if !endTime.IsZero() {
		adminWorkload.Status.EndTime = &metav1.Time{Time: endTime}
	}
	return adminWorkload, nil
}

// getRunningWorkloads retrieves workloads that are currently running.
// Filters workloads based on cluster and workspace criteria.
func (h *Handler) getRunningWorkloads(ctx context.Context, clusterName string, workspaceNames []string) ([]*v1.Workload, error) {
	filterFunc := func(w *v1.Workload) bool {
		if w.IsEnd() || !v1.IsWorkloadDispatched(w) {
			return true
		}
		return false
	}
	return commonworkload.GetWorkloadsOfWorkspace(ctx, h.Client, clusterName, workspaceNames, filterFunc)
}

// authWorkloadAction performs authorization checks for workload-related actions.
// Validates if the requesting user has permission to perform the specified action on the workload.
func (h *Handler) authWorkloadAction(c *gin.Context,
	adminWorkload *v1.Workload, verb v1.RoleVerb, requestUser *v1.User, roles []*v1.Role,
) error {
	var workspaces []string
	if adminWorkload.Spec.Workspace != "" {
		workspaces = append(workspaces, adminWorkload.Spec.Workspace)
	}
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.WorkloadKind,
		Resource:     adminWorkload,
		Verb:         verb,
		Workspaces:   workspaces,
		User:         requestUser,
		UserId:       c.GetString(common.UserId),
		Roles:        roles,
	}); err != nil {
		return err
	}
	return nil
}

// authWorkloadPriority performs authorization checks for workload priority operations.
// Validates if the requesting user has permission to set the specified priority level.
func (h *Handler) authWorkloadPriority(c *gin.Context, adminWorkload *v1.Workload,
	verb v1.RoleVerb, priority int, requestUser *v1.User, roles []*v1.Role,
) error {
	priorityKind := fmt.Sprintf("workload/%s", commonworkload.GeneratePriority(priority))
	resourceOwner := ""
	if verb == v1.UpdateVerb {
		resourceOwner = v1.GetUserId(adminWorkload)
	}
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:       c.Request.Context(),
		ResourceKind:  priorityKind,
		ResourceOwner: resourceOwner,
		Verb:          verb,
		Workspaces:    []string{adminWorkload.Spec.Workspace},
		User:          requestUser,
		Roles:         roles,
	}); err != nil {
		return err
	}
	return nil
}

// generateWorkload creates a new workload object based on the creation request.
// Populates workload metadata, specifications, and customer labels.
func (h *Handler) generateWorkload(ctx context.Context, req *types.CreateWorkloadRequest, body []byte) (*v1.Workload, error) {
	workload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(req.DisplayName),
			Labels: map[string]string{
				v1.DisplayNameLabel: req.DisplayName,
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation: req.Description,
			},
		},
	}

	var err error
	if err = json.Unmarshal(body, &workload.Spec); err != nil {
		return nil, err
	}
	if commonworkload.IsAuthoring(workload) {
		if len(req.SpecifiedNodes) > 1 {
			return nil, fmt.Errorf("the authoring can only be created with one node")
		}
	}
	genCustomerLabelsByNodes(workload, req.SpecifiedNodes)
	if len(req.SpecifiedNodes) > 0 {
		workload.Spec.Resource.Replica = len(req.SpecifiedNodes)
	}
	if req.WorkspaceId != "" {
		workload.Spec.Workspace = req.WorkspaceId
	}
	if req.Kind == common.CICDScaleSetKind {
		if !commonconfig.IsCICDEnable() {
			return nil, commonerrors.NewNotImplemented("the CICD is not enabled")
		}
		controlPlaneIp, err := h.getAdminControlPlaneIp(ctx)
		if err != nil {
			return nil, err
		}
		commonworkload.SetEnv(workload, common.AdminControlPlane, controlPlaneIp)
	}
	return workload, nil
}

func (h *Handler) getAdminControlPlaneIp(ctx context.Context) (string, error) {
	nodeList := &corev1.NodeList{}
	labelSelector := labels.SelectorFromSet(map[string]string{common.KubernetesControlPlane: ""})
	if err := h.List(ctx,
		nodeList, &client.ListOptions{LabelSelector: labelSelector}); err != nil {
		return "", err
	}
	if len(nodeList.Items) == 0 {
		return "", commonerrors.NewInternalError("failed to find the control plane")
	}
	internalIp := commonnodes.GetInternalIp(&nodeList.Items[0])
	if internalIp == "" {
		return "", commonerrors.NewInternalError("failed to find the control plane ip")
	}
	return internalIp, nil
}

// handleBatchWorkloads processes batch operations on multiple workloads.
// Supports delete, stop, and clone actions with concurrent execution.
func (h *Handler) handleBatchWorkloads(c *gin.Context, action WorkloadBatchAction) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	req := &types.BatchWorkloadsRequest{}
	if _, err = apiutils.ParseRequestBody(c.Request, req); err != nil {
		return nil, err
	}
	count := len(req.WorkloadIds)
	ch := make(chan string, count)
	defer close(ch)
	for _, id := range req.WorkloadIds {
		ch <- id
	}

	success, err := concurrent.Exec(count, func() error {
		workloadId := <-ch
		var innerErr error
		switch action {
		case BatchDelete:
			_, innerErr = h.deleteWorkloadImpl(c, workloadId, requestUser, roles)
		case BatchStop:
			_, innerErr = h.stopWorkloadImpl(c, workloadId, requestUser, roles)
		case BatchClone:
			_, innerErr = h.cloneWorkloadImpl(c, workloadId, requestUser, roles)
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

// genCustomerLabelsByNodes generates customer labels based on specified nodes.
func genCustomerLabelsByNodes(workload *v1.Workload, nodeList []string) {
	if len(nodeList) == 0 {
		return
	}
	if len(workload.Spec.CustomerLabels) > 0 {
		if _, ok := workload.Spec.CustomerLabels[v1.K8sHostName]; ok {
			return
		}
	} else {
		workload.Spec.CustomerLabels = make(map[string]string)
	}
	nodeNames := ""
	for i := range nodeList {
		if i > 0 {
			nodeNames += " "
		}
		nodeNames += nodeList[i]
	}
	workload.Spec.CustomerLabels[v1.K8sHostName] = nodeNames
}

// parseListWorkloadQuery parses and validates the query parameters for listing workloads.
// Handles URL decoding and sets default values for pagination and sorting.
func parseListWorkloadQuery(c *gin.Context) (*types.ListWorkloadRequest, error) {
	query := &types.ListWorkloadRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.UserName != "" {
		if nameUnescape, err := url.QueryUnescape(query.UserName); err == nil {
			query.UserName = nameUnescape
		}
	}
	if query.Limit <= 0 {
		query.Limit = types.DefaultQueryLimit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if query.Order == "" {
		query.Order = dbclient.DESC
	}
	if query.Description != "" {
		if desUnescape, err := url.QueryUnescape(query.Description); err == nil {
			query.Description = desUnescape
		}
	}
	return query, nil
}

// parseGetPodLogQuery parses and validates the query parameters for retrieving pod logs.
// Sets default values for tail lines and container name if not specified.
func parseGetPodLogQuery(c *gin.Context, mainContainerName string) (*types.GetPodLogRequest, error) {
	query := &types.GetPodLogRequest{}
	var err error
	if err = c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.TailLines <= 0 {
		query.TailLines = DefaultLogTailLine
	}
	if query.Container == "" {
		query.Container = mainContainerName
	}
	return query, nil
}

// cvtToListWorkloadSql converts workload list query parameters into a database SQL query.
// Builds WHERE conditions and ORDER BY clauses based on filter parameters.
func cvtToListWorkloadSql(query *types.ListWorkloadRequest) (sqrl.Sqlizer, []string) {
	dbTags := dbclient.GetWorkloadFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
	}
	if clusterId := strings.TrimSpace(query.ClusterId); clusterId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Cluster"): clusterId})
	}
	if workspaceId := strings.TrimSpace(query.WorkspaceId); workspaceId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Workspace"): workspaceId})
	}
	if query.Phase != "" {
		values := strings.Split(query.Phase, ",")
		var sqlList []sqrl.Sqlizer
		for _, val := range values {
			sqlList = append(sqlList, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Phase"): val})
		}
		dbSql = append(dbSql, sqrl.Or(sqlList))
	}
	if description := strings.TrimSpace(query.Description); description != "" {
		dbSql = append(dbSql,
			sqrl.Like{dbclient.GetFieldTag(dbTags, "Description"): fmt.Sprintf("%%%s%%", description)})
	}
	userNameField := dbclient.GetFieldTag(dbTags, "UserName")
	if userName := strings.TrimSpace(query.UserName); userName != "" {
		dbSql = append(dbSql, sqrl.Like{userNameField: fmt.Sprintf("%%%s%%", userName)})
	} else {
		userCondition := sqrl.Or{
			sqrl.NotEq{userNameField: common.UserSystem},        // username != 'system'
			sqrl.Expr(fmt.Sprintf("%s IS NULL", userNameField)), // username IS NULL
		}
		dbSql = append(dbSql, userCondition)
	}
	if userId := strings.TrimSpace(query.UserId); userId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): userId})
	}
	if sinceTime := strings.TrimSpace(query.Since); sinceTime != "" {
		if t, err := timeutil.CvtStrToRFC3339Milli(sinceTime); err == nil {
			dbSql = append(dbSql, sqrl.GtOrEq{dbclient.GetFieldTag(dbTags, "CreationTime"): t})
		} else {
			klog.ErrorS(err, "failed to parse since time")
		}
	}
	if untilTime := strings.TrimSpace(query.Until); untilTime != "" {
		if t, err := timeutil.CvtStrToRFC3339Milli(untilTime); err == nil {
			dbSql = append(dbSql, sqrl.LtOrEq{dbclient.GetFieldTag(dbTags, "CreationTime"): t})
		} else {
			klog.ErrorS(err, "failed to parse until time")
		}
	}
	if kind := strings.TrimSpace(query.Kind); kind != "" {
		values := strings.Split(query.Kind, ",")
		var sqlList []sqrl.Sqlizer
		for _, val := range values {
			gvk := v1.GroupVersionKind{Kind: val, Version: common.DefaultVersion}
			gvkStr := string(jsonutils.MarshalSilently(gvk))
			sqlList = append(sqlList, sqrl.Eq{dbclient.GetFieldTag(dbTags, "GVK"): gvkStr})
		}
		dbSql = append(dbSql, sqrl.Or(sqlList))
	}
	if workloadId := strings.TrimSpace(query.WorkloadId); workloadId != "" {
		dbSql = append(dbSql, sqrl.Like{
			dbclient.GetFieldTag(dbTags, "WorkloadId"): fmt.Sprintf("%%%s%%", workloadId),
		})
	}
	orderBy := buildOrderBy(query.SortBy, query.Order, dbTags)
	return dbSql, orderBy
}

// buildOrderBy constructs ORDER BY clause for input parameters.
// Handles primary sort field with null ordering, and adds creation_time as secondary sort.
// Returns formatted ORDER BY expressions.
func buildOrderBy(sortBy, order string, dbTags map[string]string) []string {
	var nullOrder string
	if order == dbclient.DESC {
		nullOrder = "NULLS FIRST"
	} else {
		nullOrder = "NULLS LAST"
	}

	var orderBy []string
	sortBy = strings.TrimSpace(sortBy)
	sortByTag := dbclient.GetFieldTag(dbTags, sortBy)
	if sortByTag != "" {
		orderBy = append(orderBy, fmt.Sprintf("%s %s %s", sortByTag, order, nullOrder))
	}

	creationTimeTag := dbclient.GetFieldTag(dbTags, "CreationTime")
	if sortByTag != creationTimeTag {
		if len(orderBy) > 0 {
			order = dbclient.DESC
		}
		orderBy = append(orderBy, fmt.Sprintf("%s %s %s", creationTimeTag, order, nullOrder))
	}
	return orderBy
}

// updateWorkload applies updates to a workload based on the patch request.
// Handles changes to priority, resources, image, entrypoint, and other workload properties.
func updateWorkload(adminWorkload *v1.Workload, req *types.PatchWorkloadRequest) error {
	if req.Priority != nil {
		adminWorkload.Spec.Priority = *req.Priority
	}
	if req.Replica != nil && *req.Replica != adminWorkload.Spec.Resource.Replica {
		_, ok := adminWorkload.Spec.CustomerLabels[v1.K8sHostName]
		if ok {
			return commonerrors.NewBadRequest("cannot update replica when specifying nodes")
		}
		adminWorkload.Spec.Resource.Replica = *req.Replica
	}
	if req.CPU != nil {
		adminWorkload.Spec.Resource.CPU = *req.CPU
	}
	if req.GPU != nil {
		adminWorkload.Spec.Resource.GPU = *req.GPU
	}
	if req.Memory != nil {
		adminWorkload.Spec.Resource.Memory = *req.Memory
	}
	if req.EphemeralStorage != nil {
		adminWorkload.Spec.Resource.EphemeralStorage = *req.EphemeralStorage
	}
	if req.SharedMemory != nil {
		adminWorkload.Spec.Resource.SharedMemory = *req.SharedMemory
	}
	if req.Image != nil && *req.Image != "" {
		adminWorkload.Spec.Image = *req.Image
	}
	if req.EntryPoint != nil && *req.EntryPoint != "" {
		adminWorkload.Spec.EntryPoint = *req.EntryPoint
	}
	if req.Description != nil {
		v1.SetAnnotation(adminWorkload, v1.DescriptionAnnotation, *req.Description)
	}
	if req.Timeout != nil {
		adminWorkload.Spec.Timeout = pointer.Int(*req.Timeout)
	}
	if req.Env != nil {
		adminWorkload.Spec.Env = *req.Env
	}
	if req.MaxRetry != nil {
		adminWorkload.Spec.MaxRetry = *req.MaxRetry
	}
	if req.CronJobs != nil {
		adminWorkload.Spec.CronJobs = *req.CronJobs
	}
	return nil
}

// cvtDBWorkloadToResponseItem converts a database workload record to a response item format.
// Maps database fields to the appropriate response structure with proper null value handling.
func (h *Handler) cvtDBWorkloadToResponseItem(ctx context.Context,
	w *dbclient.Workload,
) types.WorkloadResponseItem {
	result := types.WorkloadResponseItem{
		WorkloadId:    w.WorkloadId,
		WorkspaceId:   w.Workspace,
		ClusterId:     w.Cluster,
		Phase:         dbutils.ParseNullString(w.Phase),
		CreationTime:  dbutils.ParseNullTimeToString(w.CreationTime),
		StartTime:     dbutils.ParseNullTimeToString(w.StartTime),
		EndTime:       dbutils.ParseNullTimeToString(w.EndTime),
		DeletionTime:  dbutils.ParseNullTimeToString(w.DeletionTime),
		QueuePosition: w.QueuePosition,
		DispatchCount: w.DispatchCount,
		DisplayName:   w.DisplayName,
		Description:   dbutils.ParseNullString(w.Description),
		UserId:        dbutils.ParseNullString(w.UserId),
		UserName:      dbutils.ParseNullString(w.UserName),
		Priority:      w.Priority,
		IsTolerateAll: w.IsTolerateAll,
		WorkloadUid:   dbutils.ParseNullString(w.WorkloadUId),
		K8sObjectUid:  dbutils.ParseNullString(w.K8sObjectUid),
		AvgGpuUsage:   -1, // Default value when statistics are not available
	}
	if result.EndTime == "" && result.DeletionTime != "" {
		result.EndTime = result.DeletionTime
	}
	if startTime := dbutils.ParseNullTime(w.StartTime); !startTime.IsZero() {
		endTime, err := timeutil.CvtStrToRFC3339Milli(result.EndTime)
		nowTime := time.Now().UTC()
		if err != nil || endTime.After(nowTime) {
			endTime = nowTime
		}
		result.Duration = timeutil.FormatDuration(int64(endTime.Sub(startTime).Seconds()))
	} else {
		result.Duration = "0s"
	}
	json.Unmarshal([]byte(w.GVK), &result.GroupVersionKind)
	json.Unmarshal([]byte(w.Resource), &result.Resource)
	if w.Timeout > 0 {
		result.Timeout = pointer.Int(w.Timeout)
		if t := dbutils.ParseNullTime(w.StartTime); !t.IsZero() {
			result.SecondsUntilTimeout = t.Unix() + int64(w.Timeout) - time.Now().Unix()
			if result.SecondsUntilTimeout < 0 {
				result.SecondsUntilTimeout = 0
			}
		} else {
			result.SecondsUntilTimeout = -1
		}
	}
	if result.Phase == string(v1.WorkloadPending) {
		adminWorkload, err := h.getAdminWorkload(ctx, result.WorkloadId)
		if err == nil {
			result.Message = adminWorkload.Status.Message
		}
	}
	return result
}

// cvtDBWorkloadToGetResponse converts a database workload record to a detailed response format.
// Maps all database fields to the appropriate response structure including conditions, pods, etc.
func (h *Handler) cvtDBWorkloadToGetResponse(ctx context.Context,
	user *v1.User, roles []*v1.Role, dbWorkload *dbclient.Workload) *types.GetWorkloadResponse {
	result := &types.GetWorkloadResponse{
		WorkloadResponseItem: h.cvtDBWorkloadToResponseItem(ctx, dbWorkload),
		Image:                dbWorkload.Image,
		IsSupervised:         dbWorkload.IsSupervised,
		MaxRetry:             dbWorkload.MaxRetry,
	}
	if result.GroupVersionKind.Kind != common.AuthoringKind && dbWorkload.EntryPoint != "" {
		if stringutil.IsBase64(dbWorkload.EntryPoint) {
			result.EntryPoint = stringutil.Base64Decode(dbWorkload.EntryPoint)
		}
	}
	if dbWorkload.TTLSecond > 0 {
		result.TTLSecondsAfterFinished = pointer.Int(dbWorkload.TTLSecond)
	}
	if str := dbutils.ParseNullString(dbWorkload.Conditions); str != "" {
		json.Unmarshal([]byte(str), &result.Conditions)
	}
	if str := dbutils.ParseNullString(dbWorkload.Pods); str != "" {
		json.Unmarshal([]byte(str), &result.Pods)
		for i, p := range result.Pods {
			result.Pods[i].SSHAddr = h.buildSSHAddress(ctx, &p.WorkloadPod, result.UserId, result.WorkspaceId)
		}
	}
	if str := dbutils.ParseNullString(dbWorkload.Nodes); str != "" {
		json.Unmarshal([]byte(str), &result.Nodes)
	}
	if str := dbutils.ParseNullString(dbWorkload.Ranks); str != "" {
		json.Unmarshal([]byte(str), &result.Ranks)
	}
	if str := dbutils.ParseNullString(dbWorkload.CustomerLabels); str != "" {
		var customerLabels map[string]string
		json.Unmarshal([]byte(str), &customerLabels)
		if len(customerLabels) > 0 {
			result.CustomerLabels, result.SpecifiedNodes = parseCustomerLabelsAndNodes(customerLabels)
		}
	}
	if str := dbutils.ParseNullString(dbWorkload.Liveness); str != "" {
		json.Unmarshal([]byte(str), &result.Liveness)
	}
	if str := dbutils.ParseNullString(dbWorkload.Readiness); str != "" {
		json.Unmarshal([]byte(str), &result.Readiness)
	}
	if str := dbutils.ParseNullString(dbWorkload.Service); str != "" {
		json.Unmarshal([]byte(str), &result.Service)
	}
	if str := dbutils.ParseNullString(dbWorkload.Env); str != "" {
		json.Unmarshal([]byte(str), &result.Env)
		result.Env = maps.RemoveValue(result.Env, "")
	}
	if str := dbutils.ParseNullString(dbWorkload.Dependencies); str != "" {
		json.Unmarshal([]byte(str), &result.Dependencies)
	}
	if str := dbutils.ParseNullString(dbWorkload.CronJobs); str != "" {
		json.Unmarshal([]byte(str), &result.CronJobs)
	}
	// Only the user themselves or an administrator can get this info.
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:       ctx,
		ResourceKind:  v1.WorkloadKind,
		ResourceOwner: result.UserId,
		Verb:          v1.GetVerb,
		User:          user,
		Roles:         roles,
	}); err == nil {
		if str := dbutils.ParseNullString(dbWorkload.Secrets); str != "" {
			json.Unmarshal([]byte(str), &result.Secrets)
		}
	}
	return result
}

// parseCustomerLabelsAndNodes separates customer labels from node-specific labels.
// Extracts node list from customer labels and returns remaining labels separately.
func parseCustomerLabelsAndNodes(labels map[string]string) (map[string]string, []string) {
	var nodeList []string
	customerLabels := make(map[string]string)
	for key, val := range labels {
		if key == v1.K8sHostName {
			nodeList = strings.Split(val, " ")
		} else {
			customerLabels[key] = val
		}
	}
	return customerLabels, nodeList
}

// buildSSHAddress constructs the SSH address for accessing a workload pod.
// Generates the appropriate SSH command based on system configuration and pod status.
func (h *Handler) buildSSHAddress(ctx context.Context, pod *v1.WorkloadPod, userId, workspace string) string {
	if !commonconfig.IsSSHEnable() || pod.Phase != corev1.PodRunning {
		return ""
	}
	if userId == "" {
		userId = "none"
	}
	ep, err := h.clientSet.CoreV1().Endpoints(common.HigressNamespace).Get(ctx, common.HigressGateway, metav1.GetOptions{})
	if err != nil {
		klog.ErrorS(err, "Failed to get higress gateway")
		return ""
	}

	gatewayIp := ""
	for _, sub := range ep.Subsets {
		isMatch := false
		for _, p := range sub.Ports {
			if p.Port == common.HigressSSHPort && p.Protocol == corev1.ProtocolTCP {
				isMatch = true
				break
			}
		}
		if !isMatch {
			continue
		}
		if len(sub.Addresses) > 0 {
			gatewayIp = sub.Addresses[0].IP
			break
		}
	}
	if gatewayIp != "" {
		return fmt.Sprintf("ssh %s.%s.%s@%s", userId, pod.PodId, workspace, gatewayIp)
	}

	localIp, _ := netutil.GetLocalIp()
	if localIp == "" {
		return ""
	}
	return fmt.Sprintf("ssh -p %d %s.%s.%s@%s",
		commonconfig.GetSSHServerPort(), userId, pod.PodId, workspace, localIp)
}

// generateWorkloadForAuth creates a minimal workload object for authorization checks.
// Includes only the necessary information for performing authorization validations.
func generateWorkloadForAuth(name, userId, workspace, clusterId string) *v1.Workload {
	return &v1.Workload{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1.WorkloadKind,
			APIVersion: v1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				v1.UserIdLabel:      userId,
				v1.ClusterIdLabel:   clusterId,
				v1.WorkspaceIdLabel: workspace,
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: workspace,
		},
	}
}

// cvtDBWorkloadToAdminWorkload converts a database workload record to a workload CR object.
// Used for cloning workloads from database records to create new workload objects.
func cvtDBWorkloadToAdminWorkload(dbItem *dbclient.Workload) *v1.Workload {
	result := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(dbItem.DisplayName),
			Labels: map[string]string{
				v1.DisplayNameLabel: dbItem.DisplayName,
				v1.UserIdLabel:      dbutils.ParseNullString(dbItem.UserId),
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation: dbutils.ParseNullString(dbItem.Description),
				v1.UserNameAnnotation:    dbutils.ParseNullString(dbItem.UserName),
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace:     dbItem.Workspace,
			Image:         dbItem.Image,
			EntryPoint:    dbItem.EntryPoint,
			IsSupervised:  dbItem.IsSupervised,
			MaxRetry:      dbItem.MaxRetry,
			Priority:      dbItem.Priority,
			IsTolerateAll: dbItem.IsTolerateAll,
		},
	}
	json.Unmarshal([]byte(dbItem.Resource), &result.Spec.Resource)
	if str := dbutils.ParseNullString(dbItem.Env); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Env)
	}
	json.Unmarshal([]byte(dbItem.GVK), &result.Spec.GroupVersionKind)

	if dbItem.TTLSecond > 0 {
		result.Spec.TTLSecondsAfterFinished = pointer.Int(dbItem.TTLSecond)
	}
	if dbItem.Timeout > 0 {
		result.Spec.Timeout = pointer.Int(dbItem.Timeout)
	}
	if str := dbutils.ParseNullString(dbItem.CustomerLabels); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.CustomerLabels)
	}
	if str := dbutils.ParseNullString(dbItem.Liveness); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Liveness)
	}
	if str := dbutils.ParseNullString(dbItem.Readiness); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Readiness)
	}
	if str := dbutils.ParseNullString(dbItem.Service); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Service)
	}
	if str := dbutils.ParseNullString(dbItem.Dependencies); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Dependencies)
	}
	if str := dbutils.ParseNullString(dbItem.CronJobs); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.CronJobs)
	}
	if str := dbutils.ParseNullString(dbItem.Secrets); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Secrets)
	}
	return result
}

func (h *Handler) getWorkloadPodContainers(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(c.Request.Context(), requestUser)

	var (
		ctx           = c.Request.Context()
		name          = c.GetString(common.Name)
		podName       = strings.TrimSpace(c.Param(common.PodId))
		adminWorkload *v1.Workload
	)

	if commonconfig.IsDBEnable() {
		dbWorkload, err := h.dbClient.GetWorkload(ctx, name)
		if err != nil {
			return nil, err
		}
		adminWorkload = generateWorkloadForAuth(name, dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
	} else {
		adminWorkload, err = h.getAdminWorkload(ctx, name)
		if err != nil {
			return nil, err
		}
	}

	if err = h.authWorkloadAction(c, adminWorkload, v1.GetVerb, requestUser, roles); err != nil {
		return nil, err
	}

	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, v1.GetClusterId(adminWorkload))
	if err != nil {
		return nil, err
	}
	pod, err := k8sClients.ClientSet().CoreV1().Pods(v1.GetWorkspaceId(adminWorkload)).Get(c.Request.Context(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	containers := make([]types.GetWorkloadPodContainersItem, len(pod.Spec.Containers))
	for index, container := range pod.Spec.Containers {
		containers[index] = types.GetWorkloadPodContainersItem{Name: container.Name}
	}

	return &types.GetWorkloadPodContainersResponse{
		Containers: containers,
		Shells:     []string{"bash", "sh", "zsh"},
	}, nil
}
