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
		    resource = :resource,
		    image = :image,
		    entrypoint = :entrypoint,
		    phase = :phase,
		    conditions = :conditions,
		    start_time = :start_time,
		    end_time = :end_time,
		    delete_time = :delete_time,
		    scheduler_order = :scheduler_order,
		    env = :env,
		    description = :description,
		    pods = :pods,
		    dispatch_count = :dispatch_count,
		    nodes = :nodes,
		    is_supervised = :is_supervised,
		    is_tolerate_all = :is_tolerate_all,
		    timeout = :timeout 
		WHERE workload_id = :workload_id`, TWorkload)
)

func (c *Client) UpsertWorkload(ctx context.Context, workload *Workload) error {
	if workload == nil {
		return nil
	}

	db := c.db.Unsafe()
	var workloads []*Workload
	var err error
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
		_, err = db.NamedExecContext(ctx, genInsertCommand(*workload, insertWorkloadFormat, "id"), workload)
		if err != nil {
			klog.ErrorS(err, "failed to insert workload db", "id", workload.WorkloadId)
		}
	}
	return err
}

func (c *Client) SelectWorkloads(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*Workload, error) {
	startTime := time.Now().UTC()
	defer func() {
		if query != nil {
			if strQuery, args, err := query.ToSql(); err == nil {
				klog.Infof("select workload, where: %s, args: %v, cost (%v)", strQuery, args, time.Since(startTime))
				return
			}
		}
	}()

	if c.db == nil {
		return nil, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	db := c.db.Unsafe()
	if limit < 0 {
		var err error
		if limit, err = c.CountWorkloads(ctx, query); err != nil {
			return nil, err
		}
	}
	if offset < 0 {
		offset = 0
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

func (c *Client) CountWorkloads(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	if c.db == nil {
		return 0, commonerrors.NewInternalError("The client of db has not been initialized")
	}
	db := c.db.Unsafe()
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TWorkload).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

func (c *Client) SetWorkloadDeleted(ctx context.Context, workloadId string) error {
	db := c.db.Unsafe()
	cmd := fmt.Sprintf(`UPDATE %s SET is_deleted=true WHERE workload_id=$1`, TWorkload)
	_, err := db.ExecContext(ctx, cmd, workloadId)
	if err != nil {
		klog.ErrorS(err, "failed to update workload db. ", "WorkloadId", workloadId)
		return err
	}
	return nil
}

func (c *Client) SetWorkloadStopped(ctx context.Context, workloadId string) error {
	db := c.db.Unsafe()
	nowTime := dbutils.NullMetaV1Time(&metav1.Time{Time: time.Now().UTC()})
	cmd := fmt.Sprintf(`UPDATE %s SET phase='%s', end_time=$2, delete_time=$3 WHERE workload_id=$1`,
		TWorkload, v1.WorkloadStopped)
	_, err := db.ExecContext(ctx, cmd, workloadId, nowTime, nowTime)
	if err != nil {
		klog.ErrorS(err, "failed to update workload db. ", "WorkloadId", workloadId)
		return err
	}
	return nil
}

func (c *Client) SetWorkloadDescription(ctx context.Context, workloadId, description string) error {
	db := c.db.Unsafe()
	cmd := fmt.Sprintf(`UPDATE %s SET description=$1 WHERE workload_id=$2`, TWorkload)
	_, err := db.ExecContext(ctx, cmd, description, workloadId)
	if err != nil {
		klog.ErrorS(err, "failed to update workload db. ", "WorkloadId", workloadId)
		return err
	}
	return nil
}

func (c *Client) GetWorkload(ctx context.Context, workloadId string) (*Workload, error) {
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
