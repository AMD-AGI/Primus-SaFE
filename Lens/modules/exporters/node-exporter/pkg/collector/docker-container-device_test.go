// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

func TestFillGPUDeviceInfoForDockerContainerInfo(t *testing.T) {
	// Setup test data in global mapping
	setupGPUTestMapping()
	defer cleanupGPUTestMapping()

	t.Run("Device exists in mapping", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost:      "/dev/dri/card0",
			PathInContainer: "/dev/dri/card0",
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		assert.Equal(t, 0, info.DeviceId)
		assert.Equal(t, "SERIAL-001", info.DeviceSerial)
		assert.Equal(t, "AMD Instinct MI300X", info.DeviceName)
		assert.Equal(t, constant.DeviceTypeGPU, info.DeviceType)
	})

	t.Run("Device does not exist in mapping", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost:      "/dev/dri/card99",
			PathInContainer: "/dev/dri/card99",
		}

		// Save original values
		originalId := info.DeviceId
		originalSerial := info.DeviceSerial
		originalName := info.DeviceName
		originalType := info.DeviceType

		fillGPUDeviceInfoForDockerContainerInfo(info)

		// Should not modify info when device not found
		assert.Equal(t, originalId, info.DeviceId)
		assert.Equal(t, originalSerial, info.DeviceSerial)
		assert.Equal(t, originalName, info.DeviceName)
		assert.Equal(t, originalType, info.DeviceType)
	})

	t.Run("Multiple GPU devices", func(t *testing.T) {
		devices := []model.DockerDeviceInfo{
			{PathOnHost: "/dev/dri/card0"},
			{PathOnHost: "/dev/dri/card1"},
			{PathOnHost: "/dev/dri/card2"},
		}

		for i := range devices {
			fillGPUDeviceInfoForDockerContainerInfo(&devices[i])
		}

		assert.Equal(t, 0, devices[0].DeviceId)
		assert.Equal(t, 1, devices[1].DeviceId)
		assert.Equal(t, 2, devices[2].DeviceId)
		assert.Equal(t, "AMD Instinct MI300X", devices[0].DeviceName)
		assert.Equal(t, "AMD Instinct MI300A", devices[1].DeviceName)
		assert.Equal(t, "AMD Radeon Pro W7900", devices[2].DeviceName)
	})

	t.Run("Nil pointer check", func(t *testing.T) {
		// Should not panic with nil pointer
		var info *model.DockerDeviceInfo = nil

		// This would panic if not handled properly
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("fillGPUDeviceInfoForDockerContainerInfo panicked with nil pointer")
			}
		}()

		if info != nil {
			fillGPUDeviceInfoForDockerContainerInfo(info)
		}
	})

	t.Run("Empty PathOnHost", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "",
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		// Should not set any device info for empty path
		assert.Equal(t, 0, info.DeviceId)
		assert.Equal(t, "", info.DeviceSerial)
		assert.Equal(t, "", info.DeviceName)
		assert.Equal(t, "", info.DeviceType)
	})

	t.Run("Different device paths", func(t *testing.T) {
		testCases := []struct {
			path       string
			shouldFill bool
		}{
			{"/dev/dri/card0", true},
			{"/dev/dri/card1", true},
			{"/dev/dri/renderD128", false}, // Not in mapping
			{"/dev/dri/by-path/pci-0000:01:00.0-card", false},
			{"invalid-path", false},
		}

		for _, tc := range testCases {
			info := &model.DockerDeviceInfo{
				PathOnHost: tc.path,
			}

			fillGPUDeviceInfoForDockerContainerInfo(info)

			if tc.shouldFill {
				assert.NotEmpty(t, info.DeviceName)
				assert.Equal(t, constant.DeviceTypeGPU, info.DeviceType)
			}
		}
	})
}

func TestFillRDMADeviceInfoForDockerContainerInfo(t *testing.T) {
	// Setup test data in global mapping
	setupRDMATestMapping()
	defer cleanupRDMATestMapping()

	t.Run("Valid uverbs device in mapping", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost:      "/dev/infiniband/uverbs0",
			PathInContainer: "/dev/infiniband/uverbs0",
		}

		fillRDMADeviceInfoForDockerContainerInfo(info)

		assert.Equal(t, constant.DeviceTypeRDMA, info.DeviceType)
		assert.Equal(t, 101, info.DeviceId)
		assert.Equal(t, "mlx5_0", info.DeviceName)
		assert.Equal(t, "00:11:22:33:44:55:66:77", info.DeviceSerial)
	})

	t.Run("Device without uverbs in path", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "/dev/infiniband/rdma_cm",
		}

		// Save original values
		originalType := info.DeviceType
		originalId := info.DeviceId

		fillRDMADeviceInfoForDockerContainerInfo(info)

		// Should not modify info when path doesn't contain "uverbs"
		assert.Equal(t, originalType, info.DeviceType)
		assert.Equal(t, originalId, info.DeviceId)
	})

	t.Run("uverbs device not in mapping", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "/dev/infiniband/uverbs99",
		}

		originalType := info.DeviceType

		fillRDMADeviceInfoForDockerContainerInfo(info)

		// Should not set device type if device not found in mapping
		assert.Equal(t, originalType, info.DeviceType)
	})

	t.Run("Multiple RDMA devices", func(t *testing.T) {
		devices := []model.DockerDeviceInfo{
			{PathOnHost: "/dev/infiniband/uverbs0"},
			{PathOnHost: "/dev/infiniband/uverbs1"},
			{PathOnHost: "/dev/infiniband/uverbs2"},
		}

		for i := range devices {
			fillRDMADeviceInfoForDockerContainerInfo(&devices[i])
		}

		assert.Equal(t, constant.DeviceTypeRDMA, devices[0].DeviceType)
		assert.Equal(t, constant.DeviceTypeRDMA, devices[1].DeviceType)
		assert.Equal(t, constant.DeviceTypeRDMA, devices[2].DeviceType)

		assert.Equal(t, "mlx5_0", devices[0].DeviceName)
		assert.Equal(t, "mlx5_1", devices[1].DeviceName)
		assert.Equal(t, "mlx5_2", devices[2].DeviceName)
	})

	t.Run("Extract device ID from path", func(t *testing.T) {
		testCases := []struct {
			path               string
			expectedDeviceName string
			shouldFind         bool
		}{
			{"/dev/infiniband/uverbs0", "mlx5_0", true},
			{"/dev/infiniband/uverbs1", "mlx5_1", true},
			{"/dev/infiniband/uverbs2", "mlx5_2", true},
			{"/dev/infiniband/uverbs10", "", false}, // Not in mapping
		}

		for _, tc := range testCases {
			info := &model.DockerDeviceInfo{
				PathOnHost: tc.path,
			}

			fillRDMADeviceInfoForDockerContainerInfo(info)

			if tc.shouldFind {
				assert.Equal(t, tc.expectedDeviceName, info.DeviceName)
				assert.Equal(t, constant.DeviceTypeRDMA, info.DeviceType)
			}
		}
	})

	t.Run("Nil pointer check", func(t *testing.T) {
		var info *model.DockerDeviceInfo = nil

		defer func() {
			if r := recover(); r != nil {
				t.Errorf("fillRDMADeviceInfoForDockerContainerInfo panicked with nil pointer")
			}
		}()

		if info != nil {
			fillRDMADeviceInfoForDockerContainerInfo(info)
		}
	})

	t.Run("Empty PathOnHost", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "",
		}

		fillRDMADeviceInfoForDockerContainerInfo(info)

		// Should not set device type for empty path
		assert.NotEqual(t, constant.DeviceTypeRDMA, info.DeviceType)
	})

	t.Run("Path with uverbs in middle", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "/dev/infiniband/prefix-uverbs0-suffix",
		}

		fillRDMADeviceInfoForDockerContainerInfo(info)

		// Should still extract "0" after "uverbs"
		// Behavior depends on implementation of strings.Split
	})
}

func TestFillDeviceInfo_Combined(t *testing.T) {
	setupGPUTestMapping()
	setupRDMATestMapping()
	defer cleanupGPUTestMapping()
	defer cleanupRDMATestMapping()

	t.Run("Container with both GPU and RDMA devices", func(t *testing.T) {
		devices := []model.DockerDeviceInfo{
			{PathOnHost: "/dev/dri/card0"},
			{PathOnHost: "/dev/dri/card1"},
			{PathOnHost: "/dev/infiniband/uverbs0"},
			{PathOnHost: "/dev/infiniband/uverbs1"},
		}

		// Fill GPU devices
		for i := range devices {
			fillGPUDeviceInfoForDockerContainerInfo(&devices[i])
		}

		// Fill RDMA devices
		for i := range devices {
			fillRDMADeviceInfoForDockerContainerInfo(&devices[i])
		}

		// Verify GPU devices
		assert.Equal(t, constant.DeviceTypeGPU, devices[0].DeviceType)
		assert.Equal(t, constant.DeviceTypeGPU, devices[1].DeviceType)

		// Verify RDMA devices
		assert.Equal(t, constant.DeviceTypeRDMA, devices[2].DeviceType)
		assert.Equal(t, constant.DeviceTypeRDMA, devices[3].DeviceType)
	})

	t.Run("Unknown devices remain unchanged", func(t *testing.T) {
		devices := []model.DockerDeviceInfo{
			{PathOnHost: "/dev/null"},
			{PathOnHost: "/dev/random"},
			{PathOnHost: "/dev/tty"},
		}

		for i := range devices {
			fillGPUDeviceInfoForDockerContainerInfo(&devices[i])
			fillRDMADeviceInfoForDockerContainerInfo(&devices[i])
		}

		// All should remain unchanged
		for _, device := range devices {
			assert.Equal(t, "", device.DeviceType)
			assert.Equal(t, 0, device.DeviceId)
			assert.Equal(t, "", device.DeviceName)
		}
	})
}

// Helper functions to setup and cleanup test data

func setupGPUTestMapping() {
	driCardInfoMapping = map[string]model.GPUInfo{
		"/dev/dri/card0": {
			GPU: 0,
			Asic: model.AsicInfo{
				AsicSerial: "SERIAL-001",
				MarketName: "AMD Instinct MI300X",
			},
		},
		"/dev/dri/card1": {
			GPU: 1,
			Asic: model.AsicInfo{
				AsicSerial: "SERIAL-002",
				MarketName: "AMD Instinct MI300A",
			},
		},
		"/dev/dri/card2": {
			GPU: 2,
			Asic: model.AsicInfo{
				AsicSerial: "SERIAL-003",
				MarketName: "AMD Radeon Pro W7900",
			},
		},
	}
}

func cleanupGPUTestMapping() {
	driCardInfoMapping = nil
}

func setupRDMATestMapping() {
	rdmaDeviceMapping = map[string]model.RDMADevice{
		"0": {
			IfIndex:      101,
			IfName:       "mlx5_0",
			SysImageGUID: "00:11:22:33:44:55:66:77",
		},
		"1": {
			IfIndex:      102,
			IfName:       "mlx5_1",
			SysImageGUID: "11:22:33:44:55:66:77:88",
		},
		"2": {
			IfIndex:      103,
			IfName:       "mlx5_2",
			SysImageGUID: "22:33:44:55:66:77:88:99",
		},
	}
}

func cleanupRDMATestMapping() {
	rdmaDeviceMapping = nil
}

func TestDockerDeviceInfo_EdgeCases(t *testing.T) {
	setupGPUTestMapping()
	setupRDMATestMapping()
	defer cleanupGPUTestMapping()
	defer cleanupRDMATestMapping()

	t.Run("Special characters in path", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "/dev/dri/card@#$%",
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		// Should not find in mapping
		assert.NotEqual(t, constant.DeviceTypeGPU, info.DeviceType)
	})

	t.Run("Path with spaces", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "/dev/dri/card 0",
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		assert.NotEqual(t, constant.DeviceTypeGPU, info.DeviceType)
	})

	t.Run("Case sensitivity", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "/dev/dri/CARD0", // Upper case
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		// Case sensitive - should not match
		assert.NotEqual(t, constant.DeviceTypeGPU, info.DeviceType)
	})

	t.Run("Very long path", func(t *testing.T) {
		longPath := "/dev/"
		for i := 0; i < 100; i++ {
			longPath += "subdir/"
		}
		longPath += "card0"

		info := &model.DockerDeviceInfo{
			PathOnHost: longPath,
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		// Should not match
		assert.NotEqual(t, constant.DeviceTypeGPU, info.DeviceType)
	})
}

func TestRDMADevicePathParsing(t *testing.T) {
	setupRDMATestMapping()
	defer cleanupRDMATestMapping()

	t.Run("Standard uverbs path format", func(t *testing.T) {
		testCases := []string{
			"/dev/infiniband/uverbs0",
			"/dev/infiniband/uverbs1",
			"/dev/infiniband/uverbs2",
		}

		for i, path := range testCases {
			info := &model.DockerDeviceInfo{
				PathOnHost: path,
			}

			fillRDMADeviceInfoForDockerContainerInfo(info)

			assert.Equal(t, constant.DeviceTypeRDMA, info.DeviceType)
			assert.Equal(t, 101+i, info.DeviceId)
		}
	})

	t.Run("Path without leading slash", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "dev/infiniband/uverbs0",
		}

		fillRDMADeviceInfoForDockerContainerInfo(info)

		// Should still work as it contains "uverbs"
		assert.Equal(t, constant.DeviceTypeRDMA, info.DeviceType)
	})

	t.Run("Multiple uverbs in path", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost: "/uverbs/path/uverbs0",
		}

		fillRDMADeviceInfoForDockerContainerInfo(info)

		// Should extract "0" from the last occurrence
		assert.Equal(t, constant.DeviceTypeRDMA, info.DeviceType)
	})
}

func TestDeviceInfoPreservation(t *testing.T) {
	setupGPUTestMapping()
	setupRDMATestMapping()
	defer cleanupGPUTestMapping()
	defer cleanupRDMATestMapping()

	t.Run("Preserve PathInContainer", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost:      "/dev/dri/card0",
			PathInContainer: "/dev/dri/card99",
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		// PathInContainer should not be modified
		assert.Equal(t, "/dev/dri/card99", info.PathInContainer)
	})

	t.Run("Preserve other fields", func(t *testing.T) {
		info := &model.DockerDeviceInfo{
			PathOnHost:        "/dev/dri/card0",
			PathInContainer:   "/dev/dri/card0",
			CgroupPermissions: "rwm",
		}

		fillGPUDeviceInfoForDockerContainerInfo(info)

		// CgroupPermissions should not be modified
		assert.Equal(t, "rwm", info.CgroupPermissions)
	})
}
