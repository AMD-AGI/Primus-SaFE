/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import "time"

const (
	AnnotationRunID      = "actions.github.com/run-id"
	AnnotationRunNumber  = "actions.github.com/run-number"
	AnnotationJobID      = "actions.github.com/job-id"
	AnnotationWorkflow   = "actions.github.com/workflow"
	AnnotationRepository = "actions.github.com/repository"
	AnnotationBranch     = "actions.github.com/branch"
	AnnotationSHA        = "actions.github.com/sha"
)

type WorkflowRunRecord struct {
	ID               int64      `db:"id"`
	WorkloadID       string     `db:"workload_id"`
	Cluster          string     `db:"cluster"`
	GithubRunID      int64      `db:"github_run_id"`
	GithubJobID      int64      `db:"github_job_id"`
	WorkflowName     string     `db:"workflow_name"`
	GithubOwner      string     `db:"github_owner"`
	GithubRepo       string     `db:"github_repo"`
	HeadBranch       string     `db:"head_branch"`
	HeadSHA          string     `db:"head_sha"`
	Status           string     `db:"status"`
	Conclusion       string     `db:"conclusion"`
	CollectionStatus string     `db:"collection_status"`
	SyncStatus       string     `db:"sync_status"`
	ConfigID         *int64     `db:"config_id"`
	StartedAt        *time.Time `db:"started_at"`
	CompletedAt      *time.Time `db:"completed_at"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at"`
}

type EphemeralRunnerMeta struct {
	GithubRunID  int64
	GithubJobID  int64
	WorkflowName string
	Repository   string
	Branch       string
	SHA          string
	Owner        string
	Repo         string
}
