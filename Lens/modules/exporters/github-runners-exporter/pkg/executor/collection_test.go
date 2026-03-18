// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package executor

import (
	"context"
	"testing"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/stretchr/testify/assert"
)

// MockCollector implements CollectorInterface for testing
type MockCollector struct {
	Result *CollectionResult
	Error  error
}

func (m *MockCollector) CollectRun(ctx context.Context, runID int64) (*CollectionResult, error) {
	return m.Result, m.Error
}

func TestNewCollectionExecutor(t *testing.T) {
	executor := NewCollectionExecutor(nil)

	assert.NotNil(t, executor)
	assert.Equal(t, constant.TaskTypeGithubWorkflowCollection, executor.GetTaskType())
}

func TestCollectionExecutor_Validate(t *testing.T) {
	executor := NewCollectionExecutor(nil)

	t.Run("valid task with run_id", func(t *testing.T) {
		taskState := &model.WorkloadTaskState{
			WorkloadUID: "test-task-1",
			TaskType:    constant.TaskTypeGithubWorkflowCollection,
			Ext: model.ExtType{
				ExtKeyRunID: float64(123), // JSON numbers are float64
			},
		}

		err := executor.Validate(taskState)
		assert.NoError(t, err)
	})

	t.Run("invalid task without run_id", func(t *testing.T) {
		taskState := &model.WorkloadTaskState{
			WorkloadUID: "test-task-2",
			TaskType:    constant.TaskTypeGithubWorkflowCollection,
			Ext:         model.ExtType{},
		}

		err := executor.Validate(taskState)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "run_id")
	})

	t.Run("invalid task with nil ext", func(t *testing.T) {
		taskState := &model.WorkloadTaskState{
			WorkloadUID: "test-task-3",
			TaskType:    constant.TaskTypeGithubWorkflowCollection,
			Ext:         nil,
		}

		err := executor.Validate(taskState)
		assert.Error(t, err)
	})
}

func TestCollectionExecutor_Cancel(t *testing.T) {
	executor := NewCollectionExecutor(nil)

	taskState := &model.WorkloadTaskState{
		WorkloadUID: "test-task",
	}

	err := executor.Cancel(context.Background(), taskState)
	assert.NoError(t, err)
}

func TestCollectionExecutor_GetTaskType(t *testing.T) {
	executor := NewCollectionExecutor(nil)

	assert.Equal(t, constant.TaskTypeGithubWorkflowCollection, executor.GetTaskType())
}

func TestCollectionExecutor_SetCollector(t *testing.T) {
	executor := NewCollectionExecutor(nil)

	mockCollector := &MockCollector{
		Result: &CollectionResult{
			Success:      true,
			MetricsCount: 10,
		},
	}

	executor.SetCollector(mockCollector)
	assert.NotNil(t, executor.collector)
}

func TestCollectionResult(t *testing.T) {
	t.Run("success result", func(t *testing.T) {
		result := &CollectionResult{
			Success:      true,
			MetricsCount: 5,
		}
		assert.True(t, result.Success)
		assert.Equal(t, 5, result.MetricsCount)
	})

	t.Run("skipped result", func(t *testing.T) {
		result := &CollectionResult{
			Success:    true,
			SkipReason: "config disabled",
		}
		assert.True(t, result.Success)
		assert.Equal(t, "config disabled", result.SkipReason)
	})

	t.Run("error result", func(t *testing.T) {
		result := &CollectionResult{
			Success: false,
			Error:   "collection failed",
		}
		assert.False(t, result.Success)
		assert.Equal(t, "collection failed", result.Error)
	})
}

func TestExtKeyConstants(t *testing.T) {
	// Verify constants are defined correctly
	assert.Equal(t, "run_id", ExtKeyRunID)
	assert.Equal(t, "runner_set_id", ExtKeyRunnerSetID)
	assert.Equal(t, "config_id", ExtKeyConfigID)
	assert.Equal(t, "workload_name", ExtKeyWorkloadName)
	assert.Equal(t, "collection_result", ExtKeyCollectionResult)
	assert.Equal(t, "metrics_count", ExtKeyMetricsCount)
	assert.Equal(t, "error_message", ExtKeyErrorMessage)
	assert.Equal(t, "retry_count", ExtKeyRetryCount)
}

func TestBaseExecutorHelpers(t *testing.T) {
	executor := NewCollectionExecutor(nil)

	taskState := &model.WorkloadTaskState{
		Ext: model.ExtType{
			"string_val": "test",
			"int_val":    float64(42),
			"bool_val":   true,
			"map_val": map[string]interface{}{
				"key": "value",
			},
		},
	}

	t.Run("GetExtString", func(t *testing.T) {
		assert.Equal(t, "test", executor.GetExtString(taskState, "string_val"))
		assert.Equal(t, "", executor.GetExtString(taskState, "nonexistent"))
	})

	t.Run("GetExtInt", func(t *testing.T) {
		assert.Equal(t, 42, executor.GetExtInt(taskState, "int_val"))
		assert.Equal(t, 0, executor.GetExtInt(taskState, "nonexistent"))
	})

	t.Run("GetExtBool", func(t *testing.T) {
		assert.True(t, executor.GetExtBool(taskState, "bool_val"))
		assert.False(t, executor.GetExtBool(taskState, "nonexistent"))
	})

	t.Run("GetExtMap", func(t *testing.T) {
		m := executor.GetExtMap(taskState, "map_val")
		assert.NotNil(t, m)
		assert.Equal(t, "value", m["key"])
		assert.Nil(t, executor.GetExtMap(taskState, "nonexistent"))
	})
}

func TestTaskResultHelpers(t *testing.T) {
	t.Run("SuccessResult", func(t *testing.T) {
		result := task.SuccessResult(map[string]interface{}{"key": "value"})
		assert.True(t, result.Success)
		assert.Equal(t, constant.TaskStatusCompleted, result.NewStatus)
		assert.Equal(t, "value", result.UpdateExt["key"])
	})

	t.Run("FailureResult", func(t *testing.T) {
		result := task.FailureResult("test error", nil)
		assert.False(t, result.Success)
		assert.Equal(t, "test error", result.Error)
		assert.Equal(t, constant.TaskStatusFailed, result.NewStatus)
	})

	t.Run("ProgressResult", func(t *testing.T) {
		result := task.ProgressResult(nil)
		assert.True(t, result.Success)
		assert.Equal(t, constant.TaskStatusRunning, result.NewStatus)
	})
}
