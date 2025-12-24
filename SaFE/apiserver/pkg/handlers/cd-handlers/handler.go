/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	dbClient         dbclient.Interface
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

	return h, nil
}

// getAndSetUsername retrieves the user information based on the user ID stored in the context
// and sets the username in the context for further use.
// Returns the username string and any error encountered during the process.
func (h *Handler) getAndSetUsername(c *gin.Context) (string, error) {
	userId := c.GetString(common.UserId)
	if userId == "" {
		return "", nil
	}
	user, err := h.accessController.GetRequestUser(c.Request.Context(), userId)
	if err != nil {
		return "", err
	}

	userName := v1.GetUserName(user)
	c.Set(common.UserName, userName)
	return userName, nil
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

	// Normalize image versions: auto-complete image name if only version tag is provided
	normalizedVersions := make(map[string]string, len(req.ImageVersions))
	for component, version := range req.ImageVersions {
		normalizedVersions[component] = NormalizeImageVersion(component, version)
	}

	// Wrap into DeploymentConfig
	config := DeploymentConfig{
		ImageVersions: normalizedVersions,
		EnvFileConfig: req.EnvFileConfig,
	}

	// Marshal to JSON for storage
	configJSON, err := json.Marshal(config)
	if err != nil {
		return nil, commonerrors.NewBadRequest("Failed to marshal config")
	}

	username, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

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

	username, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	// Check if user is the same as requester
	// Self-approval is controlled by cd.require_approval config
	if req.DeployName == username && commonconfig.IsCDRequireApproval() {
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

		// Create OpsJob for CD deployment (managed by resource-manager)
		ctx := c.Request.Context()
		opsJob, err := h.generateCDOpsJob(ctx, req, username)
		if err != nil {
			klog.ErrorS(err, "Failed to generate CD OpsJob", "id", req.Id)
			h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, fmt.Sprintf("Failed to generate OpsJob: %v", err))
			return nil, err
		}

		if err := h.Create(ctx, opsJob); err != nil {
			klog.ErrorS(err, "Failed to create CD OpsJob", "id", req.Id)
			h.service.UpdateRequestStatus(ctx, req.Id, StatusFailed, fmt.Sprintf("Failed to create OpsJob: %v", err))
			return nil, err
		}

		// Update status to deploying
		req.Status = StatusDeploying
		if err := h.dbClient.UpdateDeploymentRequest(ctx, req); err != nil {
			klog.ErrorS(err, "Failed to update deployment request status", "id", req.Id)
		}

		klog.Infof("CD OpsJob created for deployment request %d: %s", req.Id, opsJob.Name)

		return ApprovalResp{
			Id:      req.Id,
			Status:  StatusApproved,
			JobId:   opsJob.Name,
			Message: "Deployment approved, OpsJob created and managed by resource-manager",
		}, nil

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
}

// generateCDOpsJob generates an OpsJob for CD deployment.
// Uses 'default' workspace with immediate scheduling (similar to preflight jobs).
func (h *Handler) generateCDOpsJob(ctx context.Context, req *dbclient.DeploymentRequest, username string) (*v1.OpsJob, error) {
	// Parse deployment config
	var requestConfig DeploymentConfig
	if err := json.Unmarshal([]byte(req.EnvConfig), &requestConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	// Merge with latest snapshot for deployment
	mergedConfig, err := h.service.mergeWithLatestSnapshot(ctx, requestConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to merge with latest snapshot: %v", err)
	}

	// Get deployable components
	expectedComponents := commonconfig.GetComponents()

	// Build deployment parameters
	componentTags := ""
	nodeAgentTags := ""
	hasNodeAgent := false
	hasCICD := false
	nodeAgentImage := ""
	cicdRunnerImage := ""
	cicdUnifiedImage := ""

	deployBranch := extractBranchFromEnvFileConfig(mergedConfig.EnvFileConfig)

	for _, comp := range expectedComponents {
		if tag, ok := mergedConfig.ImageVersions[comp]; ok {
			if yamlKey, isCICD := CICDComponentsMap[comp]; isCICD {
				componentTags += fmt.Sprintf("%s=%s;", yamlKey, tag)
				hasCICD = true
				if comp == ComponentCICDRunner {
					cicdRunnerImage = tag
				} else if comp == ComponentCICDUnifiedJob {
					cicdUnifiedImage = tag
				}
			} else if comp == ComponentNodeAgent {
				// Update node-agent if user explicitly requested it
				if _, userRequested := requestConfig.ImageVersions[comp]; userRequested {
					nodeAgentTags += fmt.Sprintf("%s=%s;", YAMLKeyNodeAgentImage, tag)
					hasNodeAgent = true
					nodeAgentImage = tag
				}
			} else {
				componentTags += fmt.Sprintf("%s.image=%s;", comp, tag)
			}
		}
	}

	// Generate OpsJob name
	jobName := commonutils.GenerateName(fmt.Sprintf("cd-%d", req.Id))

	// Build OpsJob inputs
	inputs := []v1.Parameter{
		{Name: v1.ParameterDeploymentRequestId, Value: fmt.Sprintf("%d", req.Id)},
		{Name: v1.ParameterDeployPhase, Value: "local"}, // Start with local deployment
		{Name: v1.ParameterComponentTags, Value: componentTags},
		{Name: v1.ParameterNodeAgentTags, Value: nodeAgentTags},
		{Name: v1.ParameterEnvFileConfig, Value: mergedConfig.EnvFileConfig},
		{Name: v1.ParameterDeployBranch, Value: deployBranch},
		{Name: v1.ParameterHasNodeAgent, Value: fmt.Sprintf("%t", hasNodeAgent)},
		{Name: v1.ParameterHasCICD, Value: fmt.Sprintf("%t", hasCICD)},
		{Name: v1.ParameterNodeAgentImage, Value: nodeAgentImage},
		{Name: v1.ParameterCICDRunnerImage, Value: cicdRunnerImage},
		{Name: v1.ParameterCICDUnifiedImage, Value: cicdUnifiedImage},
	}

	opsJob := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				// No ClusterIdLabel - CD jobs use 'default' workspace with immediate scheduling
				v1.UserIdLabel:     username,
				v1.OpsJobTypeLabel: string(v1.OpsJobCDType),
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: username,
			},
		},
		Spec: v1.OpsJobSpec{
			Type:                    v1.OpsJobCDType,
			Inputs:                  inputs,
			TimeoutSecond:           1800, // 30 minutes timeout
			TTLSecondsAfterFinished: 3600, // 1 hour TTL after completion
			IsTolerateAll:           true, // Can run on any node
			Hostpath:                []string{HostMountPath},
		},
	}

	return opsJob, nil
}

func (h *Handler) rollbackDeployment(c *gin.Context) (interface{}, error) {
	idStr := c.Param("id") // The request ID to rollback TO
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, commonerrors.NewBadRequest("Invalid ID")
	}

	username, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
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
