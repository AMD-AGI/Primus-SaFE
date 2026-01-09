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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	dbutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/utils"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TWorkload = "workload"
)

var (
	getWorkloadCmd       = fmt.Sprintf(`SELECT  * FROM %s WHERE workload_id = $1 LIMIT 1`, TWorkload)
	insertWorkloadFormat = `INSERT INTO ` + TWorkload + ` (%s) VALUES (%s)`
	updateWorkloadCmd    = fmt.Sprintf(`UPDATE %s 
		SET priority = :priority,
		    max_retry = :max_retry,
		    resources = :resources,
		    image = :image,
		    entrypoint = :entrypoint,
		    phase = :phase,
		    conditions = :conditions,
		    start_time = :start_time,
		    end_time = :end_time,
		    deletion_time = :deletion_time,
		    queue_position = :queue_position,
		    env = :env,
		    description = :description,
		    pods = :pods,
		    dispatch_count = :dispatch_count,
		    nodes = :nodes,
		    ranks = :ranks,
		    is_supervised = :is_supervised,
		    is_tolerate_all = :is_tolerate_all,
		    timeout = :timeout,
		    cron_jobs = :cron_jobs 
		WHERE workload_id = :workload_id`, TWorkload)
)

// UpsertWorkload performs the UpsertWorkload operation.
func (c *Client) UpsertWorkload(ctx context.Context, workload *Workload) error {
	if workload == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}

	var workloads []*Workload
	if err = db.SelectContext(ctx, &workloads, getWorkloadCmd, workload.WorkloadId); err != nil {
		klog.ErrorS(err, "failed to select workload", "id", workload.WorkloadId)
		return err
	}
	if len(workloads) > 0 && workloads[0] != nil {
		_, err = db.NamedExecContext(ctx, updateWorkloadCmd, workload)
		if err != nil {
			klog.ErrorS(err, "failed to upsert workload db", "id", workload.WorkloadId)
		}
	} else {
		_, err = db.NamedExecContext(ctx, generateCommand(*workload, insertWorkloadFormat, "id"), workload)
		if err != nil {
			klog.ErrorS(err, "failed to insert workload db", "id", workload.WorkloadId)
		}
	}
	return err
}

// SelectWorkloads retrieves multiple workload records.
func (c *Client) SelectWorkloads(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*Workload, error) {
	startTime := time.Now().UTC()
	defer func() {
		if query != nil {
			strQuery := dbutils.CvtToSqlStr(query)
			klog.Infof("select workload, query: %s, orderBy: %v, limit: %d, offset: %d, cost (%v)",
				strQuery, orderBy, limit, offset, time.Since(startTime))
		}
	}()
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TWorkload).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var workloads []*Workload
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &workloads, sql, args...)
	} else {
		err = db.SelectContext(ctx, &workloads, sql, args...)
	}
	return workloads, err
}

// CountWorkloads returns the total count of workloads matching the criteria.
func (c *Client) CountWorkloads(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TWorkload).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// SetWorkloadDeleted marks a workload as deleted in the database.
func (c *Client) SetWorkloadDeleted(ctx context.Context, workloadId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET is_deleted=true WHERE workload_id=$1`, TWorkload)
	_, err = db.ExecContext(ctx, cmd, workloadId)
	if err != nil {
		klog.ErrorS(err, "failed to update workload db. ", "WorkloadId", workloadId)
		return err
	}
	return nil
}

// SetWorkloadStopped marks a workload as stopped in the database.
func (c *Client) SetWorkloadStopped(ctx context.Context, workloadId string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	cmd := fmt.Sprintf(`UPDATE %s SET phase='%s', end_time=$2, deletion_time=$3 WHERE workload_id=$1`,
		TWorkload, v1.WorkloadStopped)
	_, err = db.ExecContext(ctx, cmd, workloadId, nowTime, nowTime)
	if err != nil {
		klog.ErrorS(err, "failed to update workload db. ", "WorkloadId", workloadId)
		return err
	}
	return nil
}

// SetWorkloadDescription updates the description of a workload.
func (c *Client) SetWorkloadDescription(ctx context.Context, workloadId, description string) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}
	cmd := fmt.Sprintf(`UPDATE %s SET description=$1 WHERE workload_id=$2`, TWorkload)
	_, err = db.ExecContext(ctx, cmd, description, workloadId)
	if err != nil {
		klog.ErrorS(err, "failed to update workload db. ", "WorkloadId", workloadId)
		return err
	}
	return nil
}

// GetWorkload retrieves a workload by ID.
func (c *Client) GetWorkload(ctx context.Context, workloadId string) (*Workload, error) {
	if workloadId == "" {
		return nil, commonerrors.NewBadRequest("workloadId is empty")
	}
	dbTags := GetWorkloadFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{GetFieldTag(dbTags, "WorkloadId"): workloadId},
	}
	workloads, err := c.SelectWorkloads(ctx, dbSql, nil, 1, 0)
	if err != nil {
		klog.ErrorS(err, "failed to select workload", "sql", dbutils.CvtToSqlStr(dbSql))
		return nil, err
	}
	if len(workloads) == 0 {
		return nil, commonerrors.NewNotFound(v1.WorkloadKind, workloadId)
	}
	return workloads[0], nil
}
