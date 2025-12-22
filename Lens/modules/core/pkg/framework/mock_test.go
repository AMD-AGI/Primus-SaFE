package framework

import (
	"context"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
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

// MockWorkloadFacade is a mock implementation for testing
type MockWorkloadFacade struct {
	mock.Mock
}

func (m *MockWorkloadFacade) GetGpuWorkloadByUid(ctx context.Context, uid string) (*model.GpuWorkload, error) {
	args := m.Called(ctx, uid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.GpuWorkload), args.Error(1)
}

// Implement other required methods as no-op for now
func (m *MockWorkloadFacade) CreateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	args := m.Called(ctx, gpuWorkload)
	return args.Error(0)
}

func (m *MockWorkloadFacade) UpdateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	args := m.Called(ctx, gpuWorkload)
	return args.Error(0)
}

func (m *MockWorkloadFacade) QueryWorkload(ctx context.Context, f *filter.WorkloadFilter) ([]*model.GpuWorkload, int, error) {
	return nil, 0, nil
}

func (m *MockWorkloadFacade) GetWorkloadsNamespaceList(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) GetWorkloadKindList(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) GetWorkloadNotEnd(ctx context.Context) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListRunningWorkload(ctx context.Context) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListWorkloadsByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) GetNearestWorkloadByPodUid(ctx context.Context, podUid string) (*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListTopLevelWorkloadByUids(ctx context.Context, uids []string) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListChildrenWorkloadByParentUid(ctx context.Context, parentUid string) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListWorkloadByLabelValue(ctx context.Context, labelKey, labelValue string) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListWorkloadNotEndByKind(ctx context.Context, kind string) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) CreateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error {
	return nil
}

func (m *MockWorkloadFacade) UpdateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *model.GpuWorkloadSnapshot) error {
	return nil
}

func (m *MockWorkloadFacade) GetLatestGpuWorkloadSnapshotByUid(ctx context.Context, uid string, resourceVersion int) (*model.GpuWorkloadSnapshot, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) CreateWorkloadPodReference(ctx context.Context, workloadUid, podUid string) error {
	return nil
}

func (m *MockWorkloadFacade) ListWorkloadPodReferencesByPodUids(ctx context.Context, podUids []string) ([]*model.WorkloadPodReference, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListWorkloadPodReferenceByWorkloadUid(ctx context.Context, workloadUid string) ([]*model.WorkloadPodReference, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx context.Context, workloadUid, nearestWorkloadId, typ string) (*model.WorkloadEvent, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) CreateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error {
	return nil
}

func (m *MockWorkloadFacade) UpdateWorkloadEvent(ctx context.Context, workloadEvent *model.WorkloadEvent) error {
	return nil
}

func (m *MockWorkloadFacade) GetLatestEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) GetLatestOtherWorkloadEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*model.WorkloadEvent, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) ListActiveTopLevelWorkloads(ctx context.Context, startTime, endTime time.Time, namespace string) ([]*model.GpuWorkload, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) GetAllWorkloadPodReferences(ctx context.Context) ([]*model.WorkloadPodReference, error) {
	return nil, nil
}

func (m *MockWorkloadFacade) WithCluster(clusterName string) database.WorkloadFacadeInterface {
	return m
}
