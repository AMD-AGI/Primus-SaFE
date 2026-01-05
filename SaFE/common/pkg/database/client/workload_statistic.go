/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/dal"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

// GetWorkloadStatisticByID returns the WorkloadStatistic by ID.
func (c *Client) GetWorkloadStatisticByID(ctx context.Context, id int32) (*model.WorkloadStatistic, error) {
	q := dal.Use(c.gorm).WorkloadStatistic
	item, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get workload statistic by id %d: %w", id, err)
	}
	return item, nil
}

// GetWorkloadStatisticByWorkloadID returns the WorkloadStatistic by workload ID.
func (c *Client) GetWorkloadStatisticByWorkloadID(ctx context.Context, workloadID string) (*model.WorkloadStatistic, error) {
	q := dal.Use(c.gorm).WorkloadStatistic
	item, err := q.WithContext(ctx).Where(q.WorkloadID.Eq(workloadID)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get workload statistic by workload id %s: %w", workloadID, err)
	}
	return item, nil
}

// GetWorkloadStatisticsByWorkloadID returns all WorkloadStatistics by workload ID.
func (c *Client) GetWorkloadStatisticsByWorkloadID(ctx context.Context, workloadID string) ([]*model.WorkloadStatistic, error) {
	q := dal.Use(c.gorm).WorkloadStatistic
	items, err := q.WithContext(ctx).Where(q.WorkloadID.Eq(workloadID)).Find()
	if err != nil {
		return nil, fmt.Errorf("failed to get workload statistics by workload id %s: %w", workloadID, err)
	}
	return items, nil
}

// GetWorkloadStatisticByWorkloadUID returns the WorkloadStatistic by workload UID.
func (c *Client) GetWorkloadStatisticByWorkloadUID(ctx context.Context, workloadUID string) (*model.WorkloadStatistic, error) {
	q := dal.Use(c.gorm).WorkloadStatistic
	item, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get workload statistic by workload uid %s: %w", workloadUID, err)
	}
	return item, nil
}

// GetWorkloadStatisticsByWorkloadUID returns all WorkloadStatistics by workload UID.
func (c *Client) GetWorkloadStatisticsByWorkloadUID(ctx context.Context, workloadUID string) ([]*model.WorkloadStatistic, error) {
	q := dal.Use(c.gorm).WorkloadStatistic
	items, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUID)).Find()
	if err != nil {
		return nil, fmt.Errorf("failed to get workload statistics by workload uid %s: %w", workloadUID, err)
	}
	return items, nil
}

// GetWorkloadStatisticsByClusterAndWorkspace returns all WorkloadStatistics by cluster and workspace.
func (c *Client) GetWorkloadStatisticsByClusterAndWorkspace(ctx context.Context, cluster, workspace string) ([]*model.WorkloadStatistic, error) {
	q := dal.Use(c.gorm).WorkloadStatistic
	items, err := q.WithContext(ctx).Where(q.Cluster.Eq(cluster), q.Workspace.Eq(workspace)).Find()
	if err != nil {
		return nil, fmt.Errorf("failed to get workload statistics by cluster %s and workspace %s: %w", cluster, workspace, err)
	}
	return items, nil
}

// GetWorkloadStatisticsByType returns all WorkloadStatistics by statistic type.
func (c *Client) GetWorkloadStatisticsByType(ctx context.Context, statisticType string) ([]*model.WorkloadStatistic, error) {
	q := dal.Use(c.gorm).WorkloadStatistic
	items, err := q.WithContext(ctx).Where(q.StatisticType.Eq(statisticType)).Find()
	if err != nil {
		return nil, fmt.Errorf("failed to get workload statistics by type %s: %w", statisticType, err)
	}
	return items, nil
}

// UpsertWorkloadStatistic performs the UpsertWorkloadStatistic operation.
func (c *Client) UpsertWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error {
	exist, err := c.GetWorkloadStatisticByID(ctx, stat.ID)
	if err != nil {
		return err
	}
	if exist == nil {
		// insert
		if err := dal.Use(c.gorm).WorkloadStatistic.WithContext(ctx).Create(stat); err != nil {
			return fmt.Errorf("failed to insert workload statistic %v: %w", stat, err)
		}
	} else {
		// update
		stat.ID = exist.ID
		if err := dal.Use(c.gorm).WorkloadStatistic.WithContext(ctx).Save(stat); err != nil {
			return fmt.Errorf("failed to update workload statistic %v: %w", stat, err)
		}
	}
	return nil
}

// UpdateWorkloadStatistic updates the specified resource.
func (c *Client) UpdateWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error {
	err := dal.Use(c.gorm).WorkloadStatistic.WithContext(ctx).Save(stat)
	if err != nil {
		return fmt.Errorf("failed to update workload statistic %v: %w", stat, err)
	}
	return nil
}

// DeleteWorkloadStatistic deletes the workload statistic by ID.
func (c *Client) DeleteWorkloadStatistic(ctx context.Context, id int32) error {
	q := dal.Use(c.gorm).WorkloadStatistic
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete workload statistic by id %d: %w", id, err)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workload statistic with id %d not found", id)
	}
	return nil
}

// DeleteWorkloadStatisticsByWorkloadID deletes all workload statistics by workload ID.
func (c *Client) DeleteWorkloadStatisticsByWorkloadID(ctx context.Context, workloadID string) error {
	q := dal.Use(c.gorm).WorkloadStatistic
	_, err := q.WithContext(ctx).Where(q.WorkloadID.Eq(workloadID)).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete workload statistics by workload id %s: %w", workloadID, err)
	}
	return nil
}

// CreateWorkloadStatistic creates a new workload statistic.
func (c *Client) CreateWorkloadStatistic(ctx context.Context, stat *model.WorkloadStatistic) error {
	if err := dal.Use(c.gorm).WorkloadStatistic.WithContext(ctx).Create(stat); err != nil {
		return fmt.Errorf("failed to create workload statistic %v: %w", stat, err)
	}
	return nil
}
