package gpu_aggregation

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

// WorkloadMockFacade implements database.FacadeInterface for testing
type WorkloadMockFacade struct {
	workloadFacade       database.WorkloadFacadeInterface
	podFacade            database.PodFacadeInterface
	gpuAggregationFacade database.GpuAggregationFacadeInterface
	genericCacheFacade   database.GenericCacheFacadeInterface
}

func (m *WorkloadMockFacade) GetWorkload() database.WorkloadFacadeInterface {
	return m.workloadFacade
}

func (m *WorkloadMockFacade) GetPod() database.PodFacadeInterface {
	return m.podFacade
}

func (m *WorkloadMockFacade) GetGpuAggregation() database.GpuAggregationFacadeInterface {
	return m.gpuAggregationFacade
}

func (m *WorkloadMockFacade) GetGenericCache() database.GenericCacheFacadeInterface {
	return m.genericCacheFacade
}

// Implement other methods with nil returns (not used in tests)
func (m *WorkloadMockFacade) GetNode() database.NodeFacadeInterface                               { return nil }
func (m *WorkloadMockFacade) GetContainer() database.ContainerFacadeInterface                     { return nil }
func (m *WorkloadMockFacade) GetTraining() database.TrainingFacadeInterface                       { return nil }
func (m *WorkloadMockFacade) GetStorage() database.StorageFacadeInterface                         { return nil }
func (m *WorkloadMockFacade) GetAlert() database.AlertFacadeInterface                             { return nil }
func (m *WorkloadMockFacade) GetMetricAlertRule() database.MetricAlertRuleFacadeInterface         { return nil }
func (m *WorkloadMockFacade) GetLogAlertRule() database.LogAlertRuleFacadeInterface               { return nil }
func (m *WorkloadMockFacade) GetAlertRuleAdvice() database.AlertRuleAdviceFacadeInterface         { return nil }
func (m *WorkloadMockFacade) GetClusterOverviewCache() database.ClusterOverviewCacheFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetSystemConfig() database.SystemConfigFacadeInterface               { return nil }
func (m *WorkloadMockFacade) GetJobExecutionHistory() database.JobExecutionHistoryFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetNamespaceInfo() database.NamespaceInfoFacadeInterface             { return nil }
func (m *WorkloadMockFacade) GetWorkloadStatistic() database.WorkloadStatisticFacadeInterface     { return nil }
func (m *WorkloadMockFacade) GetAiWorkloadMetadata() database.AiWorkloadMetadataFacadeInterface   { return nil }
func (m *WorkloadMockFacade) GetCheckpointEvent() database.CheckpointEventFacadeInterface         { return nil }
func (m *WorkloadMockFacade) GetDetectionConflictLog() database.DetectionConflictLogFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetGpuUsageWeeklyReport() database.GpuUsageWeeklyReportFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetNodeNamespaceMapping() database.NodeNamespaceMappingFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetTraceLensSession() database.TraceLensSessionFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetK8sService() database.K8sServiceFacadeInterface             { return nil }
func (m *WorkloadMockFacade) GetWorkloadDetection() database.WorkloadDetectionFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetWorkloadDetectionEvidence() database.WorkloadDetectionEvidenceFacadeInterface { return nil }
func (m *WorkloadMockFacade) GetDetectionCoverage() database.DetectionCoverageFacadeInterface { return nil }
func (m *WorkloadMockFacade) WithCluster(clusterName string) database.FacadeInterface       { return m }

// WorkloadMockWorkloadFacade implements database.WorkloadFacadeInterface for testing
type WorkloadMockWorkloadFacade struct {
	GetWorkloadNotEndFunc                     func(ctx context.Context) ([]*dbmodel.GpuWorkload, error)
	ListWorkloadPodReferenceByWorkloadUidFunc func(ctx context.Context, workloadUID string) ([]*dbmodel.WorkloadPodReference, error)
}

func (m *WorkloadMockWorkloadFacade) GetWorkloadNotEnd(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
	if m.GetWorkloadNotEndFunc != nil {
		return m.GetWorkloadNotEndFunc(ctx)
	}
	return nil, nil
}

func (m *WorkloadMockWorkloadFacade) ListWorkloadPodReferenceByWorkloadUid(ctx context.Context, workloadUID string) ([]*dbmodel.WorkloadPodReference, error) {
	if m.ListWorkloadPodReferenceByWorkloadUidFunc != nil {
		return m.ListWorkloadPodReferenceByWorkloadUidFunc(ctx, workloadUID)
	}
	return nil, nil
}

// Implement other required methods
func (m *WorkloadMockWorkloadFacade) WithCluster(clusterName string) database.WorkloadFacadeInterface { return m }
func (m *WorkloadMockWorkloadFacade) GetGpuWorkloadByUid(ctx context.Context, uid string) (*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) CreateGpuWorkload(ctx context.Context, gpuWorkload *dbmodel.GpuWorkload) error { return nil }
func (m *WorkloadMockWorkloadFacade) UpdateGpuWorkload(ctx context.Context, gpuWorkload *dbmodel.GpuWorkload) error { return nil }
func (m *WorkloadMockWorkloadFacade) QueryWorkload(ctx context.Context, f *filter.WorkloadFilter) ([]*dbmodel.GpuWorkload, int, error) { return nil, 0, nil }
func (m *WorkloadMockWorkloadFacade) GetWorkloadsNamespaceList(ctx context.Context) ([]string, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) GetWorkloadKindList(ctx context.Context) ([]string, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) ListRunningWorkload(ctx context.Context) ([]*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) ListWorkloadsByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) GetNearestWorkloadByPodUid(ctx context.Context, podUid string) (*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) ListTopLevelWorkloadByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) ListChildrenWorkloadByParentUid(ctx context.Context, parentUid string) ([]*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) ListWorkloadByLabelValue(ctx context.Context, labelKey, labelValue string) ([]*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) ListWorkloadNotEndByKind(ctx context.Context, kind string) ([]*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) ListActiveTopLevelWorkloads(ctx context.Context, startTime, endTime time.Time, namespace string) ([]*dbmodel.GpuWorkload, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) CreateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *dbmodel.GpuWorkloadSnapshot) error { return nil }
func (m *WorkloadMockWorkloadFacade) UpdateGpuWorkloadSnapshot(ctx context.Context, gpuWorkloadSnapshot *dbmodel.GpuWorkloadSnapshot) error { return nil }
func (m *WorkloadMockWorkloadFacade) GetLatestGpuWorkloadSnapshotByUid(ctx context.Context, uid string, resourceVersion int) (*dbmodel.GpuWorkloadSnapshot, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) CreateWorkloadPodReference(ctx context.Context, workloadUid, podUid string) error { return nil }
func (m *WorkloadMockWorkloadFacade) ListWorkloadPodReferencesByPodUids(ctx context.Context, podUids []string) ([]*dbmodel.WorkloadPodReference, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx context.Context, workloadUid, nearestWorkloadId, typ string) (*dbmodel.WorkloadEvent, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) CreateWorkloadEvent(ctx context.Context, workloadEvent *dbmodel.WorkloadEvent) error { return nil }
func (m *WorkloadMockWorkloadFacade) UpdateWorkloadEvent(ctx context.Context, workloadEvent *dbmodel.WorkloadEvent) error { return nil }
func (m *WorkloadMockWorkloadFacade) GetLatestEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*dbmodel.WorkloadEvent, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) GetLatestOtherWorkloadEvent(ctx context.Context, workloadUid, nearestWorkloadId string) (*dbmodel.WorkloadEvent, error) { return nil, nil }
func (m *WorkloadMockWorkloadFacade) GetAllWorkloadPodReferences(ctx context.Context) ([]*dbmodel.WorkloadPodReference, error) { return nil, nil }

// WorkloadMockPodFacade implements database.PodFacadeInterface for testing
type WorkloadMockPodFacade struct {
	ListPodsByUidsFunc func(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error)
}

func (m *WorkloadMockPodFacade) ListPodsByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error) {
	if m.ListPodsByUidsFunc != nil {
		return m.ListPodsByUidsFunc(ctx, uids)
	}
	return nil, nil
}

// Implement other required methods
func (m *WorkloadMockPodFacade) WithCluster(clusterName string) database.PodFacadeInterface { return m }
func (m *WorkloadMockPodFacade) CreateGpuPods(ctx context.Context, gpuPods *dbmodel.GpuPods) error { return nil }
func (m *WorkloadMockPodFacade) UpdateGpuPods(ctx context.Context, gpuPods *dbmodel.GpuPods) error { return nil }
func (m *WorkloadMockPodFacade) GetGpuPodsByPodUid(ctx context.Context, podUid string) (*dbmodel.GpuPods, error) { return nil, nil }
func (m *WorkloadMockPodFacade) GetActiveGpuPodByNodeName(ctx context.Context, nodeName string) ([]*dbmodel.GpuPods, error) { return nil, nil }
func (m *WorkloadMockPodFacade) GetHistoryGpuPodByNodeName(ctx context.Context, nodeName string, pageNum, pageSize int) ([]*dbmodel.GpuPods, int, error) { return nil, 0, nil }
func (m *WorkloadMockPodFacade) ListActivePodsByUids(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error) { return nil, nil }
func (m *WorkloadMockPodFacade) ListActiveGpuPods(ctx context.Context) ([]*dbmodel.GpuPods, error) { return nil, nil }
func (m *WorkloadMockPodFacade) CreateGpuPodsEvent(ctx context.Context, gpuPods *dbmodel.GpuPodsEvent) error { return nil }
func (m *WorkloadMockPodFacade) UpdateGpuPodsEvent(ctx context.Context, gpuPods *dbmodel.GpuPods) error { return nil }
func (m *WorkloadMockPodFacade) CreatePodSnapshot(ctx context.Context, podSnapshot *dbmodel.PodSnapshot) error { return nil }
func (m *WorkloadMockPodFacade) UpdatePodSnapshot(ctx context.Context, podSnapshot *dbmodel.PodSnapshot) error { return nil }
func (m *WorkloadMockPodFacade) GetLastPodSnapshot(ctx context.Context, podUid string, resourceVersion int) (*dbmodel.PodSnapshot, error) { return nil, nil }
func (m *WorkloadMockPodFacade) GetPodResourceByUid(ctx context.Context, uid string) (*dbmodel.PodResource, error) { return nil, nil }
func (m *WorkloadMockPodFacade) CreatePodResource(ctx context.Context, podResource *dbmodel.PodResource) error { return nil }
func (m *WorkloadMockPodFacade) UpdatePodResource(ctx context.Context, podResource *dbmodel.PodResource) error { return nil }
func (m *WorkloadMockPodFacade) ListPodResourcesByUids(ctx context.Context, uids []string) ([]*dbmodel.PodResource, error) { return nil, nil }
func (m *WorkloadMockPodFacade) QueryPodsWithFilters(ctx context.Context, namespace, podName, startTime, endTime string, page, pageSize int) ([]*dbmodel.GpuPods, int64, error) { return nil, 0, nil }
func (m *WorkloadMockPodFacade) GetAverageGPUUtilizationByNode(ctx context.Context, nodeName string) (float64, error) { return 0.0, nil }
func (m *WorkloadMockPodFacade) GetLatestGPUMetricsByNode(ctx context.Context, nodeName string) (*dbmodel.GpuDevice, error) { return nil, nil }
func (m *WorkloadMockPodFacade) QueryGPUHistoryByNode(ctx context.Context, nodeName string, startTime, endTime time.Time) ([]*dbmodel.GpuDevice, error) { return nil, nil }
func (m *WorkloadMockPodFacade) ListPodEventsByUID(ctx context.Context, podUID string) ([]*dbmodel.GpuPodsEvent, error) { return nil, nil }

// WorkloadMockGpuAggregationFacade implements database.GpuAggregationFacadeInterface for testing
type WorkloadMockGpuAggregationFacade struct {
	SaveWorkloadHourlyStatsFunc func(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error
	savedStats                  []*dbmodel.WorkloadGpuHourlyStats
}

func (m *WorkloadMockGpuAggregationFacade) SaveWorkloadHourlyStats(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
	if m.SaveWorkloadHourlyStatsFunc != nil {
		return m.SaveWorkloadHourlyStatsFunc(ctx, stats)
	}
	m.savedStats = append(m.savedStats, stats)
	return nil
}

// Implement other required methods
func (m *WorkloadMockGpuAggregationFacade) WithCluster(clusterName string) database.GpuAggregationFacadeInterface { return m }
func (m *WorkloadMockGpuAggregationFacade) SaveClusterHourlyStats(ctx context.Context, stats *dbmodel.ClusterGpuHourlyStats) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) BatchSaveClusterHourlyStats(ctx context.Context, stats []*dbmodel.ClusterGpuHourlyStats) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) GetClusterHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.ClusterGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetClusterHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) SaveNamespaceHourlyStats(ctx context.Context, stats *dbmodel.NamespaceGpuHourlyStats) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) BatchSaveNamespaceHourlyStats(ctx context.Context, stats []*dbmodel.NamespaceGpuHourlyStats) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) GetNamespaceHourlyStats(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListNamespaceHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.NamespaceGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetNamespaceHourlyStatsPaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListNamespaceHourlyStatsPaginatedWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) SaveLabelHourlyStats(ctx context.Context, stats *dbmodel.LabelGpuHourlyStats) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) BatchSaveLabelHourlyStats(ctx context.Context, stats []*dbmodel.LabelGpuHourlyStats) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) GetLabelHourlyStats(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListLabelHourlyStatsByKey(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]*dbmodel.LabelGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetLabelHourlyStatsPaginated(ctx context.Context, dimensionType, dimensionKey, dimensionValue string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListLabelHourlyStatsByKeyPaginated(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) LabelHourlyStatsExists(ctx context.Context, clusterName, dimensionType, dimensionKey, dimensionValue string, hour time.Time) (bool, error) { return false, nil }
func (m *WorkloadMockGpuAggregationFacade) BatchSaveWorkloadHourlyStats(ctx context.Context, stats []*dbmodel.WorkloadGpuHourlyStats) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) GetWorkloadHourlyStats(ctx context.Context, namespace, workloadName string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListWorkloadHourlyStats(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespace(ctx context.Context, namespace string, startTime, endTime time.Time) ([]*dbmodel.WorkloadGpuHourlyStats, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginated(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetWorkloadHourlyStatsPaginatedWithExclusion(ctx context.Context, namespace, workloadName, workloadType string, startTime, endTime time.Time, excludeNamespaces []string, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListWorkloadHourlyStatsPaginated(ctx context.Context, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListWorkloadHourlyStatsByNamespacePaginated(ctx context.Context, namespace string, startTime, endTime time.Time, opts database.PaginationOptions) (*database.PaginatedResult, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) SaveSnapshot(ctx context.Context, snapshot *dbmodel.GpuAllocationSnapshots) error { return nil }
func (m *WorkloadMockGpuAggregationFacade) GetLatestSnapshot(ctx context.Context) (*dbmodel.GpuAllocationSnapshots, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) ListSnapshots(ctx context.Context, startTime, endTime time.Time) ([]*dbmodel.GpuAllocationSnapshots, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) CleanupOldSnapshots(ctx context.Context, beforeTime time.Time) (int64, error) { return 0, nil }
func (m *WorkloadMockGpuAggregationFacade) CleanupOldHourlyStats(ctx context.Context, beforeTime time.Time) (int64, error) { return 0, nil }
func (m *WorkloadMockGpuAggregationFacade) GetDistinctNamespaces(ctx context.Context, startTime, endTime time.Time) ([]string, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetDistinctNamespacesWithExclusion(ctx context.Context, startTime, endTime time.Time, excludeNamespaces []string) ([]string, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetDistinctDimensionKeys(ctx context.Context, dimensionType string, startTime, endTime time.Time) ([]string, error) { return nil, nil }
func (m *WorkloadMockGpuAggregationFacade) GetDistinctDimensionValues(ctx context.Context, dimensionType, dimensionKey string, startTime, endTime time.Time) ([]string, error) { return nil, nil }

// WorkloadMockGenericCacheFacade implements database.GenericCacheFacadeInterface for testing
type WorkloadMockGenericCacheFacade struct {
	GetFunc func(ctx context.Context, key string, value interface{}) error
	SetFunc func(ctx context.Context, key string, value interface{}, expiration *time.Time) error
}

func (m *WorkloadMockGenericCacheFacade) Get(ctx context.Context, key string, value interface{}) error {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, key, value)
	}
	return nil
}

func (m *WorkloadMockGenericCacheFacade) Set(ctx context.Context, key string, value interface{}, expiration *time.Time) error {
	if m.SetFunc != nil {
		return m.SetFunc(ctx, key, value, expiration)
	}
	return nil
}

func (m *WorkloadMockGenericCacheFacade) WithCluster(clusterName string) database.GenericCacheFacadeInterface { return m }
func (m *WorkloadMockGenericCacheFacade) Delete(ctx context.Context, key string) error { return nil }
func (m *WorkloadMockGenericCacheFacade) Exists(ctx context.Context, key string) (bool, error) { return false, nil }
func (m *WorkloadMockGenericCacheFacade) DeleteExpired(ctx context.Context) error { return nil }

// ==================== Test Cases ====================

func TestNewWorkloadGpuAggregationJob_Default(t *testing.T) {
	job := NewWorkloadGpuAggregationJob(
		WithWorkloadClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.NotNil(t, job.config)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, DefaultWorkloadPromQueryStep, job.config.PromQueryStep)
	assert.Equal(t, "test-cluster", job.clusterName)
}

func TestNewWorkloadGpuAggregationJob_WithOptions(t *testing.T) {
	mockFacadeGetter := func(clusterName string) database.FacadeInterface {
		return &WorkloadMockFacade{}
	}

	mockPromQueryFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, query string, startTime, endTime time.Time, step int, labelFilter map[string]struct{}) ([]model.MetricsSeries, error) {
		return []model.MetricsSeries{}, nil
	}

	job := NewWorkloadGpuAggregationJob(
		WithWorkloadFacadeGetter(mockFacadeGetter),
		WithWorkloadPromQueryFunc(mockPromQueryFunc),
		WithWorkloadClusterName("test-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "test-cluster", job.clusterName)
	assert.NotNil(t, job.facadeGetter)
	assert.NotNil(t, job.promQueryFunc)
}

func TestNewWorkloadGpuAggregationJobWithConfig_WithOptions(t *testing.T) {
	config := &WorkloadGpuAggregationConfig{
		Enabled:       true,
		PromQueryStep: 30,
	}

	job := NewWorkloadGpuAggregationJobWithConfig(config,
		WithWorkloadClusterName("custom-cluster"),
	)

	assert.NotNil(t, job)
	assert.Equal(t, "custom-cluster", job.clusterName)
	assert.True(t, job.config.Enabled)
	assert.Equal(t, 30, job.config.PromQueryStep)
}

func TestWorkloadGpuAggregationJob_Run_Disabled(t *testing.T) {
	job := NewWorkloadGpuAggregationJob(
		WithWorkloadClusterName("test-cluster"),
	)
	job.config.Enabled = false

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Contains(t, stats.Messages, "Workload GPU aggregation job is disabled")
}

func TestWorkloadGpuAggregationJob_GetConfig_SetConfig(t *testing.T) {
	job := NewWorkloadGpuAggregationJob(WithWorkloadClusterName("test"))

	config := job.GetConfig()
	assert.NotNil(t, config)
	assert.True(t, config.Enabled)

	newConfig := &WorkloadGpuAggregationConfig{
		Enabled:       false,
		PromQueryStep: 120,
	}
	job.SetConfig(newConfig)

	assert.False(t, job.GetConfig().Enabled)
	assert.Equal(t, 120, job.GetConfig().PromQueryStep)
}

func TestWorkloadGpuAggregationJob_ScheduleValue(t *testing.T) {
	job := NewWorkloadGpuAggregationJob(WithWorkloadClusterName("test"))
	assert.Equal(t, "@every 5m", job.Schedule())
}

// ==================== Tests for Exported Helper Functions ====================

func TestFilterWorkloadActiveTopLevel(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 1, 0, 0, 0, time.UTC)

	workloads := []*dbmodel.GpuWorkload{
		{UID: "uid-1", Name: "top-level-active", ParentUID: "", CreatedAt: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), EndAt: time.Time{}},
		{UID: "uid-2", Name: "child-workload", ParentUID: "uid-1", CreatedAt: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC), EndAt: time.Time{}},
		{UID: "uid-3", Name: "created-after-end", ParentUID: "", CreatedAt: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), EndAt: time.Time{}},
		{UID: "uid-4", Name: "ended-before-start", ParentUID: "", CreatedAt: time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC), EndAt: time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)},
		{UID: "uid-5", Name: "active-during", ParentUID: "", CreatedAt: time.Date(2025, 1, 1, 0, 30, 0, 0, time.UTC), EndAt: time.Time{}},
	}

	result := FilterWorkloadActiveTopLevel(workloads, startTime, endTime)

	assert.Equal(t, 2, len(result))
	assert.Equal(t, "uid-1", result[0].UID)
	assert.Equal(t, "uid-5", result[1].UID)
}

func TestFilterWorkloadActiveTopLevel_EmptyInput(t *testing.T) {
	result := FilterWorkloadActiveTopLevel([]*dbmodel.GpuWorkload{}, time.Now(), time.Now())
	assert.Equal(t, 0, len(result))
}

func TestExtractValuesFromSeries(t *testing.T) {
	tests := []struct {
		name     string
		series   []model.MetricsSeries
		expected []float64
	}{
		{
			name:     "empty series",
			series:   []model.MetricsSeries{},
			expected: []float64{},
		},
		{
			name: "series with no values",
			series: []model.MetricsSeries{
				{Values: []model.TimePoint{}},
			},
			expected: []float64{},
		},
		{
			name: "series with values",
			series: []model.MetricsSeries{
				{Values: []model.TimePoint{
					{Value: 10.0},
					{Value: 20.0},
					{Value: 30.0},
				}},
			},
			expected: []float64{10.0, 20.0, 30.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractValuesFromSeries(tt.series)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateMemoryStats(t *testing.T) {
	tests := []struct {
		name        string
		series      []model.MetricsSeries
		expectedAvg float64
		expectedMax float64
	}{
		{
			name:        "empty series",
			series:      []model.MetricsSeries{},
			expectedAvg: 0,
			expectedMax: 0,
		},
		{
			name: "single value - 1GB",
			series: []model.MetricsSeries{
				{Values: []model.TimePoint{{Value: 1024 * 1024 * 1024}}},
			},
			expectedAvg: 1.0,
			expectedMax: 1.0,
		},
		{
			name: "multiple values",
			series: []model.MetricsSeries{
				{Values: []model.TimePoint{
					{Value: 1024 * 1024 * 1024},     // 1 GB
					{Value: 2 * 1024 * 1024 * 1024}, // 2 GB
					{Value: 3 * 1024 * 1024 * 1024}, // 3 GB
				}},
			},
			expectedAvg: 2.0, // (1+2+3)/3
			expectedMax: 3.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			avgGB, maxGB := CalculateMemoryStats(tt.series)
			assert.InDelta(t, tt.expectedAvg, avgGB, 0.001)
			assert.InDelta(t, tt.expectedMax, maxGB, 0.001)
		})
	}
}

func TestCountWorkloadActivePodsInHour(t *testing.T) {
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
			name: "multiple running pods",
			pods: []*dbmodel.GpuPods{
				{UID: "pod-1", Running: true, CreatedAt: time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)},
				{UID: "pod-2", Running: true, CreatedAt: time.Date(2025, 1, 1, 9, 30, 0, 0, time.UTC)},
			},
			expected: 2,
		},
		{
			name: "pod created after hour end",
			pods: []*dbmodel.GpuPods{
				{UID: "pod-1", Running: false, CreatedAt: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
			},
			expected: 1,
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
			result := CountWorkloadActivePodsInHour(tt.pods, hourStart, hourEnd)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculatePercentile_Exported(t *testing.T) {
	tests := []struct {
		name       string
		values     []float64
		percentile float64
		expected   float64
	}{
		{
			name:       "empty values",
			values:     []float64{},
			percentile: 0.5,
			expected:   0,
		},
		{
			name:       "single value",
			values:     []float64{50.0},
			percentile: 0.5,
			expected:   50.0,
		},
		{
			name:       "percentile 0",
			values:     []float64{10.0, 20.0, 30.0},
			percentile: 0,
			expected:   10.0,
		},
		{
			name:       "percentile 1",
			values:     []float64{10.0, 20.0, 30.0},
			percentile: 1,
			expected:   30.0,
		},
		{
			name:       "percentile 0.5",
			values:     []float64{10.0, 20.0, 30.0, 40.0},
			percentile: 0.5,
			expected:   20.0,
		},
		{
			name:       "percentile 0.95",
			values:     []float64{10.0, 20.0, 30.0, 40.0, 50.0, 60.0, 70.0, 80.0, 90.0, 100.0},
			percentile: 0.95,
			expected:   100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculatePercentile(tt.values, tt.percentile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildWorkloadGpuHourlyStats(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:        "test-uid",
		Name:       "test-workload",
		Namespace:  "test-ns",
		Kind:       "Deployment",
		GpuRequest: 4,
		Status:     "Running",
		ParentUID:  "",
		Labels:     dbmodel.ExtType{"app": "test"},
	}
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	utilizationValues := []float64{50.0, 60.0, 70.0, 80.0}

	stats := BuildWorkloadGpuHourlyStats("test-cluster", workload, hour, utilizationValues,
		8.0, 10.0, 16.0, 2.0, 2, 2)

	assert.Equal(t, "test-cluster", stats.ClusterName)
	assert.Equal(t, "test-ns", stats.Namespace)
	assert.Equal(t, "test-workload", stats.WorkloadName)
	assert.Equal(t, "Deployment", stats.WorkloadType)
	assert.Equal(t, float64(4), stats.AllocatedGpuCount)
	assert.Equal(t, 8.0, stats.AvgGpuMemoryUsed)
	assert.Equal(t, 10.0, stats.MaxGpuMemoryUsed)
	assert.Equal(t, 16.0, stats.AvgGpuMemoryTotal)
	assert.Equal(t, int32(4), stats.SampleCount)
	assert.NotNil(t, stats.Labels)

	// Check utilization statistics
	assert.Equal(t, 50.0, stats.MinUtilization)
	assert.Equal(t, 80.0, stats.MaxUtilization)
	assert.Equal(t, 65.0, stats.AvgUtilization) // (50+60+70+80)/4
}

func TestBuildWorkloadGpuHourlyStats_NilLabelsAndAnnotations(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:         "test-uid",
		Name:        "test-workload",
		Namespace:   "test-ns",
		Labels:      nil,
		Annotations: nil,
	}
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	stats := BuildWorkloadGpuHourlyStats("test-cluster", workload, hour, []float64{},
		0, 0, 0, 1, 1, 1)

	assert.NotNil(t, stats.Labels)
	assert.NotNil(t, stats.Annotations)
}

func TestBuildWorkloadGpuHourlyStats_EmptyUtilization(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-ns",
	}
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	stats := BuildWorkloadGpuHourlyStats("test-cluster", workload, hour, []float64{},
		0, 0, 0, 1, 1, 1)

	assert.Equal(t, float64(0), stats.AvgUtilization)
	assert.Equal(t, float64(0), stats.MinUtilization)
	assert.Equal(t, float64(0), stats.MaxUtilization)
	assert.Equal(t, int32(0), stats.SampleCount)
}

// ==================== Integration Tests with Dependency Injection ====================

func TestWorkloadGpuAggregationJob_Run_NoActiveWorkloads(t *testing.T) {
	mockCacheFacade := &WorkloadMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			return errors.New("no cache entry")
		},
	}

	mockWorkloadFacade := &WorkloadMockWorkloadFacade{
		GetWorkloadNotEndFunc: func(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
			return []*dbmodel.GpuWorkload{}, nil
		},
	}

	mockFacade := &WorkloadMockFacade{
		workloadFacade:     mockWorkloadFacade,
		genericCacheFacade: mockCacheFacade,
	}

	job := NewWorkloadGpuAggregationJob(
		WithWorkloadFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithWorkloadClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
}

func TestWorkloadGpuAggregationJob_Run_SuccessfulAggregation(t *testing.T) {
	now := time.Now()
	previousHour := now.Truncate(time.Hour).Add(-time.Hour)
	twoHoursAgo := previousHour.Add(-time.Hour)

	workload := &dbmodel.GpuWorkload{
		UID:        "test-uid",
		Name:       "test-workload",
		Namespace:  "test-ns",
		Kind:       "Deployment",
		ParentUID:  "",
		GpuRequest: 4,
		Status:     "Running",
		CreatedAt:  now.Add(-24 * time.Hour),
		EndAt:      time.Time{},
		Labels:     dbmodel.ExtType{},
	}

	mockCacheFacade := &WorkloadMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			// Return a time from 2 hours ago to trigger aggregation
			if v, ok := value.(*string); ok {
				*v = twoHoursAgo.Format(time.RFC3339)
			}
			return nil
		},
		SetFunc: func(ctx context.Context, key string, value interface{}, expiration *time.Time) error {
			return nil
		},
	}

	mockWorkloadFacade := &WorkloadMockWorkloadFacade{
		GetWorkloadNotEndFunc: func(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
			return []*dbmodel.GpuWorkload{workload}, nil
		},
		ListWorkloadPodReferenceByWorkloadUidFunc: func(ctx context.Context, workloadUID string) ([]*dbmodel.WorkloadPodReference, error) {
			return []*dbmodel.WorkloadPodReference{
				{WorkloadUID: workloadUID, PodUID: "pod-1"},
			}, nil
		},
	}

	mockPodFacade := &WorkloadMockPodFacade{
		ListPodsByUidsFunc: func(ctx context.Context, uids []string) ([]*dbmodel.GpuPods, error) {
			return []*dbmodel.GpuPods{
				{UID: "pod-1", Running: true, CreatedAt: now.Add(-24 * time.Hour)},
			}, nil
		},
	}

	savedStats := make([]*dbmodel.WorkloadGpuHourlyStats, 0)
	mockGpuAggregationFacade := &WorkloadMockGpuAggregationFacade{
		SaveWorkloadHourlyStatsFunc: func(ctx context.Context, stats *dbmodel.WorkloadGpuHourlyStats) error {
			savedStats = append(savedStats, stats)
			return nil
		},
	}

	mockFacade := &WorkloadMockFacade{
		workloadFacade:       mockWorkloadFacade,
		podFacade:            mockPodFacade,
		gpuAggregationFacade: mockGpuAggregationFacade,
		genericCacheFacade:   mockCacheFacade,
	}

	mockPromQueryFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, query string, startTime, endTime time.Time, step int, labelFilter map[string]struct{}) ([]model.MetricsSeries, error) {
		return []model.MetricsSeries{
			{Values: []model.TimePoint{{Value: 50.0}, {Value: 60.0}}},
		}, nil
	}

	job := NewWorkloadGpuAggregationJob(
		WithWorkloadFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithWorkloadPromQueryFunc(mockPromQueryFunc),
		WithWorkloadClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ItemsCreated)
}

func TestWorkloadGpuAggregationJob_Run_GetWorkloadError(t *testing.T) {
	mockCacheFacade := &WorkloadMockGenericCacheFacade{
		GetFunc: func(ctx context.Context, key string, value interface{}) error {
			return errors.New("no cache entry")
		},
	}

	mockWorkloadFacade := &WorkloadMockWorkloadFacade{
		GetWorkloadNotEndFunc: func(ctx context.Context) ([]*dbmodel.GpuWorkload, error) {
			return nil, errors.New("database error")
		},
	}

	mockFacade := &WorkloadMockFacade{
		workloadFacade:     mockWorkloadFacade,
		genericCacheFacade: mockCacheFacade,
	}

	job := NewWorkloadGpuAggregationJob(
		WithWorkloadFacadeGetter(func(clusterName string) database.FacadeInterface {
			return mockFacade
		}),
		WithWorkloadClusterName("test-cluster"),
	)

	ctx := context.Background()
	stats, err := job.Run(ctx, nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.ErrorCount)
}

// ==================== Option Functions Tests ====================

func TestWithWorkloadFacadeGetter(t *testing.T) {
	called := false
	mockGetter := func(clusterName string) database.FacadeInterface {
		called = true
		return &WorkloadMockFacade{}
	}

	job := &WorkloadGpuAggregationJob{}
	opt := WithWorkloadFacadeGetter(mockGetter)
	opt(job)

	job.facadeGetter("test")
	assert.True(t, called)
}

func TestWithWorkloadPromQueryFunc(t *testing.T) {
	called := false
	mockFunc := func(ctx context.Context, storageClientSet *clientsets.StorageClientSet, query string, startTime, endTime time.Time, step int, labelFilter map[string]struct{}) ([]model.MetricsSeries, error) {
		called = true
		return nil, nil
	}

	job := &WorkloadGpuAggregationJob{}
	opt := WithWorkloadPromQueryFunc(mockFunc)
	opt(job)

	job.promQueryFunc(context.Background(), nil, "", time.Now(), time.Now(), 60, nil)
	assert.True(t, called)
}

func TestWithWorkloadClusterNameGetter(t *testing.T) {
	mockGetter := func() string {
		return "mock-cluster"
	}

	job := &WorkloadGpuAggregationJob{}
	opt := WithWorkloadClusterNameGetter(mockGetter)
	opt(job)

	assert.Equal(t, "mock-cluster", job.clusterNameGetter())
}

func TestWithWorkloadClusterName(t *testing.T) {
	job := &WorkloadGpuAggregationJob{}
	opt := WithWorkloadClusterName("my-cluster")
	opt(job)

	assert.Equal(t, "my-cluster", job.clusterName)
}

// ==================== Constants Tests ====================

func TestWorkloadConstants(t *testing.T) {
	assert.Equal(t, 60, DefaultWorkloadPromQueryStep)
	assert.Equal(t, "job.workload_gpu_aggregation.last_processed_hour", CacheKeyWorkloadGpuAggregationLastHour)
}

func TestWorkloadQueryTemplates(t *testing.T) {
	uid := "test-uid-123"

	utilQuery := fmt.Sprintf(WorkloadUtilizationQueryTemplate, uid)
	assert.Contains(t, utilQuery, "workload_gpu_utilization")
	assert.Contains(t, utilQuery, uid)

	memUsedQuery := fmt.Sprintf(WorkloadGpuMemoryUsedQueryTemplate, uid)
	assert.Contains(t, memUsedQuery, "workload_gpu_used_vram")
	assert.Contains(t, memUsedQuery, uid)

	memTotalQuery := fmt.Sprintf(WorkloadGpuMemoryTotalQueryTemplate, uid)
	assert.Contains(t, memTotalQuery, "workload_gpu_total_vram")
	assert.Contains(t, memTotalQuery, uid)
}

// ==================== Config Tests ====================

func TestWorkloadGpuAggregationConfig_Defaults(t *testing.T) {
	config := &WorkloadGpuAggregationConfig{}

	assert.False(t, config.Enabled)
	assert.Equal(t, 0, config.PromQueryStep)
}

func TestWorkloadGpuAggregationConfig_WithValues(t *testing.T) {
	config := &WorkloadGpuAggregationConfig{
		Enabled:       true,
		PromQueryStep: 30,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, 30, config.PromQueryStep)
}

// ==================== Edge Case Tests ====================

func TestFilterWorkloadActiveTopLevel_AllChildWorkloads(t *testing.T) {
	workloads := []*dbmodel.GpuWorkload{
		{UID: "uid-1", ParentUID: "parent-1"},
		{UID: "uid-2", ParentUID: "parent-2"},
		{UID: "uid-3", ParentUID: "parent-3"},
	}

	result := FilterWorkloadActiveTopLevel(workloads, time.Now(), time.Now().Add(time.Hour))
	assert.Equal(t, 0, len(result))
}

func TestExtractValuesFromSeries_NilInput(t *testing.T) {
	result := ExtractValuesFromSeries(nil)
	assert.Equal(t, []float64{}, result)
}

func TestCalculateMemoryStats_NilInput(t *testing.T) {
	avgGB, maxGB := CalculateMemoryStats(nil)
	assert.Equal(t, float64(0), avgGB)
	assert.Equal(t, float64(0), maxGB)
}

func TestCountWorkloadActivePodsInHour_NilPods(t *testing.T) {
	result := CountWorkloadActivePodsInHour(nil, time.Now(), time.Now().Add(time.Hour))
	assert.Equal(t, int32(1), result)
}

func TestCalculatePercentile_SingleValueCase(t *testing.T) {
	values := []float64{42.0}
	
	assert.Equal(t, 42.0, CalculatePercentile(values, 0))
	assert.Equal(t, 42.0, CalculatePercentile(values, 0.5))
	assert.Equal(t, 42.0, CalculatePercentile(values, 1))
}

func TestCalculatePercentile_TwoValuesCase(t *testing.T) {
	values := []float64{10.0, 20.0}
	
	assert.Equal(t, 10.0, CalculatePercentile(values, 0))
	assert.Equal(t, 10.0, CalculatePercentile(values, 0.5))
	assert.Equal(t, 20.0, CalculatePercentile(values, 1))
}

// ==================== Time Range Tests ====================

func TestTimeRangeForAggregation(t *testing.T) {
	now := time.Now()
	currentHour := now.Truncate(time.Hour)
	previousHour := currentHour.Add(-time.Hour)

	assert.True(t, previousHour.Before(currentHour))
	assert.Equal(t, time.Hour, currentHour.Sub(previousHour))
}

func TestHourTruncation(t *testing.T) {
	input := time.Date(2025, 1, 1, 10, 30, 45, 123, time.UTC)
	expected := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	result := input.Truncate(time.Hour)
	assert.Equal(t, expected, result)
}

// ==================== Additional Coverage Tests ====================

func TestWorkloadGpuAggregationJob_NilConfig(t *testing.T) {
	job := &WorkloadGpuAggregationJob{config: nil}
	assert.Nil(t, job.GetConfig())
}

func TestBuildWorkloadGpuHourlyStats_SingleUtilizationValue(t *testing.T) {
	workload := &dbmodel.GpuWorkload{
		UID:       "test-uid",
		Name:      "test-workload",
		Namespace: "test-ns",
	}
	hour := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	stats := BuildWorkloadGpuHourlyStats("test-cluster", workload, hour, []float64{75.0},
		0, 0, 0, 1, 1, 1)

	assert.Equal(t, 75.0, stats.AvgUtilization)
	assert.Equal(t, 75.0, stats.MinUtilization)
	assert.Equal(t, 75.0, stats.MaxUtilization)
	assert.Equal(t, 75.0, stats.P50Utilization)
	assert.Equal(t, 75.0, stats.P95Utilization)
}

func TestWorkloadGpuAggregationJob_ClusterName(t *testing.T) {
	job := &WorkloadGpuAggregationJob{
		config:      &WorkloadGpuAggregationConfig{Enabled: true},
		clusterName: "test-cluster",
	}

	assert.Equal(t, "test-cluster", job.clusterName)
}

func TestCountWorkloadActivePodsInHour_PodCreatedDuringHour(t *testing.T) {
	hourStart := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	hourEnd := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)

	pods := []*dbmodel.GpuPods{
		{UID: "pod-1", Running: false, CreatedAt: time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)},
	}

	result := CountWorkloadActivePodsInHour(pods, hourStart, hourEnd)
	assert.Equal(t, int32(1), result)
}

func TestFilterWorkloadActiveTopLevel_WorkloadEndedExactlyAtStartTime(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)

	workloads := []*dbmodel.GpuWorkload{
		{
			UID:       "uid-1",
			ParentUID: "",
			CreatedAt: time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC),
			EndAt:     startTime, // Ended exactly at start time
		},
	}

	result := FilterWorkloadActiveTopLevel(workloads, startTime, endTime)
	assert.Equal(t, 1, len(result)) // Should be included since EndAt is not before startTime
}

func TestFilterWorkloadActiveTopLevel_WorkloadCreatedExactlyAtEndTime(t *testing.T) {
	startTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)

	workloads := []*dbmodel.GpuWorkload{
		{
			UID:       "uid-1",
			ParentUID: "",
			CreatedAt: endTime, // Created exactly at end time
			EndAt:     time.Time{},
		},
	}

	result := FilterWorkloadActiveTopLevel(workloads, startTime, endTime)
	assert.Equal(t, 1, len(result)) // Should be included since CreatedAt is not after endTime
}

