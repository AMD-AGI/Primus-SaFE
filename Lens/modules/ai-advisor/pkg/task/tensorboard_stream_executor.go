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
	FilePath            string         // 文件路径
	CurrentOffset       int64          // 下次读取的起始位置（基于成功解析的位置）
	LastReadTime        time.Time      // 最后读取时间
	DebugFile           *os.File       // 调试文件句柄（可选）
	DebugFilePath       string         // 调试文件路径
	TotalBytesParsed    int64          // 已成功解析的总字节数
	Metadata            *DebugMetadata // 调试元信息（可选）
	ConsecutiveFailures int            // 连续解析失败次数（用于动态调整chunk size）
	CurrentChunkSize    int64          // 当前使用的chunk size
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
				FilePath:            filePath,
				CurrentOffset:       0,
				CurrentChunkSize:    chunkSize,
				ConsecutiveFailures: 0,
			}
		} else {
			// 从checkpoint恢复的状态，也要初始化chunk size
			if e.fileStates[filePath].CurrentChunkSize == 0 {
				e.fileStates[filePath].CurrentChunkSize = chunkSize
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

	// 3. 计算读取大小（使用fileState中的动态chunk size）
	remaining := fileInfo.Size - fileState.CurrentOffset
	toRead := fileState.CurrentChunkSize
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
	// Content is already decoded from base64 by the client
	dataBytes := []byte(resp.Content)

	// Debug: verify data length consistency
	if len(dataBytes) != int(resp.BytesRead) {
		log.Warnf("Data length mismatch: BytesRead=%d, len(dataBytes)=%d",
			resp.BytesRead, len(dataBytes))
	}

	events, consumedBytes, parseErr := e.eventParser.ParseEventsWithBuffer(dataBytes)

	if parseErr != nil {
		log.Warnf("Parse error for %s at offset %d: %v", filePath, fileState.CurrentOffset, parseErr)
	}

	log.Debugf("Parse result for %s: events=%d, consumed=%d/%d bytes (%.1f%%), scalars=%d, texts=%d",
		filePath, len(events), consumedBytes, len(dataBytes),
		float64(consumedBytes)*100/max(float64(len(dataBytes)), 1),
		e.countScalars(events), e.countTexts(events))

	// 6. 核心策略：只有成功解析出事件时才更新offset
	// 如果没有解析出事件，保持offset不变，下次重新读取
	if len(events) > 0 {
		// 成功解析出事件，更新offset到已消费的位置
		fileState.CurrentOffset += int64(consumedBytes)
		fileState.TotalBytesParsed += int64(consumedBytes)
		fileState.LastReadTime = time.Now()
		fileState.ConsecutiveFailures = 0 // 重置失败计数

		log.Infof("Successfully parsed %d events from %s: offset %d -> %d (+%d bytes, %d scalars, %d texts)",
			len(events), filePath, fileState.CurrentOffset-int64(consumedBytes),
			fileState.CurrentOffset, consumedBytes, e.countScalars(events), e.countTexts(events))

		// 存储解析成功的事件到数据库
		e.storeEvents(ctx, task, gpuPod, filePath, events)

		return int64(consumedBytes), nil
	} else {
		// 没有解析出事件，增加失败计数
		fileState.ConsecutiveFailures++

		// 检查是否需要增大读取大小
		if len(dataBytes) >= int(fileState.CurrentChunkSize) {
			// 已经读取了完整的chunk size，但仍无法解析出事件
			// 可能是事件太大，需要增大chunk size重试
			if fileState.ConsecutiveFailures >= 3 && fileState.CurrentChunkSize < 10*1024*1024 {
				// 连续3次失败，且chunk size小于10MB，则加倍chunk size
				oldSize := fileState.CurrentChunkSize
				fileState.CurrentChunkSize = fileState.CurrentChunkSize * 2
				if fileState.CurrentChunkSize > 10*1024*1024 {
					fileState.CurrentChunkSize = 10 * 1024 * 1024 // 最大10MB
				}
				log.Warnf("Increasing chunk size for %s: %d -> %d bytes (consecutive failures: %d)",
					filePath, oldSize, fileState.CurrentChunkSize, fileState.ConsecutiveFailures)
				fileState.ConsecutiveFailures = 0 // 重置计数
			} else {
				log.Debugf("No events parsed from %d bytes at offset %d in %s (failure %d), waiting for more data",
					len(dataBytes), fileState.CurrentOffset, filePath, fileState.ConsecutiveFailures)
			}

			// 如果chunk size已经达到最大（10MB）且连续失败很多次，可能数据损坏
			if fileState.CurrentChunkSize >= 10*1024*1024 && fileState.ConsecutiveFailures >= 10 {
				log.Errorf("Unable to parse events from %s at offset %d after %d attempts with 10MB chunks, likely corrupted",
					filePath, fileState.CurrentOffset, fileState.ConsecutiveFailures)
				// 跳过一小段数据（1KB）尝试恢复
				fileState.CurrentOffset += 1024
				fileState.ConsecutiveFailures = 0
				return 0, fmt.Errorf("skipped corrupted data")
			}
		} else {
			// 读取的数据小于chunk size，说明已经到文件末尾，等待更多数据
			log.Debugf("Incomplete data at offset %d in %s (%d/%d bytes), waiting for more data",
				fileState.CurrentOffset, filePath, len(dataBytes), fileState.CurrentChunkSize)
			// 等待更多数据时不算作失败
			fileState.ConsecutiveFailures = 0
		}

		// offset不变，下次重新读取
		return 0, nil
	}
}

// aggregatedStepData 聚合同一个 iteration 的所有数据
type aggregatedStepData struct {
	Step     int64              // iteration (统一后的主维度)
	WallTime float64             // 时间戳
	Scalars  map[string]float32 // 包含 samples 字段
	Texts    map[string]string  // 配置元数据
	Tags     []string           // 所有 tag 列表
}

// extractBatchSize 从事件中提取 batch_size
func (e *TensorBoardStreamExecutor) extractBatchSize(events []*tensorboard.ParsedEvent) int64 {
	// 从 text metadata 中查找 batch-size 或 global_batch_size
	for _, event := range events {
		if batchSizeStr, ok := event.Texts["batch-size"]; ok {
			if bs := e.parseIntFromString(batchSizeStr); bs > 0 {
				return bs
			}
		}
		if batchSizeStr, ok := event.Texts["global_batch_size"]; ok {
			if bs := e.parseIntFromString(batchSizeStr); bs > 0 {
				return bs
			}
		}
	}
	
	// 从 scalar 中查找
	for _, event := range events {
		if batchSize, ok := event.Scalars["batch-size"]; ok && batchSize > 0 {
			return int64(batchSize)
		}
		if batchSize, ok := event.Scalars["global_batch_size"]; ok && batchSize > 0 {
			return int64(batchSize)
		}
	}
	
	// 默认值：128（LLM 训练常见值）
	return 128
}

// normalizeStep 智能判断 step 类型并转换为 iteration
// 返回：(iteration, samples)
func (e *TensorBoardStreamExecutor) normalizeStep(step int64, batchSize int64) (int64, int64) {
	// 策略1：如果 step 很小（< 100000），认为是 iteration
	if step < 100000 {
		return step, step * batchSize
	}
	
	// 策略2：如果 step 能被 batch_size 整除，认为是 samples
	if batchSize > 0 && step%batchSize == 0 {
		iteration := step / batchSize
		return iteration, step
	}
	
	// 策略3：step 很大但不能整除，可能是自定义维度，保持原样
	// 这种情况较少见，假设是 iteration
	log.Debugf("Ambiguous step value %d (batch_size=%d), treating as iteration", step, batchSize)
	return step, step * batchSize
}

// cleanTagName 清理 tag 名称，去掉 " vs samples" 等后缀
func (e *TensorBoardStreamExecutor) cleanTagName(tag string) string {
	// 去掉 " vs samples" 后缀
	tag = strings.TrimSuffix(tag, " vs samples")
	
	// 去掉 " vs steps" 后缀
	tag = strings.TrimSuffix(tag, " vs steps")
	
	return tag
}

// parseIntFromString 从字符串解析整数
func (e *TensorBoardStreamExecutor) parseIntFromString(s string) int64 {
	var value int64
	_, err := fmt.Sscanf(s, "%d", &value)
	if err != nil {
		return 0
	}
	return value
}

// countScalars counts total number of scalar metrics in events
func (e *TensorBoardStreamExecutor) countScalars(events []*tensorboard.ParsedEvent) int {
	count := 0
	for _, event := range events {
		count += len(event.Scalars)
	}
	return count
}

// countTexts counts total number of text metadata in events
func (e *TensorBoardStreamExecutor) countTexts(events []*tensorboard.ParsedEvent) int {
	count := 0
	for _, event := range events {
		count += len(event.Texts)
	}
	return count
}

// storeEvents 存储事件到数据库
func (e *TensorBoardStreamExecutor) storeEvents(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	filePath string,
	events []*tensorboard.ParsedEvent,
) {
	// Step 1: 提取 batch_size（用于判断和转换 step）
	batchSize := e.extractBatchSize(events)
	
	// Step 2: 按 iteration 聚合所有事件的数据（统一 step 维度）
	iterationAggregated := make(map[int64]*aggregatedStepData)

	for _, event := range events {
		// 智能判断 step 类型并转换为 iteration
		iteration, samples := e.normalizeStep(event.Step, batchSize)
		
		if iterationAggregated[iteration] == nil {
			iterationAggregated[iteration] = &aggregatedStepData{
				Step:     iteration,  // 使用 iteration 作为主 step
				WallTime: event.WallTime,
				Scalars:  make(map[string]float32),
				Texts:    make(map[string]string),
				Tags:     make([]string, 0),
			}
		}

		agg := iterationAggregated[iteration]

		// 合并 scalars（去掉 " vs samples" 等后缀）
		for tag, value := range event.Scalars {
			cleanTag := e.cleanTagName(tag)
			agg.Scalars[cleanTag] = value
			if !contains(agg.Tags, cleanTag) {
				agg.Tags = append(agg.Tags, cleanTag)
			}
		}

		// 合并 texts（去掉 " vs samples" 等后缀）
		for tag, text := range event.Texts {
			cleanTag := e.cleanTagName(tag)
			agg.Texts[cleanTag] = text
			if !contains(agg.Tags, cleanTag) {
				agg.Tags = append(agg.Tags, cleanTag)
			}
		}

		// 更新 samples（如果当前事件提供了更大的值）
		if existingSamples, ok := agg.Scalars["samples"]; !ok || float32(samples) > existingSamples {
			agg.Scalars["samples"] = float32(samples)
		}
		
		// 使用最新的 wall_time
		if event.WallTime > agg.WallTime {
			agg.WallTime = event.WallTime
		}
	}

	// Step 3: 存储聚合后的数据
	successCount := 0
	duplicateCount := 0

	for iteration, agg := range iterationAggregated {
		// 只存储有数据的 iteration
		if len(agg.Scalars) == 0 && len(agg.Texts) == 0 {
			continue
		}

		// 构建 performance JSON，包含所有维度和指标
		performance := model.ExtType{
			"iteration": iteration,                    // 主维度
			"samples":   agg.Scalars["samples"],       // 累计样本数
			"wall_time": agg.WallTime,                 // 时间戳
			"file":      filePath,
		}

		// 将 scalars 和 texts 的内容展平到 performance 根级别
		for tag, value := range agg.Scalars {
			// samples 已经单独处理，跳过重复
			if tag != "samples" {
				performance[tag] = value
			}
		}
		for tag, text := range agg.Texts {
			performance[tag] = text
		}

		// 保留原始的 scalars 和 texts 结构（可选，用于调试）
		if len(agg.Scalars) > 0 {
			performance["scalars"] = agg.Scalars
		}
		if len(agg.Texts) > 0 {
			performance["texts"] = agg.Texts
		}

		trainingPerf := &model.TrainingPerformance{
			WorkloadUID: task.WorkloadUID,
			PodUUID:     gpuPod.UID,
			Performance: performance,
			Iteration:   int32(iteration), // 使用统一的 iteration
			Serial:      0,
			DataSource:  "tensorflow",
			CreatedAt:   time.Now(),
		}

		// 检查是否已存在
		existing, err := e.trainingFacade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(
			ctx, task.WorkloadUID, 0, int(iteration))

		if err != nil {
			log.Warnf("Failed to check existing record for iteration %d: %v", iteration, err)
			continue
		}

		if existing != nil {
			// 记录已存在，合并新数据到现有记录
			existingPerf := existing.Performance
			if existingPerf == nil {
				existingPerf = make(model.ExtType)
			}

			mergedCount := 0
			newCount := 0

			// 合并新数据（新值会覆盖旧值）
			for key, value := range performance {
				if _, exists := existingPerf[key]; exists {
					mergedCount++
				} else {
					newCount++
				}
				existingPerf[key] = value
			}

			// 更新记录（UpdateTrainingPerformance 会保留原始的 CreatedAt）
			existing.Performance = existingPerf

			err = e.trainingFacade.UpdateTrainingPerformance(ctx, existing)
			if err != nil {
				log.Warnf("Failed to merge training performance for iteration %d: %v", iteration, err)
			} else {
				successCount++
				if successCount <= 5 {
					samples := int64(0)
					if s, ok := agg.Scalars["samples"]; ok {
						samples = int64(s)
					}
					log.Debugf("Merged performance: iteration=%d (samples=%d): %d updated, %d new (total=%d metrics)",
						iteration, samples, mergedCount, newCount, len(existingPerf))
				}
			}
		} else {
			// 记录不存在，创建新记录
			err = e.trainingFacade.CreateTrainingPerformance(ctx, trainingPerf)
			if err != nil {
				if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
					duplicateCount++
					if duplicateCount <= 3 {
						log.Debugf("Race condition: iteration %d was created by another process", iteration)
					}
				} else {
					log.Warnf("Failed to store training performance for iteration %d: %v", iteration, err)
				}
			} else {
				successCount++
				if successCount <= 5 {
					samples := int64(0)
					if s, ok := agg.Scalars["samples"]; ok {
						samples = int64(s)
					}
					log.Debugf("Created performance: iteration=%d (samples=%d): %d metrics (scalars=%d, texts=%d)",
						iteration, samples, len(agg.Scalars)+len(agg.Texts), len(agg.Scalars), len(agg.Texts))
				}
			}
		}
	}

	if successCount > 0 || duplicateCount > 0 {
		log.Infof("Stored events from %s: %d created/merged, %d skipped (unique iterations: %d)",
			filePath, successCount, duplicateCount, len(iterationAggregated))
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
