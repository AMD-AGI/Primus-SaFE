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

// mockStorageFacade creates a StorageFacade with the test database
type mockStorageFacade struct {
	StorageFacade
	db *gorm.DB
}

func (f *mockStorageFacade) getDB() *gorm.DB {
	return f.db
}

func (f *mockStorageFacade) getDAL() *dal.Query {
	return dal.Use(f.db)
}

// newTestStorageFacade creates a test StorageFacade
func newTestStorageFacade(db *gorm.DB) StorageFacadeInterface {
	return &mockStorageFacade{
		db: db,
	}
}

// TestStorageFacade_CreateStorage tests creating storage entries
func TestStorageFacade_CreateStorage(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	storage := &model.Storage{
		Name:   "test-storage",
		Kind:   "pvc",
		Config: model.ExtType{"capacity": "100Gi"},
		Source: "kubernetes",
		Status: "active",
	}
	
	err := facade.CreateStorage(ctx, storage)
	require.NoError(t, err)
	assert.NotZero(t, storage.ID)
	assert.NotZero(t, storage.CreatedAt)
	assert.NotZero(t, storage.UpdatedAt)
}

// TestStorageFacade_GetStorageByKindAndName tests getting storage by kind and name
func TestStorageFacade_GetStorageByKindAndName(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a storage entry
	storage := &model.Storage{
		Name:   "test-storage",
		Kind:   "pvc",
		Config: model.ExtType{"capacity": "100Gi"},
		Source: "kubernetes",
		Status: "active",
	}
	err := facade.CreateStorage(ctx, storage)
	require.NoError(t, err)
	
	// Get the storage entry
	result, err := facade.GetStorageByKindAndName(ctx, "pvc", "test-storage")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, storage.ID, result.ID)
	assert.Equal(t, storage.Name, result.Name)
	assert.Equal(t, storage.Kind, result.Kind)
	assert.Equal(t, storage.Source, result.Source)
	assert.Equal(t, storage.Status, result.Status)
}

// TestStorageFacade_GetStorageByKindAndName_NotFound tests getting non-existent storage
func TestStorageFacade_GetStorageByKindAndName_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	result, err := facade.GetStorageByKindAndName(ctx, "pvc", "non-existent")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestStorageFacade_UpdateStorage tests updating storage entries
func TestStorageFacade_UpdateStorage(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create a storage entry
	storage := &model.Storage{
		Name:   "test-storage",
		Kind:   "pvc",
		Config: model.ExtType{"capacity": "100Gi"},
		Source: "kubernetes",
		Status: "active",
	}
	err := facade.CreateStorage(ctx, storage)
	require.NoError(t, err)
	
	// Update the storage entry
	storage.Status = "inactive"
	storage.Config = model.ExtType{"capacity": "200Gi"}
	err = facade.UpdateStorage(ctx, storage)
	require.NoError(t, err)
	
	// Verify the update
	result, err := facade.GetStorageByKindAndName(ctx, "pvc", "test-storage")
	require.NoError(t, err)
	require.NotNil(t, result)
	
	assert.Equal(t, "inactive", result.Status)
	assert.Equal(t, "200Gi", result.Config["capacity"])
}

// TestStorageFacade_ListStorage tests listing storage entries with pagination
func TestStorageFacade_ListStorage(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create multiple storage entries
	for i := 0; i < 15; i++ {
		storage := &model.Storage{
			Name:   "test-storage-" + string(rune('a'+i)),
			Kind:   "pvc",
			Config: model.ExtType{"capacity": "100Gi"},
			Source: "kubernetes",
			Status: "active",
		}
		err := facade.CreateStorage(ctx, storage)
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
			storages, total, err := facade.ListStorage(ctx, tt.pageNum, tt.pageSize)
			require.NoError(t, err)
			
			assert.Len(t, storages, tt.expectedLen)
			assert.Equal(t, tt.expectedTotal, total)
		})
	}
}

// TestStorageFacade_ListStorage_Empty tests listing when no entries exist
func TestStorageFacade_ListStorage_Empty(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	storages, total, err := facade.ListStorage(ctx, 1, 10)
	require.NoError(t, err)
	
	assert.Empty(t, storages)
	assert.Equal(t, 0, total)
}

// TestStorageFacade_ListStorage_OrderByCreatedAt tests that results are ordered by created_at desc
func TestStorageFacade_ListStorage_OrderByCreatedAt(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create storage entries with explicit time delays
	var createdIDs []int32
	for i := 0; i < 3; i++ {
		storage := &model.Storage{
			Name:   "test-storage-" + string(rune('a'+i)),
			Kind:   "pvc",
			Config: model.ExtType{},
			Source: "kubernetes",
			Status: "active",
		}
		err := facade.CreateStorage(ctx, storage)
		require.NoError(t, err)
		createdIDs = append(createdIDs, storage.ID)
		
		time.Sleep(10 * time.Millisecond)
	}
	
	// List all entries
	storages, _, err := facade.ListStorage(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, storages, 3)
	
	// Verify they are in reverse order (newest first)
	assert.Equal(t, createdIDs[2], storages[0].ID)
	assert.Equal(t, createdIDs[1], storages[1].ID)
	assert.Equal(t, createdIDs[0], storages[2].ID)
}

// TestStorageFacade_WithCluster tests the WithCluster method
func TestStorageFacade_WithCluster(t *testing.T) {
	facade := NewStorageFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*StorageFacadeInterface)(nil), clusterFacade)
}

// TestStorageFacade_MultipleKinds tests storage with different kinds
func TestStorageFacade_MultipleKinds(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Create storage entries with different kinds
	kinds := []string{"pvc", "hostpath", "nfs", "cephfs"}
	for _, kind := range kinds {
		storage := &model.Storage{
			Name:   "test-storage",
			Kind:   kind,
			Config: model.ExtType{"type": kind},
			Source: "kubernetes",
			Status: "active",
		}
		err := facade.CreateStorage(ctx, storage)
		require.NoError(t, err)
	}
	
	// Verify we can get each one by kind
	for _, kind := range kinds {
		result, err := facade.GetStorageByKindAndName(ctx, kind, "test-storage")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, kind, result.Kind)
	}
}

// TestStorageFacade_CreateDuplicate tests creating duplicate storage
func TestStorageFacade_CreateDuplicate(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	storage1 := &model.Storage{
		Name:   "test-storage",
		Kind:   "pvc",
		Config: model.ExtType{},
		Source: "kubernetes",
		Status: "active",
	}
	err := facade.CreateStorage(ctx, storage1)
	require.NoError(t, err)
	
	// Create another with same kind and name (should succeed in this schema)
	storage2 := &model.Storage{
		Name:   "test-storage",
		Kind:   "pvc",
		Config: model.ExtType{},
		Source: "kubernetes",
		Status: "active",
	}
	err = facade.CreateStorage(ctx, storage2)
	require.NoError(t, err)
	
	// Should have 2 entries
	assert.Equal(t, int64(2), helper.Count(model.TableNameStorage))
}

// Benchmark tests
func BenchmarkStorageFacade_CreateStorage(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		storage := &model.Storage{
			Name:   "bench-storage",
			Kind:   "pvc",
			Config: model.ExtType{},
			Source: "kubernetes",
			Status: "active",
		}
		_ = facade.CreateStorage(ctx, storage)
	}
}

func BenchmarkStorageFacade_GetStorageByKindAndName(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate
	storage := &model.Storage{
		Name:   "bench-storage",
		Kind:   "pvc",
		Config: model.ExtType{},
		Source: "kubernetes",
		Status: "active",
	}
	_ = facade.CreateStorage(ctx, storage)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _ = facade.GetStorageByKindAndName(ctx, "pvc", "bench-storage")
	}
}

func BenchmarkStorageFacade_ListStorage(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestStorageFacade(helper.DB)
	ctx := context.Background()
	
	// Pre-populate with 100 entries
	for i := 0; i < 100; i++ {
		storage := &model.Storage{
			Name:   "bench-storage-" + string(rune('0'+i%10)),
			Kind:   "pvc",
			Config: model.ExtType{},
			Source: "kubernetes",
			Status: "active",
		}
		_ = facade.CreateStorage(ctx, storage)
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_, _, _ = facade.ListStorage(ctx, 1, 10)
	}
}

