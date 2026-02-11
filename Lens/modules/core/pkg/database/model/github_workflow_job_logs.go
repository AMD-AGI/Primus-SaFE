// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

const TableNameGithubWorkflowJobLogs = "github_workflow_job_logs"

// FetchStatus constants
const (
	LogFetchStatusPending = "pending"
	LogFetchStatusFetched = "fetched"
	LogFetchStatusFailed  = "failed"
	LogFetchStatusExpired = "expired"
)

// GithubWorkflowJobLogs stores locally cached job logs
type GithubWorkflowJobLogs struct {
	ID            int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RunID         int64      `gorm:"column:run_id;not null" json:"run_id"`
	GithubJobID   int64      `gorm:"column:github_job_id;not null" json:"github_job_id"`
	JobName       string     `gorm:"column:job_name;not null" json:"job_name"`
	Logs          string     `gorm:"column:logs" json:"logs,omitempty"`
	LogsSizeBytes int        `gorm:"column:logs_size_bytes;default:0" json:"logs_size_bytes"`
	FetchedAt     *time.Time `gorm:"column:fetched_at" json:"fetched_at"`
	FetchStatus   string     `gorm:"column:fetch_status;default:pending" json:"fetch_status"`
	FetchError    string     `gorm:"column:fetch_error" json:"fetch_error,omitempty"`
	CreatedAt     time.Time  `gorm:"column:created_at;default:now()" json:"created_at"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;default:now()" json:"updated_at"`
}

func (*GithubWorkflowJobLogs) TableName() string {
	return TableNameGithubWorkflowJobLogs
}
