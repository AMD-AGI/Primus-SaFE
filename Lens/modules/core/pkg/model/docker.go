package model

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/constant"
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
