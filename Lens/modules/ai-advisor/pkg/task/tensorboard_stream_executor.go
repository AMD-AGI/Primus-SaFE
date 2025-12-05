package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/tensorboard"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

// TensorBoardStreamExecutor TensorBoard 流式读取任务执行器
type TensorBoardStreamExecutor struct {
	coreTask.BaseExecutor

	streamReader   *tensorboard.StreamReader
	metadataFacade database.AiWorkloadMetadataFacadeInterface
	podFacade      database.PodFacadeInterface
	taskFacade     database.WorkloadTaskFacadeInterface
}

// NewTensorBoardStreamExecutor 创建 TensorBoard 流式读取执行器
func NewTensorBoardStreamExecutor(streamReader *tensorboard.StreamReader) *TensorBoardStreamExecutor {
	return &TensorBoardStreamExecutor{
		streamReader:   streamReader,
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
		podFacade:      database.NewPodFacade(),
		taskFacade:     database.NewWorkloadTaskFacade(),
	}
}

// GetTaskType 返回任务类型
func (e *TensorBoardStreamExecutor) GetTaskType() string {
	return constant.TaskTypeTensorBoardStream
}

// Validate 验证任务参数
func (e *TensorBoardStreamExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute 执行 TensorBoard 流式读取任务
func (e *TensorBoardStreamExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task

	log.Infof("Starting TensorBoard streaming for workload %s", task.WorkloadUID)

	// 1. 从 ai_workload_metadata 获取 TensorBoard 信息
	metadataInfo, err := e.metadataFacade.GetAiWorkloadMetadata(ctx, task.WorkloadUID)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get metadata: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	if metadataInfo == nil || metadataInfo.Metadata == nil {
		return coreTask.FailureResult(
			"no metadata found for workload",
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), fmt.Errorf("metadata not found")
	}

	// 2. 提取 TensorBoard 信息
	tensorboardInfo := e.extractTensorBoardInfo(metadataInfo.Metadata)
	if tensorboardInfo == nil || tensorboardInfo["log_dir"] == "" {
		return coreTask.FailureResult(
			"no tensorboard log_dir found in metadata",
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), fmt.Errorf("tensorboard log_dir not found")
	}

	logDir := tensorboardInfo["log_dir"].(string)
	log.Infof("TensorBoard log_dir: %s", logDir)

	// 3. 获取 pod 信息
	gpuPod, err := e.selectTargetPod(ctx, task.WorkloadUID)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get pod info: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 4. 从 ext 获取配置和 checkpoint
	checkpoint := e.GetExtMap(task, "checkpoint")
	pollInterval := e.GetExtInt(task, "poll_interval")
	if pollInterval == 0 {
		pollInterval = 5 // 默认 5 秒
	}

	// 5. 构建流式配置
	streamConfig := &tensorboard.StreamConfig{
		PollInterval:       time.Duration(pollInterval) * time.Second,
		ChunkSize:          65536, // 64KB
		BufferSize:         100,
		ReadHistorical:     false, // 不读取历史数据，只读取新增
		FollowRotation:     true,
		MaxHistoricalBytes: 0,
	}

	// 6. 启动流式读取
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	// 从 checkpoint 恢复 offset
	startOffsets := make(map[string]int64)
	if checkpoint != nil {
		if fileOffsets, ok := checkpoint["file_offsets"].(map[string]interface{}); ok {
			for file, offset := range fileOffsets {
				if offsetVal, ok := offset.(float64); ok {
					startOffsets[file] = int64(offsetVal)
				}
			}
		}
	}

	log.Infof("Starting stream with offsets: %+v", startOffsets)

	// 7. 构建流式请求
	streamReq := &tensorboard.StreamRequest{
		WorkloadUID: task.WorkloadUID,
		PodUID:      gpuPod.UID,
		LogDir:      logDir,
		Config:      streamConfig,
	}

	// 如果有 checkpoint，设置恢复状态
	if len(startOffsets) > 0 {
		streamReq.ResumeState = &tensorboard.StreamState{
			WorkloadUID: task.WorkloadUID,
			FileOffsets: startOffsets,
			LastUpdate:  time.Now(),
		}
	}

	// 8. 启动流式会话
	session, err := e.streamReader.StartStream(streamCtx, streamReq)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to start stream: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 9. 持续读取流数据并更新 offset
	go e.processStreamUpdates(ctx, task, gpuPod, session)

	// 10. 返回进行中状态（任务不会立即完成，会持续运行）
	return coreTask.ProgressResult(map[string]interface{}{
		"started_at":    time.Now().Format(time.RFC3339),
		"pod_name":      gpuPod.Name,
		"pod_namespace": gpuPod.Namespace,
		"log_dir":       logDir,
		"poll_interval": pollInterval,
		"status":        "streaming",
	}), nil
}

// processStreamUpdates 处理流式更新并定期更新 offset
func (e *TensorBoardStreamExecutor) processStreamUpdates(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	session *tensorboard.StreamSession,
) {
	defer session.Stop()

	// 用于聚合 offset 更新
	fileOffsets := make(map[string]int64)
	updateInterval := 10 * time.Second // 每 10 秒更新一次 offset

	totalBytesRead := int64(0)
	updateCount := 0

	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	log.Infof("Started processing stream updates for workload %s", task.WorkloadUID)

	for {
		select {
		case <-ctx.Done():
			log.Infof("Stream processing stopped by context for workload %s", task.WorkloadUID)

			// 最后一次更新 offset
			e.updateCheckpoint(ctx, task, fileOffsets, totalBytesRead, updateCount)
			return

		case update, ok := <-session.Updates():
			if !ok {
				// 通道已关闭，流结束
				log.Infof("Stream closed for workload %s", task.WorkloadUID)

				// 最后一次更新 offset
				e.updateCheckpoint(ctx, task, fileOffsets, totalBytesRead, updateCount)
				return
			}

			if update == nil {
				log.Warnf("Received nil update for workload %s", task.WorkloadUID)
				continue
			}

			// 更新文件 offset
			fileOffsets[update.File] = update.NewOffset
			totalBytesRead += update.BytesRead
			updateCount++

			log.Debugf("Received update for workload %s: file=%s, offset=%d->%d, bytes=%d",
				task.WorkloadUID, update.File, update.Offset, update.NewOffset, update.BytesRead)

		case <-ticker.C:
			// 定期更新 checkpoint
			if len(fileOffsets) > 0 {
				e.updateCheckpoint(ctx, task, fileOffsets, totalBytesRead, updateCount)
			}
		}
	}
}

// updateCheckpoint 更新 checkpoint 到数据库
func (e *TensorBoardStreamExecutor) updateCheckpoint(
	ctx context.Context,
	task *model.WorkloadTaskState,
	fileOffsets map[string]int64,
	totalBytesRead int64,
	updateCount int,
) {
	checkpoint := map[string]interface{}{
		"file_offsets":     fileOffsets,
		"total_bytes_read": totalBytesRead,
		"update_count":     updateCount,
		"last_update_at":   time.Now().Format(time.RFC3339),
	}

	err := e.taskFacade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, model.ExtType{
		"checkpoint": checkpoint,
	})

	if err != nil {
		log.Errorf("Failed to update checkpoint for workload %s: %v", task.WorkloadUID, err)
	} else {
		log.Debugf("Updated checkpoint for workload %s: %d files, %d bytes, %d updates",
			task.WorkloadUID, len(fileOffsets), totalBytesRead, updateCount)
	}
}

// Cancel 取消任务
func (e *TensorBoardStreamExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Cancelling TensorBoard stream for workload %s", task.WorkloadUID)

	// 停止流式会话
	err := e.streamReader.StopStream(task.WorkloadUID)
	if err != nil {
		log.Warnf("Failed to stop stream for workload %s: %v", task.WorkloadUID, err)
	}

	return nil
}

// extractTensorBoardInfo 从 metadata 中提取 TensorBoard 信息
func (e *TensorBoardStreamExecutor) extractTensorBoardInfo(metadata model.ExtType) map[string]interface{} {
	if metadata == nil {
		return nil
	}

	// 尝试从 tensorboard 字段获取
	if tb, ok := metadata["tensorboard"].(map[string]interface{}); ok {
		return tb
	}

	// 尝试从 tensorboard_info 字段获取
	if tb, ok := metadata["tensorboard_info"].(map[string]interface{}); ok {
		return tb
	}

	return nil
}

// selectTargetPod 从 workload 的所有 pod 中选择目标 pod
// 优先选择名称以 master-0 结尾的 pod，否则返回第一个
func (e *TensorBoardStreamExecutor) selectTargetPod(ctx context.Context, workloadUID string) (*model.GpuPods, error) {
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
