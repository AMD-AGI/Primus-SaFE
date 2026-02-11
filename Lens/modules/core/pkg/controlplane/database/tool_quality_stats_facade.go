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

// ToolQualityStatsFacadeInterface defines the interface for ToolQualityStats operations
type ToolQualityStatsFacadeInterface interface {
	GetByToolName(ctx context.Context, toolName string) (*model.ToolQualityStats, error)
	GetAll(ctx context.Context, offset, limit int) ([]*model.ToolQualityStats, int64, error)
	Upsert(ctx context.Context, stats *model.ToolQualityStats) error
	IncrementInvocation(ctx context.Context, toolName string, success bool, durationMs int) error
	Delete(ctx context.Context, toolName string) error
}

// ToolQualityStatsFacade implements ToolQualityStatsFacadeInterface
type ToolQualityStatsFacade struct {
	db *gorm.DB
}

// NewToolQualityStatsFacade creates a new ToolQualityStatsFacade
func NewToolQualityStatsFacade(db *gorm.DB) *ToolQualityStatsFacade {
	return &ToolQualityStatsFacade{db: db}
}

// GetByToolName retrieves quality stats for a tool
func (f *ToolQualityStatsFacade) GetByToolName(ctx context.Context, toolName string) (*model.ToolQualityStats, error) {
	var stats model.ToolQualityStats
	err := f.db.WithContext(ctx).Where("tool_name = ?", toolName).First(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

// GetAll retrieves all tool quality stats
func (f *ToolQualityStatsFacade) GetAll(ctx context.Context, offset, limit int) ([]*model.ToolQualityStats, int64, error) {
	var stats []*model.ToolQualityStats
	var total int64

	err := f.db.WithContext(ctx).Model(&model.ToolQualityStats{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = f.db.WithContext(ctx).
		Order("total_invocations DESC").
		Offset(offset).
		Limit(limit).
		Find(&stats).Error
	if err != nil {
		return nil, 0, err
	}

	return stats, total, nil
}

// Upsert creates or updates tool quality stats
func (f *ToolQualityStatsFacade) Upsert(ctx context.Context, stats *model.ToolQualityStats) error {
	stats.UpdatedAt = time.Now()
	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tool_name"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"total_invocations", "success_count", "failure_count",
				"avg_duration_ms", "p50_duration_ms", "p99_duration_ms",
				"error_rate", "last_invoked_at", "updated_at",
			}),
		}).
		Create(stats).Error
}

// IncrementInvocation increments the invocation count and updates stats
func (f *ToolQualityStatsFacade) IncrementInvocation(ctx context.Context, toolName string, success bool, durationMs int) error {
	now := time.Now()
	
	// Try to get existing stats
	existing, err := f.GetByToolName(ctx, toolName)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// Create new stats
			stats := &model.ToolQualityStats{
				ToolName:         toolName,
				TotalInvocations: 1,
				AvgDurationMs:    durationMs,
				P50DurationMs:    durationMs,
				P99DurationMs:    durationMs,
				LastInvokedAt:    &now,
			}
			if success {
				stats.SuccessCount = 1
				stats.ErrorRate = 0
			} else {
				stats.FailureCount = 1
				stats.ErrorRate = 1
			}
			return f.db.WithContext(ctx).Create(stats).Error
		}
		return err
	}

	// Update existing stats
	existing.TotalInvocations++
	if success {
		existing.SuccessCount++
	} else {
		existing.FailureCount++
	}
	existing.ErrorRate = float64(existing.FailureCount) / float64(existing.TotalInvocations)
	
	// Update average duration (simple moving average)
	existing.AvgDurationMs = int((int64(existing.AvgDurationMs)*(existing.TotalInvocations-1) + int64(durationMs)) / existing.TotalInvocations)
	existing.LastInvokedAt = &now
	existing.UpdatedAt = now

	return f.db.WithContext(ctx).Save(existing).Error
}

// Delete deletes quality stats for a tool
func (f *ToolQualityStatsFacade) Delete(ctx context.Context, toolName string) error {
	return f.db.WithContext(ctx).Where("tool_name = ?", toolName).Delete(&model.ToolQualityStats{}).Error
}
