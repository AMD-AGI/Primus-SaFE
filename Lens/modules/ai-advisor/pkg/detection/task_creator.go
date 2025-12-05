package detection

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// TaskCreator 负责在框架检测完成后创建元数据收集任务
type TaskCreator struct {
	taskFacade     database.WorkloadTaskFacadeInterface
	instanceID     string
	autoCreateTask bool // 是否自动创建任务
}

// NewTaskCreator 创建任务创建器
func NewTaskCreator(instanceID string) *TaskCreator {
	return &TaskCreator{
		taskFacade:     database.NewWorkloadTaskFacade(),
		instanceID:     instanceID,
		autoCreateTask: true, // 默认启用自动创建
	}
}

// SetAutoCreateTask 设置是否自动创建任务
func (tc *TaskCreator) SetAutoCreateTask(auto bool) {
	tc.autoCreateTask = auto
}

// OnDetectionCompleted 当检测完成时被调用
// 根据检测结果创建元数据收集任务
func (tc *TaskCreator) OnDetectionCompleted(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	if !tc.autoCreateTask {
		log.Debugf("Auto task creation disabled, skipping task creation for workload %s", workloadUID)
		return nil
	}

	// 只为已确认的检测创建任务
	if detection.Status != coreModel.DetectionStatusConfirmed {
		log.Debugf("Detection status is %s (not confirmed), skipping task creation for workload %s",
			detection.Status, workloadUID)
		return nil
	}

	// 只为训练任务创建元数据收集任务
	if !tc.isTrainingWorkload(detection) {
		log.Debugf("Workload %s is not a training task, skipping metadata collection task", workloadUID)
		return nil
	}

	log.Infof("Creating metadata collection task for workload %s (frameworks: %v)",
		workloadUID, detection.Frameworks)

	// 创建任务
	// 注意：workload 相关的具体信息（pod、node 等）存储在 ai_workload_metadata 表
	// 这里的 ext 只存储任务执行上下文
	task := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeMetadataCollection,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			// 任务执行配置
			"auto_restart":        true,
			"priority":            100,
			"max_retries":         3,
			"retry_count":         0,
			"timeout":             30, // 30 秒超时
			"include_tensorboard": true,
			"include_metrics":     true,

			// 任务元数据
			"created_by":   "detection_manager",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "framework_detection",

			// 检测概要信息（用于日志和调试）
			"detection_frameworks": detection.Frameworks,
			"detection_confidence": detection.Confidence,
		},
	}

	// 使用 Upsert 创建或更新任务
	if err := tc.taskFacade.UpsertTask(ctx, task); err != nil {
		return fmt.Errorf("failed to create metadata collection task: %w", err)
	}

	log.Infof("Metadata collection task created successfully for workload %s", workloadUID)

	return nil
}

// isTrainingWorkload 判断是否为训练任务
func (tc *TaskCreator) isTrainingWorkload(detection *coreModel.FrameworkDetection) bool {
	// 检查 TaskType
	for _, source := range detection.Sources {
		// 如果任何一个来源标记为 training，则认为是训练任务
		if source.Type == "training" || source.Type == "" {
			return true
		}
	}

	// 默认认为是训练任务（除非明确标记为 inference）
	return true
}

// extractSourceNames 提取检测来源名称
func (tc *TaskCreator) extractSourceNames(detection *coreModel.FrameworkDetection) []string {
	sources := []string{}
	seen := make(map[string]bool)

	for _, source := range detection.Sources {
		if !seen[source.Source] {
			sources = append(sources, source.Source)
			seen[source.Source] = true
		}
	}

	return sources
}

// RegisterWithDetectionManager 将 TaskCreator 注册到 DetectionManager
// 作为检测事件的监听器
func RegisterTaskCreatorWithDetectionManager(
	detectionMgr *framework.FrameworkDetectionManager,
	instanceID string,
) *TaskCreator {
	taskCreator := NewTaskCreator(instanceID)

	// 创建一个适配器，将 DetectionEvent 转换为 TaskCreator 调用
	listener := &detectionEventAdapter{
		taskCreator: taskCreator,
	}

	detectionMgr.RegisterListener(listener)

	log.Info("TaskCreator registered with DetectionManager as event listener")
	return taskCreator
}

// detectionEventAdapter 适配 DetectionEvent 到 TaskCreator
type detectionEventAdapter struct {
	taskCreator *TaskCreator
}

// OnDetectionEvent 实现 DetectionEventListener 接口
func (a *detectionEventAdapter) OnDetectionEvent(
	ctx context.Context,
	event *framework.DetectionEvent,
) error {
	// 只处理completed 和 updated 事件
	if event.Type != framework.DetectionEventTypeUpdated &&
		event.Type != framework.DetectionEventTypeCompleted {
		return nil
	}

	if event.Detection == nil {
		return nil
	}

	// 调用 TaskCreator 创建任务
	return a.taskCreator.OnDetectionCompleted(ctx, event.WorkloadUID, event.Detection)
}
