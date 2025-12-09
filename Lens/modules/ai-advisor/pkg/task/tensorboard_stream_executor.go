package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/tensorboard"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
)

// TensorBoardStreamExecutor TensorBoard 流式读取任务执行器
type TensorBoardStreamExecutor struct {
	coreTask.BaseExecutor

	streamReader   *tensorboard.StreamReader
	metadataFacade database.AiWorkloadMetadataFacadeInterface
	podFacade      database.PodFacadeInterface
	taskFacade     database.WorkloadTaskFacadeInterface
	trainingFacade database.TrainingFacadeInterface
	nodeFacade     database.NodeFacadeInterface
	eventParser    *tensorboard.EventParser

	// 文件解析状态管理
	fileBuffers map[string]*FileParseState // 每个文件的解析状态

	// Node-exporter client cache
	clientCache sync.Map // nodeName -> *client.Client
}

// FileParseState 文件解析状态
type FileParseState struct {
	Buffer            []byte         // 缓存的不完整数据
	LastValidOffset   int64          // 最后一个完整解析的 record 结束位置
	DebugFile         *os.File       // 调试文件句柄
	DebugFilePath     string         // 调试文件路径
	TotalBytesWritten int64          // 已写入调试文件的字节数
	Metadata          *DebugMetadata // 调试元信息
}

// DebugMetadata 调试元信息
type DebugMetadata struct {
	FileName          string      `json:"file_name"`
	OriginalPath      string      `json:"original_path"`
	TotalBytesWritten int64       `json:"total_bytes_written"`
	Errors            []ErrorInfo `json:"errors"`
	CreatedAt         time.Time   `json:"created_at"`
	LastUpdated       time.Time   `json:"last_updated"`
}

// ErrorInfo 错误信息
type ErrorInfo struct {
	Offset        int64     `json:"offset"`
	BufferSize    int       `json:"buffer_size"`
	Error         string    `json:"error"`
	Timestamp     time.Time `json:"timestamp"`
	BytesConsumed int       `json:"bytes_consumed"`
	EventsParsed  int       `json:"events_parsed"`
}

// NewTensorBoardStreamExecutor 创建 TensorBoard 流式读取执行器
func NewTensorBoardStreamExecutor(streamReader *tensorboard.StreamReader) *TensorBoardStreamExecutor {
	return &TensorBoardStreamExecutor{
		streamReader:   streamReader,
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
		podFacade:      database.NewPodFacade(),
		taskFacade:     database.NewWorkloadTaskFacade(),
		trainingFacade: database.NewTrainingFacade(),
		nodeFacade:     database.NewNodeFacade(),
		eventParser:    tensorboard.NewEventParser(),
		fileBuffers:    make(map[string]*FileParseState),
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

	// 1. 从任务 ext 中获取 TensorBoard 事件文件列表（允许为空）
	var eventFiles []string
	eventFilesRaw, ok := task.Ext["event_files"]
	if ok && eventFilesRaw != nil {
		// 转换 event_files 为字符串数组
		var err error
		eventFiles, err = e.parseEventFiles(eventFilesRaw)
		if err != nil {
			log.Warnf("Failed to parse event_files: %v, will wait for files to appear", err)
			eventFiles = []string{} // 设置为空数组，继续执行
		}
	}

	// 获取可选的 log_dir
	logDir := ""
	if logDirVal, ok := task.Ext["log_dir"]; ok {
		if logDirStr, ok := logDirVal.(string); ok {
			logDir = logDirStr
		}
	}

	// 如果事件文件列表为空，需要等待文件出现（只要 pod 还在就一直扫描）
	if len(eventFiles) == 0 {
		if logDir == "" {
			log.Infof("TensorBoard: waiting for files to appear (log_dir not specified, will scan container)")
		} else {
			log.Infof("TensorBoard: waiting for files to appear in %s", logDir)
		}
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

	// 5. 使用传入的 ctx 让任务系统控制生命周期
	// Stream will run until task is cancelled by task manager

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

	// 6. 获取 node-exporter client
	nodeExporterClient, err := e.getNodeExporterClient(ctx, gpuPod.NodeName)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get node-exporter client: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 7. 如果文件列表为空，先扫描文件
	if len(eventFiles) == 0 {
		log.Infof("Waiting for TensorBoard files to appear")

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
			ctx,
			task.WorkloadUID,
			gpuPod.UID,
			logDir,
			time.Duration(fileWaitTimeout)*time.Second,
			time.Duration(fileScanInterval)*time.Second,
			nodeExporterClient,
			gpuPod,
		)

		if err != nil {
			return coreTask.FailureResult(
				fmt.Sprintf("failed to detect TensorBoard files: %v", err),
				map[string]interface{}{
					"error_at":      time.Now().Format(time.RFC3339),
					"wait_timeout":  fileWaitTimeout,
					"scan_interval": fileScanInterval,
					"log_dir":       logDir,
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
	session, err := e.streamReader.StartStream(ctx, streamReq)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to start stream: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	log.Infof("Stream session started for workload %s, entering blocking mode", task.WorkloadUID)

	// 8. 同步处理流数据直到任务被取消
	// This blocks until context is cancelled or stream ends
	e.processStreamUpdates(ctx, task, gpuPod, session)

	// 9. Stream ended, return final result
	log.Infof("TensorBoard stream ended for workload %s", task.WorkloadUID)
	return coreTask.SuccessResult(map[string]interface{}{
		"ended_at":      time.Now().Format(time.RFC3339),
		"pod_name":      gpuPod.Name,
		"pod_namespace": gpuPod.Namespace,
		"event_files":   eventFiles,
		"files_count":   len(eventFiles),
		"status":        "completed",
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
	defer e.cleanupDebugFiles()

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

			log.Debugf("Received update for workload %s: file=%s, offset=%d->%d, bytes=%d",
				task.WorkloadUID, update.File, update.Offset, update.NewOffset, update.BytesRead)

			// 解析 TensorBoard event 数据并存储
			// TensorBoard event 文件以 "events.out.tfevents" 开头
			if len(update.Content) > 0 && e.isTensorBoardEventFile(update.File) {
				e.parseTensorBoardEvents(ctx, task, gpuPod, update)
			}

			// 更新文件 offset - 使用解析后的实际有效 offset
			// 这确保我们只记录已经成功解析的位置
			if fileState, exists := e.fileBuffers[update.File]; exists {
				fileOffsets[update.File] = fileState.LastValidOffset
				log.Debugf("Updated offset for %s to %d (parsed position)",
					update.File, fileState.LastValidOffset)
			} else {
				// 如果不是 event 文件或解析失败，使用原始 offset
				fileOffsets[update.File] = update.NewOffset
			}

			totalBytesRead += update.BytesRead
			updateCount++

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
	nodeExporterClient *client.Client,
	gpuPod *model.GpuPods,
) ([]string, error) {
	if logDir == "" {
		log.Infof("Waiting for TensorBoard files in pod %s (no specific dir, will scan /proc/fd) (timeout: %v, interval: %v)",
			podUID, timeout, scanInterval)
	} else {
		log.Infof("Waiting for TensorBoard files in %s (timeout: %v, interval: %v)", logDir, timeout, scanInterval)
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled while waiting for files")

		case <-ticker.C:
			// 检查是否超时
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for TensorBoard files after %v", timeout)
			}

			// 调用 FindTensorboardFiles 扫描文件
			log.Debugf("Scanning for TensorBoard files in pod %s", podUID)

			findResp, err := nodeExporterClient.FindTensorboardFiles(ctx, podUID, gpuPod.Name, gpuPod.Namespace)
			if err != nil {
				log.Warnf("Failed to scan for TensorBoard files: %v, will retry", err)
				continue
			}

			// 提取文件路径并去重
			fileSet := make(map[string]bool)
			var eventFiles []string
			for _, fileInfo := range findResp.Files {
				// 去重：只添加未见过的文件
				if !fileSet[fileInfo.FilePath] {
					fileSet[fileInfo.FilePath] = true
					eventFiles = append(eventFiles, fileInfo.FilePath)
				}
			}

			if len(eventFiles) > 0 {
				log.Infof("Found %d unique TensorBoard event files in pod %s", len(eventFiles), podUID)
				return eventFiles, nil
			}

			log.Debugf("No TensorBoard files found yet, waiting...")
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

	}

	// 方法2：查找子 workload 的 pod（递归查找层级结构）
	if len(pods) == 0 {
		childWorkloads, err := workloadFacade.ListChildrenWorkloadByParentUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to query child workloads for %s: %v", workloadUID, err)
		} else if len(childWorkloads) > 0 {
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

// isTensorBoardEventFile 检查文件名是否为 TensorBoard event 文件
func (e *TensorBoardStreamExecutor) isTensorBoardEventFile(filePath string) bool {
	// TensorBoard event 文件格式: events.out.tfevents.*
	fileName := filePath
	if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
		fileName = filePath[idx+1:]
	}
	return strings.HasPrefix(fileName, "events.out.tfevents")
}

// parseTensorBoardEvents 解析 TensorBoard event 数据并存储到数据库
// 实现流式解析：处理不完整的 record，维护缓冲区
func (e *TensorBoardStreamExecutor) parseTensorBoardEvents(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	update *tensorboard.StreamUpdate,
) {
	// TensorBoard event 文件是二进制格式
	if !e.isTensorBoardEventFile(update.File) {
		log.Debugf("Skipping non-event file: %s", update.File)
		return
	}

	// Content 字段包含二进制数据（作为字符串）
	newData := []byte(update.Content)

	if len(newData) == 0 {
		log.Debugf("Empty content for file: %s", update.File)
		return
	}

	// 1. 获取或创建该文件的解析状态
	fileState, exists := e.fileBuffers[update.File]
	if !exists {
		// 初始化文件解析状态
		fileState = &FileParseState{
			Buffer:          make([]byte, 0),
			LastValidOffset: update.Offset,
		}

		// 创建调试文件
		debugFileName := filepath.Base(update.File) + ".debug"
		debugFilePath := filepath.Join("/tmp", debugFileName)
		debugFile, err := os.OpenFile(debugFilePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			log.Warnf("Failed to create debug file %s: %v", debugFilePath, err)
		} else {
			fileState.DebugFile = debugFile
			fileState.DebugFilePath = debugFilePath
			fileState.Metadata = &DebugMetadata{
				FileName:     filepath.Base(update.File),
				OriginalPath: update.File,
				CreatedAt:    time.Now(),
				Errors:       make([]ErrorInfo, 0),
			}
			log.Infof("Created debug file: %s", debugFilePath)
		}

		e.fileBuffers[update.File] = fileState
		log.Infof("Initialized parse state for %s at offset %d", update.File, update.Offset)
	}

	// 2. 写入调试数据到文件
	if fileState.DebugFile != nil {
		n, err := fileState.DebugFile.Write(newData)
		if err != nil {
			log.Warnf("Failed to write to debug file: %v", err)
		} else {
			fileState.TotalBytesWritten += int64(n)
			if fileState.Metadata != nil {
				fileState.Metadata.TotalBytesWritten = fileState.TotalBytesWritten
			}
		}
	}

	// 3. 将新数据追加到缓冲区
	oldBufferSize := len(fileState.Buffer)
	fileState.Buffer = append(fileState.Buffer, newData...)

	log.Infof("File %s: buffer=%d bytes (was %d, added %d), offset=%d->%d, last_valid=%d",
		update.File, len(fileState.Buffer), oldBufferSize, len(newData),
		update.Offset, update.NewOffset, fileState.LastValidOffset)

	// Check for potential offset mismatch
	if update.Offset != fileState.LastValidOffset {
		log.Warnf("OFFSET MISMATCH: update.Offset=%d, but last_valid=%d, diff=%d",
			update.Offset, fileState.LastValidOffset, update.Offset-fileState.LastValidOffset)
	}

	// 4. 尝试从缓冲区解析完整的 events
	events, consumedBytes, parseErr := e.eventParser.ParseEventsWithBuffer(fileState.Buffer)

	log.Infof("Parse result: events=%d, consumed=%d/%d bytes (%.2f%%)",
		len(events), consumedBytes, len(fileState.Buffer),
		float64(consumedBytes)*100/max(float64(len(fileState.Buffer)), 1))

	// 记录错误到元信息
	if parseErr != nil || (len(events) == 0 && len(fileState.Buffer) > 1024) {
		errorInfo := ErrorInfo{
			Offset:        fileState.LastValidOffset,
			BufferSize:    len(fileState.Buffer),
			BytesConsumed: consumedBytes,
			EventsParsed:  len(events),
			Timestamp:     time.Now(),
		}
		if parseErr != nil {
			errorInfo.Error = parseErr.Error()
		} else {
			errorInfo.Error = "No events parsed with large buffer"
		}

		if fileState.Metadata != nil {
			fileState.Metadata.Errors = append(fileState.Metadata.Errors, errorInfo)
			fileState.Metadata.LastUpdated = time.Now()

			// 保存元信息到 .meta 文件
			e.saveMetadata(fileState)
		}

		log.Errorf("Parse FAILED for %s at offset %d: %v", update.File, fileState.LastValidOffset, errorInfo.Error)

		// 如果解析失败，清空缓冲区
		fileState.Buffer = fileState.Buffer[:0]
		fileState.LastValidOffset = update.NewOffset
		log.Warnf("Reset parse state for %s to offset %d", update.File, update.NewOffset)
		return
	}

	if len(events) == 0 {
		// 没有解析出完整的 event，缓冲区数据保留，等待下次
		log.Debugf("No complete events in buffer for %s, waiting for more data (buffer: %d bytes)",
			update.File, len(fileState.Buffer))

		// 防止缓冲区无限增长（如果一直无法解析）
		if len(fileState.Buffer) > 1024*1024 { // 超过 1MB
			log.Warnf("Buffer for %s exceeds 1MB without complete event, may be corrupted. Clearing buffer.",
				update.File)
			fileState.Buffer = fileState.Buffer[:0]
			fileState.LastValidOffset = update.NewOffset
		}
		return
	}

	log.Infof("Parsed %d complete events from %s (consumed %d bytes, remaining %d bytes in buffer)",
		len(events), update.File, consumedBytes, len(fileState.Buffer)-consumedBytes)

	// 4. 移除已解析的数据，保留未完成的部分
	if consumedBytes > 0 {
		// 更新 last valid offset
		fileState.LastValidOffset += int64(consumedBytes)

		// 保留未消费的数据
		if consumedBytes < len(fileState.Buffer) {
			fileState.Buffer = fileState.Buffer[consumedBytes:]
			log.Debugf("Retained %d bytes in buffer for next parse", len(fileState.Buffer))
		} else {
			fileState.Buffer = fileState.Buffer[:0]
		}
	}

	// 5. 存储每个 event 的 scalar 数据到 training_performance 表
	scalarEventCount := 0
	successCount := 0
	duplicateCount := 0

	for _, event := range events {
		if !event.IsScalarEvent() {
			continue
		}

		scalarEventCount++

		// 将所有 scalars 合并到一个 performance 记录中
		performance := model.ExtType{
			"wall_time": event.WallTime,
			"step":      event.Step,
			"scalars":   event.Scalars,
			"tags":      event.Tags,
			"file":      update.File, // 记录来源文件
		}

		trainingPerf := &model.TrainingPerformance{
			WorkloadUID: task.WorkloadUID,
			PodUUID:     gpuPod.UID,
			Performance: performance,
			Iteration:   int32(event.Step),
			Serial:      0, // 可以从 pod 名称或其他地方提取
			DataSource:  "tensorflow",
			CreatedAt:   time.Now(),
		}

		// 检查是否已存在（防止重复插入）
		existing, err := e.trainingFacade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(
			ctx, task.WorkloadUID, 0, int(event.Step))

		if err != nil {
			log.Warnf("Failed to check existing record for step %d: %v", event.Step, err)
		} else if existing != nil {
			// 记录已存在，跳过
			duplicateCount++
			if duplicateCount <= 3 {
				log.Debugf("Skipping duplicate event: step=%d (already in database)", event.Step)
			}
			continue
		}

		// 存储到数据库
		err = e.trainingFacade.CreateTrainingPerformance(ctx, trainingPerf)
		if err != nil {
			// 可能是唯一键冲突（并发插入），不算错误
			if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
				duplicateCount++
			} else {
				log.Warnf("Failed to store training performance for step %d: %v", event.Step, err)
			}
		} else {
			successCount++
			if successCount <= 5 {
				log.Debugf("Stored training performance: step=%d, scalars=%v", event.Step, event.Scalars)
			}
		}
	}

	if scalarEventCount > 0 {
		log.Infof("Processed %d scalar events from %s (stored: %d, duplicates: %d)",
			scalarEventCount, update.File, successCount, duplicateCount)
	}
}

// getNodeExporterClient gets or creates a node-exporter client for a specific node
func (e *TensorBoardStreamExecutor) getNodeExporterClient(ctx context.Context, nodeName string) (*client.Client, error) {
	cacheKey := nodeName

	// Check cache first
	if cached, ok := e.clientCache.Load(cacheKey); ok {
		return cached.(*client.Client), nil
	}

	// Get node-exporter pod on the target node using existing clientsets implementation
	k8sClient := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet.ControllerRuntimeClient

	nodeExporterK8sClient, err := clientsets.GetOrInitNodeExportersClient(ctx, nodeName, k8sClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get node-exporter client for node %s: %w", nodeName, err)
	}

	// Convert to our client type by creating a new client with the baseURL
	// The address from GetOrInitNodeExportersClient already includes http:// and port
	// Client request paths include /v1 prefix, so baseURL should not include it
	baseURL := nodeExporterK8sClient.GetRestyClient().BaseURL
	nodeExporterClient := client.NewClient(client.DefaultConfig(baseURL))

	// Cache the client
	e.clientCache.Store(cacheKey, nodeExporterClient)

	log.Infof("Created node-exporter client for node %s at %s", nodeName, baseURL)
	return nodeExporterClient, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two float64
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// saveMetadata saves metadata to a .meta file
func (e *TensorBoardStreamExecutor) saveMetadata(fileState *FileParseState) {
	if fileState.Metadata == nil || fileState.DebugFilePath == "" {
		return
	}

	metaPath := fileState.DebugFilePath + ".meta"
	data, err := json.MarshalIndent(fileState.Metadata, "", "  ")
	if err != nil {
		log.Warnf("Failed to marshal metadata: %v", err)
		return
	}

	err = ioutil.WriteFile(metaPath, data, 0644)
	if err != nil {
		log.Warnf("Failed to write metadata to %s: %v", metaPath, err)
	} else {
		log.Infof("Saved metadata to %s", metaPath)
	}
}

// cleanupDebugFiles closes and finalizes debug files
func (e *TensorBoardStreamExecutor) cleanupDebugFiles() {
	for filePath, fileState := range e.fileBuffers {
		if fileState.DebugFile != nil {
			// Save final metadata
			if fileState.Metadata != nil {
				fileState.Metadata.LastUpdated = time.Now()
				e.saveMetadata(fileState)
			}

			// Close file
			if err := fileState.DebugFile.Close(); err != nil {
				log.Warnf("Failed to close debug file for %s: %v", filePath, err)
			} else {
				log.Infof("Closed debug file: %s", fileState.DebugFilePath)
			}
		}
	}
}
