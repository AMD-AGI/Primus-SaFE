/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"database/sql"
	"time"

	"k8s.io/klog/v2"
)

// Store handles persistence of GitHub workflow data in SaFE DB.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// UpsertWorkflowRun creates or updates a github_workflow_runs record.
// Uses github_run_id + workload_id as the logical key.
func (s *Store) UpsertWorkflowRun(ctx context.Context, run *WorkflowRunRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_workflow_runs
			(workload_id, cluster, github_run_id, github_job_id, workflow_name,
			 github_owner, github_repo, head_branch, head_sha, status, started_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
		ON CONFLICT (github_run_id) WHERE github_run_id > 0
		DO UPDATE SET
			status = CASE WHEN EXCLUDED.status != '' THEN EXCLUDED.status ELSE github_workflow_runs.status END,
			github_job_id = CASE WHEN EXCLUDED.github_job_id > 0 THEN EXCLUDED.github_job_id ELSE github_workflow_runs.github_job_id END,
			workflow_name = COALESCE(NULLIF(EXCLUDED.workflow_name, ''), github_workflow_runs.workflow_name),
			head_branch = COALESCE(NULLIF(EXCLUDED.head_branch, ''), github_workflow_runs.head_branch),
			head_sha = COALESCE(NULLIF(EXCLUDED.head_sha, ''), github_workflow_runs.head_sha),
			updated_at = NOW()`,
		run.WorkloadID, run.Cluster, run.GithubRunID, run.GithubJobID,
		run.WorkflowName, run.GithubOwner, run.GithubRepo,
		run.HeadBranch, run.HeadSHA, run.Status, run.StartedAt,
	)
	if err != nil {
		klog.V(1).Infof("[github-store] upsert workflow run %d: %v", run.GithubRunID, err)
	}
	return err
}

// MarkRunCompleted updates a workflow run's status to completed.
func (s *Store) MarkRunCompleted(ctx context.Context, githubRunID int64, conclusion string, completedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_workflow_runs
		SET status = 'completed', conclusion = $2, completed_at = $3, sync_status = 'pending', updated_at = NOW()
		WHERE github_run_id = $1`,
		githubRunID, conclusion, completedAt)
	return err
}

// GetUnsyncedRuns returns runs that need GitHub API sync.
func (s *Store) GetUnsyncedRuns(ctx context.Context, limit int) ([]WorkflowRunRecord, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, workload_id, cluster, github_run_id, github_job_id, workflow_name,
		       github_owner, github_repo, head_branch, head_sha, status, sync_status
		FROM github_workflow_runs
		WHERE sync_status = 'pending' AND github_run_id > 0
		ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []WorkflowRunRecord
	for rows.Next() {
		var r WorkflowRunRecord
		rows.Scan(&r.ID, &r.WorkloadID, &r.Cluster, &r.GithubRunID, &r.GithubJobID,
			&r.WorkflowName, &r.GithubOwner, &r.GithubRepo, &r.HeadBranch, &r.HeadSHA,
			&r.Status, &r.SyncStatus)
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// MarkSynced updates a run's sync status.
func (s *Store) MarkSynced(ctx context.Context, runID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE github_workflow_runs SET sync_status = 'synced', updated_at = NOW() WHERE id = $1`, runID)
	return err
}

// UpsertRunDetails inserts or updates run details from GitHub API.
func (s *Store) UpsertRunDetails(ctx context.Context, runID int, githubRunID int64,
	htmlURL, jobsURL, logsURL, event, triggerActor string, prNumber int, workflowPath string, rawData []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_workflow_run_details
			(run_id, github_run_id, html_url, jobs_url, logs_url, event, trigger_actor, pull_request_number, workflow_path, raw_data, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (github_run_id) WHERE github_run_id > 0
		DO UPDATE SET html_url=EXCLUDED.html_url, event=EXCLUDED.event, trigger_actor=EXCLUDED.trigger_actor,
			raw_data=EXCLUDED.raw_data, updated_at=NOW()`,
		runID, githubRunID, htmlURL, jobsURL, logsURL, event, triggerActor, prNumber, workflowPath, rawData)
	return err
}

// UpsertJob inserts or updates a GitHub workflow job.
func (s *Store) UpsertJob(ctx context.Context, runID int, githubJobID int64,
	name, status, conclusion, runnerName, runnerGroupName string,
	startedAt, completedAt *time.Time, needs []string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_workflow_jobs
			(run_id, github_job_id, name, status, conclusion, started_at, completed_at, runner_name, runner_group_name, needs)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (github_job_id)
		DO UPDATE SET status=EXCLUDED.status, conclusion=EXCLUDED.conclusion,
			completed_at=EXCLUDED.completed_at, runner_name=EXCLUDED.runner_name`,
		runID, githubJobID, name, status, conclusion, startedAt, completedAt, runnerName, runnerGroupName, needs)
	return err
}

// InsertMetrics inserts a workflow metrics record.
func (s *Store) InsertMetrics(ctx context.Context, configID, runID int64,
	timestamp *time.Time, dimensions, metrics, rawData []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_workflow_metrics (config_id, run_id, timestamp, dimensions, metrics, raw_data)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		configID, runID, timestamp, dimensions, metrics, rawData)
	return err
}

// UpsertStep inserts or updates a GitHub workflow step.
func (s *Store) UpsertStep(ctx context.Context, jobID, stepNumber int,
	name, status, conclusion string, startedAt, completedAt *time.Time, durationSec int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_workflow_steps
			(job_id, step_number, name, status, conclusion, started_at, completed_at, duration_seconds)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (job_id, step_number) WHERE false
		DO NOTHING`,
		jobID, stepNumber, name, status, conclusion, startedAt, completedAt, durationSec)
	return err
}

// UpsertCommit inserts or updates a GitHub commit.
func (s *Store) UpsertCommit(ctx context.Context, sha, owner, repo, message, authorName, authorEmail string,
	authoredAt *time.Time, additions, deletions, filesChanged int) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO github_workflow_commits
			(sha, github_owner, github_repo, message, author_name, author_email, authored_at, additions, deletions, files_changed)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (sha, github_owner, github_repo) DO UPDATE SET
			message=EXCLUDED.message, additions=EXCLUDED.additions, deletions=EXCLUDED.deletions, files_changed=EXCLUDED.files_changed`,
		sha, owner, repo, message, authorName, authorEmail, authoredAt, additions, deletions, filesChanged)
	return err
}
