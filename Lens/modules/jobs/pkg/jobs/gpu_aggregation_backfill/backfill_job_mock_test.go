package gpu_aggregation_backfill

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/statistics"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// ==================== Mock Implementations ====================

// BackfillMockFacade implements database.FacadeInterface for testing
type BackfillMockFacade struct {
	nodeFacade           database.NodeFacadeInterface
	gpuAggregationFacade database.GpuAggregationFacadeInterface
	namespaceInfoFacade  database.NamespaceInfoFacadeInterface
	systemConfigFacade   database.SystemConfigFacadeInterface
}

func (m *BackfillMockFacade) GetNode() database.NodeFacadeInterface              { return m.nodeFacade }
func (m *BackfillMockFacade) GetGpuAggregation() database.GpuAggregationFacadeInterface { return m.gpuAggregationFacade }
func (m *BackfillMockFacade) GetNamespaceInfo() database.NamespaceInfoFacadeInterface   { return m.namespaceInfoFacade }
func (m *BackfillMockFacade) GetSystemConfig() database.SystemConfigFacadeInterface     { return m.systemConfigFacade }
func (m *BackfillMockFacade) GetWorkload() database.WorkloadFacadeInterface               { return nil }
func (m *BackfillMockFacade) GetPod() database.PodFacadeInterface                         { return nil }
func (m *BackfillMockFacade) GetContainer() database.ContainerFacadeInterface             { return nil }
func (m *BackfillMockFacade) GetTraining() database.TrainingFacadeInterface               { return nil }
func (m *BackfillMockFacade) GetStorage() database.StorageFacadeInterface                 { return nil }
func (m *BackfillMockFacade) GetAlert() database.AlertFacadeInterface                     { return nil }
func (m *BackfillMockFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface { return nil }
func (m *BackfillMockFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface       { return nil }
func (m *BackfillMockFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface { return nil }
func (m *BackfillMockFacade) GetClusterOverviewCache() database.ClusterOverviewCacheFacadeInterface { return nil }
func (m *BackfillMockFacade) GetGenericCache() database.GenericCacheFacadeInterface               { return nil }
func (m *BackfillMockFacade) GetJobExecutionHistory() database.JobExecutionHistoryFacadeInterface { return nil }
func (m *BackfillMockFacade) GetWorkloadStatistic() database.WorkloadStatisticFacadeInterface     { return nil }
func (m *BackfillMockFacade) GetAiWorkloadMetadata() database.AiWorkloadMetadataFacadeInterface   { return nil }
func (m *BackfillMockFacade) GetCheckpointEvent() database.CheckpointEventFacadeInterface         { return nil }
func (m *BackfillMockFacade) GetDetectionConflictLog() database.DetectionConflictLogFacadeInterface { return nil }
func (m *BackfillMockFacade) GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface { return nil }
func (m *BackfillMockFacade) GetNodeNamespaceMapping() database.NodeNamespaceMappingFacadeInterface { return nil }
func (m *BackfillMockFacade) WithCluster(clusterName string) database.FacadeInterface { return m }

// BackfillMockNodeFacade implements database.NodeFacadeInterface
type BackfillMockNodeFacade struct {
	SearchNodeFunc func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error)
}

func (m *BackfillMockNodeFacade) SearchNode(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
	if m.SearchNodeFunc != nil {
		return m.SearchNodeFunc(ctx, f)
	}
	return nil, 0, nil
}
func (m *BackfillMockNodeFacade) WithCluster(clusterName string) database.NodeFacadeInterface { return m }
func (m *BackfillMockNodeFacade) GetNodeByName(ctx context.Context, name string) (*dbmodel.Node, error) { return nil, nil }
func (m *BackfillMockNodeFacade) CreateNode(ctx context.Context, node *dbmodel.Node) error { return nil }
func (m *BackfillMockNodeFacade) UpdateNode(ctx context.Context, node *dbmodel.Node) error { return nil }
func (m *BackfillMockNodeFacade) ListGpuNodes(ctx context.Context) ([]*dbmodel.Node, error) { return nil, nil }
func (m *BackfillMockNodeFacade) GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*dbmodel.GpuDevice, error) { return nil, nil }
func (m *BackfillMockNodeFacade) CreateGpuDevice(ctx context.Context, device *dbmodel.GpuDevice) error { return nil }
func (m *BackfillMockNodeFacade) UpdateGpuDevice(ctx context.Context, device *dbmodel.GpuDevice) error { return nil }
func (m *BackfillMockNodeFacade) ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*dbmodel.GpuDevice, error) { return nil, nil }
func (m *BackfillMockNodeFacade) DeleteGpuDeviceById(ctx context.Context, id int32) error { return nil }
func (m *BackfillMockNodeFacade) GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*dbmodel.RdmaDevice, error) { return nil, nil }
func (m *BackfillMockNodeFacade) CreateRdmaDevice(ctx context.Context, rdmaDevice *dbmodel.RdmaDevice) error { return nil }
func (m *BackfillMockNodeFacade) ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*dbmodel.RdmaDevice, error) { return nil, nil }
func (m *BackfillMockNodeFacade) DeleteRdmaDeviceById(ctx context.Context, id int32) error { return nil }
func (m *BackfillMockNodeFacade) CreateNodeDeviceChangelog(ctx context.Context, changelog *dbmodel.NodeDeviceChangelog) error { return nil }

// BackfillMockGpuAggregationFacade implements database.GpuAggregationFacadeInterface
type BackfillMockGpuAggregationFacade struct {
	GetClusterHourlyStatsFunc    func(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error)
	SaveClusterHourlyStatsFunc   func(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error
	ListNamespaceHourlyStatsFunc func(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error)
	SaveNamespaceHourlyStatsFunc func(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error
	LabelHourlyStatsExistsFunc   func(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error)
	SaveLabelHourlyStatsFunc     func(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error
}

func (m *BackfillMockGpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
	if m.GetClusterHourlyStatsFunc != nil {
		return m.GetClusterHourlyStatsFunc(ctx, startTime, endTime)
	}
	return nil, nil
}
func (m *BackfillMockGpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
	if m.SaveClusterHourlyStatsFunc != nil {
		return m.SaveClusterHourlyStatsFunc(ctx, stats)
	}
	return nil
}
func (m *BackfillMockGpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	if m.ListNamespaceHourlyStatsFunc != nil {
		return m.ListNamespaceHourlyStatsFunc(ctx, startTime, endTime)
	}
	return nil, nil
}
func (m *BackfillMockGpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
	if m.SaveNamespaceHourlyStatsFunc != nil {
		return m.SaveNamespaceHourlyStatsFunc(ctx, stats)
	}
	return nil
}
func (m *BackfillMockGpuAggregationFacade) LabelHourlyStatsExists(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error) {
	if m.LabelHourlyStatsExistsFunc != nil {
		return m.LabelHourlyStatsExistsFunc(ctx, clusterName, dimensionType, dimensionKey, dimensionValue, hour)
	}
	return false, nil
}
func (m *BackfillMockGpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error {
	if m.SaveLabelHourlyStatsFunc != nil {
		return m.SaveLabelHourlyStatsFunc(ctx, stats)
	}
	return nil
}
func (m *BackfillMockGpuAggregationFacade) WithCluster(clusterName string) database.GpuAggregationFacadeInterface { return m }
func (m *BackfillMockGpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error { return nil }
func (m *BackfillMockGpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error { return nil }
func (m *BackfillMockGpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginatedWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error { return nil }
func (m *BackfillMockGpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error { return nil }
func (m *BackfillMockGpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error { return nil }
func (m *BackfillMockGpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginatedWithExclusion(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error { return nil }
func (m *BackfillMockGpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error) { return 0, nil }
func (m *BackfillMockGpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) { return 0, nil }
func (m *BackfillMockGpuAggregationFacade) GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) GetDistinctNamespacesWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string) ([]string, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error) { return nil, nil }
func (m *BackfillMockGpuAggregationFacade) GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error) { return nil, nil }

// BackfillMockNamespaceInfoFacade implements database.NamespaceInfoFacadeInterface
type BackfillMockNamespaceInfoFacade struct {
	ListFunc func(ctx context.Context) ([]*dbmodel.NamespaceInfo, error)
}

func (m *BackfillMockNamespaceInfoFacade) List(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
	if m.ListFunc != nil {
		return m.ListFunc(ctx)
	}
	return nil, nil
}
func (m *BackfillMockNamespaceInfoFacade) WithCluster(clusterName string) database.NamespaceInfoFacadeInterface { return m }
func (m *BackfillMockNamespaceInfoFacade) Create(ctx context.Context, info *dbmodel.NamespaceInfo) error { return nil }
func (m *BackfillMockNamespaceInfoFacade) DeleteByName(ctx context.Context, name string) error { return nil }
func (m *BackfillMockNamespaceInfoFacade) GetByName(ctx context.Context, name string) (*dbmodel.NamespaceInfo, error) { return nil, nil }
func (m *BackfillMockNamespaceInfoFacade) GetByNameIncludingDeleted(ctx context.Context, name string) (*dbmodel.NamespaceInfo, error) { return nil, nil }
func (m *BackfillMockNamespaceInfoFacade) Update(ctx context.Context, info *dbmodel.NamespaceInfo) error { return nil }
func (m *BackfillMockNamespaceInfoFacade) ListAllIncludingDeleted(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) { return nil, nil }
func (m *BackfillMockNamespaceInfoFacade) Recover(ctx context.Context, name string, gpuModel string, gpuResource int32) error { return nil }

// BackfillMockAllocationCalculator implements ClusterBackfillAllocationCalculatorInterface
type BackfillMockAllocationCalculator struct {
	CalculateHourlyGpuAllocationFunc func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error)
}

func (m *BackfillMockAllocationCalculator) CalculateHourlyGpuAllocation(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
	if m.CalculateHourlyGpuAllocationFunc != nil {
		return m.CalculateHourlyGpuAllocationFunc(ctx, hour)
	}
	return &statistics.GpuAllocationResult{}, nil
}

// BackfillMockNamespaceAllocationCalculator implements NamespaceBackfillAllocationCalculatorInterface
type BackfillMockNamespaceAllocationCalculator struct {
	CalculateHourlyNamespaceGpuAllocationFunc func(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error)
}

func (m *BackfillMockNamespaceAllocationCalculator) CalculateHourlyNamespaceGpuAllocation(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error) {
	if m.CalculateHourlyNamespaceGpuAllocationFunc != nil {
		return m.CalculateHourlyNamespaceGpuAllocationFunc(ctx, namespace, hour)
	}
	return &statistics.GpuAllocationResult{}, nil
}

// BackfillMockUtilizationCalculator implements NamespaceBackfillUtilizationCalculatorInterface
type BackfillMockUtilizationCalculator struct {
	CalculateHourlyNamespaceUtilizationFunc func(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult
}

func (m *BackfillMockUtilizationCalculator) CalculateHourlyNamespaceUtilization(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult {
	if m.CalculateHourlyNamespaceUtilizationFunc != nil {
		return m.CalculateHourlyNamespaceUtilizationFunc(ctx, namespace, allocationResult, hour)
	}
	return &statistics.NamespaceUtilizationResult{}
}

// ==================== Cluster Backfill Tests ====================

func TestClusterGpuAggregationBackfillJob_Run_Disabled(t *testing.T) {
	job := NewClusterGpuAggregationBackfillJob(
		WithClusterBackfillClusterName("test-cluster"),
	)
	job.config.Enabled = false

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "Cluster GPU aggregation backfill job is disabled")
}

func TestClusterGpuAggregationBackfillJob_Run_NoMissingHours(t *testing.T) {
	now := time.Now()
	hourAgo := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := hourAgo.Add(-time.Hour)

	mockGpuAggFacade := &BackfillMockGpuAggregationFacade{
		GetClusterHourlyStatsFunc: func(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
			// Return stats for all hours in the range
			hours := generateAllHours(startTime, endTime.Add(-time.Hour))
			stats := make([]*dbmodel.ClusterGpuHourlyStats, len(hours))
			for i, h := range hours {
				stats[i] = &dbmodel.ClusterGpuHourlyStats{StatHour: h}
			}
			return stats, nil
		},
	}

	mockNodeFacade := &BackfillMockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
			return []*dbmodel.Node{{Name: "node-1", GpuCount: 8}}, 1, nil
		},
	}

	mockFacade := &BackfillMockFacade{
		nodeFacade:           mockNodeFacade,
		gpuAggregationFacade: mockGpuAggFacade,
	}

	job := NewClusterGpuAggregationBackfillJob(
		WithClusterBackfillFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterBackfillClusterName("test-cluster"),
	)
	job.config.BackfillDays = 1

	// Set start time to be at least 2 hours ago to ensure there are hours to process
	_ = twoHoursAgo

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
}

func TestClusterGpuAggregationBackfillJob_Run_SuccessfulBackfill(t *testing.T) {
	savedStats := make([]*dbmodel.ClusterGpuHourlyStats, 0)

	mockGpuAggFacade := &BackfillMockGpuAggregationFacade{
		GetClusterHourlyStatsFunc: func(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
			// Return empty - all hours missing
			return []*dbmodel.ClusterGpuHourlyStats{}, nil
		},
		SaveClusterHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockNodeFacade := &BackfillMockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
			return []*dbmodel.Node{
				{Name: "node-1", GpuCount: 8},
				{Name: "node-2", GpuCount: 8},
			}, 2, nil
		},
	}

	mockFacade := &BackfillMockFacade{
		nodeFacade:           mockNodeFacade,
		gpuAggregationFacade: mockGpuAggFacade,
	}

	mockAllocationCalc := &BackfillMockAllocationCalculator{
		CalculateHourlyGpuAllocationFunc: func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{
				TotalAllocatedGpu: 8.0,
				WorkloadCount:     5,
			}, nil
		},
	}

	job := NewClusterGpuAggregationBackfillJob(
		WithClusterBackfillFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterBackfillAllocationCalculatorFactory(func(clusterName string) ClusterBackfillAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithClusterBackfillUtilizationQueryFunc(func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (float64, error) {
			return 75.0, nil
		}),
		WithClusterBackfillClusterName("test-cluster"),
	)
	job.config.BackfillDays = 1

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.True(t, len(savedStats) > 0 || stats.ItemsCreated > 0)
}

func TestClusterGpuAggregationBackfillJob_GetClusterHourlyStatsError(t *testing.T) {
	mockGpuAggFacade := &BackfillMockGpuAggregationFacade{
		GetClusterHourlyStatsFunc: func(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
			return nil, errors.New("database error")
		},
	}

	mockFacade := &BackfillMockFacade{
		gpuAggregationFacade: mockGpuAggFacade,
	}

	job := NewClusterGpuAggregationBackfillJob(
		WithClusterBackfillFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterBackfillClusterName("test-cluster"),
	)
	job.config.BackfillDays = 1

	ctx := context.Background()
	_, err := job.Run(ctx, nil, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find missing cluster stats")
}

// ==================== Namespace Backfill Tests ====================

func TestNamespaceGpuAggregationBackfillJob_Run_Disabled(t *testing.T) {
	job := NewNamespaceGpuAggregationBackfillJob(
		WithNamespaceBackfillClusterName("test-cluster"),
	)
	job.config.Enabled = false

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "Namespace GPU aggregation backfill job is disabled")
}

func TestNamespaceGpuAggregationBackfillJob_Run_SuccessfulBackfill(t *testing.T) {
	savedStats := make([]*dbmodel.NamespaceGpuHourlyStats, 0)

	mockGpuAggFacade := &BackfillMockGpuAggregationFacade{
		ListNamespaceHourlyStatsFunc: func(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
			return []*dbmodel.NamespaceGpuHourlyStats{}, nil
		},
		SaveNamespaceHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockNamespaceInfoFacade := &BackfillMockNamespaceInfoFacade{
		ListFunc: func(ctx context.Context) ([]*dbmodel.NamespaceInfo, error) {
			return []*dbmodel.NamespaceInfo{
				{Name: "production", GpuResource: 16},
				{Name: "staging", GpuResource: 8},
			}, nil
		},
	}

	mockFacade := &BackfillMockFacade{
		gpuAggregationFacade: mockGpuAggFacade,
		namespaceInfoFacade:  mockNamespaceInfoFacade,
	}

	mockAllocationCalc := &BackfillMockNamespaceAllocationCalculator{
		CalculateHourlyNamespaceGpuAllocationFunc: func(ctx context.Context, namespace string, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{
				TotalAllocatedGpu: 4.0,
				WorkloadCount:     2,
			}, nil
		},
	}

	mockUtilizationCalc := &BackfillMockUtilizationCalculator{
		CalculateHourlyNamespaceUtilizationFunc: func(ctx context.Context, namespace string, allocationResult *statistics.GpuAllocationResult, hour time.Time) *statistics.NamespaceUtilizationResult {
			return &statistics.NamespaceUtilizationResult{
				AvgUtilization: 50.0,
				MinUtilization: 30.0,
				MaxUtilization: 80.0,
			}
		},
	}

	job := NewNamespaceGpuAggregationBackfillJob(
		WithNamespaceBackfillFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithNamespaceBackfillAllocationCalculatorFactory(func(clusterName string) NamespaceBackfillAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithNamespaceBackfillUtilizationCalculatorFactory(func(clusterName string, storageClientSet *clientsets.StorageClientSet) NamespaceBackfillUtilizationCalculatorInterface {
			return mockUtilizationCalc
		}),
		WithNamespaceBackfillClusterName("test-cluster"),
	)
	job.config.BackfillDays = 1

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
}

// BackfillMockSystemConfigFacade implements database.SystemConfigFacadeInterface
type BackfillMockSystemConfigFacade struct {
	GetByKeyFunc func(ctx context.Context, key string) (*dbmodel.SystemConfig, error)
}

func (m *BackfillMockSystemConfigFacade) GetByKey(ctx context.Context, key string) (*dbmodel.SystemConfig, error) {
	if m.GetByKeyFunc != nil {
		return m.GetByKeyFunc(ctx, key)
	}
	return nil, nil
}
func (m *BackfillMockSystemConfigFacade) WithCluster(clusterName string) database.SystemConfigFacadeInterface { return m }
func (m *BackfillMockSystemConfigFacade) Create(ctx context.Context, config *dbmodel.SystemConfig) error { return nil }
func (m *BackfillMockSystemConfigFacade) Update(ctx context.Context, config *dbmodel.SystemConfig, updates map[string]interface{}) error { return nil }
func (m *BackfillMockSystemConfigFacade) Delete(ctx context.Context, key string) error { return nil }
func (m *BackfillMockSystemConfigFacade) List(ctx context.Context, query *gorm.DB) ([]*dbmodel.SystemConfig, error) { return nil, nil }
func (m *BackfillMockSystemConfigFacade) BatchGet(ctx context.Context, keys []string) ([]*dbmodel.SystemConfig, error) { return nil, nil }
func (m *BackfillMockSystemConfigFacade) CreateHistory(ctx context.Context, history *dbmodel.SystemConfigHistory) error { return nil }
func (m *BackfillMockSystemConfigFacade) GetHistory(ctx context.Context, key string, limit int) ([]*dbmodel.SystemConfigHistory, error) { return nil, nil }
func (m *BackfillMockSystemConfigFacade) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (m *BackfillMockSystemConfigFacade) GetDB() *gorm.DB { return nil }

// ==================== Label Backfill Tests ====================

func TestLabelGpuAggregationBackfillJob_Run_Disabled(t *testing.T) {
	mockSystemConfigFacade := &BackfillMockSystemConfigFacade{
		GetByKeyFunc: func(ctx context.Context, key string) (*dbmodel.SystemConfig, error) {
			return nil, errors.New("not found")
		},
	}

	mockFacade := &BackfillMockFacade{
		systemConfigFacade: mockSystemConfigFacade,
	}

	job := NewLabelGpuAggregationBackfillJob(
		WithLabelBackfillFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithLabelBackfillClusterName("test-cluster"),
	)
	job.config.Enabled = false

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "Label GPU aggregation backfill job is disabled")
}

func TestLabelGpuAggregationBackfillJob_Run_NoKeys(t *testing.T) {
	mockSystemConfigFacade := &BackfillMockSystemConfigFacade{
		GetByKeyFunc: func(ctx context.Context, key string) (*dbmodel.SystemConfig, error) {
			return nil, errors.New("not found")
		},
	}

	mockFacade := &BackfillMockFacade{
		systemConfigFacade: mockSystemConfigFacade,
	}

	job := NewLabelGpuAggregationBackfillJob(
		WithLabelBackfillFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithLabelBackfillClusterName("test-cluster"),
	)
	job.config.Enabled = true
	job.config.LabelKeys = []string{}
	job.config.AnnotationKeys = []string{}

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "No label or annotation keys configured")
}

// ==================== Helper Function Tests ====================

func TestFindMissingClusterHours(t *testing.T) {
	allHours := []time.Time{
		time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	existingStats := []*dbmodel.ClusterGpuHourlyStats{
		{StatHour: time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)},
	}

	missing := FindMissingClusterHours(allHours, existingStats)

	assert.Equal(t, 2, len(missing))
	assert.Contains(t, missing, time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC))
	assert.Contains(t, missing, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC))
}

func TestFindMissingClusterHours_AllExist(t *testing.T) {
	allHours := []time.Time{
		time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC),
	}

	existingStats := []*dbmodel.ClusterGpuHourlyStats{
		{StatHour: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{StatHour: time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)},
	}

	missing := FindMissingClusterHours(allHours, existingStats)

	assert.Equal(t, 0, len(missing))
}

func TestFindMissingNamespaceHours(t *testing.T) {
	allHours := []time.Time{
		time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
	}
	allNamespaces := []string{"production", "staging"}

	existingStats := []*dbmodel.NamespaceGpuHourlyStats{
		{StatHour: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), Namespace: "production"},
	}

	missing := FindMissingNamespaceHours(allHours, allNamespaces, existingStats)

	assert.Equal(t, 1, len(missing))
	assert.Contains(t, missing[allHours[0]], "staging")
}

func TestBuildClusterStatsFromResult(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	result := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 16.0,
		WorkloadCount:     10,
	}

	stats := BuildClusterStatsFromResult("test-cluster", hour, result)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, 16.0, stats.AllocatedGpuCount)
	assert.Equal(t, int32(10), stats.SampleCount)
}

func TestBuildNamespaceStatsFromResult(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	result := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 8.0,
		WorkloadCount:     5,
	}
	utilizationResult := &statistics.NamespaceUtilizationResult{
		AvgUtilization: 60.0,
		MinUtilization: 40.0,
		MaxUtilization: 80.0,
	}

	stats := BuildNamespaceStatsFromResult("test-cluster", "production", hour, result, utilizationResult)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, "production", stats.Namespace)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, 8.0, stats.AllocatedGpuCount)
	assert.Equal(t, int32(5), stats.ActiveWorkloadCount)
	assert.Equal(t, 60.0, stats.AvgUtilization)
}

func TestBuildLabelStatsFromAggregation(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	agg := &statistics.LabelAggregationResult{
		DimensionType:       "label",
		DimensionKey:        "app",
		DimensionValue:      "web",
		TotalAllocatedGpu:   4.0,
		ActiveWorkloadCount: 3,
	}
	utilizationStats := &statistics.UtilizationStats{
		AvgUtilization: 55.0,
		MinUtilization: 35.0,
		MaxUtilization: 75.0,
	}

	stats := BuildLabelStatsFromAggregation("test-cluster", hour, agg, utilizationStats)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, "label", stats.DimensionType)
	assert.Equal(t, "app", stats.DimensionKey)
	assert.Equal(t, "web", stats.DimensionValue)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, 4.0, stats.AllocatedGpuCount)
	assert.Equal(t, int32(3), stats.ActiveWorkloadCount)
	assert.Equal(t, 55.0, stats.AvgUtilization)
}

func TestCalculateClusterBackfillGpuCapacity(t *testing.T) {
	nodes := []*dbmodel.Node{
		{Name: "node-1", GpuCount: 8},
		{Name: "node-2", GpuCount: 4},
		{Name: "node-3", GpuCount: 0},
	}

	capacity := CalculateClusterBackfillGpuCapacity(nodes)

	assert.Equal(t, 12, capacity)
}

// ==================== Option Function Tests ====================

func TestWithClusterBackfillFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterName string) database.FacadeInterface {
		called = true
		return &BackfillMockFacade{}
	}

	job := &ClusterGpuAggregationBackfillJob{}
	opt := WithClusterBackfillFacadeGetter(mockGetter)
	opt(job)

	job.facadeGetter("test")
	assert.True(t, called)
}

func TestWithNamespaceBackfillFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterName string) database.FacadeInterface {
		called = true
		return &BackfillMockFacade{}
	}

	job := &NamespaceGpuAggregationBackfillJob{}
	opt := WithNamespaceBackfillFacadeGetter(mockGetter)
	opt(job)

	job.facadeGetter("test")
	assert.True(t, called)
}

func TestWithLabelBackfillFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterName string) database.FacadeInterface {
		called = true
		return &BackfillMockFacade{}
	}

	job := &LabelGpuAggregationBackfillJob{}
	opt := WithLabelBackfillFacadeGetter(mockGetter)
	opt(job)

	job.facadeGetter("test")
	assert.True(t, called)
}

// ==================== Schedule Tests ====================

func TestClusterBackfillJob_Schedule(t *testing.T) {
	job := NewClusterGpuAggregationBackfillJob(WithClusterBackfillClusterName("test"))
	assert.Equal(t, "@every 5m", job.Schedule())
}

func TestNamespaceBackfillJob_Schedule(t *testing.T) {
	job := NewNamespaceGpuAggregationBackfillJob(WithNamespaceBackfillClusterName("test"))
	assert.Equal(t, "@every 5m", job.Schedule())
}

func TestLabelBackfillJob_Schedule(t *testing.T) {
	job := NewLabelGpuAggregationBackfillJob(WithLabelBackfillClusterName("test"))
	assert.Equal(t, "@every 5m", job.Schedule())
}

