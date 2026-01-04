package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GithubWorkflowRunDetailsFacadeInterface defines the interface for GithubWorkflowRunDetails operations
type GithubWorkflowRunDetailsFacadeInterface interface {
	// Upsert creates or updates a run details record
	Upsert(ctx context.Context, details *model.GithubWorkflowRunDetails) error
	// GetByRunID gets run details by run ID
	GetByRunID(ctx context.Context, runID int64) (*model.GithubWorkflowRunDetails, error)
	// GetByGithubRunID gets run details by GitHub run ID
	GetByGithubRunID(ctx context.Context, githubRunID int64) (*model.GithubWorkflowRunDetails, error)
	// ListByConclusion lists run details by conclusion
	ListByConclusion(ctx context.Context, conclusion string, since *time.Time, limit int) ([]*model.GithubWorkflowRunDetails, error)
	// GetAnalytics gets workflow run analytics
	GetAnalytics(ctx context.Context, filter *WorkflowAnalyticsFilter) (*WorkflowAnalytics, error)
	// WithCluster returns a new facade instance using the specified cluster
	WithCluster(clusterName string) GithubWorkflowRunDetailsFacadeInterface
}

// WorkflowAnalyticsFilter defines the filter for analytics query
type WorkflowAnalyticsFilter struct {
	Since        *time.Time
	Until        *time.Time
	WorkflowName string
	Event        string
	Branch       string
}

// WorkflowAnalytics represents workflow run analytics
type WorkflowAnalytics struct {
	TotalRuns           int64              `json:"total_runs"`
	SuccessfulRuns      int64              `json:"successful_runs"`
	FailedRuns          int64              `json:"failed_runs"`
	CancelledRuns       int64              `json:"cancelled_runs"`
	SuccessRate         float64            `json:"success_rate"`
	AvgDurationSeconds  float64            `json:"avg_duration_seconds"`
	TotalDurationSeconds int64             `json:"total_duration_seconds"`
	RunsByDay           []DailyRunStats    `json:"runs_by_day,omitempty"`
	RunsByEvent         map[string]int64   `json:"runs_by_event,omitempty"`
	RunsByBranch        map[string]int64   `json:"runs_by_branch,omitempty"`
	RunsByConclusion    map[string]int64   `json:"runs_by_conclusion,omitempty"`
}

// DailyRunStats represents daily run statistics
type DailyRunStats struct {
	Date            string  `json:"date"`
	TotalRuns       int64   `json:"total_runs"`
	SuccessfulRuns  int64   `json:"successful_runs"`
	FailedRuns      int64   `json:"failed_runs"`
	AvgDuration     float64 `json:"avg_duration_seconds"`
}

// GithubWorkflowRunDetailsFacade implements GithubWorkflowRunDetailsFacadeInterface
type GithubWorkflowRunDetailsFacade struct {
	BaseFacade
}

// NewGithubWorkflowRunDetailsFacade creates a new GithubWorkflowRunDetailsFacade
func NewGithubWorkflowRunDetailsFacade() *GithubWorkflowRunDetailsFacade {
	return &GithubWorkflowRunDetailsFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *GithubWorkflowRunDetailsFacade) WithCluster(clusterName string) GithubWorkflowRunDetailsFacadeInterface {
	return &GithubWorkflowRunDetailsFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Upsert creates or updates a run details record
func (f *GithubWorkflowRunDetailsFacade) Upsert(ctx context.Context, details *model.GithubWorkflowRunDetails) error {
	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "run_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"github_run_id", "github_run_number", "github_run_attempt",
			"workflow_id", "workflow_name", "workflow_path",
			"status", "conclusion",
			"html_url", "jobs_url", "logs_url", "artifacts_url",
			"created_at_github", "updated_at_github", "run_started_at", "run_completed_at",
			"duration_seconds", "event", "trigger_actor", "trigger_actor_id",
			"head_sha", "head_branch", "head_repository_full_name",
			"base_sha", "base_branch",
			"pull_request_number", "pull_request_title", "pull_request_url",
			"jobs",
		}),
	}).Create(details).Error
}

// GetByRunID gets run details by run ID
func (f *GithubWorkflowRunDetailsFacade) GetByRunID(ctx context.Context, runID int64) (*model.GithubWorkflowRunDetails, error) {
	var details model.GithubWorkflowRunDetails
	err := f.getDB().WithContext(ctx).Where("run_id = ?", runID).First(&details).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &details, nil
}

// GetByGithubRunID gets run details by GitHub run ID
func (f *GithubWorkflowRunDetailsFacade) GetByGithubRunID(ctx context.Context, githubRunID int64) (*model.GithubWorkflowRunDetails, error) {
	var details model.GithubWorkflowRunDetails
	err := f.getDB().WithContext(ctx).Where("github_run_id = ?", githubRunID).First(&details).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &details, nil
}

// ListByConclusion lists run details by conclusion
func (f *GithubWorkflowRunDetailsFacade) ListByConclusion(ctx context.Context, conclusion string, since *time.Time, limit int) ([]*model.GithubWorkflowRunDetails, error) {
	query := f.getDB().WithContext(ctx).Where("conclusion = ?", conclusion)
	
	if since != nil {
		query = query.Where("created_at_github >= ?", since)
	}
	
	if limit > 0 {
		query = query.Limit(limit)
	}
	
	var details []*model.GithubWorkflowRunDetails
	err := query.Order("created_at_github DESC").Find(&details).Error
	return details, err
}

// GetAnalytics gets workflow run analytics
func (f *GithubWorkflowRunDetailsFacade) GetAnalytics(ctx context.Context, filter *WorkflowAnalyticsFilter) (*WorkflowAnalytics, error) {
	query := f.getDB().WithContext(ctx).Model(&model.GithubWorkflowRunDetails{})
	
	if filter != nil {
		if filter.Since != nil {
			query = query.Where("created_at_github >= ?", filter.Since)
		}
		if filter.Until != nil {
			query = query.Where("created_at_github <= ?", filter.Until)
		}
		if filter.WorkflowName != "" {
			query = query.Where("workflow_name = ?", filter.WorkflowName)
		}
		if filter.Event != "" {
			query = query.Where("event = ?", filter.Event)
		}
		if filter.Branch != "" {
			query = query.Where("head_branch = ?", filter.Branch)
		}
	}
	
	analytics := &WorkflowAnalytics{
		RunsByEvent:      make(map[string]int64),
		RunsByBranch:     make(map[string]int64),
		RunsByConclusion: make(map[string]int64),
	}
	
	// Get basic stats
	var basicStats struct {
		TotalRuns            int64
		SuccessfulRuns       int64
		FailedRuns           int64
		CancelledRuns        int64
		AvgDurationSeconds   float64
		TotalDurationSeconds int64
	}
	
	err := query.Select(`
		COUNT(*) as total_runs,
		COUNT(CASE WHEN conclusion = 'success' THEN 1 END) as successful_runs,
		COUNT(CASE WHEN conclusion = 'failure' THEN 1 END) as failed_runs,
		COUNT(CASE WHEN conclusion = 'cancelled' THEN 1 END) as cancelled_runs,
		COALESCE(AVG(duration_seconds), 0) as avg_duration_seconds,
		COALESCE(SUM(duration_seconds), 0) as total_duration_seconds
	`).Scan(&basicStats).Error
	
	if err != nil {
		return nil, err
	}
	
	analytics.TotalRuns = basicStats.TotalRuns
	analytics.SuccessfulRuns = basicStats.SuccessfulRuns
	analytics.FailedRuns = basicStats.FailedRuns
	analytics.CancelledRuns = basicStats.CancelledRuns
	analytics.AvgDurationSeconds = basicStats.AvgDurationSeconds
	analytics.TotalDurationSeconds = basicStats.TotalDurationSeconds
	
	if analytics.TotalRuns > 0 {
		analytics.SuccessRate = float64(analytics.SuccessfulRuns) / float64(analytics.TotalRuns) * 100
	}
	
	// Get runs by event
	var eventStats []struct {
		Event string
		Count int64
	}
	query.Select("event, COUNT(*) as count").Group("event").Scan(&eventStats)
	for _, e := range eventStats {
		analytics.RunsByEvent[e.Event] = e.Count
	}
	
	// Get runs by branch (top 10)
	var branchStats []struct {
		HeadBranch string
		Count      int64
	}
	query.Select("head_branch, COUNT(*) as count").Group("head_branch").Order("count DESC").Limit(10).Scan(&branchStats)
	for _, b := range branchStats {
		analytics.RunsByBranch[b.HeadBranch] = b.Count
	}
	
	// Get runs by conclusion
	var conclusionStats []struct {
		Conclusion string
		Count      int64
	}
	query.Select("conclusion, COUNT(*) as count").Group("conclusion").Scan(&conclusionStats)
	for _, c := range conclusionStats {
		analytics.RunsByConclusion[c.Conclusion] = c.Count
	}
	
	return analytics, nil
}

