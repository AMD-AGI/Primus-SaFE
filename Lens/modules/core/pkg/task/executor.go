package task

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// ExecutionContext task execution context
type ExecutionContext struct {
	Task       *model.WorkloadTaskState // current task
	InstanceID string                   // execution instance ID
	Cancel     context.CancelFunc       // cancel function
}

// ExecutionResult task execution result
type ExecutionResult struct {
	Success   bool                   // whether successful
	Error     string                 // error message
	UpdateExt map[string]interface{} // ext fields to update
	NewStatus string                 // new status
}

// TaskExecutor task executor interface
// Each task_type needs to implement this interface
type TaskExecutor interface {
	// GetTaskType returns the task type this executor handles
	GetTaskType() string

	// Execute executes task
	// Returning error indicates task failure
	// Returning nil indicates task success or in progress
	Execute(ctx context.Context, execCtx *ExecutionContext) (*ExecutionResult, error)

	// Cancel cancels task (optional implementation)
	Cancel(ctx context.Context, task *model.WorkloadTaskState) error

	// Validate validates task parameters (called before execution)
	Validate(task *model.WorkloadTaskState) error
}

// BaseExecutor provides common helper methods
type BaseExecutor struct{}

// GetExtString gets string value from ext field
func (b *BaseExecutor) GetExtString(task *model.WorkloadTaskState, key string) string {
	if task.Ext == nil {
		return ""
	}
	if val, ok := task.Ext[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetExtInt gets integer value from ext field
func (b *BaseExecutor) GetExtInt(task *model.WorkloadTaskState, key string) int {
	if task.Ext == nil {
		return 0
	}
	if val, ok := task.Ext[key]; ok {
		if num, ok := val.(float64); ok {
			return int(num)
		}
		if num, ok := val.(int); ok {
			return num
		}
	}
	return 0
}

// GetExtBool gets boolean value from ext field
func (b *BaseExecutor) GetExtBool(task *model.WorkloadTaskState, key string) bool {
	if task.Ext == nil {
		return false
	}
	if val, ok := task.Ext[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// GetExtMap gets map value from ext field
func (b *BaseExecutor) GetExtMap(task *model.WorkloadTaskState, key string) map[string]interface{} {
	if task.Ext == nil {
		return nil
	}
	if val, ok := task.Ext[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}

// SetExtValue sets value in ext field
func (b *BaseExecutor) SetExtValue(updates map[string]interface{}, key string, value interface{}) {
	if updates == nil {
		updates = make(map[string]interface{})
	}
	updates[key] = value
}

// SuccessResult creates success result
func SuccessResult(updates map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		Success:   true,
		UpdateExt: updates,
		NewStatus: constant.TaskStatusCompleted,
	}
}

// FailureResult creates failure result
func FailureResult(err string, updates map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		Success:   false,
		Error:     err,
		UpdateExt: updates,
		NewStatus: constant.TaskStatusFailed,
	}
}

// ProgressResult creates in-progress result
// Task stays in "running" status and won't be re-picked by scheduler
// Use this for long-running tasks that loop internally (e.g., streaming)
func ProgressResult(updates map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		Success:   true,
		UpdateExt: updates,
		NewStatus: constant.TaskStatusRunning,
	}
}

// RescheduleResult creates a result that returns task to pending status
// Task will be re-picked by scheduler after the current execution ends
// Use this for state-machine tasks that need periodic re-execution
func RescheduleResult(updates map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		Success:   true,
		UpdateExt: updates,
		NewStatus: constant.TaskStatusPending,
	}
}
