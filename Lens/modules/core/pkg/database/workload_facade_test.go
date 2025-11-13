package database

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockWorkloadFacade creates a WorkloadFacade with the test database
type mockWorkloadFacade struct {
	WorkloadFacade
	db *gorm.DB
}

func (f *mockWorkloadFacade) getDB() *gorm.DB {
	return f.db
}

// newTestWorkloadFacade creates a test WorkloadFacade
func newTestWorkloadFacade(db *gorm.DB) WorkloadFacadeInterface {
	return &mockWorkloadFacade{
		db: db,
	}
}

// ==================== GpuWorkload Tests ====================

func TestWorkloadFacade_CreateGpuWorkload(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workload := &model.GpuWorkload{
		UID:       "wl-001",
		Name:      "test-workload",
		Namespace: "default",
		Kind:      "PyTorchJob",
		Status:    "Running",
	}
	
	err := facade.CreateGpuWorkload(ctx, workload)
	require.NoError(t, err)
	assert.NotZero(t, workload.ID)
}

func TestWorkloadFacade_GetGpuWorkloadByUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workload := &model.GpuWorkload{
		UID:       "wl-002",
		Name:      "test-workload-2",
		Namespace: "default",
		Kind:      "PyTorchJob",
		Status:    "Running",
	}
	err := facade.CreateGpuWorkload(ctx, workload)
	require.NoError(t, err)
	
	result, err := facade.GetGpuWorkloadByUid(ctx, "wl-002")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, workload.UID, result.UID)
	assert.Equal(t, workload.Name, result.Name)
}

func TestWorkloadFacade_UpdateGpuWorkload(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workload := &model.GpuWorkload{
		UID:       "wl-003",
		Name:      "test-workload-3",
		Namespace: "default",
		Kind:      "PyTorchJob",
		Status:    "Running",
	}
	err := facade.CreateGpuWorkload(ctx, workload)
	require.NoError(t, err)
	
	// Update workload
	workload.Status = "Succeeded"
	workload.EndAt = time.Now()
	err = facade.UpdateGpuWorkload(ctx, workload)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetGpuWorkloadByUid(ctx, "wl-003")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Succeeded", result.Status)
	assert.NotNil(t, result.EndAt)
}

func TestWorkloadFacade_QueryWorkload(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create test workloads
	workloads := []*model.GpuWorkload{
		{UID: "q-1", Name: "job-1", Namespace: "ns-1", Kind: "PyTorchJob", Status: "Running", ParentUID: ""},
		{UID: "q-2", Name: "job-2", Namespace: "ns-1", Kind: "TFJob", Status: "Succeeded", ParentUID: ""},
		{UID: "q-3", Name: "task-1", Namespace: "ns-2", Kind: "PyTorchJob", Status: "Running", ParentUID: "q-1"},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	tests := []struct {
		name          string
		filter        *filter.WorkloadFilter
		expectedCount int
	}{
		{
			name:          "No filter (only top-level)",
			filter:        &filter.WorkloadFilter{},
			expectedCount: 2,
		},
		{
			name: "Filter by kind",
			filter: &filter.WorkloadFilter{
				Kind: stringPtr("PyTorchJob"),
			},
			expectedCount: 1,
		},
		{
			name: "Filter by namespace",
			filter: &filter.WorkloadFilter{
				Namespace: stringPtr("ns-1"),
			},
			expectedCount: 2,
		},
		{
			name: "Filter by status",
			filter: &filter.WorkloadFilter{
				Status: stringPtr("Running"),
			},
			expectedCount: 1,
		},
		{
			name: "Filter by name pattern",
			filter: &filter.WorkloadFilter{
				Name: stringPtr("job"),
			},
			expectedCount: 2,
		},
		{
			name: "With pagination",
			filter: &filter.WorkloadFilter{
				Limit:  1,
				Offset: 0,
			},
			expectedCount: 1,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := facade.QueryWorkload(ctx, tt.filter)
			require.NoError(t, err)
			assert.Len(t, results, tt.expectedCount)
			assert.Equal(t, 2, total) // Total top-level workloads
		})
	}
}

func TestWorkloadFacade_GetWorkloadsNamespaceList(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloads := []*model.GpuWorkload{
		{UID: "ns-1", Name: "job-1", Namespace: "namespace-a", Kind: "PyTorchJob", Status: "Running"},
		{UID: "ns-2", Name: "job-2", Namespace: "namespace-b", Kind: "TFJob", Status: "Running"},
		{UID: "ns-3", Name: "job-3", Namespace: "namespace-a", Kind: "PyTorchJob", Status: "Running"},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	namespaces, err := facade.GetWorkloadsNamespaceList(ctx)
	require.NoError(t, err)
	
	assert.Len(t, namespaces, 2)
	assert.Contains(t, namespaces, "namespace-a")
	assert.Contains(t, namespaces, "namespace-b")
}

func TestWorkloadFacade_GetWorkloadKindList(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloads := []*model.GpuWorkload{
		{UID: "k-1", Name: "job-1", Namespace: "default", Kind: "PyTorchJob", Status: "Running", ParentUID: ""},
		{UID: "k-2", Name: "job-2", Namespace: "default", Kind: "TFJob", Status: "Running", ParentUID: ""},
		{UID: "k-3", Name: "job-3", Namespace: "default", Kind: "PyTorchJob", Status: "Running", ParentUID: ""},
		{UID: "k-4", Name: "task-1", Namespace: "default", Kind: "MPIJob", Status: "Running", ParentUID: "k-1"},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	kinds, err := facade.GetWorkloadKindList(ctx)
	require.NoError(t, err)
	
	assert.Len(t, kinds, 2) // Only top-level kinds
	assert.Contains(t, kinds, "PyTorchJob")
	assert.Contains(t, kinds, "TFJob")
}

func TestWorkloadFacade_GetWorkloadNotEnd(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloads := []*model.GpuWorkload{
		{UID: "ne-1", Name: "running-1", Namespace: "default", Kind: "PyTorchJob", Status: "Running"},
		{UID: "ne-2", Name: "running-2", Namespace: "default", Kind: "PyTorchJob", Status: "Running"},
		{UID: "ne-3", Name: "completed", Namespace: "default", Kind: "PyTorchJob", Status: "Succeeded", EndAt: time.Now()},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	results, err := facade.GetWorkloadNotEnd(ctx)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.True(t, r.EndAt.IsZero())
	}
}

func TestWorkloadFacade_ListRunningWorkload(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloads := []*model.GpuWorkload{
		{UID: "run-1", Name: "job-1", Namespace: "default", Kind: "PyTorchJob", Status: "Running"},
		{UID: "run-2", Name: "job-2", Namespace: "default", Kind: "PyTorchJob", Status: "Running"},
		{UID: "run-3", Name: "job-3", Namespace: "default", Kind: "PyTorchJob", Status: "Succeeded"},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	results, err := facade.ListRunningWorkload(ctx)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestWorkloadFacade_ListWorkloadsByUids(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	uids := []string{"uid-1", "uid-2", "uid-3"}
	for _, uid := range uids {
		workload := &model.GpuWorkload{
			UID:       uid,
			Name:      "workload-" + uid,
			Namespace: "default",
			Kind:      "PyTorchJob",
			Status:    "Running",
		}
		err := facade.CreateGpuWorkload(ctx, workload)
		require.NoError(t, err)
	}
	
	results, err := facade.ListWorkloadsByUids(ctx, uids)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestWorkloadFacade_ListTopLevelWorkloadByUids(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloads := []*model.GpuWorkload{
		{UID: "top-1", Name: "parent-1", Namespace: "default", Kind: "PyTorchJob", Status: "Running", ParentUID: ""},
		{UID: "top-2", Name: "parent-2", Namespace: "default", Kind: "TFJob", Status: "Running", ParentUID: ""},
		{UID: "child-1", Name: "child-1", Namespace: "default", Kind: "Pod", Status: "Running", ParentUID: "top-1"},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	results, err := facade.ListTopLevelWorkloadByUids(ctx, []string{"top-1", "top-2", "child-1"})
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.Empty(t, r.ParentUID)
	}
}

func TestWorkloadFacade_ListChildrenWorkloadByParentUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	parentUID := "parent-123"
	workloads := []*model.GpuWorkload{
		{UID: parentUID, Name: "parent", Namespace: "default", Kind: "PyTorchJob", Status: "Running", ParentUID: ""},
		{UID: "child-1", Name: "child-1", Namespace: "default", Kind: "Pod", Status: "Running", ParentUID: parentUID},
		{UID: "child-2", Name: "child-2", Namespace: "default", Kind: "Pod", Status: "Running", ParentUID: parentUID},
		{UID: "other", Name: "other", Namespace: "default", Kind: "TFJob", Status: "Running", ParentUID: ""},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	results, err := facade.ListChildrenWorkloadByParentUid(ctx, parentUID)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.Equal(t, parentUID, r.ParentUID)
	}
}

func TestWorkloadFacade_ListWorkloadNotEndByKind(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloads := []*model.GpuWorkload{
		{UID: "kind-1", Name: "pytorch-1", Namespace: "default", Kind: "PyTorchJob", Status: "Running"},
		{UID: "kind-2", Name: "pytorch-2", Namespace: "default", Kind: "PyTorchJob", Status: "Running"},
		{UID: "kind-3", Name: "pytorch-3", Namespace: "default", Kind: "PyTorchJob", Status: "Succeeded", EndAt: time.Now()},
		{UID: "kind-4", Name: "tf-1", Namespace: "default", Kind: "TFJob", Status: "Running"},
	}
	
	for _, wl := range workloads {
		err := facade.CreateGpuWorkload(ctx, wl)
		require.NoError(t, err)
	}
	
	results, err := facade.ListWorkloadNotEndByKind(ctx, "PyTorchJob")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.Equal(t, "PyTorchJob", r.Kind)
		assert.True(t, r.EndAt.IsZero())
	}
}

// ==================== GpuWorkloadSnapshot Tests ====================

func TestWorkloadFacade_CreateGpuWorkloadSnapshot(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	snapshot := &model.GpuWorkloadSnapshot{
		UID:             "wl-snap-001",
		ResourceVersion: 100,
		Metadata:        model.ExtType{"labels": map[string]string{"app": "test"}},
		Detail:          model.ExtType{"phase": "Running"},
	}
	
	err := facade.CreateGpuWorkloadSnapshot(ctx, snapshot)
	require.NoError(t, err)
	assert.NotZero(t, snapshot.ID)
}

func TestWorkloadFacade_GetLatestGpuWorkloadSnapshotByUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	uid := "wl-snap-test"
	
	// Create multiple snapshots
	for i := 1; i <= 5; i++ {
		snapshot := &model.GpuWorkloadSnapshot{
			UID:             uid,
			ResourceVersion: int32(i * 10),
			Metadata:        model.ExtType{},
			Detail:          model.ExtType{"version": i},
		}
		err := facade.CreateGpuWorkloadSnapshot(ctx, snapshot)
		require.NoError(t, err)
	}
	
	// Get latest before version 40
	result, err := facade.GetLatestGpuWorkloadSnapshotByUid(ctx, uid, 40)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, int32(30), result.ResourceVersion)
}

// ==================== WorkloadPodReference Tests ====================

func TestWorkloadFacade_CreateWorkloadPodReference(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	err := facade.CreateWorkloadPodReference(ctx, "workload-001", "pod-001")
	require.NoError(t, err)
}

func TestWorkloadFacade_ListWorkloadPodReferencesByPodUids(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create references
	refs := []struct {
		workloadUID string
		podUID      string
	}{
		{"wl-1", "pod-1"},
		{"wl-1", "pod-2"},
		{"wl-2", "pod-3"},
	}
	
	for _, ref := range refs {
		err := facade.CreateWorkloadPodReference(ctx, ref.workloadUID, ref.podUID)
		require.NoError(t, err)
	}
	
	results, err := facade.ListWorkloadPodReferencesByPodUids(ctx, []string{"pod-1", "pod-2"})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestWorkloadFacade_ListWorkloadPodReferenceByWorkloadUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloadUID := "wl-ref-test"
	podUIDs := []string{"pod-a", "pod-b", "pod-c"}
	
	for _, podUID := range podUIDs {
		err := facade.CreateWorkloadPodReference(ctx, workloadUID, podUID)
		require.NoError(t, err)
	}
	
	results, err := facade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	
	for _, r := range results {
		assert.Equal(t, workloadUID, r.WorkloadUID)
	}
}

// ==================== WorkloadEvent Tests ====================

func TestWorkloadFacade_CreateWorkloadEvent(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	event := &model.WorkloadEvent{
		WorkloadUID:        "wl-001",
		NearestWorkloadUID: "wl-001",
		Type:               "Started",
	}
	
	err := facade.CreateWorkloadEvent(ctx, event)
	require.NoError(t, err)
	assert.NotZero(t, event.ID)
}

func TestWorkloadFacade_GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	event := &model.WorkloadEvent{
		WorkloadUID:        "wl-002",
		NearestWorkloadUID: "wl-002-child",
		Type:               "PodScheduled",
	}
	err := facade.CreateWorkloadEvent(ctx, event)
	require.NoError(t, err)
	
	result, err := facade.GetWorkloadEventByWorkloadUidAndNearestWorkloadIdAndType(ctx, "wl-002", "wl-002-child", "PodScheduled")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, event.WorkloadUID, result.WorkloadUID)
	assert.Equal(t, event.Type, result.Type)
}

func TestWorkloadFacade_GetLatestEvent(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	workloadUID := "wl-latest"
	nearestUID := "wl-latest-child"
	
	// Create events at different times
	for i := 1; i <= 3; i++ {
		event := &model.WorkloadEvent{
			WorkloadUID:        workloadUID,
			NearestWorkloadUID: nearestUID,
			Type:               "Event-" + string(rune('A'+i)),
		}
		err := facade.CreateWorkloadEvent(ctx, event)
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}
	
	result, err := facade.GetLatestEvent(ctx, workloadUID, nearestUID)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, "Event-"+string(rune('A'+3)), result.Type)
}

// ==================== Helper Methods ====================

func TestWorkloadFacade_WithCluster(t *testing.T) {
	facade := NewWorkloadFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*WorkloadFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkWorkloadFacade_CreateGpuWorkload(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		workload := &model.GpuWorkload{
			UID:       "bench-wl",
			Name:      "bench-workload",
			Namespace: "default",
			Kind:      "PyTorchJob",
			Status:    "Running",
		}
		_ = facade.CreateGpuWorkload(ctx, workload)
	}
}

func BenchmarkWorkloadFacade_QueryWorkload(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 50; i++ {
		workload := &model.GpuWorkload{
			UID:       string(rune('a' + i%26)),
			Name:      "workload-" + string(rune('a'+i%26)),
			Namespace: "default",
			Kind:      "PyTorchJob",
			Status:    "Running",
		}
		_ = facade.CreateGpuWorkload(ctx, workload)
	}
	
	filter := &filter.WorkloadFilter{Limit: 10}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.QueryWorkload(ctx, filter)
	}
}

func BenchmarkWorkloadFacade_ListRunningWorkload(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestWorkloadFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 30; i++ {
		workload := &model.GpuWorkload{
			UID:       string(rune('a' + i)),
			Name:      "workload-" + string(rune('a'+i)),
			Namespace: "default",
			Kind:      "PyTorchJob",
			Status:    "Running",
		}
		_ = facade.CreateGpuWorkload(ctx, workload)
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.ListRunningWorkload(ctx)
	}
}

