package database

import (
	"context"
	"testing"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/dal"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockGpuAggregationFacade creates a GpuAggregationFacade with the test database
type mockGpuAggregationFacade struct {
	GpuAggregationFacade
	db *gorm.DB
}

func (f *mockGpuAggregationFacade) getDB() *gorm.DB {
	return f.db
}

func (f *mockGpuAggregationFacade) getDAL() *dal.Query {
	return dal.Use(f.db)
}

// newTestGpuAggregationFacade creates a test GpuAggregationFacade
func newTestGpuAggregationFacade(db *gorm.DB) GpuAggregationFacadeInterface {
	return &mockGpuAggregationFacade{
		db: db,
	}
}

// ==================== ClusterGpuHourlyStats Tests ====================

func TestGpuAggregationFacade_SaveClusterHourlyStats(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	stats := &model.ClusterGpuHourlyStats{
		ClusterName:       "test-cluster",
		StatHour:          time.Now().Truncate(time.Hour),
		TotalGpuCapacity:  100,
		AllocatedGpuCount: 80,
		AllocationRate:    80.0,
		AvgUtilization:    75.5,
		MaxUtilization:    95.0,
		MinUtilization:    50.0,
		P50Utilization:    75.0,
		P95Utilization:    90.0,
		SampleCount:       60,
	}

	err := facade.SaveClusterHourlyStats(ctx, stats)
	require.NoError(t, err)
	assert.NotZero(t, stats.ID)
}

func TestGpuAggregationFacade_SaveClusterHourlyStats_Upsert(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	statHour := time.Now().Truncate(time.Hour)

	// Save initially
	stats := &model.ClusterGpuHourlyStats{
		ClusterName:       "test-cluster",
		StatHour:          statHour,
		TotalGpuCapacity:  100,
		AvgUtilization:    70.0,
		SampleCount:       50,
		AllocatedGpuCount: 70,
		AllocationRate:    70.0,
		MaxUtilization:    80.0,
		MinUtilization:    60.0,
		P50Utilization:    70.0,
		P95Utilization:    75.0,
	}
	err := facade.SaveClusterHourlyStats(ctx, stats)
	require.NoError(t, err)

	// Save again with updated values
	stats2 := &model.ClusterGpuHourlyStats{
		ClusterName:       "test-cluster",
		StatHour:          statHour,
		TotalGpuCapacity:  100,
		AvgUtilization:    80.0, // Updated
		SampleCount:       60,   // Updated
		AllocatedGpuCount: 80,
		AllocationRate:    80.0,
		MaxUtilization:    90.0,
		MinUtilization:    70.0,
		P50Utilization:    80.0,
		P95Utilization:    85.0,
	}
	err = facade.SaveClusterHourlyStats(ctx, stats2)
	require.NoError(t, err)

	// Verify it's updated, not duplicated
	results, err := facade.GetClusterHourlyStats(ctx, statHour.Add(-1*time.Hour), statHour.Add(1*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 80.0, results[0].AvgUtilization)
}

func TestGpuAggregationFacade_GetClusterHourlyStats(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	baseTime := time.Now().Truncate(time.Hour)

	// Create stats for 3 hours
	for i := 0; i < 3; i++ {
		stats := &model.ClusterGpuHourlyStats{
			ClusterName:       "test-cluster",
			StatHour:          baseTime.Add(time.Duration(i) * time.Hour),
			TotalGpuCapacity:  100,
			AvgUtilization:    float64(70 + i*5),
			SampleCount:       60,
			AllocatedGpuCount: float64(70 + i*5),
			AllocationRate:    float64(70 + i*5),
			MaxUtilization:    float64(80 + i*5),
			MinUtilization:    float64(60 + i*5),
			P50Utilization:    float64(70 + i*5),
			P95Utilization:    float64(75 + i*5),
		}
		err := facade.SaveClusterHourlyStats(ctx, stats)
		require.NoError(t, err)
	}

	// Query time range
	results, err := facade.GetClusterHourlyStats(ctx, baseTime, baseTime.Add(3*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Verify ordering
	assert.Equal(t, 70.0, results[0].AvgUtilization)
	assert.Equal(t, 80.0, results[2].AvgUtilization)
}

// ==================== NamespaceGpuHourlyStats Tests ====================

func TestGpuAggregationFacade_SaveNamespaceHourlyStats(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	stats := &model.NamespaceGpuHourlyStats{
		ClusterName:         "test-cluster",
		Namespace:           "default",
		StatHour:            time.Now().Truncate(time.Hour),
		TotalGpuCapacity:    100,
		AllocatedGpuCount:   10,
		AvgUtilization:      80.0,
		MaxUtilization:      95.0,
		MinUtilization:      60.0,
		ActiveWorkloadCount: 5,
	}

	err := facade.SaveNamespaceHourlyStats(ctx, stats)
	require.NoError(t, err)
	assert.NotZero(t, stats.ID)
}

func TestGpuAggregationFacade_GetNamespaceHourlyStats(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	baseTime := time.Now().Truncate(time.Hour)
	namespace := "test-namespace"

	// Create stats
	stats := &model.NamespaceGpuHourlyStats{
		ClusterName:         "test-cluster",
		Namespace:           namespace,
		StatHour:            baseTime,
		TotalGpuCapacity:    100,
		AllocatedGpuCount:   5,
		AvgUtilization:      75.0,
		MaxUtilization:      90.0,
		MinUtilization:      60.0,
		ActiveWorkloadCount: 3,
	}
	err := facade.SaveNamespaceHourlyStats(ctx, stats)
	require.NoError(t, err)

	// Get stats
	results, err := facade.GetNamespaceHourlyStats(ctx, namespace, baseTime.Add(-1*time.Hour), baseTime.Add(1*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, namespace, results[0].Namespace)
}

func TestGpuAggregationFacade_ListNamespaceHourlyStats(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	baseTime := time.Now().Truncate(time.Hour)

	// Create stats for multiple namespaces
	namespaces := []string{"ns-1", "ns-2", "ns-3"}
	for _, ns := range namespaces {
		stats := &model.NamespaceGpuHourlyStats{
			ClusterName:         "test-cluster",
			Namespace:           ns,
			StatHour:            baseTime,
			TotalGpuCapacity:    100,
			AllocatedGpuCount:   5,
			AvgUtilization:      75.0,
			MaxUtilization:      90.0,
			MinUtilization:      60.0,
			ActiveWorkloadCount: 3,
		}
		err := facade.SaveNamespaceHourlyStats(ctx, stats)
		require.NoError(t, err)
	}

	// List all
	results, err := facade.ListNamespaceHourlyStats(ctx, baseTime.Add(-1*time.Hour), baseTime.Add(1*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

// ==================== Snapshot Tests ====================

func TestGpuAggregationFacade_SaveSnapshot(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	snapshot := &model.GpuAllocationSnapshots{
		SnapshotTime:      time.Now(),
		ClusterName:       "test-cluster",
		DimensionType:     "cluster",
		TotalGpuCapacity:  100,
		AllocatedGpuCount: 80,
		AllocationDetails: model.ExtType{"namespaces": []string{"ns1", "ns2"}},
	}

	err := facade.SaveSnapshot(ctx, snapshot)
	require.NoError(t, err)
	assert.NotZero(t, snapshot.ID)
}

func TestGpuAggregationFacade_GetLatestSnapshot(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create multiple snapshots
	for i := 0; i < 3; i++ {
		snapshot := &model.GpuAllocationSnapshots{
			SnapshotTime:      time.Now().Add(time.Duration(i) * time.Minute),
			ClusterName:       "test-cluster",
			DimensionType:     "cluster",
			TotalGpuCapacity:  100,
			AllocatedGpuCount: 80 + int32(i),
			AllocationDetails: model.ExtType{},
		}
		err := facade.SaveSnapshot(ctx, snapshot)
		require.NoError(t, err)
	}

	// Get latest
	latest, err := facade.GetLatestSnapshot(ctx)
	require.NoError(t, err)
	require.NotNil(t, latest)

	assert.Equal(t, int32(82), latest.AllocatedGpuCount) // Last one has 80+2
}

// ==================== Cleanup Tests ====================

func TestGpuAggregationFacade_CleanupOldSnapshots(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	now := time.Now()

	// Create old and new snapshots
	for i := 0; i < 5; i++ {
		snapshot := &model.GpuAllocationSnapshots{
			SnapshotTime:      now.Add(time.Duration(i-3) * time.Hour), // Some old, some new
			ClusterName:       "test-cluster",
			DimensionType:     "cluster",
			TotalGpuCapacity:  100,
			AllocatedGpuCount: 80,
			AllocationDetails: model.ExtType{},
		}
		err := facade.SaveSnapshot(ctx, snapshot)
		require.NoError(t, err)
	}

	// Cleanup snapshots older than 2 hours ago
	deleted, err := facade.CleanupOldSnapshots(ctx, now.Add(-2*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(2), deleted) // Should delete 2 old snapshots

	// Verify remaining
	results, err := facade.ListSnapshots(ctx, now.Add(-24*time.Hour), now.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

// ==================== Pagination Tests ====================

func TestGpuAggregationFacade_GetClusterHourlyStatsPaginated(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	baseTime := time.Now().Truncate(time.Hour)

	// Create 10 stats
	for i := 0; i < 10; i++ {
		stats := &model.ClusterGpuHourlyStats{
			ClusterName:       "test-cluster",
			StatHour:          baseTime.Add(time.Duration(i) * time.Hour),
			TotalGpuCapacity:  100,
			AvgUtilization:    float64(70 + i),
			SampleCount:       60,
			AllocatedGpuCount: float64(70 + i),
			AllocationRate:    float64(70 + i),
			MaxUtilization:    float64(80 + i),
			MinUtilization:    float64(60 + i),
			P50Utilization:    float64(70 + i),
			P95Utilization:    float64(75 + i),
		}
		err := facade.SaveClusterHourlyStats(ctx, stats)
		require.NoError(t, err)
	}

	// Test pagination
	opts := PaginationOptions{
		Page:     1,
		PageSize: 5,
		OrderBy:  "time",
	}

	result, err := facade.GetClusterHourlyStatsPaginated(ctx, baseTime, baseTime.Add(20*time.Hour), opts)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, int64(10), result.Total)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 5, result.PageSize)
	assert.Equal(t, 2, result.TotalPages)

	data, ok := result.Data.([]*model.ClusterGpuHourlyStats)
	require.True(t, ok)
	assert.Len(t, data, 5)
}

// ==================== Metadata Tests ====================

func TestGpuAggregationFacade_GetDistinctNamespaces(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := helper.CreateTestContext()

	baseTime := time.Now().Truncate(time.Hour)

	// Create stats for different namespaces
	namespaces := []string{"ns-1", "ns-2", "ns-1", "ns-3"} // Duplicate ns-1
	for i, ns := range namespaces {
		stats := &model.NamespaceGpuHourlyStats{
			ClusterName:         "test-cluster",
			Namespace:           ns,
			StatHour:            baseTime.Add(time.Duration(i) * time.Hour),
			TotalGpuCapacity:    100,
			AllocatedGpuCount:   5,
			AvgUtilization:      75.0,
			MaxUtilization:      90.0,
			MinUtilization:      60.0,
			ActiveWorkloadCount: 3,
		}
		err := facade.SaveNamespaceHourlyStats(ctx, stats)
		require.NoError(t, err)
	}

	// Get distinct namespaces
	distinctNS, err := facade.GetDistinctNamespaces(ctx, baseTime.Add(-1*time.Hour), baseTime.Add(10*time.Hour))
	require.NoError(t, err)

	// Should have 3 unique namespaces
	assert.Len(t, distinctNS, 3)
	assert.Contains(t, distinctNS, "ns-1")
	assert.Contains(t, distinctNS, "ns-2")
	assert.Contains(t, distinctNS, "ns-3")
}

// ==================== Helper Methods ====================

func TestGpuAggregationFacade_WithCluster(t *testing.T) {
	facade := NewGpuAggregationFacade()

	clusterFacade := facade.WithCluster("test-cluster")

	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*GpuAggregationFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkGpuAggregationFacade_SaveClusterHourlyStats(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stats := &model.ClusterGpuHourlyStats{
			ClusterName:       "bench-cluster",
			StatHour:          time.Now().Truncate(time.Hour).Add(time.Duration(i) * time.Minute),
			TotalGpuCapacity:  100,
			AvgUtilization:    75.0,
			SampleCount:       60,
			AllocatedGpuCount: 75,
			AllocationRate:    75.0,
			MaxUtilization:    90.0,
			MinUtilization:    60.0,
			P50Utilization:    75.0,
			P95Utilization:    85.0,
		}
		_ = facade.SaveClusterHourlyStats(ctx, stats)
	}
}

func BenchmarkGpuAggregationFacade_GetClusterHourlyStats(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := context.Background()

	baseTime := time.Now().Truncate(time.Hour)

	// Pre-populate
	for i := 0; i < 24; i++ {
		stats := &model.ClusterGpuHourlyStats{
			ClusterName:       "bench-cluster",
			StatHour:          baseTime.Add(time.Duration(i) * time.Hour),
			TotalGpuCapacity:  100,
			AvgUtilization:    75.0,
			SampleCount:       60,
			AllocatedGpuCount: 75,
			AllocationRate:    75.0,
			MaxUtilization:    90.0,
			MinUtilization:    60.0,
			P50Utilization:    75.0,
			P95Utilization:    85.0,
		}
		_ = facade.SaveClusterHourlyStats(ctx, stats)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = facade.GetClusterHourlyStats(ctx, baseTime, baseTime.Add(24*time.Hour))
	}
}

func BenchmarkGpuAggregationFacade_GetClusterHourlyStatsPaginated(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestGpuAggregationFacade(helper.DB)
	ctx := context.Background()

	baseTime := time.Now().Truncate(time.Hour)

	// Pre-populate
	for i := 0; i < 100; i++ {
		stats := &model.ClusterGpuHourlyStats{
			ClusterName:       "bench-cluster",
			StatHour:          baseTime.Add(time.Duration(i) * time.Hour),
			TotalGpuCapacity:  100,
			AvgUtilization:    75.0,
			SampleCount:       60,
			AllocatedGpuCount: 75,
			AllocationRate:    75.0,
			MaxUtilization:    90.0,
			MinUtilization:    60.0,
			P50Utilization:    75.0,
			P95Utilization:    85.0,
		}
		_ = facade.SaveClusterHourlyStats(ctx, stats)
	}

	opts := PaginationOptions{Page: 1, PageSize: 20}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = facade.GetClusterHourlyStatsPaginated(ctx, baseTime, baseTime.Add(200*time.Hour), opts)
	}
}
