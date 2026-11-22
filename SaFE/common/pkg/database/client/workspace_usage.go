/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client/model"
)

const workspaceUsageBatchSize = 500

var (
	workspaceUsageSampleKey = []clause.Column{
		{Name: "cluster"}, {Name: "workspace"}, {Name: "resource"}, {Name: "sample_time"},
	}
	workspaceUsageSampleValues = []string{
		"quota", "allocated", "busy", "util_coverage", "queued_quota", "queued_user_qos",
		"queued_external", "queued_other", "running_workloads", "queued_workloads",
	}
	workspaceUsageHourlyKey = []clause.Column{
		{Name: "cluster"}, {Name: "workspace"}, {Name: "resource"}, {Name: "hour"},
	}
	workspaceUsageHourlyValues = []string{
		"quota_avg", "allocated_avg", "allocated_max", "busy_avg", "util_coverage_avg",
		"queued_avg", "queued_max", "queued_quota_avg", "queued_user_qos_avg", "queued_external_avg",
		"queued_other_avg", "queued_quota_max", "queued_user_qos_max", "queued_external_max",
		"queued_other_max", "allocated_hours", "busy_hours", "samples", "observed_seconds",
		"cluster_queued_max",
	}
)

// WorkspaceUsageFilter selects workspace usage rows. An empty Resource or Cluster matches every
// value. A nil Workspaces matches every workspace, an empty non-nil one matches none. The row
// time is in [Start, End); a zero time leaves that side open.
type WorkspaceUsageFilter struct {
	Resource   string
	Cluster    string
	Workspaces []string
	Start      time.Time
	End        time.Time
}

// isEmpty reports whether the filter can match no row.
func (f WorkspaceUsageFilter) isEmpty() bool {
	return f.Workspaces != nil && len(f.Workspaces) == 0
}

// apply adds the filter to a query whose row time is in timeColumn.
func (f WorkspaceUsageFilter) apply(db *gorm.DB, timeColumn string) *gorm.DB {
	if f.Resource != "" {
		db = db.Where("resource = ?", f.Resource)
	}
	if f.Cluster != "" {
		db = db.Where("cluster = ?", f.Cluster)
	}
	if f.Workspaces != nil {
		db = db.Where("workspace IN ?", f.Workspaces)
	}
	if !f.Start.IsZero() {
		db = db.Where(timeColumn+" >= ?", f.Start)
	}
	if !f.End.IsZero() {
		db = db.Where(timeColumn+" < ?", f.End)
	}
	return db
}

// UpsertWorkspaceUsageSamples writes samples, replacing the values of a sample with the same
// cluster, workspace, resource and time.
func (c *Client) UpsertWorkspaceUsageSamples(ctx context.Context, rows []*model.WorkspaceUsageSample) error {
	if len(rows) == 0 {
		return nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	err = db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   workspaceUsageSampleKey,
		DoUpdates: clause.AssignmentColumns(workspaceUsageSampleValues),
	}).CreateInBatches(rows, workspaceUsageBatchSize).Error
	if err != nil {
		return fmt.Errorf("failed to upsert workspace usage samples: %w", err)
	}
	return nil
}

// ListWorkspaceUsageSamples returns the samples the filter selects, oldest first.
func (c *Client) ListWorkspaceUsageSamples(ctx context.Context,
	filter WorkspaceUsageFilter) ([]*model.WorkspaceUsageSample, error) {
	if filter.isEmpty() {
		return nil, nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var rows []*model.WorkspaceUsageSample
	err = filter.apply(db.WithContext(ctx), "sample_time").Order("sample_time").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace usage samples: %w", err)
	}
	return rows, nil
}

// ListLatestWorkspaceUsageSamples returns, for each cluster, the samples of the latest time that
// cluster has a sample of the filter's resource at, no earlier than filter.Start.
// The workspace filter applies after the latest time is chosen.
func (c *Client) ListLatestWorkspaceUsageSamples(ctx context.Context,
	filter WorkspaceUsageFilter) ([]*model.WorkspaceUsageSample, error) {
	if filter.isEmpty() {
		return nil, nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	latestFilter := WorkspaceUsageFilter{Resource: filter.Resource, Cluster: filter.Cluster, Start: filter.Start}
	latest := latestFilter.apply(db.WithContext(ctx).Model(&model.WorkspaceUsageSample{}), "sample_time").
		Select("cluster, max(sample_time)").Group("cluster")
	var rows []*model.WorkspaceUsageSample
	err = filter.apply(db.WithContext(ctx), "sample_time").
		Where("(cluster, sample_time) IN (?)", latest).
		Order("cluster").Order("workspace").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list the latest workspace usage samples: %w", err)
	}
	return rows, nil
}

// DeleteWorkspaceUsageSamplesBefore deletes the samples older than before.
func (c *Client) DeleteWorkspaceUsageSamplesBefore(ctx context.Context, before time.Time) (int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return 0, err
	}
	result := db.WithContext(ctx).Where("sample_time < ?", before).Delete(&model.WorkspaceUsageSample{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete workspace usage samples: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// UpsertWorkspaceUsageHourly writes hourly rows, replacing the values of a row with the same
// cluster, workspace, resource and hour.
func (c *Client) UpsertWorkspaceUsageHourly(ctx context.Context, rows []*model.WorkspaceUsageHourly) error {
	if len(rows) == 0 {
		return nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	err = db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   workspaceUsageHourlyKey,
		DoUpdates: clause.AssignmentColumns(workspaceUsageHourlyValues),
	}).CreateInBatches(rows, workspaceUsageBatchSize).Error
	if err != nil {
		return fmt.Errorf("failed to upsert workspace usage hourly rows: %w", err)
	}
	return nil
}

// ListWorkspaceUsageHourly returns the hourly rows the filter selects, oldest first.
func (c *Client) ListWorkspaceUsageHourly(ctx context.Context,
	filter WorkspaceUsageFilter) ([]*model.WorkspaceUsageHourly, error) {
	if filter.isEmpty() {
		return nil, nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var rows []*model.WorkspaceUsageHourly
	err = filter.apply(db.WithContext(ctx), "hour").Order("hour").Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace usage hourly rows: %w", err)
	}
	return rows, nil
}

// ListWorkspaceUsageHours returns the distinct hours in [start, end) that have hourly rows.
func (c *Client) ListWorkspaceUsageHours(ctx context.Context, start, end time.Time) ([]time.Time, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var hours []time.Time
	err = WorkspaceUsageFilter{Start: start, End: end}.
		apply(db.WithContext(ctx).Model(&model.WorkspaceUsageHourly{}), "hour").
		Distinct("hour").Order("hour").Pluck("hour", &hours).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace usage hours: %w", err)
	}
	return hours, nil
}

// DeleteWorkspaceUsageHourlyBefore deletes the hourly rows older than before.
func (c *Client) DeleteWorkspaceUsageHourlyBefore(ctx context.Context, before time.Time) (int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return 0, err
	}
	result := db.WithContext(ctx).Where("hour < ?", before).Delete(&model.WorkspaceUsageHourly{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete workspace usage hourly rows: %w", result.Error)
	}
	return result.RowsAffected, nil
}
