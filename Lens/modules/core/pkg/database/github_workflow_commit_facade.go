package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GithubWorkflowCommitFacadeInterface defines the interface for GithubWorkflowCommit operations
type GithubWorkflowCommitFacadeInterface interface {
	// Upsert creates or updates a commit record
	Upsert(ctx context.Context, commit *model.GithubWorkflowCommits) error
	// GetByRunID gets a commit by run ID
	GetByRunID(ctx context.Context, runID int64) (*model.GithubWorkflowCommits, error)
	// GetBySHA gets a commit by SHA
	GetBySHA(ctx context.Context, sha string) (*model.GithubWorkflowCommits, error)
	// ListByAuthor lists commits by author email
	ListByAuthor(ctx context.Context, email string, since *time.Time, limit int) ([]*model.GithubWorkflowCommits, error)
	// GetStats gets commit statistics for a time range
	GetStats(ctx context.Context, since, until *time.Time) (*CommitStats, error)
}

// CommitStats represents commit statistics
type CommitStats struct {
	TotalCommits   int64   `json:"total_commits"`
	TotalAdditions int64   `json:"total_additions"`
	TotalDeletions int64   `json:"total_deletions"`
	TotalFiles     int64   `json:"total_files"`
	AvgAdditions   float64 `json:"avg_additions"`
	AvgDeletions   float64 `json:"avg_deletions"`
	UniqueAuthors  int64   `json:"unique_authors"`
}

// GithubWorkflowCommitFacade implements GithubWorkflowCommitFacadeInterface
type GithubWorkflowCommitFacade struct {
	BaseFacade
}

// NewGithubWorkflowCommitFacade creates a new GithubWorkflowCommitFacade
func NewGithubWorkflowCommitFacade() *GithubWorkflowCommitFacade {
	return &GithubWorkflowCommitFacade{}
}

// Upsert creates or updates a commit record
func (f *GithubWorkflowCommitFacade) Upsert(ctx context.Context, commit *model.GithubWorkflowCommits) error {
	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"sha", "message", "author_name", "author_email", "author_date",
			"committer_name", "committer_email", "committer_date",
			"additions", "deletions", "files_changed", "parent_shas", "files", "html_url",
		}),
	}).Create(commit).Error
}

// GetByRunID gets a commit by run ID
func (f *GithubWorkflowCommitFacade) GetByRunID(ctx context.Context, runID int64) (*model.GithubWorkflowCommits, error) {
	var commit model.GithubWorkflowCommits
	err := f.getDB().WithContext(ctx).Where("run_id = ?", runID).First(&commit).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &commit, nil
}

// GetBySHA gets a commit by SHA
func (f *GithubWorkflowCommitFacade) GetBySHA(ctx context.Context, sha string) (*model.GithubWorkflowCommits, error) {
	var commit model.GithubWorkflowCommits
	err := f.getDB().WithContext(ctx).Where("sha = ?", sha).First(&commit).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &commit, nil
}

// ListByAuthor lists commits by author email
func (f *GithubWorkflowCommitFacade) ListByAuthor(ctx context.Context, email string, since *time.Time, limit int) ([]*model.GithubWorkflowCommits, error) {
	query := f.getDB().WithContext(ctx).Where("author_email = ?", email)
	
	if since != nil {
		query = query.Where("author_date >= ?", since)
	}
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	var commits []*model.GithubWorkflowCommits
	err := query.Order("author_date DESC").Find(&commits).Error
	return commits, err
}

// GetStats gets commit statistics for a time range
func (f *GithubWorkflowCommitFacade) GetStats(ctx context.Context, since, until *time.Time) (*CommitStats, error) {
	query := f.getDB().WithContext(ctx).Model(&model.GithubWorkflowCommits{})
	
	if since != nil {
		query = query.Where("author_date >= ?", since)
	}
	if until != nil {
		query = query.Where("author_date <= ?", until)
	}
	
	var stats CommitStats
	
	// Get aggregate stats
	err := query.Select(`
		COUNT(*) as total_commits,
		COALESCE(SUM(additions), 0) as total_additions,
		COALESCE(SUM(deletions), 0) as total_deletions,
		COALESCE(SUM(files_changed), 0) as total_files,
		COALESCE(AVG(additions), 0) as avg_additions,
		COALESCE(AVG(deletions), 0) as avg_deletions,
		COUNT(DISTINCT author_email) as unique_authors
	`).Scan(&stats).Error
	
	return &stats, err
}

