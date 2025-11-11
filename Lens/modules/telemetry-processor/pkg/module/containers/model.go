package containers

import "time"

// ContainerEventRequest represents the HTTP request for container events
type ContainerEventRequest struct {
	Type        string                 `json:"type" binding:"required"`         // Event type: snapshot, created, running, exit, deleted, etc.
	Source      string                 `json:"source" binding:"required"`       // Source: k8s, docker
	Node        string                 `json:"node" binding:"required"`         // Node name
	ContainerID string                 `json:"container_id" binding:"required"` // Container ID
	Data        map[string]interface{} `json:"data" binding:"required"`         // Container data
}

// K8sContainerData represents Kubernetes container information
type K8sContainerData struct {
	ID              string                 `json:"id"`
	PodName         string                 `json:"pod_name"`
	PodNamespace    string                 `json:"pod_namespace"`
	PodUUID         string                 `json:"pod_uuid"`
	Status          string                 `json:"status"`
	CreatedAt       int64                  `json:"created_at"`
	ExitCode        int32                  `json:"exit_code,omitempty"`
	ExitTime        time.Time              `json:"exit_time,omitempty"`
	OOMKilled       bool                   `json:"oom_killed,omitempty"`
	Devices         *ContainerDevices      `json:"devices,omitempty"`
	ContainerStatus map[string]interface{} `json:"container_status,omitempty"`
}

// DockerContainerData represents Docker container information
type DockerContainerData struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Status  string               `json:"status"`
	StartAt time.Time            `json:"start_at"`
	Devices []DockerDeviceInfo   `json:"devices,omitempty"`
}

// ContainerDevices represents devices assigned to a container
type ContainerDevices struct {
	GPU        []DeviceInfo `json:"gpu,omitempty"`
	Infiniband []DeviceInfo `json:"infiniband,omitempty"`
}

// DeviceInfo represents a single device
type DeviceInfo struct {
	Name   string `json:"name"`
	Id     int    `json:"id"`
	Path   string `json:"path"`
	Type   string `json:"type,omitempty"`
	Kind   string `json:"kind"`
	UUID   string `json:"uuid"`
	Serial string `json:"serial"`
	Slot   string `json:"slot,omitempty"`
}

// DockerDeviceInfo represents Docker device information
type DockerDeviceInfo struct {
	DeviceName   string `json:"device_name"`
	DeviceId     int    `json:"device_id"`
	DeviceSerial string `json:"device_serial"`
	DeviceType   string `json:"device_type,omitempty"`
}

// BatchContainerEventsRequest represents a batch of container events
type BatchContainerEventsRequest struct {
	Events []ContainerEventRequest `json:"events" binding:"required"`
}

