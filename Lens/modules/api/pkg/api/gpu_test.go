// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"testing"
	"time"

	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestCvtGpuDevice2GpuDeviceInfo(t *testing.T) {
	tests := []struct {
		name     string
		input    *dbModel.GpuDevice
		expected model.GpuDeviceInfo
	}{
		{
			name: "normal GPU device - integer GB memory",
			input: &dbModel.GpuDevice{
				GpuID:       0,
				GpuModel:    "AMD MI300X",
				Memory:      81920, // 80GB in MB
				Utilization: 75.5,
				Temperature: 65.0,
				Power:       350.5,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    0,
				Model:       "AMD MI300X",
				Memory:      "80GB",
				Utilization: 75.5,
				Temperature: 65.0,
				Power:       350.5,
			},
		},
		{
			name: "normal GPU device - 16GB memory",
			input: &dbModel.GpuDevice{
				GpuID:       1,
				GpuModel:    "AMD MI250X",
				Memory:      16384, // 16GB in MB
				Utilization: 50.0,
				Temperature: 55.0,
				Power:       250.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    1,
				Model:       "AMD MI250X",
				Memory:      "16GB",
				Utilization: 50.0,
				Temperature: 55.0,
				Power:       250.0,
			},
		},
		{
			name: "GPU device - 32GB memory",
			input: &dbModel.GpuDevice{
				GpuID:       2,
				GpuModel:    "AMD MI210",
				Memory:      32768, // 32GB in MB
				Utilization: 100.0,
				Temperature: 80.0,
				Power:       300.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    2,
				Model:       "AMD MI210",
				Memory:      "32GB",
				Utilization: 100.0,
				Temperature: 80.0,
				Power:       300.0,
			},
		},
		{
			name: "idle GPU - 0% utilization",
			input: &dbModel.GpuDevice{
				GpuID:       0,
				GpuModel:    "AMD MI300X",
				Memory:      81920,
				Utilization: 0.0,
				Temperature: 40.0,
				Power:       50.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    0,
				Model:       "AMD MI300X",
				Memory:      "80GB",
				Utilization: 0.0,
				Temperature: 40.0,
				Power:       50.0,
			},
		},
		{
			name: "full load GPU - 100% utilization",
			input: &dbModel.GpuDevice{
				GpuID:       3,
				GpuModel:    "AMD MI250",
				Memory:      65536, // 64GB in MB
				Utilization: 100.0,
				Temperature: 85.0,
				Power:       400.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    3,
				Model:       "AMD MI250",
				Memory:      "64GB",
				Utilization: 100.0,
				Temperature: 85.0,
				Power:       400.0,
			},
		},
		{
			name: "small memory GPU - 8GB",
			input: &dbModel.GpuDevice{
				GpuID:       4,
				GpuModel:    "AMD Radeon VII",
				Memory:      8192, // 8GB in MB
				Utilization: 25.5,
				Temperature: 60.0,
				Power:       180.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    4,
				Model:       "AMD Radeon VII",
				Memory:      "8GB",
				Utilization: 25.5,
				Temperature: 60.0,
				Power:       180.0,
			},
		},
		{
			name: "irregular memory size - rounded down",
			input: &dbModel.GpuDevice{
				GpuID:       5,
				GpuModel:    "AMD GPU",
				Memory:      10240, // 10GB in MB
				Utilization: 33.3,
				Temperature: 58.5,
				Power:       200.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    5,
				Model:       "AMD GPU",
				Memory:      "10GB",
				Utilization: 33.3,
				Temperature: 58.5,
				Power:       200.0,
			},
		},
		{
			name: "zero memory GPU",
			input: &dbModel.GpuDevice{
				GpuID:       6,
				GpuModel:    "Test GPU",
				Memory:      0,
				Utilization: 0.0,
				Temperature: 30.0,
				Power:       0.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    6,
				Model:       "Test GPU",
				Memory:      "0GB",
				Utilization: 0.0,
				Temperature: 30.0,
				Power:       0.0,
			},
		},
		{
			name: "negative DeviceId",
			input: &dbModel.GpuDevice{
				GpuID:       -1,
				GpuModel:    "Unknown GPU",
				Memory:      16384,
				Utilization: 0.0,
				Temperature: 0.0,
				Power:       0.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    -1,
				Model:       "Unknown GPU",
				Memory:      "16GB",
				Utilization: 0.0,
				Temperature: 0.0,
				Power:       0.0,
			},
		},
		{
			name: "empty model name",
			input: &dbModel.GpuDevice{
				GpuID:       0,
				GpuModel:    "",
				Memory:      16384,
				Utilization: 50.0,
				Temperature: 60.0,
				Power:       200.0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    0,
				Model:       "",
				Memory:      "16GB",
				Utilization: 50.0,
				Temperature: 60.0,
				Power:       200.0,
			},
		},
		{
			name: "complete database object - includes all fields",
			input: &dbModel.GpuDevice{
				ID:             100,
				NodeID:         10,
				GpuID:          2,
				GpuModel:       "AMD MI300A",
				Memory:         122880, // 120GB
				Utilization:    88.8,
				Temperature:    72.5,
				Power:          380.5,
				Serial:         "SN123456789",
				RdmaDeviceName: "mlx5_0",
				RdmaGUID:       "0x1234567890abcdef",
				RdmaLid:        "100",
				CreatedAt:      time.Now(),
				UpdatedAt:      time.Now(),
				NumaNode:       0,
			},
			expected: model.GpuDeviceInfo{
				DeviceId:    2,
				Model:       "AMD MI300A",
				Memory:      "120GB",
				Utilization: 88.8,
				Temperature: 72.5,
				Power:       380.5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cvtGpuDevice2GpuDeviceInfo(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBatchCvtGpuDevice2GpuDeviceInfo(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		input := []*dbModel.GpuDevice{}
		result := batchCvtGpuDevice2GpuDeviceInfo(input)
		
		// Empty slice may return nil or empty slice, both are legal
		if result != nil {
			assert.Empty(t, result)
		}
	})

	t.Run("single GPU device", func(t *testing.T) {
		input := []*dbModel.GpuDevice{
			{
				GpuID:       0,
				GpuModel:    "AMD MI300X",
				Memory:      81920,
				Utilization: 75.5,
				Temperature: 65.0,
				Power:       350.5,
			},
		}

		result := batchCvtGpuDevice2GpuDeviceInfo(input)
		
		assert.Len(t, result, 1)
		assert.Equal(t, 0, result[0].DeviceId)
		assert.Equal(t, "AMD MI300X", result[0].Model)
		assert.Equal(t, "80GB", result[0].Memory)
		assert.Equal(t, 75.5, result[0].Utilization)
	})

	t.Run("multiple GPU devices", func(t *testing.T) {
		input := []*dbModel.GpuDevice{
			{
				GpuID:       0,
				GpuModel:    "AMD MI300X",
				Memory:      81920,
				Utilization: 75.5,
				Temperature: 65.0,
				Power:       350.5,
			},
			{
				GpuID:       1,
				GpuModel:    "AMD MI250X",
				Memory:      16384,
				Utilization: 50.0,
				Temperature: 55.0,
				Power:       250.0,
			},
			{
				GpuID:       2,
				GpuModel:    "AMD MI210",
				Memory:      32768,
				Utilization: 100.0,
				Temperature: 80.0,
				Power:       300.0,
			},
		}

		result := batchCvtGpuDevice2GpuDeviceInfo(input)
		
		assert.Len(t, result, 3)
		
		// Verify first device
		assert.Equal(t, 0, result[0].DeviceId)
		assert.Equal(t, "AMD MI300X", result[0].Model)
		assert.Equal(t, "80GB", result[0].Memory)
		
		// Verify second device
		assert.Equal(t, 1, result[1].DeviceId)
		assert.Equal(t, "AMD MI250X", result[1].Model)
		assert.Equal(t, "16GB", result[1].Memory)
		
		// Verify third device
		assert.Equal(t, 2, result[2].DeviceId)
		assert.Equal(t, "AMD MI210", result[2].Model)
		assert.Equal(t, "32GB", result[2].Memory)
	})

	t.Run("large number of GPU devices", func(t *testing.T) {
		// Simulate a node with 8 GPUs
		input := make([]*dbModel.GpuDevice, 8)
		for i := 0; i < 8; i++ {
			input[i] = &dbModel.GpuDevice{
				GpuID:       int32(i),
				GpuModel:    "AMD MI300X",
				Memory:      81920,
				Utilization: float64(i * 10),
				Temperature: 60.0 + float64(i),
				Power:       300.0 + float64(i*10),
			}
		}

		result := batchCvtGpuDevice2GpuDeviceInfo(input)
		
		assert.Len(t, result, 8)
		
		// Verify each device conversion is correct
		for i := 0; i < 8; i++ {
			assert.Equal(t, i, result[i].DeviceId)
			assert.Equal(t, "80GB", result[i].Memory)
			assert.Equal(t, float64(i*10), result[i].Utilization)
		}
	})

	t.Run("list with nil elements", func(t *testing.T) {
		input := []*dbModel.GpuDevice{
			{
				GpuID:       0,
				GpuModel:    "AMD MI300X",
				Memory:      81920,
				Utilization: 75.5,
				Temperature: 65.0,
				Power:       350.5,
			},
			nil, // nil element will cause panic
			{
				GpuID:       2,
				GpuModel:    "AMD MI210",
				Memory:      32768,
				Utilization: 100.0,
				Temperature: 80.0,
				Power:       300.0,
			},
		}

		// This test will panic because the code doesn't handle nil
		// In actual use, nil elements should be avoided
		assert.Panics(t, func() {
			batchCvtGpuDevice2GpuDeviceInfo(input)
		})
	})
}

func TestCvtGpuDevice2GpuDeviceInfo_MemoryConversion(t *testing.T) {
	tests := []struct {
		name           string
		memoryInMB     int32
		expectedMemory string
	}{
		{"1GB", 1024, "1GB"},
		{"2GB", 2048, "2GB"},
		{"4GB", 4096, "4GB"},
		{"8GB", 8192, "8GB"},
		{"16GB", 16384, "16GB"},
		{"24GB", 24576, "24GB"},
		{"32GB", 32768, "32GB"},
		{"40GB", 40960, "40GB"},
		{"48GB", 49152, "48GB"},
		{"64GB", 65536, "64GB"},
		{"80GB", 81920, "80GB"},
		{"96GB", 98304, "96GB"},
		{"120GB", 122880, "120GB"},
		{"128GB", 131072, "128GB"},
		{"192GB", 196608, "192GB"},
		{"256GB", 262144, "256GB"},
		{"not integer GB - 1.5GB", 1536, "1GB"}, // rounded down
		{"not integer GB - 2.5GB", 2560, "2GB"}, // rounded down
		{"not integer GB - 7.5GB", 7680, "7GB"}, // rounded down
		{"less than 1GB - 512MB", 512, "0GB"},
		{"less than 1GB - 768MB", 768, "0GB"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := &dbModel.GpuDevice{
				GpuID:       0,
				GpuModel:    "Test GPU",
				Memory:      tt.memoryInMB,
				Utilization: 0.0,
				Temperature: 0.0,
				Power:       0.0,
			}

			result := cvtGpuDevice2GpuDeviceInfo(input)
			assert.Equal(t, tt.expectedMemory, result.Memory)
		})
	}
}

func BenchmarkCvtGpuDevice2GpuDeviceInfo(b *testing.B) {
	device := &dbModel.GpuDevice{
		GpuID:       0,
		GpuModel:    "AMD MI300X",
		Memory:      81920,
		Utilization: 75.5,
		Temperature: 65.0,
		Power:       350.5,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cvtGpuDevice2GpuDeviceInfo(device)
	}
}

func BenchmarkBatchCvtGpuDevice2GpuDeviceInfo(b *testing.B) {
	// Create slice of 8 GPU devices
	devices := make([]*dbModel.GpuDevice, 8)
	for i := 0; i < 8; i++ {
		devices[i] = &dbModel.GpuDevice{
			GpuID:       int32(i),
			GpuModel:    "AMD MI300X",
			Memory:      81920,
			Utilization: float64(i * 10),
			Temperature: 60.0 + float64(i),
			Power:       300.0 + float64(i*10),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = batchCvtGpuDevice2GpuDeviceInfo(devices)
	}
}

