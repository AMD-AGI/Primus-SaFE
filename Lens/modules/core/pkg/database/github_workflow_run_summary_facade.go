// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// RunSummaryStatus constants for github_workflow_run_summaries.status
const (
	RunSummaryStatusQueued     = "queued"
	RunSummaryStatusInProgress = "in_progress"
	RunSummaryStatusCompleted  = "completed"
	RunSummaryStatusWaiting    = "waiting"
	RunSummaryStatusRequested  = "requested"
)

// RunSummaryConclusion constants for github_workflow_run_summaries.conclusion
const (
	RunSummaryConclusionSuccess        = "success"
	RunSummaryConclusionFailure        = "failure"
	RunSummaryConclusionCancelled      = "cancelled"
	RunSummaryConclusionSkipped        = "skipped"
	RunSummaryConclusionTimedOut       = "timed_out"
	RunSummaryConclusionActionRequired = "action_required"
	RunSummaryConclusionNeutral        = "neutral"
)

// RunSummaryCollectionStatus constants
const (
	RunSummaryCollectionPending   = "pending"
	RunSummaryCollectionPartial   = "partial"
	RunSummaryCollectionCompleted = "completed"
	RunSummaryCollectionFailed    = "failed"
)

// GithubWorkflowRunSummaryFacade provides database operations for workflow run summaries
type GithubWorkflowRunSummaryFacade struct {
	db *gorm.DB
}

// NewGithubWorkflowRunSummaryFacade creates a new facade instance
func NewGithubWorkflowRunSummaryFacade() *GithubWorkflowRunSummaryFacade {
	return &GithubWorkflowRunSummaryFacade{
		db: nil, // Will be set lazily
	}
}

// getDB returns the database connection, initializing lazily to avoid circular dependency
func (f *GithubWorkflowRunSummaryFacade) getDB() *gorm.DB {
	if f.db == nil {
		f.db = GetFacade().GetSystemConfig().GetDB()
	}
	return f.db
}

// GetOrCreateByRunID returns existing summary or creates a new one
// Returns (summary, isNew, error)
func (f *GithubWorkflowRunSummaryFacade) GetOrCreateByRunID(
	ctx context.Context,
	githubRunID int64,
	owner, repo string,
) (*model.GithubWorkflowRunSummaries, bool, error) {
	var summary model.GithubWorkflowRunSummaries

	err := f.getDB().WithContext(ctx).
		Where("github_run_id = ?", githubRunID).
		First(&summary).Error

	if err == nil {
		return &summary, false, nil // exists
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	// Create new record
	summary = model.GithubWorkflowRunSummaries{
		GithubRunID:      githubRunID,
		GithubRunAttempt: 1,
		Owner:            owner,
		Repo:             repo,
		Status:           RunSummaryStatusQueued,
		CollectionStatus: RunSummaryCollectionPending,
		GraphFetched:     false,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	if err := f.getDB().WithContext(ctx).Create(&summary).Error; err != nil {
		// Handle race condition - another goroutine may have created it
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			if err := f.getDB().WithContext(ctx).Where("github_run_id = ?", githubRunID).First(&summary).Error; err != nil {
				return nil, false, err
			}
			return &summary, false, nil
		}
		return nil, false, err
	}

	return &summary, true, nil // created
}

// GetByID retrieves a summary by ID
func (f *GithubWorkflowRunSummaryFacade) GetByID(ctx context.Context, id int64) (*model.GithubWorkflowRunSummaries, error) {
	var summary model.GithubWorkflowRunSummaries
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&summary).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &summary, err
}

// GetByGithubRunID retrieves a summary by GitHub run ID
func (f *GithubWorkflowRunSummaryFacade) GetByGithubRunID(ctx context.Context, githubRunID int64) (*model.GithubWorkflowRunSummaries, error) {
	var summary model.GithubWorkflowRunSummaries
	err := f.getDB().WithContext(ctx).Where("github_run_id = ?", githubRunID).First(&summary).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &summary, err
}

// RunSummaryFilter defines filter options for listing run summaries
type RunSummaryFilter struct {
	Owner            string
	Repo             string
	WorkflowPath     string
	Status           string
	Conclusion       string
	CollectionStatus string
	RunnerSetID      int64
	ConfigID         int64
	HeadBranch       string
	EventName        string
	Since            *time.Time
	Until            *time.Time
	Offset           int
	Limit            int
}

// ListByRepo returns runs for a repository with pagination
func (f *GithubWorkflowRunSummaryFacade) ListByRepo(
	ctx context.Context,
	owner, repo string,
	filter *RunSummaryFilter,
) ([]*model.GithubWorkflowRunSummaries, int64, error) {
	query := f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("owner = ? AND repo = ?", owner, repo)

	if filter != nil {
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.Conclusion != "" {
			query = query.Where("conclusion = ?", filter.Conclusion)
		}
		if filter.CollectionStatus != "" {
			query = query.Where("collection_status = ?", filter.CollectionStatus)
		}
		if filter.WorkflowPath != "" {
			query = query.Where("workflow_path = ?", filter.WorkflowPath)
		}
		if filter.HeadBranch != "" {
			query = query.Where("head_branch = ?", filter.HeadBranch)
		}
		if filter.EventName != "" {
			query = query.Where("event_name = ?", filter.EventName)
		}
		if filter.RunnerSetID > 0 {
			query = query.Where("primary_runner_set_id = ?", filter.RunnerSetID)
		}
		if filter.Since != nil {
			query = query.Where("created_at >= ?", *filter.Since)
		}
		if filter.Until != nil {
			query = query.Where("created_at <= ?", *filter.Until)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var summaries []*model.GithubWorkflowRunSummaries
	if filter != nil {
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
	}

	err := query.Order("created_at DESC").Find(&summaries).Error
	return summaries, total, err
}

// List returns runs with optional filtering
func (f *GithubWorkflowRunSummaryFacade) List(
	ctx context.Context,
	filter *RunSummaryFilter,
) ([]*model.GithubWorkflowRunSummaries, int64, error) {
	query := f.getDB().WithContext(ctx).Model(&model.GithubWorkflowRunSummaries{})

	if filter != nil {
		if filter.Owner != "" {
			query = query.Where("owner = ?", filter.Owner)
		}
		if filter.Repo != "" {
			query = query.Where("repo = ?", filter.Repo)
		}
		if filter.Status != "" {
			query = query.Where("status = ?", filter.Status)
		}
		if filter.Conclusion != "" {
			query = query.Where("conclusion = ?", filter.Conclusion)
		}
		if filter.CollectionStatus != "" {
			query = query.Where("collection_status = ?", filter.CollectionStatus)
		}
		if filter.WorkflowPath != "" {
			query = query.Where("workflow_path = ?", filter.WorkflowPath)
		}
		if filter.HeadBranch != "" {
			query = query.Where("head_branch = ?", filter.HeadBranch)
		}
		if filter.EventName != "" {
			query = query.Where("event_name = ?", filter.EventName)
		}
		if filter.RunnerSetID > 0 {
			query = query.Where("primary_runner_set_id = ?", filter.RunnerSetID)
		}
		if filter.ConfigID > 0 {
			query = query.Where("config_id = ?", filter.ConfigID)
		}
		if filter.Since != nil {
			query = query.Where("created_at >= ?", *filter.Since)
		}
		if filter.Until != nil {
			query = query.Where("created_at <= ?", *filter.Until)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var summaries []*model.GithubWorkflowRunSummaries
	if filter != nil {
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
	}

	err := query.Order("created_at DESC").Find(&summaries).Error
	return summaries, total, err
}

// Update updates a summary record
func (f *GithubWorkflowRunSummaryFacade) Update(ctx context.Context, summary *model.GithubWorkflowRunSummaries) error {
	summary.UpdatedAt = time.Now()
	return f.getDB().WithContext(ctx).Save(summary).Error
}

// UpdateStatus updates the status and conclusion
func (f *GithubWorkflowRunSummaryFacade) UpdateStatus(
	ctx context.Context,
	id int64,
	status, conclusion string,
) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}
	if conclusion != "" {
		updates["conclusion"] = conclusion
	}
	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateJobStats updates aggregated job statistics by querying github_workflow_runs table
func (f *GithubWorkflowRunSummaryFacade) UpdateJobStats(ctx context.Context, summaryID int64) error {
	sql := `
		UPDATE github_workflow_run_summaries rs
		SET 
			total_jobs = sub.total_jobs,
			completed_jobs = sub.completed_jobs,
			successful_jobs = sub.successful_jobs,
			failed_jobs = sub.failed_jobs,
			cancelled_jobs = sub.cancelled_jobs,
			in_progress_jobs = sub.in_progress_jobs,
			queued_jobs = sub.queued_jobs,
			total_files_processed = sub.total_files_processed,
			total_metrics_count = sub.total_metrics_count,
			updated_at = NOW()
		FROM (
			SELECT 
				run_summary_id,
				COUNT(*) as total_jobs,
				COUNT(*) FILTER (WHERE workflow_status = 'completed') as completed_jobs,
				COUNT(*) FILTER (WHERE workflow_conclusion = 'success') as successful_jobs,
				COUNT(*) FILTER (WHERE workflow_conclusion = 'failure') as failed_jobs,
				COUNT(*) FILTER (WHERE workflow_conclusion = 'cancelled') as cancelled_jobs,
				COUNT(*) FILTER (WHERE workflow_status = 'in_progress') as in_progress_jobs,
				COUNT(*) FILTER (WHERE workflow_status = 'queued') as queued_jobs,
				COALESCE(SUM(files_processed), 0) as total_files_processed,
				COALESCE(SUM(metrics_count), 0) as total_metrics_count
			FROM github_workflow_runs
			WHERE run_summary_id = ?
			GROUP BY run_summary_id
		) sub
		WHERE rs.id = sub.run_summary_id
	`
	return f.getDB().WithContext(ctx).Exec(sql, summaryID).Error
}

// UpdateGraphFetched marks the graph as fetched
func (f *GithubWorkflowRunSummaryFacade) UpdateGraphFetched(ctx context.Context, id int64, fetched bool) error {
	updates := map[string]interface{}{
		"graph_fetched": fetched,
		"updated_at":    time.Now(),
	}
	if fetched {
		updates["graph_fetched_at"] = time.Now()
	}
	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateAnalysisTriggered updates analysis trigger flags
func (f *GithubWorkflowRunSummaryFacade) UpdateAnalysisTriggered(
	ctx context.Context,
	summaryID int64,
	analysisType string, // "code" or "failure"
	triggered bool,
) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	switch analysisType {
	case "code":
		updates["code_analysis_triggered"] = triggered
		if triggered {
			updates["code_analysis_triggered_at"] = time.Now()
		}
	case "failure":
		updates["failure_analysis_triggered"] = triggered
		if triggered {
			updates["failure_analysis_triggered_at"] = time.Now()
		}
	default:
		return errors.New("unknown analysis type: " + analysisType)
	}

	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("id = ?", summaryID).
		Updates(updates).Error
}

// UpdateProgress updates the current progress info
func (f *GithubWorkflowRunSummaryFacade) UpdateProgress(
	ctx context.Context,
	id int64,
	currentJobName, currentStepName string,
	progressPercent int32,
) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"current_job_name":  currentJobName,
			"current_step_name": currentStepName,
			"progress_percent":  progressPercent,
			"last_synced_at":    time.Now(),
			"updated_at":        time.Now(),
		}).Error
}

// UpdateFromGitHub updates summary with data from GitHub API
func (f *GithubWorkflowRunSummaryFacade) UpdateFromGitHub(
	ctx context.Context,
	id int64,
	data *GitHubRunData,
) error {
	updates := map[string]interface{}{
		"github_run_number":  data.RunNumber,
		"github_run_attempt": data.RunAttempt,
		"workflow_name":      data.WorkflowName,
		"workflow_path":      data.WorkflowPath,
		"workflow_id":        data.WorkflowID,
		"head_sha":           data.HeadSha,
		"head_branch":        data.HeadBranch,
		"base_branch":        data.BaseBranch,
		"event_name":         data.EventName,
		"actor":              data.Actor,
		"triggering_actor":   data.TriggeringActor,
		"status":             data.Status,
		"run_started_at":     data.RunStartedAt,
		"last_synced_at":     time.Now(),
		"updated_at":         time.Now(),
	}
	if data.Conclusion != "" {
		updates["conclusion"] = data.Conclusion
	}
	if data.RunCompletedAt != nil {
		updates["run_completed_at"] = *data.RunCompletedAt
	}

	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// GitHubRunData holds data from GitHub API for updating run summary
type GitHubRunData struct {
	RunNumber       int32
	RunAttempt      int32
	WorkflowName    string
	WorkflowPath    string
	WorkflowID      int64
	HeadSha         string
	HeadBranch      string
	BaseBranch      string
	EventName       string
	Actor           string
	TriggeringActor string
	Status          string
	Conclusion      string
	RunStartedAt    time.Time
	RunCompletedAt  *time.Time
}

// Delete deletes a summary by ID
func (f *GithubWorkflowRunSummaryFacade) Delete(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).
		Where("id = ?", id).
		Delete(&model.GithubWorkflowRunSummaries{}).Error
}

// ListPendingGraphFetch returns runs that need graph fetching
func (f *GithubWorkflowRunSummaryFacade) ListPendingGraphFetch(ctx context.Context, limit int) ([]*model.GithubWorkflowRunSummaries, error) {
	var summaries []*model.GithubWorkflowRunSummaries
	query := f.getDB().WithContext(ctx).
		Where("graph_fetched = ?", false).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&summaries).Error
	return summaries, err
}

// ListInProgress returns runs that are in progress
func (f *GithubWorkflowRunSummaryFacade) ListInProgress(ctx context.Context, limit int) ([]*model.GithubWorkflowRunSummaries, error) {
	var summaries []*model.GithubWorkflowRunSummaries
	query := f.getDB().WithContext(ctx).
		Where("status IN ?", []string{RunSummaryStatusQueued, RunSummaryStatusInProgress}).
		Order("created_at ASC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&summaries).Error
	return summaries, err
}
