package database

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// mockSystemConfigFacade creates a SystemConfigFacade with the test database
type mockSystemConfigFacade struct {
	SystemConfigFacade
	db *gorm.DB
}

func (f *mockSystemConfigFacade) getDB() *gorm.DB {
	return f.db
}

// newTestSystemConfigFacade creates a test SystemConfigFacade
func newTestSystemConfigFacade(db *gorm.DB) SystemConfigFacadeInterface {
	return &mockSystemConfigFacade{
		db: db,
	}
}

// TestSystemConfigFacade_Create tests creating system config entries
func TestSystemConfigFacade_Create(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	config := &model.SystemConfig{
		Key:         "test.config.key",
		Value:       model.ExtType{"setting": "value"},
		Description: "Test configuration",
		Category:    "test",
		IsEncrypted: false,
		Version:     1,
		IsReadonly:  false,
		CreatedBy:   "test-user",
		UpdatedBy:   "test-user",
	}

	err := facade.Create(ctx, config)
	require.NoError(t, err)
	assert.NotZero(t, config.ID)
	assert.NotZero(t, config.CreatedAt)
	assert.NotZero(t, config.UpdatedAt)
}

// TestSystemConfigFacade_GetByKey tests getting config by key
func TestSystemConfigFacade_GetByKey(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create a config entry
	config := &model.SystemConfig{
		Key:         "test.get.key",
		Value:       model.ExtType{"data": "value"},
		Description: "Test get config",
		Category:    "test",
	}
	err := facade.Create(ctx, config)
	require.NoError(t, err)

	// Get the config
	result, err := facade.GetByKey(ctx, "test.get.key")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, config.Key, result.Key)
	assert.Equal(t, config.Description, result.Description)
	assert.Equal(t, config.Category, result.Category)
	assert.Equal(t, "value", result.Value["data"])
}

// TestSystemConfigFacade_GetByKey_NotFound tests getting non-existent config
func TestSystemConfigFacade_GetByKey_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	result, err := facade.GetByKey(ctx, "non.existent.key")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestSystemConfigFacade_Update tests updating config entries
func TestSystemConfigFacade_Update(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create a config entry
	config := &model.SystemConfig{
		Key:         "test.update.key",
		Value:       model.ExtType{"counter": float64(1)},
		Description: "Test update config",
		Category:    "test",
		Version:     1,
	}
	err := facade.Create(ctx, config)
	require.NoError(t, err)

	// Update the config
	updates := map[string]interface{}{
		"value":       model.ExtType{"counter": float64(2)},
		"description": "Updated description",
		"version":     2,
	}
	err = facade.Update(ctx, config, updates)
	require.NoError(t, err)

	// Verify the update
	result, err := facade.GetByKey(ctx, "test.update.key")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, float64(2), result.Value["counter"])
	assert.Equal(t, "Updated description", result.Description)
	assert.Equal(t, int32(2), result.Version)
}

// TestSystemConfigFacade_Delete tests deleting config entries
func TestSystemConfigFacade_Delete(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create a config entry
	config := &model.SystemConfig{
		Key:   "test.delete.key",
		Value: model.ExtType{},
	}
	err := facade.Create(ctx, config)
	require.NoError(t, err)

	// Delete the config
	err = facade.Delete(ctx, "test.delete.key")
	require.NoError(t, err)

	// Verify it's deleted
	result, err := facade.GetByKey(ctx, "test.delete.key")
	require.NoError(t, err)
	assert.Nil(t, result)
}

// TestSystemConfigFacade_Delete_NotFound tests deleting non-existent config
func TestSystemConfigFacade_Delete_NotFound(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Delete a non-existent config (should not error)
	err := facade.Delete(ctx, "non.existent.key")
	require.NoError(t, err)
}

// TestSystemConfigFacade_BatchGet tests getting multiple configs by keys
func TestSystemConfigFacade_BatchGet(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create multiple config entries
	keys := []string{"test.batch.key1", "test.batch.key2", "test.batch.key3"}
	for _, key := range keys {
		config := &model.SystemConfig{
			Key:   key,
			Value: model.ExtType{"key": key},
		}
		err := facade.Create(ctx, config)
		require.NoError(t, err)
	}

	// Batch get
	results, err := facade.BatchGet(ctx, keys)
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Verify all keys are present
	resultKeys := make(map[string]bool)
	for _, result := range results {
		resultKeys[result.Key] = true
	}
	for _, key := range keys {
		assert.True(t, resultKeys[key], "Key %s should be present", key)
	}
}

// TestSystemConfigFacade_BatchGet_PartialMatch tests batch get with some missing keys
func TestSystemConfigFacade_BatchGet_PartialMatch(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create only one config entry
	config := &model.SystemConfig{
		Key:   "test.partial.key1",
		Value: model.ExtType{},
	}
	err := facade.Create(ctx, config)
	require.NoError(t, err)

	// Batch get with both existent and non-existent keys
	keys := []string{"test.partial.key1", "test.partial.key2", "test.partial.key3"}
	results, err := facade.BatchGet(ctx, keys)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "test.partial.key1", results[0].Key)
}

// TestSystemConfigFacade_Exists tests checking if a config key exists
func TestSystemConfigFacade_Exists(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create a config entry
	config := &model.SystemConfig{
		Key:   "test.exists.key",
		Value: model.ExtType{},
	}
	err := facade.Create(ctx, config)
	require.NoError(t, err)

	// Check exists
	exists, err := facade.Exists(ctx, "test.exists.key")
	require.NoError(t, err)
	assert.True(t, exists)

	// Check non-existent key
	exists, err = facade.Exists(ctx, "test.notexists.key")
	require.NoError(t, err)
	assert.False(t, exists)
}

// TestSystemConfigFacade_List tests listing configs
func TestSystemConfigFacade_List(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create multiple config entries
	for i := 0; i < 5; i++ {
		config := &model.SystemConfig{
			Key:      "test.list.key." + string(rune('a'+i)),
			Value:    model.ExtType{},
			Category: "test",
		}
		err := facade.Create(ctx, config)
		require.NoError(t, err)
	}

	// List all configs
	query := helper.DB
	results, err := facade.List(ctx, query)
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

// TestSystemConfigFacade_List_WithFilter tests listing with filters
func TestSystemConfigFacade_List_WithFilter(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create config entries with different categories
	categories := []string{"category1", "category2", "category1"}
	for i, cat := range categories {
		config := &model.SystemConfig{
			Key:      "test.filter.key." + string(rune('a'+i)),
			Value:    model.ExtType{},
			Category: cat,
		}
		err := facade.Create(ctx, config)
		require.NoError(t, err)
	}

	// List with category filter
	query := helper.DB.Where("category = ?", "category1")
	results, err := facade.List(ctx, query)
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

// TestSystemConfigFacade_CreateHistory tests creating history records
func TestSystemConfigFacade_CreateHistory(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	history := &model.SystemConfigHistory{
		Key:          "test.history.key",
		OldValue:     model.ExtType{"version": float64(1)},
		NewValue:     model.ExtType{"version": float64(2)},
		ChangedBy:    "test-user",
		ChangeReason: "Updated version",
	}

	err := facade.CreateHistory(ctx, history)
	require.NoError(t, err)
	assert.NotZero(t, history.ID)
	assert.NotZero(t, history.ChangedAt)
}

// TestSystemConfigFacade_GetHistory tests getting history records
func TestSystemConfigFacade_GetHistory(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	// Create multiple history records
	key := "test.history.key"
	for i := 0; i < 5; i++ {
		history := &model.SystemConfigHistory{
			Key:      key,
			OldValue: model.ExtType{"version": float64(i)},
			NewValue: model.ExtType{"version": float64(i + 1)},
		}
		err := facade.CreateHistory(ctx, history)
		require.NoError(t, err)
	}

	// Get history with limit
	results, err := facade.GetHistory(ctx, key, 3)
	require.NoError(t, err)
	assert.Len(t, results, 3)

	// Get all history (no limit)
	results, err = facade.GetHistory(ctx, key, 0)
	require.NoError(t, err)
	assert.Len(t, results, 5)
}

// TestSystemConfigFacade_GetHistory_Empty tests getting history for non-existent key
func TestSystemConfigFacade_GetHistory_Empty(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	results, err := facade.GetHistory(ctx, "non.existent.key", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSystemConfigFacade_WithCluster tests the WithCluster method
func TestSystemConfigFacade_WithCluster(t *testing.T) {
	facade := NewSystemConfigFacade()

	clusterFacade := facade.WithCluster("test-cluster")

	require.NotNil(t, clusterFacade)
	assert.Implements(t, (*SystemConfigFacadeInterface)(nil), clusterFacade)
}

// TestSystemConfigFacade_GetDB tests the GetDB method
func TestSystemConfigFacade_GetDB(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)

	db := facade.GetDB()
	require.NotNil(t, db)
}

// TestSystemConfigFacade_Flags tests various boolean flags
func TestSystemConfigFacade_Flags(t *testing.T) {
	helper := NewTestHelper(t)
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := helper.CreateTestContext()

	tests := []struct {
		name        string
		isEncrypted bool
		isReadonly  bool
	}{
		{
			name:        "Normal config",
			isEncrypted: false,
			isReadonly:  false,
		},
		{
			name:        "Encrypted config",
			isEncrypted: true,
			isReadonly:  false,
		},
		{
			name:        "Readonly config",
			isEncrypted: false,
			isReadonly:  true,
		},
		{
			name:        "Encrypted and readonly",
			isEncrypted: true,
			isReadonly:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &model.SystemConfig{
				Key:         "test.flags." + tt.name,
				Value:       model.ExtType{},
				IsEncrypted: tt.isEncrypted,
				IsReadonly:  tt.isReadonly,
			}

			err := facade.Create(ctx, config)
			require.NoError(t, err)

			result, err := facade.GetByKey(ctx, config.Key)
			require.NoError(t, err)
			require.NotNil(t, result)

			assert.Equal(t, tt.isEncrypted, result.IsEncrypted)
			assert.Equal(t, tt.isReadonly, result.IsReadonly)
		})
	}
}

// Benchmark tests
func BenchmarkSystemConfigFacade_Create(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		config := &model.SystemConfig{
			Key:   "bench.key",
			Value: model.ExtType{},
		}
		_ = facade.Create(ctx, config)
	}
}

func BenchmarkSystemConfigFacade_GetByKey(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := context.Background()

	// Pre-populate
	config := &model.SystemConfig{
		Key:   "bench.key",
		Value: model.ExtType{},
	}
	_ = facade.Create(ctx, config)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = facade.GetByKey(ctx, "bench.key")
	}
}

func BenchmarkSystemConfigFacade_BatchGet(b *testing.B) {
	helper := NewTestHelper(&testing.T{})
	defer helper.Cleanup()

	facade := newTestSystemConfigFacade(helper.DB)
	ctx := context.Background()

	// Pre-populate with 10 entries
	keys := make([]string, 10)
	for i := 0; i < 10; i++ {
		key := "bench.batch.key." + string(rune('0'+i))
		keys[i] = key
		config := &model.SystemConfig{
			Key:   key,
			Value: model.ExtType{},
		}
		_ = facade.Create(ctx, config)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = facade.BatchGet(ctx, keys)
	}
}
