package database

// FacadeInterface defines the Facade interface for unit testing and mocking
type FacadeInterface interface {
	// GetNode returns the Node Facade interface
	GetNode() NodeFacadeInterface
	// GetPod returns the Pod Facade interface
	GetPod() PodFacadeInterface
	// GetWorkload returns the Workload Facade interface
	GetWorkload() WorkloadFacadeInterface
	// GetContainer returns the Container Facade interface
	GetContainer() ContainerFacadeInterface
	// GetTraining returns the Training Facade interface
	GetTraining() TrainingFacadeInterface
	// GetStorage returns the Storage Facade interface
	GetStorage() StorageFacadeInterface
	// WithCluster returns a new Facade instance using the specified cluster
	WithCluster(clusterName string) FacadeInterface
}

// Facade is the unified entry point for database operations, aggregating all sub-Facades
type Facade struct {
	Node      NodeFacadeInterface
	Pod       PodFacadeInterface
	Workload  WorkloadFacadeInterface
	Container ContainerFacadeInterface
	Training  TrainingFacadeInterface
	Storage   StorageFacadeInterface
}

// NewFacade creates a new Facade instance
func NewFacade() *Facade {
	return &Facade{
		Node:      NewNodeFacade(),
		Pod:       NewPodFacade(),
		Workload:  NewWorkloadFacade(),
		Container: NewContainerFacade(),
		Training:  NewTrainingFacade(),
		Storage:   NewStorageFacade(),
	}
}

// GetNode returns the Node Facade interface
func (f *Facade) GetNode() NodeFacadeInterface {
	return f.Node
}

// GetPod returns the Pod Facade interface
func (f *Facade) GetPod() PodFacadeInterface {
	return f.Pod
}

// GetWorkload returns the Workload Facade interface
func (f *Facade) GetWorkload() WorkloadFacadeInterface {
	return f.Workload
}

// GetContainer returns the Container Facade interface
func (f *Facade) GetContainer() ContainerFacadeInterface {
	return f.Container
}

// GetTraining returns the Training Facade interface
func (f *Facade) GetTraining() TrainingFacadeInterface {
	return f.Training
}

// GetStorage returns the Storage Facade interface
func (f *Facade) GetStorage() StorageFacadeInterface {
	return f.Storage
}

// WithCluster returns a new Facade instance, all sub-Facades use the specified cluster
func (f *Facade) WithCluster(clusterName string) FacadeInterface {
	return &Facade{
		Node:      f.Node.WithCluster(clusterName),
		Pod:       f.Pod.WithCluster(clusterName),
		Workload:  f.Workload.WithCluster(clusterName),
		Container: f.Container.WithCluster(clusterName),
		Training:  f.Training.WithCluster(clusterName),
		Storage:   f.Storage.WithCluster(clusterName),
	}
}

// Global default Facade instance
var defaultFacade = NewFacade()

// GetFacade returns the default Facade instance (using the current cluster)
func GetFacade() FacadeInterface {
	return defaultFacade
}

// GetFacadeForCluster returns a Facade instance for the specified cluster
func GetFacadeForCluster(clusterName string) FacadeInterface {
	return defaultFacade.WithCluster(clusterName)
}
