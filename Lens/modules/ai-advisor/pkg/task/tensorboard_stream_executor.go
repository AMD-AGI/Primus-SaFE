package task

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
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

	reader         *tensorboard.Reader
	metadataFacade database.AiWorkloadMetadataFacadeInterface
	podFacade      database.PodFacadeInterface
	taskFacade     database.WorkloadTaskFacadeInterface
	trainingFacade database.TrainingFacadeInterface
	nodeFacade     database.NodeFacadeInterface
	eventParser    *tensorboard.EventParser

	// 文件读取状态管理（每个文件维护一个状态）
	fileStates map[string]*FileStreamState

	// Node-exporter client cache
	clientCache sync.Map // nodeName -> *client.Client
}

// FileStreamState 文件流式读取状态
type FileStreamState struct {
	FilePath         string         // 文件路径
	CurrentOffset    int64          // 下次读取的起始位置（基于成功解析的位置）
	LastReadTime     time.Time      // 最后读取时间
	DebugFile        *os.File       // 调试文件句柄（可选）
	DebugFilePath    string         // 调试文件路径
	TotalBytesParsed int64          // 已成功解析的总字节数
	Metadata         *DebugMetadata // 调试元信息（可选）
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
func NewTensorBoardStreamExecutor() *TensorBoardStreamExecutor {
	return &TensorBoardStreamExecutor{
		reader:         tensorboard.NewReader(),
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
		podFacade:      database.NewPodFacade(),
		taskFacade:     database.NewWorkloadTaskFacade(),
		trainingFacade: database.NewTrainingFacade(),
		nodeFacade:     database.NewNodeFacade(),
		eventParser:    tensorboard.NewEventParser(),
		fileStates:     make(map[string]*FileStreamState),
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

	chunkSize := int64(65536) // 默认 64KB
	if chunkSizeVal, ok := task.Ext["chunk_size"]; ok {
		if chunkSizeInt, ok := chunkSizeVal.(int); ok {
			chunkSize = int64(chunkSizeInt)
		} else if chunkSizeFloat, ok := chunkSizeVal.(float64); ok {
			chunkSize = int64(chunkSizeFloat)
		}
	}

	// 4. 从 checkpoint 恢复 offset 并初始化文件状态
	if checkpoint != nil {
		if fileOffsets, ok := checkpoint["file_offsets"].(map[string]interface{}); ok {
			for file, offset := range fileOffsets {
				if offsetVal, ok := offset.(float64); ok {
					e.fileStates[file] = &FileStreamState{
						FilePath:      file,
						CurrentOffset: int64(offsetVal),
					}
				}
			}
		}
	}

	log.Infof("Starting stream with event files: %v, initial offsets: %+v", eventFiles, e.getFileOffsets())

	// 5. 获取 node-exporter client
	nodeExporterClient, err := e.getNodeExporterClient(ctx, gpuPod.NodeName)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get node-exporter client: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 6. 如果文件列表为空，先扫描文件
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

	// 7. 初始化文件状态（如果还没有从 checkpoint 恢复）
	for _, filePath := range eventFiles {
		if _, exists := e.fileStates[filePath]; !exists {
			e.fileStates[filePath] = &FileStreamState{
				FilePath:      filePath,
				CurrentOffset: 0,
			}
		}
	}

	log.Infof("Stream started for workload %s with %d files, poll interval %ds, chunk size %d",
		task.WorkloadUID, len(eventFiles), pollInterval, chunkSize)

	// 8. 进入流式读取循环（同步阻塞直到任务被取消）
	e.streamLoop(ctx, task, gpuPod, eventFiles, time.Duration(pollInterval)*time.Second, chunkSize)

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

// streamLoop 流式读取主循环
func (e *TensorBoardStreamExecutor) streamLoop(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	eventFiles []string,
	pollInterval time.Duration,
	chunkSize int64,
) {
	defer e.cleanupDebugFiles()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	checkpointTicker := time.NewTicker(10 * time.Second) // 每 10 秒更新一次 checkpoint
	defer checkpointTicker.Stop()

	totalBytesRead := int64(0)
	updateCount := 0

	log.Infof("Started stream loop for workload %s", task.WorkloadUID)

	for {
		select {
		case <-ctx.Done():
			log.Infof("Stream loop stopped by context for workload %s", task.WorkloadUID)
			// 最后一次更新 checkpoint
			e.updateCheckpoint(ctx, task, totalBytesRead, updateCount)
			return

		case <-ticker.C:
			// 轮询每个文件，检查是否有新数据
			for _, filePath := range eventFiles {
				if ctx.Err() != nil {
					return
				}

				fileState := e.fileStates[filePath]

				// 读取、解析并更新 offset（基于成功解析的位置）
				bytesRead, err := e.readAndParseFile(ctx, task, gpuPod, filePath, fileState, chunkSize)
				if err != nil {
					log.Errorf("Failed to read/parse file %s: %v", filePath, err)
					continue
				}

				if bytesRead > 0 {
					totalBytesRead += bytesRead
					updateCount++
				}
			}

		case <-checkpointTicker.C:
			// 定期更新 checkpoint
			e.updateCheckpoint(ctx, task, totalBytesRead, updateCount)
		}
	}
}

// readAndParseFile 读取文件、解析事件、更新offset（基于成功解析的位置）
func (e *TensorBoardStreamExecutor) readAndParseFile(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	filePath string,
	fileState *FileStreamState,
	chunkSize int64,
) (int64, error) {
	// 1. 获取文件信息
	fileInfo, err := e.reader.GetFileInfo(ctx, gpuPod.UID, filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}

	// 2. 检查是否有新数据
	if fileInfo.Size <= fileState.CurrentOffset {
		return 0, nil // 没有新数据
	}

	// 3. 计算读取大小
	remaining := fileInfo.Size - fileState.CurrentOffset
	toRead := chunkSize
	if toRead > remaining {
		toRead = remaining
	}

	// 4. 读取数据块
	resp, err := e.reader.ReadFile(ctx, gpuPod.UID, filePath, fileState.CurrentOffset, toRead)
	if err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}

	if resp.BytesRead == 0 {
		return 0, nil
	}

	log.Debugf("Read from %s: offset=%d, bytes=%d", filePath, fileState.CurrentOffset, resp.BytesRead)

	// 5. 解析事件（返回成功解析的字节数）
	dataBytes := []byte(resp.Content)
	events, consumedBytes, parseErr := e.eventParser.ParseEventsWithBuffer(dataBytes)

	if parseErr != nil {
		log.Warnf("Parse error for %s at offset %d: %v", filePath, fileState.CurrentOffset, parseErr)
	}

	log.Debugf("Parse result for %s: events=%d, consumed=%d/%d bytes (%.1f%%)",
		filePath, len(events), consumedBytes, len(dataBytes), float64(consumedBytes)*100/max(float64(len(dataBytes)), 1))

	// 6. 关键：只更新成功解析的部分，丢弃无法解析的数据
	// 下次读取从解析成功的位置开始
	if consumedBytes > 0 {
		fileState.CurrentOffset += int64(consumedBytes)
		fileState.TotalBytesParsed += int64(consumedBytes)
		fileState.LastReadTime = time.Now()

		log.Debugf("Updated offset for %s: %d (consumed %d bytes, discarded %d bytes)",
			filePath, fileState.CurrentOffset, consumedBytes, len(dataBytes)-consumedBytes)
	} else if len(dataBytes) > 1024*1024 {
		// 如果超过 1MB 还无法解析，可能数据损坏，跳过这部分
		log.Warnf("Unable to parse %d bytes from %s, skipping to avoid infinite loop",
			len(dataBytes), filePath)
		fileState.CurrentOffset += int64(len(dataBytes))
		return 0, nil
	}

	// 7. 存储解析成功的事件到数据库
	if len(events) > 0 {
		e.storeEvents(ctx, task, gpuPod, filePath, events)
	}

	return int64(consumedBytes), nil
}

// storeEvents 存储事件到数据库
func (e *TensorBoardStreamExecutor) storeEvents(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	filePath string,
	events []*tensorboard.ParsedEvent,
) {
	successCount := 0
	duplicateCount := 0

	for _, event := range events {
		if !event.IsScalarEvent() {
			continue
		}

		// 合并所有 scalars 到一个 performance 记录中
		performance := model.ExtType{
			"wall_time": event.WallTime,
			"step":      event.Step,
			"scalars":   event.Scalars,
			"tags":      event.Tags,
			"file":      filePath,
		}

		trainingPerf := &model.TrainingPerformance{
			WorkloadUID: task.WorkloadUID,
			PodUUID:     gpuPod.UID,
			Performance: performance,
			Iteration:   int32(event.Step),
			Serial:      0,
			DataSource:  "tensorflow",
			CreatedAt:   time.Now(),
		}

		// 检查是否已存在（防止重复插入）
		existing, err := e.trainingFacade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(
			ctx, task.WorkloadUID, 0, int(event.Step))

		if err != nil {
			log.Warnf("Failed to check existing record for step %d: %v", event.Step, err)
		} else if existing != nil {
			duplicateCount++
			if duplicateCount <= 3 {
				log.Debugf("Skipping duplicate event: step=%d", event.Step)
			}
			continue
		}

		// 存储到数据库
		err = e.trainingFacade.CreateTrainingPerformance(ctx, trainingPerf)
		if err != nil {
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

	if successCount > 0 || duplicateCount > 0 {
		log.Infof("Stored events from %s: %d new, %d duplicates", filePath, successCount, duplicateCount)
	}
}

// getFileOffsets 获取所有文件的当前 offset
func (e *TensorBoardStreamExecutor) getFileOffsets() map[string]int64 {
	offsets := make(map[string]int64)
	for filePath, state := range e.fileStates {
		offsets[filePath] = state.CurrentOffset
	}
	return offsets
}

// updateCheckpoint 更新 checkpoint 到数据库
func (e *TensorBoardStreamExecutor) updateCheckpoint(
	ctx context.Context,
	task *model.WorkloadTaskState,
	totalBytesRead int64,
	updateCount int,
) {
	checkpoint := map[string]interface{}{
		"file_offsets":     e.getFileOffsets(),
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
			task.WorkloadUID, len(e.fileStates), totalBytesRead, updateCount)
	}
}

// Cancel 取消任务
func (e *TensorBoardStreamExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Cancelling TensorBoard stream for workload %s", task.WorkloadUID)
	// 任务取消通过 context 控制，这里不需要额外操作
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

// cleanupDebugFiles closes and finalizes debug files (if enabled)
func (e *TensorBoardStreamExecutor) cleanupDebugFiles() {
	for filePath, fileState := range e.fileStates {
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

// saveMetadata saves metadata to a .meta file
func (e *TensorBoardStreamExecutor) saveMetadata(fileState *FileStreamState) {
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
		log.Debugf("Saved metadata to %s", metaPath)
	}
}
