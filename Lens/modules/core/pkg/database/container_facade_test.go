package database

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/dal"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockContainerFacade creates a ContainerFacade with the test database
type mockContainerFacade struct {
	ContainerFacade
	db *gorm.DB
}

func (f *mockContainerFacade) getDB() *gorm.DB {
	return f.db
}

func (f *mockContainerFacade) getDAL() *dal.Query {
	return dal.Use(f.db)
}

// newTestContainerFacade creates a test ContainerFacade
func newTestContainerFacade(db *gorm.DB) ContainerFacadeInterface {
	return &mockContainerFacade{
		db: db,
	}
}

// ==================== NodeContainer Tests ====================

func TestContainerFacade_CreateNodeContainer(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	container := &model.NodeContainer{
		ContainerID:   "container-001",
		ContainerName: "test-container",
		PodUID:        "pod-uid-001",
		PodName:       "test-pod",
		PodNamespace:  "default",
		Status:        "running",
		NodeName:      "node-1",
		Source:        "kubelet",
	}
	
	err := facade.CreateNodeContainer(ctx, container)
	require.NoError(t, err)
	assert.NotZero(t, container.ID)
}

func TestContainerFacade_GetNodeContainerByContainerId(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a container
	container := &model.NodeContainer{
		ContainerID:   "container-002",
		ContainerName: "get-test",
		PodUID:        "pod-uid-002",
		PodName:       "test-pod-2",
		PodNamespace:  "default",
		Status:        "running",
		NodeName:      "node-1",
		Source:        "kubelet",
	}
	err := facade.CreateNodeContainer(ctx, container)
	require.NoError(t, err)
	
	// Get the container
	result, err := facade.GetNodeContainerByContainerId(ctx, "container-002")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, container.ContainerID, result.ContainerID)
	assert.Equal(t, container.ContainerName, result.ContainerName)
	assert.Equal(t, container.PodUID, result.PodUID)
}

func TestContainerFacade_GetNodeContainerByContainerId_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetNodeContainerByContainerId(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestContainerFacade_UpdateNodeContainer(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a container
	container := &model.NodeContainer{
		ContainerID:   "container-003",
		ContainerName: "update-test",
		PodUID:        "pod-uid-003",
		PodName:       "test-pod-3",
		PodNamespace:  "default",
		Status:        "running",
		NodeName:      "node-1",
		Source:        "kubelet",
	}
	err := facade.CreateNodeContainer(ctx, container)
	require.NoError(t, err)
	
	// Update the container
	container.Status = "terminated"
	err = facade.UpdateNodeContainer(ctx, container)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetNodeContainerByContainerId(ctx, "container-003")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "terminated", result.Status)
}

func TestContainerFacade_ListRunningContainersByPodUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	podUID := "pod-uid-multi"
	
	// Create multiple containers for the same pod
	containers := []*model.NodeContainer{
		{ContainerID: "c1", ContainerName: "container-1", PodUID: podUID, PodName: "multi-container-pod", PodNamespace: "default", Status: "running", NodeName: "node-1", Source: "kubelet"},
		{ContainerID: "c2", ContainerName: "container-2", PodUID: podUID, PodName: "multi-container-pod", PodNamespace: "default", Status: "running", NodeName: "node-1", Source: "kubelet"},
		{ContainerID: "c3", ContainerName: "container-3", PodUID: "other-pod", PodName: "other-pod", PodNamespace: "default", Status: "running", NodeName: "node-1", Source: "kubelet"},
	}
	
	for _, c := range containers {
		err := facade.CreateNodeContainer(ctx, c)
		require.NoError(t, err)
	}
	
	// List containers for specific pod
	results, err := facade.ListRunningContainersByPodUid(ctx, podUID)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.Equal(t, podUID, r.PodUID)
	}
}

// ==================== NodeContainerDevices Tests ====================

func TestContainerFacade_CreateNodeContainerDevice(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	device := &model.NodeContainerDevices{
		ContainerID: "container-001",
		DeviceUUID:  "GPU-abc123",
		DeviceType:  "gpu",
	}
	
	err := facade.CreateNodeContainerDevice(ctx, device)
	require.NoError(t, err)
	assert.NotZero(t, device.ID)
}

func TestContainerFacade_GetNodeContainerDeviceByContainerIdAndDeviceUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	device := &model.NodeContainerDevices{
		ContainerID: "container-dev-001",
		DeviceUUID:  "GPU-xyz789",
		DeviceType:  "gpu",
	}
	err := facade.CreateNodeContainerDevice(ctx, device)
	require.NoError(t, err)
	
	result, err := facade.GetNodeContainerDeviceByContainerIdAndDeviceUid(ctx, "container-dev-001", "GPU-xyz789")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, device.ContainerID, result.ContainerID)
	assert.Equal(t, device.DeviceUUID, result.DeviceUUID)
}

func TestContainerFacade_ListContainerDevicesByContainerId(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	containerID := "container-multi-device"
	
	devices := []*model.NodeContainerDevices{
		{ContainerID: containerID, DeviceUUID: "GPU-0", DeviceType: "gpu"},
		{ContainerID: containerID, DeviceUUID: "GPU-1", DeviceType: "gpu"},
		{ContainerID: "other-container", DeviceUUID: "GPU-2", DeviceType: "gpu"},
	}
	
	for _, d := range devices {
		err := facade.CreateNodeContainerDevice(ctx, d)
		require.NoError(t, err)
	}
	
	results, err := facade.ListContainerDevicesByContainerId(ctx, containerID)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.Equal(t, containerID, r.ContainerID)
	}
}

// ==================== NodeContainerEvent Tests ====================

func TestContainerFacade_CreateNodeContainerEvent(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	event := &model.NodeContainerEvent{
		ContainerID: "container-001",
		EventType:   "start",
	}
	
	err := facade.CreateNodeContainerEvent(ctx, event)
	require.NoError(t, err)
	assert.NotZero(t, event.ID)
}

// ==================== Helper Methods ====================

func TestContainerFacade_WithCluster(t *testing.T) {
	facade := NewContainerFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*ContainerFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkContainerFacade_CreateNodeContainer(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		container := &model.NodeContainer{
			ContainerID:   "bench-container",
			ContainerName: "benchmark",
			PodUID:        "pod-uid",
			PodName:       "bench-pod",
			PodNamespace:  "default",
			Status:        "running",
			NodeName:      "node-1",
			Source:        "kubelet",
		}
		_ = facade.CreateNodeContainer(ctx, container)
	}
}

func BenchmarkContainerFacade_GetNodeContainerByContainerId(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	container := &model.NodeContainer{
		ContainerID:   "bench-get",
		ContainerName: "benchmark",
		PodUID:        "pod-uid",
		PodName:       "bench-pod",
		PodNamespace:  "default",
		Status:        "running",
		NodeName:      "node-1",
		Source:        "kubelet",
	}
	_ = facade.CreateNodeContainer(ctx, container)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.GetNodeContainerByContainerId(ctx, "bench-get")
	}
}

func BenchmarkContainerFacade_ListRunningContainersByPodUid(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestContainerFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	podUID := "bench-pod-uid"
	for i := 0; i < 10; i++ {
		container := &model.NodeContainer{
			ContainerID:   "bench-" + string(rune('a'+i)),
			ContainerName: "benchmark",
			PodUID:        podUID,
			PodName:       "bench-pod",
			PodNamespace:  "default",
			Status:        "running",
			NodeName:      "node-1",
			Source:        "kubelet",
		}
		_ = facade.CreateNodeContainer(ctx, container)
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.ListRunningContainersByPodUid(ctx, podUID)
	}
}

