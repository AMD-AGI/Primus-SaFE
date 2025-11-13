package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGPUNode tests the GPUNode struct
func TestGPUNode(t *testing.T) {
	node := GPUNode{
		Name:           "node-1",
		Ip:             "192.168.1.10",
		GpuName:        "AMD MI250X",
		GpuCount:       8,
		GpuAllocation:  6,
		GpuUtilization: 0.75,
		Status:         "Ready",
		StatusColor:    "green",
	}

	assert.Equal(t, "node-1", node.Name)
	assert.Equal(t, "192.168.1.10", node.Ip)
	assert.Equal(t, "AMD MI250X", node.GpuName)
	assert.Equal(t, 8, node.GpuCount)
	assert.Equal(t, 6, node.GpuAllocation)
	assert.InDelta(t, 0.75, node.GpuUtilization, 0.01)
	assert.Equal(t, "Ready", node.Status)
	assert.Equal(t, "green", node.StatusColor)
}

// TestGPUNode_JSONMarshal tests JSON marshaling
func TestGPUNode_JSONMarshal(t *testing.T) {
	node := GPUNode{
		Name:           "test-node",
		Ip:             "10.0.0.1",
		GpuName:        "AMD MI300X",
		GpuCount:       4,
		GpuAllocation:  2,
		GpuUtilization: 0.5,
		Status:         "NotReady",
		StatusColor:    "red",
	}

	data, err := json.Marshal(node)
	require.NoError(t, err)

	var decoded GPUNode
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, node.Name, decoded.Name)
	assert.Equal(t, node.Ip, decoded.Ip)
	assert.Equal(t, node.GpuCount, decoded.GpuCount)
	assert.Equal(t, node.Status, decoded.Status)
}

// TestSearchGpuNodeReq tests the SearchGpuNodeReq struct
func TestSearchGpuNodeReq(t *testing.T) {
	req := SearchGpuNodeReq{
		Name:    "node-1",
		GpuName: "AMD",
		Status:  "Ready",
		OrderBy: "name",
		Desc:    true,
	}
	req.PageNum = 1
	req.PageSize = 10

	assert.Equal(t, "node-1", req.Name)
	assert.Equal(t, "AMD", req.GpuName)
	assert.Equal(t, "Ready", req.Status)
	assert.Equal(t, "name", req.OrderBy)
	assert.True(t, req.Desc)
	assert.Equal(t, 1, req.PageNum)
	assert.Equal(t, 10, req.PageSize)
}

// TestSearchGpuNodeReq_ToNodeFilter tests the ToNodeFilter method
func TestSearchGpuNodeReq_ToNodeFilter(t *testing.T) {
	tests := []struct {
		name               string
		req                SearchGpuNodeReq
		expectedOffset     int
		expectedLimit      int
		expectedOrder      string
		expectedOrderBy    string
		expectedNameNil    bool
		expectedGpuNameNil bool
		expectedStatuses   []string
	}{
		{
			name: "basic filter",
			req: SearchGpuNodeReq{
				Name:    "node-1",
				GpuName: "AMD MI250X",
				Status:  "Ready",
				OrderBy: "name",
				Desc:    false,
			},
			expectedOffset:     0,
			expectedLimit:      10,
			expectedOrder:      "asc",
			expectedOrderBy:    "name",
			expectedNameNil:    false,
			expectedGpuNameNil: false,
			expectedStatuses:   []string{"Ready"},
		},
		{
			name: "descending order",
			req: SearchGpuNodeReq{
				OrderBy: "gpu_count",
				Desc:    true,
			},
			expectedOffset:     0,
			expectedLimit:      20,
			expectedOrder:      "desc",
			expectedOrderBy:    "gpu_count",
			expectedNameNil:    true,
			expectedGpuNameNil: true,
			expectedStatuses:   nil,
		},
		{
			name: "multiple statuses",
			req: SearchGpuNodeReq{
				Status: "Ready,NotReady,Unknown",
			},
			expectedOffset:     0,
			expectedLimit:      10,
			expectedOrder:      "asc",
			expectedOrderBy:    "",
			expectedNameNil:    true,
			expectedGpuNameNil: true,
			expectedStatuses:   []string{"Ready", "NotReady", "Unknown"},
		},
		{
			name: "pagination",
			req: SearchGpuNodeReq{
				OrderBy: "name",
			},
			expectedOffset:     20,
			expectedLimit:      10,
			expectedOrder:      "asc",
			expectedOrderBy:    "name",
			expectedNameNil:    true,
			expectedGpuNameNil: true,
			expectedStatuses:   nil,
		},
		{
			name: "empty request",
			req:  SearchGpuNodeReq{},
			expectedOffset:     -10,
			expectedLimit:      10,
			expectedOrder:      "asc",
			expectedOrderBy:    "",
			expectedNameNil:    true,
			expectedGpuNameNil: true,
			expectedStatuses:   nil,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set default pagination if not set
			if tt.req.PageSize == 0 {
				tt.req.PageSize = 10
			}
			if tt.req.PageNum == 0 {
				if i == 3 { // pagination test case
					tt.req.PageNum = 3
				} else {
					tt.req.PageNum = 1
				}
			}

			filter := tt.req.ToNodeFilter()

			assert.Equal(t, tt.expectedOffset, filter.Offset)
			assert.Equal(t, tt.expectedLimit, filter.Limit)
			assert.Equal(t, tt.expectedOrder, filter.Order)
			assert.Equal(t, tt.expectedOrderBy, filter.OrderBy)

			if tt.expectedNameNil {
				assert.Nil(t, filter.Name)
			} else {
				require.NotNil(t, filter.Name)
				assert.Equal(t, tt.req.Name, *filter.Name)
			}

			if tt.expectedGpuNameNil {
				assert.Nil(t, filter.GPUName)
			} else {
				require.NotNil(t, filter.GPUName)
				assert.Equal(t, tt.req.GpuName, *filter.GPUName)
			}

			assert.Equal(t, tt.expectedStatuses, filter.Status)
		})
	}
}

// TestSearchGpuNodeReq_ToNodeFilter_EdgeCases tests edge cases
func TestSearchGpuNodeReq_ToNodeFilter_EdgeCases(t *testing.T) {
	// Test with very large page numbers
	req := SearchGpuNodeReq{
		Name: "test",
	}
	req.PageNum = 10000
	req.PageSize = 100

	filter := req.ToNodeFilter()
	assert.Equal(t, 999900, filter.Offset) // (10000-1) * 100
	assert.Equal(t, 100, filter.Limit)
}

// TestGpuNodeDetail tests the GpuNodeDetail struct
func TestGpuNodeDetail(t *testing.T) {
	detail := GpuNodeDetail{
		Name:              "node-1",
		Health:            "Healthy",
		Cpu:               "64 cores",
		Memory:            "512GB",
		OS:                "Ubuntu 22.04",
		GPUDriverVersion:  "6.0.0",
		StaticGpuDetails:  "8x AMD MI250X",
		KubeletVersion:    "v1.28.0",
		ContainerdVersion: "1.7.0",
	}

	assert.Equal(t, "node-1", detail.Name)
	assert.Equal(t, "Healthy", detail.Health)
	assert.Equal(t, "64 cores", detail.Cpu)
	assert.Equal(t, "512GB", detail.Memory)
	assert.Equal(t, "6.0.0", detail.GPUDriverVersion)
}

// TestGpuNodeDetail_JSONMarshal tests JSON marshaling
func TestGpuNodeDetail_JSONMarshal(t *testing.T) {
	detail := GpuNodeDetail{
		Name:              "test-node",
		Health:            "Degraded",
		Cpu:               "32 cores",
		Memory:            "256GB",
		OS:                "CentOS 8",
		GPUDriverVersion:  "5.7.0",
		KubeletVersion:    "v1.27.0",
		ContainerdVersion: "1.6.0",
	}

	data, err := json.Marshal(detail)
	require.NoError(t, err)

	var decoded GpuNodeDetail
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, detail.Name, decoded.Name)
	assert.Equal(t, detail.Health, decoded.Health)
	assert.Equal(t, detail.GPUDriverVersion, decoded.GPUDriverVersion)
}

// TestNodeUtilization tests the NodeUtilization struct
func TestNodeUtilization(t *testing.T) {
	util := NodeUtilization{
		NodeName:       "node-1",
		CpuUtilization: 0.75,
		MemUtilization: 0.60,
		GpuUtilization: 0.85,
		GpuAllocation:  6,
		Timestamp:      1609459200,
	}

	assert.Equal(t, "node-1", util.NodeName)
	assert.InDelta(t, 0.75, util.CpuUtilization, 0.01)
	assert.InDelta(t, 0.60, util.MemUtilization, 0.01)
	assert.InDelta(t, 0.85, util.GpuUtilization, 0.01)
	assert.Equal(t, 6, util.GpuAllocation)
	assert.Equal(t, int64(1609459200), util.Timestamp)
}

// TestNodeUtilization_JSONMarshal tests JSON marshaling
func TestNodeUtilization_JSONMarshal(t *testing.T) {
	util := NodeUtilization{
		NodeName:       "test-node",
		CpuUtilization: 0.5,
		MemUtilization: 0.4,
		GpuUtilization: 0.7,
		GpuAllocation:  4,
		Timestamp:      1234567890,
	}

	data, err := json.Marshal(util)
	require.NoError(t, err)

	var decoded NodeUtilization
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, util.NodeName, decoded.NodeName)
	assert.InDelta(t, util.CpuUtilization, decoded.CpuUtilization, 0.01)
	assert.InDelta(t, util.GpuUtilization, decoded.GpuUtilization, 0.01)
	assert.Equal(t, util.GpuAllocation, decoded.GpuAllocation)
}

// BenchmarkSearchGpuNodeReq_ToNodeFilter benchmarks ToNodeFilter method
func BenchmarkSearchGpuNodeReq_ToNodeFilter(b *testing.B) {
	req := SearchGpuNodeReq{
		Name:    "node-1",
		GpuName: "AMD MI250X",
		Status:  "Ready,NotReady",
		OrderBy: "name",
		Desc:    true,
	}
	req.PageNum = 2
	req.PageSize = 20

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = req.ToNodeFilter()
	}
}

// BenchmarkGPUNode_JSONMarshal benchmarks GPUNode JSON marshaling
func BenchmarkGPUNode_JSONMarshal(b *testing.B) {
	node := GPUNode{
		Name:           "node-1",
		Ip:             "192.168.1.10",
		GpuName:        "AMD MI250X",
		GpuCount:       8,
		GpuAllocation:  6,
		GpuUtilization: 0.75,
		Status:         "Ready",
		StatusColor:    "green",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(node)
	}
}

