/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TOpsJob = "ops_job"
)

var (
	getJobCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE job_id = $1 LIMIT 1`, TOpsJob)
	insertJobFormat = `INSERT INTO ` + TOpsJob + ` (%s) VALUES (%s)`
	updateJobCmd    = fmt.Sprintf(`UPDATE %s 
		SET inputs = :inputs,
		    start_time = :start_time,
		    end_time = :end_time,
		    deletion_time = :deletion_time,
		    phase = :phase,
		    conditions = :conditions,
		    env = :env,
		    outputs = :outputs 
		WHERE job_id = :job_id`, TOpsJob)
)

// UpsertJob performs the UpsertJob operation.
func (c *Client) UpsertJob(ctx context.Context, job *OpsJob) error {
	if job == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	var jobs []*OpsJob
	if err = db.SelectContext(ctx, &jobs, getJobCmd, job.JobId); err != nil {
		return err
	}
	if len(jobs) > 0 && jobs[0] != nil {
		if _, err = db.NamedExecContext(ctx, updateJobCmd, job); err != nil {
			klog.ErrorS(err, "failed to upsert job db", "id", job.JobId)
			return err
		}
	} else {
		_, err = db.NamedExecContext(ctx, generateCommand(*job, insertJobFormat, "id"), job)
		if err != nil {
			klog.ErrorS(err, "failed to insert job db", "id", job.JobId)
			return err
		}
	}
	return nil
}

// SelectJobs performs the SelectJobs operation.
func (c *Client) SelectJobs(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*OpsJob, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TOpsJob).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var jobs []*OpsJob
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &jobs, sql, args...)
	} else {
		err = db.SelectContext(ctx, &jobs, sql, args...)
	}
	return jobs, err
}

// CountJobs returns the count of resources.
func (c *Client) CountJobs(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TOpsJob).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// SetOpsJobDeleted sets the OpsJobDeleted value.
func (c *Client) SetOpsJobDeleted(ctx context.Context, opsJobId, userId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var cmd string
	var args []interface{}
	if userId != "" {
		cmd = fmt.Sprintf(`UPDATE %s SET is_deleted = true WHERE job_id = $1 AND user_id = $2`, TOpsJob)
		args = []interface{}{opsJobId, userId}
	} else {
		cmd = fmt.Sprintf(`UPDATE %s SET is_deleted = true WHERE job_id = $1`, TOpsJob)
		args = []interface{}{opsJobId}
	}

	_, err = db.ExecContext(ctx, cmd, args...)
	if err != nil {
		klog.ErrorS(err, "failed to update opsjob db", "job_id", opsJobId, "user_id", userId)
		return err
	}
	return nil
}
