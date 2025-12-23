package task

import (
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/stretchr/testify/assert"
)

func TestBaseExecutor_GetExtString(t *testing.T) {
	executor := &BaseExecutor{}

	tests := []struct {
		name     string
		task     *model.WorkloadTaskState
		key      string
		expected string
	}{
		{
			name:     "nil ext",
			task:     &model.WorkloadTaskState{Ext: nil},
			key:      "key",
			expected: "",
		},
		{
			name:     "key not found",
			task:     &model.WorkloadTaskState{Ext: map[string]interface{}{}},
			key:      "missing",
			expected: "",
		},
		{
			name: "string value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"name": "test-value",
				},
			},
			key:      "name",
			expected: "test-value",
		},
		{
			name: "non-string value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"count": 123,
				},
			},
			key:      "count",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.GetExtString(tt.task, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseExecutor_GetExtInt(t *testing.T) {
	executor := &BaseExecutor{}

	tests := []struct {
		name     string
		task     *model.WorkloadTaskState
		key      string
		expected int
	}{
		{
			name:     "nil ext",
			task:     &model.WorkloadTaskState{Ext: nil},
			key:      "key",
			expected: 0,
		},
		{
			name:     "key not found",
			task:     &model.WorkloadTaskState{Ext: map[string]interface{}{}},
			key:      "missing",
			expected: 0,
		},
		{
			name: "float64 value (JSON unmarshal)",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"count": float64(42),
				},
			},
			key:      "count",
			expected: 42,
		},
		{
			name: "int value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"count": 100,
				},
			},
			key:      "count",
			expected: 100,
		},
		{
			name: "non-numeric value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"name": "not a number",
				},
			},
			key:      "name",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.GetExtInt(tt.task, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseExecutor_GetExtBool(t *testing.T) {
	executor := &BaseExecutor{}

	tests := []struct {
		name     string
		task     *model.WorkloadTaskState
		key      string
		expected bool
	}{
		{
			name:     "nil ext",
			task:     &model.WorkloadTaskState{Ext: nil},
			key:      "key",
			expected: false,
		},
		{
			name:     "key not found",
			task:     &model.WorkloadTaskState{Ext: map[string]interface{}{}},
			key:      "missing",
			expected: false,
		},
		{
			name: "true value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"enabled": true,
				},
			},
			key:      "enabled",
			expected: true,
		},
		{
			name: "false value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"enabled": false,
				},
			},
			key:      "enabled",
			expected: false,
		},
		{
			name: "non-bool value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"name": "not a bool",
				},
			},
			key:      "name",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.GetExtBool(tt.task, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseExecutor_GetExtMap(t *testing.T) {
	executor := &BaseExecutor{}

	tests := []struct {
		name     string
		task     *model.WorkloadTaskState
		key      string
		expected map[string]interface{}
	}{
		{
			name:     "nil ext",
			task:     &model.WorkloadTaskState{Ext: nil},
			key:      "key",
			expected: nil,
		},
		{
			name:     "key not found",
			task:     &model.WorkloadTaskState{Ext: map[string]interface{}{}},
			key:      "missing",
			expected: nil,
		},
		{
			name: "map value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"config": map[string]interface{}{
						"timeout": 30,
						"enabled": true,
					},
				},
			},
			key: "config",
			expected: map[string]interface{}{
				"timeout": 30,
				"enabled": true,
			},
		},
		{
			name: "non-map value",
			task: &model.WorkloadTaskState{
				Ext: map[string]interface{}{
					"name": "not a map",
				},
			},
			key:      "name",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := executor.GetExtMap(tt.task, tt.key)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseExecutor_SetExtValue(t *testing.T) {
	executor := &BaseExecutor{}

	t.Run("set value in existing map", func(t *testing.T) {
		updates := map[string]interface{}{
			"existing": "value",
		}
		executor.SetExtValue(updates, "new_key", "new_value")
		assert.Equal(t, "new_value", updates["new_key"])
		assert.Equal(t, "value", updates["existing"])
	})

	t.Run("set value with different types", func(t *testing.T) {
		updates := make(map[string]interface{})
		executor.SetExtValue(updates, "string", "hello")
		executor.SetExtValue(updates, "int", 42)
		executor.SetExtValue(updates, "bool", true)

		assert.Equal(t, "hello", updates["string"])
		assert.Equal(t, 42, updates["int"])
		assert.Equal(t, true, updates["bool"])
	})
}

func TestSuccessResult(t *testing.T) {
	updates := map[string]interface{}{
		"result_id": "123",
	}

	result := SuccessResult(updates)

	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Equal(t, constant.TaskStatusCompleted, result.NewStatus)
	assert.Equal(t, updates, result.UpdateExt)
}

func TestSuccessResultNilUpdates(t *testing.T) {
	result := SuccessResult(nil)

	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Equal(t, constant.TaskStatusCompleted, result.NewStatus)
	assert.Nil(t, result.UpdateExt)
}

func TestFailureResult(t *testing.T) {
	updates := map[string]interface{}{
		"last_attempt": "2024-01-01",
	}

	result := FailureResult("connection timeout", updates)

	assert.False(t, result.Success)
	assert.Equal(t, "connection timeout", result.Error)
	assert.Equal(t, constant.TaskStatusFailed, result.NewStatus)
	assert.Equal(t, updates, result.UpdateExt)
}

func TestFailureResultNilUpdates(t *testing.T) {
	result := FailureResult("some error", nil)

	assert.False(t, result.Success)
	assert.Equal(t, "some error", result.Error)
	assert.Equal(t, constant.TaskStatusFailed, result.NewStatus)
	assert.Nil(t, result.UpdateExt)
}

func TestProgressResult(t *testing.T) {
	updates := map[string]interface{}{
		"progress": 50,
	}

	result := ProgressResult(updates)

	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Equal(t, constant.TaskStatusRunning, result.NewStatus)
	assert.Equal(t, updates, result.UpdateExt)
}

func TestProgressResultNilUpdates(t *testing.T) {
	result := ProgressResult(nil)

	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Equal(t, constant.TaskStatusRunning, result.NewStatus)
	assert.Nil(t, result.UpdateExt)
}

func TestExecutionContextStruct(t *testing.T) {
	task := &model.WorkloadTaskState{
		ID:       1,
		TaskType: "test_task",
	}

	ctx := &ExecutionContext{
		Task:       task,
		InstanceID: "instance-123",
		Cancel:     nil,
	}

	assert.Equal(t, task, ctx.Task)
	assert.Equal(t, "instance-123", ctx.InstanceID)
	assert.Nil(t, ctx.Cancel)
}

func TestExecutionResultStruct(t *testing.T) {
	result := &ExecutionResult{
		Success: true,
		Error:   "",
		UpdateExt: map[string]interface{}{
			"key": "value",
		},
		NewStatus: constant.TaskStatusCompleted,
	}

	assert.True(t, result.Success)
	assert.Empty(t, result.Error)
	assert.Contains(t, result.UpdateExt, "key")
	assert.Equal(t, constant.TaskStatusCompleted, result.NewStatus)
}

