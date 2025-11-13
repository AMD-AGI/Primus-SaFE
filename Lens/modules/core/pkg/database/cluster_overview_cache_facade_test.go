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

// mockClusterOverviewCacheFacade creates a ClusterOverviewCacheFacade with the test database
type mockClusterOverviewCacheFacade struct {
	ClusterOverviewCacheFacade
	db *gorm.DB
}

func (f *mockClusterOverviewCacheFacade) getDB() *gorm.DB {
	return f.db
}

func (f *mockClusterOverviewCacheFacade) getDAL() *dal.Query {
	return dal.Use(f.db)
}

// newTestClusterOverviewCacheFacade creates a test ClusterOverviewCacheFacade
func newTestClusterOverviewCacheFacade(db *gorm.DB) ClusterOverviewCacheFacadeInterface {
	return &mockClusterOverviewCacheFacade{
		db: db,
	}
}

// TestClusterOverviewCacheFacade_CreateClusterOverviewCache tests creating cache entries
func TestClusterOverviewCacheFacade_CreateClusterOverviewCache(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	cache := &model.ClusterOverviewCache{
		ClusterName:                  "test-cluster",
		TotalNodes:                   100,
		HealthyNodes:                 95,
		FaultyNodes:                  5,
		FullyIdleNodes:               20,
		PartiallyIdleNodes:           30,
		BusyNodes:                    45,
		AllocationRate:               85.5,
		Utilization:                  72.3,
		StorageTotalSpace:            1000000.0,
		StorageUsedSpace:             750000.0,
		StorageUsagePercentage:       75.0,
		StorageTotalInodes:           500000.0,
		StorageUsedInodes:            400000.0,
		StorageInodesUsagePercentage: 80.0,
		StorageReadBandwidth:         1024.5,
		StorageWriteBandwidth:        2048.7,
		RdmaTotalTx:                  50000.0,
		RdmaTotalRx:                  48000.0,
	}
	
	err := facade.CreateClusterOverviewCache(ctx, cache)
	require.NoError(t, err)
	assert.NotZero(t, cache.ID)
	assert.NotZero(t, cache.CreatedAt)
	assert.NotZero(t, cache.UpdatedAt)
}

// TestClusterOverviewCacheFacade_GetClusterOverviewCache tests getting the first cache entry
func TestClusterOverviewCacheFacade_GetClusterOverviewCache(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a cache entry
	cache := &model.ClusterOverviewCache{
		ClusterName:                  "get-test-cluster",
		TotalNodes:                   50,
		HealthyNodes:                 48,
		FaultyNodes:                  2,
		FullyIdleNodes:               10,
		PartiallyIdleNodes:           15,
		BusyNodes:                    23,
		AllocationRate:               76.0,
		Utilization:                  65.5,
		StorageTotalSpace:            500000.0,
		StorageUsedSpace:             400000.0,
		StorageUsagePercentage:       80.0,
		StorageTotalInodes:           250000.0,
		StorageUsedInodes:            200000.0,
		StorageInodesUsagePercentage: 80.0,
		StorageReadBandwidth:         512.0,
		StorageWriteBandwidth:        1024.0,
		RdmaTotalTx:                  25000.0,
		RdmaTotalRx:                  24000.0,
	}
	err := facade.CreateClusterOverviewCache(ctx, cache)
	require.NoError(t, err)
	
	// Get the cache entry
	result, err := facade.GetClusterOverviewCache(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, cache.ClusterName, result.ClusterName)
	assert.Equal(t, cache.TotalNodes, result.TotalNodes)
	assert.Equal(t, cache.HealthyNodes, result.HealthyNodes)
	assert.Equal(t, cache.AllocationRate, result.AllocationRate)
	assert.Equal(t, cache.Utilization, result.Utilization)
}

// TestClusterOverviewCacheFacade_GetClusterOverviewCache_Empty tests getting when no entries exist
func TestClusterOverviewCacheFacade_GetClusterOverviewCache_Empty(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetClusterOverviewCache(ctx)
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestClusterOverviewCacheFacade_UpdateClusterOverviewCache tests updating cache entries
func TestClusterOverviewCacheFacade_UpdateClusterOverviewCache(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a cache entry
	cache := &model.ClusterOverviewCache{
		ClusterName:                  "update-test-cluster",
		TotalNodes:                   50,
		HealthyNodes:                 45,
		FaultyNodes:                  5,
		FullyIdleNodes:               10,
		PartiallyIdleNodes:           15,
		BusyNodes:                    20,
		AllocationRate:               60.0,
		Utilization:                  55.0,
		StorageTotalSpace:            100000.0,
		StorageUsedSpace:             50000.0,
		StorageUsagePercentage:       50.0,
		StorageTotalInodes:           50000.0,
		StorageUsedInodes:            25000.0,
		StorageInodesUsagePercentage: 50.0,
		StorageReadBandwidth:         256.0,
		StorageWriteBandwidth:        512.0,
		RdmaTotalTx:                  10000.0,
		RdmaTotalRx:                  9500.0,
	}
	err := facade.CreateClusterOverviewCache(ctx, cache)
	require.NoError(t, err)
	
	// Update the cache entry
	cache.TotalNodes = 60
	cache.HealthyNodes = 58
	cache.AllocationRate = 85.0
	cache.Utilization = 75.0
	err = facade.UpdateClusterOverviewCache(ctx, cache)
	require.NoError(t, err)
	
	// Verify the update
	result, err := facade.GetClusterOverviewCache(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, int32(60), result.TotalNodes)
	assert.Equal(t, int32(58), result.HealthyNodes)
	assert.Equal(t, 85.0, result.AllocationRate)
	assert.Equal(t, 75.0, result.Utilization)
}

// TestClusterOverviewCacheFacade_ListClusterOverviewCache tests listing cache entries with pagination
func TestClusterOverviewCacheFacade_ListClusterOverviewCache(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple cache entries
	for i := 0; i < 15; i++ {
		cache := &model.ClusterOverviewCache{
			ClusterName:                  "cluster-" + string(rune('a'+i)),
			TotalNodes:                   int32(10 + i),
			HealthyNodes:                 int32(10 + i),
			FaultyNodes:                  0,
			FullyIdleNodes:               int32(i),
			PartiallyIdleNodes:           int32(i + 1),
			BusyNodes:                    int32(i + 2),
			AllocationRate:               float64(50 + i),
			Utilization:                  float64(45 + i),
			StorageTotalSpace:            float64(10000 * (i + 1)),
			StorageUsedSpace:             float64(5000 * (i + 1)),
			StorageUsagePercentage:       50.0,
			StorageTotalInodes:           float64(1000 * (i + 1)),
			StorageUsedInodes:            float64(500 * (i + 1)),
			StorageInodesUsagePercentage: 50.0,
			StorageReadBandwidth:         float64(100 * (i + 1)),
			StorageWriteBandwidth:        float64(200 * (i + 1)),
			RdmaTotalTx:                  float64(1000 * (i + 1)),
			RdmaTotalRx:                  float64(900 * (i + 1)),
		}
		err := facade.CreateClusterOverviewCache(ctx, cache)
		require.NoError(t, err)
		
		// Add small delay to ensure different created_at times
		time.Sleep(1 * time.Millisecond)
	}
	
	// Test pagination
	tests := []struct {
		name         string
		pageNum      int
		pageSize     int
		expectedLen  int
		expectedTotal int
	}{
		{
			name:         "First page",
			pageNum:      1,
			pageSize:     10,
			expectedLen:  10,
			expectedTotal: 15,
		},
		{
			name:         "Second page",
			pageNum:      2,
			pageSize:     10,
			expectedLen:  5,
			expectedTotal: 15,
		},
		{
			name:         "All entries",
			pageNum:      1,
			pageSize:     20,
			expectedLen:  15,
			expectedTotal: 15,
		},
		{
			name:         "Zero page size (get all)",
			pageNum:      0,
			pageSize:     0,
			expectedLen:  15,
			expectedTotal: 15,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caches, total, err := facade.ListClusterOverviewCache(ctx, tt.pageNum, tt.pageSize)
			require.NoError(t, err)
			
			assert.Len(t, caches, tt.expectedLen)
			assert.Equal(t, tt.expectedTotal, total)
		})
	}
}

// TestClusterOverviewCacheFacade_ListClusterOverviewCache_Empty tests listing when no entries exist
func TestClusterOverviewCacheFacade_ListClusterOverviewCache_Empty(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	caches, total, err := facade.ListClusterOverviewCache(ctx, 1, 10)
	require.NoError(t, err)
	
	assert.Empty(t, caches)
	assert.Equal(t, 0, total)
}

// TestClusterOverviewCacheFacade_ListClusterOverviewCache_OrderByCreatedAt tests ordering
func TestClusterOverviewCacheFacade_ListClusterOverviewCache_OrderByCreatedAt(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create cache entries with explicit time delays
	var createdIDs []int32
	for i := 0; i < 3; i++ {
		cache := &model.ClusterOverviewCache{
			ClusterName:                  "cluster-" + string(rune('a'+i)),
			TotalNodes:                   int32(10 + i),
			HealthyNodes:                 int32(10 + i),
			FaultyNodes:                  0,
			FullyIdleNodes:               0,
			PartiallyIdleNodes:           0,
			BusyNodes:                    0,
			AllocationRate:               50.0,
			Utilization:                  45.0,
			StorageTotalSpace:            10000.0,
			StorageUsedSpace:             5000.0,
			StorageUsagePercentage:       50.0,
			StorageTotalInodes:           1000.0,
			StorageUsedInodes:            500.0,
			StorageInodesUsagePercentage: 50.0,
			StorageReadBandwidth:         100.0,
			StorageWriteBandwidth:        200.0,
			RdmaTotalTx:                  1000.0,
			RdmaTotalRx:                  900.0,
		}
		err := facade.CreateClusterOverviewCache(ctx, cache)
		require.NoError(t, err)
		createdIDs = append(createdIDs, cache.ID)
		
		time.Sleep(10 * time.Millisecond)
	}
	
	// List all entries
	caches, _, err := facade.ListClusterOverviewCache(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, caches, 3)
	
	// Verify they are in reverse order (newest first)
	assert.Equal(t, createdIDs[2], caches[0].ID)
	assert.Equal(t, createdIDs[1], caches[1].ID)
	assert.Equal(t, createdIDs[0], caches[2].ID)
}

// TestClusterOverviewCacheFacade_AllMetrics tests cache with all metrics populated
func TestClusterOverviewCacheFacade_AllMetrics(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	cache := &model.ClusterOverviewCache{
		ClusterName:                  "full-metrics-cluster",
		TotalNodes:                   200,
		HealthyNodes:                 190,
		FaultyNodes:                  10,
		FullyIdleNodes:               40,
		PartiallyIdleNodes:           60,
		BusyNodes:                    90,
		AllocationRate:               90.5,
		Utilization:                  82.3,
		StorageTotalSpace:            2000000.0,
		StorageUsedSpace:             1500000.0,
		StorageUsagePercentage:       75.0,
		StorageTotalInodes:           1000000.0,
		StorageUsedInodes:            800000.0,
		StorageInodesUsagePercentage: 80.0,
		StorageReadBandwidth:         2048.5,
		StorageWriteBandwidth:        4096.7,
		RdmaTotalTx:                  100000.0,
		RdmaTotalRx:                  96000.0,
	}
	
	err := facade.CreateClusterOverviewCache(ctx, cache)
	require.NoError(t, err)
	
	// Get and verify all metrics
	result, err := facade.GetClusterOverviewCache(ctx)
	require.NoError(t, err)
	require.NotNil(t, result)
	
	// Node metrics
	assert.Equal(t, int32(200), result.TotalNodes)
	assert.Equal(t, int32(190), result.HealthyNodes)
	assert.Equal(t, int32(10), result.FaultyNodes)
	assert.Equal(t, int32(40), result.FullyIdleNodes)
	assert.Equal(t, int32(60), result.PartiallyIdleNodes)
	assert.Equal(t, int32(90), result.BusyNodes)
	
	// GPU metrics
	assert.Equal(t, 90.5, result.AllocationRate)
	assert.Equal(t, 82.3, result.Utilization)
	
	// Storage metrics
	assert.Equal(t, 2000000.0, result.StorageTotalSpace)
	assert.Equal(t, 1500000.0, result.StorageUsedSpace)
	assert.Equal(t, 75.0, result.StorageUsagePercentage)
	assert.Equal(t, 1000000.0, result.StorageTotalInodes)
	assert.Equal(t, 800000.0, result.StorageUsedInodes)
	assert.Equal(t, 80.0, result.StorageInodesUsagePercentage)
	assert.Equal(t, 2048.5, result.StorageReadBandwidth)
	assert.Equal(t, 4096.7, result.StorageWriteBandwidth)
	
	// RDMA metrics
	assert.Equal(t, 100000.0, result.RdmaTotalTx)
	assert.Equal(t, 96000.0, result.RdmaTotalRx)
}

// TestClusterOverviewCacheFacade_WithCluster tests the WithCluster method
func TestClusterOverviewCacheFacade_WithCluster(t *testing.T) {
	facade := NewClusterOverviewCacheFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*ClusterOverviewCacheFacadeInterface)(nil), clusterFacade)
}

// ==================== Benchmark Tests ====================

func BenchmarkClusterOverviewCacheFacade_CreateClusterOverviewCache(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		cache := &model.ClusterOverviewCache{
			ClusterName:                  "bench-cluster",
			TotalNodes:                   100,
			HealthyNodes:                 95,
			FaultyNodes:                  5,
			FullyIdleNodes:               20,
			PartiallyIdleNodes:           30,
			BusyNodes:                    45,
			AllocationRate:               85.5,
			Utilization:                  72.3,
			StorageTotalSpace:            1000000.0,
			StorageUsedSpace:             750000.0,
			StorageUsagePercentage:       75.0,
			StorageTotalInodes:           500000.0,
			StorageUsedInodes:            400000.0,
			StorageInodesUsagePercentage: 80.0,
			StorageReadBandwidth:         1024.5,
			StorageWriteBandwidth:        2048.7,
			RdmaTotalTx:                  50000.0,
			RdmaTotalRx:                  48000.0,
		}
		_ = facade.CreateClusterOverviewCache(ctx, cache)
	}
}

func BenchmarkClusterOverviewCacheFacade_GetClusterOverviewCache(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	cache := &model.ClusterOverviewCache{
		ClusterName:                  "bench-cluster",
		TotalNodes:                   100,
		HealthyNodes:                 95,
		FaultyNodes:                  5,
		FullyIdleNodes:               20,
		PartiallyIdleNodes:           30,
		BusyNodes:                    45,
		AllocationRate:               85.5,
		Utilization:                  72.3,
		StorageTotalSpace:            1000000.0,
		StorageUsedSpace:             750000.0,
		StorageUsagePercentage:       75.0,
		StorageTotalInodes:           500000.0,
		StorageUsedInodes:            400000.0,
		StorageInodesUsagePercentage: 80.0,
		StorageReadBandwidth:         1024.5,
		StorageWriteBandwidth:        2048.7,
		RdmaTotalTx:                  50000.0,
		RdmaTotalRx:                  48000.0,
	}
	_ = facade.CreateClusterOverviewCache(ctx, cache)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.GetClusterOverviewCache(ctx)
	}
}

func BenchmarkClusterOverviewCacheFacade_ListClusterOverviewCache(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestClusterOverviewCacheFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate with 100 entries
	for i := 0; i < 100; i++ {
		cache := &model.ClusterOverviewCache{
			ClusterName:                  "cluster-" + string(rune('0'+i%10)),
			TotalNodes:                   int32(10 + i),
			HealthyNodes:                 int32(10 + i),
			FaultyNodes:                  0,
			FullyIdleNodes:               int32(i),
			PartiallyIdleNodes:           int32(i + 1),
			BusyNodes:                    int32(i + 2),
			AllocationRate:               float64(50 + i%50),
			Utilization:                  float64(45 + i%50),
			StorageTotalSpace:            float64(10000 * (i + 1)),
			StorageUsedSpace:             float64(5000 * (i + 1)),
			StorageUsagePercentage:       50.0,
			StorageTotalInodes:           float64(1000 * (i + 1)),
			StorageUsedInodes:            float64(500 * (i + 1)),
			StorageInodesUsagePercentage: 50.0,
			StorageReadBandwidth:         float64(100 * (i + 1)),
			StorageWriteBandwidth:        float64(200 * (i + 1)),
			RdmaTotalTx:                  float64(1000 * (i + 1)),
			RdmaTotalRx:                  float64(900 * (i + 1)),
		}
		_ = facade.CreateClusterOverviewCache(ctx, cache)
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.ListClusterOverviewCache(ctx, 1, 10)
	}
}

