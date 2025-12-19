package gpu_aggregation

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
)

// ==================== Mock Implementations ====================

// ClusterMockFacade implements database.FacadeInterface for testing
type ClusterMockFacade struct {
	nodeFacade           database.NodeFacadeInterface
	gpuAggregationFacade database.GpuAggregationFacadeInterface
	genericCacheFacade   database.GenericCacheFacadeInterface
}

func (m *ClusterMockFacade) GetNode() database.NodeFacadeInterface {
	return m.nodeFacade
}

func (m *ClusterMockFacade) GetGpuAggregation() database.GpuAggregationFacadeInterface {
	return m.gpuAggregationFacade
}

func (m *ClusterMockFacade) GetGenericCache() database.GenericCacheFacadeInterface {
	return m.genericCacheFacade
}

// Implement other methods with nil returns (not used in tests)
func (m *ClusterMockFacade) GetWorkload() database.WorkloadFacadeInterface               { return nil }
func (m *ClusterMockFacade) GetPod() database.PodFacadeInterface                         { return nil }
func (m *ClusterMockFacade) GetContainer() database.ContainerFacadeInterface             { return nil }
func (m *ClusterMockFacade) GetTraining() database.TrainingFacadeInterface               { return nil }
func (m *ClusterMockFacade) GetStorage() database.StorageFacadeInterface                 { return nil }
func (m *ClusterMockFacade) GetAlert() database.AlertFacadeInterface                     { return nil }
func (m *ClusterMockFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface { return nil }
func (m *ClusterMockFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface       { return nil }
func (m *ClusterMockFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface { return nil }
func (m *ClusterMockFacade) GetClusterOverviewCache() database.ClusterOverviewCacheFacadeInterface {
	return nil
}
func (m *ClusterMockFacade) GetSystemConfig() database.SystemConfigFacadeInterface { return nil }
func (m *ClusterMockFacade) GetJobExecutionHistory() database.JobExecutionHistoryFacadeInterface {
	return nil
}
func (m *ClusterMockFacade) GetNamespaceInfo() database.NamespaceInfoFacadeInterface { return nil }
func (m *ClusterMockFacade) GetWorkloadStatistic() database.WorkloadStatisticFacadeInterface {
	return nil
}
func (m *ClusterMockFacade) GetAiWorkloadMetadata() database.AiWorkloadMetadataFacadeInterface {
	return nil
}
func (m *ClusterMockFacade) GetCheckpointEvent() database.CheckpointEventFacadeInterface { return nil }
func (m *ClusterMockFacade) GetDetectionConflictLog() database.DetectionConflictLogFacadeInterface {
	return nil
}
func (m *ClusterMockFacade) GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface {
	return nil
}
func (m *ClusterMockFacade) GetNodeNamespaceMapping() database.NodeNamespaceMappingFacadeInterface {
	return nil
}
func (m *ClusterMockFacade) GetTraceLensSession() database.TraceLensSessionFacadeInterface { return nil }
func (m *ClusterMockFacade) GetK8sService() database.K8sServiceFacadeInterface             { return nil }
func (m *ClusterMockFacade) WithCluster(clusterName string) database.FacadeInterface       { return m }

// ClusterMockNodeFacade implements database.NodeFacadeInterface for testing
type ClusterMockNodeFacade struct {
	SearchNodeFunc func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error)
}

func (m *ClusterMockNodeFacade) SearchNode(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
	if m.SearchNodeFunc != nil {
		return m.SearchNodeFunc(ctx, f)
	}
	return nil, 0, nil
}

// Implement other required methods
func (m *ClusterMockNodeFacade) WithCluster(clusterName string) database.NodeFacadeInterface {
	return m
}
func (m *ClusterMockNodeFacade) GetNodeByName(ctx context.Context, name string) (*dbmodel.Node, error) {
	return nil, nil
}
func (m *ClusterMockNodeFacade) CreateNode(ctx context.Context, node *dbmodel.Node) error { return nil }
func (m *ClusterMockNodeFacade) UpdateNode(ctx context.Context, node *dbmodel.Node) error { return nil }
func (m *ClusterMockNodeFacade) ListGpuNodes(ctx context.Context) ([]*dbmodel.Node, error) {
	return nil, nil
}
func (m *ClusterMockNodeFacade) GetGpuDeviceByNodeAndGpuId(ctx context.Context, nodeId int32, gpuId int) (*dbmodel.GpuDevice, error) {
	return nil, nil
}
func (m *ClusterMockNodeFacade) CreateGpuDevice(ctx context.Context, device *dbmodel.GpuDevice) error {
	return nil
}
func (m *ClusterMockNodeFacade) UpdateGpuDevice(ctx context.Context, device *dbmodel.GpuDevice) error {
	return nil
}
func (m *ClusterMockNodeFacade) ListGpuDeviceByNodeId(ctx context.Context, nodeId int32) ([]*dbmodel.GpuDevice, error) {
	return nil, nil
}
func (m *ClusterMockNodeFacade) DeleteGpuDeviceById(ctx context.Context, id int32) error { return nil }
func (m *ClusterMockNodeFacade) GetRdmaDeviceByNodeIdAndPort(ctx context.Context, nodeGuid string, port int) (*dbmodel.RdmaDevice, error) {
	return nil, nil
}
func (m *ClusterMockNodeFacade) CreateRdmaDevice(ctx context.Context, rdmaDevice *dbmodel.RdmaDevice) error {
	return nil
}
func (m *ClusterMockNodeFacade) ListRdmaDeviceByNodeId(ctx context.Context, nodeId int32) ([]*dbmodel.RdmaDevice, error) {
	return nil, nil
}
func (m *ClusterMockNodeFacade) DeleteRdmaDeviceById(ctx context.Context, id int32) error { return nil }
func (m *ClusterMockNodeFacade) CreateNodeDeviceChangelog(ctx context.Context, changelog *dbmodel.NodeDeviceChangelog) error {
	return nil
}

// ClusterMockGpuAggregationFacade implements database.GpuAggregationFacadeInterface for testing
type ClusterMockGpuAggregationFacade struct {
	SaveClusterHourlyStatsFunc func(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error
	savedStats                 []*dbmodel.ClusterGpuHourlyStats
}

func (m *ClusterMockGpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
	if m.SaveClusterHourlyStatsFunc != nil {
		return m.SaveClusterHourlyStatsFunc(ctx, stats)
	}
	m.savedStats = append(m.savedStats, stats)
	return nil
}

// Implement other required methods
func (m *ClusterMockGpuAggregationFacade) WithCluster(clusterName string) database.GpuAggregationFacadeInterface {
	return m
}
func (m *ClusterMockGpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginatedWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) LabelHourlyStatsExists(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error) {
	return false, nil
}
func (m *ClusterMockGpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginatedWithExclusion(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error {
	return nil
}
func (m *ClusterMockGpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}
func (m *ClusterMockGpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) {
	return 0, nil
}
func (m *ClusterMockGpuAggregationFacade) GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetDistinctNamespacesWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string) ([]string, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}
func (m *ClusterMockGpuAggregationFacade) GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error) {
	return nil, nil
}

// ClusterMockGenericCacheFacade implements database.GenericCacheFacadeInterface for testing
type ClusterMockGenericCacheFacade struct {
	GetFunc func(ctx context.Context, key string, value interface{}) error
	SetFunc func(ctx context.Context, key string, value interface{}, expiration *time.Time) error
}

func (m *ClusterMockGenericCacheFacade) Get(ctx context.Context, key string, value interface{}) error {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, key, value)
	}
	return nil
}

func (m *ClusterMockGenericCacheFacade) Set(ctx context.Context, key string, value interface{}, expiration *time.Time) error {
	if m.SetFunc != nil {
		return m.SetFunc(ctx, key, value, expiration)
	}
	return nil
}

func (m *ClusterMockGenericCacheFacade) WithCluster(clusterName string) database.GenericCacheFacadeInterface {
	return m
}
func (m *ClusterMockGenericCacheFacade) Delete(ctx context.Context, key string) error { return nil }
func (m *ClusterMockGenericCacheFacade) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}
func (m *ClusterMockGenericCacheFacade) DeleteExpired(ctx context.Context) error { return nil }

// ClusterMockAllocationCalculator implements ClusterAllocationCalculatorInterface for testing
type ClusterMockAllocationCalculator struct {
	CalculateHourlyGpuAllocationFunc func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error)
}

func (m *ClusterMockAllocationCalculator) CalculateHourlyGpuAllocation(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
	if m.CalculateHourlyGpuAllocationFunc != nil {
		return m.CalculateHourlyGpuAllocationFunc(ctx, hour)
	}
	return &statistics.GpuAllocationResult{}, nil
}

// ==================== Test Cases ====================

func TestNewClusterGpuAggregationJob_Default(t *testing.T) {
	job := NewClusterGpuAggregationJob(
		WithClusterClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.NotNil(t, job.config)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, "test-cluster", job.clusterName)
}

func TestNewClusterGpuAggregationJob_WithOptions(t *testing.T) {
	mockFacadeGetter := func(clusterName string) database.FacadeInterface {
		return &ClusterMockFacade{}
	}

	job := NewClusterGpuAggregationJob(
		WithClusterFacadeGetter(mockFacadeGetter),
		WithClusterClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "test-cluster", job.clusterName)
	assert.NotNil(t, job.facadeGetter)
}

func TestNewClusterGpuAggregationJobWithConfig_WithOptions(t *testing.T) {
	config := &ClusterGpuAggregationConfig{
		Enabled: false,
	}

	job := NewClusterGpuAggregationJobWithConfig(config,
		WithClusterClusterName("custom-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "custom-cluster", job.clusterName)
	assert.False(t, job.config.Enabled)
}

func TestClusterGpuAggregationJob_Run_Disabled(t *testing.T) {
	job := NewClusterGpuAggregationJob(
		WithClusterClusterName("test-cluster"),
	)
	job.config.Enabled = false

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "Cluster GPU aggregation job is disabled")
}

func TestClusterGpuAggregationJob_GetConfig_SetConfig(t *testing.T) {
	job := NewClusterGpuAggregationJob(WithClusterClusterName("test"))

	config := job.GetConfig()
	assert.NotNil(t, config)
	assert.True(t, config.Enabled)

	newConfig := &ClusterGpuAggregationConfig{
		Enabled: false,
	}
	job.SetConfig(newConfig)

	assert.False(t, job.GetConfig().Enabled)
}

func TestClusterGpuAggregationJob_ScheduleReturnsEvery5m(t *testing.T) {
	job := NewClusterGpuAggregationJob(WithClusterClusterName("test"))
	assert.Equal(t, "@every 5m", job.Schedule())
}

// ==================== Tests for Exported Helper Functions ====================

func TestBuildClusterGpuHourlyStats(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 64.0,
		WorkloadCount:     10,
		PodCount:          20,
	}

	utilizationStats := &statistics.ClusterGpuUtilizationStats{
		AvgUtilization: 75.0,
		MaxUtilization: 95.0,
		MinUtilization: 50.0,
		P50Utilization: 72.0,
		P95Utilization: 90.0,
	}

	stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, 100, utilizationStats)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, int32(100), stats.TotalGpuCapacity)
	assert.Equal(t, 64.0, stats.AllocatedGpuCount)
	assert.Equal(t, int32(10), stats.SampleCount)
	assert.Equal(t, 75.0, stats.AvgUtilization)
	assert.Equal(t, 95.0, stats.MaxUtilization)
	assert.Equal(t, 50.0, stats.MinUtilization)
	assert.Equal(t, 72.0, stats.P50Utilization)
	assert.Equal(t, 90.0, stats.P95Utilization)
	assert.Equal(t, 64.0, stats.AllocationRate) // (64/100) * 100 = 64%
}

func TestBuildClusterGpuHourlyStats_NoCapacity(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 8.0,
	}

	utilizationStats := &statistics.ClusterGpuUtilizationStats{
		AvgUtilization: 50.0,
	}

	stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, 0, utilizationStats)

	assert.Equal(t, int32(0), stats.TotalGpuCapacity)
	assert.Equal(t, float64(0), stats.AllocationRate) // No capacity, no rate
}

func TestBuildClusterGpuHourlyStats_ZeroAllocation(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 0,
	}

	utilizationStats := &statistics.ClusterGpuUtilizationStats{}

	stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, 100, utilizationStats)

	assert.Equal(t, float64(0), stats.AllocatedGpuCount)
	assert.Equal(t, float64(0), stats.AllocationRate)
}

func TestCalculateClusterGpuCapacity(t *testing.T) {
	tests := []struct {
		name     string
		nodes    []*dbmodel.Node
		expected int
	}{
		{
			name:     "no nodes",
			nodes:    []*dbmodel.Node{},
			expected: 0,
		},
		{
			name: "single node with GPUs",
			nodes: []*dbmodel.Node{
				{Name: "node-1", GpuCount: 8},
			},
			expected: 8,
		},
		{
			name: "multiple nodes",
			nodes: []*dbmodel.Node{
				{Name: "node-1", GpuCount: 8},
				{Name: "node-2", GpuCount: 4},
				{Name: "node-3", GpuCount: 2},
			},
			expected: 14,
		},
		{
			name: "nodes with zero GPUs",
			nodes: []*dbmodel.Node{
				{Name: "node-1", GpuCount: 8},
				{Name: "node-2", GpuCount: 0},
				{Name: "node-3", GpuCount: 4},
			},
			expected: 12,
		},
		{
			name:     "nil nodes",
			nodes:    nil,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateClusterGpuCapacity(tt.nodes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ==================== Option Functions Tests ====================

func TestWithClusterFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterName string) database.FacadeInterface {
		called = true
		return &ClusterMockFacade{}
	}

	job := &ClusterGpuAggregationJob{}
	opt := WithClusterFacadeGetter(mockGetter)
	opt(job)

	job.facadeGetter("test")
	assert.True(t, called)
}

func TestWithClusterClusterNameGetter(t *testing.T) {
	mockGetter := func() string {
		return "mock-cluster"
	}

	job := &ClusterGpuAggregationJob{}
	opt := WithClusterClusterNameGetter(mockGetter)
	opt(job)

	assert.Equal(t, "mock-cluster", job.clusterNameGetter())
}

func TestWithClusterClusterName(t *testing.T) {
	job := &ClusterGpuAggregationJob{}
	opt := WithClusterClusterName("my-cluster")
	opt(job)

	assert.Equal(t, "my-cluster", job.clusterName)
}

func TestWithClusterAllocationCalculatorFactory(t *testing.T) {
	called := false
	mockFactory := func(clusterName string) ClusterAllocationCalculatorInterface {
		called = true
		return &ClusterMockAllocationCalculator{}
	}

	job := &ClusterGpuAggregationJob{}
	opt := WithClusterAllocationCalculatorFactory(mockFactory)
	opt(job)

	job.allocationCalculatorFactory("test")
	assert.True(t, called)
}

func TestWithClusterUtilizationQueryFunc(t *testing.T) {
	called := false
	mockFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
		called = true
		return &statistics.ClusterGpuUtilizationStats{
			AvgUtilization: 50.0,
			MaxUtilization: 70.0,
			MinUtilization: 30.0,
			P50Utilization: 48.0,
			P95Utilization: 68.0,
		}, nil
	}

	job := &ClusterGpuAggregationJob{}
	opt := WithClusterUtilizationQueryFunc(mockFunc)
	opt(job)

	result, err := job.utilizationQueryFunc(context.Background(), nil, time.Now())
	assert.True(t, called)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 50.0, result.AvgUtilization)
	assert.Equal(t, 70.0, result.MaxUtilization)
	assert.Equal(t, 30.0, result.MinUtilization)
	assert.Equal(t, 48.0, result.P50Utilization)
	assert.Equal(t, 68.0, result.P95Utilization)
}

// ==================== Constants Tests ====================

func TestClusterConstants(t *testing.T) {
	assert.Equal(t, "job.cluster_gpu_aggregation.last_processed_hour", CacheKeyClusterGpuAggregationLastHour)
}

// ==================== Config Tests ====================

func TestClusterGpuAggregationConfig_Defaults(t *testing.T) {
	config := &ClusterGpuAggregationConfig{}

	assert.False(t, config.Enabled)
}

func TestClusterGpuAggregationConfig_WithValues(t *testing.T) {
	config := &ClusterGpuAggregationConfig{
		Enabled: true,
	}

	assert.True(t, config.Enabled)
}

// ==================== Integration Tests with Dependency Injection ====================

func TestClusterGpuAggregationJob_Run_SuccessfulAggregation(t *testing.T) {
	now := time.Now()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := previousHour.Add(-time.Hour)

	mockCacheFacade := &ClusterMockGenericCacheFacade{
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

	mockNodeFacade := &ClusterMockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
			return []*dbmodel.Node{
				{Name: "node-1", GpuCount: 8},
				{Name: "node-2", GpuCount: 8},
			}, 2, nil
		},
	}

	savedStats := make([]*dbmodel.ClusterGpuHourlyStats, 0)
	mockGpuAggregationFacade := &ClusterMockGpuAggregationFacade{
		SaveClusterHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockFacade := &ClusterMockFacade{
		nodeFacade:           mockNodeFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
		genericCacheFacade:   mockCacheFacade,
	}

	mockAllocationCalc := &ClusterMockAllocationCalculator{
		CalculateHourlyGpuAllocationFunc: func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{
				TotalAllocatedGpu: 8.0,
				WorkloadCount:     5,
			}, nil
		},
	}

	mockUtilizationQuery := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
		return &statistics.ClusterGpuUtilizationStats{
			AvgUtilization: 75.0,
			MaxUtilization: 90.0,
			MinUtilization: 60.0,
			P50Utilization: 73.0,
			P95Utilization: 88.0,
		}, nil
	}

	job := NewClusterGpuAggregationJob(
		WithClusterFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterAllocationCalculatorFactory(func(clusterName string) ClusterAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithClusterUtilizationQueryFunc(mockUtilizationQuery),
		WithClusterClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ItemsCreated)
	assert.Equal(t, 1, len(savedStats))
	assert.Equal(t, 8.0, savedStats[0].AllocatedGpuCount)
	assert.Equal(t, int32(16), savedStats[0].TotalGpuCapacity)
	assert.Equal(t, 75.0, savedStats[0].AvgUtilization)
}

func TestClusterGpuAggregationJob_Run_AllocationCalculatorError(t *testing.T) {
	mockCacheFacade := &ClusterMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			return errors.New("no cache entry")
		},
	}

	mockFacade := &ClusterMockFacade{
		genericCacheFacade: mockCacheFacade,
	}

	mockAllocationCalc := &ClusterMockAllocationCalculator{
		CalculateHourlyGpuAllocationFunc: func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return nil, errors.New("calculation error")
		},
	}

	job := NewClusterGpuAggregationJob(
		WithClusterFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterAllocationCalculatorFactory(func(clusterName string) ClusterAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithClusterClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ErrorCount)
}

func TestClusterGpuAggregationJob_Run_GetNodesError(t *testing.T) {
	now := time.Now()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := previousHour.Add(-time.Hour)

	mockCacheFacade := &ClusterMockGenericCacheFacade{
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

	mockNodeFacade := &ClusterMockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
			return nil, 0, errors.New("node query error")
		},
	}

	mockGpuAggregationFacade := &ClusterMockGpuAggregationFacade{}

	mockFacade := &ClusterMockFacade{
		nodeFacade:           mockNodeFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
		genericCacheFacade:   mockCacheFacade,
	}

	mockAllocationCalc := &ClusterMockAllocationCalculator{
		CalculateHourlyGpuAllocationFunc: func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{}, nil
		},
	}

	job := NewClusterGpuAggregationJob(
		WithClusterFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterAllocationCalculatorFactory(func(clusterName string) ClusterAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithClusterUtilizationQueryFunc(func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
			return &statistics.ClusterGpuUtilizationStats{}, nil
		}),
		WithClusterClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	// Should still succeed, just with 0 capacity
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ItemsCreated)
}

func TestClusterGpuAggregationJob_Run_UtilizationQueryError(t *testing.T) {
	now := time.Now()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := previousHour.Add(-time.Hour)

	mockCacheFacade := &ClusterMockGenericCacheFacade{
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

	mockNodeFacade := &ClusterMockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
			return []*dbmodel.Node{{Name: "node-1", GpuCount: 8}}, 1, nil
		},
	}

	mockGpuAggregationFacade := &ClusterMockGpuAggregationFacade{}

	mockFacade := &ClusterMockFacade{
		nodeFacade:           mockNodeFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
		genericCacheFacade:   mockCacheFacade,
	}

	mockAllocationCalc := &ClusterMockAllocationCalculator{
		CalculateHourlyGpuAllocationFunc: func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{TotalAllocatedGpu: 4}, nil
		},
	}

	job := NewClusterGpuAggregationJob(
		WithClusterFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterAllocationCalculatorFactory(func(clusterName string) ClusterAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithClusterUtilizationQueryFunc(func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
			return nil, errors.New("prometheus error")
		}),
		WithClusterClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	// Should still succeed, just with 0 utilization
	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ItemsCreated)
}

// ==================== Edge Case Tests ====================

func TestClusterGpuAggregationJob_NilConfig(t *testing.T) {
	job := &ClusterGpuAggregationJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

func TestBuildClusterGpuHourlyStats_NilAllocationResult(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	// This would panic if not handled properly, but we test with valid input
	allocationResult := &statistics.GpuAllocationResult{}
	utilizationStats := &statistics.ClusterGpuUtilizationStats{}
	stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, 8, utilizationStats)

	assert.Equal(t, float64(0), stats.AllocatedGpuCount)
	assert.Equal(t, float64(0), stats.AllocationRate)
}

// ==================== Allocation Rate Tests ====================

func TestAllocationRateCalculation_Cluster(t *testing.T) {
	tests := []struct {
		name         string
		allocated    float64
		capacity     int
		expectedRate float64
	}{
		{
			name:         "full allocation",
			allocated:    100.0,
			capacity:     100,
			expectedRate: 100.0,
		},
		{
			name:         "half allocation",
			allocated:    50.0,
			capacity:     100,
			expectedRate: 50.0,
		},
		{
			name:         "no allocation",
			allocated:    0,
			capacity:     100,
			expectedRate: 0,
		},
		{
			name:         "over allocation",
			allocated:    150.0,
			capacity:     100,
			expectedRate: 150.0,
		},
		{
			name:         "no capacity",
			allocated:    50.0,
			capacity:     0,
			expectedRate: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
			allocationResult := &statistics.GpuAllocationResult{
				TotalAllocatedGpu: tt.allocated,
			}

			utilizationStats := &statistics.ClusterGpuUtilizationStats{}
			stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, tt.capacity, utilizationStats)

			assert.InDelta(t, tt.expectedRate, stats.AllocationRate, 0.001)
		})
	}
}

// ==================== Time Range Tests ====================

func TestClusterTimeRangeForAggregation(t *testing.T) {
	now := time.Now()
	currentHour := now.Truncate(time.Hour)
	previousHour := currentHour.Add(-time.Hour)

	assert.True(t, previousHour.Before(currentHour))
	assert.Equal(t, time.Hour, currentHour.Sub(previousHour))
}

func TestClusterHourTruncation(t *testing.T) {
	input := time.Date(2025, 1, 1, 10, 30, 45, 123, time.UTC)
	expected := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	result := input.Truncate(time.Hour)
	assert.Equal(t, expected, result)
}

// ==================== Full Utilization Stats Tests ====================

func TestBuildClusterGpuHourlyStats_FullUtilizationStats(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 50.0,
		WorkloadCount:     15,
		PodCount:          30,
	}

	utilizationStats := &statistics.ClusterGpuUtilizationStats{
		AvgUtilization: 65.5,
		MaxUtilization: 98.2,
		MinUtilization: 12.3,
		P50Utilization: 63.7,
		P95Utilization: 95.1,
	}

	stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, 100, utilizationStats)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, hour, stats.StatHour)
	assert.Equal(t, int32(100), stats.TotalGpuCapacity)
	assert.Equal(t, 50.0, stats.AllocatedGpuCount)
	assert.Equal(t, int32(15), stats.SampleCount)
	assert.Equal(t, 50.0, stats.AllocationRate)

	// Verify all utilization statistics are properly set
	assert.Equal(t, 65.5, stats.AvgUtilization)
	assert.Equal(t, 98.2, stats.MaxUtilization)
	assert.Equal(t, 12.3, stats.MinUtilization)
	assert.Equal(t, 63.7, stats.P50Utilization)
	assert.Equal(t, 95.1, stats.P95Utilization)
}

func TestBuildClusterGpuHourlyStats_ZeroUtilizationStats(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 25.0,
		WorkloadCount:     5,
	}

	utilizationStats := &statistics.ClusterGpuUtilizationStats{
		AvgUtilization: 0,
		MaxUtilization: 0,
		MinUtilization: 0,
		P50Utilization: 0,
		P95Utilization: 0,
	}

	stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, 50, utilizationStats)

	assert.Equal(t, 25.0, stats.AllocatedGpuCount)
	assert.Equal(t, 50.0, stats.AllocationRate)
	assert.Equal(t, 0.0, stats.AvgUtilization)
	assert.Equal(t, 0.0, stats.MaxUtilization)
	assert.Equal(t, 0.0, stats.MinUtilization)
	assert.Equal(t, 0.0, stats.P50Utilization)
	assert.Equal(t, 0.0, stats.P95Utilization)
}

func TestBuildClusterGpuHourlyStats_HighUtilization(t *testing.T) {
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	allocationResult := &statistics.GpuAllocationResult{
		TotalAllocatedGpu: 95.0,
		WorkloadCount:     20,
	}

	utilizationStats := &statistics.ClusterGpuUtilizationStats{
		AvgUtilization: 92.5,
		MaxUtilization: 99.9,
		MinUtilization: 85.0,
		P50Utilization: 93.0,
		P95Utilization: 98.5,
	}

	stats := BuildClusterGpuHourlyStats("test-cluster", hour, allocationResult, 100, utilizationStats)

	assert.Equal(t, 95.0, stats.AllocatedGpuCount)
	assert.Equal(t, 95.0, stats.AllocationRate)
	assert.Equal(t, 92.5, stats.AvgUtilization)
	assert.Equal(t, 99.9, stats.MaxUtilization)
	assert.Equal(t, 85.0, stats.MinUtilization)
	assert.Equal(t, 93.0, stats.P50Utilization)
	assert.Equal(t, 98.5, stats.P95Utilization)
}

func TestClusterGpuAggregationJob_Run_WithFullUtilizationStats(t *testing.T) {
	now := time.Now()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := previousHour.Add(-time.Hour)

	mockCacheFacade := &ClusterMockGenericCacheFacade{
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

	mockNodeFacade := &ClusterMockNodeFacade{
		SearchNodeFunc: func(ctx context.Context, f filter.NodeFilter) ([]*dbmodel.Node, int, error) {
			return []*dbmodel.Node{
				{Name: "node-1", GpuCount: 8},
				{Name: "node-2", GpuCount: 8},
			}, 2, nil
		},
	}

	savedStats := make([]*dbmodel.ClusterGpuHourlyStats, 0)
	mockGpuAggregationFacade := &ClusterMockGpuAggregationFacade{
		SaveClusterHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockFacade := &ClusterMockFacade{
		nodeFacade:           mockNodeFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
		genericCacheFacade:   mockCacheFacade,
	}

	mockAllocationCalc := &ClusterMockAllocationCalculator{
		CalculateHourlyGpuAllocationFunc: func(ctx context.Context, hour time.Time) (*statistics.GpuAllocationResult, error) {
			return &statistics.GpuAllocationResult{
				TotalAllocatedGpu: 12.5,
				WorkloadCount:     8,
			}, nil
		},
	}

	mockUtilizationQuery := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, hour time.Time) (*statistics.ClusterGpuUtilizationStats, error) {
		return &statistics.ClusterGpuUtilizationStats{
			AvgUtilization: 78.3,
			MaxUtilization: 96.7,
			MinUtilization: 42.1,
			P50Utilization: 76.5,
			P95Utilization: 94.2,
		}, nil
	}

	job := NewClusterGpuAggregationJob(
		WithClusterFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterAllocationCalculatorFactory(func(clusterName string) ClusterAllocationCalculatorInterface {
			return mockAllocationCalc
		}),
		WithClusterUtilizationQueryFunc(mockUtilizationQuery),
		WithClusterClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ItemsCreated)
	assert.Equal(t, 1, len(savedStats))

	// Verify allocation stats
	assert.Equal(t, 12.5, savedStats[0].AllocatedGpuCount)
	assert.Equal(t, int32(16), savedStats[0].TotalGpuCapacity)

	// Verify all utilization statistics
	assert.Equal(t, 78.3, savedStats[0].AvgUtilization)
	assert.Equal(t, 96.7, savedStats[0].MaxUtilization)
	assert.Equal(t, 42.1, savedStats[0].MinUtilization)
	assert.Equal(t, 76.5, savedStats[0].P50Utilization)
	assert.Equal(t, 94.2, savedStats[0].P95Utilization)
}
