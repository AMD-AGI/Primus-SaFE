// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package gpu_aggregation

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/statistics"
	"github.com/stretchr/testify/assert"
)

// ==================== Mock Implementations ====================

// NamespaceMockFacade implements database.FacadeInterface for testing
type NamespaceMockFacade struct {
	namespaceInfoFacade  database.NamespaceInfoFacadeInterface
	gpuAggregationFacade database.GpuAggregationFacadeInterface
	genericCacheFacade   database.GenericCacheFacadeInterface
}

func (m *NamespaceMockFacade) GetNamespaceInfo() database.NamespaceInfoFacadeInterface {
	return m.namespaceInfoFacade
}

func (m *NamespaceMockFacade) GetGpuAggregation() database.GpuAggregationFacadeInterface {
	return m.gpuAggregationFacade
}

func (m *NamespaceMockFacade) GetGenericCache() database.GenericCacheFacadeInterface {
	return m.genericCacheFacade
}

// Implement other methods with nil returns (not used in tests)
func (m *NamespaceMockFacade) GetWorkload() database.WorkloadFacadeInterface               { return nil }
func (m *NamespaceMockFacade) GetPod() database.PodFacadeInterface                         { return nil }
func (m *NamespaceMockFacade) GetNode() database.NodeFacadeInterface                       { return nil }
func (m *NamespaceMockFacade) GetContainer() database.ContainerFacadeInterface             { return nil }
func (m *NamespaceMockFacade) GetTraining() database.TrainingFacadeInterface               { return nil }
func (m *NamespaceMockFacade) GetStorage() database.StorageFacadeInterface                 { return nil }
func (m *NamespaceMockFacade) GetAlert() database.AlertFacadeInterface                     { return nil }
func (m *NamespaceMockFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface       { return nil }
func (m *NamespaceMockFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetClusterOverviewCache() database.ClusterOverviewCacheFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetSystemConfig() database.SystemConfigFacadeInterface               { return nil }
func (m *NamespaceMockFacade) GetJobExecutionHistory() database.JobExecutionHistoryFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetWorkloadStatistic() database.WorkloadStatisticFacadeInterface     { return nil }
func (m *NamespaceMockFacade) GetAiWorkloadMetadata() database.AiWorkloadMetadataFacadeInterface   { return nil }
func (m *NamespaceMockFacade) GetCheckpointEvent() database.CheckpointEventFacadeInterface         { return nil }
func (m *NamespaceMockFacade) GetDetectionConflictLog() database.DetectionConflictLogFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetNodeNamespaceMapping() database.NodeNamespaceMappingFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetTraceLensSession() database.TraceLensSessionFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetK8sService() database.K8sServiceFacadeInterface             { return nil }
func (m *NamespaceMockFacade) GetWorkloadDetection() database.WorkloadDetectionFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetWorkloadDetectionEvidence() database.WorkloadDetectionEvidenceFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetDetectionCoverage() database.DetectionCoverageFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetAIAgentRegistration() database.AIAgentRegistrationFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetAITask() database.AITaskFacadeInterface { return nil }
func (m *NamespaceMockFacade) GetGithubWorkflowConfig() database.GithubWorkflowConfigFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetGithubWorkflowRun() database.GithubWorkflowRunFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetGithubWorkflowSchema() database.GithubWorkflowSchemaFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetGithubWorkflowMetrics() database.GithubWorkflowMetricsFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetGithubRunnerSet() database.GithubRunnerSetFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetGithubWorkflowCommit() database.GithubWorkflowCommitFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) GetGithubWorkflowRunDetails() database.GithubWorkflowRunDetailsFacadeInterface {
	return nil
}
func (m *NamespaceMockFacade) WithCluster(clusterName string) database.FacadeInterface { return m }

// NamespaceMockNamespaceInfoFacade implements database.NamespaceInfoFacadeInterface for testing
type NamespaceMockNamespaceInfoFacade struct {
	ListFunc func(ctx context.Context) ([]*dbmodel.NamespaceInfo, error)
}

func (m *NamespaceMockNamespaceInfoFacade) List(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx)
	}
	return nil, nil
}

func (m *NamespaceMockNamespaceInfoFacade) WithCluster(clusterName string) database.NamespaceInfoFacadeInterface {
	return m
}
func (m *NamespaceMockNamespaceInfoFacade) GetByName(ctx context.Context, name string) (*dbmodel.NamespaceInfo, error) {
	return nil, nil
}
func (m *NamespaceMockNamespaceInfoFacade) GetByNameIncludingDeleted(ctx context.Context, name string) (*dbmodel.NamespaceInfo, error) {
	return nil, nil
}
func (m *NamespaceMockNamespaceInfoFacade) Create(ctx context.Context, info *dbmodel.NamespaceInfo) error {
	return nil
}
func (m *NamespaceMockNamespaceInfoFacade) Update(ctx context.Context, info *dbmodel.NamespaceInfo) error {
	return nil
}
func (m *NamespaceMockNamespaceInfoFacade) DeleteByName(ctx context.Context, name string) error {
	return nil
}
func (m *NamespaceMockNamespaceInfoFacade) ListAllIncludingDeleted(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
	return nil, nil
}
func (m *NamespaceMockNamespaceInfoFacade) Recover(ctx context.Context, name string, gpuModel string, gpuResource int32) error {
	return nil
}

// NamespaceMockGpuAggregationFacade implements database.GpuAggregationFacadeInterface for testing
type NamespaceMockGpuAggregationFacade struct {
	SaveNamespaceHourlyStatsFunc func(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error
	savedStats                   []*dbmodel.NamespaceGpuHourlyStats
}

func (m *NamespaceMockGpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
	if m.SaveNamespaceHourlyStatsFunc != nil {
		return m.SaveNamespaceHourlyStatsFunc(ctx, stats)
	}
	m.savedStats = append(m.savedStats, stats)
	return nil
}

// Implement other required methods
func (m *NamespaceMockGpuAggregationFacade) WithCluster(clusterName string) database.GpuAggregationFacadeInterface {
	return m
}
func (m *NamespaceMockGpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginatedWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) LabelHourlyStatsExists(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error) {
	return false, nil
}
func (m *NamespaceMockGpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginatedWithExclusion(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error {
	return nil
}
func (m *NamespaceMockGpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}
func (m *NamespaceMockGpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetDistinctNamespacesWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string) ([]string, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}
func (m *NamespaceMockGpuAggregationFacade) GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}

// NamespaceMockGenericCacheFacade implements database.GenericCacheFacadeInterface for testing
type NamespaceMockGenericCacheFacade struct {
	GetFunc func(ctx context.Context, key string, value interface{}) error
	SetFunc func(ctx context.Context, key string, value interface{}, expiration *time.Time) error
}

func (m *NamespaceMockGenericCacheFacade) Get(ctx context.Context, key string, value interface{}) error {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, key, value)
	}
	return nil
}

func (m *NamespaceMockGenericCacheFacade) Set(ctx context.Context, key string, value interface{}, expiration *time.Time) error {
	if m.SetFunc != nil {
		return m.SetFunc(ctx, key, value, expiration)
	}
	return nil
}

func (m *NamespaceMockGenericCacheFacade) WithCluster(clusterName string) database.GenericCacheFacadeInterface {
	return m
}
func (m *NamespaceMockGenericCacheFacade) Delete(ctx context.Context, key string) error       { return nil }
func (m *NamespaceMockGenericCacheFacade) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (m *NamespaceMockGenericCacheFacade) DeleteExpired(ctx context.Context) error            { return nil }

// MockAllocationCalculator implements AllocationCalculatorInterface for testing
type MockAllocationCalculator struct {
	CalculateHourlyNamespaceGpuAllocationFunc func(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error)
}

func (m *MockAllocationCalculator) CalculateHourlyNamespaceGpuAllocation(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error) {
	if m.CalculateHourlyNamespaceGpuAllocationFunc != nil {
		return m.CalculateHourlyNamespaceGpuAllocationFunc(ctx, namespace, hour)
	}
	return &statistics.GpuAllocationResult{}, nil
}

// MockUtilizationCalculator implements UtilizationCalculatorInterface for testing
type MockUtilizationCalculator struct {
	CalculateHourlyNamespaceUtilizationFunc func(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult
}

func (m *MockUtilizationCalculator) CalculateHourlyNamespaceUtilization(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult {
	if m.CalculateHourlyNamespaceUtilizationFunc != nil {
		return m.CalculateHourlyNamespaceUtilizationFunc(ctx, namespace, allocationResult, hour)
	}
	return &statistics.NamespaceUtilizationResult{}
}

// ==================== Test Cases ====================

func TestNewNamespaceGpuAggregationJob_Default(t *testing.T) {
	job := NewNamespaceGpuAggregationJob(
		WithNamespaceClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.NotNil(t, job.config)
	assert.True(t, job.config.Enabled)
	assert.False(t, job.config.IncludeSystemNamespaces)
	assert.Equal(t, []string{}, job.config.ExcludeNamespaces)
	assert.Equal(t, "test-cluster", job.clusterName)
}

func TestNewNamespaceGpuAggregationJob_WithOptions(t *testing.T) {
	mockFacadeGetter := func(clusterName string) database.FacadeInterface {
		return &NamespaceMockFacade{}
	}

	job := NewNamespaceGpuAggregationJob(
		WithNamespaceFacadeGetter(mockFacadeGetter),
		WithNamespaceClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "test-cluster", job.clusterName)
	assert.NotNil(t, job.facadeGetter)
}

func TestNewNamespaceGpuAggregationJobWithConfig_WithOptions(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"excluded-ns"},
		IncludeSystemNamespaces: true,
	}

	job := NewNamespaceGpuAggregationJobWithConfig(config,
		WithNamespaceClusterName("custom-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "custom-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.True(t, job.config.IncludeSystemNamespaces)
	assert.Contains(t, job.config.ExcludeNamespaces, "excluded-ns")
}

func TestNamespaceGpuAggregationJob_Run_Disabled(t *testing.T) {
	job := NewNamespaceGpuAggregationJob(
		WithNamespaceClusterName("test-cluster"),
	)
	job.config.Enabled = false

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "Namespace GPU aggregation job is disabled")
}

func TestNamespaceGpuAggregationJob_GetConfig_SetConfig(t *testing.T) {
	job := NewNamespaceGpuAggregationJob(WithNamespaceClusterName("test"))

	config := job.GetConfig()
	assert.NotNil(t, config)
	assert.True(t, config.Enabled)

	newConfig := &NamespaceGpuAggregationConfig{
		Enabled:                 false,
		ExcludeNamespaces:       []string{"ns1", "ns2"},
		IncludeSystemNamespaces: true,
	}
	job.SetConfig(newConfig)

	assert.False(t, job.GetConfig().Enabled)
	assert.True(t, job.GetConfig().IncludeSystemNamespaces)
	assert.Equal(t, 2, len(job.GetConfig().ExcludeNamespaces))
}

func TestNamespaceGpuAggregationJob_ScheduleReturnsEvery5m(t *testing.T) {
	job := NewNamespaceGpuAggregationJob(WithNamespaceClusterName("test"))
	assert.Equal(t, "@every 5m", job.Schedule())
}

// ==================== Tests for Exported Helper Functions ====================

func TestShouldExcludeNamespace(t *testing.T) {
	tests := []struct {
		name                    string
		namespace               string
		excludeList             []string
		includeSystemNamespaces bool
		expected                bool
	}{
		{
			name:                    "normal namespace not excluded",
			namespace:               "my-app",
			excludeList:             []string{},
			includeSystemNamespaces: false,
			expected:                false,
		},
		{
			name:                    "namespace in exclusion list",
			namespace:               "excluded-ns",
			excludeList:             []string{"excluded-ns", "another-excluded"},
			includeSystemNamespaces: false,
			expected:                true,
		},
		{
			name:                    "system namespace excluded by default",
			namespace:               "kube-system",
			excludeList:             []string{},
			includeSystemNamespaces: false,
			expected:                true,
		},
		{
			name:                    "kube-public excluded by default",
			namespace:               "kube-public",
			excludeList:             []string{},
			includeSystemNamespaces: false,
			expected:                true,
		},
		{
			name:                    "kube-node-lease excluded by default",
			namespace:               "kube-node-lease",
			excludeList:             []string{},
			includeSystemNamespaces: false,
			expected:                true,
		},
		{
			name:                    "system namespace included when flag is true",
			namespace:               "kube-system",
			excludeList:             []string{},
			includeSystemNamespaces: true,
			expected:                false,
		},
		{
			name:                    "system namespace can still be in exclusion list",
			namespace:               "kube-system",
			excludeList:             []string{"kube-system"},
			includeSystemNamespaces: true,
			expected:                true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldExcludeNamespace(tt.namespace, tt.excludeList, tt.includeSystemNamespaces)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildNamespaceGpuHourlyStats(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 8.0,
		WorkloadCount:     5,
		PodCount:          10,
	}

	utilizationResult := &statistics.NamespaceUtilizationResult{
		AvgUtilization: 75.0,
		MinUtilization: 50.0,
		MaxUtilization: 95.0,
	}

	stats := BuildNamespaceGpuHourlyStats("test-cluster", "test-ns", hour, allocationResult, utilizationResult, 16)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, "test-ns", stats.Namespace)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, 8.0, stats.AllocatedGpuCount)
	assert.Equal(t, int32(5), stats.ActiveWorkloadCount)
	assert.Equal(t, int32(16), stats.TotalGpuCapacity)
	assert.Equal(t, 50.0, stats.AllocationRate) // (8/16) * 100 = 50%
	assert.Equal(t, 75.0, stats.AvgUtilization)
	assert.Equal(t, 50.0, stats.MinUtilization)
	assert.Equal(t, 95.0, stats.MaxUtilization)
}

func TestBuildNamespaceGpuHourlyStats_NoQuota(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 4.0,
		WorkloadCount:     2,
	}

	stats := BuildNamespaceGpuHourlyStats("test-cluster", "test-ns", hour, allocationResult, nil, 0)

	assert.Equal(t, int32(0), stats.TotalGpuCapacity)
	assert.Equal(t, float64(0), stats.AllocationRate)
	assert.Equal(t, float64(0), stats.AvgUtilization)
}

func TestBuildNamespaceGpuHourlyStats_NilUtilizationResult(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 4.0,
	}

	stats := BuildNamespaceGpuHourlyStats("test-cluster", "test-ns", hour, allocationResult, nil, 8)

	assert.Equal(t, float64(0), stats.AvgUtilization)
	assert.Equal(t, float64(0), stats.MinUtilization)
	assert.Equal(t, float64(0), stats.MaxUtilization)
}

// ==================== Option Functions Tests ====================

func TestWithNamespaceFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterName string) database.FacadeInterface {
		called = true
		return &NamespaceMockFacade{}
	}

	job := &NamespaceGpuAggregationJob{}
	opt := WithNamespaceFacadeGetter(mockGetter)
	opt(job)

	job.facadeGetter("test")
	assert.True(t, called)
}

func TestWithNamespaceClusterNameGetter(t *testing.T) {
	mockGetter := func() string {
		return "mock-cluster"
	}

	job := &NamespaceGpuAggregationJob{}
	opt := WithNamespaceClusterNameGetter(mockGetter)
	opt(job)

	assert.Equal(t, "mock-cluster", job.clusterNameGetter())
}

func TestWithNamespaceClusterName(t *testing.T) {
	job := &NamespaceGpuAggregationJob{}
	opt := WithNamespaceClusterName("my-cluster")
	opt(job)

	assert.Equal(t, "my-cluster", job.clusterName)
}

func TestWithAllocationCalculatorFactory(t *testing.T) {
	called := false
	mockFactory := func(clusterName string) AllocationCalculatorInterface {
		called = true
		return &MockAllocationCalculator{}
	}

	job := &NamespaceGpuAggregationJob{}
	opt := WithAllocationCalculatorFactory(mockFactory)
	opt(job)

	job.allocationCalculatorFactory("test")
	assert.True(t, called)
}

func TestWithUtilizationCalculatorFactory(t *testing.T) {
	called := false
	mockFactory := func(clusterName string, storageClientSet *clientsets.StorageClientSet) UtilizationCalculatorInterface {
		called = true
		return &MockUtilizationCalculator{}
	}

	job := &NamespaceGpuAggregationJob{}
	opt := WithUtilizationCalculatorFactory(mockFactory)
	opt(job)

	job.utilizationCalculatorFactory("test", nil)
	assert.True(t, called)
}

// ==================== Constants Tests ====================

func TestNamespaceConstants(t *testing.T) {
	assert.Equal(t, "job.namespace_gpu_aggregation.last_processed_hour", CacheKeyNamespaceGpuAggregationLastHour)
}

func TestSystemNamespaces(t *testing.T) {
	assert.Contains(t, SystemNamespaces, "kube-system")
	assert.Contains(t, SystemNamespaces, "kube-public")
	assert.Contains(t, SystemNamespaces, "kube-node-lease")
	assert.Equal(t, 3, len(SystemNamespaces))
}

// ==================== Config Tests ====================

func TestNamespaceGpuAggregationConfig_Defaults(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{}

	assert.False(t, config.Enabled)
	assert.False(t, config.IncludeSystemNamespaces)
	assert.Nil(t, config.ExcludeNamespaces)
}

func TestNamespaceGpuAggregationConfig_WithValues(t *testing.T) {
	config := &NamespaceGpuAggregationConfig{
		Enabled:                 true,
		ExcludeNamespaces:       []string{"ns1", "ns2"},
		IncludeSystemNamespaces: true,
	}

	assert.True(t, config.Enabled)
	assert.True(t, config.IncludeSystemNamespaces)
	assert.Equal(t, 2, len(config.ExcludeNamespaces))
}

// ==================== Integration Tests with Dependency Injection ====================

func TestNamespaceGpuAggregationJob_Run_NoNamespaces(t *testing.T) {
	mockCacheFacade := &NamespaceMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			return errors.New("no cache entry")
		},
	}

	mockNamespaceInfoFacade := &NamespaceMockNamespaceInfoFacade{
		ListFunc: func(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
			return []*dbmodel.NamespaceInfo{}, nil
		},
	}

	mockFacade := &NamespaceMockFacade{
		namespaceInfoFacade: mockNamespaceInfoFacade,
		genericCacheFacade:  mockCacheFacade,
	}

	job := NewNamespaceGpuAggregationJob(
		WithNamespaceFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithNamespaceClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
}

func TestNamespaceGpuAggregationJob_Run_SuccessfulAggregation(t *testing.T) {
	now := time.Now()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := previousHour.Add(-time.Hour)

	mockCacheFacade := &NamespaceMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			if v, ok := value.(*string); ok {
				*v = twoHoursAgo.Format(time.RFC3339)
			}
			return nil
		},
		SetFunc: func(ctx context.Context, key string, value interface{}, expiration *time.Time) error {
			return nil
		},
	}

	mockNamespaceInfoFacade := &NamespaceMockNamespaceInfoFacade{
		ListFunc: func(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
			return []*dbmodel.NamespaceInfo{
				{Name: "ns1", GpuResource: 8},
				{Name: "ns2", GpuResource: 16},
			}, nil
		},
	}

	savedStats := make([]*dbmodel.NamespaceGpuHourlyStats, 0)
	mockGpuAggregationFacade := &NamespaceMockGpuAggregationFacade{
		SaveNamespaceHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockFacade := &NamespaceMockFacade{
		namespaceInfoFacade:  mockNamespaceInfoFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
		genericCacheFacade:   mockCacheFacade,
	}

	mockAllocationCalc := &MockAllocationCalculator{
		CalculateHourlyNamespaceGpuAllocationFunc: func(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{
				TotalAllocatedGpu: 4.0,
				WorkloadCount:     2,
			}, nil
		},
	}

	mockUtilizationCalc := &MockUtilizationCalculator{
		CalculateHourlyNamespaceUtilizationFunc: func(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult {
			return &statistics.NamespaceUtilizationResult{
				AvgUtilization: 50.0,
				MinUtilization: 30.0,
				MaxUtilization: 70.0,
			}
		},
	}

	job := NewNamespaceGpuAggregationJob(
		WithNamespaceFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithAllocationCalculatorFactory(func(clusterName string) AllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithUtilizationCalculatorFactory(func(clusterName string, storageClientSet *clientsets.StorageClientSet) UtilizationCalculatorInterface {
			return mockUtilizationCalc
		}),
		WithNamespaceClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(2), stats.ItemsCreated)
	assert.Equal(t, 2, len(savedStats))
}

func TestNamespaceGpuAggregationJob_Run_GetNamespaceInfoError(t *testing.T) {
	mockCacheFacade := &NamespaceMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			return errors.New("no cache entry")
		},
	}

	mockNamespaceInfoFacade := &NamespaceMockNamespaceInfoFacade{
		ListFunc: func(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
			return nil, errors.New("database error")
		},
	}

	mockFacade := &NamespaceMockFacade{
		namespaceInfoFacade: mockNamespaceInfoFacade,
		genericCacheFacade:  mockCacheFacade,
	}

	job := NewNamespaceGpuAggregationJob(
		WithNamespaceFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithNamespaceClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ErrorCount)
}

func TestNamespaceGpuAggregationJob_Run_SystemNamespacesExcluded(t *testing.T) {
	now := time.Now()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := previousHour.Add(-time.Hour)

	mockCacheFacade := &NamespaceMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			if v, ok := value.(*string); ok {
				*v = twoHoursAgo.Format(time.RFC3339)
			}
			return nil
		},
		SetFunc: func(ctx context.Context, key string, value interface{}, expiration *time.Time) error {
			return nil
		},
	}

	mockNamespaceInfoFacade := &NamespaceMockNamespaceInfoFacade{
		ListFunc: func(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
			return []*dbmodel.NamespaceInfo{
				{Name: "kube-system", GpuResource: 0},
				{Name: "kube-public", GpuResource: 0},
				{Name: "my-app", GpuResource: 8},
			}, nil
		},
	}

	savedStats := make([]*dbmodel.NamespaceGpuHourlyStats, 0)
	mockGpuAggregationFacade := &NamespaceMockGpuAggregationFacade{
		SaveNamespaceHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockFacade := &NamespaceMockFacade{
		namespaceInfoFacade:  mockNamespaceInfoFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
		genericCacheFacade:   mockCacheFacade,
	}

	job := NewNamespaceGpuAggregationJob(
		WithNamespaceFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithAllocationCalculatorFactory(func(clusterName string) AllocationCalculatorInterface {
			return &MockAllocationCalculator{}
		}),
		WithUtilizationCalculatorFactory(func(clusterName string, storageClientSet *clientsets.StorageClientSet) UtilizationCalculatorInterface {
			return &MockUtilizationCalculator{}
		}),
		WithNamespaceClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ItemsCreated) // Only my-app should be processed
	assert.Equal(t, 1, len(savedStats))
	assert.Equal(t, "my-app", savedStats[0].Namespace)
}

// ==================== Edge Case Tests ====================

func TestShouldExcludeNamespace_EmptyExcludeList(t *testing.T) {
	result := ShouldExcludeNamespace("my-namespace", []string{}, false)
	assert.False(t, result)
}

func TestShouldExcludeNamespace_NilExcludeList(t *testing.T) {
	result := ShouldExcludeNamespace("my-namespace", nil, false)
	assert.False(t, result)
}

func TestBuildNamespaceGpuHourlyStats_ZeroAllocation(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 0,
		WorkloadCount:     0,
	}

	stats := BuildNamespaceGpuHourlyStats("test-cluster", "test-ns", hour, allocationResult, nil, 8)

	assert.Equal(t, float64(0), stats.AllocatedGpuCount)
	assert.Equal(t, float64(0), stats.AllocationRate)
}

func TestNamespaceGpuAggregationJob_NilConfig(t *testing.T) {
	job := &NamespaceGpuAggregationJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

// ==================== Time Range Tests ====================

func TestNamespaceTimeRangeForAggregation(t *testing.T) {
	now := time.Now()
	currentHour := now.Truncate(time.Hour)
	previousHour := currentHour.Add(-time.Hour)

	assert.True(t, previousHour.Before(currentHour))
	assert.Equal(t, time.Hour, currentHour.Sub(previousHour))
}

func TestNamespaceHourTruncation(t *testing.T) {
	input := time.Date(2025, 1, 1, 10, 30, 45, 123, time.UTC)
	expected := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	result := input.Truncate(time.Hour)
	assert.Equal(t, expected, result)
}

// ==================== Allocation Rate Tests ====================

func TestAllocationRateCalculation(t *testing.T) {
	tests := []struct {
		name          string
		allocated     float64
		quota         int32
		expectedRate  float64
	}{
		{
			name:         "full allocation",
			allocated:    8.0,
			quota:        8,
			expectedRate: 100.0,
		},
		{
			name:         "half allocation",
			allocated:    4.0,
			quota:        8,
			expectedRate: 50.0,
		},
		{
			name:         "no allocation",
			allocated:    0,
			quota:        8,
			expectedRate: 0,
		},
		{
			name:         "over allocation",
			allocated:    16.0,
			quota:        8,
			expectedRate: 200.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
			allocationResult := &statistics.GpuAllocationResult{
				TotalAllocatedGpu: tt.allocated,
			}

			stats := BuildNamespaceGpuHourlyStats("test-cluster", "test-ns", hour, allocationResult, nil, tt.quota)

			assert.InDelta(t, tt.expectedRate, stats.AllocationRate, 0.001)
		})
	}
}

