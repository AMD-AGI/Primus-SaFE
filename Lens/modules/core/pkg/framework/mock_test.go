package framework

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// MockAiWorkloadMetadataFacade is a mock implementation for testing
type MockAiWorkloadMetadataFacade struct {
	mock.Mock
}

func (m *MockAiWorkloadMetadataFacade) GetAiWorkloadMetadata(ctx context.Context, workloadUID string) (*model.AiWorkloadMetadata, error) {
	args := m.Called(ctx, workloadUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.AiWorkloadMetadata), args.Error(1)
}

func (m *MockAiWorkloadMetadataFacade) CreateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

func (m *MockAiWorkloadMetadataFacade) UpdateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	args := m.Called(ctx, metadata)
	return args.Error(0)
}

func (m *MockAiWorkloadMetadataFacade) FindCandidateWorkloads(ctx context.Context, imagePrefix string, timeWindow time.Time, minConfidence float64, limit int) ([]*model.AiWorkloadMetadata, error) {
	args := m.Called(ctx, imagePrefix, timeWindow, minConfidence, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.AiWorkloadMetadata), args.Error(1)
}

func (m *MockAiWorkloadMetadataFacade) ListAiWorkloadMetadataByUIDs(ctx context.Context, workloadUIDs []string) ([]*model.AiWorkloadMetadata, error) {
	args := m.Called(ctx, workloadUIDs)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.AiWorkloadMetadata), args.Error(1)
}

func (m *MockAiWorkloadMetadataFacade) DeleteAiWorkloadMetadata(ctx context.Context, workloadUID string) error {
	args := m.Called(ctx, workloadUID)
	return args.Error(0)
}

func (m *MockAiWorkloadMetadataFacade) WithCluster(clusterName string) database.AiWorkloadMetadataFacadeInterface {
	args := m.Called(clusterName)
	return args.Get(0).(database.AiWorkloadMetadataFacadeInterface)
}

