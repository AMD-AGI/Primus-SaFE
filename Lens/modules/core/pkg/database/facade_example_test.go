package database_test

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/filter"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
)

// MockFacade is a mock Facade implementation for unit testing
type MockFacade struct {
	MockNode      database.NodeFacadeInterface
	MockPod       database.PodFacadeInterface
	MockWorkload  database.WorkloadFacadeInterface
	MockContainer database.ContainerFacadeInterface
	MockTraining  database.TrainingFacadeInterface
	MockStorage   database.StorageFacadeInterface
}

// Ensure MockFacade implements FacadeInterface
var _ database.FacadeInterface = (*MockFacade)(nil)

func (m *MockFacade) GetNode() database.NodeFacadeInterface {
	return m.MockNode
}

func (m *MockFacade) GetPod() database.PodFacadeInterface {
	return m.MockPod
}

func (m *MockFacade) GetWorkload() database.WorkloadFacadeInterface {
	return m.MockWorkload
}

func (m *MockFacade) GetContainer() database.ContainerFacadeInterface {
	return m.MockContainer
}

func (m *MockFacade) GetTraining() database.TrainingFacadeInterface {
	return m.MockTraining
}

func (m *MockFacade) GetStorage() database.StorageFacadeInterface {
	return m.MockStorage
}

func (m *MockFacade) WithCluster(clusterName string) database.FacadeInterface {
	// Returns a new MockFacade, can customize behavior as needed
	return &MockFacade{
		MockNode:      m.MockNode,
		MockPod:       m.MockPod,
		MockWorkload:  m.MockWorkload,
		MockContainer: m.MockContainer,
		MockTraining:  m.MockTraining,
		MockStorage:   m.MockStorage,
	}
}

// ===== Mock Node Facade =====
type MockNodeFacade struct {
	CreateNodeFunc    func(ctx context.Context, node *model.Node) error
	GetNodeByNameFunc func(ctx context.Context, name string) (*model.Node, error)
}

var _ database.NodeFacadeInterface = (*MockNodeFacade)(nil)

func (m *MockNodeFacade) CreateNode(ctx context.Context, node *model.Node) error {
	if m.CreateNodeFunc != nil {
		return m.CreateNodeFunc(ctx, node)
	}
	return nil
}

func (m *MockNodeFacade) UpdateNode(ctx context.Context, node *model.Node) error {
	return nil
}

func (m *MockNodeFacade) GetNodeByName(ctx context.Context, name string) (*model.Node, error) {
	if m.GetNodeByNameFunc != nil {
		return m.GetNodeByNameFunc(ctx, name)
	}
	return nil, nil
}

// Other method implementations omitted...
func (m *MockNodeFacade) SearchNode(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
	return nil, 0, nil
}

func (m *MockNodeFacade) GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*model.GpuDevice, error) {
	return nil, nil
}

func (m *MockNodeFacade) CreateGpuDevice(ctx context.Context, device *model.GpuDevice) error {
	return nil
}

func (m *MockNodeFacade) UpdateGpuDevice(ctx context.Context, device *model.GpuDevice) error {
	return nil
}

func (m *MockNodeFacade) ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.GpuDevice, error) {
	return nil, nil
}

func (m *MockNodeFacade) DeleteGpuDeviceById(ctx context.Context, id int32) error {
	return nil
}

func (m *MockNodeFacade) GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*model.RdmaDevice, error) {
	return nil, nil
}

func (m *MockNodeFacade) CreateRdmaDevice(ctx context.Context, rdmaDevice *model.RdmaDevice) error {
	return nil
}

func (m *MockNodeFacade) ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.RdmaDevice, error) {
	return nil, nil
}

func (m *MockNodeFacade) DeleteRdmaDeviceById(ctx context.Context, id int32) error {
	return nil
}

func (m *MockNodeFacade) CreateNodeDeviceChangelog(ctx context.Context, changelog *model.NodeDeviceChangelog) error {
	return nil
}

func (m *MockNodeFacade) WithCluster(clusterName string) database.NodeFacadeInterface {
	return m
}

// ===== Usage Example =====

// Example: A business service that needs to use Facade
type MyService struct {
	facade database.FacadeInterface
}

func NewMyService(facade database.FacadeInterface) *MyService {
	return &MyService{
		facade: facade,
	}
}

func (s *MyService) GetNodeInfo(ctx context.Context, nodeName string) (*model.Node, error) {
	// In production code, you can directly access the field (backward compatible)
	// return s.facade.Node.GetNodeByName(ctx, nodeName)

	// Or use the interface method (recommended, easier to test)
	return s.facade.GetNode().GetNodeByName(ctx, nodeName)
}

/*
// Unit test example (use in _test.go files):

func TestMyService_GetNodeInfo(t *testing.T) {
	// Create Mock Facade
	mockNodeFacade := &MockNodeFacade{
		GetNodeByNameFunc: func(ctx context.Context, name string) (*model.Node, error) {
			// Return test data
			return &model.Node{
				Name: name,
				ID:   1,
			}, nil
		},
	}

	mockFacade := &MockFacade{
		MockNode: mockNodeFacade,
	}

	// Create service instance, inject Mock Facade
	service := NewMyService(mockFacade)

	// Execute test
	node, err := service.GetNodeInfo(context.Background(), "test-node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.Name != "test-node" {
		t.Errorf("expected node name 'test-node', got '%s'", node.Name)
	}
}

// Multi-cluster test example:

func TestMyService_WithMultiCluster(t *testing.T) {
	mockFacade := &MockFacade{
		MockNode: &MockNodeFacade{
			GetNodeByNameFunc: func(ctx context.Context, name string) (*model.Node, error) {
				return &model.Node{Name: name}, nil
			},
		},
	}

	// Test cluster switching
	cluster1Facade := mockFacade.WithCluster("cluster-1")
	service := NewMyService(cluster1Facade)

	node, err := service.GetNodeInfo(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.Name != "node-1" {
		t.Errorf("expected node name 'node-1', got '%s'", node.Name)
	}
}

// Example using third-party Mock library (like gomock):

// 1. First generate mock code:
// mockgen -source=Lens/modules/core/pkg/database/facade.go -destination=Lens/modules/core/pkg/database/mock/mock_facade.go -package=mock

// 2. Use in tests:
import (
	"testing"
	"github.com/golang/mock/gomock"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/mock"
)

func TestMyService_WithGomock(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create Mock Facade
	mockFacade := mock.NewMockFacadeInterface(ctrl)
	mockNodeFacade := mock.NewMockNodeFacadeInterface(ctrl)

	// Set up expected calls
	mockFacade.EXPECT().GetNode().Return(mockNodeFacade)
	mockNodeFacade.EXPECT().GetNodeByName(gomock.Any(), "test-node").Return(&model.Node{
		Name: "test-node",
		ID:   1,
	}, nil)

	// Create service instance
	service := NewMyService(mockFacade)

	// Execute test
	node, err := service.GetNodeInfo(context.Background(), "test-node")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if node.Name != "test-node" {
		t.Errorf("expected node name 'test-node', got '%s'", node.Name)
	}
}
*/
