/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cd

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/client-go/kubernetes"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"
)

type handleFunc func(*gin.Context) (interface{}, error)

// handle executes the handler function and processes the response/error
func handle(c *gin.Context, fn handleFunc) {
	response, err := fn(c)
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	code := http.StatusOK
	if c.Writer.Status() > 0 {
		code = c.Writer.Status()
	}
	switch responseType := response.(type) {
	case []byte:
		c.Data(code, common.JsonContentType, responseType)
	case string:
		c.Data(code, common.JsonContentType, []byte(responseType))
	default:
		c.JSON(code, responseType)
	}
}

type Handler struct {
	client.Client
	service          *Service
	clientSet        kubernetes.Interface
	dbClient         *dbclient.Client
	httpClient       httpclient.Interface
	clientManager    *commonutils.ObjectManager
	accessController *authority.AccessController
}

func NewHandler(mgr ctrlruntime.Manager) (*Handler, error) {
	clientSet, err := k8sclient.NewClientSetWithRestConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	var dbClient *dbclient.Client
	if commonconfig.IsDBEnable() {
		if dbClient = dbclient.NewClient(); dbClient == nil {
			return nil, fmt.Errorf("failed to new db client")
		}
	}

	h := &Handler{
		Client:           mgr.GetClient(),
		clientSet:        clientSet,
		dbClient:         dbClient,
		service:          NewService(dbClient, clientSet),
		httpClient:       httpclient.NewClient(),
		clientManager:    commonutils.NewObjectManagerSingleton(),
		accessController: authority.NewAccessController(mgr.GetClient()),
	}

	return h, nil
}

// CreateDeploymentRequest handles creation of a new deployment request
func (h *Handler) CreateDeploymentRequest(c *gin.Context) {
	handle(c, h.createDeploymentRequest)
}

// ListDeploymentRequests lists requests
func (h *Handler) ListDeploymentRequests(c *gin.Context) {
	handle(c, h.listDeploymentRequests)
}

// GetDeploymentRequest gets details
func (h *Handler) GetDeploymentRequest(c *gin.Context) {
	handle(c, h.getDeploymentRequest)
}

// ApproveDeploymentRequest handles approval
func (h *Handler) ApproveDeploymentRequest(c *gin.Context) {
	handle(c, h.approveDeploymentRequest)
}

// RollbackDeployment handles rollback
func (h *Handler) RollbackDeployment(c *gin.Context) {
	handle(c, h.rollbackDeployment)
}

// GetCurrentEnvConfig gets the current .env file configuration
func (h *Handler) GetCurrentEnvConfig(c *gin.Context) {
	handle(c, h.getCurrentEnvConfig)
}

func (h *Handler) createDeploymentRequest(c *gin.Context) (interface{}, error) {
	var req CreateDeploymentRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	// Validate image versions
	if len(req.ImageVersions) == 0 {
		return nil, commonerrors.NewBadRequest("image_versions cannot be empty")
	}

	// Wrap into DeploymentConfig
	config := DeploymentConfig{
		ImageVersions: req.ImageVersions,
		EnvFileConfig: req.EnvFileConfig,
	}

	// Marshal to JSON for storage
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, commonerrors.NewBadRequest("Failed to marshal config")
	}

	username := c.GetString(common.UserName)

	dbReq := &dbclient.DeploymentRequest{
		DeployName:  username,
		Status:      StatusPendingApproval,
		EnvConfig:   string(configJSON),
		Description: dbutils.NullString(req.Description),
	}

	id, err := h.dbClient.CreateDeploymentRequest(c.Request.Context(), dbReq)
	if err != nil {
		return nil, err
	}

	return CreateDeploymentRequestResp{Id: id}, nil
}

func (h *Handler) listDeploymentRequests(c *gin.Context) (interface{}, error) {
	limit := 10 // Default
	offset := 0

	// Basic query param parsing (could be improved)
	if l := c.Query("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil {
			limit = val
		}
	}
	if o := c.Query("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil {
			offset = val
		}
	}

	query := sqrl.Eq{}
	// Filter by status
	if status := c.Query("status"); status != "" {
		query = sqrl.Eq{"status": status}
	}

	orderBy := []string{"created_at DESC"}

	list, err := h.dbClient.ListDeploymentRequests(c.Request.Context(), query, orderBy, limit, offset)
	if err != nil {
		return nil, err
	}

	total, err := h.dbClient.CountDeploymentRequests(c.Request.Context(), query)
	if err != nil {
		return nil, err
	}

	resp := ListDeploymentRequestsResp{
		TotalCount: total,
		Items:      make([]*DeploymentRequestItem, 0),
	}

	for _, item := range list {
		resp.Items = append(resp.Items, h.service.cvtDBRequestToItem(item))
	}

	return resp, nil
}

func (h *Handler) getDeploymentRequest(c *gin.Context) (interface{}, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, commonerrors.NewBadRequest("Invalid ID")
	}

	req, err := h.dbClient.GetDeploymentRequest(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}

	// Parse stored config
	var config DeploymentConfig
	if err := json.Unmarshal([]byte(req.EnvConfig), &config); err != nil {
		// Fallback for old format or invalid data
		config = DeploymentConfig{
			ImageVersions: make(map[string]string),
			EnvFileConfig: "",
		}
	}

	resp := GetDeploymentRequestResp{
		DeploymentRequestItem: *h.service.cvtDBRequestToItem(req),
		ImageVersions:         config.ImageVersions,
		EnvFileConfig:         config.EnvFileConfig,
	}

	return resp, nil
}

func (h *Handler) approveDeploymentRequest(c *gin.Context) (interface{}, error) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, commonerrors.NewBadRequest("Invalid ID")
	}

	var body ApprovalReq
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	req, err := h.dbClient.GetDeploymentRequest(c.Request.Context(), id)
	if err != nil {
		return nil, err
	}

	username := c.GetString(common.UserName)
	// Check if user is the same as requester
	if req.DeployName == username {
		return nil, commonerrors.NewForbidden("Cannot approve your own request")
	}

	if req.Status != StatusPendingApproval {
		return nil, commonerrors.NewBadRequest("Request is not pending approval")
	}

	req.ApproverName = dbutils.NullString(username)
	req.ApprovedAt = dbutils.NullTime(time.Now().UTC())

	if body.Approved {
		req.Status = StatusApproved
		req.ApprovalResult = dbutils.NullString(StatusApproved)
		// Update status first
		if err := h.dbClient.UpdateDeploymentRequest(c.Request.Context(), req); err != nil {
			return nil, err
		}

		// Execute Deployment (Async)
		go func() {
			ctx := context.Background()
			jobName, err := h.service.ExecuteDeployment(ctx, req)
			if err != nil {
				klog.ErrorS(err, "Deployment failed", "id", req.Id)
				h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, "Deployment execution failed")
				return
			}

			// Wait for job completion
			if err := h.service.WaitForJobCompletion(ctx, jobName, JobNamespace); err != nil {
				klog.ErrorS(err, "Job execution failed", "job", jobName)
				h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, err.Error())
				return
			}

			// Verify Deployment Rollout (Check if pods are actually running)
			if err := h.service.VerifyDeploymentRollout(ctx, req.EnvConfig); err != nil {
				klog.ErrorS(err, "Deployment verification failed", "id", req.Id)
				h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, fmt.Sprintf("Job succeeded but rollout failed: %v", err))
				return
			}

			// Success: Update status and create snapshot
			h.service.UpdateRequestStatus(ctx, req.Id, StatusDeployed, "")
			h.service.CreateSnapshot(ctx, req.Id, req.EnvConfig)
		}()

	} else {
		req.Status = StatusRejected
		req.ApprovalResult = dbutils.NullString(StatusRejected)
		// Append rejection reason if description exists
		desc := ""
		if req.Description.Valid {
			desc = req.Description.String + ". "
		}
		desc += fmt.Sprintf("Rejection reason: %s", body.Reason)
		req.Description = dbutils.NullString(desc)

		if err := h.dbClient.UpdateDeploymentRequest(c.Request.Context(), req); err != nil {
			return nil, err
		}
	}

	return nil, nil
}

func (h *Handler) rollbackDeployment(c *gin.Context) (interface{}, error) {
	idStr := c.Param("id") // The request ID to rollback TO
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, commonerrors.NewBadRequest("Invalid ID")
	}

	username := c.GetString(common.UserName)
	newId, err := h.service.Rollback(c.Request.Context(), id, username)
	if err != nil {
		return nil, err
	}

	return CreateDeploymentRequestResp{Id: newId}, nil
}

func (h *Handler) getCurrentEnvConfig(c *gin.Context) (interface{}, error) {
	content, err := h.service.GetCurrentEnvConfig(c.Request.Context())
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("Failed to get env config: %v", err))
	}

	return GetCurrentEnvConfigResp{
		EnvFileConfig: content,
	}, nil
}
