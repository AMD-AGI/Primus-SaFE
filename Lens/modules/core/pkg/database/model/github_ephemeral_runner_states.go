// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

// GithubEphemeralRunnerStates stores raw K8s state of EphemeralRunner resources.
// This table acts as a lightweight mirror of the K8s state, decoupled from business logic.
// The RunnerStateProcessor reads from this table and drives workflow_run lifecycle and task creation.
type GithubEphemeralRunnerStates struct {
	ID              int64  `gorm:"column:id;primaryKey;autoIncrement:true" json:"id"`
	Namespace       string `gorm:"column:namespace;not null" json:"namespace"`
	Name            string `gorm:"column:name;not null" json:"name"`
	UID             string `gorm:"column:uid;not null;default:''" json:"uid"`
	RunnerSetName   string `gorm:"column:runner_set_name;not null;default:''" json:"runner_set_name"`
	RunnerType      string `gorm:"column:runner_type;not null;default:'unknown'" json:"runner_type"`
	Phase           string `gorm:"column:phase;not null;default:''" json:"phase"`
	GithubRunID     int64  `gorm:"column:github_run_id;not null;default:0" json:"github_run_id"`
	GithubJobID     int64  `gorm:"column:github_job_id;not null;default:0" json:"github_job_id"`
	GithubRunNumber int32  `gorm:"column:github_run_number;not null;default:0" json:"github_run_number"`
	WorkflowName    string `gorm:"column:workflow_name;not null;default:''" json:"workflow_name"`
	HeadSha         string `gorm:"column:head_sha;not null;default:''" json:"head_sha"`
	HeadBranch      string `gorm:"column:head_branch;not null;default:''" json:"head_branch"`
	Repository      string `gorm:"column:repository;not null;default:''" json:"repository"`
	PodPhase        string `gorm:"column:pod_phase;not null;default:''" json:"pod_phase"`
	PodCondition    string `gorm:"column:pod_condition;not null;default:''" json:"pod_condition"`
	PodMessage      string `gorm:"column:pod_message;not null;default:''" json:"pod_message"`

	// SaFE workload association
	SafeWorkloadID string `gorm:"column:safe_workload_id;not null;default:''" json:"safe_workload_id"` // Associated SaFE UnifiedJob workload name

	// Lifecycle flags
	IsCompleted bool `gorm:"column:is_completed;not null;default:false" json:"is_completed"`
	IsDeleted   bool `gorm:"column:is_deleted;not null;default:false" json:"is_deleted"`

	// K8s timestamps
	CreationTimestamp time.Time `gorm:"column:creation_timestamp" json:"creation_timestamp"`
	CompletionTime    time.Time `gorm:"column:completion_time" json:"completion_time"`
	DeletionTime      time.Time `gorm:"column:deletion_time" json:"deletion_time"`

	// Processing state (managed by RunnerStateProcessor)
	WorkflowRunID   int64     `gorm:"column:workflow_run_id;not null;default:0" json:"workflow_run_id"`
	RunSummaryID    int64     `gorm:"column:run_summary_id;not null;default:0" json:"run_summary_id"`
	LastStatus      string    `gorm:"column:last_status;not null;default:''" json:"last_status"`
	LastProcessedAt time.Time `gorm:"column:last_processed_at" json:"last_processed_at"`

	// Timestamps
	CreatedAt time.Time `gorm:"column:created_at;not null;default:now()" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:now()" json:"updated_at"`
}

// TableName returns the table name
func (GithubEphemeralRunnerStates) TableName() string {
	return "github_ephemeral_runner_states"
}
