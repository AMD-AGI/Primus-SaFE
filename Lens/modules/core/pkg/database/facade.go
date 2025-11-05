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
	// GetAlert returns the Alert Facade interface
	GetAlert() AlertFacadeInterface
	// GetMetricAlertRule returns the MetricAlertRule Facade interface
	GetMetricAlertRule() MetricAlertRuleFacadeInterface
	// GetLogAlertRule returns the LogAlertRule Facade interface
	GetLogAlertRule() LogAlertRuleFacadeInterface
	// GetAlertRuleAdvice returns the AlertRuleAdvice Facade interface
	GetAlertRuleAdvice() AlertRuleAdviceFacadeInterface
	// GetClusterOverviewCache returns the ClusterOverviewCache Facade interface
	GetClusterOverviewCache() ClusterOverviewCacheFacadeInterface
	// GetGenericCache returns the GenericCache Facade interface
	GetGenericCache() GenericCacheFacadeInterface
	// GetGpuAggregation returns the GpuAggregation Facade interface
	GetGpuAggregation() GpuAggregationFacadeInterface
	// GetSystemConfig returns the SystemConfig Facade interface
	GetSystemConfig() SystemConfigFacadeInterface
	// WithCluster returns a new Facade instance using the specified cluster
	WithCluster(clusterName string) FacadeInterface
}

// Facade is the unified entry point for database operations, aggregating all sub-Facades
type Facade struct {
	Node                 NodeFacadeInterface
	Pod                  PodFacadeInterface
	Workload             WorkloadFacadeInterface
	Container            ContainerFacadeInterface
	Training             TrainingFacadeInterface
	Storage              StorageFacadeInterface
	Alert                AlertFacadeInterface
	MetricAlertRule      MetricAlertRuleFacadeInterface
	LogAlertRule         LogAlertRuleFacadeInterface
	AlertRuleAdvice      AlertRuleAdviceFacadeInterface
	ClusterOverviewCache ClusterOverviewCacheFacadeInterface
	GenericCache         GenericCacheFacadeInterface
	GpuAggregation       GpuAggregationFacadeInterface
	SystemConfig         SystemConfigFacadeInterface
}

// NewFacade creates a new Facade instance
func NewFacade() *Facade {
	return &Facade{
		Node:                 NewNodeFacade(),
		Pod:                  NewPodFacade(),
		Workload:             NewWorkloadFacade(),
		Container:            NewContainerFacade(),
		Training:             NewTrainingFacade(),
		Storage:              NewStorageFacade(),
		Alert:                NewAlertFacade(),
		MetricAlertRule:      NewMetricAlertRuleFacade(),
		LogAlertRule:         NewLogAlertRuleFacade(),
		AlertRuleAdvice:      NewAlertRuleAdviceFacade(),
		ClusterOverviewCache: NewClusterOverviewCacheFacade(),
		GenericCache:         NewGenericCacheFacade(),
		GpuAggregation:       NewGpuAggregationFacade(),
		SystemConfig:         NewSystemConfigFacade(),
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

// GetAlert returns the Alert Facade interface
func (f *Facade) GetAlert() AlertFacadeInterface {
	return f.Alert
}

// GetMetricAlertRule returns the MetricAlertRule Facade interface
func (f *Facade) GetMetricAlertRule() MetricAlertRuleFacadeInterface {
	return f.MetricAlertRule
}

// GetLogAlertRule returns the LogAlertRule Facade interface
func (f *Facade) GetLogAlertRule() LogAlertRuleFacadeInterface {
	return f.LogAlertRule
}

// GetAlertRuleAdvice returns the AlertRuleAdvice Facade interface
func (f *Facade) GetAlertRuleAdvice() AlertRuleAdviceFacadeInterface {
	return f.AlertRuleAdvice
}

// GetClusterOverviewCache returns the ClusterOverviewCache Facade interface
func (f *Facade) GetClusterOverviewCache() ClusterOverviewCacheFacadeInterface {
	return f.ClusterOverviewCache
}

// GetGenericCache returns the GenericCache Facade interface
func (f *Facade) GetGenericCache() GenericCacheFacadeInterface {
	return f.GenericCache
}

// GetGpuAggregation returns the GpuAggregation Facade interface
func (f *Facade) GetGpuAggregation() GpuAggregationFacadeInterface {
	return f.GpuAggregation
}

// GetSystemConfig returns the SystemConfig Facade interface
func (f *Facade) GetSystemConfig() SystemConfigFacadeInterface {
	return f.SystemConfig
}

// WithCluster returns a new Facade instance, all sub-Facades use the specified cluster
func (f *Facade) WithCluster(clusterName string) FacadeInterface {
	return &Facade{
		Node:                 f.Node.WithCluster(clusterName),
		Pod:                  f.Pod.WithCluster(clusterName),
		Workload:             f.Workload.WithCluster(clusterName),
		Container:            f.Container.WithCluster(clusterName),
		Training:             f.Training.WithCluster(clusterName),
		Storage:              f.Storage.WithCluster(clusterName),
		Alert:                f.Alert.WithCluster(clusterName),
		MetricAlertRule:      f.MetricAlertRule.WithCluster(clusterName),
		LogAlertRule:         f.LogAlertRule.WithCluster(clusterName),
		AlertRuleAdvice:      f.AlertRuleAdvice.WithCluster(clusterName),
		ClusterOverviewCache: f.ClusterOverviewCache.WithCluster(clusterName),
		GenericCache:         f.GenericCache.WithCluster(clusterName),
		GpuAggregation:       f.GpuAggregation.WithCluster(clusterName),
		SystemConfig:         f.SystemConfig.WithCluster(clusterName),
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
