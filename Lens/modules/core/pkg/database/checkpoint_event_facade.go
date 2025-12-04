package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// CheckpointEventFacadeInterface defines the database operation interface for CheckpointEvent
type CheckpointEventFacadeInterface interface {
	// CheckpointEvent operations
	CreateCheckpointEvent(ctx context.Context, event *model.CheckpointEvent) error
	GetCheckpointEventByWorkloadAndIteration(ctx context.Context, workloadUID string, iteration int) (*model.CheckpointEvent, error)
	ListCheckpointEventsByWorkload(ctx context.Context, workloadUID string) ([]*model.CheckpointEvent, error)
	ListCheckpointEventsByWorkloadAndTimeRange(ctx context.Context, workloadUID string, start, end time.Time) ([]*model.CheckpointEvent, error)
	ListCheckpointEventsByType(ctx context.Context, workloadUID, eventType string) ([]*model.CheckpointEvent, error)
	UpdateCheckpointEvent(ctx context.Context, event *model.CheckpointEvent) error
	GetLatestCheckpointEvent(ctx context.Context, workloadUID string) (*model.CheckpointEvent, error)

	// WithCluster method
	WithCluster(clusterName string) CheckpointEventFacadeInterface
}

// CheckpointEventFacade implements CheckpointEventFacadeInterface
type CheckpointEventFacade struct {
	BaseFacade
}

// NewCheckpointEventFacade creates a new CheckpointEventFacade instance
func NewCheckpointEventFacade() CheckpointEventFacadeInterface {
	return &CheckpointEventFacade{}
}

func (f *CheckpointEventFacade) WithCluster(clusterName string) CheckpointEventFacadeInterface {
	return &CheckpointEventFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// CreateCheckpointEvent creates a new checkpoint event
func (f *CheckpointEventFacade) CreateCheckpointEvent(ctx context.Context, event *model.CheckpointEvent) error {
	db := f.getDB()
	return db.WithContext(ctx).Create(event).Error
}

// GetCheckpointEventByWorkloadAndIteration retrieves a checkpoint event by workload UID and iteration
func (f *CheckpointEventFacade) GetCheckpointEventByWorkloadAndIteration(ctx context.Context, workloadUID string, iteration int) (*model.CheckpointEvent, error) {
	db := f.getDB()
	var event model.CheckpointEvent
	
	err := db.WithContext(ctx).
		Where("workload_uid = ? AND iteration = ?", workloadUID, iteration).
		First(&event).Error
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	
	return &event, nil
}

// ListCheckpointEventsByWorkload retrieves all checkpoint events for a workload
func (f *CheckpointEventFacade) ListCheckpointEventsByWorkload(ctx context.Context, workloadUID string) ([]*model.CheckpointEvent, error) {
	db := f.getDB()
	var events []*model.CheckpointEvent
	
	err := db.WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		Order("created_at DESC").
		Find(&events).Error
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	
	return events, nil
}

// ListCheckpointEventsByWorkloadAndTimeRange retrieves checkpoint events within a time range
func (f *CheckpointEventFacade) ListCheckpointEventsByWorkloadAndTimeRange(ctx context.Context, workloadUID string, start, end time.Time) ([]*model.CheckpointEvent, error) {
	db := f.getDB()
	var events []*model.CheckpointEvent
	
	err := db.WithContext(ctx).
		Where("workload_uid = ? AND created_at >= ? AND created_at <= ?", workloadUID, start, end).
		Order("created_at ASC").
		Find(&events).Error
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	
	return events, nil
}

// ListCheckpointEventsByType retrieves checkpoint events by event type
func (f *CheckpointEventFacade) ListCheckpointEventsByType(ctx context.Context, workloadUID, eventType string) ([]*model.CheckpointEvent, error) {
	db := f.getDB()
	var events []*model.CheckpointEvent
	
	err := db.WithContext(ctx).
		Where("workload_uid = ? AND event_type = ?", workloadUID, eventType).
		Order("created_at DESC").
		Find(&events).Error
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	
	return events, nil
}

// UpdateCheckpointEvent updates an existing checkpoint event
func (f *CheckpointEventFacade) UpdateCheckpointEvent(ctx context.Context, event *model.CheckpointEvent) error {
	db := f.getDB()
	return db.WithContext(ctx).Save(event).Error
}

// GetLatestCheckpointEvent retrieves the most recent checkpoint event for a workload
func (f *CheckpointEventFacade) GetLatestCheckpointEvent(ctx context.Context, workloadUID string) (*model.CheckpointEvent, error) {
	db := f.getDB()
	var event model.CheckpointEvent
	
	err := db.WithContext(ctx).
		Where("workload_uid = ?", workloadUID).
		Order("created_at DESC").
		First(&event).Error
	
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	
	return &event, nil
}

