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
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	commonjob "github.com/AMD-AIG-AIMA/SAFE/common/pkg/ops_job"
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
	req, err := parseCreateOpsJobQuery(c)
	if err != nil {
		return nil, err
	}

	ctx := c.Request.Context()
	var job *v1.OpsJob
	switch req.Type {
	case v1.OpsJobAddonType, v1.OpsJobPreflightType:
		job, err = h.generateAddonJob(ctx, req)
	case v1.OpsJobDumpLogType:
		job, err = h.generateDumpLogJob(ctx, req)
	default:
		err = fmt.Errorf("unsupported ops job type")
	}
	if err != nil || job == nil {
		return nil, err
	}
	if err = h.Create(ctx, job); err != nil {
		klog.ErrorS(err, "failed to create ops job")
		return nil, err
	}
	klog.Infof("create ops job: %s, type: %s, params: %v, user: %s",
		job.Name, job.Spec.Type, job.Spec.Inputs, req.UserName)
	return &types.CreateOpsJobResponse{JobId: job.Name}, nil
}

func (h *Handler) listOpsJob(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	query, err := parseListOpsJobQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}

	ctx := c.Request.Context()
	dbSql := cvtToListOpsJobSql(query)
	jobs, err := h.dbClient.SelectJobs(ctx, dbSql, query.SortBy, query.Order, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	count, err := h.dbClient.CountJobs(ctx, dbSql)
	if err != nil {
		return nil, err
	}
	result := &types.GetOpsJobResponse{
		TotalCount: count,
	}
	for _, job := range jobs {
		result.Items = append(result.Items, cvtToOpsJobResponse(job))
	}
	return result, nil
}

func (h *Handler) getOpsJob(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	jobId := c.GetString(types.Name)
	if jobId == "" {
		return nil, commonerrors.NewBadRequest("the jobId is empty")
	}

	dbSql := cvtToGetOpsJobSql(jobId)
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, "", "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("job %s is not found", jobId))
	}
	return cvtToOpsJobResponse(jobs[0]), nil
}

func (h *Handler) deleteOpsJob(c *gin.Context) (interface{}, error) {
	name := c.GetString(types.Name)
	if name == "" {
		return nil, commonerrors.NewBadRequest("opsJobId is empty")
	}
	ctx := c.Request.Context()
	opsJob := &v1.OpsJob{}
	if h.Get(ctx, client.ObjectKey{Name: name}, opsJob) == nil {
		if err := h.Delete(ctx, opsJob); err != nil {
			return nil, err
		}
	}
	if err := commonjob.CleanupJobRelatedInfo(ctx, h.Client, name); err != nil {
		klog.ErrorS(err, "failed to cleanup ops job labels")
	}
	if commonconfig.IsDBEnable() {
		if err := h.dbClient.SetOpsJobDeleted(ctx, name); err != nil {
			return nil, err
		}
	}
	klog.Infof("delete opsJob %s", name)
	return nil, nil
}

func (h *Handler) generateAddonJob(ctx context.Context, req *types.CreateOpsJobRequest) (*v1.OpsJob, error) {
	job := generateOpsJob(req)
	if job.Spec.Cluster == "" {
		if nodeParam := job.GetParameter(v1.ParameterNode); nodeParam != nil {
			adminNode, err := h.getAdminNode(ctx, nodeParam.Value)
			if err != nil {
				return nil, err
			}
			job.Spec.Cluster = v1.GetClusterId(adminNode)
		}
	}
	return job, nil
}

func (h *Handler) generateDumpLogJob(ctx context.Context, req *types.CreateOpsJobRequest) (*v1.OpsJob, error) {
	if !commonconfig.IsLogEnable() {
		return nil, commonerrors.NewInternalError("The logging function is not enabled")
	}
	if !commonconfig.IsS3Enable() {
		return nil, commonerrors.NewInternalError("The s3 function is not enabled")
	}
	job := generateOpsJob(req)

	workloadParam := job.GetParameter(v1.ParameterWorkload)
	if workloadParam == nil {
		return nil, commonerrors.NewBadRequest(
			fmt.Sprintf("%s must be specified in the job.", v1.ParameterWorkload))
	}
	if commonconfig.IsDBEnable() {
		workload, err := h.dbClient.GetWorkload(ctx, workloadParam.Value)
		if err != nil {
			return nil, err
		}
		job.Spec.Cluster = workload.Cluster
		v1.SetLabel(job, v1.WorkspaceIdLabel, workload.Workspace)
	} else {
		workload, err := h.getAdminWorkload(ctx, workloadParam.Value)
		if err != nil {
			return nil, err
		}
		job.Spec.Cluster = v1.GetClusterId(workload)
		v1.SetLabel(job, v1.WorkspaceIdLabel, workload.Spec.Workspace)
	}
	return job, nil
}

func generateOpsJob(req *types.CreateOpsJobRequest) *v1.OpsJob {
	job := &v1.OpsJob{
		Spec: v1.OpsJobSpec{
			Cluster:       req.Cluster,
			Type:          req.Type,
			Inputs:        req.Inputs,
			TimeoutSecond: req.TimeoutSecond,
		},
	}
	if req.UserName != "" {
		v1.SetAnnotation(job, v1.UserNameAnnotation, req.UserName)
	}
	v1.SetAnnotation(job, v1.OpsJobBatchCountAnnotation, strconv.Itoa(req.BatchCount))
	v1.SetAnnotation(job, v1.OpsJobAvailRatioAnnotation,
		strconv.FormatFloat(*req.AvailableRatio, 'f', -1, 64))
	if req.SecurityUpgrade {
		v1.SetAnnotation(job, v1.OpsJobSecurityUpgradeAnnotation, "")
	}
	if req.GpuProduct != "" {
		v1.SetLabel(job, v1.GpuProductNameLabel, strings.ToLower(req.GpuProduct))
	}
	return job
}

func parseCreateOpsJobQuery(c *gin.Context) (*types.CreateOpsJobRequest, error) {
	req := &types.CreateOpsJobRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse ops job request", "body", string(body))
		return nil, err
	}
	if req.BatchCount <= 0 {
		req.BatchCount = 1
	}
	if req.AvailableRatio == nil || *req.AvailableRatio <= 0 {
		req.AvailableRatio = pointer.Float64(1.0)
	}
	return req, nil
}

func parseListOpsJobQuery(c *gin.Context) (*types.GetOpsJobRequest, error) {
	query := &types.GetOpsJobRequest{}
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
	return query, nil
}

func cvtToListOpsJobSql(query *types.GetOpsJobRequest) sqrl.Sqlizer {
	dbTags := dbclient.GetOpsJobFieldTags()
	createTime := dbclient.GetFieldTag(dbTags, "CreateTime")
	dbSql := sqrl.And{
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
	if userName := strings.TrimSpace(query.UserName); userName != "" {
		dbSql = append(dbSql, sqrl.Like{
			dbclient.GetFieldTag(dbTags, "UserName"): fmt.Sprintf("%%%s%%", userName)})
	}
	return dbSql
}

func cvtToGetOpsJobSql(jobId string) sqrl.Sqlizer {
	dbTags := dbclient.GetOpsJobFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "JobId"): jobId},
	}
	return dbSql
}

func cvtToOpsJobResponse(job *dbclient.OpsJob) types.GetOpsJobResponseItem {
	result := types.GetOpsJobResponseItem{
		JobId:      job.JobId,
		Cluster:    job.Cluster,
		Workspace:  dbutils.ParseNullString(job.Workspace),
		Type:       v1.OpsJobType(job.Type),
		UserName:   dbutils.ParseNullString(job.UserName),
		Phase:      v1.OpsJobPhase(dbutils.ParseNullString(job.Phase)),
		CreateTime: dbutils.ParseNullTimeToString(job.CreateTime),
		StartTime:  dbutils.ParseNullTimeToString(job.StartTime),
		EndTime:    dbutils.ParseNullTimeToString(job.EndTime),
		DeleteTime: dbutils.ParseNullTimeToString(job.DeleteTime),
	}
	if result.Phase == "" {
		result.Phase = v1.OpsJobPending
	}
	result.Inputs = deserializeParams(string(job.Inputs))
	if outputs := dbutils.ParseNullString(job.Outputs); outputs != "" {
		json.Unmarshal([]byte(outputs), &result.Outputs)
	}
	if conditions := dbutils.ParseNullString(job.Conditions); conditions != "" {
		json.Unmarshal([]byte(conditions), &result.Conditions)
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
