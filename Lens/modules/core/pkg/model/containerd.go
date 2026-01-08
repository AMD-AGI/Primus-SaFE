// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import (
	runtimespec "github.com/opencontainers/runtime-spec/specs-go"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
	"time"
)

const (
	ContainerEventTypeSnapshot    = "snapshot"
	ContainerEventTypeCreate      = "create"
	ContainerEventTypeDelete      = "delete"
	ContainerEventTypeUpdate      = "update"
	ContainerEventTypeTaskCreate  = "task_create"
	ContainerEventTypeStart       = "task_start"
	ContainerEventTypeStop        = "task_stop"
	ContainerEventTypeTaskDeleted = "task_deleted"
	ContainerEventTypeRestart     = "restart"
	ContainerEventTypeDead        = "dead"
	ContainerEventTypeOOMKilled   = "oomKilled"
)

type ContainerEvent struct {
	Type string     `json:"type"` // "update" or "delete"
	Data *Container `json:"data"`
}

type PodInfo struct {
	PodName         string                `json:"pod_name"`
	PodUuid         string                `json:"pod_uuid"`
	PodNamespace    string                `json:"pod_namespace"`
	PodLabels       map[string]string     `json:"pod_labels"`
	PodAnnotations  map[string]string     `json:"pod_annotations"`
	ContainerdPodId string                `json:"containerd_pod_id"`
	NodeName        string                `json:"node_name"`
	WorkloadId      string                `json:"workload_id"`
	Type            string                `json:"type"`
	ComponentName   string                `json:"component_name"`
	Containers      map[string]*Container `json:"containers"`
	Source          string                `json:"source"`
}

type Container struct {
	runtimeapi.ContainerStatus
	PodName      string            `json:"pod_name"`
	PodNamespace string            `json:"pod_namespace"`
	PodUuid      string            `json:"pod_uuid"`
	Image        string            `json:"image"`
	Info         *ContainerInfo    `json:"info"`
	Devices      *ContainerDevices `json:"devices"`
	Status       string            `json:"status"`
	Pid          uint32            `json:"pid"`
	OOMKilled    bool              `json:"oom_killed"`
	ExitTime     time.Time         `json:"exit_time"`
	LastUpdated  time.Time         `json:"last_updated"`
}

func (c Container) HasGpu() bool {
	return c.Devices != nil && len(c.Devices.GPU) > 0
}

// ContainerInfo is extra information for a container.
type ContainerInfo struct {
	SandboxID      string                      `json:"sandboxID"`
	Pid            uint32                      `json:"pid"`
	Removing       bool                        `json:"removing"`
	SnapshotKey    string                      `json:"snapshotKey"`
	Snapshotter    string                      `json:"snapshotter"`
	RuntimeType    string                      `json:"runtimeType"`
	RuntimeOptions interface{}                 `json:"runtimeOptions"`
	Config         *runtimeapi.ContainerConfig `json:"config"`
	RuntimeSpec    *runtimespec.Spec           `json:"runtimeSpec"`
}

type ContainerDevices struct {
	GPU        []*DeviceInfo `json:"gpu"`
	Infiniband []*DeviceInfo `json:"infiniband"`
}

type DeviceInfo struct {
	Name   string `json:"name"`
	Id     int    `json:"id"`
	Path   string `json:"path"`
	Type   string `json:"type"`
	Kind   string `json:"kind"`
	UUID   string `json:"uuid"`
	Serial string `json:"serial"`
	Slot   string `json:"slot"`
}
