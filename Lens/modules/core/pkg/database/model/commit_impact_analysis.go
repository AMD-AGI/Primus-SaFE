// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameCommitImpactAnalysis = "commit_impact_analysis"

// CommitImpactAnalysis represents commit impact analysis results
type CommitImpactAnalysis struct {
	ID              int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ConfigID        int64     `gorm:"column:config_id;not null" json:"config_id"`
	RunID           int64     `gorm:"column:run_id;not null" json:"run_id"`
	CommitSHA       string    `gorm:"column:commit_sha;not null" json:"commit_sha"`
	AuthorName      string    `gorm:"column:author_name" json:"author_name"`
	CommitMessage   string    `gorm:"column:commit_message" json:"commit_message"`
	FilesChanged    ExtJSON   `gorm:"column:files_changed" json:"files_changed"`
	ImpactedMetrics ExtJSON   `gorm:"column:impacted_metrics;not null;default:[]" json:"impacted_metrics"`
	ImpactScore     *float64  `gorm:"column:impact_score;type:decimal(5,2)" json:"impact_score"`
	IsLikelyCause   bool      `gorm:"column:is_likely_cause;not null;default:false" json:"is_likely_cause"`
	AnalyzedAt      time.Time `gorm:"column:analyzed_at;not null;default:now()" json:"analyzed_at"`
	CreatedAt       time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
}

func (*CommitImpactAnalysis) TableName() string {
	return TableNameCommitImpactAnalysis
}

// FileChangeInfo represents a changed file with stats
type FileChangeInfo struct {
	Path      string `json:"path"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Status    string `json:"status,omitempty"` // added, modified, deleted
}

// MetricImpact represents the impact on a specific metric
type MetricImpact struct {
	Metric        string  `json:"metric"`
	ChangePercent float64 `json:"change_pct"`
	Confidence    float64 `json:"confidence"`
	Reason        string  `json:"reason,omitempty"`
}
