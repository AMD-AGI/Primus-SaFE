package database

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/dal"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockNodeFacade creates a NodeFacade with the test database
type mockNodeFacade struct {
	NodeFacade
	db *gorm.DB
}

func (f *mockNodeFacade) getDB() *gorm.DB {
	return f.db
}

func (f *mockNodeFacade) getDAL() *dal.Query {
	return dal.Use(f.db)
}

// newTestNodeFacade creates a test NodeFacade
func newTestNodeFacade(db *gorm.DB) NodeFacadeInterface {
	return &mockNodeFacade{
		db: db,
	}
}

// TestNodeFacade_CreateNode tests creating node entries
func TestNodeFacade_CreateNode(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	node := &model.Node{
		Name:           "test-node-1",
		Address:        "192.168.1.100",
		GpuName:        "AMD MI300X",
		GpuAllocation:  4,
		GpuCount:       8,
		GpuUtilization: 75.5,
		Status:         "active",
		CPU:            "AMD EPYC 9654",
		CPUCount:       96,
		Memory:         "512Gi",
		K8sVersion:     "v1.28.0",
		K8sStatus:      "Ready",
	}
	
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	assert.NotZero(t, node.ID)
	assert.NotZero(t, node.CreatedAt)
	assert.NotZero(t, node.UpdatedAt)
}

// TestNodeFacade_UpdateNode tests updating node entries
func TestNodeFacade_UpdateNode(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node
	node := &model.Node{
		Name:           "test-node-update",
		Address:        "192.168.1.101",
		GpuAllocation:  2,
		GpuCount:       8,
		GpuUtilization: 50.0,
		Status:         "active",
	}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	// Update the node
	node.GpuAllocation = 6
	node.GpuUtilization = 85.5
	node.Status = "busy"
	err = facade.UpdateNode(ctx, node)
	require.NoError(t, err)
	
	// Verify the update
	result, err := facade.GetNodeByName(ctx, "test-node-update")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, int32(6), result.GpuAllocation)
	assert.Equal(t, 85.5, result.GpuUtilization)
	assert.Equal(t, "busy", result.Status)
}

// TestNodeFacade_GetNodeByName tests getting node by name
func TestNodeFacade_GetNodeByName(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node
	node := &model.Node{
		Name:    "test-node-get",
		Address: "192.168.1.102",
		Status:  "active",
	}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	// Get the node by name
	result, err := facade.GetNodeByName(ctx, "test-node-get")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, node.Name, result.Name)
	assert.Equal(t, node.Address, result.Address)
	assert.Equal(t, node.Status, result.Status)
}

// TestNodeFacade_GetNodeByName_NotFound tests getting non-existent node
func TestNodeFacade_GetNodeByName_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetNodeByName(ctx, "non-existent-node")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestNodeFacade_SearchNode tests searching nodes with filters
func TestNodeFacade_SearchNode(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple nodes
	nodes := []*model.Node{
		{Name: "node-1", Address: "192.168.1.1", GpuName: "AMD MI300X", GpuCount: 8, GpuUtilization: 50.0, Status: "active"},
		{Name: "node-2", Address: "192.168.1.2", GpuName: "AMD MI300X", GpuCount: 8, GpuUtilization: 75.0, Status: "active"},
		{Name: "node-3", Address: "192.168.1.3", GpuName: "AMD MI250X", GpuCount: 4, GpuUtilization: 90.0, Status: "busy"},
		{Name: "node-4", Address: "192.168.1.4", GpuName: "AMD MI300X", GpuCount: 8, GpuUtilization: 25.0, Status: "idle"},
	}
	for _, node := range nodes {
		err := facade.CreateNode(ctx, node)
		require.NoError(t, err)
	}
	
	tests := []struct {
		name          string
		filter        filter.NodeFilter
		expectedCount int
	}{
		{
			name:          "No filter - get all",
			filter:        filter.NodeFilter{},
			expectedCount: 4,
		},
		{
			name: "Filter by name pattern",
			filter: filter.NodeFilter{
				Name: stringPtr("node-1"),
			},
			expectedCount: 1,
		},
		{
			name: "Filter by GPU name",
			filter: filter.NodeFilter{
				GPUName: stringPtr("MI300X"),
			},
			expectedCount: 3,
		},
		{
			name: "Filter by GPU count",
			filter: filter.NodeFilter{
				GPUCount: intPtr(8),
			},
			expectedCount: 3,
		},
		{
			name: "Filter by GPU utilization range",
			filter: filter.NodeFilter{
				GPUUtilMin: float64Ptr(50.0),
				GPUUtilMax: float64Ptr(80.0),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by status",
			filter: filter.NodeFilter{
				Status: []string{"active"},
			},
			expectedCount: 2,
		},
		{
			name: "Multiple filters",
			filter: filter.NodeFilter{
				GPUName: stringPtr("MI300X"),
				Status:  []string{"active"},
			},
			expectedCount: 2,
		},
		{
			name: "With pagination",
			filter: filter.NodeFilter{
				Limit:  2,
				Offset: 1,
			},
			expectedCount: 2,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, count, err := facade.SearchNode(ctx, tt.filter)
			require.NoError(t, err)
			
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, 4, count) // Total count should always be 4
		})
	}
}

// TestNodeFacade_SearchNode_OrderBy tests ordering search results
func TestNodeFacade_SearchNode_OrderBy(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create nodes with different utilizations
	nodes := []*model.Node{
		{Name: "node-a", GpuUtilization: 50.0},
		{Name: "node-b", GpuUtilization: 90.0},
		{Name: "node-c", GpuUtilization: 30.0},
	}
	for _, node := range nodes {
		err := facade.CreateNode(ctx, node)
		require.NoError(t, err)
	}
	
	// Search with ordering
	f := filter.NodeFilter{
		OrderBy: "gpu_utilization",
		Order:   "ASC",
	}
	results, _, err := facade.SearchNode(ctx, f)
	require.NoError(t, err)
	require.Len(t, results, 3)
	
	// Verify order (ascending)
	assert.Equal(t, "node-c", results[0].Name)
	assert.Equal(t, "node-a", results[1].Name)
	assert.Equal(t, "node-b", results[2].Name)
}

// TestNodeFacade_CreateGpuDevice tests creating GPU device entries
func TestNodeFacade_CreateGpuDevice(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node first
	node := &model.Node{Name: "gpu-test-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	// Create a GPU device
	device := &model.GpuDevice{
		NodeID:      node.ID,
		GpuID:       0,
		GpuModel:    "AMD MI300X",
		Memory:      192,
		Utilization: 75.5,
		Temperature: 65.0,
		Power:       350.0,
		Serial:      "GPU-12345",
	}
	
	err = facade.CreateGpuDevice(ctx, device)
	require.NoError(t, err)
	assert.NotZero(t, device.ID)
}

// TestNodeFacade_GetGpuDeviceByNodeAndGpuId tests getting GPU device by node and GPU ID
func TestNodeFacade_GetGpuDeviceByNodeAndGpuId(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node and GPU device
	node := &model.Node{Name: "gpu-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	device := &model.GpuDevice{
		NodeID:   node.ID,
		GpuID:    3,
		GpuModel: "AMD MI300X",
	}
	err = facade.CreateGpuDevice(ctx, device)
	require.NoError(t, err)
	
	// Get the GPU device
	result, err := facade.GetGpuDeviceByNodeAndGpuId(ctx, node.ID, 3)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, device.NodeID, result.NodeID)
	assert.Equal(t, device.GpuID, result.GpuID)
	assert.Equal(t, device.GpuModel, result.GpuModel)
}

// TestNodeFacade_UpdateGpuDevice tests updating GPU device entries
func TestNodeFacade_UpdateGpuDevice(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node and GPU device
	node := &model.Node{Name: "gpu-update-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	device := &model.GpuDevice{
		NodeID:      node.ID,
		GpuID:       0,
		Utilization: 50.0,
		Temperature: 60.0,
	}
	err = facade.CreateGpuDevice(ctx, device)
	require.NoError(t, err)
	
	// Update the GPU device
	device.Utilization = 85.0
	device.Temperature = 75.0
	err = facade.UpdateGpuDevice(ctx, device)
	require.NoError(t, err)
	
	// Verify the update
	result, err := facade.GetGpuDeviceByNodeAndGpuId(ctx, node.ID, 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, 85.0, result.Utilization)
	assert.Equal(t, 75.0, result.Temperature)
}

// TestNodeFacade_ListGpuDeviceByNodeId tests listing GPU devices by node ID
func TestNodeFacade_ListGpuDeviceByNodeId(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node
	node := &model.Node{Name: "multi-gpu-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	// Create multiple GPU devices
	for i := 0; i < 8; i++ {
		device := &model.GpuDevice{
			NodeID:   node.ID,
			GpuID:    int32(i),
			GpuModel: "AMD MI300X",
		}
		err = facade.CreateGpuDevice(ctx, device)
		require.NoError(t, err)
	}
	
	// List all GPU devices for the node
	devices, err := facade.ListGpuDeviceByNodeId(ctx, node.ID)
	require.NoError(t, err)
	assert.Len(t, devices, 8)
	
	// Verify they are ordered by GPU ID
	for i, device := range devices {
		assert.Equal(t, int32(i), device.GpuID)
	}
}

// TestNodeFacade_DeleteGpuDeviceById tests deleting GPU devices
func TestNodeFacade_DeleteGpuDeviceById(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node and GPU device
	node := &model.Node{Name: "delete-gpu-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	device := &model.GpuDevice{
		NodeID: node.ID,
		GpuID:  0,
	}
	err = facade.CreateGpuDevice(ctx, device)
	require.NoError(t, err)
	
	// Delete the GPU device
	err = facade.DeleteGpuDeviceById(ctx, device.ID)
	require.NoError(t, err)
	
	// Verify it's deleted
	result, err := facade.GetGpuDeviceByNodeAndGpuId(ctx, node.ID, 0)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestNodeFacade_CreateRdmaDevice tests creating RDMA device entries
func TestNodeFacade_CreateRdmaDevice(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node first
	node := &model.Node{Name: "rdma-test-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	// Create an RDMA device
	device := &model.RdmaDevice{
		NodeID:   node.ID,
		Ifname:   "mlx5_0",
		NodeGUID: "0000:0000:0000:0001",
		IfIndex:  1,
		Fw:       "20.35.1012",
		NodeType: "CA",
	}
	
	err = facade.CreateRdmaDevice(ctx, device)
	require.NoError(t, err)
	assert.NotZero(t, device.ID)
}

// TestNodeFacade_GetRdmaDeviceByNodeIdAndPort tests getting RDMA device
func TestNodeFacade_GetRdmaDeviceByNodeIdAndPort(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node and RDMA device
	node := &model.Node{Name: "rdma-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	nodeGUID := "0000:0000:0000:0002"
	device := &model.RdmaDevice{
		NodeID:   node.ID,
		Ifname:   "mlx5_1",
		NodeGUID: nodeGUID,
		IfIndex:  2,
	}
	err = facade.CreateRdmaDevice(ctx, device)
	require.NoError(t, err)
	
	// Get the RDMA device
	result, err := facade.GetRdmaDeviceByNodeIdAndPort(ctx, nodeGUID, 2)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, device.NodeID, result.NodeID)
	assert.Equal(t, device.Ifname, result.Ifname)
	assert.Equal(t, device.NodeGUID, result.NodeGUID)
	assert.Equal(t, device.IfIndex, result.IfIndex)
}

// TestNodeFacade_ListRdmaDeviceByNodeId tests listing RDMA devices by node ID
func TestNodeFacade_ListRdmaDeviceByNodeId(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node
	node := &model.Node{Name: "multi-rdma-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	// Create multiple RDMA devices
	for i := 0; i < 4; i++ {
		device := &model.RdmaDevice{
			NodeID:   node.ID,
			Ifname:   "mlx5_" + string(rune('0'+i)),
			NodeGUID: "0000:0000:0000:000" + string(rune('0'+i)),
			IfIndex:  int32(i),
		}
		err = facade.CreateRdmaDevice(ctx, device)
		require.NoError(t, err)
	}
	
	// List all RDMA devices for the node
	devices, err := facade.ListRdmaDeviceByNodeId(ctx, node.ID)
	require.NoError(t, err)
	assert.Len(t, devices, 4)
	
	// Verify they are ordered by IfIndex
	for i, device := range devices {
		assert.Equal(t, int32(i), device.IfIndex)
	}
}

// TestNodeFacade_DeleteRdmaDeviceById tests deleting RDMA devices
func TestNodeFacade_DeleteRdmaDeviceById(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a node and RDMA device
	node := &model.Node{Name: "delete-rdma-node"}
	err := facade.CreateNode(ctx, node)
	require.NoError(t, err)
	
	nodeGUID := "0000:0000:0000:0005"
	device := &model.RdmaDevice{
		NodeID:   node.ID,
		NodeGUID: nodeGUID,
		IfIndex:  1,
	}
	err = facade.CreateRdmaDevice(ctx, device)
	require.NoError(t, err)
	
	// Delete the RDMA device
	err = facade.DeleteRdmaDeviceById(ctx, device.ID)
	require.NoError(t, err)
	
	// Verify it's deleted
	result, err := facade.GetRdmaDeviceByNodeIdAndPort(ctx, nodeGUID, 1)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestNodeFacade_WithCluster tests the WithCluster method
func TestNodeFacade_WithCluster(t *testing.T) {
	facade := NewNodeFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*NodeFacadeInterface)(nil), clusterFacade)
}

// Benchmark tests
func BenchmarkNodeFacade_CreateNode(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		node := &model.Node{
			Name:    "bench-node",
			Address: "192.168.1.100",
		}
		_ = facade.CreateNode(ctx, node)
	}
}

func BenchmarkNodeFacade_GetNodeByName(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	node := &model.Node{
		Name:    "bench-node",
		Address: "192.168.1.100",
	}
	_ = facade.CreateNode(ctx, node)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.GetNodeByName(ctx, "bench-node")
	}
}

func BenchmarkNodeFacade_SearchNode(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestNodeFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate with 100 nodes
	for i := 0; i < 100; i++ {
		node := &model.Node{
			Name:           "bench-node-" + string(rune('0'+i%10)),
			GpuUtilization: float64(i % 100),
		}
		_ = facade.CreateNode(ctx, node)
	}
	
	f := filter.NodeFilter{
		Limit: 10,
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.SearchNode(ctx, f)
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func float64Ptr(f float64) *float64 {
	return &f
}

