/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TJob = "ops_job"
)

var (
	getJobCmd       = fmt.Sprintf(`SELECT * FROM %s WHERE job_id = $1 LIMIT 1`, TJob)
	insertJobFormat = `INSERT INTO ` + TJob + ` (%s) VALUES (%s)`
	updateJobCmd    = fmt.Sprintf(`UPDATE %s 
		SET inputs = :inputs,
		    start_time = :start_time,
		    end_time = :end_time,
		    delete_time = :delete_time,
		    phase = :phase,
		    conditions = :conditions,
		    message = :message,
		    outputs = :outputs 
		WHERE job_id = :job_id`, TJob)
)

func (c *Client) UpsertJob(ctx context.Context, job *OpsJob) error {
	if job == nil {
		return nil
	}
	db := c.db.Unsafe()
	jobs := []*OpsJob{}
	var err error
	if err = db.SelectContext(ctx, &jobs, getJobCmd, job.JobId); err != nil {
		return err
	}
	if len(jobs) > 0 && jobs[0] != nil {
		if _, err = db.NamedExecContext(ctx, updateJobCmd, job); err != nil {
			klog.ErrorS(err, "failed to upsert job db", "id", job.JobId)
			return err
		}
	} else {
		_, err = db.NamedExecContext(ctx, genInsertCommand(*job, insertJobFormat, "id"), job)
		if err != nil {
			klog.ErrorS(err, "failed to insert job db", "id", job.JobId)
			return err
		}
	}
	return nil
}

func (c *Client) SelectJobs(ctx context.Context, query sqrl.Sqlizer, sortBy, order string, limit, offset int) ([]*OpsJob, error) {
	if c.db == nil {
		return nil, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	db := c.db.Unsafe()
	orderBy := func() []string {
		var results []string
		if sortBy == "" || order == "" {
			return results
		}
		if order == DESC {
			results = append(results, fmt.Sprintf("%s desc", sortBy))
		} else {
			results = append(results, fmt.Sprintf("%s asc", sortBy))
		}
		return results
	}()
	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TJob).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var jobs []*OpsJob
	ctx2, cancel := context.WithTimeout(ctx, time.Duration(commonconfig.GetDBRequestTimeoutSecond())*time.Second)
	defer cancel()
	err = db.SelectContext(ctx2, &jobs, sql, args...)
	return jobs, err
}

func (c *Client) CountJobs(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	if c.db == nil {
		return 0, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	db := c.db.Unsafe()
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TJob).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}
