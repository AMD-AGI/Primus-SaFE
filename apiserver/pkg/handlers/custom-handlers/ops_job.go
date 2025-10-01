/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

func (h *Handler) CreateOpsJob(c *gin.Context) {
	handle(c, h.createOpsJob)
}

func (h *Handler) ListOpsJob(c *gin.Context) {
	handle(c, h.listOpsJob)
}

func (h *Handler) GetOpsJob(c *gin.Context) {
	handle(c, h.getOpsJob)
}

func (h *Handler) DeleteOpsJob(c *gin.Context) {
	handle(c, h.deleteOpsJob)
}

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

func (h *Handler) listOpsJob(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	query, err := h.parseListOpsJobQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}

	dbSql := cvtToListOpsJobSql(query)
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, query.SortBy, query.Order, query.Limit, query.Offset)
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
		result.Items = append(result.Items, cvtToOpsJobResponseItem(job, false))
	}
	return result, nil
}

func (h *Handler) getOpsJob(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	dbSql, err := h.cvtToGetOpsJobSql(c)
	if err != nil {
		return nil, err
	}
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, "", "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, commonerrors.NewNotFoundWithMessage("the opsjob is not found")
	}
	return cvtToOpsJobResponseItem(jobs[0], true), nil
}

func (h *Handler) deleteOpsJob(c *gin.Context) (interface{}, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("opsJobId is empty")
	}
	ctx := c.Request.Context()
	opsJob := &v1.OpsJob{}
	isFound := false
	if h.Get(ctx, client.ObjectKey{Name: name}, opsJob) == nil {
		if err = h.auth.Authorize(authority.Input{
			Context:      ctx,
			ResourceKind: v1.OpsJobKind,
			Verb:         v1.DeleteVerb,
			UserId:       c.GetString(common.UserId),
		}); err != nil {
			return nil, err
		}
		if err = h.Delete(ctx, opsJob); err != nil {
			return nil, err
		}
		isFound = true
	}
	if commonconfig.IsDBEnable() {
		userId := ""
		if requestUser != nil && !requestUser.IsSystemAdmin() {
			userId = requestUser.Name
		}
		if err = h.dbClient.SetOpsJobDeleted(ctx, name, userId); err != nil {
			return nil, err
		}
		isFound = true
	}
	if !isFound {
		return nil, commonerrors.NewNotFoundWithMessage("the opsjob is not found")
	}

	if err = commonjob.CleanupJobRelatedInfo(ctx, h.Client, name); err != nil {
		klog.ErrorS(err, "failed to cleanup ops job labels")
	}
	klog.Infof("delete opsJob %s", name)
	return nil, nil
}

func (h *Handler) generateAddonJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
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
	job := genDefaultOpsJob(c, &req.BaseOpsJobRequest)
	if req.SecurityUpgrade {
		v1.SetAnnotation(job, v1.OpsJobSecurityUpgradeAnnotation, "")
	}
	v1.SetAnnotation(job, v1.OpsJobBatchCountAnnotation, strconv.Itoa(req.BatchCount))
	v1.SetAnnotation(job, v1.OpsJobAvailRatioAnnotation,
		strconv.FormatFloat(*req.AvailableRatio, 'f', -1, 64))

	if err = h.genOpsJobInputs(c.Request.Context(), job, &req.BaseOpsJobRequest); err != nil {
		return nil, err
	}
	return job, nil
}

func (h *Handler) generatePreflightJob(c *gin.Context, body []byte) (*v1.OpsJob, error) {
	requestUser, err := h.getAndSetUsername(c)
	if err != nil {
		return nil, err
	}

	if err = h.auth.AuthorizeSystemAdmin(authority.Input{
		Context: c.Request.Context(),
		User:    requestUser,
	}); err != nil {
		return nil, err
	}

	req := &types.CreatePreflightRequest{}
	if err = jsonutils.Unmarshal(body, req); err != nil {
		return nil, err
	}
	job := genDefaultOpsJob(c, &req.BaseOpsJobRequest)
	job.Spec.Resource = req.Resource
	job.Spec.Image = req.Image
	job.Spec.EntryPoint = req.EntryPoint
	job.Spec.Env = req.Env
	job.Spec.IsTolerateAll = req.IsTolerateAll

	if err = h.genOpsJobInputs(c.Request.Context(), job, &req.BaseOpsJobRequest); err != nil {
		return nil, err
	}
	return job, nil
}

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
	job := genDefaultOpsJob(c, &req.BaseOpsJobRequest)

	workloadParam := job.GetParameter(v1.ParameterWorkload)
	if workloadParam == nil {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("%s must be specified in the job.", v1.ParameterWorkload))
	}
	// Compatible with the old API.
	if req.Name == "" {
		req.Name = workloadParam.Value
		v1.SetLabel(job, v1.DisplayNameLabel, workloadParam.Value)
	}

	workload, err := h.getWorkloadInternal(c.Request.Context(), workloadParam.Value)
	if err != nil {
		return nil, err
	}
	if err = h.auth.Authorize(authority.Input{
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

func genDefaultOpsJob(c *gin.Context, req *types.BaseOpsJobRequest) *v1.OpsJob {
	job := &v1.OpsJob{
		ObjectMeta: metav1.ObjectMeta{
			Name: commonutils.GenerateName(req.Name),
			Labels: map[string]string{
				v1.UserIdLabel:      c.GetString(common.UserId),
				v1.DisplayNameLabel: req.Name,
			},
			Annotations: map[string]string{
				v1.UserNameAnnotation: c.GetString(common.UserName),
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

func (h *Handler) genOpsJobInputs(ctx context.Context, job *v1.OpsJob, req *types.BaseOpsJobRequest) error {
	if job.GetParameter(v1.ParameterNode) != nil {
		return nil
	}
	excludedNodesSet := sets.NewSetByKeys(req.ExcludedNodes...)
	if workloadParam := job.GetParameter(v1.ParameterWorkload); workloadParam != nil {
		nodes, err := h.getNodesOfWorkload(ctx, workloadParam.Value)
		if err != nil {
			return err
		}
		for _, n := range nodes {
			if excludedNodesSet.Has(n) {
				continue
			}
			job.Spec.Inputs = append(job.Spec.Inputs, v1.Parameter{Name: v1.ParameterNode, Value: n})
		}
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
	} else if clusterParam := job.GetParameter(v1.ParameterCluster); clusterParam != nil {
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

func (h *Handler) getNodesOfWorkload(ctx context.Context, workloadId string) ([]string, error) {
	if commonconfig.IsDBEnable() {
		workload, err := h.dbClient.GetWorkload(ctx, workloadId)
		if err != nil {
			return nil, err
		}
		if str := dbutils.ParseNullString(workload.Nodes); str != "" {
			var nodes [][]string
			json.Unmarshal([]byte(str), &nodes)
			if len(nodes) > 0 {
				return nodes[len(nodes)-1], nil
			}
		}
	} else {
		workload, err := h.getAdminWorkload(ctx, workloadId)
		if err != nil {
			return nil, err
		}
		if len(workload.Status.Nodes) > 0 {
			return workload.Status.Nodes[len(workload.Status.Nodes)-1], nil
		}
	}
	return nil, nil
}

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
		query.SortBy = dbclient.GetFieldTag(dbTags, "CreateTime")
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
	if err = h.auth.AuthorizeSystemAdmin(authority.Input{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}); err != nil {
		query.UserId = c.GetString(common.UserId)
	}
	return query, nil
}

func cvtToListOpsJobSql(query *types.ListOpsJobRequest) sqrl.Sqlizer {
	dbTags := dbclient.GetOpsJobFieldTags()
	createTime := dbclient.GetFieldTag(dbTags, "CreateTime")
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.GtOrEq{createTime: query.SinceTime},
		sqrl.LtOrEq{createTime: query.UntilTime},
	}
	if cluster := strings.TrimSpace(query.Cluster); cluster != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "Cluster"): cluster})
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
	return dbSql
}

func (h *Handler) cvtToGetOpsJobSql(c *gin.Context) (sqrl.Sqlizer, error) {
	jobId := c.GetString(types.Name)
	if jobId == "" {
		return nil, commonerrors.NewBadRequest("the jobId is empty")
	}
	dbTags := dbclient.GetOpsJobFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "JobId"): jobId},
	}
	if err := h.auth.AuthorizeSystemAdmin(authority.Input{
		Context: c.Request.Context(),
		UserId:  c.GetString(common.UserId),
	}); err != nil {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): c.GetString(common.UserId)})
	}
	return dbSql, nil
}

func parseCreateOpsJobRequest(c *gin.Context) (*types.BaseOpsJobRequest, []byte, error) {
	req := &types.BaseOpsJobRequest{}
	body, err := getBodyFromRequest(c.Request, req)
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

func cvtToOpsJobResponseItem(job *dbclient.OpsJob, isNeedDetail bool) types.OpsJobResponseItem {
	result := types.OpsJobResponseItem{
		JobId:        job.JobId,
		JobName:      commonutils.GetBaseFromName(job.JobId),
		Cluster:      job.Cluster,
		Workspace:    dbutils.ParseNullString(job.Workspace),
		UserId:       dbutils.ParseNullString(job.UserId),
		UserName:     dbutils.ParseNullString(job.UserName),
		Type:         v1.OpsJobType(job.Type),
		Phase:        v1.OpsJobPhase(dbutils.ParseNullString(job.Phase)),
		CreationTime: dbutils.ParseNullTimeToString(job.CreateTime),
		StartTime:    dbutils.ParseNullTimeToString(job.StartTime),
		EndTime:      dbutils.ParseNullTimeToString(job.EndTime),
		DeletionTime: dbutils.ParseNullTimeToString(job.DeleteTime),
	}
	if result.Phase == "" {
		result.Phase = v1.OpsJobPending
	}
	if !isNeedDetail {
		return result
	}

	if conditions := dbutils.ParseNullString(job.Conditions); conditions != "" {
		json.Unmarshal([]byte(conditions), &result.Conditions)
	}
	result.Inputs = deserializeParams(string(job.Inputs))
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
	return result
}

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
