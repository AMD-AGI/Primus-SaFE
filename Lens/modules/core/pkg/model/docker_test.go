// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDockerDeviceInfo tests the DockerDeviceInfo struct
func TestDockerDeviceInfo(t *testing.T) {
	device := DockerDeviceInfo{
		PathOnHost:        "/dev/dri/renderD128",
		PathInContainer:   "/dev/dri/renderD128",
		CgroupPermissions: "rwm",
		DeviceType:        constant.DeviceTypeGPU,
		DeviceSerial:      "GPU-123",
		DeviceName:        "AMD MI250X",
		DeviceId:          0,
		Kind:              "GPU",
		Slot:              "0000:43:00.0",
	}

	assert.Equal(t, "/dev/dri/renderD128", device.PathOnHost)
	assert.Equal(t, "/dev/dri/renderD128", device.PathInContainer)
	assert.Equal(t, "rwm", device.CgroupPermissions)
	assert.Equal(t, constant.DeviceTypeGPU, device.DeviceType)
	assert.Equal(t, "GPU-123", device.DeviceSerial)
	assert.Equal(t, "AMD MI250X", device.DeviceName)
	assert.Equal(t, 0, device.DeviceId)
	assert.Equal(t, "GPU", device.Kind)
	assert.Equal(t, "0000:43:00.0", device.Slot)
}

// TestDockerDeviceInfo_JSONMarshal tests JSON marshaling
func TestDockerDeviceInfo_JSONMarshal(t *testing.T) {
	device := DockerDeviceInfo{
		PathOnHost:      "/dev/dri/renderD128",
		PathInContainer: "/dev/dri/renderD128",
		DeviceType:      "gpu",
		DeviceId:        0,
	}

	data, err := json.Marshal(device)
	require.NoError(t, err)

	var decoded DockerDeviceInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, device.PathOnHost, decoded.PathOnHost)
	assert.Equal(t, device.DeviceType, decoded.DeviceType)
	assert.Equal(t, device.DeviceId, decoded.DeviceId)
}

// TestDockerMountInfo tests the DockerMountInfo struct
func TestDockerMountInfo(t *testing.T) {
	mount := DockerMountInfo{
		Type:        "bind",
		Source:      "/host/path",
		Destination: "/container/path",
	}

	assert.Equal(t, "bind", mount.Type)
	assert.Equal(t, "/host/path", mount.Source)
	assert.Equal(t, "/container/path", mount.Destination)
}

// TestDockerMountInfo_JSONMarshal tests JSON marshaling
func TestDockerMountInfo_JSONMarshal(t *testing.T) {
	mount := DockerMountInfo{
		Type:        "volume",
		Source:      "data-volume",
		Destination: "/data",
	}

	data, err := json.Marshal(mount)
	require.NoError(t, err)

	var decoded DockerMountInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, mount.Type, decoded.Type)
	assert.Equal(t, mount.Source, decoded.Source)
	assert.Equal(t, mount.Destination, decoded.Destination)
}

// TestDockerContainerInfo tests the DockerContainerInfo struct
func TestDockerContainerInfo(t *testing.T) {
	startTime := time.Now()
	container := DockerContainerInfo{
		ID:   "container-123",
		Name: "my-container",
		Labels: map[string]string{
			"app": "test",
			"env": "prod",
		},
		Cmd: "/bin/bash",
		Devices: []DockerDeviceInfo{
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 0},
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 1},
		},
		Mounts: []DockerMountInfo{
			{Type: "bind", Source: "/host", Destination: "/container"},
		},
		StartAt: startTime,
		Status:  "running",
	}

	assert.Equal(t, "container-123", container.ID)
	assert.Equal(t, "my-container", container.Name)
	assert.Len(t, container.Labels, 2)
	assert.Equal(t, "test", container.Labels["app"])
	assert.Len(t, container.Devices, 2)
	assert.Len(t, container.Mounts, 1)
	assert.Equal(t, "running", container.Status)
}

// TestDockerContainerInfo_GpuDeviceCount tests the GpuDeviceCount method
func TestDockerContainerInfo_GpuDeviceCount(t *testing.T) {
	tests := []struct {
		name          string
		devices       []DockerDeviceInfo
		expectedCount int
	}{
		{
			name: "two GPU devices",
			devices: []DockerDeviceInfo{
				{DeviceType: constant.DeviceTypeGPU},
				{DeviceType: constant.DeviceTypeGPU},
			},
			expectedCount: 2,
		},
		{
			name: "mixed devices",
			devices: []DockerDeviceInfo{
				{DeviceType: constant.DeviceTypeGPU},
				{DeviceType: "rdma"},
				{DeviceType: constant.DeviceTypeGPU},
				{DeviceType: "cpu"},
			},
			expectedCount: 2,
		},
		{
			name:          "no devices",
			devices:       []DockerDeviceInfo{},
			expectedCount: 0,
		},
		{
			name: "no GPU devices",
			devices: []DockerDeviceInfo{
				{DeviceType: "rdma"},
				{DeviceType: "cpu"},
			},
			expectedCount: 0,
		},
		{
			name: "single GPU",
			devices: []DockerDeviceInfo{
				{DeviceType: constant.DeviceTypeGPU},
			},
			expectedCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			container := DockerContainerInfo{
				Devices: tt.devices,
			}

			count := container.GpuDeviceCount()
			assert.Equal(t, tt.expectedCount, count)
		})
	}
}

// TestDockerContainerInfo_EmptyLabels tests container with no labels
func TestDockerContainerInfo_EmptyLabels(t *testing.T) {
	container := DockerContainerInfo{
		ID:     "container-1",
		Labels: map[string]string{},
	}

	assert.NotNil(t, container.Labels)
	assert.Len(t, container.Labels, 0)
}

// TestDockerContainerInfo_NilLabels tests container with nil labels
func TestDockerContainerInfo_NilLabels(t *testing.T) {
	container := DockerContainerInfo{
		ID:     "container-1",
		Labels: nil,
	}

	assert.Nil(t, container.Labels)
}

// TestDockerContainerInfo_JSONMarshal tests JSON marshaling
func TestDockerContainerInfo_JSONMarshal(t *testing.T) {
	container := DockerContainerInfo{
		ID:   "test-container",
		Name: "test",
		Labels: map[string]string{
			"key": "value",
		},
		Devices: []DockerDeviceInfo{
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 0},
		},
		Status: "running",
	}

	data, err := json.Marshal(container)
	require.NoError(t, err)

	var decoded DockerContainerInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, container.ID, decoded.ID)
	assert.Equal(t, container.Name, decoded.Name)
	assert.Equal(t, container.Status, decoded.Status)
	assert.Len(t, decoded.Devices, 1)
}

// TestDockerContainerInfo_MultipleGPUs tests container with multiple GPUs
func TestDockerContainerInfo_MultipleGPUs(t *testing.T) {
	container := DockerContainerInfo{
		ID: "gpu-container",
		Devices: []DockerDeviceInfo{
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 0, DeviceName: "GPU-0"},
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 1, DeviceName: "GPU-1"},
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 2, DeviceName: "GPU-2"},
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 3, DeviceName: "GPU-3"},
		},
	}

	count := container.GpuDeviceCount()
	assert.Equal(t, 4, count)
}

// BenchmarkDockerContainerInfo_GpuDeviceCount benchmarks GpuDeviceCount method
func BenchmarkDockerContainerInfo_GpuDeviceCount(b *testing.B) {
	container := DockerContainerInfo{
		Devices: []DockerDeviceInfo{
			{DeviceType: constant.DeviceTypeGPU},
			{DeviceType: "rdma"},
			{DeviceType: constant.DeviceTypeGPU},
			{DeviceType: "cpu"},
			{DeviceType: constant.DeviceTypeGPU},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = container.GpuDeviceCount()
	}
}

// BenchmarkDockerContainerInfo_JSONMarshal benchmarks JSON marshaling
func BenchmarkDockerContainerInfo_JSONMarshal(b *testing.B) {
	container := DockerContainerInfo{
		ID:   "test-container",
		Name: "test",
		Labels: map[string]string{
			"app": "test",
		},
		Devices: []DockerDeviceInfo{
			{DeviceType: constant.DeviceTypeGPU, DeviceId: 0},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(container)
	}
}

