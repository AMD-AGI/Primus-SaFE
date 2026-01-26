// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GithubWorkflowStepFacade provides database operations for workflow steps
type GithubWorkflowStepFacade struct {
	db *gorm.DB
}

// NewGithubWorkflowStepFacade creates a new facade instance
func NewGithubWorkflowStepFacade() *GithubWorkflowStepFacade {
	return &GithubWorkflowStepFacade{
		db: GetFacade().GetSystemConfig().GetDB(),
	}
}

// Upsert creates or updates a step
func (f *GithubWorkflowStepFacade) Upsert(ctx context.Context, step *model.GithubWorkflowSteps) error {
	if step.CreatedAt.IsZero() {
		step.CreatedAt = time.Now()
	}

	return f.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "job_id"}, {Name: "step_number"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"name", "status", "conclusion", "started_at", "completed_at", "duration_seconds",
			}),
		}).
		Create(step).Error
}

// GetByID retrieves a step by ID
func (f *GithubWorkflowStepFacade) GetByID(ctx context.Context, id int64) (*model.GithubWorkflowSteps, error) {
	var step model.GithubWorkflowSteps
	err := f.db.WithContext(ctx).Where("id = ?", id).First(&step).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &step, err
}

// ListByJobID lists all steps for a job
func (f *GithubWorkflowStepFacade) ListByJobID(ctx context.Context, jobID int64) ([]*model.GithubWorkflowSteps, error) {
	var steps []*model.GithubWorkflowSteps
	err := f.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Order("step_number ASC").
		Find(&steps).Error
	return steps, err
}

// DeleteByJobID deletes all steps for a job
func (f *GithubWorkflowStepFacade) DeleteByJobID(ctx context.Context, jobID int64) error {
	return f.db.WithContext(ctx).
		Where("job_id = ?", jobID).
		Delete(&model.GithubWorkflowSteps{}).Error
}

// GetFailedStep returns the first failed step for a job
func (f *GithubWorkflowStepFacade) GetFailedStep(ctx context.Context, jobID int64) (*model.GithubWorkflowSteps, error) {
	var step model.GithubWorkflowSteps
	err := f.db.WithContext(ctx).
		Where("job_id = ? AND conclusion = ?", jobID, WorkflowConclusionFailure).
		Order("step_number ASC").
		First(&step).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &step, err
}

// CountByJobID counts steps by job_id grouped by conclusion
func (f *GithubWorkflowStepFacade) CountByJobID(ctx context.Context, jobID int64) (total, success, failed int, err error) {
	var results []struct {
		Conclusion string
		Count      int
	}

	err = f.db.WithContext(ctx).
		Model(&model.GithubWorkflowSteps{}).
		Select("conclusion, COUNT(*) as count").
		Where("job_id = ?", jobID).
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
