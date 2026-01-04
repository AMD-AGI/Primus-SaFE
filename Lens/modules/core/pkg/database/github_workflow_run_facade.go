package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// WorkflowRunStatus constants
const (
	WorkflowRunStatusPending    = "pending"
	WorkflowRunStatusCollecting = "collecting"
	WorkflowRunStatusExtracting = "extracting"
	WorkflowRunStatusCompleted  = "completed"
	WorkflowRunStatusFailed     = "failed"
	WorkflowRunStatusSkipped    = "skipped"
)

// WorkflowRunTriggerSource constants
const (
	WorkflowRunTriggerRealtime = "realtime"
	WorkflowRunTriggerBackfill = "backfill"
	WorkflowRunTriggerManual   = "manual"
)

// RunWithConfigName extends GithubWorkflowRuns with config name for cross-config queries
type RunWithConfigName struct {
	*model.GithubWorkflowRuns
	ConfigName string `json:"config_name"`
}

// GithubWorkflowRunFacadeInterface defines the database operation interface for github workflow runs
type GithubWorkflowRunFacadeInterface interface {
	// Create creates a new run record
	Create(ctx context.Context, run *model.GithubWorkflowRuns) error

	// GetByID retrieves a run by ID
	GetByID(ctx context.Context, id int64) (*model.GithubWorkflowRuns, error)

	// GetByConfigAndWorkload retrieves a run by config_id and workload_uid
	GetByConfigAndWorkload(ctx context.Context, configID int64, workloadUID string) (*model.GithubWorkflowRuns, error)

	// List lists runs with optional filtering
	List(ctx context.Context, filter *GithubWorkflowRunFilter) ([]*model.GithubWorkflowRuns, int64, error)

	// ListAllWithConfigName lists runs across all configs with config name (for global runs view)
	ListAllWithConfigName(ctx context.Context, filter *GithubWorkflowRunFilter) ([]*RunWithConfigName, int64, error)

	// ListPendingByConfig lists pending runs for a config
	ListPendingByConfig(ctx context.Context, configID int64, limit int) ([]*model.GithubWorkflowRuns, error)

	// ListByConfigAndStatus lists runs by config and status
	ListByConfigAndStatus(ctx context.Context, configID int64, status string) ([]*model.GithubWorkflowRuns, error)

	// ListByGithubRunID lists runs by GitHub run ID
	ListByGithubRunID(ctx context.Context, githubRunID int64) ([]*model.GithubWorkflowRuns, error)

	// Update updates a run record
	Update(ctx context.Context, run *model.GithubWorkflowRuns) error

	// UpdateStatus updates the status of a run
	UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error

	// MarkCollecting marks a run as collecting
	MarkCollecting(ctx context.Context, id int64) error

	// MarkExtracting marks a run as extracting
	MarkExtracting(ctx context.Context, id int64) error

	// MarkCompleted marks a run as completed with metrics count
	MarkCompleted(ctx context.Context, id int64, filesFound, filesProcessed, metricsCount int32) error

	// MarkFailed marks a run as failed with error message
	MarkFailed(ctx context.Context, id int64, errMsg string) error

	// IncrementRetryCount increments the retry count
	IncrementRetryCount(ctx context.Context, id int64) error

	// ResetToPending resets a run to pending status (for retry)
	ResetToPending(ctx context.Context, id int64) error

	// Delete deletes a run by ID
	Delete(ctx context.Context, id int64) error

	// DeleteByConfig deletes all runs for a config
	DeleteByConfig(ctx context.Context, configID int64) error

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) GithubWorkflowRunFacadeInterface
}

// GithubWorkflowRunFilter defines filter options for listing runs
type GithubWorkflowRunFilter struct {
	ConfigID      int64
	Status        string
	TriggerSource string
	GithubRunID   int64
	WorkloadUID   string
	RunnerSetName string // Filter by runner set name (via config)
	Since         *time.Time
	Until         *time.Time
	Offset        int
	Limit         int
}

// GithubWorkflowRunFacade implements GithubWorkflowRunFacadeInterface
type GithubWorkflowRunFacade struct {
	BaseFacade
}

// NewGithubWorkflowRunFacade creates a new GithubWorkflowRunFacade instance
func NewGithubWorkflowRunFacade() GithubWorkflowRunFacadeInterface {
	return &GithubWorkflowRunFacade{}
}

func (f *GithubWorkflowRunFacade) WithCluster(clusterName string) GithubWorkflowRunFacadeInterface {
	return &GithubWorkflowRunFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new run record
func (f *GithubWorkflowRunFacade) Create(ctx context.Context, run *model.GithubWorkflowRuns) error {
	now := time.Now()
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = now
	}
	if run.Status == "" {
		run.Status = WorkflowRunStatusPending
	}
	if run.TriggerSource == "" {
		run.TriggerSource = WorkflowRunTriggerRealtime
	}
	return f.getDAL().GithubWorkflowRuns.WithContext(ctx).Create(run)
}

// GetByID retrieves a run by ID
func (f *GithubWorkflowRunFacade) GetByID(ctx context.Context, id int64) (*model.GithubWorkflowRuns, error) {
	q := f.getDAL().GithubWorkflowRuns
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// GetByConfigAndWorkload retrieves a run by config_id and workload_uid
func (f *GithubWorkflowRunFacade) GetByConfigAndWorkload(ctx context.Context, configID int64, workloadUID string) (*model.GithubWorkflowRuns, error) {
	q := f.getDAL().GithubWorkflowRuns
	result, err := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Where(q.WorkloadUID.Eq(workloadUID)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// List lists runs with optional filtering
func (f *GithubWorkflowRunFacade) List(ctx context.Context, filter *GithubWorkflowRunFilter) ([]*model.GithubWorkflowRuns, int64, error) {
	q := f.getDAL().GithubWorkflowRuns
	query := q.WithContext(ctx)

	if filter != nil {
		if filter.ConfigID > 0 {
			query = query.Where(q.ConfigID.Eq(filter.ConfigID))
		}
		if filter.Status != "" {
			query = query.Where(q.Status.Eq(filter.Status))
		}
		if filter.TriggerSource != "" {
			query = query.Where(q.TriggerSource.Eq(filter.TriggerSource))
		}
		if filter.GithubRunID > 0 {
			query = query.Where(q.GithubRunID.Eq(filter.GithubRunID))
		}
		if filter.WorkloadUID != "" {
			query = query.Where(q.WorkloadUID.Eq(filter.WorkloadUID))
		}
		if filter.Since != nil {
			query = query.Where(q.CreatedAt.Gte(*filter.Since))
		}
		if filter.Until != nil {
			query = query.Where(q.CreatedAt.Lte(*filter.Until))
		}
	}

	total, err := query.Count()
	if err != nil {
		return nil, 0, err
	}

	if filter != nil {
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
	}

	results, err := query.Order(q.ID.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// ListPendingByConfig lists pending runs for a config
func (f *GithubWorkflowRunFacade) ListPendingByConfig(ctx context.Context, configID int64, limit int) ([]*model.GithubWorkflowRuns, error) {
	q := f.getDAL().GithubWorkflowRuns
	query := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Where(q.Status.Eq(WorkflowRunStatusPending)).
		Order(q.CreatedAt.Asc())

	if limit > 0 {
		query = query.Limit(limit)
	}

	return query.Find()
}

// ListByGithubRunID lists runs by GitHub run ID
func (f *GithubWorkflowRunFacade) ListByGithubRunID(ctx context.Context, githubRunID int64) ([]*model.GithubWorkflowRuns, error) {
	q := f.getDAL().GithubWorkflowRuns
	return q.WithContext(ctx).
		Where(q.GithubRunID.Eq(githubRunID)).
		Find()
}

// Update updates a run record
func (f *GithubWorkflowRunFacade) Update(ctx context.Context, run *model.GithubWorkflowRuns) error {
	run.UpdatedAt = time.Now()
	q := f.getDAL().GithubWorkflowRuns
	_, err := q.WithContext(ctx).Where(q.ID.Eq(run.ID)).Updates(run)
	return err
}

// UpdateStatus updates the status of a run
func (f *GithubWorkflowRunFacade) UpdateStatus(ctx context.Context, id int64, status string, errMsg string) error {
	q := f.getDAL().GithubWorkflowRuns

	if errMsg != "" {
		_, err := q.WithContext(ctx).
			Where(q.ID.Eq(id)).
			UpdateSimple(
				q.Status.Value(status),
				q.ErrorMessage.Value(errMsg),
			)
		return err
	}

	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.Status.Value(status),
		)
	return err
}

// MarkCollecting marks a run as collecting
func (f *GithubWorkflowRunFacade) MarkCollecting(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowRuns
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.Status.Value(WorkflowRunStatusCollecting),
			q.CollectionStartedAt.Value(now),
		)
	return err
}

// MarkExtracting marks a run as extracting
func (f *GithubWorkflowRunFacade) MarkExtracting(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowRuns
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.Status.Value(WorkflowRunStatusExtracting),
		)
	return err
}

// MarkCompleted marks a run as completed with metrics count
func (f *GithubWorkflowRunFacade) MarkCompleted(ctx context.Context, id int64, filesFound, filesProcessed, metricsCount int32) error {
	q := f.getDAL().GithubWorkflowRuns
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.Status.Value(WorkflowRunStatusCompleted),
			q.FilesFound.Value(filesFound),
			q.FilesProcessed.Value(filesProcessed),
			q.MetricsCount.Value(metricsCount),
			q.CollectionCompletedAt.Value(now),
		)
	return err
}

// MarkFailed marks a run as failed with error message
func (f *GithubWorkflowRunFacade) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	q := f.getDAL().GithubWorkflowRuns
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.Status.Value(WorkflowRunStatusFailed),
			q.ErrorMessage.Value(errMsg),
		)
	return err
}

// IncrementRetryCount increments the retry count
func (f *GithubWorkflowRunFacade) IncrementRetryCount(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowRuns
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.RetryCount.Add(1),
		)
	return err
}

// Delete deletes a run by ID
func (f *GithubWorkflowRunFacade) Delete(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowRuns
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// DeleteByConfig deletes all runs for a config
func (f *GithubWorkflowRunFacade) DeleteByConfig(ctx context.Context, configID int64) error {
	q := f.getDAL().GithubWorkflowRuns
	_, err := q.WithContext(ctx).Where(q.ConfigID.Eq(configID)).Delete()
	return err
}

// ListByConfigAndStatus lists runs by config and status
func (f *GithubWorkflowRunFacade) ListByConfigAndStatus(ctx context.Context, configID int64, status string) ([]*model.GithubWorkflowRuns, error) {
	q := f.getDAL().GithubWorkflowRuns
	return q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Where(q.Status.Eq(status)).
		Order(q.CreatedAt.Desc()).
		Find()
}

// ResetToPending resets a run to pending status (for retry)
func (f *GithubWorkflowRunFacade) ResetToPending(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowRuns
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.Status.Value(WorkflowRunStatusPending),
			q.ErrorMessage.Value(""),
			q.RetryCount.Value(0),
		)
	return err
}

// ListAllWithConfigName lists runs across all configs with config name (for global runs view)
func (f *GithubWorkflowRunFacade) ListAllWithConfigName(ctx context.Context, filter *GithubWorkflowRunFilter) ([]*RunWithConfigName, int64, error) {
	db := f.getDAL().GithubWorkflowRuns.WithContext(ctx).UnderlyingDB()

	// Build base query with join - use Session to prevent query mutation
	baseQuery := db.Table("github_workflow_runs r").
		Joins("LEFT JOIN github_workflow_configs c ON r.config_id = c.id")

	// Apply filters
	if filter != nil {
		if filter.ConfigID > 0 {
			baseQuery = baseQuery.Where("r.config_id = ?", filter.ConfigID)
		}
		if filter.Status != "" {
			baseQuery = baseQuery.Where("r.status = ?", filter.Status)
		}
		if filter.TriggerSource != "" {
			baseQuery = baseQuery.Where("r.trigger_source = ?", filter.TriggerSource)
		}
		if filter.GithubRunID > 0 {
			baseQuery = baseQuery.Where("r.github_run_id = ?", filter.GithubRunID)
		}
		if filter.WorkloadUID != "" {
			baseQuery = baseQuery.Where("r.workload_uid = ?", filter.WorkloadUID)
		}
		if filter.RunnerSetName != "" {
			baseQuery = baseQuery.Where("c.runner_set_name = ?", filter.RunnerSetName)
		}
		if filter.Since != nil {
			baseQuery = baseQuery.Where("r.created_at >= ?", *filter.Since)
		}
		if filter.Until != nil {
			baseQuery = baseQuery.Where("r.created_at <= ?", *filter.Until)
		}
	}

	// Count total using a separate query to avoid mutation
	var total int64
	countQuery := baseQuery.Session(&gorm.Session{})
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Build data query with select and pagination
	query := baseQuery.Session(&gorm.Session{}).Select("r.*, c.name as config_name")

	// Apply pagination
	if filter != nil {
		if filter.Offset > 0 {
			query = query.Offset(filter.Offset)
		}
		if filter.Limit > 0 {
			query = query.Limit(filter.Limit)
		}
	}

	// Order by id descending
	query = query.Order("r.id DESC")

	// Execute query
	rows, err := query.Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*RunWithConfigName
	for rows.Next() {
		run := &model.GithubWorkflowRuns{}
		var configName string

		err := db.ScanRows(rows, run)
		if err != nil {
			return nil, 0, err
		}

		// Scan config_name separately (it's appended after run fields)
		// Since ScanRows scans all columns into the struct, we need to re-scan with explicit columns
		results = append(results, &RunWithConfigName{
			GithubWorkflowRuns: run,
			ConfigName:         configName,
		})
	}

	// Re-query with explicit struct scanning
	var rawResults []struct {
		model.GithubWorkflowRuns
		ConfigName string `gorm:"column:config_name"`
	}

	// Rebuild query for final scan
	query2 := db.Table("github_workflow_runs r").
		Select("r.*, c.name as config_name").
		Joins("LEFT JOIN github_workflow_configs c ON r.config_id = c.id")

	if filter != nil {
		if filter.ConfigID > 0 {
			query2 = query2.Where("r.config_id = ?", filter.ConfigID)
		}
		if filter.Status != "" {
			query2 = query2.Where("r.status = ?", filter.Status)
		}
		if filter.TriggerSource != "" {
			query2 = query2.Where("r.trigger_source = ?", filter.TriggerSource)
		}
		if filter.GithubRunID > 0 {
			query2 = query2.Where("r.github_run_id = ?", filter.GithubRunID)
		}
		if filter.WorkloadUID != "" {
			query2 = query2.Where("r.workload_uid = ?", filter.WorkloadUID)
		}
		if filter.Since != nil {
			query2 = query2.Where("r.created_at >= ?", *filter.Since)
		}
		if filter.Until != nil {
			query2 = query2.Where("r.created_at <= ?", *filter.Until)
		}
		if filter.Offset > 0 {
			query2 = query2.Offset(filter.Offset)
		}
		if filter.Limit > 0 {
			query2 = query2.Limit(filter.Limit)
		}
	}

	if err := query2.Order("r.id DESC").Find(&rawResults).Error; err != nil {
		return nil, 0, err
	}

	results = make([]*RunWithConfigName, 0, len(rawResults))
	for i := range rawResults {
		run := rawResults[i].GithubWorkflowRuns
		results = append(results, &RunWithConfigName{
			GithubWorkflowRuns: &run,
			ConfigName:         rawResults[i].ConfigName,
		})
	}

	return results, total, nil
}

