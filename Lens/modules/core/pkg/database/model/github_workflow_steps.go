// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

const TableNameGithubWorkflowSteps = "github_workflow_steps"

// GithubWorkflowSteps represents a step within a GitHub Actions job
type GithubWorkflowSteps struct {
	ID              int64      `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	JobID           int64      `gorm:"column:job_id;not null" json:"job_id"`
	StepNumber      int        `gorm:"column:step_number;not null" json:"step_number"`
	Name            string     `gorm:"column:name;not null" json:"name"`
	Status          string     `gorm:"column:status" json:"status"`
	Conclusion      string     `gorm:"column:conclusion" json:"conclusion"`
	StartedAt       *time.Time `gorm:"column:started_at" json:"started_at"`
	CompletedAt     *time.Time `gorm:"column:completed_at" json:"completed_at"`
	DurationSeconds int        `gorm:"column:duration_seconds" json:"duration_seconds"`
	CreatedAt       time.Time  `gorm:"column:created_at;default:now()" json:"created_at"`
}

func (*GithubWorkflowSteps) TableName() string {
	return TableNameGithubWorkflowSteps
}
