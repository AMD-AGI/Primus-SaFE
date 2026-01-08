package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// GithubWorkflowConfigFacadeInterface defines the database operation interface for github workflow configs
type GithubWorkflowConfigFacadeInterface interface {
	// Create creates a new config record
	Create(ctx context.Context, config *model.GithubWorkflowConfigs) error

	// GetByID retrieves a config by ID
	GetByID(ctx context.Context, id int64) (*model.GithubWorkflowConfigs, error)

	// GetByRunnerSetID retrieves a config by runner_set_id
	GetByRunnerSetID(ctx context.Context, runnerSetID int64) (*model.GithubWorkflowConfigs, error)

	// ListByRunnerSetID lists all configs for a runner set
	ListByRunnerSetID(ctx context.Context, runnerSetID int64) ([]*model.GithubWorkflowConfigs, error)

	// GetByRunnerSet retrieves a config by runner set namespace, name, and cluster
	GetByRunnerSet(ctx context.Context, namespace, name, clusterName string) (*model.GithubWorkflowConfigs, error)

	// List lists configs with optional filtering
	List(ctx context.Context, filter *GithubWorkflowConfigFilter) ([]*model.GithubWorkflowConfigs, int64, error)

	// ListEnabled lists all enabled configs
	ListEnabled(ctx context.Context) ([]*model.GithubWorkflowConfigs, error)

	// Update updates a config record
	Update(ctx context.Context, config *model.GithubWorkflowConfigs) error

	// UpdateLastChecked updates the last_checked_at timestamp
	UpdateLastChecked(ctx context.Context, id int64) error

	// UpdateLastProcessedWorkload updates the last_processed_workload_uid
	UpdateLastProcessedWorkload(ctx context.Context, id int64, workloadUID string) error

	// UpdateMetricSchemaID updates the metric_schema_id
	UpdateMetricSchemaID(ctx context.Context, id int64, schemaID int64) error

	// Delete deletes a config by ID
	Delete(ctx context.Context, id int64) error

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) GithubWorkflowConfigFacadeInterface
}

// GithubWorkflowConfigFilter defines filter options for listing configs
type GithubWorkflowConfigFilter struct {
	RunnerSetID int64
	Enabled     *bool
	ClusterName string
	GithubOwner string
	GithubRepo  string
	Offset      int
	Limit       int
}

// GithubWorkflowConfigFacade implements GithubWorkflowConfigFacadeInterface
type GithubWorkflowConfigFacade struct {
	BaseFacade
}

// NewGithubWorkflowConfigFacade creates a new GithubWorkflowConfigFacade instance
func NewGithubWorkflowConfigFacade() GithubWorkflowConfigFacadeInterface {
	return &GithubWorkflowConfigFacade{}
}

func (f *GithubWorkflowConfigFacade) WithCluster(clusterName string) GithubWorkflowConfigFacadeInterface {
	return &GithubWorkflowConfigFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new config record
func (f *GithubWorkflowConfigFacade) Create(ctx context.Context, config *model.GithubWorkflowConfigs) error {
	now := time.Now()
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}
	if config.UpdatedAt.IsZero() {
		config.UpdatedAt = now
	}
	return f.getDAL().GithubWorkflowConfigs.WithContext(ctx).Create(config)
}

// GetByID retrieves a config by ID
func (f *GithubWorkflowConfigFacade) GetByID(ctx context.Context, id int64) (*model.GithubWorkflowConfigs, error) {
	q := f.getDAL().GithubWorkflowConfigs
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// GetByRunnerSetID retrieves a config by runner_set_id
func (f *GithubWorkflowConfigFacade) GetByRunnerSetID(ctx context.Context, runnerSetID int64) (*model.GithubWorkflowConfigs, error) {
	q := f.getDAL().GithubWorkflowConfigs
	result, err := q.WithContext(ctx).
		Where(q.RunnerSetID.Eq(runnerSetID)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// ListByRunnerSetID lists all configs for a runner set
func (f *GithubWorkflowConfigFacade) ListByRunnerSetID(ctx context.Context, runnerSetID int64) ([]*model.GithubWorkflowConfigs, error) {
	q := f.getDAL().GithubWorkflowConfigs
	return q.WithContext(ctx).
		Where(q.RunnerSetID.Eq(runnerSetID)).
		Order(q.ID.Desc()).
		Find()
}

// GetByRunnerSet retrieves a config by runner set namespace, name, and cluster
func (f *GithubWorkflowConfigFacade) GetByRunnerSet(ctx context.Context, namespace, name, clusterName string) (*model.GithubWorkflowConfigs, error) {
	q := f.getDAL().GithubWorkflowConfigs
	result, err := q.WithContext(ctx).
		Where(q.RunnerSetNamespace.Eq(namespace)).
		Where(q.RunnerSetName.Eq(name)).
		Where(q.ClusterName.Eq(clusterName)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// List lists configs with optional filtering
func (f *GithubWorkflowConfigFacade) List(ctx context.Context, filter *GithubWorkflowConfigFilter) ([]*model.GithubWorkflowConfigs, int64, error) {
	q := f.getDAL().GithubWorkflowConfigs
	query := q.WithContext(ctx)

	if filter != nil {
		if filter.RunnerSetID > 0 {
			query = query.Where(q.RunnerSetID.Eq(filter.RunnerSetID))
		}
		if filter.Enabled != nil {
			query = query.Where(q.Enabled.Is(*filter.Enabled))
		}
		if filter.ClusterName != "" {
			query = query.Where(q.ClusterName.Eq(filter.ClusterName))
		}
		if filter.GithubOwner != "" {
			query = query.Where(q.GithubOwner.Eq(filter.GithubOwner))
		}
		if filter.GithubRepo != "" {
			query = query.Where(q.GithubRepo.Eq(filter.GithubRepo))
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

// ListEnabled lists all enabled configs
func (f *GithubWorkflowConfigFacade) ListEnabled(ctx context.Context) ([]*model.GithubWorkflowConfigs, error) {
	q := f.getDAL().GithubWorkflowConfigs
	return q.WithContext(ctx).Where(q.Enabled.Is(true)).Find()
}

// Update updates a config record
func (f *GithubWorkflowConfigFacade) Update(ctx context.Context, config *model.GithubWorkflowConfigs) error {
	config.UpdatedAt = time.Now()
	q := f.getDAL().GithubWorkflowConfigs
	_, err := q.WithContext(ctx).Where(q.ID.Eq(config.ID)).Updates(config)
	return err
}

// UpdateLastChecked updates the last_checked_at timestamp
func (f *GithubWorkflowConfigFacade) UpdateLastChecked(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowConfigs
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.LastCheckedAt.Value(now),
			q.UpdatedAt.Value(now),
		)
	return err
}

// UpdateLastProcessedWorkload updates the last_processed_workload_uid
func (f *GithubWorkflowConfigFacade) UpdateLastProcessedWorkload(ctx context.Context, id int64, workloadUID string) error {
	q := f.getDAL().GithubWorkflowConfigs
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.LastProcessedWorkloadUID.Value(workloadUID),
			q.UpdatedAt.Value(now),
		)
	return err
}

// UpdateMetricSchemaID updates the metric_schema_id
func (f *GithubWorkflowConfigFacade) UpdateMetricSchemaID(ctx context.Context, id int64, schemaID int64) error {
	q := f.getDAL().GithubWorkflowConfigs
	now := time.Now()
	_, err := q.WithContext(ctx).
		Where(q.ID.Eq(id)).
		UpdateSimple(
			q.MetricSchemaID.Value(schemaID),
			q.UpdatedAt.Value(now),
		)
	return err
}

// Delete deletes a config by ID
func (f *GithubWorkflowConfigFacade) Delete(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowConfigs
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

