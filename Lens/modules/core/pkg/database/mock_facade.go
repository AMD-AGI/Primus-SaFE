// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package database

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// MockFacade is a mock implementation of FacadeInterface for testing.
// Facade mocks for tests live here; when adding or changing a facade interface,
// update only these mocks. Downstream modules should use NewMockFacade() and
// inject sub-facades (Node, Pod, Workload, etc.) from this package.
type MockFacade struct {
	Node                     NodeFacadeInterface
	Pod                      PodFacadeInterface
	Workload                 WorkloadFacadeInterface
	GpuUsageWeeklyReportMock GpuUsageWeeklyReportFacadeInterface
	GpuAggregationMock       GpuAggregationFacadeInterface
	NamespaceInfo            NamespaceInfoFacadeInterface
	SystemConfig             SystemConfigFacadeInterface
	GenericCache             GenericCacheFacadeInterface
}

// NewMockFacade creates a new MockFacade with default mock implementations
func NewMockFacade() *MockFacade {
	return &MockFacade{
		GpuUsageWeeklyReportMock: NewMockGpuUsageWeeklyReportFacade(),
		GpuAggregationMock:       NewMockGpuAggregationFacade(),
	}
}

// Implement FacadeInterface methods
func (m *MockFacade) GetNode() NodeFacadeInterface     { return m.Node }
func (m *MockFacade) GetPod() PodFacadeInterface       { return m.Pod }
func (m *MockFacade) GetWorkload() WorkloadFacadeInterface { return m.Workload }
func (m *MockFacade) GetContainer() ContainerFacadeInterface                             { return nil }
func (m *MockFacade) GetTraining() TrainingFacadeInterface                               { return nil }
func (m *MockFacade) GetStorage() StorageFacadeInterface                                 { return nil }
func (m *MockFacade) GetAlert() AlertFacadeInterface                                     { return nil }
func (m *MockFacade) GetMetricAlertRule() MetricAlertRuleFacadeInterface                 { return nil }
func (m *MockFacade) GetLogAlertRule() LogAlertRuleFacadeInterface                       { return nil }
func (m *MockFacade) GetAlertRuleAdvice() AlertRuleAdviceFacadeInterface                 { return nil }
func (m *MockFacade) GetClusterOverviewCache() ClusterOverviewCacheFacadeInterface       { return nil }
func (m *MockFacade) GetGenericCache() GenericCacheFacadeInterface             { return m.GenericCache }
func (m *MockFacade) GetSystemConfig() SystemConfigFacadeInterface             { return m.SystemConfig }
func (m *MockFacade) GetJobExecutionHistory() JobExecutionHistoryFacadeInterface { return nil }
func (m *MockFacade) GetNamespaceInfo() NamespaceInfoFacadeInterface           { return m.NamespaceInfo }
func (m *MockFacade) GetWorkloadStatistic() WorkloadStatisticFacadeInterface             { return nil }
func (m *MockFacade) GetAiWorkloadMetadata() AiWorkloadMetadataFacadeInterface           { return nil }
func (m *MockFacade) GetCheckpointEvent() CheckpointEventFacadeInterface                 { return nil }
func (m *MockFacade) GetDetectionConflictLog() DetectionConflictLogFacadeInterface       { return nil }
func (m *MockFacade) GetNodeNamespaceMapping() NodeNamespaceMappingFacadeInterface       { return nil }
func (m *MockFacade) GetTraceLensSession() TraceLensSessionFacadeInterface               { return nil }
func (m *MockFacade) GetK8sService() K8sServiceFacadeInterface                           { return nil }
func (m *MockFacade) GetWorkloadDetection() WorkloadDetectionFacadeInterface             { return nil }
func (m *MockFacade) GetWorkloadDetectionEvidence() WorkloadDetectionEvidenceFacadeInterface {
	return nil
}
func (m *MockFacade) GetDetectionCoverage() DetectionCoverageFacadeInterface     { return nil }
func (m *MockFacade) GetAIAgentRegistration() AIAgentRegistrationFacadeInterface { return nil }
func (m *MockFacade) GetAITask() AITaskFacadeInterface                           { return nil }
func (m *MockFacade) GetGithubWorkflowConfig() GithubWorkflowConfigFacadeInterface {
	return nil
}
func (m *MockFacade) GetGithubWorkflowRun() GithubWorkflowRunFacadeInterface { return nil }
func (m *MockFacade) GetGithubWorkflowSchema() GithubWorkflowSchemaFacadeInterface {
	return nil
}
func (m *MockFacade) GetGithubWorkflowMetrics() GithubWorkflowMetricsFacadeInterface { return nil }
func (m *MockFacade) GetGithubRunnerSet() GithubRunnerSetFacadeInterface             { return nil }
func (m *MockFacade) GetGithubWorkflowCommit() GithubWorkflowCommitFacadeInterface   { return nil }
func (m *MockFacade) GetGithubWorkflowRunDetails() GithubWorkflowRunDetailsFacadeInterface {
	return nil
}
func (m *MockFacade) GetDashboardSummary() DashboardSummaryFacadeInterface { return nil }
func (m *MockFacade) GetMetricBaseline() MetricBaselineFacadeInterface     { return nil }
func (m *MockFacade) GetCommitImpactAnalysis() CommitImpactAnalysisFacadeInterface {
	return nil
}
func (m *MockFacade) GetNotificationChannel() NotificationChannelFacadeInterface { return nil }
func (m *MockFacade) GetPodRunningPeriods() PodRunningPeriodsFacadeInterface         { return nil }
func (m *MockFacade) GetWorkloadCodeSnapshot() WorkloadCodeSnapshotFacadeInterface  { return nil }
func (m *MockFacade) GetImageRegistryCache() ImageRegistryCacheFacadeInterface      { return nil }
func (m *MockFacade) GetIntentRule() IntentRuleFacadeInterface                      { return nil }
func (m *MockFacade) GetWorkloadResource() WorkloadResourceFacadeInterface          { return nil }
func (m *MockFacade) GetProfilerFile() ProfilerFileFacadeInterface                  { return nil }

func (m *MockFacade) GetGpuUsageWeeklyReport() GpuUsageWeeklyReportFacadeInterface {
	return m.GpuUsageWeeklyReportMock
}

func (m *MockFacade) GetGpuAggregation() GpuAggregationFacadeInterface {
	return m.GpuAggregationMock
}

func (m *MockFacade) WithCluster(clusterName string) FacadeInterface {
	return m
}

func (m *MockFacade) GetGithubWorkflowRunSummary() *GithubWorkflowRunSummaryFacade {
	return nil
}

// MockGpuUsageWeeklyReportFacade is a mock implementation for testing
type MockGpuUsageWeeklyReportFacade struct {
	// Store mock data
	Reports map[string]*model.GpuUsageWeeklyReports

	// Configurable callbacks for custom behavior
	OnCreate            func(ctx context.Context, report *model.GpuUsageWeeklyReports) error
	OnGetByID           func(ctx context.Context, id string) (*model.GpuUsageWeeklyReports, error)
	OnUpdate            func(ctx context.Context, report *model.GpuUsageWeeklyReports) error
	OnList              func(ctx context.Context, clusterName string, status string, pageNum, pageSize int) ([]*model.GpuUsageWeeklyReports, int64, error)
	OnGetLatestByCluster func(ctx context.Context, clusterName string) (*model.GpuUsageWeeklyReports, error)
	OnDeleteOlderThan   func(ctx context.Context, before time.Time) (int64, error)
	OnUpdateStatus      func(ctx context.Context, id string, status string) error
}

// NewMockGpuUsageWeeklyReportFacade creates a new mock facade
func NewMockGpuUsageWeeklyReportFacade() *MockGpuUsageWeeklyReportFacade {
	return &MockGpuUsageWeeklyReportFacade{
		Reports: make(map[string]*model.GpuUsageWeeklyReports),
	}
}

func (m *MockGpuUsageWeeklyReportFacade) Create(ctx context.Context, report *model.GpuUsageWeeklyReports) error {
	if m.OnCreate != nil {
		return m.OnCreate(ctx, report)
	}
	m.Reports[report.ID] = report
	return nil
}

func (m *MockGpuUsageWeeklyReportFacade) GetByID(ctx context.Context, id string) (*model.GpuUsageWeeklyReports, error) {
	if m.OnGetByID != nil {
		return m.OnGetByID(ctx, id)
	}
	return m.Reports[id], nil
}

func (m *MockGpuUsageWeeklyReportFacade) Update(ctx context.Context, report *model.GpuUsageWeeklyReports) error {
	if m.OnUpdate != nil {
		return m.OnUpdate(ctx, report)
	}
	m.Reports[report.ID] = report
	return nil
}

func (m *MockGpuUsageWeeklyReportFacade) List(ctx context.Context, clusterName string, status string, pageNum, pageSize int) ([]*model.GpuUsageWeeklyReports, int64, error) {
	if m.OnList != nil {
		return m.OnList(ctx, clusterName, status, pageNum, pageSize)
	}
	var reports []*model.GpuUsageWeeklyReports
	for _, r := range m.Reports {
		if clusterName == "" || r.ClusterName == clusterName {
			if status == "" || r.Status == status {
				reports = append(reports, r)
			}
		}
	}
	return reports, int64(len(reports)), nil
}

func (m *MockGpuUsageWeeklyReportFacade) GetLatestByCluster(ctx context.Context, clusterName string) (*model.GpuUsageWeeklyReports, error) {
	if m.OnGetLatestByCluster != nil {
		return m.OnGetLatestByCluster(ctx, clusterName)
	}
	var latest *model.GpuUsageWeeklyReports
	for _, r := range m.Reports {
		if r.ClusterName == clusterName {
			if latest == nil || r.GeneratedAt.After(latest.GeneratedAt) {
				latest = r
			}
		}
	}
	return latest, nil
}

func (m *MockGpuUsageWeeklyReportFacade) DeleteOlderThan(ctx context.Context, before time.Time) (int64, error) {
	if m.OnDeleteOlderThan != nil {
		return m.OnDeleteOlderThan(ctx, before)
	}
	var deleted int64
	for id, r := range m.Reports {
		if r.GeneratedAt.Before(before) {
			delete(m.Reports, id)
			deleted++
		}
	}
	return deleted, nil
}

func (m *MockGpuUsageWeeklyReportFacade) UpdateStatus(ctx context.Context, id string, status string) error {
	if m.OnUpdateStatus != nil {
		return m.OnUpdateStatus(ctx, id, status)
	}
	if r, ok := m.Reports[id]; ok {
		r.Status = status
	}
	return nil
}

func (m *MockGpuUsageWeeklyReportFacade) WithCluster(clusterName string) GpuUsageWeeklyReportFacadeInterface {
	return m
}

// MockGpuAggregationFacade is a mock implementation for testing
type MockGpuAggregationFacade struct {
	// Store mock data
	ClusterHourlyStats   []*model.ClusterGpuHourlyStats
	NamespaceHourlyStats []*model.NamespaceGpuHourlyStats

	// Configurable callbacks (used by jobs tests)
	OnGetClusterHourlyStats    func(ctx context.Context, startTime, endTime time.Time) ([]*model.ClusterGpuHourlyStats, error)
	SaveClusterHourlyStatsFunc func(ctx context.Context, stats *model.ClusterGpuHourlyStats) error
	ListNamespaceHourlyStatsFunc func(ctx context.Context, startTime, endTime time.Time) ([]*model.NamespaceGpuHourlyStats, error)
	SaveNamespaceHourlyStatsFunc func(ctx context.Context, stats *model.NamespaceGpuHourlyStats) error
	SaveWorkloadHourlyStatsFunc func(ctx context.Context, stats *model.WorkloadGpuHourlyStats) error
	ListWorkloadHourlyStatsFunc func(ctx context.Context, startTime, endTime time.Time) ([]*model.WorkloadGpuHourlyStats, error)
}

// NewMockGpuAggregationFacade creates a new mock facade
func NewMockGpuAggregationFacade() *MockGpuAggregationFacade {
	return &MockGpuAggregationFacade{
		ClusterHourlyStats:   make([]*model.ClusterGpuHourlyStats, 0),
		NamespaceHourlyStats: make([]*model.NamespaceGpuHourlyStats, 0),
	}
}

func (m *MockGpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *model.ClusterGpuHourlyStats) error {
	if m.SaveClusterHourlyStatsFunc != nil {
		return m.SaveClusterHourlyStatsFunc(ctx, stats)
	}
	m.ClusterHourlyStats = append(m.ClusterHourlyStats, stats)
	return nil
}

func (m *MockGpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*model.ClusterGpuHourlyStats) error {
	m.ClusterHourlyStats = append(m.ClusterHourlyStats, stats...)
	return nil
}

func (m *MockGpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*model.ClusterGpuHourlyStats, error) {
	if m.OnGetClusterHourlyStats != nil {
		return m.OnGetClusterHourlyStats(ctx, startTime, endTime)
	}
	var result []*model.ClusterGpuHourlyStats
	for _, s := range m.ClusterHourlyStats {
		if (s.StatHour.Equal(startTime) || s.StatHour.After(startTime)) &&
			(s.StatHour.Equal(endTime) || s.StatHour.Before(endTime)) {
			result = append(result, s)
		}
	}
	return result, nil
}

func (m *MockGpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *model.NamespaceGpuHourlyStats) error {
	if m.SaveNamespaceHourlyStatsFunc != nil {
		return m.SaveNamespaceHourlyStatsFunc(ctx, stats)
	}
	return nil
}

func (m *MockGpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*model.NamespaceGpuHourlyStats) error {
	return nil
}

func (m *MockGpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*model.NamespaceGpuHourlyStats, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*model.NamespaceGpuHourlyStats, error) {
	if m.ListNamespaceHourlyStatsFunc != nil {
		return m.ListNamespaceHourlyStatsFunc(ctx, startTime, endTime)
	}
	return nil, nil
}

func (m *MockGpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListNamespaceHourlyStatsPaginatedWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *model.LabelGpuHourlyStats) error {
	return nil
}

func (m *MockGpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*model.LabelGpuHourlyStats) error {
	return nil
}

func (m *MockGpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*model.LabelGpuHourlyStats, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*model.LabelGpuHourlyStats, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) LabelHourlyStatsExists(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error) {
	return false, nil
}

func (m *MockGpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *model.WorkloadGpuHourlyStats) error {
	if m.SaveWorkloadHourlyStatsFunc != nil {
		return m.SaveWorkloadHourlyStatsFunc(ctx, stats)
	}
	return nil
}

func (m *MockGpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*model.WorkloadGpuHourlyStats) error {
	return nil
}

func (m *MockGpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*model.WorkloadGpuHourlyStats, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*model.WorkloadGpuHourlyStats, error) {
	if m.ListWorkloadHourlyStatsFunc != nil {
		return m.ListWorkloadHourlyStatsFunc(ctx, startTime, endTime)
	}
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*model.WorkloadGpuHourlyStats, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) GetWorkloadHourlyStatsPaginatedWithExclusion(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, excludeNamespaces []string, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts PaginationOptions) (*PaginatedResult, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *model.GpuAllocationSnapshots) error {
	return nil
}

func (m *MockGpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*model.GpuAllocationSnapshots, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*model.GpuAllocationSnapshots, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}

func (m *MockGpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}

func (m *MockGpuAggregationFacade) GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) GetDistinctNamespacesWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string) ([]string, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}

func (m *MockGpuAggregationFacade) WithCluster(clusterName string) GpuAggregationFacadeInterface {
	return m
}

