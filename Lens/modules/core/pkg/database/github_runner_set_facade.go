package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GithubRunnerSetFacadeInterface defines the interface for GithubRunnerSet operations
type GithubRunnerSetFacadeInterface interface {
	// Upsert creates or updates a runner set
	Upsert(ctx context.Context, runnerSet *model.GithubRunnerSets) error
	// GetByUID gets a runner set by UID
	GetByUID(ctx context.Context, uid string) (*model.GithubRunnerSets, error)
	// GetByNamespaceName gets a runner set by namespace and name
	GetByNamespaceName(ctx context.Context, namespace, name string) (*model.GithubRunnerSets, error)
	// List lists all active runner sets
	List(ctx context.Context) ([]*model.GithubRunnerSets, error)
	// ListByNamespace lists runner sets in a namespace
	ListByNamespace(ctx context.Context, namespace string) ([]*model.GithubRunnerSets, error)
	// MarkDeleted marks a runner set as deleted
	MarkDeleted(ctx context.Context, uid string) error
	// UpdateStatus updates the runner counts
	UpdateStatus(ctx context.Context, uid string, current, desired int) error
	// CleanupStale removes runner sets not synced since the given time
	CleanupStale(ctx context.Context, before time.Time) (int64, error)
	// WithCluster returns a new facade instance using the specified cluster
	WithCluster(clusterName string) GithubRunnerSetFacadeInterface
}

// GithubRunnerSetFacade implements GithubRunnerSetFacadeInterface
type GithubRunnerSetFacade struct {
	BaseFacade
}

// NewGithubRunnerSetFacade creates a new GithubRunnerSetFacade
func NewGithubRunnerSetFacade() *GithubRunnerSetFacade {
	return &GithubRunnerSetFacade{}
}

// WithCluster returns a new facade instance using the specified cluster
func (f *GithubRunnerSetFacade) WithCluster(clusterName string) GithubRunnerSetFacadeInterface {
	return &GithubRunnerSetFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Upsert creates or updates a runner set
func (f *GithubRunnerSetFacade) Upsert(ctx context.Context, runnerSet *model.GithubRunnerSets) error {
	runnerSet.LastSyncAt = time.Now()
	runnerSet.UpdatedAt = time.Now()

	return f.getDB().WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "uid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"name", "namespace", "github_config_url", "github_config_secret",
			"runner_group", "github_owner", "github_repo", "min_runners", "max_runners",
			"status", "current_runners", "desired_runners", "last_sync_at", "updated_at",
		}),
	}).Create(runnerSet).Error
}

// GetByUID gets a runner set by UID
func (f *GithubRunnerSetFacade) GetByUID(ctx context.Context, uid string) (*model.GithubRunnerSets, error) {
	var runnerSet model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).Where("uid = ?", uid).First(&runnerSet).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &runnerSet, nil
}

// GetByNamespaceName gets a runner set by namespace and name
func (f *GithubRunnerSetFacade) GetByNamespaceName(ctx context.Context, namespace, name string) (*model.GithubRunnerSets, error) {
	var runnerSet model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).
		Where("namespace = ? AND name = ? AND status = ?", namespace, name, model.RunnerSetStatusActive).
		First(&runnerSet).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &runnerSet, nil
}

// List lists all active runner sets
func (f *GithubRunnerSetFacade) List(ctx context.Context) ([]*model.GithubRunnerSets, error) {
	var runnerSets []*model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).
		Where("status = ?", model.RunnerSetStatusActive).
		Order("namespace, name").
		Find(&runnerSets).Error
	return runnerSets, err
}

// ListByNamespace lists runner sets in a namespace
func (f *GithubRunnerSetFacade) ListByNamespace(ctx context.Context, namespace string) ([]*model.GithubRunnerSets, error) {
	var runnerSets []*model.GithubRunnerSets
	err := f.getDB().WithContext(ctx).
		Where("namespace = ? AND status = ?", namespace, model.RunnerSetStatusActive).
		Order("name").
		Find(&runnerSets).Error
	return runnerSets, err
}

// MarkDeleted marks a runner set as deleted
func (f *GithubRunnerSetFacade) MarkDeleted(ctx context.Context, uid string) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GithubRunnerSets{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"status":     model.RunnerSetStatusDeleted,
			"updated_at": time.Now(),
		}).Error
}

// UpdateStatus updates the runner counts
func (f *GithubRunnerSetFacade) UpdateStatus(ctx context.Context, uid string, current, desired int) error {
	return f.getDB().WithContext(ctx).
		Model(&model.GithubRunnerSets{}).
		Where("uid = ?", uid).
		Updates(map[string]interface{}{
			"current_runners": current,
			"desired_runners": desired,
			"updated_at":      time.Now(),
		}).Error
}

// CleanupStale removes runner sets not synced since the given time
func (f *GithubRunnerSetFacade) CleanupStale(ctx context.Context, before time.Time) (int64, error) {
	result := f.getDB().WithContext(ctx).
		Model(&model.GithubRunnerSets{}).
		Where("last_sync_at < ? AND status = ?", before, model.RunnerSetStatusActive).
		Updates(map[string]interface{}{
			"status":     model.RunnerSetStatusDeleted,
			"updated_at": time.Now(),
		})
	return result.RowsAffected, result.Error
}

