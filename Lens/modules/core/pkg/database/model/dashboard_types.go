// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

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
