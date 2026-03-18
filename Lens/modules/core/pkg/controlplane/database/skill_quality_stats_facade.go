// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SkillQualityStatsFacadeInterface defines the interface for SkillQualityStats operations
type SkillQualityStatsFacadeInterface interface {
	GetBySkillName(ctx context.Context, skillName string) (*model.SkillQualityStats, error)
	List(ctx context.Context, offset, limit int) ([]*model.SkillQualityStats, int64, error)
	GetTopByExecutions(ctx context.Context, limit int) ([]*model.SkillQualityStats, error)
	GetTopByRating(ctx context.Context, limit int) ([]*model.SkillQualityStats, error)
	Upsert(ctx context.Context, stats *model.SkillQualityStats) error
	IncrementExecution(ctx context.Context, skillName string, success bool, durationMs int) error
	UpdateRating(ctx context.Context, skillName string, avgRating float64) error
}

// SkillQualityStatsFacade implements SkillQualityStatsFacadeInterface
type SkillQualityStatsFacade struct {
	db *gorm.DB
}

// NewSkillQualityStatsFacade creates a new SkillQualityStatsFacade
func NewSkillQualityStatsFacade(db *gorm.DB) *SkillQualityStatsFacade {
	return &SkillQualityStatsFacade{db: db}
}

// GetBySkillName retrieves stats for a skill
func (f *SkillQualityStatsFacade) GetBySkillName(ctx context.Context, skillName string) (*model.SkillQualityStats, error) {
	var stats model.SkillQualityStats
	err := f.db.WithContext(ctx).Where("skill_name = ?", skillName).First(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

// List retrieves paginated stats
func (f *SkillQualityStatsFacade) List(ctx context.Context, offset, limit int) ([]*model.SkillQualityStats, int64, error) {
	var stats []*model.SkillQualityStats
	var total int64

	err := f.db.WithContext(ctx).Model(&model.SkillQualityStats{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = f.db.WithContext(ctx).
		Order("total_executions DESC").
		Offset(offset).
		Limit(limit).
		Find(&stats).Error
	if err != nil {
		return nil, 0, err
	}

	return stats, total, nil
}

// GetTopByExecutions retrieves top skills by execution count
func (f *SkillQualityStatsFacade) GetTopByExecutions(ctx context.Context, limit int) ([]*model.SkillQualityStats, error) {
	var stats []*model.SkillQualityStats
	err := f.db.WithContext(ctx).
		Order("total_executions DESC").
		Limit(limit).
		Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// GetTopByRating retrieves top skills by average rating
func (f *SkillQualityStatsFacade) GetTopByRating(ctx context.Context, limit int) ([]*model.SkillQualityStats, error) {
	var stats []*model.SkillQualityStats
	err := f.db.WithContext(ctx).
		Where("avg_rating IS NOT NULL").
		Order("avg_rating DESC").
		Limit(limit).
		Find(&stats).Error
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// Upsert creates or updates stats
func (f *SkillQualityStatsFacade) Upsert(ctx context.Context, stats *model.SkillQualityStats) error {
	stats.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "skill_name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"total_executions", "success_count", "failure_count",
				"avg_duration_ms", "avg_rating", "last_used_at", "updated_at",
			}),
		}).
		Create(stats).Error
}

// IncrementExecution increments execution counts
func (f *SkillQualityStatsFacade) IncrementExecution(ctx context.Context, skillName string, success bool, durationMs int) error {
	now := time.Now()

	// Try to get existing stats
	existing, err := f.GetBySkillName(ctx, skillName)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new stats
			stats := &model.SkillQualityStats{
				SkillName:       skillName,
				TotalExecutions: 1,
				AvgDurationMs:   durationMs,
				LastUsedAt:      &now,
			}
			if success {
				stats.SuccessCount = 1
			} else {
				stats.FailureCount = 1
			}
			return f.db.WithContext(ctx).Create(stats).Error
		}
		return err
	}

	// Update existing stats
	updates := map[string]interface{}{
		"total_executions": gorm.Expr("total_executions + 1"),
		"last_used_at":     now,
		"updated_at":       now,
	}

	if success {
		updates["success_count"] = gorm.Expr("success_count + 1")
	} else {
		updates["failure_count"] = gorm.Expr("failure_count + 1")
	}

	// Calculate new average duration
	if existing.TotalExecutions > 0 {
		newAvg := (existing.AvgDurationMs*int(existing.TotalExecutions) + durationMs) / int(existing.TotalExecutions+1)
		updates["avg_duration_ms"] = newAvg
	} else {
		updates["avg_duration_ms"] = durationMs
	}

	return f.db.WithContext(ctx).Model(&model.SkillQualityStats{}).
		Where("skill_name = ?", skillName).
		Updates(updates).Error
}

// UpdateRating updates the average rating
func (f *SkillQualityStatsFacade) UpdateRating(ctx context.Context, skillName string, avgRating float64) error {
	return f.db.WithContext(ctx).Model(&model.SkillQualityStats{}).
		Where("skill_name = ?", skillName).
		Updates(map[string]interface{}{
			"avg_rating": avgRating,
			"updated_at": time.Now(),
		}).Error
}
