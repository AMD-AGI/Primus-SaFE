/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"context"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/klog/v2"
)

const (
	TPostTrainingRunMetric = "post_training_run_metric"

	postTrainingMetricBatchSize = 1000
	// maxPostTrainingMetricNameBytes bounds a metric name well below the ~2.7 KB a btree index entry may take,
	// as the name is part of the unique (run_id, name, step) index and one over-long entry fails the whole insert.
	maxPostTrainingMetricNameBytes = 1024
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
// batch of points and the advanced log cursor are saved together or not at all. When dropAfterStep is set, the
// points after that step, which an attempt resumed from a checkpoint at that step writes again, are deleted
// first in the same transaction. A point written again keeps the newer value. It returns
// gorm.ErrRecordNotFound, and saves nothing, when runFields is set but the run does not exist.
func (c *Client) SavePostTrainingRunMetrics(
	ctx context.Context,
	runID string,
	points []*PostTrainingRunMetric,
	runFields map[string]interface{},
	dropAfterStep *int,
) error {
	points = dedupePostTrainingRunMetrics(runID, points)
	if len(points) == 0 && len(runFields) == 0 && dropAfterStep == nil {
		return nil
	}
	db, err := c.GetGormDB()
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if dropAfterStep != nil {
			if err := tx.Where("run_id = ? AND step > ?", runID, *dropAfterStep).
				Delete(&PostTrainingRunMetric{}).Error; err != nil {
				return err
			}
		}
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

// CountPostTrainingRunMetricsAfterStep counts the points after step, which a new attempt resumed from a
// checkpoint at that step replaces.
func (c *Client) CountPostTrainingRunMetricsAfterStep(ctx context.Context, runID string, step int) (int64, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return 0, err
	}
	var n int64
	err = db.WithContext(ctx).Model(&PostTrainingRunMetric{}).
		Where("run_id = ? AND step > ?", runID, step).Count(&n).Error
	return n, err
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

// ListLatestPostTrainingRunMetrics returns at most limit points of a run, or of its named metrics, by name and
// step, which bounds what a curve request reads of a run with very many points. A run within the limit is read
// whole. Past it each series keeps its newest points, as many as seriesQuota gives it: the steps of the series are
// not on one axis, so the newest points of the run as a whole would be those of the series that counts in the
// largest numbers.
func (c *Client) ListLatestPostTrainingRunMetrics(ctx context.Context, runID string, names []string,
	limit int) ([]*PostTrainingRunMetric, error) {
	db, err := c.GetGormDB()
	if err != nil {
		return nil, err
	}
	// selected returns a query of the run's points, or of those of its named metrics, that selects columns.
	selected := func(columns string) *gorm.DB {
		query := db.WithContext(ctx).Model(&PostTrainingRunMetric{}).Select(columns).Where("run_id = ?", runID)
		if len(names) > 0 {
			query = query.Where("name IN ?", names)
		}
		return query
	}
	var series []struct {
		Name   string
		Points int64
	}
	if err := selected("name, COUNT(*) AS points").Group("name").Scan(&series).Error; err != nil {
		return nil, err
	}
	counts := make([]int64, len(series))
	for i, s := range series {
		counts[i] = s.Points
	}
	quota, cut := seriesQuota(counts, limit)
	if !cut {
		return c.ListPostTrainingRunMetrics(ctx, runID, names)
	}
	ranked := selected("*, ROW_NUMBER() OVER (PARTITION BY name ORDER BY step DESC) AS rn")
	var points []*PostTrainingRunMetric
	if err := db.WithContext(ctx).Table("(?) AS latest", ranked).Where("rn <= ?", quota).
		Order("name ASC, step ASC").Find(&points).Error; err != nil {
		return nil, err
	}
	return points, nil
}

// seriesQuota tells whether series of counts points are more than limit together and, if so, how many of its
// newest points each may keep: a series with fewer keeps all of its points, and what those leave of the limit is
// shared evenly among the longer ones. Every series keeps at least one point.
func seriesQuota(counts []int64, limit int) (int64, bool) {
	sorted := slices.Clone(counts)
	slices.Sort(sorted)
	left := int64(limit)
	for i, n := range sorted {
		longer := int64(len(sorted) - i)
		if n*longer > left {
			return max(1, left/longer), true
		}
		left -= n
	}
	return 0, false
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
// since a single upsert statement cannot update the same row twice. It drops the points of names too long to
// index, logging each such name once, so that one of them cannot fail the batch, and with it the cursor saved in
// the same transaction, on every retry.
func dedupePostTrainingRunMetrics(runID string, points []*PostTrainingRunMetric) []*PostTrainingRunMetric {
	index := make(map[postTrainingMetricKey]int, len(points))
	out := make([]*PostTrainingRunMetric, 0, len(points))
	var dropped map[string]bool
	for _, p := range points {
		if p == nil {
			continue
		}
		if len(p.Name) > maxPostTrainingMetricNameBytes {
			if !dropped[p.Name] {
				if dropped == nil {
					dropped = map[string]bool{}
				}
				dropped[p.Name] = true
				klog.Warningf("post training run %s: dropping the points of metric %.80q..., whose name is %d "+
					"bytes long, over the limit of %d", runID, p.Name, len(p.Name), maxPostTrainingMetricNameBytes)
			}
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
