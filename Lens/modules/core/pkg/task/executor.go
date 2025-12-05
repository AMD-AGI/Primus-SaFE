package task

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// ExecutionContext 任务执行上下文
type ExecutionContext struct {
	Task       *model.WorkloadTaskState // 当前任务
	InstanceID string                   // 执行实例 ID
	Cancel     context.CancelFunc       // 取消函数
}

// ExecutionResult 任务执行结果
type ExecutionResult struct {
	Success   bool                   // 是否成功
	Error     string                 // 错误信息
	UpdateExt map[string]interface{} // 需要更新的 ext 字段
	NewStatus string                 // 新状态
}

// TaskExecutor 任务执行器接口
// 每种 task_type 需要实现这个接口
type TaskExecutor interface {
	// GetTaskType 返回此执行器处理的任务类型
	GetTaskType() string

	// Execute 执行任务
	// 返回 error 表示任务失败
	// 返回 nil 表示任务成功或正在进行中
	Execute(ctx context.Context, execCtx *ExecutionContext) (*ExecutionResult, error)

	// Cancel 取消任务（可选实现）
	Cancel(ctx context.Context, task *model.WorkloadTaskState) error

	// Validate 验证任务参数（在执行前调用）
	Validate(task *model.WorkloadTaskState) error
}

// BaseExecutor 提供一些通用的辅助方法
type BaseExecutor struct{}

// GetExtString 从 ext 字段获取字符串值
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

// GetExtInt 从 ext 字段获取整数值
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

// GetExtBool 从 ext 字段获取布尔值
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

// GetExtMap 从 ext 字段获取 map 值
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

// SetExtValue 设置 ext 字段的值
func (b *BaseExecutor) SetExtValue(updates map[string]interface{}, key string, value interface{}) {
	if updates == nil {
		updates = make(map[string]interface{})
	}
	updates[key] = value
}

// SuccessResult 创建成功结果
func SuccessResult(updates map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		Success:   true,
		UpdateExt: updates,
		NewStatus: constant.TaskStatusCompleted,
	}
}

// FailureResult 创建失败结果
func FailureResult(err string, updates map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		Success:   false,
		Error:     err,
		UpdateExt: updates,
		NewStatus: constant.TaskStatusFailed,
	}
}

// ProgressResult 创建进行中结果
func ProgressResult(updates map[string]interface{}) *ExecutionResult {
	return &ExecutionResult{
		Success:   true,
		UpdateExt: updates,
		NewStatus: constant.TaskStatusRunning,
	}
}
