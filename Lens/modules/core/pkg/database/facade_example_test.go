package database_test

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/filter"
	"github.com/AMD-AGI/primus-lens/core/pkg/database/model"
)

// MockFacade is a mock Facade implementation for unit testing
type MockFacade struct {
	MockNode            database.NodeFacadeInterface
	MockPod             database.PodFacadeInterface
	MockWorkload        database.WorkloadFacadeInterface
	MockContainer       database.ContainerFacadeInterface
	MockTraining        database.TrainingFacadeInterface
	MockStorage         database.StorageFacadeInterface
	MockAlert           database.AlertFacadeInterface
	MockMetricAlertRule database.MetricAlertRuleFacadeInterface
	MockLogAlertRule    database.LogAlertRuleFacadeInterface
	MockAlertRuleAdvice database.AlertRuleAdviceFacadeInterface
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

func (m *MockFacade) GetAlert() database.AlertFacadeInterface {
	return m.MockAlert
}

func (m *MockFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface {
	return m.MockMetricAlertRule
}

func (m *MockFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface {
	return m.MockLogAlertRule
}

func (m *MockFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface {
	return m.MockAlertRuleAdvice
}

func (m *MockFacade) WithCluster(clusterName string) database.FacadeInterface {
	// Returns a new MockFacade, can customize behavior as needed
	return &MockFacade{
		MockNode:            m.MockNode,
		MockPod:             m.MockPod,
		MockWorkload:        m.MockWorkload,
		MockContainer:       m.MockContainer,
		MockTraining:        m.MockTraining,
		MockStorage:         m.MockStorage,
		MockAlert:           m.MockAlert,
		MockMetricAlertRule: m.MockMetricAlertRule,
		MockLogAlertRule:    m.MockLogAlertRule,
		MockAlertRuleAdvice: m.MockAlertRuleAdvice,
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
