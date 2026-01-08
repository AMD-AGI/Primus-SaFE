// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package gpu_aggregation_backfill

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/statistics"
	"github.com/stretchr/testify/assert"
)

// ==================== Mock Implementations ====================

type MockBackfillFacade struct {
	nodeFacade           database.NodeFacadeInterface
	gpuAggregationFacade database.GpuAggregationFacadeInterface
}

func (m *MockBackfillFacade) GetNode() database.NodeFacadeInterface {
	return m.nodeFacade
}

func (m *MockBackfillFacade) GetGpuAggregation() database.GpuAggregationFacadeInterface {
	return m.gpuAggregationFacade
}

func (m *MockBackfillFacade) GetGenericCache() database.GenericCacheFacadeInterface           { return nil }
func (m *MockBackfillFacade) GetWorkload() database.WorkloadFacadeInterface                   { return nil }
func (m *MockBackfillFacade) GetPod() database.PodFacadeInterface                             { return nil }
func (m *MockBackfillFacade) GetContainer() database.ContainerFacadeInterface                 { return nil }
func (m *MockBackfillFacade) GetTraining() database.TrainingFacadeInterface                   { return nil }
func (m *MockBackfillFacade) GetStorage() database.StorageFacadeInterface                     { return nil }
func (m *MockBackfillFacade) GetAlert() database.AlertFacadeInterface                         { return nil }
func (m *MockBackfillFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface     { return nil }
func (m *MockBackfillFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface           { return nil }
func (m *MockBackfillFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface     { return nil }
func (m *MockBackfillFacade) GetClusterOverviewCache() database.ClusterOverviewCacheFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetSystemConfig() database.SystemConfigFacadeInterface               { return nil }
func (m *MockBackfillFacade) GetJobExecutionHistory() database.JobExecutionHistoryFacadeInterface { return nil }
func (m *MockBackfillFacade) GetNamespaceInfo() database.NamespaceInfoFacadeInterface             { return nil }
func (m *MockBackfillFacade) GetWorkloadStatistic() database.WorkloadStatisticFacadeInterface     { return nil }
func (m *MockBackfillFacade) GetAiWorkloadMetadata() database.AiWorkloadMetadataFacadeInterface   { return nil }
func (m *MockBackfillFacade) GetCheckpointEvent() database.CheckpointEventFacadeInterface         { return nil }
func (m *MockBackfillFacade) GetDetectionConflictLog() database.DetectionConflictLogFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetNodeNamespaceMapping() database.NodeNamespaceMappingFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetTraceLensSession() database.TraceLensSessionFacadeInterface { return nil }
func (m *MockBackfillFacade) GetK8sService() database.K8sServiceFacadeInterface             { return nil }
func (m *MockBackfillFacade) GetWorkloadDetection() database.WorkloadDetectionFacadeInterface { return nil }
func (m *MockBackfillFacade) GetWorkloadDetectionEvidence() database.WorkloadDetectionEvidenceFacadeInterface { return nil }
func (m *MockBackfillFacade) GetDetectionCoverage() database.DetectionCoverageFacadeInterface { return nil }
func (m *MockBackfillFacade) GetAIAgentRegistration() database.AIAgentRegistrationFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetAITask() database.AITaskFacadeInterface { return nil }
func (m *MockBackfillFacade) GetGithubWorkflowConfig() database.GithubWorkflowConfigFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetGithubWorkflowRun() database.GithubWorkflowRunFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetGithubWorkflowSchema() database.GithubWorkflowSchemaFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetGithubWorkflowMetrics() database.GithubWorkflowMetricsFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetGithubRunnerSet() database.GithubRunnerSetFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetGithubWorkflowCommit() database.GithubWorkflowCommitFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) GetGithubWorkflowRunDetails() database.GithubWorkflowRunDetailsFacadeInterface {
	return nil
}
func (m *MockBackfillFacade) WithCluster(clusterName string) database.FacadeInterface { return m }

type MockBackfillNodeFacade struct {
	SearchNodeFunc func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error)
}

func (m *MockBackfillNodeFacade) SearchNode(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
	if m.SearchNodeFunc != nil {
		return m.SearchNodeFunc(ctx, f)
	}
	return nil, 0, nil
}

func (m *MockBackfillNodeFacade) WithCluster(clusterName string) database.NodeFacadeInterface {
	return m
}
func (m *MockBackfillNodeFacade) GetNodeByName(ctx context.Context, name string) (*dbmodel.Node, error) {
	return nil, nil
}
func (m *MockBackfillNodeFacade) CreateNode(ctx context.Context, node *dbmodel.Node) error { return nil }
func (m *MockBackfillNodeFacade) UpdateNode(ctx context.Context, node *dbmodel.Node) error { return nil }
func (m *MockBackfillNodeFacade) ListGpuNodes(ctx context.Context) ([]*dbmodel.Node, error) {
	return nil, nil
}
func (m *MockBackfillNodeFacade) GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*dbmodel.GpuDevice, error) {
	return nil, nil
}
func (m *MockBackfillNodeFacade) CreateGpuDevice(ctx context.Context, device *dbmodel.GpuDevice) error {
	return nil
}
func (m *MockBackfillNodeFacade) UpdateGpuDevice(ctx context.Context, device *dbmodel.GpuDevice) error {
	return nil
}
func (m *MockBackfillNodeFacade) ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*dbmodel.GpuDevice, error) {
	return nil, nil
}
func (m *MockBackfillNodeFacade) DeleteGpuDeviceById(ctx context.Context, id int32) error { return nil }
func (m *MockBackfillNodeFacade) GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*dbmodel.RdmaDevice, error) {
	return nil, nil
}
func (m *MockBackfillNodeFacade) CreateRdmaDevice(ctx context.Context, rdmaDevice *dbmodel.RdmaDevice) error {
	return nil
}
func (m *MockBackfillNodeFacade) ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*dbmodel.RdmaDevice, error) {
	return nil, nil
}
func (m *MockBackfillNodeFacade) DeleteRdmaDeviceById(ctx context.Context, id int32) error { return nil }
func (m *MockBackfillNodeFacade) CreateNodeDeviceChangelog(ctx context.Context, changelog *dbmodel.NodeDeviceChangelog) error {
	return nil
}

type MockBackfillGpuAggregationFacade struct {
	SaveClusterHourlyStatsFunc func(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error
	savedStats                 []*dbmodel.ClusterGpuHourlyStats
}

func (m *MockBackfillGpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
	if m.SaveClusterHourlyStatsFunc != nil {
		return m.SaveClusterHourlyStatsFunc(ctx, stats)
	}
	m.savedStats = append(m.savedStats, stats)
	return nil
}

func (m *MockBackfillGpuAggregationFacade) WithCluster(clusterName string) database.GpuAggregationFacadeInterface {
	return m
}
func (m *MockBackfillGpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListNamespaceHourlyStatsPaginatedWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) LabelHourlyStatsExists(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error) {
	return false, nil
}
func (m *MockBackfillGpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetWorkloadHourlyStatsPaginatedWithExclusion(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error {
	return nil
}
func (m *MockBackfillGpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}
func (m *MockBackfillGpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}
func (m *MockBackfillGpuAggregationFacade) GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetDistinctNamespacesWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string) ([]string, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}
func (m *MockBackfillGpuAggregationFacade) GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}

type MockBackfillAllocationCalculator struct {
	CalculateHourlyGpuAllocationFunc func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error)
}

func (m *MockBackfillAllocationCalculator) CalculateHourlyGpuAllocation(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
	if m.CalculateHourlyGpuAllocationFunc != nil {
		return m.CalculateHourlyGpuAllocationFunc(ctx, hour)
	}
	return &statistics.GpuAllocationResult{}, nil
}

// ==================== Test Cases ====================

func TestBuildClusterStatsFromResult_WithFullUtilization(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	result := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 48.5,
		WorkloadCount:     12,
	}

	stats := BuildClusterStatsFromResult("test-cluster", hour, result)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, 48.5, stats.AllocatedGpuCount)
	assert.Equal(t, int32(12), stats.SampleCount)

	// Utilization fields should be zero initially (will be set later)
	assert.Equal(t, 0.0, stats.AvgUtilization)
	assert.Equal(t, 0.0, stats.MaxUtilization)
	assert.Equal(t, 0.0, stats.MinUtilization)
	assert.Equal(t, 0.0, stats.P50Utilization)
	assert.Equal(t, 0.0, stats.P95Utilization)
}

func TestCreateZeroClusterStats_AllFieldsZero(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	stats := CreateZeroClusterStats("test-cluster", hour)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, int32(0), stats.TotalGpuCapacity)
	assert.Equal(t, 0.0, stats.AllocatedGpuCount)
	assert.Equal(t, 0.0, stats.AllocationRate)

	// All utilization fields should be zero
	assert.Equal(t, 0.0, stats.AvgUtilization)
	assert.Equal(t, 0.0, stats.MaxUtilization)
	assert.Equal(t, 0.0, stats.MinUtilization)
	assert.Equal(t, 0.0, stats.P50Utilization)
	assert.Equal(t, 0.0, stats.P95Utilization)
	assert.Equal(t, int32(0), stats.SampleCount)
}

func TestBackfillWithFullUtilizationStats(t *testing.T) {
	mockNodeFacade := &MockBackfillNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
			return []*dbmodel.Node{
				{Name: "node-1", GpuCount: 8},
			}, 1, nil
		},
	}

	savedStats := make([]*dbmodel.ClusterGpuHourlyStats, 0)
	mockGpuAggregationFacade := &MockBackfillGpuAggregationFacade{
		SaveClusterHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockFacade := &MockBackfillFacade{
		nodeFacade:           mockNodeFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
	}

	mockAllocationCalc := &MockBackfillAllocationCalculator{
		CalculateHourlyGpuAllocationFunc: func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{
				TotalAllocatedGpu: 6.0,
				WorkloadCount:     3,
			}, nil
		},
	}

	mockUtilizationQuery := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
		return &statistics.ClusterGpuUtilizationStats{
			AvgUtilization: 72.5,
			MaxUtilization: 95.3,
			MinUtilization: 45.2,
			P50Utilization: 70.8,
			P95Utilization: 93.1,
		}, nil
	}

	job := NewClusterGpuAggregationBackfillJob(
		WithClusterBackfillFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterBackfillAllocationCalculatorFactory(func(clusterName string) ClusterBackfillAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithClusterBackfillUtilizationQueryFunc(mockUtilizationQuery),
		WithClusterBackfillClusterName("test-cluster"),
	)

	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	_, err := job.backfillClusterStats(context.Background(), "test-cluster", []time.Time{hour}, nil)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(savedStats))

	// Verify allocation stats
	assert.Equal(t, 6.0, savedStats[0].AllocatedGpuCount)
	assert.Equal(t, int32(8), savedStats[0].TotalGpuCapacity)

	// Verify all utilization statistics are properly set
	assert.Equal(t, 72.5, savedStats[0].AvgUtilization)
	assert.Equal(t, 95.3, savedStats[0].MaxUtilization)
	assert.Equal(t, 45.2, savedStats[0].MinUtilization)
	assert.Equal(t, 70.8, savedStats[0].P50Utilization)
	assert.Equal(t, 93.1, savedStats[0].P95Utilization)
}

func TestNewClusterGpuAggregationBackfillJob_Default(t *testing.T) {
	job := NewClusterGpuAggregationBackfillJob(
		WithClusterBackfillClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.NotNil(t, job.config)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, "test-cluster", job.clusterName)
	assert.Equal(t, DefaultClusterBackfillDays, job.config.BackfillDays)
	assert.Equal(t, DefaultClusterBatchSize, job.config.BatchSize)
}

func TestWithClusterBackfillUtilizationQueryFunc(t *testing.T) {
	called := false
	mockFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
		called = true
		return &statistics.ClusterGpuUtilizationStats{
			AvgUtilization: 80.0,
			MaxUtilization: 95.0,
			MinUtilization: 60.0,
			P50Utilization: 78.0,
			P95Utilization: 92.0,
		}, nil
	}

	job := &ClusterGpuAggregationBackfillJob{}
	opt := WithClusterBackfillUtilizationQueryFunc(mockFunc)
	opt(job)

	result, err := job.utilizationQueryFunc(context.Background(), nil, time.Now())
	assert.True(t, called)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 80.0, result.AvgUtilization)
	assert.Equal(t, 95.0, result.MaxUtilization)
	assert.Equal(t, 60.0, result.MinUtilization)
	assert.Equal(t, 78.0, result.P50Utilization)
	assert.Equal(t, 92.0, result.P95Utilization)
}

