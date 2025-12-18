/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ==================== Mock Implementations ====================

// MockFacade implements database.FacadeInterface for testing
type MockFacade struct {
	nodeFacade                  database.NodeFacadeInterface
	namespaceInfoFacade         database.NamespaceInfoFacadeInterface
	nodeNamespaceMappingFacade  database.NodeNamespaceMappingFacadeInterface
}

func (m *MockFacade) GetNode() database.NodeFacadeInterface                           { return m.nodeFacade }
func (m *MockFacade) GetNamespaceInfo() database.NamespaceInfoFacadeInterface         { return m.namespaceInfoFacade }
func (m *MockFacade) GetNodeNamespaceMapping() database.NodeNamespaceMappingFacadeInterface { return m.nodeNamespaceMappingFacade }
func (m *MockFacade) GetWorkload() database.WorkloadFacadeInterface                   { return nil }
func (m *MockFacade) GetPod() database.PodFacadeInterface                             { return nil }
func (m *MockFacade) GetContainer() database.ContainerFacadeInterface                 { return nil }
func (m *MockFacade) GetTraining() database.TrainingFacadeInterface                   { return nil }
func (m *MockFacade) GetStorage() database.StorageFacadeInterface                     { return nil }
func (m *MockFacade) GetAlert() database.AlertFacadeInterface                         { return nil }
func (m *MockFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface     { return nil }
func (m *MockFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface           { return nil }
func (m *MockFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface     { return nil }
func (m *MockFacade) GetClusterOverviewCache() database.ClusterOverviewCacheFacadeInterface { return nil }
func (m *MockFacade) GetGenericCache() database.GenericCacheFacadeInterface           { return nil }
func (m *MockFacade) GetGpuAggregation() database.GpuAggregationFacadeInterface       { return nil }
func (m *MockFacade) GetSystemConfig() database.SystemConfigFacadeInterface           { return nil }
func (m *MockFacade) GetJobExecutionHistory() database.JobExecutionHistoryFacadeInterface { return nil }
func (m *MockFacade) GetWorkloadStatistic() database.WorkloadStatisticFacadeInterface { return nil }
func (m *MockFacade) GetAiWorkloadMetadata() database.AiWorkloadMetadataFacadeInterface { return nil }
func (m *MockFacade) GetCheckpointEvent() database.CheckpointEventFacadeInterface     { return nil }
func (m *MockFacade) GetDetectionConflictLog() database.DetectionConflictLogFacadeInterface { return nil }
func (m *MockFacade) GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface { return nil }
func (m *MockFacade) GetTraceLensSession() database.TraceLensSessionFacadeInterface        { return nil }
func (m *MockFacade) WithCluster(clusterName string) database.FacadeInterface              { return m }

// MockNamespaceInfoFacade implements database.NamespaceInfoFacadeInterface
type MockNamespaceInfoFacade struct {
	ListAllIncludingDeletedFunc func(ctx context.Context) ([]*model.NamespaceInfo, error)
	CreateFunc                  func(ctx context.Context, info *model.NamespaceInfo) error
	UpdateFunc                  func(ctx context.Context, info *model.NamespaceInfo) error
	DeleteByNameFunc            func(ctx context.Context, name string) error
	RecoverFunc                 func(ctx context.Context, name string, gpuModel string, gpuResource int32) error
	GetByNameFunc               func(ctx context.Context, name string) (*model.NamespaceInfo, error)
}

func (m *MockNamespaceInfoFacade) ListAllIncludingDeleted(ctx context.Context) ([]*model.NamespaceInfo, error) {
	if m.ListAllIncludingDeletedFunc != nil {
		return m.ListAllIncludingDeletedFunc(ctx)
	}
	return nil, nil
}
func (m *MockNamespaceInfoFacade) Create(ctx context.Context, info *model.NamespaceInfo) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, info)
	}
	return nil
}
func (m *MockNamespaceInfoFacade) Update(ctx context.Context, info *model.NamespaceInfo) error {
	if m.UpdateFunc != nil {
		return m.UpdateFunc(ctx, info)
	}
	return nil
}
func (m *MockNamespaceInfoFacade) DeleteByName(ctx context.Context, name string) error {
	if m.DeleteByNameFunc != nil {
		return m.DeleteByNameFunc(ctx, name)
	}
	return nil
}
func (m *MockNamespaceInfoFacade) Recover(ctx context.Context, name string, gpuModel string, gpuResource int32) error {
	if m.RecoverFunc != nil {
		return m.RecoverFunc(ctx, name, gpuModel, gpuResource)
	}
	return nil
}
func (m *MockNamespaceInfoFacade) GetByName(ctx context.Context, name string) (*model.NamespaceInfo, error) {
	if m.GetByNameFunc != nil {
		return m.GetByNameFunc(ctx, name)
	}
	return nil, nil
}
func (m *MockNamespaceInfoFacade) WithCluster(clusterName string) database.NamespaceInfoFacadeInterface { return m }
func (m *MockNamespaceInfoFacade) GetByNameIncludingDeleted(ctx context.Context, name string) (*model.NamespaceInfo, error) { return nil, nil }
func (m *MockNamespaceInfoFacade) List(ctx context.Context) ([]*model.NamespaceInfo, error) { return nil, nil }

// MockNodeFacade implements database.NodeFacadeInterface
type MockNodeFacade struct {
	SearchNodeFunc func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error)
}

func (m *MockNodeFacade) SearchNode(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
	if m.SearchNodeFunc != nil {
		return m.SearchNodeFunc(ctx, f)
	}
	return nil, 0, nil
}
func (m *MockNodeFacade) WithCluster(clusterName string) database.NodeFacadeInterface { return m }
func (m *MockNodeFacade) GetNodeByName(ctx context.Context, name string) (*model.Node, error) { return nil, nil }
func (m *MockNodeFacade) CreateNode(ctx context.Context, node *model.Node) error { return nil }
func (m *MockNodeFacade) UpdateNode(ctx context.Context, node *model.Node) error { return nil }
func (m *MockNodeFacade) ListGpuNodes(ctx context.Context) ([]*model.Node, error) { return nil, nil }
func (m *MockNodeFacade) GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*model.GpuDevice, error) { return nil, nil }
func (m *MockNodeFacade) CreateGpuDevice(ctx context.Context, device *model.GpuDevice) error { return nil }
func (m *MockNodeFacade) UpdateGpuDevice(ctx context.Context, device *model.GpuDevice) error { return nil }
func (m *MockNodeFacade) ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.GpuDevice, error) { return nil, nil }
func (m *MockNodeFacade) DeleteGpuDeviceById(ctx context.Context, id int32) error { return nil }
func (m *MockNodeFacade) GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*model.RdmaDevice, error) { return nil, nil }
func (m *MockNodeFacade) CreateRdmaDevice(ctx context.Context, rdmaDevice *model.RdmaDevice) error { return nil }
func (m *MockNodeFacade) ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*model.RdmaDevice, error) { return nil, nil }
func (m *MockNodeFacade) DeleteRdmaDeviceById(ctx context.Context, id int32) error { return nil }
func (m *MockNodeFacade) CreateNodeDeviceChangelog(ctx context.Context, changelog *model.NodeDeviceChangelog) error { return nil }

// MockNodeNamespaceMappingFacade implements database.NodeNamespaceMappingFacadeInterface
type MockNodeNamespaceMappingFacade struct {
	ListActiveByNamespaceNameFunc         func(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error)
	CreateFunc                            func(ctx context.Context, mapping *model.NodeNamespaceMapping) error
	SoftDeleteFunc                        func(ctx context.Context, id int32) error
	GetLatestHistoryByNodeAndNamespaceFunc func(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMappingHistory, error)
	UpdateHistoryRecordEndFunc            func(ctx context.Context, historyID int32, recordEnd time.Time) error
	CreateHistoryFunc                     func(ctx context.Context, history *model.NodeNamespaceMappingHistory) error
}

func (m *MockNodeNamespaceMappingFacade) ListActiveByNamespaceName(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error) {
	if m.ListActiveByNamespaceNameFunc != nil {
		return m.ListActiveByNamespaceNameFunc(ctx, namespaceName)
	}
	return nil, nil
}
func (m *MockNodeNamespaceMappingFacade) Create(ctx context.Context, mapping *model.NodeNamespaceMapping) error {
	if m.CreateFunc != nil {
		return m.CreateFunc(ctx, mapping)
	}
	return nil
}
func (m *MockNodeNamespaceMappingFacade) SoftDelete(ctx context.Context, id int32) error {
	if m.SoftDeleteFunc != nil {
		return m.SoftDeleteFunc(ctx, id)
	}
	return nil
}
func (m *MockNodeNamespaceMappingFacade) GetLatestHistoryByNodeAndNamespace(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMappingHistory, error) {
	if m.GetLatestHistoryByNodeAndNamespaceFunc != nil {
		return m.GetLatestHistoryByNodeAndNamespaceFunc(ctx, nodeID, namespaceID)
	}
	return nil, nil
}
func (m *MockNodeNamespaceMappingFacade) UpdateHistoryRecordEnd(ctx context.Context, historyID int32, recordEnd time.Time) error {
	if m.UpdateHistoryRecordEndFunc != nil {
		return m.UpdateHistoryRecordEndFunc(ctx, historyID, recordEnd)
	}
	return nil
}
func (m *MockNodeNamespaceMappingFacade) CreateHistory(ctx context.Context, history *model.NodeNamespaceMappingHistory) error {
	if m.CreateHistoryFunc != nil {
		return m.CreateHistoryFunc(ctx, history)
	}
	return nil
}
func (m *MockNodeNamespaceMappingFacade) WithCluster(clusterName string) database.NodeNamespaceMappingFacadeInterface { return m }
func (m *MockNodeNamespaceMappingFacade) Update(ctx context.Context, mapping *model.NodeNamespaceMapping) error { return nil }
func (m *MockNodeNamespaceMappingFacade) Delete(ctx context.Context, id int32) error { return nil }
func (m *MockNodeNamespaceMappingFacade) GetByNodeAndNamespace(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMapping, error) { return nil, nil }
func (m *MockNodeNamespaceMappingFacade) GetByNodeName(ctx context.Context, nodeName string) ([]*model.NodeNamespaceMapping, error) { return nil, nil }
func (m *MockNodeNamespaceMappingFacade) GetByNamespaceName(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error) { return nil, nil }
func (m *MockNodeNamespaceMappingFacade) ListActiveByNamespaceID(ctx context.Context, namespaceID int64) ([]*model.NodeNamespaceMapping, error) { return nil, nil }
func (m *MockNodeNamespaceMappingFacade) ListHistoryByNamespaceAtTime(ctx context.Context, namespaceID int64, atTime time.Time) ([]*model.NodeNamespaceMappingHistory, error) { return nil, nil }
func (m *MockNodeNamespaceMappingFacade) ListHistoryByNamespaceNameAtTime(ctx context.Context, namespaceName string, atTime time.Time) ([]*model.NodeNamespaceMappingHistory, error) { return nil, nil }

// MockWorkspaceLister implements WorkspaceLister
type MockWorkspaceLister struct {
	ListWorkspacesFunc func(ctx context.Context) ([]primusSafeV1.Workspace, error)
}

func (m *MockWorkspaceLister) ListWorkspaces(ctx context.Context) ([]primusSafeV1.Workspace, error) {
	if m.ListWorkspacesFunc != nil {
		return m.ListWorkspacesFunc(ctx)
	}
	return nil, nil
}

// ==================== Test Helper Functions ====================

func createTestWorkspace(name string, gpuCount int64, nodeFlavor string) primusSafeV1.Workspace {
	ws := primusSafeV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: primusSafeV1.WorkspaceSpec{
			NodeFlavor: nodeFlavor,
		},
	}
	if gpuCount > 0 {
		ws.Status.TotalResources = corev1.ResourceList{
			corev1.ResourceName(AMDGPUResourceName): resource.MustParse(string(rune(gpuCount + '0'))),
		}
	}
	return ws
}

func createTestWorkspaceWithAMDGPU(name string, gpuCount int64) primusSafeV1.Workspace {
	return primusSafeV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: primusSafeV1.WorkspaceStatus{
			TotalResources: corev1.ResourceList{
				corev1.ResourceName(AMDGPUResourceName): *resource.NewQuantity(gpuCount, resource.DecimalSI),
			},
		},
	}
}

func createTestWorkspaceWithNVIDIAGPU(name string, gpuCount int64) primusSafeV1.Workspace {
	return primusSafeV1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: primusSafeV1.WorkspaceStatus{
			TotalResources: corev1.ResourceList{
				corev1.ResourceName(NVIDIAGPUResourceName): *resource.NewQuantity(gpuCount, resource.DecimalSI),
			},
		},
	}
}

// ==================== Test Cases ====================

func TestNamespaceSyncService_Name(t *testing.T) {
	svc := NewNamespaceSyncService(nil)
	assert.Equal(t, "namespace-sync", svc.Name())
}

func TestNewNamespaceSyncService_Default(t *testing.T) {
	svc := NewNamespaceSyncService(nil)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.facadeGetter)
	assert.NotNil(t, svc.defaultFacadeGetter)
	assert.NotNil(t, svc.timeNow)
}

func TestNewNamespaceSyncService_WithOptions(t *testing.T) {
	customTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	customTimeFunc := func() time.Time { return customTime }

	mockFacade := &MockFacade{}
	mockFacadeGetter := func(clusterID string) database.FacadeInterface { return mockFacade }
	mockDefaultFacadeGetter := func() database.FacadeInterface { return mockFacade }
	mockWorkspaceLister := &MockWorkspaceLister{}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(mockFacadeGetter),
		WithNamespaceSyncDefaultFacadeGetter(mockDefaultFacadeGetter),
		WithNamespaceSyncTimeNow(customTimeFunc),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	assert.NotNil(t, svc)
	assert.Equal(t, customTime, svc.timeNow())
	assert.Equal(t, mockWorkspaceLister, svc.workspaceLister)
}

// ==================== ExtractGpuResource Tests ====================

func TestExtractGpuResource_NoResources(t *testing.T) {
	workspace := primusSafeV1.Workspace{}
	result := ExtractGpuResource(&workspace)
	assert.Equal(t, int32(0), result)
}

func TestExtractGpuResource_AMDGpu(t *testing.T) {
	workspace := createTestWorkspaceWithAMDGPU("test", 8)
	result := ExtractGpuResource(&workspace)
	assert.Equal(t, int32(8), result)
}

func TestExtractGpuResource_NVIDIAGpu(t *testing.T) {
	workspace := createTestWorkspaceWithNVIDIAGPU("test", 4)
	result := ExtractGpuResource(&workspace)
	assert.Equal(t, int32(4), result)
}

func TestExtractGpuResource_AMDPreferredOverNVIDIA(t *testing.T) {
	workspace := primusSafeV1.Workspace{
		Status: primusSafeV1.WorkspaceStatus{
			TotalResources: corev1.ResourceList{
				corev1.ResourceName(AMDGPUResourceName):    *resource.NewQuantity(8, resource.DecimalSI),
				corev1.ResourceName(NVIDIAGPUResourceName): *resource.NewQuantity(4, resource.DecimalSI),
			},
		},
	}
	result := ExtractGpuResource(&workspace)
	assert.Equal(t, int32(8), result) // AMD should be preferred
}

// ==================== GetGpuModel Tests ====================

func TestGetGpuModel_WithNodeFlavor(t *testing.T) {
	workspace := primusSafeV1.Workspace{
		Spec: primusSafeV1.WorkspaceSpec{
			NodeFlavor: "MI300X",
		},
	}
	result := GetGpuModel(&workspace)
	assert.Equal(t, "MI300X", result)
}

func TestGetGpuModel_NoNodeFlavor(t *testing.T) {
	workspace := primusSafeV1.Workspace{}
	result := GetGpuModel(&workspace)
	assert.Equal(t, "", result)
}

// ==================== GroupWorkspacesByCluster Tests ====================

func TestGroupWorkspacesByCluster_Empty(t *testing.T) {
	result := GroupWorkspacesByCluster([]primusSafeV1.Workspace{})
	assert.Equal(t, 0, len(result))
}

func TestGroupWorkspacesByCluster_SingleCluster(t *testing.T) {
	workspaces := []primusSafeV1.Workspace{
		{ObjectMeta: metav1.ObjectMeta{Name: "ws1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "ws2"}},
	}
	result := GroupWorkspacesByCluster(workspaces)

	// All should be in the same cluster (empty cluster ID by default)
	assert.Equal(t, 1, len(result))
	assert.Equal(t, 2, len(result[""]))
}

// ==================== BuildWorkspaceNodesMap Tests ====================

func TestBuildWorkspaceNodesMap_Empty(t *testing.T) {
	result := BuildWorkspaceNodesMap([]*model.Node{})
	assert.Equal(t, 0, len(result))
}

func TestBuildWorkspaceNodesMap_WithLabels(t *testing.T) {
	nodes := []*model.Node{
		{
			Name:   "node1",
			Labels: model.ExtType{primusSafeV1.WorkspaceIdLabel: "workspace-a"},
		},
		{
			Name:   "node2",
			Labels: model.ExtType{primusSafeV1.WorkspaceIdLabel: "workspace-a"},
		},
		{
			Name:   "node3",
			Labels: model.ExtType{primusSafeV1.WorkspaceIdLabel: "workspace-b"},
		},
	}

	result := BuildWorkspaceNodesMap(nodes)

	assert.Equal(t, 2, len(result))
	assert.Equal(t, 2, len(result["workspace-a"]))
	assert.Equal(t, 1, len(result["workspace-b"]))
}

func TestBuildWorkspaceNodesMap_NoLabels(t *testing.T) {
	nodes := []*model.Node{
		{Name: "node1"},
		{Name: "node2", Labels: model.ExtType{}},
	}

	result := BuildWorkspaceNodesMap(nodes)
	assert.Equal(t, 0, len(result))
}

// ==================== Run Tests ====================

func TestNamespaceSyncService_Run_NoWorkspaces(t *testing.T) {
	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return []primusSafeV1.Workspace{}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
}

func TestNamespaceSyncService_Run_ListWorkspacesError(t *testing.T) {
	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return nil, errors.New("k8s error")
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "k8s error")
}

func TestNamespaceSyncService_Run_CreateNewNamespaceInfo(t *testing.T) {
	createdInfos := make([]*model.NamespaceInfo, 0)

	mockNamespaceInfoFacade := &MockNamespaceInfoFacade{
		ListAllIncludingDeletedFunc: func(ctx context.Context) ([]*model.NamespaceInfo, error) {
			return []*model.NamespaceInfo{}, nil
		},
		CreateFunc: func(ctx context.Context, info *model.NamespaceInfo) error {
			createdInfos = append(createdInfos, info)
			return nil
		},
	}

	mockNodeFacade := &MockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
			return []*model.Node{}, 0, nil
		},
	}

	mockNodeNamespaceMappingFacade := &MockNodeNamespaceMappingFacade{}

	mockFacade := &MockFacade{
		namespaceInfoFacade:        mockNamespaceInfoFacade,
		nodeFacade:                 mockNodeFacade,
		nodeNamespaceMappingFacade: mockNodeNamespaceMappingFacade,
	}

	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return []primusSafeV1.Workspace{
				createTestWorkspaceWithAMDGPU("production", 16),
			}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(func(clusterID string) database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncDefaultFacadeGetter(func() database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(createdInfos))
	assert.Equal(t, "production", createdInfos[0].Name)
	assert.Equal(t, int32(16), createdInfos[0].GpuResource)
}

func TestNamespaceSyncService_Run_UpdateExistingNamespaceInfo(t *testing.T) {
	updatedInfos := make([]*model.NamespaceInfo, 0)

	existingInfo := &model.NamespaceInfo{
		ID:          1,
		Name:        "production",
		GpuModel:    "old-model",
		GpuResource: 8,
	}

	mockNamespaceInfoFacade := &MockNamespaceInfoFacade{
		ListAllIncludingDeletedFunc: func(ctx context.Context) ([]*model.NamespaceInfo, error) {
			return []*model.NamespaceInfo{existingInfo}, nil
		},
		UpdateFunc: func(ctx context.Context, info *model.NamespaceInfo) error {
			updatedInfos = append(updatedInfos, info)
			return nil
		},
	}

	mockNodeFacade := &MockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
			return []*model.Node{}, 0, nil
		},
	}

	mockFacade := &MockFacade{
		namespaceInfoFacade: mockNamespaceInfoFacade,
		nodeFacade:          mockNodeFacade,
		nodeNamespaceMappingFacade: &MockNodeNamespaceMappingFacade{},
	}

	ws := createTestWorkspaceWithAMDGPU("production", 16)
	ws.Spec.NodeFlavor = "MI300X"

	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return []primusSafeV1.Workspace{ws}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(func(clusterID string) database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncDefaultFacadeGetter(func() database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(updatedInfos))
	assert.Equal(t, int32(16), updatedInfos[0].GpuResource)
	assert.Equal(t, "MI300X", updatedInfos[0].GpuModel)
}

func TestNamespaceSyncService_Run_RecoverDeletedNamespaceInfo(t *testing.T) {
	recoveredNames := make([]string, 0)

	existingInfo := &model.NamespaceInfo{
		ID:        1,
		Name:      "production",
		DeletedAt: gorm.DeletedAt{Valid: true, Time: time.Now().Add(-time.Hour)},
	}

	mockNamespaceInfoFacade := &MockNamespaceInfoFacade{
		ListAllIncludingDeletedFunc: func(ctx context.Context) ([]*model.NamespaceInfo, error) {
			return []*model.NamespaceInfo{existingInfo}, nil
		},
		RecoverFunc: func(ctx context.Context, name string, gpuModel string, gpuResource int32) error {
			recoveredNames = append(recoveredNames, name)
			return nil
		},
	}

	mockNodeFacade := &MockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
			return []*model.Node{}, 0, nil
		},
	}

	mockFacade := &MockFacade{
		namespaceInfoFacade: mockNamespaceInfoFacade,
		nodeFacade:          mockNodeFacade,
		nodeNamespaceMappingFacade: &MockNodeNamespaceMappingFacade{},
	}

	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return []primusSafeV1.Workspace{
				createTestWorkspaceWithAMDGPU("production", 16),
			}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(func(clusterID string) database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncDefaultFacadeGetter(func() database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(recoveredNames))
	assert.Equal(t, "production", recoveredNames[0])
}

func TestNamespaceSyncService_Run_SoftDeleteOrphanedNamespaceInfo(t *testing.T) {
	deletedNames := make([]string, 0)

	existingInfo := &model.NamespaceInfo{
		ID:   1,
		Name: "orphaned-namespace",
	}

	mockNamespaceInfoFacade := &MockNamespaceInfoFacade{
		ListAllIncludingDeletedFunc: func(ctx context.Context) ([]*model.NamespaceInfo, error) {
			return []*model.NamespaceInfo{existingInfo}, nil
		},
		DeleteByNameFunc: func(ctx context.Context, name string) error {
			deletedNames = append(deletedNames, name)
			return nil
		},
	}

	mockNodeFacade := &MockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
			return []*model.Node{}, 0, nil
		},
	}

	mockFacade := &MockFacade{
		namespaceInfoFacade: mockNamespaceInfoFacade,
		nodeFacade:          mockNodeFacade,
		nodeNamespaceMappingFacade: &MockNodeNamespaceMappingFacade{},
	}

	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			// Return workspaces that don't include the orphaned one
			return []primusSafeV1.Workspace{
				createTestWorkspaceWithAMDGPU("production", 16),
			}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(func(clusterID string) database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncDefaultFacadeGetter(func() database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(deletedNames))
	assert.Equal(t, "orphaned-namespace", deletedNames[0])
}

func TestNamespaceSyncService_Run_SkipWorkspaceWithNoGpu(t *testing.T) {
	createdInfos := make([]*model.NamespaceInfo, 0)

	mockNamespaceInfoFacade := &MockNamespaceInfoFacade{
		ListAllIncludingDeletedFunc: func(ctx context.Context) ([]*model.NamespaceInfo, error) {
			return []*model.NamespaceInfo{}, nil
		},
		CreateFunc: func(ctx context.Context, info *model.NamespaceInfo) error {
			createdInfos = append(createdInfos, info)
			return nil
		},
	}

	mockNodeFacade := &MockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
			return []*model.Node{}, 0, nil
		},
	}

	mockFacade := &MockFacade{
		namespaceInfoFacade: mockNamespaceInfoFacade,
		nodeFacade:          mockNodeFacade,
		nodeNamespaceMappingFacade: &MockNodeNamespaceMappingFacade{},
	}

	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return []primusSafeV1.Workspace{
				{ObjectMeta: metav1.ObjectMeta{Name: "no-gpu-workspace"}},
			}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(func(clusterID string) database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncDefaultFacadeGetter(func() database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 0, len(createdInfos)) // No namespace_info created for workspaces without GPU
}

// ==================== Node Mapping Tests ====================

func TestNamespaceSyncService_Run_AddNodeMapping(t *testing.T) {
	createdMappings := make([]*model.NodeNamespaceMapping, 0)

	existingInfo := &model.NamespaceInfo{
		ID:          1,
		Name:        "production",
		GpuResource: 16,
	}

	mockNamespaceInfoFacade := &MockNamespaceInfoFacade{
		ListAllIncludingDeletedFunc: func(ctx context.Context) ([]*model.NamespaceInfo, error) {
			return []*model.NamespaceInfo{existingInfo}, nil
		},
		GetByNameFunc: func(ctx context.Context, name string) (*model.NamespaceInfo, error) {
			if name == "production" {
				return existingInfo, nil
			}
			return nil, nil
		},
	}

	mockNodeFacade := &MockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
			return []*model.Node{
				{
					ID:     1,
					Name:   "node-1",
					Labels: model.ExtType{primusSafeV1.WorkspaceIdLabel: "production"},
				},
			}, 1, nil
		},
	}

	mockNodeNamespaceMappingFacade := &MockNodeNamespaceMappingFacade{
		ListActiveByNamespaceNameFunc: func(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error) {
			return []*model.NodeNamespaceMapping{}, nil // No existing mappings
		},
		CreateFunc: func(ctx context.Context, mapping *model.NodeNamespaceMapping) error {
			createdMappings = append(createdMappings, mapping)
			return nil
		},
		GetLatestHistoryByNodeAndNamespaceFunc: func(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMappingHistory, error) {
			return nil, nil
		},
		CreateHistoryFunc: func(ctx context.Context, history *model.NodeNamespaceMappingHistory) error {
			return nil
		},
	}

	mockFacade := &MockFacade{
		namespaceInfoFacade:        mockNamespaceInfoFacade,
		nodeFacade:                 mockNodeFacade,
		nodeNamespaceMappingFacade: mockNodeNamespaceMappingFacade,
	}

	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return []primusSafeV1.Workspace{
				createTestWorkspaceWithAMDGPU("production", 16),
			}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(func(clusterID string) database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncDefaultFacadeGetter(func() database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(createdMappings))
	assert.Equal(t, "node-1", createdMappings[0].NodeName)
	assert.Equal(t, "production", createdMappings[0].NamespaceName)
}

func TestNamespaceSyncService_Run_RemoveNodeMapping(t *testing.T) {
	softDeletedIDs := make([]int32, 0)

	existingInfo := &model.NamespaceInfo{
		ID:          1,
		Name:        "production",
		GpuResource: 16,
	}

	existingMapping := &model.NodeNamespaceMapping{
		ID:            1,
		NodeID:        1,
		NodeName:      "old-node",
		NamespaceID:   1,
		NamespaceName: "production",
	}

	mockNamespaceInfoFacade := &MockNamespaceInfoFacade{
		ListAllIncludingDeletedFunc: func(ctx context.Context) ([]*model.NamespaceInfo, error) {
			return []*model.NamespaceInfo{existingInfo}, nil
		},
		GetByNameFunc: func(ctx context.Context, name string) (*model.NamespaceInfo, error) {
			if name == "production" {
				return existingInfo, nil
			}
			return nil, nil
		},
	}

	mockNodeFacade := &MockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
			return []*model.Node{}, 0, nil // No nodes in workspace anymore
		},
	}

	mockNodeNamespaceMappingFacade := &MockNodeNamespaceMappingFacade{
		ListActiveByNamespaceNameFunc: func(ctx context.Context, namespaceName string) ([]*model.NodeNamespaceMapping, error) {
			return []*model.NodeNamespaceMapping{existingMapping}, nil
		},
		SoftDeleteFunc: func(ctx context.Context, id int32) error {
			softDeletedIDs = append(softDeletedIDs, id)
			return nil
		},
		GetLatestHistoryByNodeAndNamespaceFunc: func(ctx context.Context, nodeID int32, namespaceID int64) (*model.NodeNamespaceMappingHistory, error) {
			return nil, nil
		},
	}

	mockFacade := &MockFacade{
		namespaceInfoFacade:        mockNamespaceInfoFacade,
		nodeFacade:                 mockNodeFacade,
		nodeNamespaceMappingFacade: mockNodeNamespaceMappingFacade,
	}

	mockWorkspaceLister := &MockWorkspaceLister{
		ListWorkspacesFunc: func(ctx context.Context) ([]primusSafeV1.Workspace, error) {
			return []primusSafeV1.Workspace{
				createTestWorkspaceWithAMDGPU("production", 16),
			}, nil
		},
	}

	svc := NewNamespaceSyncService(nil,
		WithNamespaceSyncFacadeGetter(func(clusterID string) database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncDefaultFacadeGetter(func() database.FacadeInterface { return mockFacade }),
		WithNamespaceSyncWorkspaceLister(mockWorkspaceLister),
	)

	ctx := context.Background()
	err := svc.Run(ctx)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(softDeletedIDs))
	assert.Equal(t, int32(1), softDeletedIDs[0])
}

// ==================== Constants Tests ====================

func TestConstants(t *testing.T) {
	assert.Equal(t, "amd.com/gpu", AMDGPUResourceName)
	assert.Equal(t, "nvidia.com/gpu", NVIDIAGPUResourceName)
	assert.Equal(t, -48*time.Hour, DefaultHistoryStartOffset)
}

// ==================== NodeMappingSyncStats Tests ====================

func TestNodeMappingSyncStats_ZeroValues(t *testing.T) {
	stats := &NodeMappingSyncStats{}
	assert.Equal(t, 0, stats.Added)
	assert.Equal(t, 0, stats.Removed)
	assert.Equal(t, 0, stats.Updated)
}

// ==================== Option Functions Tests ====================

func TestWithNamespaceSyncFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterID string) database.FacadeInterface {
		called = true
		return &MockFacade{}
	}

	svc := &NamespaceSyncService{}
	opt := WithNamespaceSyncFacadeGetter(mockGetter)
	opt(svc)

	svc.facadeGetter("test")
	assert.True(t, called)
}

func TestWithNamespaceSyncDefaultFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func() database.FacadeInterface {
		called = true
		return &MockFacade{}
	}

	svc := &NamespaceSyncService{}
	opt := WithNamespaceSyncDefaultFacadeGetter(mockGetter)
	opt(svc)

	svc.defaultFacadeGetter()
	assert.True(t, called)
}

func TestWithNamespaceSyncTimeNow(t *testing.T) {
	customTime := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	mockTimeFunc := func() time.Time {
		return customTime
	}

	svc := &NamespaceSyncService{}
	opt := WithNamespaceSyncTimeNow(mockTimeFunc)
	opt(svc)

	assert.Equal(t, customTime, svc.timeNow())
}

func TestWithNamespaceSyncWorkspaceLister(t *testing.T) {
	mockLister := &MockWorkspaceLister{}

	svc := &NamespaceSyncService{}
	opt := WithNamespaceSyncWorkspaceLister(mockLister)
	opt(svc)

	assert.Equal(t, mockLister, svc.workspaceLister)
}

