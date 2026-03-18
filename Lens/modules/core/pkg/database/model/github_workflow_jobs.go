// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

const TableNameGithubWorkflowJobs = "github_workflow_jobs"

// GithubWorkflowJobs represents a job within a GitHub Actions workflow run
type GithubWorkflowJobs struct {
	ID              int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	RunID           int64      `gorm:"column:run_id;not null" json:"run_id"`
	GithubJobID     int64      `gorm:"column:github_job_id;not null" json:"github_job_id"`
	Name            string     `gorm:"column:name;not null" json:"name"`
	Needs           string     `gorm:"column:needs" json:"needs"` // JSON array of job names this job depends on
	Status          string     `gorm:"column:status" json:"status"`
	Conclusion      string     `gorm:"column:conclusion" json:"conclusion"`
	StartedAt       *time.Time `gorm:"column:started_at" json:"started_at"`
	CompletedAt     *time.Time `gorm:"column:completed_at" json:"completed_at"`
	DurationSeconds int        `gorm:"column:duration_seconds" json:"duration_seconds"`
	RunnerID        int64      `gorm:"column:runner_id" json:"runner_id"`
	RunnerName      string     `gorm:"column:runner_name" json:"runner_name"`
	RunnerGroupName string     `gorm:"column:runner_group_name" json:"runner_group_name"`
	StepsCount      int        `gorm:"column:steps_count;default:0" json:"steps_count"`
	StepsCompleted  int        `gorm:"column:steps_completed;default:0" json:"steps_completed"`
	StepsFailed     int        `gorm:"column:steps_failed;default:0" json:"steps_failed"`
	HTMLURL         string     `gorm:"column:html_url" json:"html_url"`
	CreatedAt       time.Time  `gorm:"column:created_at;default:now()" json:"created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;default:now()" json:"updated_at"`
}

func (*GithubWorkflowJobs) TableName() string {
	return TableNameGithubWorkflowJobs
}
