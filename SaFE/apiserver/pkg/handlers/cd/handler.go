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

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/channel"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/notification/model"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/stringutil"
	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
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
	emailChannel     *channel.EmailChannel
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

	// Initialize email channel if notification is enabled
	if commonconfig.IsNotificationEnable() {
		conf, err := channel.ReadConfigFromFile(commonconfig.GetNotificationConfig())
		if err != nil {
			klog.Warningf("Failed to read notification config: %v", err)
		} else if conf.Email != nil {
			emailCh := &channel.EmailChannel{}
			if err := emailCh.Init(*conf); err != nil {
				klog.Warningf("Failed to initialize email channel: %v", err)
			} else {
				h.emailChannel = emailCh
				klog.Info("Email channel initialized for CD handler")
			}
		}
	}

	// Set deployment failure callback for email notification
	h.service.SetDeploymentFailureCallback(func(ctx context.Context, req *dbclient.DeploymentRequest, reason string) {
		h.sendDeploymentFailureEmail(ctx, req, reason)
	})

	// Recover any stuck deploying requests after apiserver restart
	go func() {
		// Wait a bit for all services to be ready
		time.Sleep(5 * time.Second)
		ctx := context.Background()
		if err := h.service.RecoverDeployingRequests(ctx); err != nil {
			klog.ErrorS(err, "Failed to recover deploying requests")
		}
	}()

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

// GetDeployableComponents returns the list of deployable components
func (h *Handler) GetDeployableComponents(c *gin.Context) {
	handle(c, h.getDeployableComponents)
}

func (h *Handler) getDeployableComponents(c *gin.Context) (interface{}, error) {
	// Read components from config (sourced from values.yaml via ConfigMap)
	components := commonconfig.GetComponents()
	return GetDeployableComponentsResp{Components: components}, nil
}

func (h *Handler) createDeploymentRequest(c *gin.Context) (interface{}, error) {
	var req CreateDeploymentRequestReq
	if err := c.ShouldBindJSON(&req); err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	// Validate: at least one of image_versions or env_file_config must be provided
	if len(req.ImageVersions) == 0 && req.EnvFileConfig == "" {
		return nil, commonerrors.NewBadRequest("at least one of image_versions or env_file_config must be provided")
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

// getUserEmail retrieves the email address for a given username.
// Returns empty string if user not found or email not set.
func (h *Handler) getUserEmail(ctx context.Context, username string) string {
	// Build label selector to find user by username (MD5 hash)
	req, err := labels.NewRequirement(v1.UserNameMd5Label, selection.Equals, []string{stringutil.MD5(username)})
	if err != nil {
		klog.ErrorS(err, "Failed to create label requirement", "username", username)
		return ""
	}
	selector := labels.NewSelector().Add(*req)

	userList := &v1.UserList{}
	if err := h.List(ctx, userList, &client.ListOptions{LabelSelector: selector}); err != nil {
		klog.ErrorS(err, "Failed to get user for email lookup", "username", username)
		return ""
	}

	if len(userList.Items) == 0 {
		klog.Warningf("User not found: %s", username)
		return ""
	}

	return v1.GetUserEmail(&userList.Items[0])
}

// sendDeploymentFailureEmail sends an email notification when deployment fails.
// It looks up the deployer's email and sends a failure notification.
func (h *Handler) sendDeploymentFailureEmail(ctx context.Context, req *dbclient.DeploymentRequest, failReason string) {
	if h.emailChannel == nil {
		klog.Warning("Email channel not initialized, skipping failure notification")
		return
	}

	// Get deployer's email
	email := h.getUserEmail(ctx, req.DeployName)
	if email == "" {
		klog.Warningf("No email found for user %s, skipping failure notification", req.DeployName)
		return
	}

	// Build email content
	message := &model.Message{
		Email: &model.EmailMessage{
			To:    []string{email},
			Title: fmt.Sprintf("[CD Deployment Failed] Request #%d Failed", req.Id),
			Content: fmt.Sprintf(`
				<h2>Deployment Failure Notification</h2>
				<table style="border-collapse: collapse; width: 100%%;">
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Request ID</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd;">%d</td>
					</tr>
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Deployer</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
					</tr>
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Approver</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
					</tr>
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Failure Reason</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd; color: #c53030;">%s</td>
					</tr>
					<tr>
						<td style="padding: 8px; border: 1px solid #ddd;"><strong>Time</strong></td>
						<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
					</tr>
				</table>
				<p style="margin-top: 16px; color: #666;">Please check the deployment logs for more details.</p>
			`, req.Id, req.DeployName, req.ApproverName.String, failReason, time.Now().Format(time.DateTime)),
		},
	}

	if err := h.emailChannel.Send(ctx, message); err != nil {
		klog.ErrorS(err, "Failed to send deployment failure email", "id", req.Id, "email", email)
	} else {
		klog.Infof("Deployment failure email sent for request %d to %s", req.Id, email)
	}
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
			result, err := h.service.ExecuteDeployment(ctx, req)
			if err != nil {
				klog.ErrorS(err, "Deployment failed", "id", req.Id)
				h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, "Deployment execution failed")
				h.sendDeploymentFailureEmail(ctx, req, fmt.Sprintf("Deployment execution failed: %v", err))
				return
			}

			// Wait for local job completion
			if err := h.service.WaitForJobCompletion(ctx, result.LocalJobName, JobNamespace); err != nil {
				klog.ErrorS(err, "Job execution failed", "job", result.LocalJobName)
				h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, err.Error())
				h.sendDeploymentFailureEmail(ctx, req, fmt.Sprintf("Job execution failed: %v", err))
				return
			}

			// If there are remote cluster updates (node_agent or cicd), execute them
			if result.HasNodeAgent || result.HasCICD {
				klog.Infof("Remote cluster updates needed: node_agent=%v, cicd=%v", result.HasNodeAgent, result.HasCICD)
				remoteJobName, err := h.service.ExecuteRemoteClusterUpdates(ctx, req.Id, result)
				if err != nil {
					klog.ErrorS(err, "Remote cluster update failed", "id", req.Id)
					h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, fmt.Sprintf("Remote cluster update failed: %v", err))
					h.sendDeploymentFailureEmail(ctx, req, fmt.Sprintf("Remote cluster update failed: %v", err))
					return
				}

				// Wait for remote job completion
				if err := h.service.WaitForJobCompletion(ctx, remoteJobName, JobNamespace); err != nil {
					klog.ErrorS(err, "Remote cluster job failed", "job", remoteJobName)
					h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, fmt.Sprintf("Remote cluster job failed: %v", err))
					h.sendDeploymentFailureEmail(ctx, req, fmt.Sprintf("Remote cluster job failed: %v", err))
					return
				}
			}

			// Verify Deployment Rollout (Check if pods are actually running)
			if err := h.service.VerifyDeploymentRollout(ctx, req.EnvConfig); err != nil {
				klog.ErrorS(err, "Deployment verification failed", "id", req.Id)
				h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, fmt.Sprintf("Job succeeded but rollout failed: %v", err))
				h.sendDeploymentFailureEmail(ctx, req, fmt.Sprintf("Rollout verification failed: %v", err))
				return
			}

			// Success: Update status and create snapshot
			h.service.UpdateRequestStatus(ctx, req.Id, StatusDeployed, "")
			if err := h.service.CreateSnapshot(ctx, req.Id, req.EnvConfig); err != nil {
				klog.ErrorS(err, "Failed to create snapshot", "id", req.Id)
				// Don't fail the deployment, snapshot is for historical record
			} else {
				klog.Infof("Snapshot created for request %d", req.Id)
			}
		}()

	} else {
		req.Status = StatusRejected
		req.ApprovalResult = dbutils.NullString(StatusRejected)
		req.RejectionReason = dbutils.NullString(body.Reason)

		if err := h.dbClient.UpdateDeploymentRequest(c.Request.Context(), req); err != nil {
			return nil, err
		}

		return ApprovalResp{
			Id:      req.Id,
			Status:  StatusRejected,
			Message: "Deployment request rejected",
		}, nil
	}

	return ApprovalResp{
		Id:      req.Id,
		Status:  StatusApproved,
		Message: "Deployment approved and started, running in background",
	}, nil
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
