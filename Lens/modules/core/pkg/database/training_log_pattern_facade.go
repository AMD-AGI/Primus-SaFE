// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// TrainingLogPatternFacadeInterface defines operations for the training_log_pattern table
type TrainingLogPatternFacadeInterface interface {
	// ListEnabledByType returns all enabled patterns of a given type, ordered by priority DESC
	ListEnabledByType(ctx context.Context, patternType string) ([]*model.TrainingLogPattern, error)

	// ListAllEnabled returns all enabled patterns, ordered by pattern_type then priority DESC
	ListAllEnabled(ctx context.Context) ([]*model.TrainingLogPattern, error)

	// ListBySourceWorkload returns patterns discovered from a specific workload
	ListBySourceWorkload(ctx context.Context, workloadUID string) ([]*model.TrainingLogPattern, error)

	// Create inserts a new pattern
	Create(ctx context.Context, p *model.TrainingLogPattern) error

	// Upsert inserts or updates (by pattern_type + md5(pattern) unique constraint)
	Upsert(ctx context.Context, p *model.TrainingLogPattern) error

	// UpdateHitCount increments hit_count and sets last_hit_at
	UpdateHitCount(ctx context.Context, id int64) error

	// GetChangedSince returns patterns updated since a given timestamp (for reload detection)
	GetChangedSince(ctx context.Context, since time.Time) ([]*model.TrainingLogPattern, error)

	// HasChangesSince returns true if any patterns were updated since the given timestamp
	HasChangesSince(ctx context.Context, since time.Time) (bool, error)
}

// TrainingLogPatternFacade implements TrainingLogPatternFacadeInterface
type TrainingLogPatternFacade struct {
	BaseFacade
}

// NewTrainingLogPatternFacade creates a new facade
func NewTrainingLogPatternFacade() TrainingLogPatternFacadeInterface {
	return &TrainingLogPatternFacade{}
}

func (f *TrainingLogPatternFacade) ListEnabledByType(ctx context.Context, patternType string) ([]*model.TrainingLogPattern, error) {
	var results []*model.TrainingLogPattern
	err := f.getDB().WithContext(ctx).
		Where("enabled = ? AND pattern_type = ?", true, patternType).
		Order("priority DESC, id ASC").
		Find(&results).Error
	return results, err
}

func (f *TrainingLogPatternFacade) ListAllEnabled(ctx context.Context) ([]*model.TrainingLogPattern, error) {
	var results []*model.TrainingLogPattern
	err := f.getDB().WithContext(ctx).
		Where("enabled = ?", true).
		Order("pattern_type, priority DESC, id ASC").
		Find(&results).Error
	return results, err
}

func (f *TrainingLogPatternFacade) ListBySourceWorkload(ctx context.Context, workloadUID string) ([]*model.TrainingLogPattern, error) {
	var results []*model.TrainingLogPattern
	err := f.getDB().WithContext(ctx).
		Where("source_workload_uid = ?", workloadUID).
		Find(&results).Error
	return results, err
}

func (f *TrainingLogPatternFacade) Create(ctx context.Context, p *model.TrainingLogPattern) error {
	return f.getDB().WithContext(ctx).Create(p).Error
}

func (f *TrainingLogPatternFacade) Upsert(ctx context.Context, p *model.TrainingLogPattern) error {
	// Use raw SQL for ON CONFLICT with md5 expression index
	return f.getDB().WithContext(ctx).Exec(`
		INSERT INTO training_log_pattern
			(pattern, pattern_type, event_subtype, source, source_workload_uid, framework,
			 name, description, sample_line, enabled, priority, confidence, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())
		ON CONFLICT (pattern_type, md5(pattern)) DO UPDATE SET
			sample_line = EXCLUDED.sample_line,
			source_workload_uid = COALESCE(EXCLUDED.source_workload_uid, training_log_pattern.source_workload_uid),
			updated_at = NOW()
	`, p.Pattern, p.PatternType, p.EventSubtype, p.Source, p.SourceWorkloadUID, p.Framework,
		p.Name, p.Description, p.SampleLine, p.Enabled, p.Priority, p.Confidence).Error
}

func (f *TrainingLogPatternFacade) UpdateHitCount(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).Exec(`
		UPDATE training_log_pattern SET hit_count = hit_count + 1, last_hit_at = NOW() WHERE id = ?
	`, id).Error
}

func (f *TrainingLogPatternFacade) GetChangedSince(ctx context.Context, since time.Time) ([]*model.TrainingLogPattern, error) {
	var results []*model.TrainingLogPattern
	err := f.getDB().WithContext(ctx).
		Where("updated_at > ?", since).
		Find(&results).Error
	return results, err
}

func (f *TrainingLogPatternFacade) HasChangesSince(ctx context.Context, since time.Time) (bool, error) {
	var count int64
	err := f.getDB().WithContext(ctx).
		Model(&model.TrainingLogPattern{}).
		Where("updated_at > ?", since).
		Count(&count).Error
	return count > 0, err
}
