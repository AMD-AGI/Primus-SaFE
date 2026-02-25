// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

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
	// GetJobExecutionHistory returns the JobExecutionHistory Facade interface
	GetJobExecutionHistory() JobExecutionHistoryFacadeInterface
	// GetNamespaceInfo returns the NamespaceInfo Facade interface
	GetNamespaceInfo() NamespaceInfoFacadeInterface
	// GetWorkloadStatistic returns the WorkloadStatistic Facade interface
	GetWorkloadStatistic() WorkloadStatisticFacadeInterface
	// GetAiWorkloadMetadata returns the AiWorkloadMetadata Facade interface
	GetAiWorkloadMetadata() AiWorkloadMetadataFacadeInterface
	// GetCheckpointEvent returns the CheckpointEvent Facade interface
	GetCheckpointEvent() CheckpointEventFacadeInterface
	// GetDetectionConflictLog returns the DetectionConflictLog Facade interface
	GetDetectionConflictLog() DetectionConflictLogFacadeInterface
	// GetGpuUsageWeeklyReport returns the GpuUsageWeeklyReport Facade interface
	GetGpuUsageWeeklyReport() GpuUsageWeeklyReportFacadeInterface
	// GetNodeNamespaceMapping returns the NodeNamespaceMapping Facade interface
	GetNodeNamespaceMapping() NodeNamespaceMappingFacadeInterface
	// GetTraceLensSession returns the TraceLensSession Facade interface
	GetTraceLensSession() TraceLensSessionFacadeInterface
	// GetK8sService returns the K8sService Facade interface
	GetK8sService() K8sServiceFacadeInterface
	// GetWorkloadDetection returns the WorkloadDetection Facade interface
	GetWorkloadDetection() WorkloadDetectionFacadeInterface
	// GetWorkloadDetectionEvidence returns the WorkloadDetectionEvidence Facade interface
	GetWorkloadDetectionEvidence() WorkloadDetectionEvidenceFacadeInterface
	// GetDetectionCoverage returns the DetectionCoverage Facade interface
	GetDetectionCoverage() DetectionCoverageFacadeInterface
	// GetAIAgentRegistration returns the AIAgentRegistration Facade interface
	GetAIAgentRegistration() AIAgentRegistrationFacadeInterface
	// GetAITask returns the AITask Facade interface
	GetAITask() AITaskFacadeInterface
	// GetGithubWorkflowConfig returns the GithubWorkflowConfig Facade interface
	GetGithubWorkflowConfig() GithubWorkflowConfigFacadeInterface
	// GetGithubWorkflowRun returns the GithubWorkflowRun Facade interface
	GetGithubWorkflowRun() GithubWorkflowRunFacadeInterface
	// GetGithubWorkflowSchema returns the GithubWorkflowSchema Facade interface
	GetGithubWorkflowSchema() GithubWorkflowSchemaFacadeInterface
	// GetGithubWorkflowMetrics returns the GithubWorkflowMetrics Facade interface
	GetGithubWorkflowMetrics() GithubWorkflowMetricsFacadeInterface
	// GetGithubRunnerSet returns the GithubRunnerSet Facade interface
	GetGithubRunnerSet() GithubRunnerSetFacadeInterface
	// GetGithubWorkflowCommit returns the GithubWorkflowCommit Facade interface
	GetGithubWorkflowCommit() GithubWorkflowCommitFacadeInterface
	// GetGithubWorkflowRunDetails returns the GithubWorkflowRunDetails Facade interface
	GetGithubWorkflowRunDetails() GithubWorkflowRunDetailsFacadeInterface
	// GetGithubWorkflowRunSummary returns the GithubWorkflowRunSummary Facade interface
	GetGithubWorkflowRunSummary() *GithubWorkflowRunSummaryFacade
	// GetDashboardSummary returns the DashboardSummary Facade interface
	GetDashboardSummary() DashboardSummaryFacadeInterface
	// GetMetricBaseline returns the MetricBaseline Facade interface
	GetMetricBaseline() MetricBaselineFacadeInterface
	// GetCommitImpactAnalysis returns the CommitImpactAnalysis Facade interface
	GetCommitImpactAnalysis() CommitImpactAnalysisFacadeInterface
	// GetNotificationChannel returns the NotificationChannel Facade interface
	GetNotificationChannel() NotificationChannelFacadeInterface
	// GetPodRunningPeriods returns the PodRunningPeriods Facade interface
	GetPodRunningPeriods() PodRunningPeriodsFacadeInterface
	// GetWorkloadCodeSnapshot returns the WorkloadCodeSnapshot Facade interface
	GetWorkloadCodeSnapshot() WorkloadCodeSnapshotFacadeInterface
	// GetImageRegistryCache returns the ImageRegistryCache Facade interface
	GetImageRegistryCache() ImageRegistryCacheFacadeInterface
	// GetIntentRule returns the IntentRule Facade interface
	GetIntentRule() IntentRuleFacadeInterface
	// GetWorkloadResource returns the WorkloadResource Facade interface
	GetWorkloadResource() WorkloadResourceFacadeInterface
	// GetProfilerFile returns the ProfilerFile Facade interface
	GetProfilerFile() ProfilerFileFacadeInterface
	// WithCluster returns a new Facade instance using the specified cluster
	WithCluster(clusterName string) FacadeInterface
}

// Facade is the unified entry point for database operations, aggregating all sub-Facades
type Facade struct {
	Node                      NodeFacadeInterface
	Pod                       PodFacadeInterface
	Workload                  WorkloadFacadeInterface
	Container                 ContainerFacadeInterface
	Training                  TrainingFacadeInterface
	Storage                   StorageFacadeInterface
	Alert                     AlertFacadeInterface
	MetricAlertRule           MetricAlertRuleFacadeInterface
	LogAlertRule              LogAlertRuleFacadeInterface
	AlertRuleAdvice           AlertRuleAdviceFacadeInterface
	ClusterOverviewCache      ClusterOverviewCacheFacadeInterface
	GenericCache              GenericCacheFacadeInterface
	GpuAggregation            GpuAggregationFacadeInterface
	SystemConfig              SystemConfigFacadeInterface
	JobExecutionHistory       JobExecutionHistoryFacadeInterface
	NamespaceInfo             NamespaceInfoFacadeInterface
	AiWorkloadMetadata        AiWorkloadMetadataFacadeInterface
	CheckpointEvent           CheckpointEventFacadeInterface
	DetectionConflictLog      DetectionConflictLogFacadeInterface
	WorkloadStatistic         WorkloadStatisticFacadeInterface
	GpuUsageWeeklyReport      GpuUsageWeeklyReportFacadeInterface
	NodeNamespaceMapping      NodeNamespaceMappingFacadeInterface
	TraceLensSession          TraceLensSessionFacadeInterface
	K8sService                K8sServiceFacadeInterface
	WorkloadDetection         WorkloadDetectionFacadeInterface
	WorkloadDetectionEvidence WorkloadDetectionEvidenceFacadeInterface
	DetectionCoverage         DetectionCoverageFacadeInterface
	AIAgentRegistration       AIAgentRegistrationFacadeInterface
	AITask                    AITaskFacadeInterface
	GithubWorkflowConfig      GithubWorkflowConfigFacadeInterface
	GithubWorkflowRun         GithubWorkflowRunFacadeInterface
	GithubWorkflowSchema      GithubWorkflowSchemaFacadeInterface
	GithubWorkflowMetrics     GithubWorkflowMetricsFacadeInterface
	GithubRunnerSet           GithubRunnerSetFacadeInterface
	GithubWorkflowCommit      GithubWorkflowCommitFacadeInterface
	GithubWorkflowRunDetails  GithubWorkflowRunDetailsFacadeInterface
	GithubWorkflowRunSummary  *GithubWorkflowRunSummaryFacade
	DashboardSummary          DashboardSummaryFacadeInterface
	MetricBaseline            MetricBaselineFacadeInterface
	CommitImpactAnalysis      CommitImpactAnalysisFacadeInterface
	NotificationChannel       NotificationChannelFacadeInterface
	PodRunningPeriods         PodRunningPeriodsFacadeInterface
	WorkloadCodeSnapshot     WorkloadCodeSnapshotFacadeInterface
	ImageRegistryCache       ImageRegistryCacheFacadeInterface
	IntentRule               IntentRuleFacadeInterface
	WorkloadResource         WorkloadResourceFacadeInterface
	ProfilerFile             ProfilerFileFacadeInterface
}

// NewFacade creates a new Facade instance
func NewFacade() *Facade {
	return &Facade{
		Node:                      NewNodeFacade(),
		Pod:                       NewPodFacade(),
		Workload:                  NewWorkloadFacade(),
		Container:                 NewContainerFacade(),
		Training:                  NewTrainingFacade(),
		Storage:                   NewStorageFacade(),
		Alert:                     NewAlertFacade(),
		MetricAlertRule:           NewMetricAlertRuleFacade(),
		LogAlertRule:              NewLogAlertRuleFacade(),
		AlertRuleAdvice:           NewAlertRuleAdviceFacade(),
		ClusterOverviewCache:      NewClusterOverviewCacheFacade(),
		GenericCache:              NewGenericCacheFacade(),
		GpuAggregation:            NewGpuAggregationFacade(),
		SystemConfig:              NewSystemConfigFacade(),
		JobExecutionHistory:       NewJobExecutionHistoryFacade(),
		NamespaceInfo:             NewNamespaceInfoFacade(),
		AiWorkloadMetadata:        NewAiWorkloadMetadataFacade(),
		CheckpointEvent:           NewCheckpointEventFacade(),
		DetectionConflictLog:      NewDetectionConflictLogFacade(),
		WorkloadStatistic:         NewWorkloadStatisticFacade(),
		GpuUsageWeeklyReport:      NewGpuUsageWeeklyReportFacade(),
		NodeNamespaceMapping:      NewNodeNamespaceMappingFacade(),
		TraceLensSession:          NewTraceLensSessionFacade(),
		K8sService:                NewK8sServiceFacade(),
		WorkloadDetection:         NewWorkloadDetectionFacade(),
		WorkloadDetectionEvidence: NewWorkloadDetectionEvidenceFacade(),
		DetectionCoverage:         NewDetectionCoverageFacade(),
		AIAgentRegistration:       NewAIAgentRegistrationFacade(),
		AITask:                    NewAITaskFacade(),
		GithubWorkflowConfig:      NewGithubWorkflowConfigFacade(),
		GithubWorkflowRun:         NewGithubWorkflowRunFacade(),
		GithubWorkflowSchema:      NewGithubWorkflowSchemaFacade(),
		GithubWorkflowMetrics:     NewGithubWorkflowMetricsFacade(),
		GithubRunnerSet:           NewGithubRunnerSetFacade(),
		GithubWorkflowCommit:      NewGithubWorkflowCommitFacade(),
		GithubWorkflowRunDetails:  NewGithubWorkflowRunDetailsFacade(),
		GithubWorkflowRunSummary:  NewGithubWorkflowRunSummaryFacade(),
		DashboardSummary:          NewDashboardSummaryFacade(),
		MetricBaseline:            NewMetricBaselineFacade(),
		CommitImpactAnalysis:      NewCommitImpactAnalysisFacade(),
		NotificationChannel:       NewNotificationChannelFacade(),
		PodRunningPeriods:         NewPodRunningPeriodsFacade(),
		WorkloadCodeSnapshot:     NewWorkloadCodeSnapshotFacade(),
		ImageRegistryCache:       NewImageRegistryCacheFacade(),
		IntentRule:               NewIntentRuleFacade(),
		WorkloadResource:         NewWorkloadResourceFacade(),
		ProfilerFile:             NewProfilerFileFacade(),
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

// GetJobExecutionHistory returns the JobExecutionHistory Facade interface
func (f *Facade) GetJobExecutionHistory() JobExecutionHistoryFacadeInterface {
	return f.JobExecutionHistory
}

// GetNamespaceInfo returns the NamespaceInfo Facade interface
func (f *Facade) GetNamespaceInfo() NamespaceInfoFacadeInterface {
	return f.NamespaceInfo
}

// GetWorkloadStatistic returns the WorkloadStatistic Facade interface
func (f *Facade) GetWorkloadStatistic() WorkloadStatisticFacadeInterface {
	return f.WorkloadStatistic
}

// GetAiWorkloadMetadata returns the AiWorkloadMetadata Facade interface
func (f *Facade) GetAiWorkloadMetadata() AiWorkloadMetadataFacadeInterface {
	return f.AiWorkloadMetadata
}

// GetCheckpointEvent returns the CheckpointEvent Facade interface
func (f *Facade) GetCheckpointEvent() CheckpointEventFacadeInterface {
	return f.CheckpointEvent
}

// GetDetectionConflictLog returns the DetectionConflictLog Facade interface
func (f *Facade) GetDetectionConflictLog() DetectionConflictLogFacadeInterface {
	return f.DetectionConflictLog
}

// GetGpuUsageWeeklyReport returns the GpuUsageWeeklyReport Facade interface
func (f *Facade) GetGpuUsageWeeklyReport() GpuUsageWeeklyReportFacadeInterface {
	return f.GpuUsageWeeklyReport
}

// GetNodeNamespaceMapping returns the NodeNamespaceMapping Facade interface
func (f *Facade) GetNodeNamespaceMapping() NodeNamespaceMappingFacadeInterface {
	return f.NodeNamespaceMapping
}

// GetTraceLensSession returns the TraceLensSession Facade interface
func (f *Facade) GetTraceLensSession() TraceLensSessionFacadeInterface {
	return f.TraceLensSession
}

// GetK8sService returns the K8sService Facade interface
func (f *Facade) GetK8sService() K8sServiceFacadeInterface {
	return f.K8sService
}

// GetWorkloadDetection returns the WorkloadDetection Facade interface
func (f *Facade) GetWorkloadDetection() WorkloadDetectionFacadeInterface {
	return f.WorkloadDetection
}

// GetWorkloadDetectionEvidence returns the WorkloadDetectionEvidence Facade interface
func (f *Facade) GetWorkloadDetectionEvidence() WorkloadDetectionEvidenceFacadeInterface {
	return f.WorkloadDetectionEvidence
}

// GetDetectionCoverage returns the DetectionCoverage Facade interface
func (f *Facade) GetDetectionCoverage() DetectionCoverageFacadeInterface {
	return f.DetectionCoverage
}

// GetAIAgentRegistration returns the AIAgentRegistration Facade interface
func (f *Facade) GetAIAgentRegistration() AIAgentRegistrationFacadeInterface {
	return f.AIAgentRegistration
}

// GetAITask returns the AITask Facade interface
func (f *Facade) GetAITask() AITaskFacadeInterface {
	return f.AITask
}

// GetGithubWorkflowConfig returns the GithubWorkflowConfig Facade interface
func (f *Facade) GetGithubWorkflowConfig() GithubWorkflowConfigFacadeInterface {
	return f.GithubWorkflowConfig
}

// GetGithubWorkflowRun returns the GithubWorkflowRun Facade interface
func (f *Facade) GetGithubWorkflowRun() GithubWorkflowRunFacadeInterface {
	return f.GithubWorkflowRun
}

// GetGithubWorkflowSchema returns the GithubWorkflowSchema Facade interface
func (f *Facade) GetGithubWorkflowSchema() GithubWorkflowSchemaFacadeInterface {
	return f.GithubWorkflowSchema
}

// GetGithubWorkflowMetrics returns the GithubWorkflowMetrics Facade interface
func (f *Facade) GetGithubWorkflowMetrics() GithubWorkflowMetricsFacadeInterface {
	return f.GithubWorkflowMetrics
}

// GetGithubRunnerSet returns the GithubRunnerSet Facade interface
func (f *Facade) GetGithubRunnerSet() GithubRunnerSetFacadeInterface {
	return f.GithubRunnerSet
}

// GetGithubWorkflowCommit returns the GithubWorkflowCommit Facade interface
func (f *Facade) GetGithubWorkflowCommit() GithubWorkflowCommitFacadeInterface {
	return f.GithubWorkflowCommit
}

// GetGithubWorkflowRunDetails returns the GithubWorkflowRunDetails Facade interface
func (f *Facade) GetGithubWorkflowRunDetails() GithubWorkflowRunDetailsFacadeInterface {
	return f.GithubWorkflowRunDetails
}

// GetGithubWorkflowRunSummary returns the GithubWorkflowRunSummary Facade interface
func (f *Facade) GetGithubWorkflowRunSummary() *GithubWorkflowRunSummaryFacade {
	return f.GithubWorkflowRunSummary
}

// GetDashboardSummary returns the DashboardSummary Facade interface
func (f *Facade) GetDashboardSummary() DashboardSummaryFacadeInterface {
	return f.DashboardSummary
}

// GetMetricBaseline returns the MetricBaseline Facade interface
func (f *Facade) GetMetricBaseline() MetricBaselineFacadeInterface {
	return f.MetricBaseline
}

// GetCommitImpactAnalysis returns the CommitImpactAnalysis Facade interface
func (f *Facade) GetCommitImpactAnalysis() CommitImpactAnalysisFacadeInterface {
	return f.CommitImpactAnalysis
}

// GetNotificationChannel returns the NotificationChannel Facade interface
func (f *Facade) GetNotificationChannel() NotificationChannelFacadeInterface {
	return f.NotificationChannel
}

// GetPodRunningPeriods returns the PodRunningPeriods Facade interface
func (f *Facade) GetPodRunningPeriods() PodRunningPeriodsFacadeInterface {
	return f.PodRunningPeriods
}

// GetWorkloadCodeSnapshot returns the WorkloadCodeSnapshot Facade interface
func (f *Facade) GetWorkloadCodeSnapshot() WorkloadCodeSnapshotFacadeInterface {
	return f.WorkloadCodeSnapshot
}

// GetImageRegistryCache returns the ImageRegistryCache Facade interface
func (f *Facade) GetImageRegistryCache() ImageRegistryCacheFacadeInterface {
	return f.ImageRegistryCache
}

// GetIntentRule returns the IntentRule Facade interface
func (f *Facade) GetIntentRule() IntentRuleFacadeInterface {
	return f.IntentRule
}

// GetWorkloadResource returns the WorkloadResource Facade interface
func (f *Facade) GetWorkloadResource() WorkloadResourceFacadeInterface {
	return f.WorkloadResource
}

// GetProfilerFile returns the ProfilerFile Facade interface
func (f *Facade) GetProfilerFile() ProfilerFileFacadeInterface {
	return f.ProfilerFile
}

// WithCluster returns a new Facade instance, all sub-Facades use the specified cluster
func (f *Facade) WithCluster(clusterName string) FacadeInterface {
	return &Facade{
		Node:                      f.Node.WithCluster(clusterName),
		Pod:                       f.Pod.WithCluster(clusterName),
		Workload:                  f.Workload.WithCluster(clusterName),
		Container:                 f.Container.WithCluster(clusterName),
		Training:                  f.Training.WithCluster(clusterName),
		Storage:                   f.Storage.WithCluster(clusterName),
		Alert:                     f.Alert.WithCluster(clusterName),
		MetricAlertRule:           f.MetricAlertRule.WithCluster(clusterName),
		LogAlertRule:              f.LogAlertRule.WithCluster(clusterName),
		AlertRuleAdvice:           f.AlertRuleAdvice.WithCluster(clusterName),
		ClusterOverviewCache:      f.ClusterOverviewCache.WithCluster(clusterName),
		GenericCache:              f.GenericCache.WithCluster(clusterName),
		GpuAggregation:            f.GpuAggregation.WithCluster(clusterName),
		SystemConfig:              f.SystemConfig.WithCluster(clusterName),
		JobExecutionHistory:       f.JobExecutionHistory.WithCluster(clusterName),
		NamespaceInfo:             f.NamespaceInfo.WithCluster(clusterName),
		AiWorkloadMetadata:        f.AiWorkloadMetadata.WithCluster(clusterName),
		CheckpointEvent:           f.CheckpointEvent.WithCluster(clusterName),
		DetectionConflictLog:      f.DetectionConflictLog.WithCluster(clusterName),
		WorkloadStatistic:         f.WorkloadStatistic.WithCluster(clusterName),
		GpuUsageWeeklyReport:      f.GpuUsageWeeklyReport.WithCluster(clusterName),
		NodeNamespaceMapping:      f.NodeNamespaceMapping.WithCluster(clusterName),
		TraceLensSession:          f.TraceLensSession.WithCluster(clusterName),
		K8sService:                f.K8sService.WithCluster(clusterName),
		WorkloadDetection:         f.WorkloadDetection.WithCluster(clusterName),
		WorkloadDetectionEvidence: f.WorkloadDetectionEvidence.WithCluster(clusterName),
		DetectionCoverage:         f.DetectionCoverage.WithCluster(clusterName),
		AIAgentRegistration:       f.AIAgentRegistration.WithCluster(clusterName),
		AITask:                    f.AITask.WithCluster(clusterName),
		GithubWorkflowConfig:      f.GithubWorkflowConfig.WithCluster(clusterName),
		GithubWorkflowRun:         f.GithubWorkflowRun.WithCluster(clusterName),
		GithubWorkflowSchema:      f.GithubWorkflowSchema.WithCluster(clusterName),
		GithubWorkflowMetrics:     f.GithubWorkflowMetrics.WithCluster(clusterName),
		GithubRunnerSet:           f.GithubRunnerSet.WithCluster(clusterName),
		GithubWorkflowCommit:      f.GithubWorkflowCommit.WithCluster(clusterName),
		GithubWorkflowRunDetails:  f.GithubWorkflowRunDetails.WithCluster(clusterName),
		GithubWorkflowRunSummary:  f.GithubWorkflowRunSummary.WithCluster(clusterName),
		DashboardSummary:          f.DashboardSummary.WithCluster(clusterName),
		MetricBaseline:            f.MetricBaseline.WithCluster(clusterName),
		CommitImpactAnalysis:      f.CommitImpactAnalysis.WithCluster(clusterName),
		NotificationChannel:       f.NotificationChannel.WithCluster(clusterName),
		PodRunningPeriods:         f.PodRunningPeriods.WithCluster(clusterName),
		WorkloadCodeSnapshot:     f.WorkloadCodeSnapshot.WithCluster(clusterName),
		ImageRegistryCache:       f.ImageRegistryCache.WithCluster(clusterName),
		IntentRule:               f.IntentRule.WithCluster(clusterName),
		WorkloadResource:         f.WorkloadResource.WithCluster(clusterName),
		ProfilerFile:             f.ProfilerFile.WithCluster(clusterName),
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
