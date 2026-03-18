// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameSkillQualityStats = "skill_quality_stats"

// SkillQualityStats represents aggregated quality statistics for a skill
type SkillQualityStats struct {
	ID              int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	SkillName       string     `gorm:"column:skill_name;not null;uniqueIndex" json:"skill_name"`
	TotalExecutions int64      `gorm:"column:total_executions;default:0" json:"total_executions"`
	SuccessCount    int64      `gorm:"column:success_count;default:0" json:"success_count"`
	FailureCount    int64      `gorm:"column:failure_count;default:0" json:"failure_count"`
	AvgDurationMs   int        `gorm:"column:avg_duration_ms" json:"avg_duration_ms"`
	AvgRating       float64    `gorm:"column:avg_rating;type:numeric(3,2)" json:"avg_rating"`
	LastUsedAt      *time.Time `gorm:"column:last_used_at" json:"last_used_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*SkillQualityStats) TableName() string {
	return TableNameSkillQualityStats
}
