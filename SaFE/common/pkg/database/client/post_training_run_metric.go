/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TPostTrainingRunMetric = "post_training_run_metric"

	// PostTrainingSystemMetricPrefix marks the launcher's GPU series. Their step is seconds since the run first
	// entered training, so they are kept when a resumed attempt drops points after its resume step.
	PostTrainingSystemMetricPrefix = "system/"

	postTrainingMetricBatchSize = 1000
)

// PostTrainingRunMetric is one point of a run's metric series; (run_id, name, step) is unique.
type PostTrainingRunMetric struct {
	ID       int64     `gorm:"column:id;primaryKey;autoIncrement" json:"-" db:"id"`
	RunID    string    `gorm:"column:run_id" json:"runId" db:"run_id"`
	Name     string    `gorm:"column:name" json:"name" db:"name"`
	Step     int       `gorm:"column:step" json:"step" db:"step"`
	Value    float64   `gorm:"column:value" json:"value" db:"value"`
	WallTime time.Time `gorm:"column:wall_time" json:"wallTime" db:"wall_time"`
}

// TableName sets the SQL table name for PostTrainingRunMetric.
func (PostTrainingRunMetric) TableName() string {
	return TPostTrainingRunMetric
}

// postTrainingMetricKey identifies one point within a run.
type postTrainingMetricKey struct {
	name string
	step int
}

// SavePostTrainingRunMetrics stores points under runID and sets runFields on the run in one transaction, so a
// batch of points and the advanced log cursor are saved together or not at all. A point written again keeps
// the newer value. It returns gorm.ErrRecordNotFound, and saves nothing, when runFields is set but the run
// does not exist.
func (c *Client) SavePostTrainingRunMetrics(
	ctx context.Context,
	runID string,
	points []*PostTrainingRunMetric,
	runFields map[string]interface{},
) error {
	points = dedupePostTrainingRunMetrics(runID, points)
	if len(points) == 0 && len(runFields) == 0 {
		return nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(points) > 0 {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "run_id"}, {Name: "name"}, {Name: "step"}},
				DoUpdates: clause.AssignmentColumns([]string{"value", "wall_time"}),
			}).CreateInBatches(points, postTrainingMetricBatchSize).Error; err != nil {
				return err
			}
		}
		if len(runFields) > 0 {
			return updatePostTrainingRun(tx, runID, runFields)
		}
		return nil
	})
}

// DeletePostTrainingRunMetricsAfterStep removes the points after step, except the system series, when a new
// attempt resumes from a checkpoint at that step.
func (c *Client) DeletePostTrainingRunMetricsAfterStep(ctx context.Context, runID string, step int) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).
		Where("run_id = ? AND step > ? AND name NOT LIKE ?", runID, step, PostTrainingSystemMetricPrefix+"%").
		Delete(&PostTrainingRunMetric{}).Error
}

// ListPostTrainingRunMetrics returns a run's points for the given names, or for all names when names is
// empty, ordered by name and step.
func (c *Client) ListPostTrainingRunMetrics(
	ctx context.Context,
	runID string,
	names []string,
) ([]*PostTrainingRunMetric, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	query := db.WithContext(ctx).Where("run_id = ?", runID)
	if len(names) > 0 {
		query = query.Where("name IN ?", names)
	}
	var points []*PostTrainingRunMetric
	if err := query.Order("name ASC, step ASC").Find(&points).Error; err != nil {
		return nil, err
	}
	return points, nil
}

// ListPostTrainingRunMetricNames returns the distinct metric names a run has points for, sorted.
func (c *Client) ListPostTrainingRunMetricNames(ctx context.Context, runID string) ([]string, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	var names []string
	if err := db.WithContext(ctx).
		Model(&PostTrainingRunMetric{}).
		Where("run_id = ?", runID).
		Distinct("name").
		Order("name ASC").
		Pluck("name", &names).Error; err != nil {
		return nil, err
	}
	return names, nil
}

// DeletePostTrainingRunMetrics removes all points of a run.
func (c *Client) DeletePostTrainingRunMetrics(ctx context.Context, runID string) error {
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Where("run_id = ?", runID).Delete(&PostTrainingRunMetric{}).Error
}

// dedupePostTrainingRunMetrics assigns runID to every point and keeps the last point for each (name, step),
// since a single upsert statement cannot update the same row twice.
func dedupePostTrainingRunMetrics(runID string, points []*PostTrainingRunMetric) []*PostTrainingRunMetric {
	index := make(map[postTrainingMetricKey]int, len(points))
	out := make([]*PostTrainingRunMetric, 0, len(points))
	for _, p := range points {
		if p == nil {
			continue
		}
		p.RunID = runID
		key := postTrainingMetricKey{name: p.Name, step: p.Step}
		if i, ok := index[key]; ok {
			out[i] = p
			continue
		}
		index[key] = len(out)
		out = append(out, p)
	}
	return out
}
