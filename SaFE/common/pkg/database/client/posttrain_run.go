/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"strings"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

const (
	TPosttrainRun = "posttrain_run"
)

var (
	upsertPosttrainRunCmd = fmt.Sprintf(`INSERT INTO %s (
		run_id, workload_id, display_name, train_type, strategy, algorithm,
		workspace, cluster, user_id, user_name, base_model_id, base_model_name,
		dataset_id, dataset_name, image, node_count, gpu_per_node, cpu, memory,
		shared_memory, ephemeral_storage, priority, timeout, export_model,
		output_path, status, parameter_snapshot, resource_snapshot, created_at, updated_at, deletion_time, is_deleted
	) VALUES (
		:run_id, :workload_id, :display_name, :train_type, :strategy, :algorithm,
		:workspace, :cluster, :user_id, :user_name, :base_model_id, :base_model_name,
		:dataset_id, :dataset_name, :image, :node_count, :gpu_per_node, :cpu, :memory,
		:shared_memory, :ephemeral_storage, :priority, :timeout, :export_model,
		:output_path, :status, :parameter_snapshot, :resource_snapshot, :created_at, :updated_at, :deletion_time, :is_deleted
	)
	ON CONFLICT (run_id) DO UPDATE SET
		display_name = EXCLUDED.display_name,
		train_type = EXCLUDED.train_type,
		strategy = EXCLUDED.strategy,
		algorithm = EXCLUDED.algorithm,
		workspace = EXCLUDED.workspace,
		cluster = EXCLUDED.cluster,
		user_id = EXCLUDED.user_id,
		user_name = EXCLUDED.user_name,
		base_model_id = EXCLUDED.base_model_id,
		base_model_name = EXCLUDED.base_model_name,
		dataset_id = EXCLUDED.dataset_id,
		dataset_name = EXCLUDED.dataset_name,
		image = EXCLUDED.image,
		node_count = EXCLUDED.node_count,
		gpu_per_node = EXCLUDED.gpu_per_node,
		cpu = EXCLUDED.cpu,
		memory = EXCLUDED.memory,
		shared_memory = EXCLUDED.shared_memory,
		ephemeral_storage = EXCLUDED.ephemeral_storage,
		priority = EXCLUDED.priority,
		timeout = EXCLUDED.timeout,
		export_model = EXCLUDED.export_model,
		output_path = EXCLUDED.output_path,
		status = EXCLUDED.status,
		parameter_snapshot = EXCLUDED.parameter_snapshot,
		resource_snapshot = EXCLUDED.resource_snapshot,
		updated_at = EXCLUDED.updated_at,
		deletion_time = EXCLUDED.deletion_time,
		is_deleted = EXCLUDED.is_deleted`, TPosttrainRun)
	setPosttrainRunDeletedCmd = fmt.Sprintf(`UPDATE %s SET is_deleted = true, deletion_time = $2, updated_at = $2 WHERE run_id = $1`, TPosttrainRun)
)

// UpsertPosttrainRun creates or updates a posttrain run record.
func (c *Client) UpsertPosttrainRun(ctx context.Context, run *PosttrainRun) error {
	if run == nil {
		return commonerrors.NewBadRequest("the input is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	if !run.CreatedAt.Valid {
		run.CreatedAt = pqNullTime(time.Now().UTC())
	}
	run.UpdatedAt = pqNullTime(time.Now().UTC())
	_, err = db.NamedExecContext(ctx, upsertPosttrainRunCmd, run)
	if err != nil {
		klog.ErrorS(err, "failed to upsert posttrain run", "runId", run.RunID)
	}
	return err
}

// GetPosttrainRunView retrieves a posttrain run by run ID.
func (c *Client) GetPosttrainRunView(ctx context.Context, runID string) (*PosttrainRunView, error) {
	if runID == "" {
		return nil, commonerrors.NewBadRequest("runId is empty")
	}
	filter := &PosttrainRunFilter{
		Limit:  1,
		Offset: 0,
	}
	viewQuery := newPosttrainRunViewQuery(filter).Where(sqrl.Eq{"p.run_id": runID})
	row, err := c.selectOnePosttrainRunView(ctx, viewQuery)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, commonerrors.NewNotFound("posttrain run", runID)
	}
	return row, nil
}

// ListPosttrainRunViews lists posttrain runs with optional filters.
func (c *Client) ListPosttrainRunViews(ctx context.Context, filter *PosttrainRunFilter) ([]*PosttrainRunView, int, error) {
	if filter == nil {
		filter = &PosttrainRunFilter{}
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	dataQuery := newPosttrainRunViewQuery(filter)
	rows, err := c.selectPosttrainRunViews(ctx, dataQuery)
	if err != nil {
		return nil, 0, err
	}
	countQuery := newPosttrainRunCountQuery(filter)
	total, err := c.countPosttrainRuns(ctx, countQuery)
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// SetPosttrainRunDeleted marks a posttrain run record as deleted.
func (c *Client) SetPosttrainRunDeleted(ctx context.Context, runID string) error {
	if runID == "" {
		return commonerrors.NewBadRequest("runId is empty")
	}
	db, err := c.getDB()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = db.ExecContext(ctx, setPosttrainRunDeletedCmd, runID, now)
	if err != nil {
		klog.ErrorS(err, "failed to soft delete posttrain run", "runId", runID)
	}
	return err
}

func (c *Client) selectOnePosttrainRunView(ctx context.Context, query sqrl.SelectBuilder) (*PosttrainRunView, error) {
	rows, err := c.selectPosttrainRunViews(ctx, query.Limit(1))
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func (c *Client) selectPosttrainRunViews(ctx context.Context, query sqrl.SelectBuilder) ([]*PosttrainRunView, error) {
	db, err := c.getDB()
	if err != nil {
		return nil, err
	}
	sqlStr, args, err := query.PlaceholderFormat(sqrl.Dollar).ToSql()
	if err != nil {
		return nil, err
	}
	var rows []*PosttrainRunView
	if c.RequestTimeout > 0 {
		ctx2, cancel := context.WithTimeout(ctx, c.RequestTimeout)
		defer cancel()
		err = db.SelectContext(ctx2, &rows, sqlStr, args...)
	} else {
		err = db.SelectContext(ctx, &rows, sqlStr, args...)
	}
	return rows, err
}

func (c *Client) countPosttrainRuns(ctx context.Context, query sqrl.SelectBuilder) (int, error) {
	db, err := c.getDB()
	if err != nil {
		return 0, err
	}
	sqlStr, args, err := query.PlaceholderFormat(sqrl.Dollar).ToSql()
	if err != nil {
		return 0, err
	}
	var count int
	err = db.GetContext(ctx, &count, sqlStr, args...)
	return count, err
}

func newPosttrainRunViewQuery(filter *PosttrainRunFilter) sqrl.SelectBuilder {
	query := sqrl.Select(
		"p.id",
		"p.run_id",
		"p.workload_id",
		"COALESCE(w.workload_uid, '') AS workload_uid",
		"p.display_name",
		"p.train_type",
		"p.strategy",
		"p.algorithm",
		"p.workspace",
		"p.cluster",
		"p.user_id",
		"p.user_name",
		"p.base_model_id",
		"p.base_model_name",
		"p.dataset_id",
		"p.dataset_name",
		"p.image",
		"p.node_count",
		"p.gpu_per_node",
		"p.cpu",
		"p.memory",
		"p.shared_memory",
		"p.ephemeral_storage",
		"p.priority",
		"p.timeout",
		"p.export_model",
		"p.output_path",
		"p.parameter_snapshot",
		"p.resource_snapshot",
		"COALESCE(w.phase, p.status) AS status",
		"COALESCE(w.creation_time, p.created_at) AS created_at",
		"w.start_time",
		"w.end_time",
		"COALESCE(w.deletion_time, p.deletion_time) AS deletion_time",
		"(SELECT m.id FROM model m WHERE m.sft_job_id = p.workload_id AND m.is_deleted = false ORDER BY m.created_at DESC LIMIT 1) AS model_id",
		"(SELECT m.display_name FROM model m WHERE m.sft_job_id = p.workload_id AND m.is_deleted = false ORDER BY m.created_at DESC LIMIT 1) AS model_display_name",
		"(SELECT m.phase FROM model m WHERE m.sft_job_id = p.workload_id AND m.is_deleted = false ORDER BY m.created_at DESC LIMIT 1) AS model_phase",
		"(SELECT m.origin FROM model m WHERE m.sft_job_id = p.workload_id AND m.is_deleted = false ORDER BY m.created_at DESC LIMIT 1) AS model_origin",
	).From(TPosttrainRun + " p").
		LeftJoin(TWorkload + " w ON w.workload_id = p.workload_id").
		Where(sqrl.Eq{"p.is_deleted": false})

	query = applyPosttrainRunFilters(query, filter)
	orderExpr := "COALESCE(w.creation_time, p.created_at)"
	switch filter.SortBy {
	case "createdAt":
		orderExpr = "COALESCE(w.creation_time, p.created_at)"
	case "startTime":
		orderExpr = "w.start_time"
	case "endTime":
		orderExpr = "w.end_time"
	case "displayName":
		orderExpr = "p.display_name"
	case "status":
		orderExpr = "COALESCE(w.phase, p.status)"
	}
	if strings.EqualFold(filter.Order, ASC) {
		query = query.OrderBy(orderExpr + " ASC")
	} else {
		query = query.OrderBy(orderExpr + " DESC")
	}
	return query.Limit(uint64(filter.Limit)).Offset(uint64(filter.Offset))
}

func newPosttrainRunCountQuery(filter *PosttrainRunFilter) sqrl.SelectBuilder {
	query := sqrl.Select("COUNT(*)").
		From(TPosttrainRun + " p").
		LeftJoin(TWorkload + " w ON w.workload_id = p.workload_id").
		Where(sqrl.Eq{"p.is_deleted": false})
	return applyPosttrainRunFilters(query, filter)
}

func applyPosttrainRunFilters(query sqrl.SelectBuilder, filter *PosttrainRunFilter) sqrl.SelectBuilder {
	if filter == nil {
		return query
	}
	if filter.Workspace != "" {
		query = query.Where(sqrl.Eq{"p.workspace": filter.Workspace})
	}
	if filter.TrainType != "" {
		query = query.Where(sqrl.Eq{"p.train_type": filter.TrainType})
	}
	if filter.Strategy != "" {
		query = query.Where(sqrl.Eq{"p.strategy": filter.Strategy})
	}
	if filter.UserID != "" {
		query = query.Where(sqrl.Eq{"p.user_id": filter.UserID})
	}
	if filter.Status != "" {
		query = query.Where(sqrl.Expr("COALESCE(w.phase, p.status) = ?", filter.Status))
	}
	if filter.Search != "" {
		keyword := "%" + filter.Search + "%"
		query = query.Where(sqrl.Or{
			sqrl.ILike{"p.run_id": keyword},
			sqrl.ILike{"p.display_name": keyword},
			sqrl.ILike{"p.base_model_name": keyword},
			sqrl.ILike{"p.dataset_name": keyword},
		})
	}
	return query
}

func pqNullTime(t time.Time) pq.NullTime {
	return pq.NullTime{Time: t, Valid: !t.IsZero()}
}
