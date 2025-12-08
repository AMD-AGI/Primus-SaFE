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

	// 1. 从任务 ext 中获取 TensorBoard 事件文件列表
	eventFilesRaw, ok := task.Ext["event_files"]
	if !ok || eventFilesRaw == nil {
		return coreTask.FailureResult(
			"no event_files found in task ext",
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), fmt.Errorf("event_files not found in task ext")
	}

	// 转换 event_files 为字符串数组
	eventFiles, err := e.parseEventFiles(eventFilesRaw)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("invalid event_files: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), fmt.Errorf("invalid event_files")
	}

	// 获取可选的 log_dir
	logDir := ""
	if logDirVal, ok := task.Ext["log_dir"]; ok {
		if logDirStr, ok := logDirVal.(string); ok {
			logDir = logDirStr
		}
	}

	// 检查是否需要等待文件出现
	waitForFiles := false
	if waitVal, ok := task.Ext["wait_for_files"]; ok {
		if waitBool, ok := waitVal.(bool); ok {
			waitForFiles = waitBool
		}
	}

	// 如果事件文件列表为空但配置了等待，需要等待文件出现
	if len(eventFiles) == 0 {
		if !waitForFiles || logDir == "" {
			return coreTask.FailureResult(
				"no event files provided and wait_for_files not configured",
				map[string]interface{}{
					"error_at": time.Now().Format(time.RFC3339),
				},
			), fmt.Errorf("no event files to stream")
		}
		log.Infof("TensorBoard: waiting for files to appear in %s", logDir)
	} else {
		log.Infof("TensorBoard event files: %v (log_dir: %s)", eventFiles, logDir)
	}

	// 2. 获取 pod 信息
	gpuPod, err := e.selectTargetPod(ctx, task.WorkloadUID)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get pod info: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 3. 从 ext 获取配置和 checkpoint
	var checkpoint map[string]interface{}
	if checkpointVal, ok := task.Ext["checkpoint"]; ok {
		if checkpointMap, ok := checkpointVal.(map[string]interface{}); ok {
			checkpoint = checkpointMap
		}
	}

	pollInterval := 5 // 默认 5 秒
	if pollIntervalVal, ok := task.Ext["poll_interval"]; ok {
		if pollIntervalInt, ok := pollIntervalVal.(int); ok {
			pollInterval = pollIntervalInt
		} else if pollIntervalFloat, ok := pollIntervalVal.(float64); ok {
			pollInterval = int(pollIntervalFloat)
		}
	}

	// 4. 构建流式配置
	streamConfig := &tensorboard.StreamConfig{
		PollInterval:       time.Duration(pollInterval) * time.Second,
		ChunkSize:          65536, // 64KB
		BufferSize:         100,
		ReadHistorical:     false, // 不读取历史数据，只读取新增
		FollowRotation:     true,
		MaxHistoricalBytes: 0,
	}

	// 5. 启动流式读取
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

	log.Infof("Starting stream with event files: %v, offsets: %+v", eventFiles, startOffsets)

	// 6. 如果需要等待文件，先扫描文件
	if waitForFiles && len(eventFiles) == 0 {
		log.Infof("Waiting for TensorBoard files to appear in %s", logDir)

		fileWaitTimeout := 300 // 默认5分钟
		if timeoutVal, ok := task.Ext["file_wait_timeout"]; ok {
			if timeoutInt, ok := timeoutVal.(int); ok {
				fileWaitTimeout = timeoutInt
			} else if timeoutFloat, ok := timeoutVal.(float64); ok {
				fileWaitTimeout = int(timeoutFloat)
			}
		}

		fileScanInterval := 10 // 默认10秒
		if intervalVal, ok := task.Ext["file_scan_interval"]; ok {
			if intervalInt, ok := intervalVal.(int); ok {
				fileScanInterval = intervalInt
			} else if intervalFloat, ok := intervalVal.(float64); ok {
				fileScanInterval = int(intervalFloat)
			}
		}

		// 等待文件出现
		detectedFiles, err := e.waitForTensorBoardFiles(
			streamCtx,
			task.WorkloadUID,
			gpuPod.UID,
			logDir,
			time.Duration(fileWaitTimeout)*time.Second,
			time.Duration(fileScanInterval)*time.Second,
		)

		if err != nil {
			return coreTask.FailureResult(
				fmt.Sprintf("failed to detect TensorBoard files: %v", err),
				map[string]interface{}{
					"error_at":     time.Now().Format(time.RFC3339),
					"wait_timeout": fileWaitTimeout,
				},
			), err
		}

		eventFiles = detectedFiles
		log.Infof("TensorBoard files detected: %v", eventFiles)
	}

	// 7. 构建流式请求（使用精确的事件文件列表）
	streamReq := &tensorboard.StreamRequest{
		WorkloadUID: task.WorkloadUID,
		PodUID:      gpuPod.UID,
		EventFiles:  eventFiles, // 使用精确的事件文件列表
		LogDir:      logDir,     // 保留作为参考
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

	// 7. 启动流式会话
	session, err := e.streamReader.StartStream(streamCtx, streamReq)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to start stream: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 8. 持续读取流数据并更新 offset
	go e.processStreamUpdates(ctx, task, gpuPod, session)

	// 9. 返回进行中状态（任务不会立即完成，会持续运行）
	return coreTask.ProgressResult(map[string]interface{}{
		"started_at":    time.Now().Format(time.RFC3339),
		"pod_name":      gpuPod.Name,
		"pod_namespace": gpuPod.Namespace,
		"event_files":   eventFiles,
		"files_count":   len(eventFiles),
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

// waitForTensorBoardFiles 等待 TensorBoard 文件出现
func (e *TensorBoardStreamExecutor) waitForTensorBoardFiles(
	ctx context.Context,
	workloadUID string,
	podUID string,
	logDir string,
	timeout time.Duration,
	scanInterval time.Duration,
) ([]string, error) {
	log.Infof("Waiting for TensorBoard files in %s (timeout: %v, interval: %v)", logDir, timeout, scanInterval)

	// TODO: 实现真正的文件扫描逻辑
	// 需要通过 node-exporter API 定期调用 FindTensorboardFiles
	// 当前先使用简化逻辑

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while waiting for files")

		case <-time.After(time.Until(deadline)):
			return nil, fmt.Errorf("timeout waiting for TensorBoard files after %v", timeout)

		case <-ticker.C:
			// 尝试通过 fd 扫描查找文件
			// 这里需要调用 FindTensorboardFiles 接口
			log.Debugf("Scanning for TensorBoard files in pod %s", podUID)

			// TODO: 实际调用 node-exporter API 扫描文件
			// 由于需要 client，这里暂时返回错误，实际使用时需要完善

			// 临时方案：如果logDir存在，假设文件会在那里出现
			// 实际部署时需要真正的文件扫描逻辑

			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for TensorBoard files")
			}
		}
	}
}

// parseEventFiles 解析事件文件列表
func (e *TensorBoardStreamExecutor) parseEventFiles(eventFilesRaw interface{}) ([]string, error) {
	var eventFiles []string

	// 尝试将接口类型转换为字符串切片
	switch v := eventFilesRaw.(type) {
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok {
				eventFiles = append(eventFiles, str)
			}
		}
	case []string:
		eventFiles = v
	default:
		return nil, fmt.Errorf("unsupported event_files type: %T", eventFilesRaw)
	}

	return eventFiles, nil
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
