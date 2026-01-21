/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TEvaluationTask = "evaluation_task"
)

var (
	getEvaluationTaskCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE task_id = $1 LIMIT 1`, TEvaluationTask)
	insertEvaluationTaskFormat = `INSERT INTO ` + TEvaluationTask + ` (%s) VALUES (%s)`
	updateEvaluationTaskCmd    = fmt.Sprintf(`UPDATE %s 
		SET task_name = :task_name,
		    description = :description,
		    service_id = :service_id,
		    service_type = :service_type,
		    service_name = :service_name,
		    benchmarks = :benchmarks,
		    eval_params = :eval_params,
		    ops_job_id = :ops_job_id,
		    status = :status,
		    progress = :progress,
		    result_summary = :result_summary,
		    report_s3_path = :report_s3_path,
		    start_time = :start_time,
		    end_time = :end_time
		WHERE task_id = :task_id`, TEvaluationTask)
)

// UpsertEvaluationTask performs the UpsertEvaluationTask operation.
func (c *Client) UpsertEvaluationTask(ctx context.Context, task *EvaluationTask) error {
	if task == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var tasks []*EvaluationTask
	if err = db.SelectContext(ctx, &tasks, getEvaluationTaskCmd, task.TaskId); err != nil {
		klog.ErrorS(err, "failed to select evaluation task", "id", task.TaskId)
		return err
	}
	if len(tasks) > 0 && tasks[0] != nil {
		_, err = db.NamedExecContext(ctx, updateEvaluationTaskCmd, task)
		if err != nil {
			klog.ErrorS(err, "failed to update evaluation task db", "id", task.TaskId)
		}
	} else {
		_, err = db.NamedExecContext(ctx, generateCommand(*task, insertEvaluationTaskFormat, "id"), task)
		if err != nil {
			klog.ErrorS(err, "failed to insert evaluation task db", "id", task.TaskId)
		}
	}
	return err
}

// SelectEvaluationTasks retrieves multiple evaluation task records.
func (c *Client) SelectEvaluationTasks(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*EvaluationTask, error) {
	startTime := time.Now().UTC()
	defer func() {
		if query != nil {
			strQuery := dbutils.CvtToSqlStr(query)
			klog.Infof("select evaluation tasks, query: %s, orderBy: %v, limit: %d, offset: %d, cost (%v)",
				strQuery, orderBy, limit, offset, time.Since(startTime))
		}
	}()
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TEvaluationTask).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var tasks []*EvaluationTask
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &tasks, sql, args...)
	} else {
		err = db.SelectContext(ctx, &tasks, sql, args...)
	}
	return tasks, err
}

// CountEvaluationTasks returns the total count of evaluation tasks matching the criteria.
func (c *Client) CountEvaluationTasks(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TEvaluationTask).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// GetEvaluationTask retrieves an evaluation task by ID.
func (c *Client) GetEvaluationTask(ctx context.Context, taskId string) (*EvaluationTask, error) {
	if taskId == "" {
		return nil, commonerrors.NewBadRequest("taskId is empty")
	}
	dbTags := GetEvaluationTaskFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{GetFieldTag(dbTags, "TaskId"): taskId},
	}
	tasks, err := c.SelectEvaluationTasks(ctx, dbSql, nil, 1, 0)
	if err != nil {
		klog.ErrorS(err, "failed to select evaluation task", "sql", dbutils.CvtToSqlStr(dbSql))
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, commonerrors.NewNotFoundWithMessage(fmt.Sprintf("evaluation task %s not found", taskId))
	}
	return tasks[0], nil
}

// SetEvaluationTaskDeleted marks an evaluation task as deleted in the database.
func (c *Client) SetEvaluationTaskDeleted(ctx context.Context, taskId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET is_deleted=true, end_time=$2 WHERE task_id=$1`, TEvaluationTask)
	_, err = db.ExecContext(ctx, cmd, taskId, time.Now().UTC())
	if err != nil {
		klog.ErrorS(err, "failed to delete evaluation task", "TaskId", taskId)
		return err
	}
	return nil
}

// UpdateEvaluationTaskStatus updates the status and progress of an evaluation task.
func (c *Client) UpdateEvaluationTaskStatus(ctx context.Context, taskId string, status EvaluationTaskStatus, progress int) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET status=$1, progress=$2 WHERE task_id=$3`, TEvaluationTask)
	_, err = db.ExecContext(ctx, cmd, status, progress, taskId)
	if err != nil {
		klog.ErrorS(err, "failed to update evaluation task status", "TaskId", taskId)
		return err
	}
	return nil
}

// UpdateEvaluationTaskOpsJobId updates the ops_job_id of an evaluation task.
func (c *Client) UpdateEvaluationTaskOpsJobId(ctx context.Context, taskId, opsJobId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET ops_job_id=$1 WHERE task_id=$2`, TEvaluationTask)
	_, err = db.ExecContext(ctx, cmd, opsJobId, taskId)
	if err != nil {
		klog.ErrorS(err, "failed to update evaluation task ops_job_id", "TaskId", taskId, "OpsJobId", opsJobId)
		return err
	}
	return nil
}

// UpdateEvaluationTaskResult updates the result summary and report path of an evaluation task.
func (c *Client) UpdateEvaluationTaskResult(ctx context.Context, taskId string, resultSummary, reportS3Path string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET result_summary=$1, report_s3_path=$2, end_time=$3, status=$4 WHERE task_id=$5`, TEvaluationTask)
	_, err = db.ExecContext(ctx, cmd, resultSummary, reportS3Path, time.Now().UTC(), EvaluationTaskStatusSucceeded, taskId)
	if err != nil {
		klog.ErrorS(err, "failed to update evaluation task result", "TaskId", taskId)
		return err
	}
	return nil
}

// UpdateEvaluationTaskStartTime updates the start time and status of an evaluation task.
func (c *Client) UpdateEvaluationTaskStartTime(ctx context.Context, taskId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET start_time=$1, status=$2 WHERE task_id=$3`, TEvaluationTask)
	_, err = db.ExecContext(ctx, cmd, time.Now().UTC(), EvaluationTaskStatusRunning, taskId)
	if err != nil {
		klog.ErrorS(err, "failed to update evaluation task start time", "TaskId", taskId)
		return err
	}
	return nil
}

// SetEvaluationTaskFailed marks an evaluation task as failed.
func (c *Client) SetEvaluationTaskFailed(ctx context.Context, taskId, message string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET status=$1, result_summary=$2, end_time=$3 WHERE task_id=$4`, TEvaluationTask)
	resultSummary := fmt.Sprintf(`{"error": "%s"}`, message)
	_, err = db.ExecContext(ctx, cmd, EvaluationTaskStatusFailed, resultSummary, time.Now().UTC(), taskId)
	if err != nil {
		klog.ErrorS(err, "failed to set evaluation task failed", "TaskId", taskId)
		return err
	}
	return nil
}

