// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameToolQualityStats = "tool_quality_stats"

// ToolQualityStats represents aggregated quality statistics for a tool
type ToolQualityStats struct {
	ID               int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ToolName         string     `gorm:"column:tool_name;not null;uniqueIndex" json:"tool_name"`
	TotalInvocations int64      `gorm:"column:total_invocations;default:0" json:"total_invocations"`
	SuccessCount     int64      `gorm:"column:success_count;default:0" json:"success_count"`
	FailureCount     int64      `gorm:"column:failure_count;default:0" json:"failure_count"`
	AvgDurationMs    int        `gorm:"column:avg_duration_ms" json:"avg_duration_ms"`
	P50DurationMs    int        `gorm:"column:p50_duration_ms" json:"p50_duration_ms"`
	P99DurationMs    int        `gorm:"column:p99_duration_ms" json:"p99_duration_ms"`
	ErrorRate        float64    `gorm:"column:error_rate" json:"error_rate"`
	LastInvokedAt    *time.Time `gorm:"column:last_invoked_at" json:"last_invoked_at"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the table name
func (*ToolQualityStats) TableName() string {
	return TableNameToolQualityStats
}
