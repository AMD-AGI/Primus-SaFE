// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"time"
)

const TableNameDashboardSummaries = "dashboard_summaries"

// DashboardSummaries represents pre-aggregated Dashboard summary data
type DashboardSummaries struct {
	ID                       int64     `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	ConfigID                 int64     `gorm:"column:config_id;not null" json:"config_id"`
	SummaryDate              time.Time `gorm:"column:summary_date;not null;type:date" json:"summary_date"`
	CurrentRunID             *int64    `gorm:"column:current_run_id" json:"current_run_id"`
	PreviousRunID            *int64    `gorm:"column:previous_run_id" json:"previous_run_id"`
	BuildStatus              string    `gorm:"column:build_status" json:"build_status"`
	BuildDurationSeconds     *int      `gorm:"column:build_duration_seconds" json:"build_duration_seconds"`
	CommitCount              int       `gorm:"column:commit_count;not null;default:0" json:"commit_count"`
	PRCount                  int       `gorm:"column:pr_count;not null;default:0" json:"pr_count"`
	ContributorCount         int       `gorm:"column:contributor_count;not null;default:0" json:"contributor_count"`
	TotalAdditions           int       `gorm:"column:total_additions;not null;default:0" json:"total_additions"`
	TotalDeletions           int       `gorm:"column:total_deletions;not null;default:0" json:"total_deletions"`
	OverallPerfChangePercent *float64  `gorm:"column:overall_perf_change_percent;type:decimal(10,2)" json:"overall_perf_change_percent"`
	RegressionCount          int       `gorm:"column:regression_count;not null;default:0" json:"regression_count"`
	ImprovementCount         int       `gorm:"column:improvement_count;not null;default:0" json:"improvement_count"`
	NewMetricCount           int       `gorm:"column:new_metric_count;not null;default:0" json:"new_metric_count"`
	TopImprovements          ExtJSON   `gorm:"column:top_improvements;not null;default:[]" json:"top_improvements"`
	TopRegressions           ExtJSON   `gorm:"column:top_regressions;not null;default:[]" json:"top_regressions"`
	TopContributors          ExtJSON   `gorm:"column:top_contributors;not null;default:[]" json:"top_contributors"`
	ActiveAlerts             ExtJSON   `gorm:"column:active_alerts;not null;default:[]" json:"active_alerts"`
	GeneratedAt              time.Time `gorm:"column:generated_at;not null;default:now()" json:"generated_at"`
	ExpiresAt                *time.Time `gorm:"column:expires_at" json:"expires_at"`
	IsStale                  bool      `gorm:"column:is_stale;not null;default:false" json:"is_stale"`
	CreatedAt                time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt                time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

func (*DashboardSummaries) TableName() string {
	return TableNameDashboardSummaries
}

// PerformanceChange represents a performance change entry for improvements/regressions
type PerformanceChange struct {
	Metric        string                 `json:"metric"`
	Dimensions    map[string]interface{} `json:"dimensions,omitempty"`
	CurrentValue  float64                `json:"current_value"`
	PreviousValue float64                `json:"previous_value"`
	ChangePercent float64                `json:"change_percent"`
	Unit          string                 `json:"unit,omitempty"`
	LikelyCommit  *CommitSummary         `json:"likely_commit,omitempty"`
}

// CommitSummary represents a brief commit summary
type CommitSummary struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Message string `json:"message"`
}

// ContributorSummary represents a contributor's summary
type ContributorSummary struct {
	Author    string `json:"author"`
	Email     string `json:"email,omitempty"`
	Commits   int    `json:"commits"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	PRs       int    `json:"prs,omitempty"`
}

// AlertInfo represents an alert entry
type AlertInfo struct {
	Type     string `json:"type"`
	Metric   string `json:"metric,omitempty"`
	Severity string `json:"severity"`
	Message  string `json:"message,omitempty"`
}
