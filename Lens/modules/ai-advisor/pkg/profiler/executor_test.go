package profiler

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// NewProfilerCollectionExecutor Tests
// ============================================================================

func TestNewProfilerCollectionExecutor(t *testing.T) {
	storage := newMockStorageBackend()
	config := &CollectorConfig{
		AutoCollect: true,
	}
	collector, err := NewCollector(config, storage, "http://localhost:8080")
	require.NoError(t, err)

	metadataMgr, _ := NewMetadataManager(nil)

	executor := NewProfilerCollectionExecutor(collector, metadataMgr, nil)

	require.NotNil(t, executor)
	assert.NotNil(t, executor.collector)
	assert.NotNil(t, executor.configService)
}

func TestNewProfilerCollectionExecutor_NilDependencies(t *testing.T) {
	// Should handle nil dependencies gracefully
	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	require.NotNil(t, executor)
	assert.Nil(t, executor.collector)
	assert.Nil(t, executor.metadataMgr)
}

// ============================================================================
// GetTaskType Tests
// ============================================================================

func TestProfilerCollectionExecutor_GetTaskType(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	taskType := executor.GetTaskType()
	assert.Equal(t, constant.TaskTypeProfilerCollection, taskType)
}

// ============================================================================
// Validate Tests
// ============================================================================

func TestProfilerCollectionExecutor_Validate(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)

	tests := []struct {
		name      string
		taskState *model.WorkloadTaskState
		wantErr   bool
		errMsg    string
	}{
		{
			name: "valid task state",
			taskState: &model.WorkloadTaskState{
				WorkloadUID: "test-workload-uid",
				TaskType:    constant.TaskTypeProfilerCollection,
			},
			wantErr: false,
		},
		{
			name: "empty workload uid",
			taskState: &model.WorkloadTaskState{
				WorkloadUID: "",
				TaskType:    constant.TaskTypeProfilerCollection,
			},
			wantErr: true,
			errMsg:  "workload_uid is required",
		},
		{
			name: "with ext data",
			taskState: &model.WorkloadTaskState{
				WorkloadUID: "test-uid",
				TaskType:    constant.TaskTypeProfilerCollection,
				Ext: model.ExtType{
					"auto_collect": true,
					"max_retries":  3,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := executor.Validate(tt.taskState)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
// extractTaskConfig Tests
// ============================================================================

func TestProfilerCollectionExecutor_extractTaskConfig(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)

	tests := []struct {
		name         string
		taskState    *model.WorkloadTaskState
		expectedAuto bool
	}{
		{
			name: "with auto_collect true",
			taskState: &model.WorkloadTaskState{
				WorkloadUID: "test-uid",
				Ext: model.ExtType{
					"auto_collect": true,
				},
			},
			expectedAuto: true,
		},
		{
			name: "with auto_collect false",
			taskState: &model.WorkloadTaskState{
				WorkloadUID: "test-uid",
				Ext: model.ExtType{
					"auto_collect": false,
				},
			},
			expectedAuto: false,
		},
		{
			name: "without auto_collect",
			taskState: &model.WorkloadTaskState{
				WorkloadUID: "test-uid",
				Ext:         model.ExtType{},
			},
			expectedAuto: false,
		},
		{
			name: "nil ext",
			taskState: &model.WorkloadTaskState{
				WorkloadUID: "test-uid",
				Ext:         nil,
			},
			expectedAuto: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := executor.extractTaskConfig(tt.taskState)
			require.NotNil(t, config)
			assert.Equal(t, tt.expectedAuto, config.AutoCollect)
		})
	}
}

// ============================================================================
// shouldContinue Tests (with mocked workload)
// ============================================================================

func TestProfilerCollectionExecutor_shouldContinue_AutoRestartFalse(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	ctx := context.Background()

	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-uid",
		Ext: model.ExtType{
			"auto_restart": false,
		},
	}

	result := executor.shouldContinue(ctx, taskState, 1)
	assert.False(t, result)
}

func TestProfilerCollectionExecutor_shouldContinue_MaxExecutions(t *testing.T) {
	// Skip this test as it requires database connection
	t.Skip("Requires database connection - integration test")

	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	ctx := context.Background()

	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-uid",
		Ext: model.ExtType{
			"auto_restart":   true,
			"max_executions": 5,
		},
	}

	// Not at max yet
	result := executor.shouldContinue(ctx, taskState, 4)
	assert.True(t, result)

	// At max
	result = executor.shouldContinue(ctx, taskState, 5)
	assert.False(t, result)

	// Over max
	result = executor.shouldContinue(ctx, taskState, 6)
	assert.False(t, result)
}

func TestProfilerCollectionExecutor_shouldContinue_UnlimitedExecutions(t *testing.T) {
	// Skip this test as it requires database connection
	t.Skip("Requires database connection - integration test")

	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	ctx := context.Background()

	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-uid",
		Ext: model.ExtType{
			"auto_restart":   true,
			"max_executions": 0, // Unlimited
		},
	}

	// Should continue with high execution count
	result := executor.shouldContinue(ctx, taskState, 100)
	// Will return true because max_executions is 0 (unlimited)
	// But workload lookup will fail, so it returns true by default
	assert.True(t, result)
}

// ============================================================================
// Cancel Tests
// ============================================================================

func TestProfilerCollectionExecutor_Cancel(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	ctx := context.Background()

	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-uid",
		TaskType:    constant.TaskTypeProfilerCollection,
	}

	err := executor.Cancel(ctx, taskState)
	assert.NoError(t, err)
}

// ============================================================================
// loadFrameworkConfig Tests (with mocked metadata)
// ============================================================================

func TestProfilerCollectionExecutor_loadFrameworkConfig_NoMetadata(t *testing.T) {
	// Skip this test as it requires database connection
	t.Skip("Requires database connection - integration test")

	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	ctx := context.Background()

	// This will fail because metadataFacade is not properly initialized
	config, err := executor.loadFrameworkConfig(ctx, "test-workload")
	assert.Error(t, err)
	assert.Nil(t, config)
}

// ============================================================================
// Model Tests
// ============================================================================

func TestWorkloadTaskState_ExtType(t *testing.T) {
	ext := model.ExtType{
		"string_key": "value",
		"int_key":    42,
		"bool_key":   true,
		"float_key":  3.14,
	}

	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-uid",
		TaskType:    "test-type",
		Status:      "pending",
		Ext:         ext,
	}

	assert.Equal(t, "test-uid", taskState.WorkloadUID)
	assert.Equal(t, "test-type", taskState.TaskType)
	assert.Equal(t, "pending", taskState.Status)
	assert.Equal(t, "value", taskState.Ext["string_key"])
}

// ============================================================================
// BaseExecutor Helper Tests
// ============================================================================

func TestProfilerCollectionExecutor_GetExtBool(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)

	tests := []struct {
		name     string
		ext      model.ExtType
		key      string
		expected bool
	}{
		{
			name:     "true value",
			ext:      model.ExtType{"key": true},
			key:      "key",
			expected: true,
		},
		{
			name:     "false value",
			ext:      model.ExtType{"key": false},
			key:      "key",
			expected: false,
		},
		{
			name:     "missing key",
			ext:      model.ExtType{},
			key:      "key",
			expected: false,
		},
		{
			name:     "nil ext",
			ext:      nil,
			key:      "key",
			expected: false,
		},
		{
			name:     "non-bool value",
			ext:      model.ExtType{"key": "not a bool"},
			key:      "key",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskState := &model.WorkloadTaskState{Ext: tt.ext}
			result := executor.GetExtBool(taskState, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfilerCollectionExecutor_GetExtInt(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)

	tests := []struct {
		name     string
		ext      model.ExtType
		key      string
		expected int
	}{
		{
			name:     "int value",
			ext:      model.ExtType{"key": 42},
			key:      "key",
			expected: 42,
		},
		{
			name:     "float64 value",
			ext:      model.ExtType{"key": float64(42)},
			key:      "key",
			expected: 42,
		},
		{
			name:     "missing key",
			ext:      model.ExtType{},
			key:      "key",
			expected: 0,
		},
		{
			name:     "nil ext",
			ext:      nil,
			key:      "key",
			expected: 0,
		},
		{
			name:     "non-int value",
			ext:      model.ExtType{"key": "not an int"},
			key:      "key",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskState := &model.WorkloadTaskState{Ext: tt.ext}
			result := executor.GetExtInt(taskState, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProfilerCollectionExecutor_GetExtString(t *testing.T) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)

	tests := []struct {
		name     string
		ext      model.ExtType
		key      string
		expected string
	}{
		{
			name:     "string value",
			ext:      model.ExtType{"key": "value"},
			key:      "key",
			expected: "value",
		},
		{
			name:     "missing key",
			ext:      model.ExtType{},
			key:      "key",
			expected: "",
		},
		{
			name:     "nil ext",
			ext:      nil,
			key:      "key",
			expected: "",
		},
		{
			name:     "non-string value",
			ext:      model.ExtType{"key": 42},
			key:      "key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskState := &model.WorkloadTaskState{Ext: tt.ext}
			result := executor.GetExtString(taskState, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// CollectorConfig Tests
// ============================================================================

func TestCollectorConfig_Fields(t *testing.T) {
	config := &CollectorConfig{
		AutoCollect: true,
	}

	assert.True(t, config.AutoCollect)
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestProfilerCollectionExecutor_Integration(t *testing.T) {
	storage := newMockStorageBackend()
	config := &CollectorConfig{
		AutoCollect: true,
	}
	collector, err := NewCollector(config, storage, "http://localhost:8080")
	require.NoError(t, err)

	executor := NewProfilerCollectionExecutor(collector, nil, nil)
	require.NotNil(t, executor)

	// Test task type
	assert.Equal(t, constant.TaskTypeProfilerCollection, executor.GetTaskType())

	// Test validation
	validTask := &model.WorkloadTaskState{
		WorkloadUID: "test-uid",
	}
	assert.NoError(t, executor.Validate(validTask))

	invalidTask := &model.WorkloadTaskState{
		WorkloadUID: "",
	}
	assert.Error(t, executor.Validate(invalidTask))
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkProfilerCollectionExecutor_Validate(b *testing.B) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-workload-uid",
		TaskType:    constant.TaskTypeProfilerCollection,
		Ext: model.ExtType{
			"auto_collect": true,
			"max_retries":  3,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = executor.Validate(taskState)
	}
}

func BenchmarkProfilerCollectionExecutor_extractTaskConfig(b *testing.B) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-uid",
		Ext: model.ExtType{
			"auto_collect":   true,
			"min_confidence": "medium",
			"max_file_size":  1073741824,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = executor.extractTaskConfig(taskState)
	}
}

func BenchmarkProfilerCollectionExecutor_GetExtHelpers(b *testing.B) {
	executor := NewProfilerCollectionExecutor(nil, nil, nil)
	taskState := &model.WorkloadTaskState{
		Ext: model.ExtType{
			"bool_key":   true,
			"int_key":    42,
			"string_key": "value",
		},
	}

	b.Run("GetExtBool", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = executor.GetExtBool(taskState, "bool_key")
		}
	})

	b.Run("GetExtInt", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = executor.GetExtInt(taskState, "int_key")
		}
	})

	b.Run("GetExtString", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = executor.GetExtString(taskState, "string_key")
		}
	})
}
