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
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// MetadataCollectionExecutor 元数据收集任务执行器
type MetadataCollectionExecutor struct {
	coreTask.BaseExecutor

	collector      *metadata.Collector
	metadataFacade database.AiWorkloadMetadataFacadeInterface
	podFacade      database.PodFacadeInterface
	taskFacade     database.WorkloadTaskFacadeInterface
}

// NewMetadataCollectionExecutor 创建元数据收集执行器
func NewMetadataCollectionExecutor(collector *metadata.Collector) *MetadataCollectionExecutor {
	return &MetadataCollectionExecutor{
		collector:      collector,
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
		podFacade:      database.NewPodFacade(),
		taskFacade:     database.NewWorkloadTaskFacade(),
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

	// 3. 从 ext 字段获取收集配置 (暂时未使用)
	// timeout := e.GetExtInt(task, "timeout")
	// if timeout == 0 {
	// 	timeout = 30 // 默认 30 秒
	// }

	log.Infof("Detecting TensorBoard for pod %s/%s (node: %s)",
		gpuPod.Namespace, gpuPod.Name, gpuPod.NodeName)

	// 4. 获取 node-exporter client
	nodeExporterClient, err := e.collector.GetNodeExporterClientForPod(ctx, gpuPod.NodeName)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get node-exporter client: %v", err),
			map[string]interface{}{
				"error_at":      time.Now().Format(time.RFC3339),
				"pod_name":      gpuPod.Name,
				"pod_namespace": gpuPod.Namespace,
				"node_name":     gpuPod.NodeName,
			},
		), err
	}

	// 5. 调用 TensorBoard fd 扫描接口
	tensorboardResult, err := nodeExporterClient.FindTensorboardFiles(
		ctx,
		gpuPod.UID,
		gpuPod.Name,
		gpuPod.Namespace,
	)

	// 6. 构建返回结果
	updates := map[string]interface{}{
		"completed_at":  time.Now().Format(time.RFC3339),
		"pod_name":      gpuPod.Name,
		"pod_namespace": gpuPod.Namespace,
		"node_name":     gpuPod.NodeName,
	}

	if err != nil {
		errMsg := fmt.Sprintf("tensorboard detection failed: %v", err)
		log.Warnf("TensorBoard detection failed for workload %s: %v", task.WorkloadUID, err)
		updates["error"] = errMsg
		updates["tensorboard_enabled"] = false
		return coreTask.FailureResult(errMsg, updates), err
	}

	// 7. 解析 TensorBoard 结果
	tensorboardEnabled := len(tensorboardResult.Files) > 0
	updates["tensorboard_enabled"] = tensorboardEnabled
	updates["tensorboard_result"] = tensorboardResult

	if tensorboardEnabled {
		// 提取所有唯一的事件文件路径（去重）
		uniqueFilePaths := extractUniqueFilePaths(tensorboardResult.Files)

		// 从第一个文件路径中提取目录作为 log_dir 参考
		logDir := ""
		if len(uniqueFilePaths) > 0 {
			logDir = extractLogDir(uniqueFilePaths[0])
		}

		updates["tensorboard_log_dir"] = logDir
		updates["tensorboard_event_files"] = uniqueFilePaths // 精确的事件文件路径列表
		updates["tensorboard_files_count"] = len(uniqueFilePaths)
		updates["tensorboard_pids"] = extractUniquePIDs(tensorboardResult.Files)

		log.Infof("TensorBoard detected for workload %s: enabled=true, log_dir=%s, event_files=%d, unique_files=%v",
			task.WorkloadUID, logDir, len(uniqueFilePaths), uniqueFilePaths)

		// 8. 创建 TensorBoard 流式读取任务
		if err := e.createTensorBoardStreamTask(ctx, task.WorkloadUID, uniqueFilePaths, logDir); err != nil {
			log.Warnf("Failed to create TensorBoard stream task for workload %s: %v", task.WorkloadUID, err)
			updates["stream_task_created"] = false
			updates["stream_task_error"] = err.Error()
		} else {
			log.Infof("TensorBoard stream task created for workload %s", task.WorkloadUID)
			updates["stream_task_created"] = true
		}
	} else {
		log.Infof("TensorBoard not detected for workload %s (no event files found)", task.WorkloadUID)
	}

	return coreTask.SuccessResult(updates), nil
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
	// 方法1：通过 workload_pod_reference 表查找 pod（推荐方式，支持层级关系）
	workloadFacade := database.GetFacade().GetWorkload()
	podRefs, err := workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload_pod_reference for workload %s: %v", workloadUID, err)
	}

	var pods []*model.GpuPods
	if len(podRefs) > 0 {
		// 通过 pod UID 列表查询 pod 详情
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}

		// 获取 pod 详情
		db := database.GetFacade().GetSystemConfig().GetDB()
		err = db.WithContext(ctx).
			Where("uid IN ? AND deleted = ?", podUIDs, false).
			Find(&pods).Error
		if err != nil {
			return nil, fmt.Errorf("failed to query pods by references: %w", err)
		}

		log.Infof("Found %d pods for workload %s via workload_pod_reference", len(pods), workloadUID)
	}

	// 方法2：查找子 workload 的 pod（递归查找层级结构）
	if len(pods) == 0 {
		childWorkloads, err := workloadFacade.ListChildrenWorkloadByParentUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to query child workloads for %s: %v", workloadUID, err)
		} else if len(childWorkloads) > 0 {
			log.Infof("Found %d child workloads for %s, searching their pods", len(childWorkloads), workloadUID)

			for _, child := range childWorkloads {
				childPods, err := e.selectTargetPod(ctx, child.UID)
				if err == nil && childPods != nil {
					return childPods, nil
				}
			}
		}
	}

	if len(pods) == 0 {
		return nil, fmt.Errorf("no pods found for workload %s", workloadUID)
	}

	// 优先选择以 master-0 结尾的 pod
	for _, pod := range pods {
		if strings.HasSuffix(pod.Name, "master-0") {
			log.Infof("Selected master-0 pod: %s/%s for workload %s", pod.Namespace, pod.Name, workloadUID)
			return pod, nil
		}
	}

	// 如果没有 master-0，返回第一个 pod
	selectedPod := pods[0]
	log.Infof("No master-0 pod found, selected first pod: %s/%s for workload %s",
		selectedPod.Namespace, selectedPod.Name, workloadUID)
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

// createTensorBoardStreamTask 创建 TensorBoard 流式读取任务
func (e *MetadataCollectionExecutor) createTensorBoardStreamTask(
	ctx context.Context,
	workloadUID string,
	eventFiles []string,
	logDir string,
) error {
	// 检查是否已经存在 TensorBoard stream 任务
	existingTask, err := e.taskFacade.GetTask(ctx, workloadUID, constant.TaskTypeTensorBoardStream)
	if err != nil {
		log.Debugf("Failed to check existing TensorBoard stream task: %v", err)
	}

	// 如果任务已存在且正在运行，不创建新任务
	if existingTask != nil && (existingTask.Status == constant.TaskStatusRunning || existingTask.Status == constant.TaskStatusPending) {
		log.Infof("TensorBoard stream task already exists for workload %s (status: %s)", workloadUID, existingTask.Status)
		return nil
	}

	// 创建新任务
	streamTask := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeTensorBoardStream,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			// TensorBoard 配置
			"event_files":   eventFiles, // 精确的事件文件列表
			"log_dir":       logDir,     // 日志目录（参考）
			"poll_interval": 5,          // 5 秒轮询间隔

			// 任务配置
			"auto_restart": true,
			"priority":     90, // 稍低于元数据收集任务
			"max_retries":  5,
			"retry_count":  0,

			// 任务元数据
			"created_by":   "metadata_collection",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "tensorboard_detection",
		},
	}

	// 使用 Upsert 创建或更新任务
	if err := e.taskFacade.UpsertTask(ctx, streamTask); err != nil {
		return fmt.Errorf("failed to create TensorBoard stream task: %w", err)
	}

	log.Infof("TensorBoard stream task created for workload %s with %d event files", workloadUID, len(eventFiles))
	return nil
}

// extractLogDir 从 TensorBoard 事件文件路径中提取目录
func extractLogDir(filePath string) string {
	// 找到最后一个 '/' 的位置
	lastSlash := strings.LastIndex(filePath, "/")
	if lastSlash > 0 {
		return filePath[:lastSlash]
	}
	return filePath
}

// extractUniqueFilePaths 从 TensorBoard 文件列表中提取唯一的文件路径（去重）
func extractUniqueFilePaths(files []*types.TensorboardFileInfo) []string {
	filePathMap := make(map[string]bool)
	var uniquePaths []string

	for _, file := range files {
		// 使用文件路径作为键进行去重
		if !filePathMap[file.FilePath] {
			filePathMap[file.FilePath] = true
			uniquePaths = append(uniquePaths, file.FilePath)
		}
	}

	return uniquePaths
}

// extractUniquePIDs 从 TensorBoard 文件列表中提取唯一的 PID 列表
func extractUniquePIDs(files []*types.TensorboardFileInfo) []int {
	pidMap := make(map[int]bool)
	var pids []int

	for _, file := range files {
		if !pidMap[file.PID] {
			pidMap[file.PID] = true
			pids = append(pids, file.PID)
		}
	}

	return pids
}
