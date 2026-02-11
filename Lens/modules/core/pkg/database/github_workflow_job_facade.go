// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/github"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GithubWorkflowJobFacade provides database operations for workflow jobs
type GithubWorkflowJobFacade struct {
	BaseFacade
}

// NewGithubWorkflowJobFacade creates a new facade instance
func NewGithubWorkflowJobFacade() *GithubWorkflowJobFacade {
	return &GithubWorkflowJobFacade{}
}

// WithCluster returns a new facade instance for the specified cluster
func (f *GithubWorkflowJobFacade) WithCluster(clusterName string) *GithubWorkflowJobFacade {
	return &GithubWorkflowJobFacade{
		BaseFacade: BaseFacade{clusterName: clusterName},
	}
}

// JobWithSteps extends GithubWorkflowJobs with steps
type JobWithSteps struct {
	*model.GithubWorkflowJobs
	Steps []*model.GithubWorkflowSteps `json:"steps,omitempty"`
}

// Upsert creates or updates a job.
// The "needs" field is only updated when the incoming value is non-empty,
// preventing SyncFromGitHub (which has no workflow YAML) from overwriting
// the dependency data set by GraphFetchExecutor.
func (f *GithubWorkflowJobFacade) Upsert(ctx context.Context, job *model.GithubWorkflowJobs) error {
	job.UpdatedAt = time.Now()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now()
	}

	// Base columns that are always updated on conflict
	updates := []clause.Assignment{
		{Column: clause.Column{Name: "name"}, Value: gorm.Expr("EXCLUDED.name")},
		{Column: clause.Column{Name: "status"}, Value: gorm.Expr("EXCLUDED.status")},
		{Column: clause.Column{Name: "conclusion"}, Value: gorm.Expr("EXCLUDED.conclusion")},
		{Column: clause.Column{Name: "started_at"}, Value: gorm.Expr("EXCLUDED.started_at")},
		{Column: clause.Column{Name: "completed_at"}, Value: gorm.Expr("EXCLUDED.completed_at")},
		{Column: clause.Column{Name: "duration_seconds"}, Value: gorm.Expr("EXCLUDED.duration_seconds")},
		{Column: clause.Column{Name: "runner_id"}, Value: gorm.Expr("EXCLUDED.runner_id")},
		{Column: clause.Column{Name: "runner_name"}, Value: gorm.Expr("EXCLUDED.runner_name")},
		{Column: clause.Column{Name: "runner_group_name"}, Value: gorm.Expr("EXCLUDED.runner_group_name")},
		{Column: clause.Column{Name: "steps_count"}, Value: gorm.Expr("EXCLUDED.steps_count")},
		{Column: clause.Column{Name: "steps_completed"}, Value: gorm.Expr("EXCLUDED.steps_completed")},
		{Column: clause.Column{Name: "steps_failed"}, Value: gorm.Expr("EXCLUDED.steps_failed")},
		{Column: clause.Column{Name: "html_url"}, Value: gorm.Expr("EXCLUDED.html_url")},
		{Column: clause.Column{Name: "updated_at"}, Value: gorm.Expr("EXCLUDED.updated_at")},
		// Only update needs when the incoming value is non-empty (preserves graph_fetch data)
		{Column: clause.Column{Name: "needs"}, Value: gorm.Expr("CASE WHEN EXCLUDED.needs != '' THEN EXCLUDED.needs ELSE github_workflow_jobs.needs END")},
	}

	return f.getDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "run_id"}, {Name: "github_job_id"}},
			DoUpdates: clause.Set(updates),
		}).
		Create(job).Error
}

// GetByID retrieves a job by ID
func (f *GithubWorkflowJobFacade) GetByID(ctx context.Context, id int64) (*model.GithubWorkflowJobs, error) {
	var job model.GithubWorkflowJobs
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&job).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &job, err
}

// ListByRunID lists all jobs for a workflow run
func (f *GithubWorkflowJobFacade) ListByRunID(ctx context.Context, runID int64) ([]*model.GithubWorkflowJobs, error) {
	var jobs []*model.GithubWorkflowJobs
	err := f.getDB().WithContext(ctx).
		Where("run_id = ?", runID).
		Order("id ASC").
		Find(&jobs).Error
	return jobs, err
}

// ListByRunIDWithSteps lists all jobs with their steps
func (f *GithubWorkflowJobFacade) ListByRunIDWithSteps(ctx context.Context, runID int64) ([]*JobWithSteps, error) {
	jobs, err := f.ListByRunID(ctx, runID)
	if err != nil {
		return nil, err
	}

	stepFacade := NewGithubWorkflowStepFacade().WithCluster(f.clusterName)
	result := make([]*JobWithSteps, len(jobs))
	for i, job := range jobs {
		steps, _ := stepFacade.ListByJobID(ctx, job.ID)
		result[i] = &JobWithSteps{
			GithubWorkflowJobs: job,
			Steps:              steps,
		}
	}
	return result, nil
}

// DeleteByRunID deletes all jobs for a run
func (f *GithubWorkflowJobFacade) DeleteByRunID(ctx context.Context, runID int64) error {
	// First delete all steps for jobs in this run
	subQuery := f.getDB().Model(&model.GithubWorkflowJobs{}).Select("id").Where("run_id = ?", runID)
	if err := f.getDB().WithContext(ctx).
		Where("job_id IN (?)", subQuery).
		Delete(&model.GithubWorkflowSteps{}).Error; err != nil {
		return err
	}

	// Then delete jobs
	return f.getDB().WithContext(ctx).
		Where("run_id = ?", runID).
		Delete(&model.GithubWorkflowJobs{}).Error
}

// SyncFromGitHub syncs jobs and steps from GitHub API data
func (f *GithubWorkflowJobFacade) SyncFromGitHub(ctx context.Context, runID int64, ghJobs []github.JobInfo) error {
	stepFacade := NewGithubWorkflowStepFacade().WithCluster(f.clusterName)

	for _, ghJob := range ghJobs {
		// Calculate duration
		var duration int
		if ghJob.StartedAt != nil && ghJob.CompletedAt != nil {
			duration = int(ghJob.CompletedAt.Sub(*ghJob.StartedAt).Seconds())
		}

		// Count steps
		stepsCompleted := 0
		stepsFailed := 0
		for _, step := range ghJob.Steps {
			if step.Conclusion == "success" {
				stepsCompleted++
			} else if step.Conclusion == "failure" {
				stepsFailed++
			}
		}

		job := &model.GithubWorkflowJobs{
			RunID:           runID,
			GithubJobID:     ghJob.ID,
			Name:            ghJob.Name,
			Status:          ghJob.Status,
			Conclusion:      ghJob.Conclusion,
			StartedAt:       ghJob.StartedAt,
			CompletedAt:     ghJob.CompletedAt,
			DurationSeconds: duration,
			RunnerID:        ghJob.RunnerID,
			RunnerName:      ghJob.RunnerName,
			StepsCount:      len(ghJob.Steps),
			StepsCompleted:  stepsCompleted,
			StepsFailed:     stepsFailed,
		}

		if err := f.Upsert(ctx, job); err != nil {
			return err
		}

		// Get job ID for steps
		savedJob, err := f.GetByGithubJobID(ctx, runID, ghJob.ID)
		if err != nil || savedJob == nil {
			continue
		}

		// Sync steps
		for _, ghStep := range ghJob.Steps {
			var stepDuration int
			if ghStep.StartedAt != nil && ghStep.CompletedAt != nil {
				stepDuration = int(ghStep.CompletedAt.Sub(*ghStep.StartedAt).Seconds())
			}

			step := &model.GithubWorkflowSteps{
				JobID:           savedJob.ID,
				StepNumber:      ghStep.Number,
				Name:            ghStep.Name,
				Status:          ghStep.Status,
				Conclusion:      ghStep.Conclusion,
				StartedAt:       ghStep.StartedAt,
				CompletedAt:     ghStep.CompletedAt,
				DurationSeconds: stepDuration,
			}

			if err := stepFacade.Upsert(ctx, step); err != nil {
				return err
			}
		}
	}

	return nil
}

// GetByGithubJobID retrieves a job by run_id and github_job_id
func (f *GithubWorkflowJobFacade) GetByGithubJobID(ctx context.Context, runID, githubJobID int64) (*model.GithubWorkflowJobs, error) {
	var job model.GithubWorkflowJobs
	err := f.getDB().WithContext(ctx).
		Where("run_id = ? AND github_job_id = ?", runID, githubJobID).
		First(&job).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &job, err
}

// FindByGithubJobIDWithSteps looks up a single GitHub job (and its steps) by
// github_job_id across any run_id.  Used by the RunDetail page to display only
// the single job executed by a specific K8s runner.
func (f *GithubWorkflowJobFacade) FindByGithubJobIDWithSteps(ctx context.Context, githubJobID int64) (*JobWithSteps, error) {
	var job model.GithubWorkflowJobs
	err := f.getDB().WithContext(ctx).
		Where("github_job_id = ?", githubJobID).
		Order("id DESC").
		First(&job).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	stepFacade := NewGithubWorkflowStepFacade().WithCluster(f.clusterName)
	steps, _ := stepFacade.ListByJobID(ctx, job.ID)
	return &JobWithSteps{
		GithubWorkflowJobs: &job,
		Steps:              steps,
	}, nil
}

// CountByRunID counts jobs by run_id grouped by conclusion
func (f *GithubWorkflowJobFacade) CountByRunID(ctx context.Context, runID int64) (total, success, failed int, err error) {
	var results []struct {
		Conclusion string
		Count      int
	}

	err = f.getDB().WithContext(ctx).
		Model(&model.GithubWorkflowJobs{}).
		Select("conclusion, COUNT(*) as count").
		Where("run_id = ?", runID).
		Group("conclusion").
		Find(&results).Error

	if err != nil {
		return
	}

	for _, r := range results {
		total += r.Count
		switch r.Conclusion {
		case WorkflowConclusionSuccess:
			success = r.Count
		case WorkflowConclusionFailure:
			failed = r.Count
		}
	}
	return
}

// ListByRunSummaryID lists unique GitHub jobs for a run summary.
// It deduplicates by github_job_id, keeping the record with the most complete data
// (preferring rows that have needs populated and the latest update).
func (f *GithubWorkflowJobFacade) ListByRunSummaryID(ctx context.Context, runSummaryID int64) ([]*model.GithubWorkflowJobs, error) {
	var jobs []*model.GithubWorkflowJobs
	// Use DISTINCT ON (github_job_id) to deduplicate.
	// Order by needs DESC so rows with needs populated come first, then by id ASC.
	err := f.getDB().WithContext(ctx).
		Select("DISTINCT ON (github_workflow_jobs.github_job_id) github_workflow_jobs.*").
		Joins("JOIN github_workflow_runs r ON r.id = github_workflow_jobs.run_id").
		Where("r.run_summary_id = ?", runSummaryID).
		Order("github_workflow_jobs.github_job_id, github_workflow_jobs.needs DESC, github_workflow_jobs.id ASC").
		Find(&jobs).Error
	return jobs, err
}

// ListByRunSummaryIDWithSteps lists all GitHub jobs with steps for a run summary
func (f *GithubWorkflowJobFacade) ListByRunSummaryIDWithSteps(ctx context.Context, runSummaryID int64) ([]*JobWithSteps, error) {
	jobs, err := f.ListByRunSummaryID(ctx, runSummaryID)
	if err != nil {
		return nil, err
	}

	stepFacade := NewGithubWorkflowStepFacade().WithCluster(f.clusterName)
	result := make([]*JobWithSteps, len(jobs))
	for i, job := range jobs {
		steps, _ := stepFacade.ListByJobID(ctx, job.ID)
		result[i] = &JobWithSteps{
			GithubWorkflowJobs: job,
			Steps:              steps,
		}
	}
	return result, nil
}
