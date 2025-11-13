package database

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	corev1 "k8s.io/api/core/v1"
)

// mockPodFacade creates a PodFacade with the test database
type mockPodFacade struct {
	PodFacade
	db *gorm.DB
}

func (f *mockPodFacade) getDB() *gorm.DB {
	return f.db
}

// newTestPodFacade creates a test PodFacade
func newTestPodFacade(db *gorm.DB) PodFacadeInterface {
	return &mockPodFacade{
		db: db,
	}
}

// ==================== GpuPods Tests ====================

func TestPodFacade_CreateGpuPods(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	pod := &model.GpuPods{
		UID:       "pod-uid-001",
		Name:      "test-pod",
		Namespace: "default",
		NodeName:  "node-1",
		Phase:     string(corev1.PodRunning),
		Running:   true,
	}
	
	err := facade.CreateGpuPods(ctx, pod)
	require.NoError(t, err)
	assert.NotZero(t, pod.ID)
}

func TestPodFacade_GetGpuPodsByPodUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	pod := &model.GpuPods{
		UID:       "pod-uid-002",
		Name:      "test-pod-2",
		Namespace: "default",
		NodeName:  "node-1",
		Phase:     string(corev1.PodRunning),
		Running:   true,
	}
	err := facade.CreateGpuPods(ctx, pod)
	require.NoError(t, err)
	
	result, err := facade.GetGpuPodsByPodUid(ctx, "pod-uid-002")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, pod.UID, result.UID)
	assert.Equal(t, pod.Name, result.Name)
}

func TestPodFacade_GetGpuPodsByPodUid_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetGpuPodsByPodUid(ctx, "non-existent")
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestPodFacade_UpdateGpuPods(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	pod := &model.GpuPods{
		UID:       "pod-uid-003",
		Name:      "test-pod-3",
		Namespace: "default",
		NodeName:  "node-1",
		Phase:     string(corev1.PodRunning),
		Running:   true,
	}
	err := facade.CreateGpuPods(ctx, pod)
	require.NoError(t, err)
	
	// Update pod
	pod.Phase = string(corev1.PodSucceeded)
	pod.Running = false
	err = facade.UpdateGpuPods(ctx, pod)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetGpuPodsByPodUid(ctx, "pod-uid-003")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, string(corev1.PodSucceeded), result.Phase)
	assert.False(t, result.Running)
}

func TestPodFacade_GetActiveGpuPodByNodeName(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create active and inactive pods
	pods := []*model.GpuPods{
		{UID: "pod-1", Name: "pod-1", Namespace: "default", NodeName: "node-1", Phase: string(corev1.PodRunning), Running: true},
		{UID: "pod-2", Name: "pod-2", Namespace: "default", NodeName: "node-1", Phase: string(corev1.PodRunning), Running: true},
		{UID: "pod-3", Name: "pod-3", Namespace: "default", NodeName: "node-1", Phase: string(corev1.PodSucceeded), Running: false},
		{UID: "pod-4", Name: "pod-4", Namespace: "default", NodeName: "node-2", Phase: string(corev1.PodRunning), Running: true},
	}
	
	for _, p := range pods {
		err := facade.CreateGpuPods(ctx, p)
		require.NoError(t, err)
	}
	
	results, err := facade.GetActiveGpuPodByNodeName(ctx, "node-1")
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.True(t, r.Running)
		assert.Equal(t, "node-1", r.NodeName)
	}
}

func TestPodFacade_GetHistoryGpuPodByNodeName(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create history pods
	for i := 0; i < 5; i++ {
		pod := &model.GpuPods{
			UID:       string(rune('a' + i)),
			Name:      "history-pod",
			Namespace: "default",
			NodeName:  "node-1",
			Phase:     string(corev1.PodSucceeded),
			Running:   false,
		}
		err := facade.CreateGpuPods(ctx, pod)
		require.NoError(t, err)
	}
	
	// Get with pagination
	results, count, err := facade.GetHistoryGpuPodByNodeName(ctx, "node-1", 1, 3)
	require.NoError(t, err)
	assert.Len(t, results, 3)
	assert.Equal(t, 5, count)
}

func TestPodFacade_ListActivePodsByUids(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	uids := []string{"active-1", "active-2", "inactive-1"}
	pods := []*model.GpuPods{
		{UID: "active-1", Name: "pod-1", Namespace: "default", NodeName: "node-1", Running: true, Phase: string(corev1.PodRunning)},
		{UID: "active-2", Name: "pod-2", Namespace: "default", NodeName: "node-1", Running: true, Phase: string(corev1.PodRunning)},
		{UID: "inactive-1", Name: "pod-3", Namespace: "default", NodeName: "node-1", Running: false, Phase: string(corev1.PodSucceeded)},
	}
	
	for _, p := range pods {
		err := facade.CreateGpuPods(ctx, p)
		require.NoError(t, err)
	}
	
	results, err := facade.ListActivePodsByUids(ctx, uids)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	
	for _, r := range results {
		assert.True(t, r.Running)
	}
}

func TestPodFacade_ListPodsByUids(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	uids := []string{"list-1", "list-2", "list-3"}
	for _, uid := range uids {
		pod := &model.GpuPods{
			UID:       uid,
			Name:      "pod-" + uid,
			Namespace: "default",
			NodeName:  "node-1",
			Phase:     string(corev1.PodRunning),
			Running:   true,
		}
		err := facade.CreateGpuPods(ctx, pod)
		require.NoError(t, err)
	}
	
	results, err := facade.ListPodsByUids(ctx, uids)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestPodFacade_ListActiveGpuPods(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create mix of active and inactive pods
	pods := []*model.GpuPods{
		{UID: "active-a", Name: "pod-a", Namespace: "default", NodeName: "node-1", Running: true, Phase: string(corev1.PodRunning)},
		{UID: "active-b", Name: "pod-b", Namespace: "default", NodeName: "node-1", Running: true, Phase: string(corev1.PodRunning)},
		{UID: "inactive-c", Name: "pod-c", Namespace: "default", NodeName: "node-1", Running: false, Phase: string(corev1.PodSucceeded)},
	}
	
	for _, p := range pods {
		err := facade.CreateGpuPods(ctx, p)
		require.NoError(t, err)
	}
	
	results, err := facade.ListActiveGpuPods(ctx)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

// ==================== GpuPodsEvent Tests ====================

func TestPodFacade_CreateGpuPodsEvent(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	event := &model.GpuPodsEvent{
		PodUUID:   "pod-uid-001",
		EventType: "Started",
		PodPhase:  string(corev1.PodRunning),
	}
	
	err := facade.CreateGpuPodsEvent(ctx, event)
	require.NoError(t, err)
	assert.NotZero(t, event.ID)
}

// ==================== PodSnapshot Tests ====================

func TestPodFacade_CreatePodSnapshot(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	snapshot := &model.PodSnapshot{
		PodUID:          "pod-uid-001",
		ResourceVersion: 100,
		Spec:            model.ExtType{"containers": []string{"app"}},
		Status:          model.ExtType{"phase": "Running"},
	}
	
	err := facade.CreatePodSnapshot(ctx, snapshot)
	require.NoError(t, err)
	assert.NotZero(t, snapshot.ID)
}

func TestPodFacade_UpdatePodSnapshot(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	snapshot := &model.PodSnapshot{
		PodUID:          "pod-uid-002",
		ResourceVersion: 100,
		Spec:            model.ExtType{},
		Status:          model.ExtType{"phase": "Running"},
	}
	err := facade.CreatePodSnapshot(ctx, snapshot)
	require.NoError(t, err)
	
	snapshot.ResourceVersion = 101
	snapshot.Status = model.ExtType{"phase": "Succeeded"}
	err = facade.UpdatePodSnapshot(ctx, snapshot)
	require.NoError(t, err)
}

func TestPodFacade_GetLastPodSnapshot(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	podUID := "pod-snapshot-test"
	
	// Create multiple snapshots with different versions
	for i := 1; i <= 5; i++ {
		snapshot := &model.PodSnapshot{
			PodUID:          podUID,
			ResourceVersion: int32(i * 10),
			Spec:            model.ExtType{},
			Status:          model.ExtType{"version": i},
		}
		err := facade.CreatePodSnapshot(ctx, snapshot)
		require.NoError(t, err)
	}
	
	// Get last snapshot before version 40
	result, err := facade.GetLastPodSnapshot(ctx, podUID, 40)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, int32(30), result.ResourceVersion)
}

// ==================== PodResource Tests ====================

func TestPodFacade_CreatePodResource(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	resource := &model.PodResource{
		UID:          "pod-res-001",
		GpuModel:     "MI250X",
		GpuAllocated: 8,
	}
	
	err := facade.CreatePodResource(ctx, resource)
	require.NoError(t, err)
	assert.NotZero(t, resource.ID)
}

func TestPodFacade_GetPodResourceByUid(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	resource := &model.PodResource{
		UID:          "pod-res-002",
		GpuModel:     "MI250X",
		GpuAllocated: 4,
	}
	err := facade.CreatePodResource(ctx, resource)
	require.NoError(t, err)
	
	result, err := facade.GetPodResourceByUid(ctx, "pod-res-002")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, resource.UID, result.UID)
	assert.Equal(t, resource.GpuAllocated, result.GpuAllocated)
}

func TestPodFacade_UpdatePodResource(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	resource := &model.PodResource{
		UID:          "pod-res-003",
		GpuModel:     "MI250X",
		GpuAllocated: 4,
	}
	err := facade.CreatePodResource(ctx, resource)
	require.NoError(t, err)
	
	// Update resource
	resource.GpuModel = "MI300X"
	resource.GpuAllocated = 8
	err = facade.UpdatePodResource(ctx, resource)
	require.NoError(t, err)
	
	// Verify update
	result, err := facade.GetPodResourceByUid(ctx, "pod-res-003")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "MI300X", result.GpuModel)
	assert.Equal(t, int32(8), result.GpuAllocated)
}

// ==================== Helper Methods ====================

func TestPodFacade_WithCluster(t *testing.T) {
	facade := NewPodFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*PodFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkPodFacade_CreateGpuPods(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		pod := &model.GpuPods{
			UID:       "bench-pod",
			Name:      "bench-pod",
			Namespace: "default",
			NodeName:  "node-1",
			Phase:     string(corev1.PodRunning),
			Running:   true,
		}
		_ = facade.CreateGpuPods(ctx, pod)
	}
}

func BenchmarkPodFacade_GetGpuPodsByPodUid(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	pod := &model.GpuPods{
		UID:       "bench-get-pod",
		Name:      "bench-pod",
		Namespace: "default",
		NodeName:  "node-1",
		Phase:     string(corev1.PodRunning),
		Running:   true,
	}
	_ = facade.CreateGpuPods(ctx, pod)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.GetGpuPodsByPodUid(ctx, "bench-get-pod")
	}
}

func BenchmarkPodFacade_ListActiveGpuPods(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestPodFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	for i := 0; i < 50; i++ {
		pod := &model.GpuPods{
			UID:       string(rune('a' + i%26)),
			Name:      "bench-pod",
			Namespace: "default",
			NodeName:  "node-1",
			Phase:     string(corev1.PodRunning),
			Running:   true,
		}
		_ = facade.CreateGpuPods(ctx, pod)
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.ListActiveGpuPods(ctx)
	}
}

