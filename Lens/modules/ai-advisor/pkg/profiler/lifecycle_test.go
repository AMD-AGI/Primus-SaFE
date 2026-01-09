// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package profiler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// DefaultLifecycleConfig Tests
// ============================================================================

func TestDefaultLifecycleConfig(t *testing.T) {
	config := DefaultLifecycleConfig()

	require.NotNil(t, config)
	assert.Equal(t, 30, config.DefaultRetentionDays)
	assert.Equal(t, 0.9, config.StorageThreshold)
	assert.True(t, config.SafeDelete)
	assert.Equal(t, 1, config.SafeDeleteWaitDays)

	// Check retention by type
	assert.NotNil(t, config.RetentionByType)
	assert.Equal(t, 30, config.RetentionByType["chrome_trace"])
	assert.Equal(t, 60, config.RetentionByType["stack_trace"])
	assert.Equal(t, 7, config.RetentionByType["memory_dump"])
	assert.Equal(t, 30, config.RetentionByType["kineto"])
}

// ============================================================================
// LifecycleConfig Tests
// ============================================================================

func TestLifecycleConfig_Fields(t *testing.T) {
	config := &LifecycleConfig{
		DefaultRetentionDays: 14,
		RetentionByType: map[string]int{
			"chrome_trace": 7,
			"memory_dump":  3,
		},
		RetentionByWorkload: map[string]int{
			"workload-1": 30,
			"workload-2": 60,
		},
		StorageThreshold:   0.85,
		SafeDelete:         false,
		SafeDeleteWaitDays: 3,
	}

	assert.Equal(t, 14, config.DefaultRetentionDays)
	assert.Equal(t, 7, config.RetentionByType["chrome_trace"])
	assert.Equal(t, 30, config.RetentionByWorkload["workload-1"])
	assert.Equal(t, 0.85, config.StorageThreshold)
	assert.False(t, config.SafeDelete)
	assert.Equal(t, 3, config.SafeDeleteWaitDays)
}

// ============================================================================
// NewLifecycleManager Tests
// ============================================================================

func TestNewLifecycleManager(t *testing.T) {
	storage := newMockStorageBackend()

	tests := []struct {
		name   string
		config *LifecycleConfig
	}{
		{
			name:   "with nil config uses defaults",
			config: nil,
		},
		{
			name: "with custom config",
			config: &LifecycleConfig{
				DefaultRetentionDays: 14,
				StorageThreshold:     0.8,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewLifecycleManager(nil, storage, tt.config)
			require.NotNil(t, manager)
			assert.NotNil(t, manager.config)
			assert.NotNil(t, manager.storageBackend)

			if tt.config == nil {
				// Should use default config
				assert.Equal(t, 30, manager.config.DefaultRetentionDays)
			} else {
				assert.Equal(t, tt.config.DefaultRetentionDays, manager.config.DefaultRetentionDays)
			}
		})
	}
}

// ============================================================================
// isFileExpired Tests
// ============================================================================

func TestLifecycleManager_isFileExpired(t *testing.T) {
	storage := newMockStorageBackend()

	tests := []struct {
		name     string
		config   *LifecycleConfig
		file     *ProfilerFileMetadata
		expected bool
	}{
		{
			name: "file expired by default retention",
			config: &LifecycleConfig{
				DefaultRetentionDays: 30,
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now().AddDate(0, 0, -31),
				FileType:    "unknown",
				WorkloadUID: "unknown-workload",
			},
			expected: true,
		},
		{
			name: "file not expired within default retention",
			config: &LifecycleConfig{
				DefaultRetentionDays: 30,
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now().AddDate(0, 0, -15),
				FileType:    "unknown",
				WorkloadUID: "unknown-workload",
			},
			expected: false,
		},
		{
			name: "file expired by type-specific retention",
			config: &LifecycleConfig{
				DefaultRetentionDays: 30,
				RetentionByType: map[string]int{
					"memory_dump": 7,
				},
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now().AddDate(0, 0, -10),
				FileType:    "memory_dump",
				WorkloadUID: "unknown-workload",
			},
			expected: true,
		},
		{
			name: "file not expired within type-specific retention",
			config: &LifecycleConfig{
				DefaultRetentionDays: 30,
				RetentionByType: map[string]int{
					"memory_dump": 7,
				},
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now().AddDate(0, 0, -5),
				FileType:    "memory_dump",
				WorkloadUID: "unknown-workload",
			},
			expected: false,
		},
		{
			name: "file expired by workload-specific retention",
			config: &LifecycleConfig{
				DefaultRetentionDays: 30,
				RetentionByWorkload: map[string]int{
					"special-workload": 7,
				},
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now().AddDate(0, 0, -10),
				FileType:    "chrome_trace",
				WorkloadUID: "special-workload",
			},
			expected: true,
		},
		{
			name: "workload policy takes precedence over type",
			config: &LifecycleConfig{
				DefaultRetentionDays: 30,
				RetentionByType: map[string]int{
					"chrome_trace": 60, // Would not be expired
				},
				RetentionByWorkload: map[string]int{
					"special-workload": 7, // Should be used
				},
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now().AddDate(0, 0, -10),
				FileType:    "chrome_trace",
				WorkloadUID: "special-workload",
			},
			expected: true, // Workload policy says expired
		},
		{
			name: "file just at retention boundary - expired",
			config: &LifecycleConfig{
				DefaultRetentionDays: 30,
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now().AddDate(0, 0, -30),
				FileType:    "unknown",
				WorkloadUID: "unknown-workload",
			},
			expected: true, // Exactly 30 days is at the boundary, considered expired
		},
		{
			name: "newly collected file",
			config: &LifecycleConfig{
				DefaultRetentionDays: 1,
			},
			file: &ProfilerFileMetadata{
				CollectedAt: time.Now(),
				FileType:    "chrome_trace",
				WorkloadUID: "workload",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewLifecycleManager(nil, storage, tt.config)
			result := manager.isFileExpired(tt.file)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// getTotalStorageSize Tests
// ============================================================================

func TestLifecycleManager_getTotalStorageSize(t *testing.T) {
	storage := newMockStorageBackend()
	manager := NewLifecycleManager(nil, storage, DefaultLifecycleConfig())

	tests := []struct {
		name     string
		files    []*ProfilerFileMetadata
		expected int64
	}{
		{
			name:     "empty file list",
			files:    []*ProfilerFileMetadata{},
			expected: 0,
		},
		{
			name: "single file",
			files: []*ProfilerFileMetadata{
				{FileSize: 1024},
			},
			expected: 1024,
		},
		{
			name: "multiple files",
			files: []*ProfilerFileMetadata{
				{FileSize: 1024},
				{FileSize: 2048},
				{FileSize: 512},
			},
			expected: 3584,
		},
		{
			name: "large files",
			files: []*ProfilerFileMetadata{
				{FileSize: 1073741824},     // 1 GB
				{FileSize: 2147483648},     // 2 GB
			},
			expected: 3221225472, // 3 GB
		},
		{
			name: "zero size files",
			files: []*ProfilerFileMetadata{
				{FileSize: 0},
				{FileSize: 0},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.getTotalStorageSize(tt.files)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// CleanupResult Tests
// ============================================================================

func TestCleanupResult_Fields(t *testing.T) {
	start := time.Now()
	end := start.Add(5 * time.Second)
	
	result := &CleanupResult{
		StartTime:    start,
		EndTime:      end,
		Duration:     end.Sub(start),
		TotalScanned: 100,
		DeletedCount: 25,
		FreedSpace:   1073741824, // 1 GB
		Errors:       []string{"error1", "error2"},
	}

	assert.Equal(t, start, result.StartTime)
	assert.Equal(t, end, result.EndTime)
	assert.Equal(t, 5*time.Second, result.Duration)
	assert.Equal(t, 100, result.TotalScanned)
	assert.Equal(t, 25, result.DeletedCount)
	assert.Equal(t, int64(1073741824), result.FreedSpace)
	assert.Len(t, result.Errors, 2)
}

// ============================================================================
// CleanupExpiredFiles Tests
// ============================================================================

func TestLifecycleManager_CleanupExpiredFiles(t *testing.T) {
	storage := newMockStorageBackend()
	manager := NewLifecycleManager(nil, storage, DefaultLifecycleConfig())

	ctx := context.Background()
	result, err := manager.CleanupExpiredFiles(ctx)

	// Currently returns empty result due to TODO implementation
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.TotalScanned)
	assert.Equal(t, 0, result.DeletedCount)
	assert.False(t, result.StartTime.IsZero())
	assert.False(t, result.EndTime.IsZero())
	assert.GreaterOrEqual(t, result.Duration.Nanoseconds(), int64(0))
}

// ============================================================================
// CleanupByStorageUsage Tests
// ============================================================================

func TestLifecycleManager_CleanupByStorageUsage(t *testing.T) {
	storage := newMockStorageBackend()

	tests := []struct {
		name         string
		config       *LifecycleConfig
		currentUsage float64
		shouldSkip   bool
	}{
		{
			name: "below threshold - skip",
			config: &LifecycleConfig{
				StorageThreshold: 0.9,
			},
			currentUsage: 0.7,
			shouldSkip:   true,
		},
		{
			name: "at threshold - cleanup",
			config: &LifecycleConfig{
				StorageThreshold: 0.9,
			},
			currentUsage: 0.9,
			shouldSkip:   false,
		},
		{
			name: "above threshold - cleanup",
			config: &LifecycleConfig{
				StorageThreshold: 0.9,
			},
			currentUsage: 0.95,
			shouldSkip:   false,
		},
		{
			name: "way above threshold",
			config: &LifecycleConfig{
				StorageThreshold: 0.8,
			},
			currentUsage: 0.99,
			shouldSkip:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := NewLifecycleManager(nil, storage, tt.config)
			ctx := context.Background()

			result, err := manager.CleanupByStorageUsage(ctx, tt.currentUsage)
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.shouldSkip {
				// When skipped, returns empty result
				assert.Equal(t, 0, result.TotalScanned)
			}
		})
	}
}

// ============================================================================
// MarkForDeletion Tests
// ============================================================================

func TestLifecycleManager_MarkForDeletion(t *testing.T) {
	storage := newMockStorageBackend()
	manager := NewLifecycleManager(nil, storage, DefaultLifecycleConfig())

	ctx := context.Background()
	err := manager.MarkForDeletion(ctx, 123)

	// Currently returns "not implemented yet" error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not implemented")
}

// ============================================================================
// DeleteMarkedFiles Tests
// ============================================================================

func TestLifecycleManager_DeleteMarkedFiles(t *testing.T) {
	storage := newMockStorageBackend()
	config := &LifecycleConfig{
		SafeDelete:         true,
		SafeDeleteWaitDays: 1,
	}
	manager := NewLifecycleManager(nil, storage, config)

	ctx := context.Background()
	result, err := manager.DeleteMarkedFiles(ctx)

	// Currently returns empty result due to TODO implementation
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.TotalScanned)
	assert.Equal(t, 0, result.DeletedCount)
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestLifecycleManager_Integration_FullCleanupFlow(t *testing.T) {
	storage := newMockStorageBackend()
	config := &LifecycleConfig{
		DefaultRetentionDays: 30,
		RetentionByType: map[string]int{
			"memory_dump":  7,
			"chrome_trace": 30,
		},
		RetentionByWorkload: map[string]int{
			"important-workload": 90,
		},
		StorageThreshold:   0.9,
		SafeDelete:         true,
		SafeDeleteWaitDays: 1,
	}
	manager := NewLifecycleManager(nil, storage, config)

	// Test file expiration check with various scenarios
	files := []*ProfilerFileMetadata{
		{
			ID:          1,
			WorkloadUID: "regular-workload",
			FileType:    "chrome_trace",
			FileSize:    1024,
			CollectedAt: time.Now().AddDate(0, 0, -31), // Expired
		},
		{
			ID:          2,
			WorkloadUID: "important-workload",
			FileType:    "chrome_trace",
			FileSize:    2048,
			CollectedAt: time.Now().AddDate(0, 0, -31), // Not expired due to workload policy
		},
		{
			ID:          3,
			WorkloadUID: "regular-workload",
			FileType:    "memory_dump",
			FileSize:    4096,
			CollectedAt: time.Now().AddDate(0, 0, -10), // Expired due to type policy
		},
	}

	// Check each file
	assert.True(t, manager.isFileExpired(files[0]))  // Regular workload, old chrome_trace
	assert.False(t, manager.isFileExpired(files[1])) // Important workload, 90 day retention
	assert.True(t, manager.isFileExpired(files[2]))  // Memory dump, 7 day retention

	// Test total storage calculation
	totalSize := manager.getTotalStorageSize(files)
	assert.Equal(t, int64(7168), totalSize) // 1024 + 2048 + 4096
}

// ============================================================================
// Edge Cases Tests
// ============================================================================

func TestLifecycleManager_EdgeCases(t *testing.T) {
	storage := newMockStorageBackend()

	t.Run("nil metadata manager", func(t *testing.T) {
		manager := NewLifecycleManager(nil, storage, nil)
		assert.NotNil(t, manager)
		assert.Nil(t, manager.metadataMgr)
	})

	t.Run("zero retention days", func(t *testing.T) {
		config := &LifecycleConfig{
			DefaultRetentionDays: 0,
		}
		manager := NewLifecycleManager(nil, storage, config)

		file := &ProfilerFileMetadata{
			CollectedAt: time.Now(),
			FileType:    "test",
			WorkloadUID: "test",
		}
		// With 0 retention days, file should be immediately expired
		assert.True(t, manager.isFileExpired(file))
	})

	t.Run("negative retention days", func(t *testing.T) {
		config := &LifecycleConfig{
			DefaultRetentionDays: -1,
		}
		manager := NewLifecycleManager(nil, storage, config)

		file := &ProfilerFileMetadata{
			CollectedAt: time.Now(),
			FileType:    "test",
			WorkloadUID: "test",
		}
		// With negative retention, file should be expired
		assert.True(t, manager.isFileExpired(file))
	})

	t.Run("empty retention maps", func(t *testing.T) {
		config := &LifecycleConfig{
			DefaultRetentionDays: 30,
			RetentionByType:      map[string]int{},
			RetentionByWorkload:  map[string]int{},
		}
		manager := NewLifecycleManager(nil, storage, config)

		file := &ProfilerFileMetadata{
			CollectedAt: time.Now().AddDate(0, 0, -15),
			FileType:    "chrome_trace",
			WorkloadUID: "workload",
		}
		// Should use default retention
		assert.False(t, manager.isFileExpired(file))
	})
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkLifecycleManager_isFileExpired(b *testing.B) {
	storage := newMockStorageBackend()
	config := &LifecycleConfig{
		DefaultRetentionDays: 30,
		RetentionByType: map[string]int{
			"chrome_trace": 30,
			"memory_dump":  7,
		},
		RetentionByWorkload: map[string]int{
			"special-workload": 60,
		},
	}
	manager := NewLifecycleManager(nil, storage, config)

	file := &ProfilerFileMetadata{
		CollectedAt: time.Now().AddDate(0, 0, -20),
		FileType:    "chrome_trace",
		WorkloadUID: "regular-workload",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.isFileExpired(file)
	}
}

func BenchmarkLifecycleManager_getTotalStorageSize(b *testing.B) {
	storage := newMockStorageBackend()
	manager := NewLifecycleManager(nil, storage, DefaultLifecycleConfig())

	files := make([]*ProfilerFileMetadata, 100)
	for i := 0; i < 100; i++ {
		files[i] = &ProfilerFileMetadata{
			FileSize: int64(1024 * (i + 1)),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.getTotalStorageSize(files)
	}
}

