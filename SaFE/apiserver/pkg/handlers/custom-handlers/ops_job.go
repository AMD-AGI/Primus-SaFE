/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// CreateOpsJob handles the creation of a new ops job resource.
// It parses the request, generates the appropriate job type, and creates it in the system.
// Returns the created job ID on success.
func (h *Handler) CreateOpsJob(c *gin.Context) {
	handle(c, h.createOpsJob)
}

// ListOpsJob handles listing ops job resources with filtering and pagination.
// It queries the database for ops jobs based on provided query parameters.
// Returns a list of ops jobs matching the criteria.
func (h *Handler) ListOpsJob(c *gin.Context) {
	handle(c, h.listOpsJob)
}

// GetOpsJob retrieves detailed information about a specific ops job.
// It queries the database for the specified job and returns its complete information.
// Returns an error if the job is not found.
func (h *Handler) GetOpsJob(c *gin.Context) {
	handle(c, h.getOpsJob)
}

// DeleteOpsJob handles deletion of an ops job resource.
// It removes the job from both the k8s cluster and database (if enabled),
// and cleans up any related job resource.
func (h *Handler) DeleteOpsJob(c *gin.Context) {
	handle(c, h.deleteOpsJob)
}

// StopOpsJob handles stopping an ops job resource.
// It removes the job from the k8s cluster and cleans up related job information.
func (h *Handler) StopOpsJob(c *gin.Context) {
	handle(c, h.stopOpsJob)
}

// createOpsJob implements the ops job creation logic.
// It determines the job type, generates the appropriate job object,
// and persists it in the system.
func (h *Handler) createOpsJob(c *gin.Context) (interface{}, error) {
	req, body, err := parseCreateOpsJobRequest(c)
	if err != nil {
		return nil, err
	}

	ctx := c.Request.Context()
	var job *v1.OpsJob
	switch req.Type {
	case v1.OpsJobAddonType:
		job, err = h.generateAddonJob(c, body)
	case v1.OpsJobPreflightType:
		job, err = h.generatePreflightJob(c, body)
	case v1.OpsJobDumpLogType:
		job, err = h.generateDumpLogJob(c, body)
	case v1.OpsJobRebootType:
		job, err = h.generateRebootJob(c, body)
	case v1.OpsJobExportImageType:
		job, err = h.generateExportImageJob(c, body)
	case v1.OpsJobPrewarmType:
		job, err = h.generatePrewarmImageJob(c, body)
	case v1.OpsJobDownloadType:
		job, err = h.generateDownloadJob(c, body)

	default:
		err = fmt.Errorf("unsupported ops job type(%s)", req.Type)
	}
	if err != nil || job == nil {
		return nil, err
	}

	if err = h.Create(ctx, job); err != nil {
		klog.ErrorS(err, "failed to create ops job")
		return nil, err
	}
	klog.Infof("create ops job: %s, type: %s, params: %v, user: %s",
		job.Name, job.Spec.Type, job.Spec.Inputs, c.GetString(common.UserName))
	return &types.CreateOpsJobResponse{JobId: job.Name}, nil
}

// listOpsJob implements the ops job listing logic.
// It checks if database functionality is enabled, parses query parameters,
// executes database queries, and formats the response.
func (h *Handler) listOpsJob(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	query, err := h.parseListOpsJobQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}

	dbSql, orderBy := cvtToListOpsJobSql(query)
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	count, err := h.dbClient.CountJobs(c.Request.Context(), dbSql)
	if err != nil {
		return nil, err
	}
	result := &types.ListOpsJobResponse{
		TotalCount: count,
	}
	for _, job := range jobs {
		result.Items = append(result.Items, cvtToOpsJobResponseItem(job))
	}
	return result, nil
}

// getOpsJob implements the logic for retrieving a single ops job's detailed information
// Returns an error if the job is not found or database is not enabled.
func (h *Handler) getOpsJob(c *gin.Context) (interface{}, error) {
	opsJob, err := h.getOpsJobFromDB(c)
	if err != nil {
		return nil, err
	}
	return cvtToGetOpsJobResponse(opsJob), nil
}

// getOpsJobFromDB queries the database for the specified job
// Returns an error if the job is not found or database is not enabled.
func (h *Handler) getOpsJobFromDB(c *gin.Context) (*dbclient.OpsJob, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	dbSql, err := h.cvtToGetOpsJobSql(c)
	if err != nil {
		return nil, err
	}
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, []string{}, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, commonerrors.NewNotFoundWithMessage("the opsjob is not found")
	}
	return jobs[0], nil
}

// deleteOpsJob implements ops job deletion logic.
// It removes the job from the k8s cluster, marks it as deleted in the database (if enabled),
// and cleans up any related job information.
func (h *Handler) deleteOpsJob(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	name := c.GetString(common.Name)
	isFound, err := h.deleteAdminOpsJob(c, name)
	if err != nil {
		return nil, err
	}
	if commonconfig.IsDBEnable() {
		opsJob, err := h.getOpsJobFromDB(c)
		if err != nil {
			if !commonerrors.IsNotFound(err) {
				return nil, err
			}
		} else {
			if err = h.accessController.Authorize(authority.AccessInput{
				Context:       c.Request.Context(),
				ResourceKind:  v1.OpsJobKind,
				ResourceOwner: dbutils.ParseNullString(opsJob.UserId),
				Verb:          v1.DeleteVerb,
				User:          requestUser,
			}); err != nil {
				return nil, err
			}
			if err = h.dbClient.SetOpsJobDeleted(c.Request.Context(), name); err != nil {
				return nil, err
			}
			isFound = true
		}
	}
	if !isFound {
		return nil, commonerrors.NewNotFoundWithMessage("the opsjob is not found")
	}

	if err = commonjob.CleanupJobRelatedResource(c.Request.Context(), h.Client, name); err != nil {
		klog.ErrorS(err, "failed to cleanup ops job labels")
	}
	klog.Infof("delete opsJob %s", name)
	return nil, nil
}

// stopOpsJob implements ops job stopping logic.
// It removes the job from the k8s cluster and cleans up related job resource.
func (h *Handler) stopOpsJob(c *gin.Context) (interface{}, error) {
	name := c.GetString(common.Name)
	isFound, err := h.deleteAdminOpsJob(c, name)
	if err != nil {
		return nil, err
	}
	if !isFound {
		return nil, commonerrors.NewNotFoundWithMessage("the opsjob is not found")
	}
	if err = commonjob.CleanupJobRelatedResource(c.Request.Context(), h.Client, name); err != nil {
		klog.ErrorS(err, "failed to cleanup ops job labels")
	}
	klog.Infof("stop opsJob %s", name)
	return nil, nil
}

// deleteAdminOpsJob removes an ops job resource from the k8s cluster.
// It performs authorization checks, retrieves the job, and deletes it from the cluster.
// Returns true if the job was found and deleted, false if not found.
func (h *Handler) deleteAdminOpsJob(c *gin.Context, opsJobId string) (bool, error) {
	if opsJobId == "" {
		return false, commonerrors.NewBadRequest("the opsJobId is empty")
	}
	ctx := c.Request.Context()
	opsJob := &v1.OpsJob{}
	err := h.Get(ctx, client.ObjectKey{Name: opsJobId}, opsJob)
	if err != nil {
		return false, client.IgnoreNotFound(err)
	}

	if err = h.accessController.Authorize(authority.AccessInput{
		Context:  ctx,
		Resource: opsJob,
		Verb:     v1.DeleteVerb,
		UserId:   c.GetString(common.UserId),
	}); err != nil {
		return false, err
	}
	if err = h.Delete(ctx, opsJob); err != nil {
		return false, err
	}
	return true, nil
}

// generateAddonJob creates an addon-type ops job.
// It authorizes the request, parses addon-specific parameters,
// and generates a job object with appropriate annotations.
func (h *Handler) generateAddonJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.AddOnTemplateKind,
		Verb:         v1.CreateVerb,
		User:         requestUser,
	}); err != nil {
		return nil, err
	}

	req := &types.CreateAddonRequest{}
	if err = jsonutils.Unmarshal(body, req); err != nil {
		return nil, err
	}
	if req.BatchCount <= 0 {
		req.BatchCount = 1
	}
	if req.AvailableRatio == nil || *req.AvailableRatio <= 0 {
		req.AvailableRatio = pointer.Float64(1.0)
	}
	job := genDefaultOpsJob(&req.BaseOpsJobRequest, requestUser)
	job.Spec.ExcludedNodes = req.ExcludedNodes
	if req.SecurityUpgrade {
		v1.SetAnnotation(job, v1.OpsJobSecurityUpgradeAnnotation, "")
	}
	v1.SetAnnotation(job, v1.OpsJobBatchCountAnnotation, strconv.Itoa(req.BatchCount))
	v1.SetAnnotation(job, v1.OpsJobAvailRatioAnnotation,
		strconv.FormatFloat(*req.AvailableRatio, 'f', -1, 64))

	if err = h.generateOpsJobNodesInput(c.Request.Context(), job); err != nil {
		return nil, err
	}
	return job, nil
}

// generatePreflightJob creates a preflight-type ops job.
// It authorizes the request for system admin,
// and generates a job object with resource, image, and entrypoint specifications.
func (h *Handler) generatePreflightJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	req := &types.CreatePreflightRequest{}
	if err := jsonutils.Unmarshal(body, req); err != nil {
		return nil, err
	}

	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	job := genDefaultOpsJob(&req.BaseOpsJobRequest, requestUser)
	job.Spec.Resource = req.Resource
	job.Spec.Image = req.Image
	job.Spec.EntryPoint = req.EntryPoint
	job.Spec.Env = req.Env
	job.Spec.IsTolerateAll = req.IsTolerateAll
	job.Spec.ExcludedNodes = req.ExcludedNodes
	job.Spec.Hostpath = req.Hostpath
	if req.WorkspaceId != "" {
		v1.SetLabel(job, v1.WorkspaceIdLabel, req.WorkspaceId)
	}
	if err = h.generateOpsJobNodesInput(c.Request.Context(), job); err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: authority.PreflightKind,
		Verb:         v1.CreateVerb,
		Workspaces:   []string{v1.GetWorkspaceId(job)},
		User:         requestUser,
	}); err != nil {
		return nil, err
	}
	return job, nil
}

// generateDumpLogJob creates a dump log-type ops job.
// It checks if logging and S3 functionality are enabled, authorizes the request,
// validates workload access, and generates a job object with appropriate labels.
func (h *Handler) generateDumpLogJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	if !commonconfig.IsOpenSearchEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	if !commonconfig.IsS3Enable() {
		return nil, commonerrors.NewInternalError("The s3 function is not enabled")
	}

	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	req := &types.CreateDumplogRequest{}
	if err = jsonutils.Unmarshal(body, req); err != nil {
		return nil, err
	}
	job := genDefaultOpsJob(&req.BaseOpsJobRequest, requestUser)

	workloadParam := job.GetParameter(v1.ParameterWorkload)
	if workloadParam == nil {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("%s must be specified in the job.", v1.ParameterWorkload))
	}

	workload, err := h.getWorkloadForAuth(c.Request.Context(), workloadParam.Value)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    c.Request.Context(),
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		User:       requestUser,
	}); err != nil {
		return nil, err
	}
	v1.SetLabel(job, v1.WorkspaceIdLabel, workload.Spec.Workspace)
	return job, nil
}

// generateRebootJob create a reboot-type ops job.
func (h *Handler) generateRebootJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: v1.NodeKind,
		Verb:         v1.UpdateVerb,
		User:         requestUser,
	}); err != nil {
		return nil, err
	}

	req := &types.BaseOpsJobRequest{}
	if err = jsonutils.Unmarshal(body, req); err != nil {
		return nil, err
	}

	return genDefaultOpsJob(req, requestUser), nil
}

// generateExportImageJob creates an export-image-type ops job.
// It parses the workload ID from request body, retrieves workload information,
// and generates a job object to export the workload image to Harbor.
func (h *Handler) generateExportImageJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	// Parse request to get workload ID
	req := &types.BaseOpsJobRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		return nil, commonerrors.NewBadRequest("failed to parse request body: " + err.Error())
	}

	// Extract workload ID from inputs
	var workloadId string
	for _, param := range req.Inputs {
		if param.Name == v1.ParameterWorkload || param.Name == "workloadId" {
			workloadId = param.Value
			break
		}
	}
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workload ID is required in inputs")
	}

	// Get workload information for authorization
	ctx := c.Request.Context()
	workload, err := h.getWorkloadForAuth(ctx, workloadId)
	if err != nil {
		return nil, err
	}

	// Check authorization
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workload,
		Verb:       v1.GetVerb,
		Workspaces: []string{workload.Spec.Workspace},
		User:       requestUser,
	}); err != nil {
		return nil, err
	}

	// Get full workload to access image field
	adminWorkload, err := h.getAdminWorkload(ctx, workloadId)
	if err != nil {
		return nil, err
	}

	// Validate workload has image
	if adminWorkload.Spec.Image == "" {
		return nil, commonerrors.NewBadRequest("workload image is empty")
	}

	// Build BaseOpsJobRequest for genDefaultOpsJob
	jobName := fmt.Sprintf("custom-%s", workloadId)

	// Preserve user's original inputs (including label if provided)
	newInputs := make([]v1.Parameter, 0, len(req.Inputs)+1)
	newInputs = append(newInputs, req.Inputs...) // Keep original inputs (workload, label, etc.)

	// Add image parameter (system-generated)
	newInputs = append(newInputs, v1.Parameter{Name: "image", Value: adminWorkload.Spec.Image})

	jobReq := &types.BaseOpsJobRequest{
		Name:                    jobName,
		Type:                    v1.OpsJobExportImageType,
		Inputs:                  newInputs, // Use merged inputs
		TimeoutSecond:           commonconfig.GetOpsJobTimeoutSecond(),
		TTLSecondsAfterFinished: commonconfig.GetOpsJobTTLSecond(),
	}

	// Generate base OpsJob using genDefaultOpsJob
	job := genDefaultOpsJob(jobReq, requestUser)

	// Add export-image specific labels
	job.Labels[v1.WorkloadIdLabel] = workloadId
	job.Labels[v1.WorkspaceIdLabel] = adminWorkload.Spec.Workspace

	return job, nil
}

// generatePrewarmImageJob creates a prewarm-type ops job.
// It parses the workload ID from request body, retrieves workload information,
// and generates a job object to prewarm the workload image on cluster nodes.
func (h *Handler) generatePrewarmImageJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	// Parse request to get workload ID
	req := &types.BaseOpsJobRequest{}
	if err := json.Unmarshal(body, req); err != nil {
		return nil, commonerrors.NewBadRequest("failed to parse request body: " + err.Error())
	}

	// Extract image and workspace from inputs
	var image, workspace string
	for _, param := range req.Inputs {
		if param.Name == v1.ParameterImage {
			image = param.Value
		}
		if param.Name == v1.ParameterWorkspace {
			workspace = param.Value
		}
	}
	if image == "" {
		return nil, commonerrors.NewBadRequest("image is required in inputs")
	}
	if workspace == "" {
		return nil, commonerrors.NewBadRequest("workspace is required in inputs")
	}

	// Get workspace information to retrieve cluster id
	ctx := c.Request.Context()
	workspaceObj, err := h.getAdminWorkspace(ctx, workspace)
	if err != nil {
		return nil, commonerrors.NewBadRequest("failed to get workspace: " + err.Error())
	}

	// Check authorization
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:    ctx,
		Resource:   workspaceObj,
		Verb:       v1.GetVerb,
		User:       requestUser,
		Workspaces: []string{workspace},
	}); err != nil {
		return nil, err
	}

	jobName := fmt.Sprintf("prewarm-%s", time.Now().Format("200601021504"))

	// Build BaseOpsJobRequest for prewarm job
	jobReq := &types.BaseOpsJobRequest{
		Name:                    jobName,
		Type:                    v1.OpsJobPrewarmType,
		Inputs:                  req.Inputs,
		TimeoutSecond:           commonconfig.GetOpsJobTimeoutSecond(),
		TTLSecondsAfterFinished: commonconfig.GetOpsJobTTLSecond(),
	}

	// Generate base OpsJob using genDefaultOpsJob
	job := genDefaultOpsJob(jobReq, requestUser)

	// Add workspace and cluster labels for statistics and tracking
	job.Labels[v1.WorkspaceIdLabel] = workspace
	job.Labels[v1.ClusterIdLabel] = workspaceObj.Spec.Cluster

	return job, nil
}

// generateDownloadJob creates a download file-type ops job.
// it validates workspace access, and generates a job object with appropriate labels.
func (h *Handler) generateDownloadJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	req := &types.CreateDownloadRequest{}
	if err = jsonutils.Unmarshal(body, req); err != nil {
		return nil, err
	}
	job := genDefaultOpsJob(&req.BaseOpsJobRequest, requestUser)

	secretParam := job.GetParameter(v1.ParameterSecret)
	if secretParam == nil {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("%s must be specified in the job.", v1.ParameterSecret))
	}
	workspaceParam := job.GetParameter(v1.ParameterWorkspace)
	if workspaceParam == nil {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("%s must be specified in the job.", v1.ParameterWorkspace))
	}

	_, err = h.getAndAuthorizeSecret(c.Request.Context(), secretParam.Value, workspaceParam.Value, requestUser, v1.GetVerb)
	if err != nil {
		return nil, err
	}
	if err = h.accessController.Authorize(authority.AccessInput{
		Context:      c.Request.Context(),
		ResourceKind: authority.DownloadKind,
		Verb:         v1.CreateVerb,
		Workspaces:   []string{workspaceParam.Value},
		User:         requestUser,
	}); err != nil {
		return nil, err
	}

	v1.SetLabel(job, v1.WorkspaceIdLabel, workspaceParam.Value)
	return job, nil
}

// genDefaultOpsJob creates a default ops job object with common properties.
// It sets up the job metadata including name, labels, annotations, and basic specifications.
func genDefaultOpsJob(req *types.BaseOpsJobRequest, requestUser *v1.User) *v1.OpsJob {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(req.Name),
			Labels: map[string]string{
				v1.UserIdLabel:      requestUser.Name,
				v1.DisplayNameLabel: req.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: v1.GetUserName(requestUser),
			},
		},
		Spec: v1.OpsJobSpec{
			Type:                    req.Type,
			Inputs:                  req.Inputs,
			TimeoutSecond:           req.TimeoutSecond,
			TTLSecondsAfterFinished: req.TTLSecondsAfterFinished,
		},
	}
	if v1.GetUserName(job) == "" {
		v1.SetAnnotation(job, v1.UserNameAnnotation, v1.GetUserId(job))
	}
	return job
}

// generateOpsJobNodesInput generates node parameters for an ops job based on the specified scope.
// It determines the target nodes by resolving the job's scope parameter (workload, workspace, or cluster)
// and populates the job inputs with the corresponding node names. Nodes in the excludedNodes(parameter of request) list
// are filtered out. The function ensures that ops jobs are ultimately executed on a per-node basis.
func (h *Handler) generateOpsJobNodesInput(ctx context.Context, job *v1.OpsJob) error {
	excludedNodesSet := sets.NewSetByKeys(job.Spec.ExcludedNodes...)
	if workloadParam := job.GetParameter(v1.ParameterNode); workloadParam != nil {
		allNodes := job.GetParameters(v1.ParameterNode)
		newInputs := make([]v1.Parameter, 0, len(allNodes))
		for i, n := range allNodes {
			if excludedNodesSet.Has(n.Value) {
				continue
			}
			newInputs = append(newInputs, *allNodes[i])
		}
		if len(newInputs) != len(allNodes) {
			job.Spec.Inputs = newInputs
		}
	} else if workloadParam = job.GetParameter(v1.ParameterWorkload); workloadParam != nil {
		nodes, workspaceId, err := h.getNodesOfWorkload(ctx, workloadParam.Value)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			if excludedNodesSet.Has(n) {
				continue
			}
			job.Spec.Inputs = append(job.Spec.Inputs, v1.Parameter{Name: v1.ParameterNode, Value: n})
		}
		v1.SetLabel(job, v1.WorkspaceIdLabel, workspaceId)
	} else if workspaceParam := job.GetParameter(v1.ParameterWorkspace); workspaceParam != nil {
		nodes, err := commonnodes.GetNodesOfWorkspaces(ctx, h.Client, []string{workspaceParam.Value}, nil)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			if excludedNodesSet.Has(n.Name) {
				continue
			}
			job.Spec.Inputs = append(job.Spec.Inputs, v1.Parameter{Name: v1.ParameterNode, Value: n.Name})
		}
		v1.SetLabel(job, v1.WorkspaceIdLabel, workspaceParam.Value)
	} else if clusterParam := job.GetParameter(v1.ParameterCluster); clusterParam != nil {
		if v1.GetWorkspaceId(job) != "" {
			return commonerrors.NewBadRequest("cannot run cluster-wide job when workspaceId is already specified")
		}
		nodes, err := commonnodes.GetNodesOfCluster(ctx, h.Client, clusterParam.Value, nil)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			if excludedNodesSet.Has(n.Name) {
				continue
			}
			job.Spec.Inputs = append(job.Spec.Inputs, v1.Parameter{Name: v1.ParameterNode, Value: n.Name})
		}
	} else {
		return commonerrors.NewBadRequest("the nodes of ops-job is not specified")
	}
	return nil
}

// getNodesOfWorkload retrieves the list of nodes associated with a workload.
// It queries either the database or k8s cluster based on configuration to get node information.
// the workspaceId is also returned
func (h *Handler) getNodesOfWorkload(ctx context.Context, workloadId string) ([]string, string, error) {
	if commonconfig.IsDBEnable() {
		workload, err := h.dbClient.GetWorkload(ctx, workloadId)
		if err != nil {
			return nil, "", err
		}
		if str := dbutils.ParseNullString(workload.Nodes); str != "" {
			var nodes [][]string
			if json.Unmarshal([]byte(str), &nodes) == nil && len(nodes) > 0 {
				return nodes[len(nodes)-1], workload.Workspace, nil
			}
		}
	} else {
		workload, err := h.getAdminWorkload(ctx, workloadId)
		if err != nil {
			return nil, "", err
		}
		if len(workload.Status.Nodes) > 0 {
			return workload.Status.Nodes[len(workload.Status.Nodes)-1], workload.Spec.Workspace, nil
		}
	}
	return nil, "", nil
}

// parseListOpsJobQuery parses and validates the query parameters for listing ops jobs.
// It sets default values, validates time ranges, and handles authorization for system admin vs regular users.
func (h *Handler) parseListOpsJobQuery(c *gin.Context) (*types.ListOpsJobRequest, error) {
	query := &types.ListOpsJobRequest{}
	err := c.ShouldBindWith(&query, binding.Query)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}
	if query.Limit <= 0 {
		query.Limit = types.DefaultQueryLimit
	}
	if query.Order == "" {
		query.Order = dbclient.DESC
	}
	if query.SortBy == "" {
		dbTags := dbclient.GetOpsJobFieldTags()
		query.SortBy = dbclient.GetFieldTag(dbTags, "CreationTime")
	} else {
		query.SortBy = strings.ToLower(query.SortBy)
	}

	if query.Until == "" {
		query.UntilTime = time.Now().UTC()
	} else {
		query.UntilTime, err = time.Parse(timeutil.TimeRFC3339Milli, query.Until)
		if err != nil {
			return nil, err
		}
	}
	if query.Since != "" {
		query.SinceTime, err = time.Parse(timeutil.TimeRFC3339Milli, query.Since)
		if err != nil {
			return nil, err
		}
	} else {
		query.SinceTime = query.UntilTime.Add(-time.Hour * 24 * 30).UTC()
	}
	if query.SinceTime.After(query.UntilTime) {
		return nil, commonerrors.NewBadRequest("the since time is greater than until time")
	}
	if err = h.accessController.AuthorizeSystemAdmin(authority.AccessInput{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}, true); err != nil {
		query.UserId = c.GetString(common.UserId)
	}
	return query, nil
}

// cvtToListOpsJobSql converts the ops job list query parameters into an SQL query.
// It builds WHERE conditions based on filter parameters like cluster ID, phase, job type, user, and time range.
func cvtToListOpsJobSql(query *types.ListOpsJobRequest) (sqrl.Sqlizer, []string) {
	dbTags := dbclient.GetOpsJobFieldTags()
	creationTime := dbclient.GetFieldTag(dbTags, "CreationTime")
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.GtOrEq{creationTime: query.SinceTime},
		sqrl.LtOrEq{creationTime: query.UntilTime},
	}
	if clusterId := strings.TrimSpace(query.ClusterId); clusterId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Cluster"): clusterId})
	}
	if workspaceId := strings.TrimSpace(query.WorkspaceId); workspaceId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Workspace"): workspaceId})
	}
	if phase := strings.TrimSpace(string(query.Phase)); phase != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Phase"): phase})
	}
	if jobType := strings.TrimSpace(string(query.Type)); jobType != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "type"): jobType})
	}
	if userId := strings.TrimSpace(query.UserId); userId != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): userId})
	}
	if userName := strings.TrimSpace(query.UserName); userName != "" {
		dbSql = append(dbSql, sqrl.Like{
			dbclient.GetFieldTag(dbTags, "UserName"): fmt.Sprintf("%%%s%%", userName)})
	}
	orderBy := buildOrderBy(query.SortBy, query.Order, dbTags)
	return dbSql, orderBy
}

// cvtToGetOpsJobSql converts the get ops job request into an SQL query.
// It builds a query to retrieve a specific job by ID, with user access restrictions if not system admin.
func (h *Handler) cvtToGetOpsJobSql(c *gin.Context) (sqrl.Sqlizer, error) {
	jobId := c.GetString(common.Name)
	if jobId == "" {
		return nil, commonerrors.NewBadRequest("the jobId is empty")
	}
	dbTags := dbclient.GetOpsJobFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "JobId"): jobId},
	}
	if err := h.accessController.AuthorizeSystemAdmin(authority.AccessInput{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}, true); err != nil {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): c.GetString(common.UserId)})
	}
	return dbSql, nil
}

// parseCreateOpsJobRequest parses and validates the request for creating an ops job.
// It ensures required fields like name, type, and inputs are provided.
func parseCreateOpsJobRequest(c *gin.Context) (*types.BaseOpsJobRequest, []byte, error) {
	req := &types.BaseOpsJobRequest{}
	body, err := apiutils.ParseRequestBody(c.Request, req)
	if err != nil {
		return nil, nil, err
	}
	if req.Name == "" {
		return nil, nil, commonerrors.NewBadRequest("the job name is empty")
	}
	if req.Type == "" {
		return nil, nil, commonerrors.NewBadRequest("the job type is empty")
	}
	if len(req.Inputs) == 0 {
		return nil, nil, commonerrors.NewBadRequest("the job inputs is empty")
	}
	return req, body, nil
}

// cvtToOpsJobResponseItem converts a database ops job record to a response item format.
// Maps database fields to the appropriate response structure with proper null value handling.
func cvtToOpsJobResponseItem(job *dbclient.OpsJob) types.OpsJobResponseItem {
	result := types.OpsJobResponseItem{
		JobId:         job.JobId,
		JobName:       commonutils.GetBaseFromName(job.JobId),
		ClusterId:     job.Cluster,
		WorkspaceId:   dbutils.ParseNullString(job.Workspace),
		UserId:        dbutils.ParseNullString(job.UserId),
		UserName:      dbutils.ParseNullString(job.UserName),
		Type:          v1.OpsJobType(job.Type),
		Phase:         v1.OpsJobPhase(dbutils.ParseNullString(job.Phase)),
		CreationTime:  dbutils.ParseNullTimeToString(job.CreationTime),
		StartTime:     dbutils.ParseNullTimeToString(job.StartTime),
		EndTime:       dbutils.ParseNullTimeToString(job.EndTime),
		DeletionTime:  dbutils.ParseNullTimeToString(job.DeletionTime),
		TimeoutSecond: job.Timeout,
	}
	if result.Phase == "" {
		result.Phase = v1.OpsJobPending
	}
	return result
}

// cvtToGetOpsJobResponse converts a database ops job record to a detailed response format.
// Maps all database fields to the appropriate response structure including conditions, inputs, outputs, etc.
func cvtToGetOpsJobResponse(job *dbclient.OpsJob) types.GetOpsJobResponse {
	result := types.GetOpsJobResponse{
		OpsJobResponseItem: cvtToOpsJobResponseItem(job),
		IsTolerateAll:      job.IsTolerateAll,
	}
	if conditions := dbutils.ParseNullString(job.Conditions); conditions != "" {
		json.Unmarshal([]byte(conditions), &result.Conditions)
	}
	result.Inputs = deserializeParams(string(job.Inputs))
	if result.Type == v1.OpsJobAddonType || result.Type == v1.OpsJobPreflightType {
		if hasParameters(result.Inputs, v1.ParameterWorkload, v1.ParameterWorkspace, v1.ParameterCluster) {
			result.Inputs = getParametersExcept(result.Inputs, v1.ParameterNode)
		}
	}
	if outputs := dbutils.ParseNullString(job.Outputs); outputs != "" {
		json.Unmarshal([]byte(outputs), &result.Outputs)
	}
	if env := dbutils.ParseNullString(job.Env); env != "" {
		json.Unmarshal([]byte(env), &result.Env)
	}
	if resource := dbutils.ParseNullString(job.Resource); resource != "" {
		json.Unmarshal([]byte(resource), &result.Resource)
	}
	if image := dbutils.ParseNullString(job.Image); image != "" {
		result.Image = image
	}
	if entryPoint := dbutils.ParseNullString(job.EntryPoint); entryPoint != "" {
		result.EntryPoint = entryPoint
	}
	if hostpath := dbutils.ParseNullString(job.Hostpath); hostpath != "" {
		json.Unmarshal([]byte(hostpath), &result.Hostpath)
	}
	if nodes := dbutils.ParseNullString(job.ExcludedNodes); nodes != "" {
		json.Unmarshal([]byte(nodes), &result.ExcludedNodes)
	}
	return result
}

// deserializeParams converts a serialized parameter string into a slice of Parameter objects.
// It parses the string representation of parameters and converts them to structured format.
func deserializeParams(strInput string) []v1.Parameter {
	if len(strInput) <= 1 {
		return nil
	}
	strInput = strInput[1 : len(strInput)-1]
	splitParams := strings.Split(strInput, ",")
	var result []v1.Parameter
	for _, p := range splitParams {
		param := v1.CvtStringToParam(p)
		if param != nil {
			result = append(result, *param)
		}
	}
	return result
}

// getParametersExcept returns all parameters except those with the specified name
func getParametersExcept(inputs []v1.Parameter, ignoreName string) []v1.Parameter {
	var result []v1.Parameter
	for i, param := range inputs {
		if param.Name == ignoreName {
			continue
		}
		result = append(result, inputs[i])
	}
	return result
}

// hasParameters checks if a parameter with the given names exist.
func hasParameters(inputs []v1.Parameter, names ...string) bool {
	for _, name := range names {
		for _, param := range inputs {
			if param.Name == name {
				return true
			}
		}
	}
	return false
}
