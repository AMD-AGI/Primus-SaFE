/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package custom_handlers

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/custom-handlers/types"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

func (h *Handler) CreateJob(c *gin.Context) {
	handle(c, h.createJob)
}

func (h *Handler) ListJob(c *gin.Context) {
	handle(c, h.listJob)
}

func (h *Handler) GetJob(c *gin.Context) {
	handle(c, h.getJob)
}

func (h *Handler) createJob(c *gin.Context) (interface{}, error) {
	req := &types.CreateJobRequest{}
	body, err := getBodyFromRequest(c.Request, req)
	if err != nil {
		klog.ErrorS(err, "failed to parse job request", "body", string(body))
		return nil, err
	}

	var job *v1.Job
	switch req.Type {
	case v1.JobAddonType:
		job, err = generateAddonJob(req)
	default:
		err = fmt.Errorf("unsupported job type")
	}
	if err != nil || job == nil {
		return nil, err
	}
	if err = h.Create(c.Request.Context(), job); err != nil {
		klog.ErrorS(err, "failed to create job")
		return nil, err
	}
	klog.Infof("create job: %s, type: %s, params: %v, user: %s",
		job.Name, job.Spec.Type, job.Spec.Inputs, req.UserName)
	return &types.CreateJobResponse{JobId: job.Name}, nil
}

func (h *Handler) listJob(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	query, err := parseListJobQuery(c)
	if err != nil {
		klog.ErrorS(err, "failed to parse query")
		return nil, err
	}

	dbSql := cvtToListJobSql(query)
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, query.SortBy, query.Order, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}
	count, err := h.dbClient.CountJobs(c.Request.Context(), dbSql)
	if err != nil {
		return nil, err
	}
	result := &types.GetJobResponse{
		TotalCount: count,
	}
	for _, job := range jobs {
		result.Items = append(result.Items, cvtToJobResponse(job))
	}
	return result, nil
}

func (h *Handler) getJob(c *gin.Context) (interface{}, error) {
	if !commonconfig.IsDBEnable() {
		return nil, commonerrors.NewInternalError("the database function is not enabled")
	}
	jobId := c.GetString(types.Name)
	if jobId == "" {
		return nil, commonerrors.NewBadRequest("the jobId is empty")
	}

	dbSql := cvtToGetJobSql(jobId)
	jobs, err := h.dbClient.SelectJobs(c.Request.Context(), dbSql, "", "", 1, 0)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("job %s is not found", jobId))
	}
	return cvtToJobResponse(jobs[0]), nil
}

func generateAddonJob(req *types.CreateJobRequest) (*v1.Job, error) {
	job := generateJob(req)
	jobName := ""
	if p := job.GetParameter(v1.ParameterNodeTemplate); p != nil {
		jobName = p.Value
	} else if p = job.GetParameter(v1.ParameterAddonTemplate); p != nil {
		jobName = p.Value
	} else {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("either %s or %s must be specified in the job.",
			v1.ParameterAddonTemplate, v1.ParameterNodeTemplate))
	}

	if p := job.GetParameter(v1.ParameterNode); p != nil {
		jobName = p.Value + "-" + jobName
	}

	job.Name = jobName
	if req.SecurityUpgrade {
		v1.SetAnnotation(job, v1.JobSecurityUpgradeAnnotation, "")
	}
	batchCount := req.BatchCount
	if batchCount == 0 {
		batchCount = commonconfig.GetJobBatchCount()
	}
	v1.SetAnnotation(job, v1.JobBatchCountAnnotation, strconv.Itoa(batchCount))
	return job, nil
}

func generateJob(req *types.CreateJobRequest) *v1.Job {
	job := &v1.Job{
		Spec: v1.JobSpec{
			Cluster:       req.Cluster,
			Type:          req.Type,
			Inputs:        req.Inputs,
			TimeoutSecond: req.TimeoutSecond,
		},
	}
	v1.SetAnnotation(job, v1.UserNameAnnotation, req.JobName)
	nowTime := time.Now()
	v1.SetAnnotation(job, v1.JobDispatchTimeAnnotation, timeutil.FormatRFC3339(&nowTime))
	return job
}

func parseListJobQuery(c *gin.Context) (*types.GetJobRequest, error) {
	query := &types.GetJobRequest{}
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
		dbTags := dbclient.GetJobFieldTags()
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

func cvtToListJobSql(query *types.GetJobRequest) sqrl.Sqlizer {
	dbTags := dbclient.GetJobFieldTags()
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

func cvtToGetJobSql(jobId string) sqrl.Sqlizer {
	dbTags := dbclient.GetJobFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "JobId"): jobId},
	}
	return dbSql
}

func cvtToJobResponse(job *dbclient.Job) types.GetJobResponseItem {
	result := types.GetJobResponseItem{
		JobId:      job.JobId,
		JobName:    dbutils.ParseNullString(job.JobName),
		Cluster:    job.Cluster,
		Workspace:  dbutils.ParseNullString(job.Workspace),
		Type:       v1.JobType(job.Type),
		UserName:   dbutils.ParseNullString(job.UserName),
		Phase:      v1.JobPhase(dbutils.ParseNullString(job.Phase)),
		CreateTime: dbutils.ParseNullTimeToString(job.CreateTime),
		StartTime:  dbutils.ParseNullTimeToString(job.StartTime),
		EndTime:    dbutils.ParseNullTimeToString(job.EndTime),
		DeleteTime: dbutils.ParseNullTimeToString(job.DeleteTime),
		Message:    dbutils.ParseNullString(job.Message),
	}
	if result.Phase == "" {
		result.Phase = v1.JobPending
	}
	result.Inputs = deserializeParams(string(job.Inputs))
	result.Outputs = deserializeParams(dbutils.ParseNullString(job.Outputs))
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
