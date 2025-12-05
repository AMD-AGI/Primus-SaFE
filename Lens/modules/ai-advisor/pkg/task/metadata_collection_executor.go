package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

// MetadataCollectionExecutor 元数据收集任务执行器
type MetadataCollectionExecutor struct {
	coreTask.BaseExecutor

	collector      *metadata.Collector
	metadataFacade database.AiWorkloadMetadataFacadeInterface
	podFacade      database.PodFacadeInterface
}

// NewMetadataCollectionExecutor 创建元数据收集执行器
func NewMetadataCollectionExecutor(collector *metadata.Collector) *MetadataCollectionExecutor {
	return &MetadataCollectionExecutor{
		collector:      collector,
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
		podFacade:      database.NewPodFacade(),
	}
}

// GetTaskType 返回任务类型
func (e *MetadataCollectionExecutor) GetTaskType() string {
	return constant.TaskTypeMetadataCollection
}

// Validate 验证任务参数
func (e *MetadataCollectionExecutor) Validate(task *model.WorkloadTaskState) error {
	// 检查必要的参数
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}

	return nil
}

// Execute 执行元数据收集任务
func (e *MetadataCollectionExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task

	log.Infof("Starting metadata collection for workload %s", task.WorkloadUID)

	// 1. 从 ai_workload_metadata 表获取检测信息
	detectionInfo, err := e.metadataFacade.GetAiWorkloadMetadata(ctx, task.WorkloadUID)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get detection info: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	if detectionInfo == nil {
		return coreTask.FailureResult(
			"no detection info found for workload",
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), fmt.Errorf("detection info not found")
	}

	// 2. 从 gpu_pods 表获取 pod 信息
	// workload_uid 对应 gpu_pods 表的 owner_uid 字段
	// 优先选择以 master-0 结尾的 pod
	gpuPod, err := e.selectTargetPod(ctx, task.WorkloadUID)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get pod info: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	if gpuPod == nil {
		return coreTask.FailureResult(
			"no pod found for workload",
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), fmt.Errorf("no pod found for workload")
	}

	// 3. 从 ext 字段获取收集配置
	timeout := e.GetExtInt(task, "timeout")
	if timeout == 0 {
		timeout = 30 // 默认 30 秒
	}

	// includeTensorBoard := e.GetExtBool(task, "include_tensorboard")
	// 总是包含 TensorBoard（在 scripts 中指定）

	// 4. 构建收集请求
	collectionReq := &metadata.CollectionRequest{
		WorkloadUID:  task.WorkloadUID,
		PodName:      gpuPod.Name,
		PodNamespace: gpuPod.Namespace,
		PodUID:       gpuPod.UID,
		NodeName:     gpuPod.NodeName,
		Timeout:      timeout,
		Force:        false, // 使用缓存
		Scripts:      e.extractScripts(detectionInfo),
	}

	log.Infof("Collecting metadata for pod %s/%s (node: %s)",
		gpuPod.Namespace, gpuPod.Name, gpuPod.NodeName)

	// 5. 执行元数据收集
	result, err := e.collector.CollectMetadata(ctx, collectionReq)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("metadata collection failed: %v", err),
			map[string]interface{}{
				"error_at":        time.Now().Format(time.RFC3339),
				"pod_name":        gpuPod.Name,
				"pod_namespace":   gpuPod.Namespace,
				"collection_time": result.Duration,
			},
		), err
	}

	// 6. 构建返回结果
	// 直接存储整个 result 用于排查问题
	updates := map[string]interface{}{
		"completed_at":      time.Now().Format(time.RFC3339),
		"collection_result": result,
		"pod_name":          gpuPod.Name,
		"pod_namespace":     gpuPod.Namespace,
		"node_name":         gpuPod.NodeName,
	}

	if result.Success {
		log.Infof("Metadata collection completed successfully for workload %s", task.WorkloadUID)
		return coreTask.SuccessResult(updates), nil
	} else {
		log.Warnf("Metadata collection failed for workload %s: %s", task.WorkloadUID, result.Error)
		return coreTask.FailureResult(result.Error, updates), fmt.Errorf("collection failed: %s", result.Error)
	}
}

// Cancel 取消任务
func (e *MetadataCollectionExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	// 元数据收集任务可以直接取消（通过 context）
	log.Infof("Metadata collection task cancelled for workload %s", task.WorkloadUID)
	return nil
}

// extractScripts 从检测信息中提取需要运行的脚本
func (e *MetadataCollectionExecutor) extractScripts(detection *model.AiWorkloadMetadata) []string {
	scripts := []string{}

	// 根据检测的框架选择脚本
	framework := detection.Framework
	if framework != "" {
		scripts = append(scripts, framework)
	}

	// 从 metadata 中获取额外的框架信息
	if detection.Metadata != nil {
		if wrapperFw, ok := detection.Metadata["wrapper_framework"].(string); ok && wrapperFw != "" {
			scripts = append(scripts, wrapperFw)
		}
		if baseFw, ok := detection.Metadata["base_framework"].(string); ok && baseFw != "" {
			if !contains(scripts, baseFw) {
				scripts = append(scripts, baseFw)
			}
		}
	}

	// 总是包含 tensorboard 脚本（通用）
	if !contains(scripts, "tensorboard") {
		scripts = append(scripts, "tensorboard")
	}

	return scripts
}

// selectTargetPod 从 workload 的所有 pod 中选择目标 pod
// 优先选择名称以 master-0 结尾的 pod，否则返回第一个
func (e *MetadataCollectionExecutor) selectTargetPod(ctx context.Context, workloadUID string) (*model.GpuPods, error) {
	// 获取数据库连接
	db := database.GetFacade().GetSystemConfig().GetDB()

	// 查询 owner_uid = workloadUID 且未删除的 pod
	var pods []*model.GpuPods
	err := db.WithContext(ctx).
		Where("owner_uid = ? AND deleted = ?", workloadUID, false).
		Find(&pods).Error

	if err != nil {
		return nil, fmt.Errorf("failed to query pods: %w", err)
	}

	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods found for workload %s", workloadUID)
	}

	log.Infof("Found %d pods for workload %s", len(pods), workloadUID)

	// 优先选择以 master-0 结尾的 pod
	for _, pod := range pods {
		if strings.HasSuffix(pod.Name, "master-0") {
			log.Infof("Selected master-0 pod: %s/%s", pod.Namespace, pod.Name)
			return pod, nil
		}
	}

	// 如果没有 master-0，返回第一个 pod
	selectedPod := pods[0]
	log.Infof("No master-0 pod found, selected first pod: %s/%s", selectedPod.Namespace, selectedPod.Name)
	return selectedPod, nil
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
