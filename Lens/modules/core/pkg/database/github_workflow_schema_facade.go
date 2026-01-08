package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// SchemaGeneratedBy constants
const (
	SchemaGeneratedByAI     = "ai"
	SchemaGeneratedByUser   = "user"
	SchemaGeneratedBySystem = "system"
)

// GithubWorkflowSchemaFacadeInterface defines the database operation interface for github workflow metric schemas
type GithubWorkflowSchemaFacadeInterface interface {
	// Create creates a new schema record
	Create(ctx context.Context, schema *model.GithubWorkflowMetricSchemas) error

	// GetByID retrieves a schema by ID
	GetByID(ctx context.Context, id int64) (*model.GithubWorkflowMetricSchemas, error)

	// GetActiveByConfig retrieves the active schema for a config
	GetActiveByConfig(ctx context.Context, configID int64) (*model.GithubWorkflowMetricSchemas, error)

	// GetByConfigAndVersion retrieves a schema by config_id and version
	GetByConfigAndVersion(ctx context.Context, configID int64, version int32) (*model.GithubWorkflowMetricSchemas, error)

	// ListByConfig lists all schemas for a config
	ListByConfig(ctx context.Context, configID int64) ([]*model.GithubWorkflowMetricSchemas, error)

	// GetLatestVersion gets the latest version number for a config
	GetLatestVersion(ctx context.Context, configID int64) (int32, error)

	// Update updates a schema record
	Update(ctx context.Context, schema *model.GithubWorkflowMetricSchemas) error

	// SetActive sets a schema as active (and deactivates others for the same config)
	SetActive(ctx context.Context, configID int64, schemaID int64) error

	// Delete deletes a schema by ID
	Delete(ctx context.Context, id int64) error

	// DeleteByConfig deletes all schemas for a config
	DeleteByConfig(ctx context.Context, configID int64) error

	// WithCluster returns a new facade instance for the specified cluster
	WithCluster(clusterName string) GithubWorkflowSchemaFacadeInterface
}

// GithubWorkflowSchemaFacade implements GithubWorkflowSchemaFacadeInterface
type GithubWorkflowSchemaFacade struct {
	BaseFacade
}

// NewGithubWorkflowSchemaFacade creates a new GithubWorkflowSchemaFacade instance
func NewGithubWorkflowSchemaFacade() GithubWorkflowSchemaFacadeInterface {
	return &GithubWorkflowSchemaFacade{}
}

func (f *GithubWorkflowSchemaFacade) WithCluster(clusterName string) GithubWorkflowSchemaFacadeInterface {
	return &GithubWorkflowSchemaFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Create creates a new schema record
func (f *GithubWorkflowSchemaFacade) Create(ctx context.Context, schema *model.GithubWorkflowMetricSchemas) error {
	now := time.Now()
	if schema.CreatedAt.IsZero() {
		schema.CreatedAt = now
	}
	if schema.UpdatedAt.IsZero() {
		schema.UpdatedAt = now
	}
	if schema.Version == 0 {
		// Auto-increment version
		latestVersion, err := f.GetLatestVersion(ctx, schema.ConfigID)
		if err != nil {
			return err
		}
		schema.Version = latestVersion + 1
	}
	return f.getDAL().GithubWorkflowMetricSchemas.WithContext(ctx).Create(schema)
}

// GetByID retrieves a schema by ID
func (f *GithubWorkflowSchemaFacade) GetByID(ctx context.Context, id int64) (*model.GithubWorkflowMetricSchemas, error) {
	q := f.getDAL().GithubWorkflowMetricSchemas
	result, err := q.WithContext(ctx).Where(q.ID.Eq(id)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// GetActiveByConfig retrieves the active schema for a config
func (f *GithubWorkflowSchemaFacade) GetActiveByConfig(ctx context.Context, configID int64) (*model.GithubWorkflowMetricSchemas, error) {
	q := f.getDAL().GithubWorkflowMetricSchemas
	result, err := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Where(q.IsActive.Is(true)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// GetByConfigAndVersion retrieves a schema by config_id and version
func (f *GithubWorkflowSchemaFacade) GetByConfigAndVersion(ctx context.Context, configID int64, version int32) (*model.GithubWorkflowMetricSchemas, error) {
	q := f.getDAL().GithubWorkflowMetricSchemas
	result, err := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Where(q.Version.Eq(version)).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// ListByConfig lists all schemas for a config
func (f *GithubWorkflowSchemaFacade) ListByConfig(ctx context.Context, configID int64) ([]*model.GithubWorkflowMetricSchemas, error) {
	q := f.getDAL().GithubWorkflowMetricSchemas
	return q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Order(q.Version.Desc()).
		Find()
}

// GetLatestVersion gets the latest version number for a config
func (f *GithubWorkflowSchemaFacade) GetLatestVersion(ctx context.Context, configID int64) (int32, error) {
	q := f.getDAL().GithubWorkflowMetricSchemas
	result, err := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		Order(q.Version.Desc()).
		First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return result.Version, nil
}

// Update updates a schema record
func (f *GithubWorkflowSchemaFacade) Update(ctx context.Context, schema *model.GithubWorkflowMetricSchemas) error {
	schema.UpdatedAt = time.Now()
	q := f.getDAL().GithubWorkflowMetricSchemas
	_, err := q.WithContext(ctx).Where(q.ID.Eq(schema.ID)).Updates(schema)
	return err
}

// SetActive sets a schema as active (and deactivates others for the same config)
func (f *GithubWorkflowSchemaFacade) SetActive(ctx context.Context, configID int64, schemaID int64) error {
	q := f.getDAL().GithubWorkflowMetricSchemas
	now := time.Now()

	// Deactivate all schemas for this config
	_, err := q.WithContext(ctx).
		Where(q.ConfigID.Eq(configID)).
		UpdateSimple(
			q.IsActive.Value(false),
			q.UpdatedAt.Value(now),
		)
	if err != nil {
		return err
	}

	// Activate the specified schema
	_, err = q.WithContext(ctx).
		Where(q.ID.Eq(schemaID)).
		UpdateSimple(
			q.IsActive.Value(true),
			q.UpdatedAt.Value(now),
		)
	return err
}

// Delete deletes a schema by ID
func (f *GithubWorkflowSchemaFacade) Delete(ctx context.Context, id int64) error {
	q := f.getDAL().GithubWorkflowMetricSchemas
	_, err := q.WithContext(ctx).Where(q.ID.Eq(id)).Delete()
	return err
}

// DeleteByConfig deletes all schemas for a config
func (f *GithubWorkflowSchemaFacade) DeleteByConfig(ctx context.Context, configID int64) error {
	q := f.getDAL().GithubWorkflowMetricSchemas
	_, err := q.WithContext(ctx).Where(q.ConfigID.Eq(configID)).Delete()
	return err
}

