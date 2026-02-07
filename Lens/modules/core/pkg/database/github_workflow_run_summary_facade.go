// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"errors"
	"fmt"
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
	BaseFacade
}

// NewGithubWorkflowRunSummaryFacade creates a new facade instance
func NewGithubWorkflowRunSummaryFacade() *GithubWorkflowRunSummaryFacade {
	return &GithubWorkflowRunSummaryFacade{}
}

// WithCluster returns a new facade for the specified cluster
func (f *GithubWorkflowRunSummaryFacade) WithCluster(clusterName string) *GithubWorkflowRunSummaryFacade {
	return &GithubWorkflowRunSummaryFacade{
		BaseFacade: BaseFacade{clusterName: clusterName},
	}
}

// GetOrCreateByRunID returns existing summary or creates a new one.
// runnerSetID is used to populate PrimaryRunnerSetID when creating a new summary,
// and to backfill it on existing summaries that have PrimaryRunnerSetID = 0.
// Returns (summary, isNew, error)
func (f *GithubWorkflowRunSummaryFacade) GetOrCreateByRunID(
	ctx context.Context,
	githubRunID int64,
	owner, repo string,
	runnerSetID int64,
) (*model.GithubWorkflowRunSummaries, bool, error) {
	db := f.getDB()
	if db == nil {
		return nil, false, fmt.Errorf("GetOrCreateByRunID: database connection is nil for github_run_id %d", githubRunID)
	}

	var summary model.GithubWorkflowRunSummaries

	err := db.WithContext(ctx).
		Where("github_run_id = ?", githubRunID).
		First(&summary).Error

	if err == nil {
		// Additional check: ensure ID is valid (handles edge case where First() returns nil error but empty struct)
		if summary.ID > 0 {
			// Backfill PrimaryRunnerSetID if missing
			if summary.PrimaryRunnerSetID == 0 && runnerSetID > 0 {
				summary.PrimaryRunnerSetID = runnerSetID
				db.WithContext(ctx).Model(&summary).Update("primary_runner_set_id", runnerSetID)
			}
			return &summary, false, nil // exists
		}
		// If ID is 0, something went wrong - fall through to create
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, fmt.Errorf("GetOrCreateByRunID: query failed for github_run_id %d: %w", githubRunID, err)
	}

	// Create new record
	summary = model.GithubWorkflowRunSummaries{
		GithubRunID:        githubRunID,
		GithubRunAttempt:   1,
		Owner:              owner,
		Repo:               repo,
		PrimaryRunnerSetID: runnerSetID,
		Status:             RunSummaryStatusQueued,
		CollectionStatus:   RunSummaryCollectionPending,
		GraphFetched:       false,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := db.WithContext(ctx).Create(&summary).Error; err != nil {
		// Handle race condition - another goroutine may have created it
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			if err := db.WithContext(ctx).Where("github_run_id = ?", githubRunID).First(&summary).Error; err != nil {
				return nil, false, fmt.Errorf("GetOrCreateByRunID: re-query after duplicate key failed for github_run_id %d: %w", githubRunID, err)
			}
			if summary.ID > 0 {
				// Backfill PrimaryRunnerSetID if missing
				if summary.PrimaryRunnerSetID == 0 && runnerSetID > 0 {
					summary.PrimaryRunnerSetID = runnerSetID
					db.WithContext(ctx).Model(&summary).Update("primary_runner_set_id", runnerSetID)
				}
				return &summary, false, nil
			}
			return nil, false, fmt.Errorf("GetOrCreateByRunID: re-query returned invalid ID for github_run_id %d", githubRunID)
		}
		return nil, false, fmt.Errorf("GetOrCreateByRunID: create failed for github_run_id %d: %w", githubRunID, err)
	}

	// Verify ID was populated after create
	if summary.ID == 0 {
		return nil, false, fmt.Errorf("GetOrCreateByRunID: create succeeded but ID is 0 for github_run_id %d", githubRunID)
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
	WorkflowName     string
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
	db := f.getDB()
	if db == nil {
		return nil, 0, errors.New("database connection not available")
	}
	query := db.WithContext(ctx).
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
		if filter.WorkflowName != "" {
			query = query.Where("workflow_name = ?", filter.WorkflowName)
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
		if filter.WorkflowName != "" {
			query = query.Where("workflow_name = ?", filter.WorkflowName)
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
// Note: Uses 'status' field (K8s workload status) since 'workflow_status' (GitHub status) is not always populated
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
				COUNT(*) FILTER (WHERE status IN ('completed', 'failed', 'skipped')) as completed_jobs,
				COUNT(*) FILTER (WHERE status = 'completed' AND (workflow_conclusion IS NULL OR workflow_conclusion = '' OR workflow_conclusion = 'success')) as successful_jobs,
				COUNT(*) FILTER (WHERE status = 'failed' OR workflow_conclusion = 'failure') as failed_jobs,
				COUNT(*) FILTER (WHERE workflow_conclusion = 'cancelled') as cancelled_jobs,
				COUNT(*) FILTER (WHERE status = 'running') as in_progress_jobs,
				COUNT(*) FILTER (WHERE status = 'pending') as queued_jobs,
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

// RefreshStatusFromJobs updates job statistics and derives status from them
// This should be called whenever a job's status changes
func (f *GithubWorkflowRunSummaryFacade) RefreshStatusFromJobs(ctx context.Context, summaryID int64) error {
	// First update job stats
	if err := f.UpdateJobStats(ctx, summaryID); err != nil {
		return fmt.Errorf("failed to update job stats: %w", err)
	}

	// Get the updated summary to check stats
	summary, err := f.GetByID(ctx, summaryID)
	if err != nil {
		return fmt.Errorf("failed to get summary: %w", err)
	}
	if summary == nil {
		return nil
	}

	// Derive status from job stats
	var newStatus, newConclusion string

	if summary.TotalJobs == 0 {
		// No jobs yet
		newStatus = RunSummaryStatusQueued
	} else if summary.CompletedJobs == summary.TotalJobs {
		// All jobs completed
		newStatus = RunSummaryStatusCompleted
		if summary.FailedJobs > 0 {
			newConclusion = RunSummaryConclusionFailure
		} else if summary.CancelledJobs > 0 {
			newConclusion = RunSummaryConclusionCancelled
		} else {
			newConclusion = RunSummaryConclusionSuccess
		}
	} else if summary.InProgressJobs > 0 {
		// Some jobs running
		newStatus = RunSummaryStatusInProgress
	} else if summary.QueuedJobs > 0 {
		// Jobs waiting to start
		newStatus = RunSummaryStatusQueued
	} else {
		// Fallback to in_progress
		newStatus = RunSummaryStatusInProgress
	}

	// Calculate progress
	var progressPercent int32
	if summary.TotalJobs > 0 {
		progressPercent = int32((float64(summary.CompletedJobs) / float64(summary.TotalJobs)) * 100)
	}

	// Update status and progress
	updates := map[string]interface{}{
		"status":           newStatus,
		"progress_percent": progressPercent,
		"last_synced_at":   time.Now(),
		"updated_at":       time.Now(),
	}
	if newConclusion != "" {
		updates["conclusion"] = newConclusion
	}
	if newStatus == RunSummaryStatusCompleted && summary.RunCompletedAt.IsZero() {
		updates["run_completed_at"] = time.Now()
	}

	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("id = ?", summaryID).
		Updates(updates).Error
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
		"last_synced_at":     time.Now(),
		"updated_at":         time.Now(),
	}
	if !data.RunStartedAt.IsZero() {
		updates["run_started_at"] = data.RunStartedAt
	}
	if data.DisplayTitle != "" {
		updates["display_title"] = data.DisplayTitle
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
	DisplayTitle    string
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

// ListDistinctWorkflowNames returns distinct workflow names for a repository
func (f *GithubWorkflowRunSummaryFacade) ListDistinctWorkflowNames(
	ctx context.Context,
	owner, repo string,
) ([]string, error) {
	var names []string
	err := f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("owner = ? AND repo = ? AND workflow_name != ''", owner, repo).
		Distinct("workflow_name").
		Order("workflow_name ASC").
		Pluck("workflow_name", &names).Error
	return names, err
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

// GetActivePlaceholderByRunnerSet returns an active placeholder summary for a runner set
func (f *GithubWorkflowRunSummaryFacade) GetActivePlaceholderByRunnerSet(ctx context.Context, runnerSetID int64) (*model.GithubWorkflowRunSummaries, error) {
	var summary model.GithubWorkflowRunSummaries
	err := f.getDB().WithContext(ctx).
		Where("primary_runner_set_id = ? AND is_placeholder = true AND status NOT IN ?",
			runnerSetID, []string{RunSummaryStatusCompleted}).
		Order("created_at DESC").
		First(&summary).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &summary, err
}

// Create creates a new run summary
func (f *GithubWorkflowRunSummaryFacade) Create(ctx context.Context, summary *model.GithubWorkflowRunSummaries) (*model.GithubWorkflowRunSummaries, error) {
	now := time.Now()
	if summary.CreatedAt.IsZero() {
		summary.CreatedAt = now
	}
	if summary.UpdatedAt.IsZero() {
		summary.UpdatedAt = now
	}
	if summary.Status == "" {
		summary.Status = RunSummaryStatusQueued
	}
	if summary.CollectionStatus == "" {
		summary.CollectionStatus = RunSummaryCollectionPending
	}

	if err := f.getDB().WithContext(ctx).Create(summary).Error; err != nil {
		return nil, err
	}
	return summary, nil
}

// UpdateRunnerCounts updates the active launcher/worker counts and error count for a summary
func (f *GithubWorkflowRunSummaryFacade) UpdateRunnerCounts(ctx context.Context, summaryID int64, activeLaunchers, activeWorkers, errorCount int32) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowRunSummaries{}).
		Where("id = ?", summaryID).
		Updates(map[string]interface{}{
			"active_launchers": activeLaunchers,
			"active_workers":   activeWorkers,
			"error_count":      errorCount,
			"updated_at":       time.Now(),
		}).Error
}

// CleanupOrphanPlaceholders deletes placeholder summaries that have no runs referencing them
// and are older than the specified duration
func (f *GithubWorkflowRunSummaryFacade) CleanupOrphanPlaceholders(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result := f.getDB().WithContext(ctx).Exec(`
		DELETE FROM github_workflow_run_summaries
		WHERE is_placeholder = true
		AND created_at < ?
		AND NOT EXISTS (
			SELECT 1 FROM github_workflow_runs r
			WHERE r.run_summary_id = github_workflow_run_summaries.id
		)
	`, cutoff)
	return result.RowsAffected, result.Error
}
