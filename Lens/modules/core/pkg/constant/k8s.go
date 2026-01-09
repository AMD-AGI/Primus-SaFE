// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package constant

const (
	ContainerdK8SContainerName = "io.kubernetes.container.name"
	ContainerdK8SPodName       = "io.kubernetes.pod.name"
	ContainerdK8SPodNamespace  = "io.kubernetes.pod.namespace"
	ContainerdK8SPodUid        = "io.kubernetes.pod.uid"
)

const (
	ContainerStatusCreated   = "Created"
	ContainerStatusRunning   = "Running"
	ContainerStatusExit      = "Exit"
	ContainerStatusDeleted   = "Deleted"
	ContainerStatusOOMKilled = "OOMKilled"
)

const (
	ContainerSourceK8S    = "k8s"
	ContainerSourceDocker = "docker"
)
