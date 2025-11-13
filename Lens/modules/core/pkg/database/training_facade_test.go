package database

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockTrainingFacade creates a TrainingFacade with the test database
type mockTrainingFacade struct {
	TrainingFacade
	db *gorm.DB
}

func (f *mockTrainingFacade) getDB() *gorm.DB {
	return f.db
}

// newTestTrainingFacade creates a test TrainingFacade
func newTestTrainingFacade(db *gorm.DB) TrainingFacadeInterface {
	return &mockTrainingFacade{
		db: db,
	}
}

// ==================== TrainingPerformance Tests ====================

func TestTrainingFacade_CreateTrainingPerformance(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	perf := &model.TrainingPerformance{
		WorkloadUID: "workload-001",
		Serial:      1,
		Iteration:   100,
		Performance: model.ExtType{
			"tflops":     150.5,
			"throughput": 1000.0,
			"loss":       0.5,
		},
	}

	err := facade.CreateTrainingPerformance(ctx, perf)
	require.NoError(t, err)
	assert.NotZero(t, perf.ID)
}

func TestTrainingFacade_GetTrainingPerformanceByWorkloadIdSerialAndIteration(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	perf := &model.TrainingPerformance{
		WorkloadUID: "workload-002",
		Serial:      2,
		Iteration:   200,
		Performance: model.ExtType{
			"tflops":     200.0,
			"throughput": 2000.0,
			"loss":       0.3,
		},
	}
	err := facade.CreateTrainingPerformance(ctx, perf)
	require.NoError(t, err)

	result, err := facade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx, "workload-002", 2, 200)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, perf.WorkloadUID, result.WorkloadUID)
	assert.Equal(t, perf.Serial, result.Serial)
	assert.Equal(t, perf.Iteration, result.Iteration)
	assert.NotNil(t, result.Performance)
}

func TestTrainingFacade_GetTrainingPerformanceByWorkloadIdSerialAndIteration_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	result, err := facade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx, "non-existent", 1, 1)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestTrainingFacade_ListWorkloadPerformanceByWorkloadIdAndTimeRange(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	workloadUID := "workload-time-test"
	now := time.Now()

	// Create performances at different times
	performances := []*model.TrainingPerformance{
		{WorkloadUID: workloadUID, Serial: 1, Iteration: 100, Performance: model.ExtType{"tflops": 100.0, "throughput": 1000.0, "loss": 0.5}, CreatedAt: now.Add(-3 * time.Hour)},
		{WorkloadUID: workloadUID, Serial: 1, Iteration: 200, Performance: model.ExtType{"tflops": 110.0, "throughput": 1100.0, "loss": 0.4}, CreatedAt: now.Add(-2 * time.Hour)},
		{WorkloadUID: workloadUID, Serial: 1, Iteration: 300, Performance: model.ExtType{"tflops": 120.0, "throughput": 1200.0, "loss": 0.3}, CreatedAt: now.Add(-1 * time.Hour)},
		{WorkloadUID: "other-workload", Serial: 1, Iteration: 100, Performance: model.ExtType{"tflops": 90.0, "throughput": 900.0, "loss": 0.6}, CreatedAt: now.Add(-2 * time.Hour)},
	}

	for _, p := range performances {
		err := facade.CreateTrainingPerformance(ctx, p)
		require.NoError(t, err)
	}

	// Query with time range
	start := now.Add(-4 * time.Hour)
	end := now
	results, err := facade.ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx, workloadUID, start, end)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify ordering (should be ascending by created_at)
	for i := 0; i < len(results)-1; i++ {
		assert.True(t, results[i].CreatedAt.Before(results[i+1].CreatedAt) || results[i].CreatedAt.Equal(results[i+1].CreatedAt))
	}

	// Verify all are from the same workload
	for _, r := range results {
		assert.Equal(t, workloadUID, r.WorkloadUID)
	}
}

func TestTrainingFacade_ListWorkloadPerformanceByWorkloadIdAndTimeRange_EmptyRange(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	now := time.Now()

	// Create performance outside the query range
	perf := &model.TrainingPerformance{
		WorkloadUID: "workload-empty-test",
		Serial:      1,
		Iteration:   100,
		Performance: model.ExtType{"tflops": 100.0, "throughput": 1000.0, "loss": 0.5},
		CreatedAt:   now.Add(-10 * time.Hour),
	}
	err := facade.CreateTrainingPerformance(ctx, perf)
	require.NoError(t, err)

	// Query with range that doesn't include the performance
	start := now.Add(-2 * time.Hour)
	end := now
	results, err := facade.ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx, "workload-empty-test", start, end)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestTrainingFacade_ListTrainingPerformanceByWorkloadIdsAndTimeRange(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	now := time.Now()
	workloadUIDs := []string{"wl-1", "wl-2", "wl-3"}

	// Create performances for multiple workloads
	performances := []*model.TrainingPerformance{
		{WorkloadUID: "wl-1", Serial: 1, Iteration: 100, Performance: model.ExtType{"tflops": 100.0}, CreatedAt: now.Add(-2 * time.Hour)},
		{WorkloadUID: "wl-1", Serial: 1, Iteration: 200, Performance: model.ExtType{"tflops": 110.0}, CreatedAt: now.Add(-1 * time.Hour)},
		{WorkloadUID: "wl-2", Serial: 1, Iteration: 100, Performance: model.ExtType{"tflops": 105.0}, CreatedAt: now.Add(-2 * time.Hour)},
		{WorkloadUID: "wl-3", Serial: 1, Iteration: 100, Performance: model.ExtType{"tflops": 95.0}, CreatedAt: now.Add(-1 * time.Hour)},
		{WorkloadUID: "wl-other", Serial: 1, Iteration: 100, Performance: model.ExtType{"tflops": 90.0}, CreatedAt: now.Add(-1 * time.Hour)},
	}

	for _, p := range performances {
		err := facade.CreateTrainingPerformance(ctx, p)
		require.NoError(t, err)
	}

	// Query multiple workloads
	start := now.Add(-3 * time.Hour)
	end := now
	results, err := facade.ListTrainingPerformanceByWorkloadIdsAndTimeRange(ctx, workloadUIDs, start, end)
	require.NoError(t, err)
	assert.Len(t, results, 4)

	// Verify all results are from the requested workloads
	for _, r := range results {
		assert.Contains(t, workloadUIDs, r.WorkloadUID)
	}

	// Verify ordering
	for i := 0; i < len(results)-1; i++ {
		assert.True(t, results[i].CreatedAt.Before(results[i+1].CreatedAt) || results[i].CreatedAt.Equal(results[i+1].CreatedAt))
	}
}

func TestTrainingFacade_ListTrainingPerformanceByWorkloadIdsAndTimeRange_EmptyList(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	now := time.Now()

	// Query with empty workload list
	results, err := facade.ListTrainingPerformanceByWorkloadIdsAndTimeRange(ctx, []string{}, now.Add(-1*time.Hour), now)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestTrainingFacade_ListTrainingPerformanceByWorkloadIdsAndTimeRange_MultipleSerials(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	workloadUID := "wl-multi-serial"
	now := time.Now()

	// Create performances with different serials
	performances := []*model.TrainingPerformance{
		{WorkloadUID: workloadUID, Serial: 1, Iteration: 100, Performance: model.ExtType{"tflops": 100.0}, CreatedAt: now.Add(-3 * time.Hour)},
		{WorkloadUID: workloadUID, Serial: 2, Iteration: 100, Performance: model.ExtType{"tflops": 105.0}, CreatedAt: now.Add(-2 * time.Hour)},
		{WorkloadUID: workloadUID, Serial: 3, Iteration: 100, Performance: model.ExtType{"tflops": 110.0}, CreatedAt: now.Add(-1 * time.Hour)},
	}

	for _, p := range performances {
		err := facade.CreateTrainingPerformance(ctx, p)
		require.NoError(t, err)
	}

	results, err := facade.ListTrainingPerformanceByWorkloadIdsAndTimeRange(ctx, []string{workloadUID}, now.Add(-4*time.Hour), now)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify serials are in order
	assert.Equal(t, int32(1), results[0].Serial)
	assert.Equal(t, int32(2), results[1].Serial)
	assert.Equal(t, int32(3), results[2].Serial)
}

func TestTrainingFacade_TrainingPerformance_ProgressTracking(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := helper.CreateTestContext()

	workloadUID := "wl-progress"
	now := time.Now()

	// Simulate training progress over iterations
	iterations := []struct {
		iteration int
		metrics   map[string]float64
	}{
		{100, map[string]float64{"tflops": 150.0, "throughput": 1000.0, "loss": 1.0}},
		{200, map[string]float64{"tflops": 155.0, "throughput": 1050.0, "loss": 0.8}},
		{300, map[string]float64{"tflops": 160.0, "throughput": 1100.0, "loss": 0.6}},
		{400, map[string]float64{"tflops": 165.0, "throughput": 1150.0, "loss": 0.4}},
		{500, map[string]float64{"tflops": 170.0, "throughput": 1200.0, "loss": 0.2}},
	}

	for i, iter := range iterations {
		perfData := model.ExtType{}
		for k, v := range iter.metrics {
			perfData[k] = v
		}
		perf := &model.TrainingPerformance{
			WorkloadUID: workloadUID,
			Serial:      1,
			Iteration:   int32(iter.iteration),
			Performance: perfData,
			CreatedAt:   now.Add(time.Duration(i) * time.Minute),
		}
		err := facade.CreateTrainingPerformance(ctx, perf)
		require.NoError(t, err)
	}

	// Get all performance data
	results, err := facade.ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx, workloadUID, now.Add(-1*time.Hour), now.Add(1*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 5)

	// Verify progress: iterations should increase
	assert.Less(t, results[0].Iteration, results[4].Iteration)
}

// ==================== Helper Methods ====================

func TestTrainingFacade_WithCluster(t *testing.T) {
	facade := NewTrainingFacade()

	clusterFacade := facade.WithCluster("test-cluster")

	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*TrainingFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkTrainingFacade_CreateTrainingPerformance(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		perf := &model.TrainingPerformance{
			WorkloadUID: "bench-workload",
			Serial:      1,
			Iteration:   int32(i),
			Performance: model.ExtType{"tflops": 150.0, "throughput": 1000.0, "loss": 0.5},
		}
		_ = facade.CreateTrainingPerformance(ctx, perf)
	}
}

func BenchmarkTrainingFacade_GetTrainingPerformanceByWorkloadIdSerialAndIteration(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := context.Background()

	// Pre-populate
	perf := &model.TrainingPerformance{
		WorkloadUID: "bench-get",
		Serial:      1,
		Iteration:   100,
		Performance: model.ExtType{"tflops": 150.0},
	}
	_ = facade.CreateTrainingPerformance(ctx, perf)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = facade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(ctx, "bench-get", 1, 100)
	}
}

func BenchmarkTrainingFacade_ListWorkloadPerformanceByWorkloadIdAndTimeRange(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := context.Background()

	workloadUID := "bench-list"
	now := time.Now()

	// Pre-populate with 100 performances
	for i := 0; i < 100; i++ {
		perf := &model.TrainingPerformance{
			WorkloadUID: workloadUID,
			Serial:      1,
			Iteration:   int32(i * 10),
			Performance: model.ExtType{"tflops": 150.0},
			CreatedAt:   now.Add(time.Duration(i) * time.Second),
		}
		_ = facade.CreateTrainingPerformance(ctx, perf)
	}

	start := now.Add(-1 * time.Hour)
	end := now.Add(2 * time.Hour)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = facade.ListWorkloadPerformanceByWorkloadIdAndTimeRange(ctx, workloadUID, start, end)
	}
}

func BenchmarkTrainingFacade_ListTrainingPerformanceByWorkloadIdsAndTimeRange(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestTrainingFacade(helper.DB)
	ctx := context.Background()

	workloadUIDs := []string{"wl-1", "wl-2", "wl-3"}
	now := time.Now()

	// Pre-populate with performances for multiple workloads
	for _, uid := range workloadUIDs {
		for i := 0; i < 30; i++ {
			perf := &model.TrainingPerformance{
				WorkloadUID: uid,
				Serial:      1,
				Iteration:   int32(i * 10),
				Performance: model.ExtType{"tflops": 150.0},
				CreatedAt:   now.Add(time.Duration(i) * time.Second),
			}
			_ = facade.CreateTrainingPerformance(ctx, perf)
		}
	}

	start := now.Add(-1 * time.Hour)
	end := now.Add(1 * time.Hour)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = facade.ListTrainingPerformanceByWorkloadIdsAndTimeRange(ctx, workloadUIDs, start, end)
	}
}
