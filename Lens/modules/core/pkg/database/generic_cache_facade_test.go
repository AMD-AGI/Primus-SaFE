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

// mockGenericCacheFacade creates a GenericCacheFacade with the test database
type mockGenericCacheFacade struct {
	GenericCacheFacade
	db *gorm.DB
}

func (f *mockGenericCacheFacade) getDB() *gorm.DB {
	return f.db
}

func (f *mockGenericCacheFacade) getDAL() *dal.Query {
	return dal.Use(f.db)
}

// newTestGenericCacheFacade creates a test GenericCacheFacade
func newTestGenericCacheFacade(db *gorm.DB) GenericCacheFacadeInterface {
	return &mockGenericCacheFacade{
		db: db,
	}
}

// TestGenericCacheFacade_Set_Get tests setting and getting cache entries
func TestGenericCacheFacade_Set_Get(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	tests := []struct {
		name      string
		key       string
		value     interface{}
		expiresAt *time.Time
	}{
		{
			name:      "Simple string value",
			key:       "test-key-1",
			value:     map[string]interface{}{"data": "test-value"},
			expiresAt: nil,
		},
		{
			name:      "Complex object value",
			key:       "test-key-2",
			value:     map[string]interface{}{"name": "test", "count": 42, "active": true},
			expiresAt: nil,
		},
		{
			name:      "With expiration",
			key:       "test-key-3",
			value:     map[string]interface{}{"data": "expires"},
			expiresAt: timePtr(time.Now().Add(1 * time.Hour)),
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set the cache entry
			err := facade.Set(ctx, tt.key, tt.value, tt.expiresAt)
			require.NoError(t, err)
			
			// Get the cache entry
			var result map[string]interface{}
			err = facade.Get(ctx, tt.key, &result)
			require.NoError(t, err)
			
			// Verify the value
			assert.Equal(t, tt.value, result)
		})
	}
}

// TestGenericCacheFacade_Set_Array tests setting and getting array values
func TestGenericCacheFacade_Set_Array(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Set an array value
	arrayValue := []string{"item1", "item2", "item3"}
	err := facade.Set(ctx, "array-key", arrayValue, nil)
	require.NoError(t, err)
	
	// Get the array value
	var result []string
	err = facade.Get(ctx, "array-key", &result)
	require.NoError(t, err)
	
	assert.Equal(t, arrayValue, result)
}

// TestGenericCacheFacade_Get_NotFound tests getting non-existent cache entries
func TestGenericCacheFacade_Get_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	var result map[string]interface{}
	err := facade.Get(ctx, "non-existent-key", &result)
	
	assert.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestGenericCacheFacade_Get_Expired tests getting expired cache entries
func TestGenericCacheFacade_Get_Expired(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Set a cache entry that expires in the past
	pastTime := time.Now().Add(-1 * time.Hour)
	value := map[string]interface{}{"data": "expired"}
	err := facade.Set(ctx, "expired-key", value, &pastTime)
	require.NoError(t, err)
	
	// Try to get the expired entry
	var result map[string]interface{}
	err = facade.Get(ctx, "expired-key", &result)
	
	assert.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestGenericCacheFacade_Set_Update tests updating existing cache entries
func TestGenericCacheFacade_Set_Update(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	key := "update-key"
	
	// Set initial value
	initialValue := map[string]interface{}{"data": "initial"}
	err := facade.Set(ctx, key, initialValue, nil)
	require.NoError(t, err)
	
	// Update the value
	updatedValue := map[string]interface{}{"data": "updated"}
	err = facade.Set(ctx, key, updatedValue, nil)
	require.NoError(t, err)
	
	// Get and verify the updated value
	var result map[string]interface{}
	err = facade.Get(ctx, key, &result)
	require.NoError(t, err)
	
	assert.Equal(t, updatedValue, result)
	
	// Verify only one record exists
	assert.Equal(t, int64(1), helper.Count(model.TableNameGenericCache))
}

// TestGenericCacheFacade_Delete tests deleting cache entries
func TestGenericCacheFacade_Delete(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Set a cache entry
	key := "delete-key"
	value := map[string]interface{}{"data": "to-delete"}
	err := facade.Set(ctx, key, value, nil)
	require.NoError(t, err)
	
	// Delete the entry
	err = facade.Delete(ctx, key)
	require.NoError(t, err)
	
	// Verify it's deleted
	var result map[string]interface{}
	err = facade.Get(ctx, key, &result)
	assert.Error(t, err)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

// TestGenericCacheFacade_Delete_NotFound tests deleting non-existent entries
func TestGenericCacheFacade_Delete_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Delete a non-existent entry (should not error)
	err := facade.Delete(ctx, "non-existent-key")
	require.NoError(t, err)
}

// TestGenericCacheFacade_DeleteExpired tests deleting expired entries
func TestGenericCacheFacade_DeleteExpired(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Set multiple cache entries
	pastTime := time.Now().Add(-1 * time.Hour)
	futureTime := time.Now().Add(1 * time.Hour)
	
	// Expired entry
	err := facade.Set(ctx, "expired-1", map[string]interface{}{"data": "expired1"}, &pastTime)
	require.NoError(t, err)
	
	err = facade.Set(ctx, "expired-2", map[string]interface{}{"data": "expired2"}, &pastTime)
	require.NoError(t, err)
	
	// Valid entry
	err = facade.Set(ctx, "valid-1", map[string]interface{}{"data": "valid1"}, &futureTime)
	require.NoError(t, err)
	
	// No expiration
	err = facade.Set(ctx, "no-expiry", map[string]interface{}{"data": "noexpiry"}, nil)
	require.NoError(t, err)
	
	// Delete expired entries
	err = facade.DeleteExpired(ctx)
	require.NoError(t, err)
	
	// Verify expired entries are deleted
	var result map[string]interface{}
	err = facade.Get(ctx, "expired-1", &result)
	assert.Error(t, err)
	
	// Verify valid entries still exist
	err = facade.Get(ctx, "valid-1", &result)
	assert.NoError(t, err)
	
	err = facade.Get(ctx, "no-expiry", &result)
	assert.NoError(t, err)
	
	// Should have 2 entries left
	assert.Equal(t, int64(2), helper.Count(model.TableNameGenericCache))
}

// TestGenericCacheFacade_WithCluster tests the WithCluster method
func TestGenericCacheFacade_WithCluster(t *testing.T) {
	facade := NewGenericCacheFacade()
	
	clusterFacade := facade.WithCluster("test-cluster")
	
	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*GenericCacheFacadeInterface)(nil), clusterFacade)
}

// TestGenericCacheFacade_ConcurrentAccess tests concurrent access to the cache
func TestGenericCacheFacade_ConcurrentAccess(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	// Set initial values
	for i := 0; i < 10; i++ {
		key := "concurrent-key-" + string(rune('0'+i))
		value := map[string]interface{}{"counter": i}
		err := facade.Set(ctx, key, value, nil)
		require.NoError(t, err)
	}
	
	// Concurrent reads
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(index int) {
			key := "concurrent-key-" + string(rune('0'+index))
			var result map[string]interface{}
			err := facade.Get(ctx, key, &result)
			assert.NoError(t, err)
			done <- true
		}(i)
	}
	
	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestGenericCacheFacade_ComplexTypes tests various complex data types
func TestGenericCacheFacade_ComplexTypes(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := helper.CreateTestContext()
	
	tests := []struct {
		name  string
		value interface{}
	}{
		{
			name:  "Nested map",
			value: map[string]interface{}{"level1": map[string]interface{}{"level2": "value"}},
		},
		{
			name:  "Array of maps",
			value: []map[string]interface{}{{"key1": "value1"}, {"key2": "value2"}},
		},
		{
			name:  "Mixed types",
			value: map[string]interface{}{"string": "text", "number": 123, "bool": true, "null": nil},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "complex-" + tt.name
			
			err := facade.Set(ctx, key, tt.value, nil)
			require.NoError(t, err)
			
			var result interface{}
			err = facade.Get(ctx, key, &result)
			require.NoError(t, err)
		})
	}
}

// Benchmark tests
func BenchmarkGenericCacheFacade_Set(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := context.Background()
	value := map[string]interface{}{"data": "benchmark"}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = facade.Set(ctx, "bench-key", value, nil)
	}
}

func BenchmarkGenericCacheFacade_Get(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := context.Background()
	value := map[string]interface{}{"data": "benchmark"}
	
	// Pre-populate
	_ = facade.Set(ctx, "bench-key", value, nil)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		var result map[string]interface{}
		_ = facade.Get(ctx, "bench-key", &result)
	}
}

func BenchmarkGenericCacheFacade_Delete(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()
	
	facade := newTestGenericCacheFacade(helper.DB)
	ctx := context.Background()
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		value := map[string]interface{}{"data": "benchmark"}
		_ = facade.Set(ctx, "bench-key", value, nil)
		b.StartTimer()
		
		_ = facade.Delete(ctx, "bench-key")
	}
}

// Helper function to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}

