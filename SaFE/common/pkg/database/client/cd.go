/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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

var (
	insertDeploymentRequestFormat   = `INSERT INTO ` + TDeploymentRequest + ` (%s) VALUES (%s)`
	insertEnvironmentSnapshotFormat = `INSERT INTO ` + TEnvironmentSnapshot + ` (%s) VALUES (%s)`
)

const (
	TDeploymentRequest   = "deployment_requests"
	TEnvironmentSnapshot = "environment_snapshots"
)

type CDInterface interface {
	CreateDeploymentRequest(ctx context.Context, req *DeploymentRequest) (int64, error)
	GetDeploymentRequest(ctx context.Context, id int64) (*DeploymentRequest, error)
	ListDeploymentRequests(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*DeploymentRequest, error)
	CountDeploymentRequests(ctx context.Context, query sqrl.Sqlizer) (int, error)
	UpdateDeploymentRequest(ctx context.Context, req *DeploymentRequest) error

	CreateEnvironmentSnapshot(ctx context.Context, snapshot *EnvironmentSnapshot) (int64, error)
	GetEnvironmentSnapshot(ctx context.Context, id int64) (*EnvironmentSnapshot, error)
	ListEnvironmentSnapshots(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*EnvironmentSnapshot, error)
}

// CreateDeploymentRequest inserts a new deployment request
func (c *Client) CreateDeploymentRequest(ctx context.Context, req *DeploymentRequest) (int64, error) {
	if req == nil {
		return 0, commonerrors.NewBadRequest("request is nil")
	}
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}

	// Ensure timestamps
	now := time.Now().UTC()
	req.CreatedAt = dbutils.NullTime(now)
	req.UpdatedAt = dbutils.NullTime(now)

	cmd := generateCommand(*req, insertDeploymentRequestFormat, "id")
	// Using NamedQuery to get ID back if needed, or simple Exec
	// For PostgreSQL, we can use RETURNING id
	cmd += " RETURNING id"

	rows, err := db.NamedQueryContext(ctx, cmd, req)
	if err != nil {
		klog.ErrorS(err, "failed to insert deployment request")
		return 0, err
	}
	defer rows.Close()

	var id int64
	if rows.Next() {
		if err := rows.Scan(&id); err != nil {
			klog.ErrorS(err, "failed to scan inserted deployment request ID")
			return 0, err
		}
	}
	return id, nil
}

// GetDeploymentRequest gets a request by ID
func (c *Client) GetDeploymentRequest(ctx context.Context, id int64) (*DeploymentRequest, error) {
	dbTags := GetDeploymentRequestFieldTags()
	query := sqrl.Eq{GetFieldTag(dbTags, "Id"): id}
	list, err := c.ListDeploymentRequests(ctx, query, nil, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, commonerrors.NewNotFound("deployment_request", fmt.Sprintf("%d", id))
	}
	return list[0], nil
}

// ListDeploymentRequests lists requests
func (c *Client) ListDeploymentRequests(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*DeploymentRequest, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TDeploymentRequest).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var list []*DeploymentRequest
	if err = db.SelectContext(ctx, &list, sql, args...); err != nil {
		return nil, err
	}
	return list, nil
}

// CountDeploymentRequests counts requests
func (c *Client) CountDeploymentRequests(ctx context.Context, query sqrl.Sqlizer) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sql, args, err := sqrl.Select("COUNT(*)").PlaceholderFormat(sqrl.Dollar).From(TDeploymentRequest).Where(query).ToSql()
	if err != nil {
		return 0, err
	}
	var cnt int
	err = db.GetContext(ctx, &cnt, sql, args...)
	return cnt, err
}

// UpdateDeploymentRequest updates fields
func (c *Client) UpdateDeploymentRequest(ctx context.Context, req *DeploymentRequest) error {
	db, err := c.getDB()
	if err != nil {
		return err
	}

	req.UpdatedAt = dbutils.NullTime(time.Now().UTC())

	// Construct generic update
	// Note: simplified update, usually we update specific fields based on input
	cmd := fmt.Sprintf(`UPDATE %s SET 
		status=:status, 
		approver_name=:approver_name, 
		approval_result=:approval_result,
		env_config=:env_config,
		description=:description,
		rejection_reason=:rejection_reason,
		failure_reason=:failure_reason,
		rollback_from_id=:rollback_from_id,
		updated_at=:updated_at,
		approved_at=:approved_at
		WHERE id=:id`, TDeploymentRequest)

	_, err = db.NamedExecContext(ctx, cmd, req)
	return err
}

// CreateEnvironmentSnapshot creates a snapshot
func (c *Client) CreateEnvironmentSnapshot(ctx context.Context, snapshot *EnvironmentSnapshot) (int64, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}

	now := time.Now().UTC()
	snapshot.CreatedAt = dbutils.NullTime(now)
	snapshot.UpdatedAt = dbutils.NullTime(now)

	cmd := generateCommand(*snapshot, insertEnvironmentSnapshotFormat, "id")
	cmd += " RETURNING id"

	rows, err := db.NamedQueryContext(ctx, cmd, snapshot)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var id int64
	if rows.Next() {
		rows.Scan(&id)
	}
	return id, nil
}

// GetEnvironmentSnapshot gets a snapshot by ID
func (c *Client) GetEnvironmentSnapshot(ctx context.Context, id int64) (*EnvironmentSnapshot, error) {
	dbTags := GetEnvironmentSnapshotFieldTags()
	query := sqrl.Eq{GetFieldTag(dbTags, "Id"): id}
	list, err := c.ListEnvironmentSnapshots(ctx, query, nil, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, commonerrors.NewNotFound("environment_snapshot", fmt.Sprintf("%d", id))
	}
	return list[0], nil
}

// GetEnvironmentSnapshotByRequestId gets a snapshot by deployment_request_id
func (c *Client) GetEnvironmentSnapshotByRequestId(ctx context.Context, reqId int64) (*EnvironmentSnapshot, error) {
	dbTags := GetEnvironmentSnapshotFieldTags()
	query := sqrl.Eq{GetFieldTag(dbTags, "DeploymentRequestId"): reqId}
	list, err := c.ListEnvironmentSnapshots(ctx, query, nil, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, commonerrors.NewNotFound("environment_snapshot", fmt.Sprintf("request_id=%d", reqId))
	}
	return list[0], nil
}

// ListEnvironmentSnapshots lists snapshots
func (c *Client) ListEnvironmentSnapshots(ctx context.Context, query sqrl.Sqlizer, orderBy []string, limit, offset int) ([]*EnvironmentSnapshot, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}

	sql, args, err := sqrl.Select("*").PlaceholderFormat(sqrl.Dollar).
		From(TEnvironmentSnapshot).
		Where(query).
		OrderBy(orderBy...).
		Limit(uint64(limit)).
		Offset(uint64(offset)).ToSql()
	if err != nil {
		return nil, err
	}

	var list []*EnvironmentSnapshot
	if err = db.SelectContext(ctx, &list, sql, args...); err != nil {
		return nil, err
	}
	return list, nil
}
