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
