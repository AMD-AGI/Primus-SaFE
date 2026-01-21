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

// CommitImpactAnalysisFacadeInterface defines the interface for CommitImpactAnalysis operations
type CommitImpactAnalysisFacadeInterface interface {
	// Create creates a new commit impact analysis
	Create(ctx context.Context, analysis *model.CommitImpactAnalysis) error
	// Upsert creates or updates a commit impact analysis
	Upsert(ctx context.Context, analysis *model.CommitImpactAnalysis) error
	// GetByID gets an analysis by ID
	GetByID(ctx context.Context, id int64) (*model.CommitImpactAnalysis, error)
	// GetByRunAndCommit gets an analysis by run ID and commit SHA
	GetByRunAndCommit(ctx context.Context, runID int64, commitSHA string) (*model.CommitImpactAnalysis, error)
	// ListByRun lists all analyses for a run
	ListByRun(ctx context.Context, runID int64) ([]*model.CommitImpactAnalysis, error)
	// ListLikelyCausesByRun lists analyses marked as likely causes for a run
	ListLikelyCausesByRun(ctx context.Context, runID int64) ([]*model.CommitImpactAnalysis, error)
	// ListByConfig lists all analyses for a config within a time range
	ListByConfig(ctx context.Context, configID int64, since *time.Time, limit int) ([]*model.CommitImpactAnalysis, error)
	// ListByCommitSHA lists all analyses for a specific commit SHA
	ListByCommitSHA(ctx context.Context, commitSHA string) ([]*model.CommitImpactAnalysis, error)
	// Delete deletes a commit impact analysis
	Delete(ctx context.Context, id int64) error
	// DeleteByRun deletes all analyses for a run
	DeleteByRun(ctx context.Context, runID int64) (int64, error)
	// BulkCreate creates multiple analyses in batch
	BulkCreate(ctx context.Context, analyses []*model.CommitImpactAnalysis) error
	// WithCluster returns a new facade instance using the specified cluster
	WithCluster(clusterName string) CommitImpactAnalysisFacadeInterface
}

// CommitImpactAnalysisFacade implements CommitImpactAnalysisFacadeInterface
type CommitImpactAnalysisFacade struct {
	BaseFacade
}

// NewCommitImpactAnalysisFacade creates a new CommitImpactAnalysisFacade
func NewCommitImpactAnalysisFacade() *CommitImpactAnalysisFacade {
	return &CommitImpactAnalysisFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *CommitImpactAnalysisFacade) WithCluster(clusterName string) CommitImpactAnalysisFacadeInterface {
	return &CommitImpactAnalysisFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new commit impact analysis
func (f *CommitImpactAnalysisFacade) Create(ctx context.Context, analysis *model.CommitImpactAnalysis) error {
	return f.getDB().WithContext(ctx).Create(analysis).Error
}

// Upsert creates or updates a commit impact analysis
func (f *CommitImpactAnalysisFacade) Upsert(ctx context.Context, analysis *model.CommitImpactAnalysis) error {
	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "run_id"}, {Name: "commit_sha"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"author_name", "commit_message", "files_changed", "impacted_metrics",
			"impact_score", "is_likely_cause", "analyzed_at",
		}),
	}).Create(analysis).Error
}

// GetByID gets an analysis by ID
func (f *CommitImpactAnalysisFacade) GetByID(ctx context.Context, id int64) (*model.CommitImpactAnalysis, error) {
	var analysis model.CommitImpactAnalysis
	err := f.getDB().WithContext(ctx).Where("id = ?", id).First(&analysis).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &analysis, nil
}

// GetByRunAndCommit gets an analysis by run ID and commit SHA
func (f *CommitImpactAnalysisFacade) GetByRunAndCommit(ctx context.Context, runID int64, commitSHA string) (*model.CommitImpactAnalysis, error) {
	var analysis model.CommitImpactAnalysis
	err := f.getDB().WithContext(ctx).
		Where("run_id = ? AND commit_sha = ?", runID, commitSHA).
		First(&analysis).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &analysis, nil
}

// ListByRun lists all analyses for a run
func (f *CommitImpactAnalysisFacade) ListByRun(ctx context.Context, runID int64) ([]*model.CommitImpactAnalysis, error) {
	var analyses []*model.CommitImpactAnalysis
	err := f.getDB().WithContext(ctx).
		Where("run_id = ?", runID).
		Order("impact_score DESC NULLS LAST, analyzed_at DESC").
		Find(&analyses).Error
	return analyses, err
}

// ListLikelyCausesByRun lists analyses marked as likely causes for a run
func (f *CommitImpactAnalysisFacade) ListLikelyCausesByRun(ctx context.Context, runID int64) ([]*model.CommitImpactAnalysis, error) {
	var analyses []*model.CommitImpactAnalysis
	err := f.getDB().WithContext(ctx).
		Where("run_id = ? AND is_likely_cause = ?", runID, true).
		Order("impact_score DESC NULLS LAST").
		Find(&analyses).Error
	return analyses, err
}

// ListByConfig lists all analyses for a config within a time range
func (f *CommitImpactAnalysisFacade) ListByConfig(ctx context.Context, configID int64, since *time.Time, limit int) ([]*model.CommitImpactAnalysis, error) {
	query := f.getDB().WithContext(ctx).Where("config_id = ?", configID)

	if since != nil {
		query = query.Where("analyzed_at >= ?", since)
	}

	query = query.Order("analyzed_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	var analyses []*model.CommitImpactAnalysis
	err := query.Find(&analyses).Error
	return analyses, err
}

// ListByCommitSHA lists all analyses for a specific commit SHA
func (f *CommitImpactAnalysisFacade) ListByCommitSHA(ctx context.Context, commitSHA string) ([]*model.CommitImpactAnalysis, error) {
	var analyses []*model.CommitImpactAnalysis
	err := f.getDB().WithContext(ctx).
		Where("commit_sha = ?", commitSHA).
		Order("analyzed_at DESC").
		Find(&analyses).Error
	return analyses, err
}

// Delete deletes a commit impact analysis
func (f *CommitImpactAnalysisFacade) Delete(ctx context.Context, id int64) error {
	return f.getDB().WithContext(ctx).Delete(&model.CommitImpactAnalysis{}, id).Error
}

// DeleteByRun deletes all analyses for a run
func (f *CommitImpactAnalysisFacade) DeleteByRun(ctx context.Context, runID int64) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Where("run_id = ?", runID).
		Delete(&model.CommitImpactAnalysis{})
	return result.RowsAffected, result.Error
}

// BulkCreate creates multiple analyses in batch
func (f *CommitImpactAnalysisFacade) BulkCreate(ctx context.Context, analyses []*model.CommitImpactAnalysis) error {
	if len(analyses) == 0 {
		return nil
	}
	return f.getDB().WithContext(ctx).CreateInBatches(analyses, 100).Error
}
