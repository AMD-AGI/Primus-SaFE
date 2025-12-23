package workload_stats_backfill

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
)

// ==================== Mock Implementations ====================

// MockFacade implements database.FacadeInterface for testing
type MockFacade struct {
	workloadFacade       database.WorkloadFacadeInterface
	podFacade            database.PodFacadeInterface
	gpuAggregationFacade database.GpuAggregationFacadeInterface
}

func (m *MockFacade) GetWorkload() database.WorkloadFacadeInterface {
	return m.workloadFacade
}

func (m *MockFacade) GetPod() database.PodFacadeInterface {
	return m.podFacade
}

func (m *MockFacade) GetGpuAggregation() database.GpuAggregationFacadeInterface {
	return m.gpuAggregationFacade
}

// Implement other methods with nil returns (not used in tests)
func (m *MockFacade) GetNode() database.NodeFacadeInterface                       { return nil }
func (m *MockFacade) GetContainer() database.ContainerFacadeInterface             { return nil }
func (m *MockFacade) GetTraining() database.TrainingFacadeInterface               { return nil }
func (m *MockFacade) GetStorage() database.StorageFacadeInterface                 { return nil }
func (m *MockFacade) GetAlert() database.AlertFacadeInterface                     { return nil }
func (m *MockFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface { return nil }
func (m *MockFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface       { return nil }
func (m *MockFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface { return nil }
func (m *MockFacade) GetClusterOverviewCache() database.ClusterOverviewCacheFacadeInterface {
	return nil
}
func (m *MockFacade) GetGenericCache() database.GenericCacheFacadeInterface               { return nil }
func (m *MockFacade) GetSystemConfig() database.SystemConfigFacadeInterface               { return nil }
func (m *MockFacade) GetJobExecutionHistory() database.JobExecutionHistoryFacadeInterface { return nil }
func (m *MockFacade) GetNamespaceInfo() database.NamespaceInfoFacadeInterface             { return nil }
func (m *MockFacade) GetWorkloadStatistic() database.WorkloadStatisticFacadeInterface     { return nil }
func (m *MockFacade) GetAiWorkloadMetadata() database.AiWorkloadMetadataFacadeInterface   { return nil }
func (m *MockFacade) GetCheckpointEvent() database.CheckpointEventFacadeInterface         { return nil }
func (m *MockFacade) GetDetectionConflictLog() database.DetectionConflictLogFacadeInterface {
	return nil
}
func (m *MockFacade) GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface {
	return nil
}
func (m *MockFacade) GetNodeNamespaceMapping() database.NodeNamespaceMappingFacadeInterface {
	return nil
}
func (m *MockFacade) GetTraceLensSession() database.TraceLensSessionFacadeInterface { return nil }
func (m *MockFacade) GetK8sService() database.K8sServiceFacadeInterface             { return nil }
func (m *MockFacade) GetWorkloadDetection() database.WorkloadDetectionFacadeInterface { return nil }
func (m *MockFacade) GetWorkloadDetectionEvidence() database.WorkloadDetectionEvidenceFacadeInterface { return nil }
func (m *MockFacade) GetDetectionCoverage() database.DetectionCoverageFacadeInterface { return nil }
func (m *MockFacade) WithCluster(clusterName string) database.FacadeInterface       { return m }

// MockWorkloadFacade implements database.WorkloadFacadeInterface for testing
type MockWorkloadFacade struct {
	GetWorkloadNotEndFunc                     func(ctx context.Context) ([]*dbmodel.GpuWorkload, error)
	ListWorkloadPodReferenceByWorkloadUidFunc func(ctx context.Context, workloadUID string) ([]*dbmodel.WorkloadPodReference, error)
}

func (m *MockWorkloadFacade) GetWorkloadNotEnd(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
	if m.GetWorkloadNotEndFunc != nil {
		return m.GetWorkloadNotEndFunc(ctx)
	}
	return nil, nil
}

func (m *MockWorkloadFacade) ListWorkloadPodReferenceByWorkloadUid(ctx context.Context, workloadUID string) ([]*dbmodel.WorkloadPodReference, error) {
	if m.ListWorkloadPodReferenceByWorkloadUidFunc != nil {
		return m.ListWorkloadPodReferenceByWorkloadUidFunc(ctx, workloadUID)
	}
	return nil, nil
}

// Implement other required methods
func (m *MockWorkloadFacade) WithCluster(clusterName string) database.WorkloadFacadeInterface {
	return m
}
func (m *MockWorkloadFacade) GetGpuWorkloadByUid(ctx context.Context, uid string) (*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) CreateGpuWorkload(ctx context.Context, gpuWorkload *dbmodel.GpuWorkload) error {
	return nil
}
func (m *MockWorkloadFacade) UpdateGpuWorkload(ctx context.Context, gpuWorkload *dbmodel.GpuWorkload) error {
	return nil
}
func (m *MockWorkloadFacade) QueryWorkload(ctx context.Context, f *filter.WorkloadFilter) ([]*dbmodel.GpuWorkload, int, error) {
	return nil, 0, nil
}
func (m *MockWorkloadFacade) GetWorkloadsNamespaceList(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) GetWorkloadKindList(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListRunningWorkload(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListWorkloadsByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) GetNearestWorkloadByPodUid(ctx context.Context, podUid string) (*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListTopLevelWorkloadByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListChildrenWorkloadByParentUid(ctx context.Context, parentUid string) ([]*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListWorkloadByLabelValue(ctx context.Context, labelKey, labelValue string) ([]*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListWorkloadNotEndByKind(ctx context.Context, kind string) ([]*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) ListActiveTopLevelWorkloads(ctx context.Context, startTime, endTime time.Time, namespace string) ([]*dbmodel.GpuWorkload, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) CreateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *dbmodel.GpuWorkloadSnapshot) error {
	return nil
}
func (m *MockWorkloadFacade) UpdateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *dbmodel.GpuWorkloadSnapshot) error {
	return nil
}
func (m *MockWorkloadFacade) GetLatestGpuWorkloadSnapshotByUid(ctx context.Context, uid string, resourceVersion int) (*dbmodel.GpuWorkloadSnapshot, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) CreateWorkloadPodReference(ctx context.Context, workloadUid, podUid string) error {
	return nil
}
func (m *MockWorkloadFacade) ListWorkloadPodReferencesByPodUids(ctx context.Context, podUids []string) ([]*dbmodel.WorkloadPodReference, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx context.Context, workloadUid, nearestWorkloadId, typ string) (*dbmodel.WorkloadEvent, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) CreateWorkloadEvent(ctx context.Context, workloadEvent *dbmodel.WorkloadEvent) error {
	return nil
}
func (m *MockWorkloadFacade) UpdateWorkloadEvent(ctx context.Context, workloadEvent *dbmodel.WorkloadEvent) error {
	return nil
}
func (m *MockWorkloadFacade) GetLatestEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*dbmodel.WorkloadEvent, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) GetLatestOtherWorkloadEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*dbmodel.WorkloadEvent, error) {
	return nil, nil
}
func (m *MockWorkloadFacade) GetAllWorkloadPodReferences(ctx context.Context) ([]*dbmodel.WorkloadPodReference, error) {
	return nil, nil
}

// MockPodFacade implements database.PodFacadeInterface for testing
type MockPodFacade struct {
	ListPodsByUidsFunc func(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error)
}

func (m *MockPodFacade) ListPodsByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error) {
	if m.ListPodsByUidsFunc != nil {
		return m.ListPodsByUidsFunc(ctx, uids)
	}
	return nil, nil
}

// Implement other required methods
func (m *MockPodFacade) WithCluster(clusterName string) database.PodFacadeInterface { return m }
func (m *MockPodFacade) CreateGpuPods(ctx context.Context, gpuPods *dbmodel.GpuPods) error {
	return nil
}
func (m *MockPodFacade) UpdateGpuPods(ctx context.Context, gpuPods *dbmodel.GpuPods) error {
	return nil
}
func (m *MockPodFacade) GetGpuPodsByPodUid(ctx context.Context, podUid string) (*dbmodel.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) GetActiveGpuPodByNodeName(ctx context.Context, nodeName string) ([]*dbmodel.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) GetHistoryGpuPodByNodeName(ctx context.Context, nodeName string, pageNum, pageSize int) ([]*dbmodel.GpuPods, int, error) {
	return nil, 0, nil
}
func (m *MockPodFacade) ListActivePodsByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) ListActiveGpuPods(ctx context.Context) ([]*dbmodel.GpuPods, error) {
	return nil, nil
}
func (m *MockPodFacade) CreateGpuPodsEvent(ctx context.Context, gpuPods *dbmodel.GpuPodsEvent) error {
	return nil
}
func (m *MockPodFacade) UpdateGpuPodsEvent(ctx context.Context, gpuPods *dbmodel.GpuPods) error {
	return nil
}
func (m *MockPodFacade) CreatePodSnapshot(ctx context.Context, podSnapshot *dbmodel.PodSnapshot) error {
	return nil
}
func (m *MockPodFacade) UpdatePodSnapshot(ctx context.Context, podSnapshot *dbmodel.PodSnapshot) error {
	return nil
}
func (m *MockPodFacade) GetLastPodSnapshot(ctx context.Context, podUid string, resourceVersion int) (*dbmodel.PodSnapshot, error) {
	return nil, nil
}
func (m *MockPodFacade) GetPodResourceByUid(ctx context.Context, uid string) (*dbmodel.PodResource, error) {
	return nil, nil
}
func (m *MockPodFacade) CreatePodResource(ctx context.Context, podResource *dbmodel.PodResource) error {
	return nil
}
func (m *MockPodFacade) UpdatePodResource(ctx context.Context, podResource *dbmodel.PodResource) error {
	return nil
}
func (m *MockPodFacade) ListPodResourcesByUids(ctx context.Context, uids []string) ([]*dbmodel.PodResource, error) {
	return nil, nil
}
func (m *MockPodFacade) QueryPodsWithFilters(ctx context.Context, namespace, podName, startTime, endTime string, page, pageSize int) ([]*dbmodel.GpuPods, int64, error) {
	return nil, 0, nil
}
func (m *MockPodFacade) GetAverageGPUUtilizationByNode(ctx context.Context, nodeName string) (float64, error) {
	return 0.0, nil
}
func (m *MockPodFacade) GetLatestGPUMetricsByNode(ctx context.Context, nodeName string) (*dbmodel.GpuDevice, error) {
	return nil, nil
}
func (m *MockPodFacade) QueryGPUHistoryByNode(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]*dbmodel.GpuDevice, error) {
	return nil, nil
}
func (m *MockPodFacade) ListPodEventsByUID(ctx context.Context, podUID string) ([]*dbmodel.GpuPodsEvent, error) {
	return nil, nil
}

// MockGpuAggregationFacade implements database.GpuAggregationFacadeInterface for testing
type MockGpuAggregationFacade struct {
	ListWorkloadHourlyStatsFunc func(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error)
	SaveWorkloadHourlyStatsFunc func(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error
	savedStats                  []*dbmodel.WorkloadGpuHourlyStats
}

func (m *MockGpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	if m.ListWorkloadHourlyStatsFunc != nil {
		return m.ListWorkloadHourlyStatsFunc(ctx, startTime, endTime)
	}
	return nil, nil
}

func (m *MockGpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
	if m.SaveWorkloadHourlyStatsFunc != nil {
		return m.SaveWorkloadHourlyStatsFunc(ctx, stats)
	}
	m.savedStats = append(m.savedStats, stats)
	return nil
}

// Implement other required methods
func (m *MockGpuAggregationFacade) WithCluster(clusterName string) database.GpuAggregationFacadeInterface {
	return m
}
func (m *MockGpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error {
	return nil
}
func (m *MockGpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error {
	return nil
}
func (m *MockGpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error {
	return nil
}
func (m *MockGpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error {
	return nil
}
func (m *MockGpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListNamespaceHourlyStatsPaginatedWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *MockGpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error {
	return nil
}
func (m *MockGpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) LabelHourlyStatsExists(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error) {
	return false, nil
}
func (m *MockGpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error {
	return nil
}
func (m *MockGpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) GetWorkloadHourlyStatsPaginatedWithExclusion(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error {
	return nil
}
func (m *MockGpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error) {
	return nil, nil
}
func (m *MockGpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) {
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

// ==================== Test Cases ====================

func TestWorkloadStatsBackfillConfig_DefaultValues(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{}

	assert.False(t, config.Enabled, "Default Enabled should be false")
	assert.Equal(t, 0, config.BackfillDays, "Default BackfillDays should be 0")
	assert.Equal(t, 0, config.PromQueryStep, "Default PromQueryStep should be 0")
}

func TestWorkloadStatsBackfillJob_GetConfig(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  7,
		PromQueryStep: 30,
	}

	job := &WorkloadStatsBackfillJob{
		config: config,
	}

	result := job.GetConfig()
	assert.Equal(t, config, result, "GetConfig should return the config")
}

func TestWorkloadStatsBackfillJob_SetConfig(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config: &WorkloadStatsBackfillConfig{
			Enabled: false,
		},
	}

	newConfig := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  14,
		PromQueryStep: 60,
	}

	job.SetConfig(newConfig)
	assert.Equal(t, newConfig, job.config, "SetConfig should update the config")
}

func TestWorkloadStatsBackfillJob_Schedule(t *testing.T) {
	job := &WorkloadStatsBackfillJob{}
	assert.Equal(t, "@every 1m", job.Schedule(), "Schedule should return @every 1m")
}

func TestConstants(t *testing.T) {
	assert.Equal(t, 2, DefaultBackfillDays)
	assert.Equal(t, 60, DefaultPromQueryStep)
}

func TestWorkloadUtilizationQueryTemplate(t *testing.T) {
	uid := "test-uid-123"
	expected := `avg(workload_gpu_utilization{workload_uid="test-uid-123"})`
	result := fmt.Sprintf(WorkloadUtilizationQueryTemplate, uid)
	assert.Equal(t, expected, result, "Query template should be formatted correctly")
}

func TestWorkloadGpuMemoryUsedQueryTemplate(t *testing.T) {
	uid := "test-uid-456"
	expected := `avg(workload_gpu_used_vram{workload_uid="test-uid-456"})`
	result := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, uid)
	assert.Equal(t, expected, result, "Query template should be formatted correctly")
}

func TestWorkloadGpuMemoryTotalQueryTemplate(t *testing.T) {
	uid := "test-uid-789"
	expected := `avg(workload_gpu_total_vram{workload_uid="test-uid-789"})`
	result := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, uid)
	assert.Equal(t, expected, result, "Query template should be formatted correctly")
}

func TestWorkloadHourEntry_Structure(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-namespace",
		Kind:      "Deployment",
	}

	testHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	entry := WorkloadHourEntry{
		Workload: workload,
		Hour:     testHour,
	}

	assert.Equal(t, workload, entry.Workload)
	assert.Equal(t, testHour, entry.Hour)
	assert.Equal(t, "test-uid", entry.Workload.UID)
	assert.Equal(t, "test-workload", entry.Workload.Name)
	assert.Equal(t, "test-namespace", entry.Workload.Namespace)
}

// ==================== Tests for Exported Helper Functions ====================

func TestFilterActiveTopLevelWorkloads(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	workloads := []*dbmodel.GpuWorkload{
		{UID: "uid-1", Name: "top-level-active", ParentUID: "", CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Time{}},
		{UID: "uid-2", Name: "child-workload", ParentUID: "uid-1", CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Time{}},
		{UID: "uid-3", Name: "created-after-end", ParentUID: "", CreatedAt: time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC), EndAt: time.Time{}},
		{UID: "uid-4", Name: "ended-before-start", ParentUID: "", CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)},
		{UID: "uid-5", Name: "active-during-range", ParentUID: "", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), EndAt: time.Time{}},
	}

	result := FilterActiveTopLevelWorkloads(workloads, startTime, endTime)

	assert.Equal(t, 2, len(result))
	assert.Equal(t, "uid-1", result[0].UID)
	assert.Equal(t, "uid-5", result[1].UID)
}

func TestBuildExistingStatsMap(t *testing.T) {
	stats := []*dbmodel.WorkloadGpuHourlyStats{
		{Namespace: "ns-1", WorkloadName: "wl-1", StatHour: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
		{Namespace: "ns-1", WorkloadName: "wl-1", StatHour: time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)},
		{Namespace: "ns-2", WorkloadName: "wl-2", StatHour: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
	}

	result := BuildExistingStatsMap(stats)

	assert.Equal(t, 3, len(result))
	_, exists := result["ns-1/wl-1/2025-01-01T10:00:00Z"]
	assert.True(t, exists)
	_, exists = result["ns-1/wl-1/2025-01-01T11:00:00Z"]
	assert.True(t, exists)
	_, exists = result["ns-2/wl-2/2025-01-01T10:00:00Z"]
	assert.True(t, exists)
}

func TestFindMissingEntries(t *testing.T) {
	workloads := []*dbmodel.GpuWorkload{
		{UID: "uid-1", Name: "wl-1", Namespace: "ns-1", CreatedAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC), EndAt: time.Time{}},
	}

	existingStatsMap := map[string]struct{}{
		"ns-1/wl-1/2025-01-01T10:00:00Z": {},
	}

	startTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	result := FindMissingEntries(workloads, existingStatsMap, startTime, endTime)

	assert.Equal(t, 2, len(result))
	assert.Equal(t, time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC), result[0].Hour)
	assert.Equal(t, time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), result[1].Hour)
}

func TestCalculateAverageFromSeries(t *testing.T) {
	tests := []struct {
		name     string
		series   []model.MetricsSeries
		expected float64
	}{
		{
			name:     "empty series",
			series:   []model.MetricsSeries{},
			expected: 0,
		},
		{
			name: "series with no values",
			series: []model.MetricsSeries{
				{Values: []model.TimePoint{}},
			},
			expected: 0,
		},
		{
			name: "series with single value",
			series: []model.MetricsSeries{
				{Values: []model.TimePoint{{Value: 50.0}}},
			},
			expected: 50.0,
		},
		{
			name: "series with multiple values",
			series: []model.MetricsSeries{
				{Values: []model.TimePoint{{Value: 10.0}, {Value: 20.0}, {Value: 30.0}}},
			},
			expected: 20.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateAverageFromSeries(tt.series)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBytesToGB(t *testing.T) {
	tests := []struct {
		name     string
		bytes    float64
		expected float64
	}{
		{"zero bytes", 0, 0},
		{"1 GB", 1024 * 1024 * 1024, 1.0},
		{"2 GB", 2 * 1024 * 1024 * 1024, 2.0},
		{"0.5 GB", 512 * 1024 * 1024, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BytesToGB(tt.bytes)
			assert.InDelta(t, tt.expected, result, 0.001)
		})
	}
}

func TestCountActivePodsInHour(t *testing.T) {
	hourStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	hourEnd := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		pods     []*dbmodel.GpuPods
		expected int32
	}{
		{
			name:     "no pods returns 1",
			pods:     []*dbmodel.GpuPods{},
			expected: 1,
		},
		{
			name: "running pod",
			pods: []*dbmodel.GpuPods{
				{UID: "pod-1", Running: true, CreatedAt: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)},
			},
			expected: 1,
		},
		{
			name: "pod created during hour",
			pods: []*dbmodel.GpuPods{
				{UID: "pod-1", Running: false, CreatedAt: time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)},
			},
			expected: 1,
		},
		{
			name: "pod created after hour end",
			pods: []*dbmodel.GpuPods{
				{UID: "pod-1", Running: false, CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
			},
			expected: 1,
		},
		{
			name: "multiple running pods",
			pods: []*dbmodel.GpuPods{
				{UID: "pod-1", Running: true, CreatedAt: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)},
				{UID: "pod-2", Running: true, CreatedAt: time.Date(2025, 1, 1, 9, 30, 0, 0, time.UTC)},
				{UID: "pod-3", Running: true, CreatedAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)},
			},
			expected: 3,
		},
		{
			name: "pod existed before hour and not deleted",
			pods: []*dbmodel.GpuPods{
				{UID: "pod-1", Running: false, Deleted: false, CreatedAt: time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountActivePodsInHour(tt.pods, hourStart, hourEnd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildWorkloadHourlyStats(t *testing.T) {
	entry := WorkloadHourEntry{
		Workload: &dbmodel.GpuWorkload{
			UID:        "test-uid",
			Name:       "test-workload",
			Namespace:  "test-ns",
			Kind:       "Deployment",
			GpuRequest: 4,
			Status:     "Running",
			ParentUID:  "",
			Labels:     dbmodel.ExtType{"app": "test"},
		},
		Hour: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	stats := BuildWorkloadHourlyStats("test-cluster", entry, 75.5, 8.0, 16.0, 2.0, 2, 2)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, "test-ns", stats.Namespace)
	assert.Equal(t, "test-workload", stats.WorkloadName)
	assert.Equal(t, "Deployment", stats.WorkloadType)
	assert.Equal(t, float64(4), stats.AllocatedGpuCount)
	assert.Equal(t, 75.5, stats.AvgUtilization)
	assert.Equal(t, 8.0, stats.AvgGpuMemoryUsed)
	assert.Equal(t, 16.0, stats.AvgGpuMemoryTotal)
	assert.Equal(t, float64(2), stats.AvgReplicaCount)
	assert.NotNil(t, stats.Labels)
}

func TestBuildWorkloadHourlyStats_NilLabelsAndAnnotations(t *testing.T) {
	entry := WorkloadHourEntry{
		Workload: &dbmodel.GpuWorkload{
			UID:         "test-uid",
			Name:        "test-workload",
			Namespace:   "test-ns",
			Labels:      nil,
			Annotations: nil,
		},
		Hour: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
	}

	stats := BuildWorkloadHourlyStats("test-cluster", entry, 0, 0, 0, 1, 1, 1)

	assert.NotNil(t, stats.Labels)
	assert.NotNil(t, stats.Annotations)
}

// ==================== Integration Tests with Dependency Injection ====================

func TestNewWorkloadStatsBackfillJob_WithOptions(t *testing.T) {
	mockFacadeGetter := func(clusterName string) database.FacadeInterface {
		return &MockFacade{}
	}

	mockPromQueryFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, query string, startTime, endTime time.Time, step int, labelFilter map[string]struct{}) ([]model.MetricsSeries, error) {
		return []model.MetricsSeries{}, nil
	}

	job := NewWorkloadStatsBackfillJob(
		WithFacadeGetter(mockFacadeGetter),
		WithPromQueryFunc(mockPromQueryFunc),
		WithClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "test-cluster", job.clusterName)
	assert.NotNil(t, job.facadeGetter)
	assert.NotNil(t, job.promQueryFunc)
}

func TestNewWorkloadStatsBackfillJobWithConfig_WithOptions(t *testing.T) {
	config := &WorkloadStatsBackfillConfig{
		Enabled:       true,
		BackfillDays:  5,
		PromQueryStep: 30,
	}

	job := NewWorkloadStatsBackfillJobWithConfig(config,
		WithClusterName("custom-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "custom-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 5, job.config.BackfillDays)
	assert.Equal(t, 30, job.config.PromQueryStep)
}

func TestWorkloadStatsBackfillJob_Run_Disabled(t *testing.T) {
	job := NewWorkloadStatsBackfillJob(
		WithClusterName("test-cluster"),
	)
	job.config.Enabled = false

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "Workload stats backfill job is disabled")
}

func TestWorkloadStatsBackfillJob_Run_NoActiveWorkloads(t *testing.T) {
	mockWorkloadFacade := &MockWorkloadFacade{
		GetWorkloadNotEndFunc: func(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
			return []*dbmodel.GpuWorkload{}, nil
		},
	}

	mockFacade := &MockFacade{
		workloadFacade: mockWorkloadFacade,
	}

	job := NewWorkloadStatsBackfillJob(
		WithFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "No active top-level workloads found")
}

func TestWorkloadStatsBackfillJob_Run_GetWorkloadError(t *testing.T) {
	mockWorkloadFacade := &MockWorkloadFacade{
		GetWorkloadNotEndFunc: func(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
			return nil, errors.New("database error")
		},
	}

	mockFacade := &MockFacade{
		workloadFacade: mockWorkloadFacade,
	}

	job := NewWorkloadStatsBackfillJob(
		WithFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterName("test-cluster"),
	)

	ctx := context.Background()
	_, err := job.Run(ctx, nil, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get recently active workloads")
}

func TestWorkloadStatsBackfillJob_Run_NoMissingStats(t *testing.T) {
	now := time.Now()
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-ns",
		ParentUID: "",
		CreatedAt: now.Add(-48 * time.Hour),
		EndAt:     time.Time{},
	}

	mockWorkloadFacade := &MockWorkloadFacade{
		GetWorkloadNotEndFunc: func(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
			return []*dbmodel.GpuWorkload{workload}, nil
		},
	}

	existingStats := make([]*dbmodel.WorkloadGpuHourlyStats, 0)
	endTime := now.Truncate(time.Hour).Add(-time.Hour)
	startTime := endTime.Add(-48 * time.Hour)
	for h := startTime; !h.After(endTime); h = h.Add(time.Hour) {
		existingStats = append(existingStats, &dbmodel.WorkloadGpuHourlyStats{
			Namespace:    "test-ns",
			WorkloadName: "test-workload",
			StatHour:     h,
		})
	}

	mockGpuAggregationFacade := &MockGpuAggregationFacade{
		ListWorkloadHourlyStatsFunc: func(ctx context.Context, st, et time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
			return existingStats, nil
		},
	}

	mockFacade := &MockFacade{
		workloadFacade:       mockWorkloadFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
	}

	job := NewWorkloadStatsBackfillJob(
		WithFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "No missing workload stats found")
}

func TestWorkloadStatsBackfillJob_Run_SuccessfulBackfill(t *testing.T) {
	now := time.Now()
	workload := &dbmodel.GpuWorkload{
		UID:        "test-uid",
		Name:       "test-workload",
		Namespace:  "test-ns",
		Kind:       "Deployment",
		ParentUID:  "",
		GpuRequest: 4,
		Status:     "Running",
		CreatedAt:  now.Add(-2 * time.Hour),
		EndAt:      time.Time{},
		Labels:     dbmodel.ExtType{},
	}

	mockWorkloadFacade := &MockWorkloadFacade{
		GetWorkloadNotEndFunc: func(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
			return []*dbmodel.GpuWorkload{workload}, nil
		},
		ListWorkloadPodReferenceByWorkloadUidFunc: func(ctx context.Context, workloadUID string) ([]*dbmodel.WorkloadPodReference, error) {
			return []*dbmodel.WorkloadPodReference{
				{WorkloadUID: workloadUID, PodUID: "pod-1"},
			}, nil
		},
	}

	mockPodFacade := &MockPodFacade{
		ListPodsByUidsFunc: func(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error) {
			return []*dbmodel.GpuPods{
				{UID: "pod-1", Running: true, CreatedAt: now.Add(-2 * time.Hour)},
			}, nil
		},
	}

	savedStats := make([]*dbmodel.WorkloadGpuHourlyStats, 0)
	mockGpuAggregationFacade := &MockGpuAggregationFacade{
		ListWorkloadHourlyStatsFunc: func(ctx context.Context, st, et time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) {
			return []*dbmodel.WorkloadGpuHourlyStats{}, nil
		},
		SaveWorkloadHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockFacade := &MockFacade{
		workloadFacade:       mockWorkloadFacade,
		podFacade:            mockPodFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
	}

	mockPromQueryFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, query string, startTime, endTime time.Time, step int, labelFilter map[string]struct{}) ([]model.MetricsSeries, error) {
		return []model.MetricsSeries{
			{Values: []model.TimePoint{{Value: 50.0}, {Value: 60.0}}},
		}, nil
	}

	job := NewWorkloadStatsBackfillJob(
		WithFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithPromQueryFunc(mockPromQueryFunc),
		WithClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Greater(t, stats.ItemsCreated, int64(0))
}

// ==================== Edge Case Tests ====================

func TestFilterActiveTopLevelWorkloads_EmptyInput(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 3, 0, 0, 0, 0, time.UTC)

	result := FilterActiveTopLevelWorkloads([]*dbmodel.GpuWorkload{}, startTime, endTime)
	assert.Equal(t, 0, len(result))
}

func TestBuildExistingStatsMap_EmptyInput(t *testing.T) {
	result := BuildExistingStatsMap([]*dbmodel.WorkloadGpuHourlyStats{})
	assert.Equal(t, 0, len(result))
}

func TestFindMissingEntries_NoWorkloads(t *testing.T) {
	result := FindMissingEntries([]*dbmodel.GpuWorkload{}, map[string]struct{}{}, time.Now(), time.Now())
	assert.Equal(t, 0, len(result))
}

func TestCalculateAverageFromSeries_NilInput(t *testing.T) {
	result := CalculateAverageFromSeries(nil)
	assert.Equal(t, float64(0), result)
}

func TestCountActivePodsInHour_EmptyPods(t *testing.T) {
	result := CountActivePodsInHour([]*dbmodel.GpuPods{}, time.Now(), time.Now().Add(time.Hour))
	assert.Equal(t, int32(1), result)
}

// ==================== Option Functions Tests ====================

func TestWithFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterName string) database.FacadeInterface {
		called = true
		return &MockFacade{}
	}

	job := &WorkloadStatsBackfillJob{}
	opt := WithFacadeGetter(mockGetter)
	opt(job)

	job.facadeGetter("test")
	assert.True(t, called)
}

func TestWithPromQueryFunc(t *testing.T) {
	called := false
	mockFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, query string, startTime, endTime time.Time, step int, labelFilter map[string]struct{}) ([]model.MetricsSeries, error) {
		called = true
		return nil, nil
	}

	job := &WorkloadStatsBackfillJob{}
	opt := WithPromQueryFunc(mockFunc)
	opt(job)

	job.promQueryFunc(context.Background(), nil, "", time.Now(), time.Now(), 60, nil)
	assert.True(t, called)
}

func TestWithClusterNameGetter(t *testing.T) {
	mockGetter := func() string {
		return "mock-cluster"
	}

	job := &WorkloadStatsBackfillJob{}
	opt := WithClusterNameGetter(mockGetter)
	opt(job)

	assert.Equal(t, "mock-cluster", job.clusterNameGetter())
}

func TestWithClusterName(t *testing.T) {
	job := &WorkloadStatsBackfillJob{}
	opt := WithClusterName("my-cluster")
	opt(job)

	assert.Equal(t, "my-cluster", job.clusterName)
}

// ==================== Additional Tests for Coverage ====================

func TestTimeRangeCalculation(t *testing.T) {
	now := time.Now()
	endTime := now.Truncate(time.Hour).Add(-time.Hour)
	backfillDays := 2
	startTime := endTime.Add(-time.Duration(backfillDays) * 24 * time.Hour)

	assert.True(t, endTime.Before(now), "End time should be before now")
	assert.True(t, startTime.Before(endTime), "Start time should be before end time")

	expectedDuration := time.Duration(backfillDays) * 24 * time.Hour
	actualDuration := endTime.Sub(startTime)
	assert.Equal(t, expectedDuration, actualDuration, "Duration should match backfill days")
}

func TestHourTruncation(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "already truncated",
			input:    time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "with minutes",
			input:    time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
		{
			name:     "with seconds",
			input:    time.Date(2025, 1, 1, 10, 0, 45, 0, time.UTC),
			expected: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.Truncate(time.Hour)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMissingStatsMapKey(t *testing.T) {
	namespace := "test-ns"
	workloadName := "test-workload"
	statHour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	key := fmt.Sprintf("%s/%s/%s", namespace, workloadName, statHour.Format(time.RFC3339))

	expectedKey := "test-ns/test-workload/2025-01-01T10:00:00Z"
	assert.Equal(t, expectedKey, key)
}

func TestWorkloadStatsBackfillJob_ClusterName(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config:      &WorkloadStatsBackfillConfig{Enabled: true},
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
}

func TestWorkloadStatsBackfillJob_EmptyClusterName(t *testing.T) {
	job := &WorkloadStatsBackfillJob{
		config:      &WorkloadStatsBackfillConfig{Enabled: true},
		clusterName: "",
	}

	assert.Empty(t, job.clusterName)
}

func TestWorkloadLabelsAndAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		labels      dbmodel.ExtType
		annotations dbmodel.ExtType
	}{
		{
			name:        "nil labels and annotations",
			labels:      nil,
			annotations: nil,
		},
		{
			name:        "empty labels and annotations",
			labels:      dbmodel.ExtType{},
			annotations: dbmodel.ExtType{},
		},
		{
			name:        "with labels only",
			labels:      dbmodel.ExtType{"app": "test", "team": "ml"},
			annotations: nil,
		},
		{
			name:        "with annotations only",
			labels:      nil,
			annotations: dbmodel.ExtType{"project": "ml-training"},
		},
		{
			name:        "with both",
			labels:      dbmodel.ExtType{"app": "test"},
			annotations: dbmodel.ExtType{"project": "ml-training"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workload := &dbmodel.GpuWorkload{
				UID:         "test-uid",
				Labels:      tt.labels,
				Annotations: tt.annotations,
			}

			entry := WorkloadHourEntry{
				Workload: workload,
				Hour:     time.Now().Truncate(time.Hour),
			}

			if tt.labels == nil {
				assert.Nil(t, entry.Workload.Labels)
			} else {
				assert.NotNil(t, entry.Workload.Labels)
			}

			if tt.annotations == nil {
				assert.Nil(t, entry.Workload.Annotations)
			} else {
				assert.NotNil(t, entry.Workload.Annotations)
			}
		})
	}
}

func TestDefaultConstants_Values(t *testing.T) {
	assert.Equal(t, 2, DefaultBackfillDays, "Default backfill days should be 2")
	assert.Equal(t, 60, DefaultPromQueryStep, "Default prom query step should be 60")
}

func TestQueryTemplates_AllFormats(t *testing.T) {
	uid := "test-workload-uid-12345"

	templates := []struct {
		name     string
		template string
		metric   string
	}{
		{"utilization", WorkloadUtilizationQueryTemplate, "workload_gpu_utilization"},
		{"memory_used", WorkloadGpuMemoryUsedQueryTemplate, "workload_gpu_used_vram"},
		{"memory_total", WorkloadGpuMemoryTotalQueryTemplate, "workload_gpu_total_vram"},
	}

	for _, tt := range templates {
		t.Run(tt.name, func(t *testing.T) {
			query := fmt.Sprintf(tt.template, uid)
			assert.Contains(t, query, "avg(")
			assert.Contains(t, query, tt.metric)
			assert.Contains(t, query, uid)
			assert.Contains(t, query, "workload_uid=")
		})
	}
}

func TestWorkloadStatsBackfillJob_NilConfig(t *testing.T) {
	job := &WorkloadStatsBackfillJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

func TestWorkloadStatsBackfillConfig_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      *WorkloadStatsBackfillConfig
		isValidDays bool
		isValidStep bool
	}{
		{
			name: "valid config",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  DefaultBackfillDays,
				PromQueryStep: DefaultPromQueryStep,
			},
			isValidDays: true,
			isValidStep: true,
		},
		{
			name: "zero days",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  0,
				PromQueryStep: 60,
			},
			isValidDays: false,
			isValidStep: true,
		},
		{
			name: "negative days",
			config: &WorkloadStatsBackfillConfig{
				Enabled:       true,
				BackfillDays:  -1,
				PromQueryStep: 60,
			},
			isValidDays: false,
			isValidStep: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isValidDays, tt.config.BackfillDays > 0)
			assert.Equal(t, tt.isValidStep, tt.config.PromQueryStep > 0)
		})
	}
}
