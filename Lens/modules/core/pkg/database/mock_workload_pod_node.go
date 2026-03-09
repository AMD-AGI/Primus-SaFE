// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package database provides canonical mock implementations for WorkloadFacadeInterface,
// PodFacadeInterface, and NodeFacadeInterface. Downstream modules (jobs, ai-advisor) should
// use these mocks and inject them into MockFacade instead of duplicating implementations.
// When facade interfaces change, update only these mocks.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// MockWorkloadFacade is the canonical mock for WorkloadFacadeInterface. All methods are no-op
// by default; optional callback fields allow tests to stub specific behavior.
type MockWorkloadFacade struct {
	GetGpuWorkloadByUidFunc                    func(ctx context.Context, uid string) (*model.GpuWorkload, error)
	ListWorkloadPodReferenceByWorkloadUidFunc  func(ctx context.Context, workloadUID string) ([]*model.WorkloadPodReference, error)
	GetWorkloadNotEndFunc                      func(ctx context.Context) ([]*model.GpuWorkload, error)
}

// NewMockWorkloadFacade returns a new MockWorkloadFacade.
func NewMockWorkloadFacade() *MockWorkloadFacade {
	return &MockWorkloadFacade{}
}

func (m *MockWorkloadFacade) GetGpuWorkloadByUid(ctx context.Context, uid string) (*model.GpuWorkload, error) {
	if m.GetGpuWorkloadByUidFunc != nil {
		return m.GetGpuWorkloadByUidFunc(ctx, uid)
	}
	return nil, nil
}
func (m *MockWorkloadFacade) GetGpuWorkloadByName(ctx context.Context, name string) (*model.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) CreateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	return nil
}
func (m *MockWorkloadFacade) UpdateGpuWorkload(ctx context.Context, gpuWorkload *model.GpuWorkload) error {
	return nil
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
	if m.GetWorkloadNotEndFunc != nil {
		return m.GetWorkloadNotEndFunc(ctx)
	}
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
func (m *MockWorkloadFacade) ListCompletedWorkloadsByKindAndParent(ctx context.Context, kind, parentUID string, since time.Time, limit int) ([]*model.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListCompletedWorkloadsByKindAndNamespace(ctx context.Context, kind, namespace string, since time.Time, limit int) ([]*model.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListActiveTopLevelWorkloads(ctx context.Context, startTime, endTime time.Time, namespace string) ([]*model.GpuWorkload, error) {
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
	if m.ListWorkloadPodReferenceByWorkloadUidFunc != nil {
		return m.ListWorkloadPodReferenceByWorkloadUidFunc(ctx, workloadUid)
	}
	return nil, nil
}
func (m *MockWorkloadFacade) GetAllWorkloadPodReferences(ctx context.Context) ([]*model.WorkloadPodReference, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListWorkloadUidsByPodUids(ctx context.Context, podUids []string) ([]string, error) {
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
func (m *MockWorkloadFacade) WithCluster(clusterName string) WorkloadFacadeInterface {
	return m
}

// MockPodFacade is the canonical mock for PodFacadeInterface. Default no-op; GpuPods map and
// optional callbacks allow tests to stub GetGpuPodsByPodUid / ListPodsByUids.
type MockPodFacade struct {
	GpuPods            map[string]*model.GpuPods
	OnGetGpuPodsByPodUid func(ctx context.Context, podUid string) (*model.GpuPods, error)
	ListPodsByUidsFunc  func(ctx context.Context, uids []string) ([]*model.GpuPods, error)
}

// NewMockPodFacade returns a new MockPodFacade.
func NewMockPodFacade() *MockPodFacade {
	return &MockPodFacade{
		GpuPods: make(map[string]*model.GpuPods),
	}
}

// AddGpuPod is a test helper to add a pod to the mock map (for GetGpuPodsByPodUid).
func (m *MockPodFacade) AddGpuPod(podUID, nodeName string) {
	m.GpuPods[podUID] = &model.GpuPods{UID: podUID, NodeName: nodeName}
}

func (m *MockPodFacade) CreateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error {
	return nil
}
func (m *MockPodFacade) UpdateGpuPods(ctx context.Context, gpuPods *model.GpuPods) error {
	return nil
}
func (m *MockPodFacade) GetGpuPodsByPodUid(ctx context.Context, podUid string) (*model.GpuPods, error) {
	if m.OnGetGpuPodsByPodUid != nil {
		return m.OnGetGpuPodsByPodUid(ctx, podUid)
	}
	if p, ok := m.GpuPods[podUid]; ok {
		return p, nil
	}
	return nil, nil
}
func (m *MockPodFacade) GetGpuPodsByNamespaceName(ctx context.Context, namespace, name string) (*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) GetActiveGpuPodByNodeName(ctx context.Context, nodeName string) ([]*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) GetHistoryGpuPodByNodeName(ctx context.Context, nodeName string, pageNum, pageSize int) ([]*model.GpuPods, int, error) {
	return nil, 0, nil
}
func (m *MockPodFacade) ListActivePodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) ListPodsByUids(ctx context.Context, uids []string) ([]*model.GpuPods, error) {
	if m.ListPodsByUidsFunc != nil {
		return m.ListPodsByUidsFunc(ctx, uids)
	}
	return nil, nil
}
func (m *MockPodFacade) ListActiveGpuPods(ctx context.Context) ([]*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) ListRunningGpuPods(ctx context.Context) ([]*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) GetRunningPodsByOwnerUID(ctx context.Context, ownerUID string) ([]*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) GetRunningPodsByNamePrefix(ctx context.Context, namespace, namePrefix string) ([]*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) ListPodsActiveInTimeRange(ctx context.Context, startTime, endTime time.Time) ([]*model.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) CreateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPodsEvent) error {
	return nil
}
func (m *MockPodFacade) UpdateGpuPodsEvent(ctx context.Context, gpuPods *model.GpuPods) error {
	return nil
}
func (m *MockPodFacade) CreatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error {
	return nil
}
func (m *MockPodFacade) UpdatePodSnapshot(ctx context.Context, podSnapshot *model.PodSnapshot) error {
	return nil
}
func (m *MockPodFacade) GetLastPodSnapshot(ctx context.Context, podUid string, resourceVersion int) (*model.PodSnapshot, error) {
	return nil, nil
}
func (m *MockPodFacade) UpsertLatestPodSnapshot(ctx context.Context, snapshot *model.PodSnapshot) error {
	return nil
}
func (m *MockPodFacade) GetPodResourceByUid(ctx context.Context, uid string) (*model.PodResource, error) {
	return nil, nil
}
func (m *MockPodFacade) CreatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return nil
}
func (m *MockPodFacade) UpdatePodResource(ctx context.Context, podResource *model.PodResource) error {
	return nil
}
func (m *MockPodFacade) ListPodResourcesByUids(ctx context.Context, uids []string) ([]*model.PodResource, error) {
	return nil, nil
}
func (m *MockPodFacade) QueryPodsWithFilters(ctx context.Context, namespace, podName, startTime, endTime string, page, pageSize int) ([]*model.GpuPods, int64, error) {
	return nil, 0, nil
}
func (m *MockPodFacade) GetAverageGPUUtilizationByNode(ctx context.Context, nodeName string) (float64, error) {
	return 0, nil
}
func (m *MockPodFacade) GetLatestGPUMetricsByNode(ctx context.Context, nodeName string) (*model.GpuDevice, error) {
	return nil, nil
}
func (m *MockPodFacade) QueryGPUHistoryByNode(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]*model.GpuDevice, error) {
	return nil, nil
}
func (m *MockPodFacade) ListPodEventsByUID(ctx context.Context, podUID string) ([]*model.GpuPodsEvent, error) {
	return nil, nil
}
func (m *MockPodFacade) WithCluster(clusterName string) PodFacadeInterface {
	return m
}

// MockNodeFacade is the canonical mock for NodeFacadeInterface. SearchNodeFunc allows tests to stub SearchNode.
type MockNodeFacade struct {
	SearchNodeFunc func(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error)
}

// NewMockNodeFacade returns a new MockNodeFacade.
func NewMockNodeFacade() *MockNodeFacade {
	return &MockNodeFacade{}
}

func (m *MockNodeFacade) CreateNode(ctx context.Context, node *model.Node) error {
	return nil
}
func (m *MockNodeFacade) UpdateNode(ctx context.Context, node *model.Node) error {
	return nil
}
func (m *MockNodeFacade) GetNodeByName(ctx context.Context, name string) (*model.Node, error) {
	return nil, nil
}
func (m *MockNodeFacade) SearchNode(ctx context.Context, f filter.NodeFilter) ([]*model.Node, int, error) {
	if m.SearchNodeFunc != nil {
		return m.SearchNodeFunc(ctx, f)
	}
	return nil, 0, nil
}
func (m *MockNodeFacade) ListGpuNodes(ctx context.Context) ([]*model.Node, error) {
	return nil, nil
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
func (m *MockNodeFacade) WithCluster(clusterName string) NodeFacadeInterface {
	return m
}
