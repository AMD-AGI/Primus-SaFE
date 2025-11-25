package database

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
)

// TrainingFacadeInterface defines the database operation interface for Training
type TrainingFacadeInterface interface {
	// TrainingPerformance operations
	GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx context.Context, workloadUid string, serial int, iteration int) (*model.TrainingPerformance, error)
	CreateTrainingPerformance(ctx context.Context, trainingPerformance *model.TrainingPerformance) error
	UpdateTrainingPerformance(ctx context.Context, trainingPerformance *model.TrainingPerformance) error
	ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx context.Context, workloadUid string, start, end time.Time) ([]*model.TrainingPerformance, error)
	ListTrainingPerformanceByWorkloadIdsAndTimeRange(ctx context.Context, workloadUids []string, start, end time.Time) ([]*model.TrainingPerformance, error)

	// WithCluster method
	WithCluster(clusterName string) TrainingFacadeInterface
}

// TrainingFacade implements TrainingFacadeInterface
type TrainingFacade struct {
	BaseFacade
}

// NewTrainingFacade creates a new TrainingFacade instance
func NewTrainingFacade() TrainingFacadeInterface {
	return &TrainingFacade{}
}

func (f *TrainingFacade) WithCluster(clusterName string) TrainingFacadeInterface {
	return &TrainingFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// TrainingPerformance operation implementations
func (f *TrainingFacade) GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx context.Context, workloadUid string, serial int, iteration int) (*model.TrainingPerformance, error) {
	q := f.getDAL().TrainingPerformance
	result, err := q.WithContext(ctx).Where(q.Serial.Eq(int32(serial))).Where(q.Iteration.Eq(int32(iteration))).Where(q.WorkloadUID.Eq(workloadUid)).First()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if result.ID == 0 {
		return nil, nil
	}
	return result, nil
}

func (f *TrainingFacade) CreateTrainingPerformance(ctx context.Context, trainingPerformance *model.TrainingPerformance) error {
	return f.getDAL().TrainingPerformance.WithContext(ctx).Create(trainingPerformance)
}

// UpdateTrainingPerformance updates an existing training performance record
// It deletes the old record and creates a new one with updated data
// This approach preserves the original created_at timestamp while updating the performance data
func (f *TrainingFacade) UpdateTrainingPerformance(ctx context.Context, trainingPerformance *model.TrainingPerformance) error {
	if trainingPerformance.ID == 0 {
		return errors.New("cannot update training performance with ID = 0")
	}

	db := f.getDB()

	// Delete the old record
	if err := db.WithContext(ctx).Delete(&model.TrainingPerformance{}, trainingPerformance.ID).Error; err != nil {
		return err
	}

	// Reset ID to 0 for creation
	trainingPerformance.ID = 0

	// Create new record with updated data
	return f.getDAL().TrainingPerformance.WithContext(ctx).Create(trainingPerformance)
}

func (f *TrainingFacade) ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx context.Context, workloadUid string, start, end time.Time) ([]*model.TrainingPerformance, error) {
	q := f.getDAL().TrainingPerformance
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.Eq(workloadUid)).Where(q.CreatedAt.Gte(start)).Where(q.CreatedAt.Lte(end)).Order(q.CreatedAt.Asc()).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func (f *TrainingFacade) ListTrainingPerformanceByWorkloadIdsAndTimeRange(ctx context.Context, workloadUids []string, start, end time.Time) ([]*model.TrainingPerformance, error) {
	q := f.getDAL().TrainingPerformance
	result, err := q.WithContext(ctx).Where(q.WorkloadUID.In(workloadUids...)).Where(q.CreatedAt.Gte(start)).Where(q.CreatedAt.Lte(end)).Order(q.CreatedAt.Asc()).Find()
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}
