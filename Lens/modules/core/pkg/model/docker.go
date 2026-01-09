// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"time"
)

type DockerDeviceInfo struct {
	PathOnHost        string
	PathInContainer   string
	CgroupPermissions string
	DeviceType        string
	DeviceSerial      string
	DeviceName        string
	DeviceId          int
	Kind              string
	Slot              string
}

type DockerMountInfo struct {
	Type        string
	Source      string
	Destination string
}

type DockerContainerInfo struct {
	ID      string
	Name    string
	Labels  map[string]string
	Cmd     string
	Devices []DockerDeviceInfo
	Mounts  []DockerMountInfo
	StartAt time.Time
	Status  string
}

func (info DockerContainerInfo) GpuDeviceCount() int {
	count := 0
	for _, dev := range info.Devices {
		if dev.DeviceType == constant.DeviceTypeGPU {
			count++
		}
	}
	return count
}
