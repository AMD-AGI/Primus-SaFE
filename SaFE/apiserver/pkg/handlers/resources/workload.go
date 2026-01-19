/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package resources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/resources/view"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonnodes "github.com/AMD-AIG-AIMA/SAFE/common/pkg/nodes"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	commonworkload "github.com/AMD-AIG-AIMA/SAFE/common/pkg/workload"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/backoff"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/concurrent"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	maputil "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/maps"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/netutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type WorkloadBatchAction string

const (
	defaultLogTailLine int64 = 1000
	defaultRetryCount        = 5
	defaultRetryDelay        = 200 * time.Millisecond

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
	ctx := c.Request.Context()
	req := &view.CreateWorkloadRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		return nil, err
	}
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	roles := h.accessController.GetRoles(ctx, requestUser)

	mainWorkload, err := h.generateWorkload(ctx, req, body, requestUser)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	var preheatWorkload *v1.Workload
	isSucceed := false
	defer func() {
		if !isSucceed {
			h.cleanUpWorkloads(ctx, mainWorkload, preheatWorkload)
		}
	}()

	if req.Preheat {
		preheatWorkload, err = h.createPreheatWorkload(c, mainWorkload, req, requestUser, roles)
		if err != nil {
			return nil, err
		}
		mainWorkload.Spec.Dependencies = []string{preheatWorkload.Name}
	}

	resp, err := h.createWorkloadImpl(c, mainWorkload, requestUser, roles)
	if err != nil {
		return nil, err
	}
	isSucceed = true
	return resp, nil
}

// createWorkloadImpl performs the actual workload creation in the system.
// Handles authorization checks, workload creation in etcd, and initial phase setting.
func (h *Handler) createWorkloadImpl(c *gin.Context,
	workload *v1.Workload, requestUser *v1.User, roles []*v1.Role) (*view.CreateWorkloadResponse, error) {
	var err error
	if err = h.authWorkloadAction(c, workload, v1.CreateVerb, v1.WorkloadKind, requestUser, roles); err != nil {
		klog.ErrorS(err, "failed to auth workload", "workload", workload.Name,
			"workspace", workload.Spec.Workspace, "user", c.GetString(common.UserName))
		return nil, err
	}
	priorityKind := generatePriority(workload.Spec.Priority)
	if err = h.authWorkloadAction(c, workload, v1.CreateVerb, priorityKind, requestUser, roles); err != nil {
		klog.ErrorS(err, "failed to auth workload priority", "workload", workload.Name,
			"priority", workload.Spec.Priority, "user", c.GetString(common.UserName))
		return nil, err
	}

	if v1.IsPrivileged(workload) {
		if err = h.authWorkloadAction(c, workload, v1.CreateVerb, authority.WorkloadPrivilegedKind, requestUser, roles); err != nil {
			klog.ErrorS(err, "failed to auth workload privileged",
				"workload", workload.Name, "user", c.GetString(common.UserName))
			return nil, err
		}
	}

	for i, sec := range workload.Spec.Secrets {
		secret, err := h.getAndAuthorizeSecret(c.Request.Context(), sec.Id, workload.Spec.Workspace, requestUser, v1.GetVerb)
		if err != nil {
			klog.ErrorS(err, "failed to get workload secrets", "workload", workload.Name,
				"secret", sec.Id, "user", c.GetString(common.UserName))
			return nil, err
		}
		workload.Spec.Secrets[i].Type = v1.SecretType(v1.GetSecretType(secret))
	}
	if v1.GetUserId(workload) == "" {
		v1.SetLabel(workload, v1.UserIdLabel, requestUser.Name)
	}
	if v1.GetUserName(workload) == "" {
		v1.SetAnnotation(workload, v1.UserNameAnnotation, v1.GetUserName(requestUser))
	}
	if err = h.Create(c.Request.Context(), workload); err != nil {
		return nil, err
	}
	if err = h.updateWorkloadPhase(c.Request.Context(), workload, v1.WorkloadPending, nil); err != nil {
		return nil, err
	}
	klog.Infof("create workload, name: %s, user: %s/%s, priority: %d, timeout: %d, resources: %s",
		workload.Name, c.GetString(common.UserName), c.GetString(common.UserId),
		workload.Spec.Priority, workload.GetTimeout(), string(jsonutils.MarshalSilently(workload.Spec.Resources)))
	return &view.CreateWorkloadResponse{WorkloadId: workload.Name}, nil
}

// cleanUpWorkloads cleans up workloads when workload creation fails
// It deletes the main workload and preheat workload if they exist,
// and cleans up any CICD secrets associated with the main workload
func (h *Handler) cleanUpWorkloads(ctx context.Context, mainWorkload, preheatWorkload *v1.Workload) {
	h.cleanupCICDSecrets(ctx, mainWorkload)
	if preheatWorkload != nil {
		if err := h.Delete(ctx, preheatWorkload); err != nil && !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete preheat workload", "workload", preheatWorkload.Name)
		}
	}
	if mainWorkload != nil {
		if err := h.Delete(ctx, mainWorkload); err != nil && !apierrors.IsNotFound(err) {
			klog.ErrorS(err, "failed to delete main workload", "workload", mainWorkload.Name)
		}
	}
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
	if err = h.authWorkloadAction(c, adminWorkload, v1.ListVerb, v1.WorkloadKind, requestUser, roles); err != nil {
		return nil, err
	}

	dbSql, orderBy := cvtToListWorkloadSql(query)
	ctx := c.Request.Context()
	workloads, err := h.dbClient.SelectWorkloads(ctx, dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}

	result := &view.ListWorkloadResponse{}
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
	if err = h.authWorkloadAction(c, adminWorkload, v1.GetVerb, v1.WorkloadKind, requestUser, roles); err != nil {
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
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, v1.WorkloadKind, requestUser, roles); err != nil {
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
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, v1.WorkloadKind, requestUser, roles); err != nil {
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
		Type:               string(v1.AdminStopped),
		Status:             metav1.ConditionTrue,
		Message:            message,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             commonworkload.GenerateDispatchReason(v1.GetWorkloadDispatchCnt(adminWorkload)),
	}
	if err := h.updateWorkloadPhase(ctx, adminWorkload, v1.WorkloadStopped, cond); err != nil {
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
		phase := dbutils.ParseNullString(dbWorkload.Phase)
		if phase == string(v1.WorkloadStopped) || phase == string(v1.WorkloadSucceeded) || phase == string(v1.WorkloadFailed) {
			return nil, nil
		}
		adminWorkload = generateWorkloadForAuth(name,
			dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, v1.WorkloadKind, requestUser, roles); err != nil {
			return nil, err
		}
		if err = h.dbClient.SetWorkloadStopped(c.Request.Context(), name); err != nil {
			return nil, err
		}
	} else {
		if adminWorkload.IsEnd() {
			return nil, nil
		}
		if err = h.authWorkloadAction(c, adminWorkload, v1.DeleteVerb, v1.WorkloadKind, requestUser, roles); err != nil {
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
	ctx := c.Request.Context()
	roles := h.accessController.GetRoles(ctx, requestUser)

	name := c.GetString(common.Name)
	adminWorkload, err := h.getAdminWorkload(ctx, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, commonerrors.NewInternalError("The workload can only be edited when it is running.")
		}
		return nil, err
	}

	req := &view.PatchWorkloadRequest{}
	if _, err = apiutils.ParseRequestBody(c.Request, req); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	if err = h.authWorkloadAction(c, adminWorkload, v1.UpdateVerb, v1.WorkloadKind, requestUser, roles); err != nil {
		return nil, err
	}
	if req.Priority != nil {
		priorityKind := generatePriority(*req.Priority)
		if err = h.authWorkloadAction(c, adminWorkload, v1.UpdateVerb, priorityKind, requestUser, roles); err != nil {
			return nil, err
		}
	}

	if err = backoff.ConflictRetry(func() error {
		var innerError error
		if innerError = applyWorkloadPatch(adminWorkload, req); innerError != nil {
			return innerError
		}
		if innerError = h.updateWorkload(ctx, adminWorkload, requestUser, req); innerError == nil {
			return nil
		}
		if apierrors.IsConflict(innerError) {
			adminWorkload, _ = h.getAdminWorkload(ctx, name)
			if adminWorkload == nil {
				return commonerrors.NewNotFoundWithMessage(fmt.Sprintf("The workload %s is not found", name))
			}
		}
		return innerError
	}, defaultRetryCount, defaultRetryDelay); err != nil {
		klog.ErrorS(err, "failed to update workload", "name", adminWorkload.Name)
		return nil, err
	}
	klog.Infof("update workload, name: %s, request: %s", name, string(jsonutils.MarshalSilently(*req)))
	return nil, nil
}

// updateWorkload updates the workload in the system and handles CICD secret updates
// if a new GitHub PAT token is provided in the request
func (h *Handler) updateWorkload(ctx context.Context,
	adminWorkload *v1.Workload, requestUser *v1.User, req *view.PatchWorkloadRequest) error {
	err := h.Update(ctx, adminWorkload)
	if err != nil {
		return err
	}

	if req.Env != nil && commonworkload.IsCICDScalingRunnerSet(adminWorkload) {
		if newToken := (*req.Env)[GithubPAT]; newToken != "" {
			patch := client.MergeFrom(adminWorkload.DeepCopy())
			if err = h.updateCICDSecret(ctx, adminWorkload, requestUser, newToken); err != nil {
				klog.ErrorS(err, "failed to update cicd secret")
				return err
			}
			if err = h.Patch(ctx, adminWorkload, patch); err != nil {
				klog.ErrorS(err, "failed to patch workload")
				return err
			}
		}
	}
	return nil
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
	if commonworkload.IsCICDScalingRunnerSet(workload) {
		return nil, commonerrors.NewNotImplemented("the clone function is not supported for cicd scaling runner")
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  c.Request.Context(),
		Resource: workload,
		Verb:     v1.GetVerb,
		User:     requestUser,
		Roles:    roles,
	}); err != nil {
		return nil, err
	}
	v1.RemoveLabel(workload, v1.UserIdLabel)
	v1.RemoveAnnotation(workload, v1.UserNameAnnotation)
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
	if err = h.authWorkloadAction(c, workload, v1.GetVerb, v1.WorkloadKind, requestUser, roles); err != nil {
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
	return &view.GetWorkloadPodLogResponse{
		WorkloadId: workload.Name,
		PodId:      podName,
		Namespace:  workload.Spec.Workspace,
		Logs:       strings.Split(string(podLogs), "\n"),
	}, nil
}

// updateWorkloadPhase updates the phase of a workload and optionally adds a condition.
// Handles status updates including setting end time for stopped workloads.
func (h *Handler) updateWorkloadPhase(ctx context.Context,
	workload *v1.Workload, phase v1.WorkloadPhase, cond *metav1.Condition) error {
	shouldUpdateConditions := func(workload *v1.Workload, cond *metav1.Condition) bool {
		if cond == nil {
			return false
		}
		return meta.FindStatusCondition(workload.Status.Conditions, cond.Type) == nil
	}
	name := workload.Name
	if err := backoff.ConflictRetry(func() error {
		if phase == workload.Status.Phase && !shouldUpdateConditions(workload, cond) {
			return nil
		}
		// Build a minimal JSON merge patch for status sub-resource with RV precondition
		statusPatch := map[string]any{}
		if phase != workload.Status.Phase {
			statusPatch["phase"] = phase
			if phase == v1.WorkloadStopped && workload.Status.EndTime == nil {
				statusPatch["endTime"] = &metav1.Time{Time: time.Now().UTC()}
			}
		}
		if shouldUpdateConditions(workload, cond) {
			statusPatch["conditions"] = append(workload.Status.Conditions, *cond)
		}
		patchObj := map[string]any{
			"metadata": map[string]any{
				"resourceVersion": workload.ResourceVersion,
			},
			"status": statusPatch,
		}
		p := jsonutils.MarshalSilently(patchObj)
		if innerError := h.Status().Patch(ctx, workload, client.RawPatch(apitypes.MergePatchType, p)); innerError == nil {
			return nil
		} else {
			if apierrors.IsConflict(innerError) {
				if workload, _ = h.getAdminWorkload(ctx, name); workload == nil {
					return commonerrors.NewNotFoundWithMessage(fmt.Sprintf("The workload %s is not found", name))
				}
			}
			return innerError
		}
	}, defaultRetryCount, defaultRetryDelay); err != nil {
		klog.ErrorS(err, "failed to update workload status", "name", workload.Name)
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
	adminWorkload *v1.Workload, verb v1.RoleVerb, resourceKind string, requestUser *v1.User, roles []*v1.Role) error {
	var workspaces []string
	if adminWorkload.Spec.Workspace != "" {
		workspaces = append(workspaces, adminWorkload.Spec.Workspace)
	}
	resourceOwner := v1.GetUserId(adminWorkload)
	if verb == v1.CreateVerb {
		resourceOwner = ""
	}
	if err := h.accessController.Authorize(authority.AccessInput{
		Context:       c.Request.Context(),
		ResourceKind:  resourceKind,
		ResourceOwner: resourceOwner,
		ResourceName:  adminWorkload.Name,
		Verb:          verb,
		Workspaces:    workspaces,
		User:          requestUser,
		UserId:        c.GetString(common.UserId),
		Roles:         roles,
	}); err != nil {
		return err
	}
	return nil
}

// generateWorkload creates a new workload object based on the creation request.
// Populates workload metadata, specifications, and customer labels.
func (h *Handler) generateWorkload(ctx context.Context,
	req *view.CreateWorkloadRequest, body []byte, requestUser *v1.User) (*v1.Workload, error) {
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
	if req.WorkloadId != "" {
		workload.Name = req.WorkloadId
	}
	var err error
	if err = json.Unmarshal(body, &workload.Spec); err != nil {
		return nil, err
	}
	controlPlaneIp, err := h.getAdminControlPlaneIp(ctx)
	if err != nil {
		return nil, err
	}
	v1.SetAnnotation(workload, v1.AdminControlPlaneAnnotation, controlPlaneIp)

	if commonworkload.IsAuthoring(workload) {
		if len(req.SpecifiedNodes) > 1 {
			return nil, fmt.Errorf("the authoring can only be created with one node")
		}
	}
	genCustomerLabelsByNodes(workload, req.SpecifiedNodes, v1.K8sHostName)
	if len(req.SpecifiedNodes) == 0 {
		genCustomerLabelsByNodes(workload, req.ExcludedNodes, common.ExcludedNodes)
	}
	if req.WorkspaceId != "" {
		workload.Spec.Workspace = req.WorkspaceId
	}
	for key, val := range req.Labels {
		if !strings.HasPrefix(key, v1.PrimusSafePrefix) {
			workload.Labels[key] = val
		}
	}
	for key, val := range req.Annotations {
		if !strings.HasPrefix(key, v1.PrimusSafePrefix) {
			workload.Annotations[key] = val
		}
	}
	if commonworkload.IsCICDScalingRunnerSet(workload) {
		if err = h.generateCICDScaleRunnerSet(ctx, workload, requestUser); err != nil {
			return nil, err
		}
	}
	if req.UserEntity != nil && h.accessController.AuthorizeSystemAdmin(
		authority.AccessInput{Context: ctx, User: requestUser}, false) == nil {
		v1.SetLabel(workload, v1.UserIdLabel, req.UserEntity.Id)
		v1.SetAnnotation(workload, v1.UserNameAnnotation, req.UserEntity.Name)
	}
	if req.Privileged {
		v1.SetAnnotation(workload, v1.WorkloadPrivilegedAnnotation, v1.TrueStr)
	}
	return workload, nil
}

// createPreheatWorkload create a preheat workload based on the main workload configuration for resource warming up
func (h *Handler) createPreheatWorkload(c *gin.Context, mainWorkload *v1.Workload,
	mainQuery *view.CreateWorkloadRequest, requestUser *v1.User, roles []*v1.Role) (*v1.Workload, error) {
	displayName := v1.GetDisplayName(mainWorkload)
	description := "preheat"
	if len(displayName) > commonutils.MaxPytorchJobNameLen-len(description)-1 {
		displayName = displayName[:commonutils.MaxPytorchJobNameLen-len(description)-1]
	}
	displayName = description + "-" + displayName

	preheatWorkload := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(displayName),
			Labels: map[string]string{
				v1.DisplayNameLabel: displayName,
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation:       v1.OpsJobKind,
				v1.RequireNodeSpreadAnnotation: v1.TrueStr,
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace: mainWorkload.Spec.Workspace,
			GroupVersionKind: v1.GroupVersionKind{
				Kind:    common.PytorchJobKind,
				Version: common.DefaultVersion,
			},
			Priority:                mainWorkload.Spec.Priority,
			TTLSecondsAfterFinished: pointer.Int(10),
			Timeout:                 pointer.Int(3600),
			IsTolerateAll:           mainWorkload.Spec.IsTolerateAll,
			CustomerLabels:          mainWorkload.Spec.CustomerLabels,
			Secrets:                 mainWorkload.Spec.Secrets,
		},
	}
	for i := range mainWorkload.Spec.Resources {
		preheatWorkload.Spec.Images = append(preheatWorkload.Spec.Images, mainWorkload.Spec.Images[i])
		preheatWorkload.Spec.EntryPoints = append(preheatWorkload.Spec.EntryPoints,
			stringutil.Base64Encode("echo \"preheat finished\""))
		preheatWorkload.Spec.Resources = append(preheatWorkload.Spec.Resources, v1.WorkloadResource{
			CPU:              "1",
			Memory:           "8Gi",
			EphemeralStorage: "50Gi",
		})
	}

	if len(mainQuery.SpecifiedNodes) > 0 {
		preheatWorkload.Spec.Resources[0].Replica = len(mainQuery.SpecifiedNodes)
	} else {
		workspace, err := h.getAdminWorkspace(c.Request.Context(), preheatWorkload.Spec.Workspace)
		if err != nil {
			return nil, err
		}
		if mainWorkload.Spec.IsTolerateAll {
			preheatWorkload.Spec.Resources[0].Replica = workspace.CurrentReplica()
		} else {
			preheatWorkload.Spec.Resources[0].Replica = workspace.Status.AvailableReplica
		}
	}
	resp, err := h.createWorkloadImpl(c, preheatWorkload, requestUser, roles)
	if err != nil {
		return nil, err
	}
	preheatWorkload.Name = resp.WorkloadId

	return preheatWorkload, nil
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

	req := &view.BatchWorkloadsRequest{}
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
func genCustomerLabelsByNodes(workload *v1.Workload, nodeList []string, labelKey string) {
	if len(nodeList) == 0 {
		return
	}
	if len(workload.Spec.CustomerLabels) > 0 {
		if _, ok := workload.Spec.CustomerLabels[labelKey]; ok {
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
	workload.Spec.CustomerLabels[labelKey] = nodeNames
}

// parseListWorkloadQuery parses and validates the query parameters for listing workloads.
// Handles URL decoding and sets default values for pagination and sorting.
func parseListWorkloadQuery(c *gin.Context) (*view.ListWorkloadRequest, error) {
	query := &view.ListWorkloadRequest{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.UserName != "" {
		if nameUnescape, err := url.QueryUnescape(query.UserName); err == nil {
			query.UserName = nameUnescape
		}
	}
	if query.Limit <= 0 {
		query.Limit = view.DefaultQueryLimit
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
func parseGetPodLogQuery(c *gin.Context, mainContainerName string) (*view.GetPodLogRequest, error) {
	query := &view.GetPodLogRequest{}
	var err error
	if err = c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.TailLines <= 0 {
		query.TailLines = defaultLogTailLine
	}
	if query.Container == "" {
		query.Container = mainContainerName
	}
	return query, nil
}

// cvtToListWorkloadSql converts workload list query parameters into a database SQL query.
// Builds WHERE conditions and ORDER BY clauses based on filter parameters.
func cvtToListWorkloadSql(query *view.ListWorkloadRequest) (sqrl.Sqlizer, []string) {
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
	descriptionField := dbclient.GetFieldTag(dbTags, "Description")
	if description := strings.TrimSpace(query.Description); description != "" {
		dbSql = append(dbSql,
			sqrl.Like{descriptionField: fmt.Sprintf("%%%s%%", description)})
	} else {
		dbSql = append(dbSql, sqrl.Or{
			sqrl.Eq{descriptionField: nil},              // Description IS NULL
			sqrl.NotEq{descriptionField: v1.OpsJobKind}, // Description != 'OpsJobKind'
		})
	}

	userNameField := dbclient.GetFieldTag(dbTags, "UserName")
	if userName := strings.TrimSpace(query.UserName); userName != "" {
		dbSql = append(dbSql, sqrl.Like{userNameField: fmt.Sprintf("%%%s%%", userName)})
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
	if scaleRunnerSet := strings.TrimSpace(query.ScaleRunnerSet); scaleRunnerSet != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "ScaleRunnerSet"): scaleRunnerSet})
	}
	if scaleRunnerId := strings.TrimSpace(query.ScaleRunnerId); scaleRunnerId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "ScaleRunnerId"): scaleRunnerId})
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

// applyWorkloadPatch applies updates to a workload based on the patch request.
// Handles changes to priority, resources, image, entrypoint, and other workload properties.
func applyWorkloadPatch(adminWorkload *v1.Workload, req *view.PatchWorkloadRequest) error {
	if req.Priority != nil {
		adminWorkload.Spec.Priority = *req.Priority
	}
	if req.Resources != nil {
		reqCount := 0
		for _, res := range *req.Resources {
			reqCount += res.Replica
		}
		if reqCount != commonworkload.GetTotalCount(adminWorkload) {
			_, ok := adminWorkload.Spec.CustomerLabels[v1.K8sHostName]
			if ok {
				return commonerrors.NewBadRequest("cannot update replica when specifying nodes")
			}
		}
		adminWorkload.Spec.Resources = *req.Resources
	}
	if req.Images != nil && len(*req.Images) > 0 {
		adminWorkload.Spec.Images = *req.Images
	}
	if req.EntryPoints != nil && len(*req.EntryPoints) > 0 {
		adminWorkload.Spec.EntryPoints = *req.EntryPoints
	}
	if req.Description != nil {
		v1.SetAnnotation(adminWorkload, v1.DescriptionAnnotation, *req.Description)
	}
	if req.Timeout != nil {
		adminWorkload.Spec.Timeout = pointer.Int(*req.Timeout)
	}
	if req.Env != nil {
		adminWorkload.Spec.Env = maputil.Copy(*req.Env, GithubPAT)
	}
	if req.MaxRetry != nil {
		adminWorkload.Spec.MaxRetry = *req.MaxRetry
	}
	if req.CronJobs != nil {
		adminWorkload.Spec.CronJobs = *req.CronJobs
	}
	if req.Service != nil {
		adminWorkload.Spec.Service = req.Service
	}
	return nil
}

// cvtDBWorkloadToResponseItem converts a database workload record to a response item format.
// Maps database fields to the appropriate response structure with proper null value handling.
func (h *Handler) cvtDBWorkloadToResponseItem(ctx context.Context, dbWorkload *dbclient.Workload) view.WorkloadResponseItem {
	result := view.WorkloadResponseItem{
		WorkloadId:     dbWorkload.WorkloadId,
		WorkspaceId:    dbWorkload.Workspace,
		ClusterId:      dbWorkload.Cluster,
		Phase:          dbutils.ParseNullString(dbWorkload.Phase),
		CreationTime:   dbutils.ParseNullTimeToString(dbWorkload.CreationTime),
		StartTime:      dbutils.ParseNullTimeToString(dbWorkload.StartTime),
		EndTime:        dbutils.ParseNullTimeToString(dbWorkload.EndTime),
		DeletionTime:   dbutils.ParseNullTimeToString(dbWorkload.DeletionTime),
		QueuePosition:  dbWorkload.QueuePosition,
		DispatchCount:  dbWorkload.DispatchCount,
		DisplayName:    dbWorkload.DisplayName,
		Description:    dbutils.ParseNullString(dbWorkload.Description),
		UserId:         dbutils.ParseNullString(dbWorkload.UserId),
		UserName:       dbutils.ParseNullString(dbWorkload.UserName),
		Priority:       dbWorkload.Priority,
		IsTolerateAll:  dbWorkload.IsTolerateAll,
		WorkloadUid:    dbutils.ParseNullString(dbWorkload.WorkloadUId),
		AvgGpuUsage:    -1, // Default value when statistics are not available
		ScaleRunnerSet: dbutils.ParseNullString(dbWorkload.ScaleRunnerSet),
		ScaleRunnerId:  dbutils.ParseNullString(dbWorkload.ScaleRunnerId),
		MaxRetry:       dbWorkload.MaxRetry,
	}
	if result.EndTime == "" && result.DeletionTime != "" {
		result.EndTime = result.DeletionTime
	}
	if startTime := dbutils.ParseNullTime(dbWorkload.StartTime); !startTime.IsZero() {
		endTime, err := timeutil.CvtStrToRFC3339Milli(result.EndTime)
		nowTime := time.Now().UTC()
		if err != nil || endTime.After(nowTime) {
			endTime = nowTime
		}
		result.Duration = timeutil.FormatDuration(int64(endTime.Sub(startTime).Seconds()))
	} else {
		result.Duration = "0s"
	}
	json.Unmarshal([]byte(dbWorkload.GVK), &result.GroupVersionKind)
	result.Resources = cvtToWorkloadResources(dbWorkload, result.GroupVersionKind.Kind)

	if dbWorkload.Timeout > 0 {
		result.Timeout = pointer.Int(dbWorkload.Timeout)
		if result.EndTime == "" {
			if t := dbutils.ParseNullTime(dbWorkload.StartTime); !t.IsZero() {
				result.SecondsUntilTimeout = t.Unix() + int64(dbWorkload.Timeout) - time.Now().Unix()
				if result.SecondsUntilTimeout < 0 {
					result.SecondsUntilTimeout = 0
				}
			} else {
				result.SecondsUntilTimeout = -1
			}
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
	user *v1.User, roles []*v1.Role, dbWorkload *dbclient.Workload) *view.GetWorkloadResponse {
	result := &view.GetWorkloadResponse{
		WorkloadResponseItem: h.cvtDBWorkloadToResponseItem(ctx, dbWorkload),
		IsSupervised:         dbWorkload.IsSupervised,
	}
	result.Images = cvtToWorkloadImages(dbWorkload, len(result.Resources))
	if result.GroupVersionKind.Kind != common.AuthoringKind {
		result.EntryPoints = cvtToWorkloadEntryPoints(dbWorkload, len(result.Resources))
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
			result.CustomerLabels, result.SpecifiedNodes, result.ExcludedNodes = parseCustomerLabels(customerLabels)
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
	}
	if str := dbutils.ParseNullString(dbWorkload.Dependencies); str != "" {
		var dependencies []string
		json.Unmarshal([]byte(str), &dependencies)
		for _, id := range dependencies {
			item, err := h.dbClient.GetWorkload(ctx, id)
			if err == nil && dbutils.ParseNullString(item.Description) != v1.OpsJobKind {
				result.Dependencies = append(result.Dependencies, id)
			}
		}
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

// parseCustomerLabels separates user-defined labels from node selection labels.
// Returns custom labels, specified nodes, and excluded nodes.
func parseCustomerLabels(labels map[string]string) (map[string]string, []string, []string) {
	var specifiedNodes []string
	var excludedNodes []string
	customerLabels := make(map[string]string)
	for key, val := range labels {
		switch key {
		case v1.K8sHostName:
			specifiedNodes = strings.Split(val, " ")
		case common.ExcludedNodes:
			excludedNodes = strings.Split(val, " ")
		default:
			customerLabels[key] = val
		}
	}
	return customerLabels, specifiedNodes, excludedNodes
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
func cvtDBWorkloadToAdminWorkload(dbWorkload *dbclient.Workload) *v1.Workload {
	result := &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(dbWorkload.DisplayName),
			Labels: map[string]string{
				v1.DisplayNameLabel: dbWorkload.DisplayName,
				v1.UserIdLabel:      dbutils.ParseNullString(dbWorkload.UserId),
			},
			Annotations: map[string]string{
				v1.DescriptionAnnotation: dbutils.ParseNullString(dbWorkload.Description),
				v1.UserNameAnnotation:    dbutils.ParseNullString(dbWorkload.UserName),
			},
		},
		Spec: v1.WorkloadSpec{
			Workspace:     dbWorkload.Workspace,
			IsSupervised:  dbWorkload.IsSupervised,
			MaxRetry:      dbWorkload.MaxRetry,
			Priority:      dbWorkload.Priority,
			IsTolerateAll: dbWorkload.IsTolerateAll,
		},
	}
	json.Unmarshal([]byte(dbWorkload.GVK), &result.Spec.GroupVersionKind)
	result.Spec.Resources = cvtToWorkloadResources(dbWorkload, result.SpecKind())
	result.Spec.Images = cvtToWorkloadImages(dbWorkload, len(result.Spec.Resources))
	result.Spec.EntryPoints = cvtToWorkloadEntryPoints(dbWorkload, len(result.Spec.Resources))

	if str := dbutils.ParseNullString(dbWorkload.Env); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Env)
	}
	if dbWorkload.TTLSecond > 0 {
		result.Spec.TTLSecondsAfterFinished = pointer.Int(dbWorkload.TTLSecond)
	}
	if dbWorkload.Timeout > 0 {
		result.Spec.Timeout = pointer.Int(dbWorkload.Timeout)
	}
	if str := dbutils.ParseNullString(dbWorkload.CustomerLabels); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.CustomerLabels)
	}
	if str := dbutils.ParseNullString(dbWorkload.Liveness); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Liveness)
	}
	if str := dbutils.ParseNullString(dbWorkload.Readiness); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Readiness)
	}
	if str := dbutils.ParseNullString(dbWorkload.Service); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Service)
	}
	if str := dbutils.ParseNullString(dbWorkload.Dependencies); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Dependencies)
	}
	if str := dbutils.ParseNullString(dbWorkload.CronJobs); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.CronJobs)
	}
	if str := dbutils.ParseNullString(dbWorkload.Secrets); str != "" {
		json.Unmarshal([]byte(str), &result.Spec.Secrets)
	}
	if str := dbutils.ParseNullString(dbWorkload.ScaleRunnerSet); str != "" {
		if len(result.Spec.Env) == 0 {
			result.Spec.Env = make(map[string]string)
		}
		result.Spec.Env[common.ScaleRunnerSetID] = str
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
		adminWorkload = generateWorkloadForAuth(name,
			dbutils.ParseNullString(dbWorkload.UserId), dbWorkload.Workspace, dbWorkload.Cluster)
	} else {
		adminWorkload, err = h.getAdminWorkload(ctx, name)
		if err != nil {
			return nil, err
		}
	}

	if err = h.authWorkloadAction(c, adminWorkload, v1.GetVerb, v1.WorkloadKind, requestUser, roles); err != nil {
		return nil, err
	}

	k8sClients, err := commonutils.GetK8sClientFactory(h.clientManager, v1.GetClusterId(adminWorkload))
	if err != nil {
		return nil, err
	}
	pod, err := k8sClients.ClientSet().CoreV1().Pods(
		v1.GetWorkspaceId(adminWorkload)).Get(c.Request.Context(), podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	containers := make([]view.GetWorkloadPodContainersItem, len(pod.Spec.Containers))
	for index, container := range pod.Spec.Containers {
		containers[index] = view.GetWorkloadPodContainersItem{Name: container.Name}
	}

	return &view.GetWorkloadPodContainersResponse{
		Containers: containers,
		Shells:     []string{"bash", "sh", "zsh"},
	}, nil
}

func cvtToWorkloadResources(dbWorkload *dbclient.Workload, kind string) []v1.WorkloadResource {
	var resources []v1.WorkloadResource
	if val := dbutils.ParseNullString(dbWorkload.Resources); val != "" {
		json.Unmarshal([]byte(val), &resources)
	}
	if len(resources) == 0 {
		var resource v1.WorkloadResource
		if json.Unmarshal([]byte(dbWorkload.Resource), &resource) == nil {
			resources = commonworkload.ConvertResourceToList(resource, kind)
		}
	}
	return resources
}

func cvtToWorkloadImages(dbWorkload *dbclient.Workload, count int) []string {
	var images []string
	if val := dbutils.ParseNullString(dbWorkload.Images); val != "" {
		json.Unmarshal([]byte(val), &images)
	}
	if len(images) == 0 && dbWorkload.Image != "" {
		for i := 0; i < count; i++ {
			images = append(images, dbWorkload.Image)
		}
	}
	return images
}
func cvtToWorkloadEntryPoints(dbWorkload *dbclient.Workload, count int) []string {
	var entryPoints []string
	if val := dbutils.ParseNullString(dbWorkload.EntryPoints); val != "" {
		json.Unmarshal([]byte(val), &entryPoints)
	}
	if len(entryPoints) == 0 && dbWorkload.EntryPoint != "" {
		for i := 0; i < count; i++ {
			entryPoints = append(entryPoints, dbWorkload.EntryPoint)
		}
	}
	for i := 0; i < len(entryPoints); i++ {
		if stringutil.IsBase64(entryPoints[i]) {
			entryPoints[i] = stringutil.Base64Decode(entryPoints[i])
		}
	}
	return entryPoints
}

func generatePriority(priority int) string {
	return fmt.Sprintf("workload/%s", commonworkload.GeneratePriority(priority))
}
