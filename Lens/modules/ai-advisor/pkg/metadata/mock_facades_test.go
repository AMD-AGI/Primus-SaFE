package metadata

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// MockNodeFacade is a mock implementation of NodeFacadeInterface
type MockNodeFacade struct {
	Nodes          map[string]*model.Node
	GetByNameErr   error
	GetByNameCalls int
}

// NewMockNodeFacade creates a new mock node facade
func NewMockNodeFacade() *MockNodeFacade {
	return &MockNodeFacade{
		Nodes: make(map[string]*model.Node),
	}
}

// GetNodeByName retrieves node by name
func (m *MockNodeFacade) GetNodeByName(ctx context.Context, nodeName string) (*model.Node, error) {
	m.GetByNameCalls++

	if m.GetByNameErr != nil {
		return nil, m.GetByNameErr
	}

	node, ok := m.Nodes[nodeName]
	if !ok {
		return nil, nil
	}

	return node, nil
}

// AddNode adds a node to the mock
func (m *MockNodeFacade) AddNode(name, address string) {
	m.Nodes[name] = &model.Node{
		Name:    name,
		Address: address,
	}
}

// MockPodFacade is a mock implementation of PodFacadeInterface
type MockPodFacade struct {
	GpuPods          map[string]*model.GpuPods
	GetByPodUidErr   error
	GetByPodUidCalls int
}

// NewMockPodFacade creates a new mock pod facade
func NewMockPodFacade() *MockPodFacade {
	return &MockPodFacade{
		GpuPods: make(map[string]*model.GpuPods),
	}
}

// GetGpuPodsByPodUid retrieves GPU pod by pod UID
func (m *MockPodFacade) GetGpuPodsByPodUid(ctx context.Context, podUID string) (*model.GpuPods, error) {
	m.GetByPodUidCalls++

	if m.GetByPodUidErr != nil {
		return nil, m.GetByPodUidErr
	}

	pod, ok := m.GpuPods[podUID]
	if !ok {
		return nil, nil
	}

	return pod, nil
}

// AddGpuPod adds a GPU pod to the mock
func (m *MockPodFacade) AddGpuPod(podUID, nodeName string) {
	m.GpuPods[podUID] = &model.GpuPods{
		UID:      podUID,
		NodeName: nodeName,
	}
}

// Implement all required methods from PodFacadeInterface
func (m *MockPodFacade) WithCluster(clusterName string) database.PodFacadeInterface { return m }
func (m *MockPodFacade) CreateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error { return nil }
func (m *MockPodFacade) UpdateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error { return nil }
func (m *MockPodFacade) GetActiveGpuPodByNodeName(ctx context.Context, nodeName string) ([]*model.GpuPods, error) { return nil, nil }
func (m *MockPodFacade) GetHistoryGpuPodByNodeName(ctx context.Context, nodeName string, pageNum, pageSize int) ([]*model.GpuPods, int, error) { return nil, 0, nil }
func (m *MockPodFacade) ListActivePodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) { return nil, nil }
func (m *MockPodFacade) ListPodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) { return nil, nil }
func (m *MockPodFacade) ListActiveGpuPods(ctx context.Context) ([]*model.GpuPods, error) { return nil, nil }
func (m *MockPodFacade) CreateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPodsEvent) error { return nil }
func (m *MockPodFacade) UpdateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPods) error { return nil }
func (m *MockPodFacade) CreatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error { return nil }
func (m *MockPodFacade) UpdatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error { return nil }
func (m *MockPodFacade) GetLastPodSnapshot(ctx context.Context, podUid string, resourceVersion int) (*model.PodSnapshot, error) { return nil, nil }
func (m *MockPodFacade) GetPodResourceByUid(ctx context.Context, uid string) (*model.PodResource, error) { return nil, nil }
func (m *MockPodFacade) CreatePodResource(ctx context.Context, podResource *model.PodResource) error { return nil }
func (m *MockPodFacade) UpdatePodResource(ctx context.Context, podResource *model.PodResource) error { return nil }
func (m *MockPodFacade) ListPodResourcesByUids(ctx context.Context, uids []string) ([]*model.PodResource, error) { return nil, nil }
func (m *MockPodFacade) QueryPodsWithFilters(ctx context.Context, namespace, podName, startTime, endTime string, page, pageSize int) ([]*model.GpuPods, int64, error) { return nil, 0, nil }
func (m *MockPodFacade) GetAverageGPUUtilizationByNode(ctx context.Context, nodeName string) (float64, error) { return 0.0, nil }
func (m *MockPodFacade) GetLatestGPUMetricsByNode(ctx context.Context, nodeName string) (*model.GpuDevice, error) { return nil, nil }
func (m *MockPodFacade) QueryGPUHistoryByNode(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]*model.GpuDevice, error) { return nil, nil }
func (m *MockPodFacade) ListPodEventsByUID(ctx context.Context, podUID string) ([]*model.GpuPodsEvent, error) { return nil, nil }

// MockAiWorkloadMetadataFacade is a mock implementation of AiWorkloadMetadataFacadeInterface
type MockAiWorkloadMetadataFacade struct {
	Metadata  map[string]*model.AiWorkloadMetadata
	CreateErr error
	GetErr    error
	UpdateErr error
	DeleteErr error

	CreateCalls int
	GetCalls    int
	UpdateCalls int
	DeleteCalls int
}

// NewMockAiWorkloadMetadataFacade creates a new mock AI workload metadata facade
func NewMockAiWorkloadMetadataFacade() *MockAiWorkloadMetadataFacade {
	return &MockAiWorkloadMetadataFacade{
		Metadata: make(map[string]*model.AiWorkloadMetadata),
	}
}

// CreateAiWorkloadMetadata creates AI workload metadata
func (m *MockAiWorkloadMetadataFacade) CreateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	m.CreateCalls++

	if m.CreateErr != nil {
		return m.CreateErr
	}

	if _, exists := m.Metadata[metadata.WorkloadUID]; exists {
		return fmt.Errorf("metadata already exists for workload %s", metadata.WorkloadUID)
	}

	m.Metadata[metadata.WorkloadUID] = metadata
	return nil
}

// GetAiWorkloadMetadata retrieves AI workload metadata
func (m *MockAiWorkloadMetadataFacade) GetAiWorkloadMetadata(ctx context.Context, workloadUID string) (*model.AiWorkloadMetadata, error) {
	m.GetCalls++

	if m.GetErr != nil {
		return nil, m.GetErr
	}

	metadata, ok := m.Metadata[workloadUID]
	if !ok {
		return nil, nil
	}

	return metadata, nil
}

// UpdateAiWorkloadMetadata updates AI workload metadata
func (m *MockAiWorkloadMetadataFacade) UpdateAiWorkloadMetadata(ctx context.Context, metadata *model.AiWorkloadMetadata) error {
	m.UpdateCalls++

	if m.UpdateErr != nil {
		return m.UpdateErr
	}

	if _, exists := m.Metadata[metadata.WorkloadUID]; !exists {
		return fmt.Errorf("metadata not found for workload %s", metadata.WorkloadUID)
	}

	m.Metadata[metadata.WorkloadUID] = metadata
	return nil
}

// DeleteAiWorkloadMetadata deletes AI workload metadata
func (m *MockAiWorkloadMetadataFacade) DeleteAiWorkloadMetadata(ctx context.Context, workloadUID string) error {
	m.DeleteCalls++

	if m.DeleteErr != nil {
		return m.DeleteErr
	}

	delete(m.Metadata, workloadUID)
	return nil
}

// FindCandidateWorkloads finds candidate workloads for reuse
func (m *MockAiWorkloadMetadataFacade) FindCandidateWorkloads(ctx context.Context, imagePrefix string, timeWindow time.Time, minConfidence float64, limit int) ([]*model.AiWorkloadMetadata, error) {
	var candidates []*model.AiWorkloadMetadata

	for _, metadata := range m.Metadata {
		if metadata.ImagePrefix == imagePrefix {
			candidates = append(candidates, metadata)
		}
	}

	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}

	return candidates, nil
}

// ListAiWorkloadMetadataByUIDs retrieves multiple metadata records by workload UIDs
func (m *MockAiWorkloadMetadataFacade) ListAiWorkloadMetadataByUIDs(ctx context.Context, workloadUIDs []string) ([]*model.AiWorkloadMetadata, error) {
	var results []*model.AiWorkloadMetadata

	for _, uid := range workloadUIDs {
		if metadata, ok := m.Metadata[uid]; ok {
			results = append(results, metadata)
		}
	}

	return results, nil
}

// WithCluster returns the facade itself (for testing)
func (m *MockAiWorkloadMetadataFacade) WithCluster(clusterName string) database.AiWorkloadMetadataFacadeInterface {
	return m
}
