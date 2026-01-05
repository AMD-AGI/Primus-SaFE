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

// GetNodeStatisticByID returns the NodeStatistic by ID.
func (c *Client) GetNodeStatisticByID(ctx context.Context, id int32) (*model.NodeStatistic, error) {
	q := dal.Use(c.gorm).NodeStatistic
	item, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get node statistic by id %d: %w", id, err)
	}
	return item, nil
}

// GetNodeStatisticByClusterAndNode returns the NodeStatistic by cluster and node name.
func (c *Client) GetNodeStatisticByClusterAndNode(ctx context.Context, cluster, nodeName string) (*model.NodeStatistic, error) {
	q := dal.Use(c.gorm).NodeStatistic
	item, err := q.WithContext(ctx).Where(q.Cluster.Eq(cluster), q.NodeName.Eq(nodeName)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get node statistic by cluster %s and node %s: %w", cluster, nodeName, err)
	}
	return item, nil
}

// GetNodeStatisticsByCluster returns all NodeStatistics by cluster.
func (c *Client) GetNodeStatisticsByCluster(ctx context.Context, cluster string) ([]*model.NodeStatistic, error) {
	q := dal.Use(c.gorm).NodeStatistic
	items, err := q.WithContext(ctx).Where(q.Cluster.Eq(cluster)).Find()
	if err != nil {
		return nil, fmt.Errorf("failed to get node statistics by cluster %s: %w", cluster, err)
	}
	return items, nil
}

// GetNodeStatisticsByNodeNames returns NodeStatistics by node names, optionally filtered by cluster.
// This method is optimized for batch queries.
func (c *Client) GetNodeStatisticsByNodeNames(ctx context.Context, cluster string, nodeNames []string) ([]*model.NodeStatistic, error) {
	if len(nodeNames) == 0 {
		return []*model.NodeStatistic{}, nil
	}

	q := dal.Use(c.gorm).NodeStatistic
	query := q.WithContext(ctx).Where(q.NodeName.In(nodeNames...))

	// Filter by cluster if specified
	if cluster != "" {
		query = query.Where(q.Cluster.Eq(cluster))
	}

	items, err := query.Find()
	if err != nil {
		return nil, fmt.Errorf("failed to get node statistics by node names: %w", err)
	}
	return items, nil
}

// GetNodeGpuUtilizationMap returns a map of node names to GPU utilization values.
// This is optimized for the listNode API use case.
func (c *Client) GetNodeGpuUtilizationMap(ctx context.Context, cluster string, nodeNames []string) (map[string]float64, error) {
	if len(nodeNames) == 0 {
		return make(map[string]float64), nil
	}

	q := dal.Use(c.gorm).NodeStatistic
	query := q.WithContext(ctx).Select(q.NodeName, q.GpuUtilization)

	// Filter by cluster if specified
	if cluster != "" {
		query = query.Where(q.Cluster.Eq(cluster))
	}

	// Filter by node names
	query = query.Where(q.NodeName.In(nodeNames...))

	// Use a struct for scanning
	var statistics []struct {
		NodeName       string
		GpuUtilization float64
	}

	err := query.Scan(&statistics)
	if err != nil {
		return nil, fmt.Errorf("failed to get node GPU utilization map: %w", err)
	}

	// Build result map
	result := make(map[string]float64, len(statistics))
	for _, stat := range statistics {
		result[stat.NodeName] = stat.GpuUtilization
	}

	// Debug logging (only if results don't match expectations)
	if len(result) != len(nodeNames) {
		// Use fmt.Printf as klog might not be available in this package
		fmt.Printf("WARNING: GetNodeGpuUtilizationMap - requested %d nodes, got %d results. cluster=%q, requested=%v, found=%v\n",
			len(nodeNames), len(result), cluster, nodeNames, getKeys(result))
	}

	return result, nil
}

// Helper function to get keys from map for debugging
func getKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// CreateNodeStatistic creates a new node statistic.
func (c *Client) CreateNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error {
	if err := dal.Use(c.gorm).NodeStatistic.WithContext(ctx).Create(stat); err != nil {
		return fmt.Errorf("failed to create node statistic %v: %w", stat, err)
	}
	return nil
}

// UpdateNodeStatistic updates the specified node statistic.
func (c *Client) UpdateNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error {
	err := dal.Use(c.gorm).NodeStatistic.WithContext(ctx).Save(stat)
	if err != nil {
		return fmt.Errorf("failed to update node statistic %v: %w", stat, err)
	}
	return nil
}

// UpsertNodeStatistic performs upsert operation for node statistic.
// It updates if record exists, otherwise creates a new one.
func (c *Client) UpsertNodeStatistic(ctx context.Context, stat *model.NodeStatistic) error {
	exist, err := c.GetNodeStatisticByClusterAndNode(ctx, stat.Cluster, stat.NodeName)
	if err != nil {
		return err
	}
	if exist == nil {
		// insert
		if err := dal.Use(c.gorm).NodeStatistic.WithContext(ctx).Create(stat); err != nil {
			return fmt.Errorf("failed to insert node statistic %v: %w", stat, err)
		}
	} else {
		// update
		stat.ID = exist.ID
		stat.CreatedAt = exist.CreatedAt
		if err := dal.Use(c.gorm).NodeStatistic.WithContext(ctx).Save(stat); err != nil {
			return fmt.Errorf("failed to update node statistic %v: %w", stat, err)
		}
	}
	return nil
}

// DeleteNodeStatistic deletes the node statistic by ID.
func (c *Client) DeleteNodeStatistic(ctx context.Context, id int32) error {
	q := dal.Use(c.gorm).NodeStatistic
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete node statistic by id %d: %w", id, err)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("node statistic with id %d not found", id)
	}
	return nil
}

// DeleteNodeStatisticByClusterAndNode deletes the node statistic by cluster and node name.
func (c *Client) DeleteNodeStatisticByClusterAndNode(ctx context.Context, cluster, nodeName string) error {
	q := dal.Use(c.gorm).NodeStatistic
	_, err := q.WithContext(ctx).Where(q.Cluster.Eq(cluster), q.NodeName.Eq(nodeName)).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete node statistic by cluster %s and node %s: %w", cluster, nodeName, err)
	}
	return nil
}

// DeleteNodeStatisticsByCluster deletes all node statistics by cluster.
func (c *Client) DeleteNodeStatisticsByCluster(ctx context.Context, cluster string) error {
	q := dal.Use(c.gorm).NodeStatistic
	_, err := q.WithContext(ctx).Where(q.Cluster.Eq(cluster)).Delete()
	if err != nil {
		return fmt.Errorf("failed to delete node statistics by cluster %s: %w", cluster, err)
	}
	return nil
}
