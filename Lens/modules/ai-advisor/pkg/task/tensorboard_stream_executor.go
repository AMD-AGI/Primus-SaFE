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

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/hyperparameters"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/tensorboard"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/client"
)

// TensorBoardStreamExecutor TensorBoard streaming task executor
type TensorBoardStreamExecutor struct {
	coreTask.BaseExecutor

	reader          *tensorboard.Reader
	metadataFacade  database.AiWorkloadMetadataFacadeInterface
	podFacade       database.PodFacadeInterface
	taskFacade      database.WorkloadTaskFacadeInterface
	trainingFacade  database.TrainingFacadeInterface
	nodeFacade      database.NodeFacadeInterface
	workloadFacade  database.WorkloadFacadeInterface
	eventParser     *tensorboard.EventParser

	// Hyperparameters collection
	hpCollector      *hyperparameters.Collector
	hpStorage        *hyperparameters.Storage
	hpCollected      bool       // flag: whether hyperparameters have been collected
	hpCollectedMutex sync.Mutex // mutex to protect hpCollected

	// File reading state management (one state per file)
	fileStates map[string]*FileStreamState

	// Node-exporter client cache
	clientCache sync.Map // nodeName -> *client.Client
}

// FileStreamState file streaming read state
type FileStreamState struct {
	FilePath            string         // file path
	CurrentOffset       int64          // starting position for next read (based on successfully parsed position)
	LastReadTime        time.Time      // last read time
	DebugFile           *os.File       // debug file handle (optional)
	DebugFilePath       string         // debug file path
	TotalBytesParsed    int64          // total bytes successfully parsed
	Metadata            *DebugMetadata // debug metadata (optional)
	ConsecutiveFailures int            // consecutive parse failure count (for dynamic chunk size adjustment)
	CurrentChunkSize    int64          // current chunk size in use
}

// DebugMetadata debug metadata
type DebugMetadata struct {
	FileName          string      `json:"file_name"`
	OriginalPath      string      `json:"original_path"`
	TotalBytesWritten int64       `json:"total_bytes_written"`
	Errors            []ErrorInfo `json:"errors"`
	CreatedAt         time.Time   `json:"created_at"`
	LastUpdated       time.Time   `json:"last_updated"`
}

// ErrorInfo error information
type ErrorInfo struct {
	Offset        int64     `json:"offset"`
	BufferSize    int       `json:"buffer_size"`
	Error         string    `json:"error"`
	Timestamp     time.Time `json:"timestamp"`
	BytesConsumed int       `json:"bytes_consumed"`
	EventsParsed  int       `json:"events_parsed"`
}

// NewTensorBoardStreamExecutor creates TensorBoard streaming executor
func NewTensorBoardStreamExecutor() *TensorBoardStreamExecutor {
	return &TensorBoardStreamExecutor{
		reader:         tensorboard.NewReader(),
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
		podFacade:      database.NewPodFacade(),
		taskFacade:     database.NewWorkloadTaskFacade(),
		trainingFacade: database.NewTrainingFacade(),
		nodeFacade:     database.NewNodeFacade(),
		workloadFacade: database.NewWorkloadFacade(),
		eventParser:    tensorboard.NewEventParser(),
		hpCollector:    hyperparameters.NewCollector(),
		hpStorage:      hyperparameters.NewStorage(),
		hpCollected:    false,
		fileStates:     make(map[string]*FileStreamState),
	}
}

// GetTaskType returns task type
func (e *TensorBoardStreamExecutor) GetTaskType() string {
	return constant.TaskTypeTensorBoardStream
}

// Validate validates task parameters
func (e *TensorBoardStreamExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute executes TensorBoard streaming task
func (e *TensorBoardStreamExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task

	log.Infof("Starting TensorBoard streaming for workload %s", task.WorkloadUID)

	// 1. Get TensorBoard event files list from task ext (can be empty)
	var eventFiles []string
	eventFilesRaw, ok := task.Ext["event_files"]
	if ok && eventFilesRaw != nil {
		// Convert event_files to string array
		var err error
		eventFiles, err = e.parseEventFiles(eventFilesRaw)
		if err != nil {
			log.Warnf("Failed to parse event_files: %v, will wait for files to appear", err)
			eventFiles = []string{} // set to empty array, continue execution
		}
	}

	// Get optional log_dir
	logDir := ""
	if logDirVal, ok := task.Ext["log_dir"]; ok {
		if logDirStr, ok := logDirVal.(string); ok {
			logDir = logDirStr
		}
	}

	// If event files list is empty, need to wait for files to appear (keep scanning as long as pod exists)
	if len(eventFiles) == 0 {
		if logDir == "" {
			log.Infof("TensorBoard: waiting for files to appear (log_dir not specified, will scan container)")
		} else {
			log.Infof("TensorBoard: waiting for files to appear in %s", logDir)
		}
	} else {
		log.Infof("TensorBoard event files: %v (log_dir: %s)", eventFiles, logDir)
	}

	// 2. Get pod information
	gpuPod, err := e.selectTargetPod(ctx, task.WorkloadUID)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get pod info: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 3. Get configuration and checkpoint from ext
	var checkpoint map[string]interface{}
	if checkpointVal, ok := task.Ext["checkpoint"]; ok {
		if checkpointMap, ok := checkpointVal.(map[string]interface{}); ok {
			checkpoint = checkpointMap
		}
	}

	pollInterval := 5 // default 5 seconds
	if pollIntervalVal, ok := task.Ext["poll_interval"]; ok {
		if pollIntervalInt, ok := pollIntervalVal.(int); ok {
			pollInterval = pollIntervalInt
		} else if pollIntervalFloat, ok := pollIntervalVal.(float64); ok {
			pollInterval = int(pollIntervalFloat)
		}
	}

	chunkSize := int64(65536) // default 64KB
	if chunkSizeVal, ok := task.Ext["chunk_size"]; ok {
		if chunkSizeInt, ok := chunkSizeVal.(int); ok {
			chunkSize = int64(chunkSizeInt)
		} else if chunkSizeFloat, ok := chunkSizeVal.(float64); ok {
			chunkSize = int64(chunkSizeFloat)
		}
	}

	// 4. Restore offset from checkpoint and initialize file state
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

	// 5. Get node-exporter client
	nodeExporterClient, err := e.getNodeExporterClient(ctx, gpuPod.NodeName)
	if err != nil {
		return coreTask.FailureResult(
			fmt.Sprintf("failed to get node-exporter client: %v", err),
			map[string]interface{}{
				"error_at": time.Now().Format(time.RFC3339),
			},
		), err
	}

	// 6. If file list is empty, scan for files first
	if len(eventFiles) == 0 {
		log.Infof("Waiting for TensorBoard files to appear")

		fileWaitTimeout := 300 // default 5 minutes
		if timeoutVal, ok := task.Ext["file_wait_timeout"]; ok {
			if timeoutInt, ok := timeoutVal.(int); ok {
				fileWaitTimeout = timeoutInt
			} else if timeoutFloat, ok := timeoutVal.(float64); ok {
				fileWaitTimeout = int(timeoutFloat)
			}
		}

		fileScanInterval := 10 // default 10 seconds
		if intervalVal, ok := task.Ext["file_scan_interval"]; ok {
			if intervalInt, ok := intervalVal.(int); ok {
				fileScanInterval = intervalInt
			} else if intervalFloat, ok := intervalVal.(float64); ok {
				fileScanInterval = int(intervalFloat)
			}
		}

		// Wait for files to appear
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

	// 7. Initialize file state (if not yet restored from checkpoint)
	for _, filePath := range eventFiles {
		if _, exists := e.fileStates[filePath]; !exists {
			e.fileStates[filePath] = &FileStreamState{
				FilePath:            filePath,
				CurrentOffset:       0,
				CurrentChunkSize:    chunkSize,
				ConsecutiveFailures: 0,
			}
		} else {
			// For state restored from checkpoint, also initialize chunk size
			if e.fileStates[filePath].CurrentChunkSize == 0 {
				e.fileStates[filePath].CurrentChunkSize = chunkSize
			}
		}
	}

	log.Infof("Stream started for workload %s with %d files, poll interval %ds, chunk size %d",
		task.WorkloadUID, len(eventFiles), pollInterval, chunkSize)

	// 8. Enter streaming read loop (blocks synchronously until task is cancelled)
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

// streamLoop main streaming read loop
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

	checkpointTicker := time.NewTicker(10 * time.Second) // update checkpoint every 10 seconds
	defer checkpointTicker.Stop()

	// Check workload status every 30 seconds
	workloadCheckTicker := time.NewTicker(30 * time.Second)
	defer workloadCheckTicker.Stop()

	totalBytesRead := int64(0)
	updateCount := 0

	log.Infof("Started stream loop for workload %s", task.WorkloadUID)

	for {
		select {
		case <-ctx.Done():
			log.Infof("Stream loop stopped by context for workload %s", task.WorkloadUID)
			// Last checkpoint update
			e.updateCheckpoint(ctx, task, totalBytesRead, updateCount)
			return

		case <-workloadCheckTicker.C:
			// Check if workload has ended
			if e.isWorkloadEnded(ctx, task.WorkloadUID) {
				log.Infof("Workload %s has ended, stopping TensorBoard stream", task.WorkloadUID)
				e.updateCheckpoint(ctx, task, totalBytesRead, updateCount)
				return
			}

		case <-ticker.C:
			// Poll each file to check for new data
			for _, filePath := range eventFiles {
				if ctx.Err() != nil {
					return
				}

				fileState := e.fileStates[filePath]

				// Read, parse and update offset (based on successfully parsed position)
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
			// Periodically update checkpoint
			e.updateCheckpoint(ctx, task, totalBytesRead, updateCount)
		}
	}
}

// readAndParseFile reads file, parses events, updates offset (based on successfully parsed position)
func (e *TensorBoardStreamExecutor) readAndParseFile(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	filePath string,
	fileState *FileStreamState,
	chunkSize int64,
) (int64, error) {
	// 1. Get file information
	fileInfo, err := e.reader.GetFileInfo(ctx, gpuPod.UID, filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}

	// 2. Check if there is new data
	if fileInfo.Size <= fileState.CurrentOffset {
		return 0, nil // no new data
	}

	// 3. Calculate read size (using dynamic chunk size from fileState)
	remaining := fileInfo.Size - fileState.CurrentOffset
	toRead := fileState.CurrentChunkSize
	if toRead > remaining {
		toRead = remaining
	}

	// 4. Read data chunk
	resp, err := e.reader.ReadFile(ctx, gpuPod.UID, filePath, fileState.CurrentOffset, toRead)
	if err != nil {
		return 0, fmt.Errorf("failed to read file: %w", err)
	}

	if resp.BytesRead == 0 {
		return 0, nil
	}

	log.Debugf("Read from %s: offset=%d, bytes=%d", filePath, fileState.CurrentOffset, resp.BytesRead)

	// 5. Parse events (returns successfully parsed byte count)
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

	// 6. Core strategy: only update offset when events are successfully parsed
	// If no events parsed, keep offset unchanged and reread next time
	if len(events) > 0 {
		// Successfully parsed events, update offset to consumed position
		fileState.CurrentOffset += int64(consumedBytes)
		fileState.TotalBytesParsed += int64(consumedBytes)
		fileState.LastReadTime = time.Now()
		fileState.ConsecutiveFailures = 0 // reset failure count

		log.Infof("Successfully parsed %d events from %s: offset %d -> %d (+%d bytes, %d scalars, %d texts)",
			len(events), filePath, fileState.CurrentOffset-int64(consumedBytes),
			fileState.CurrentOffset, consumedBytes, e.countScalars(events), e.countTexts(events))

		// Store successfully parsed events to database
		e.storeEvents(ctx, task, gpuPod, filePath, events)

		return int64(consumedBytes), nil
	} else {
		// No events parsed, increase failure count
		fileState.ConsecutiveFailures++

		// Check if need to increase read size
		if len(dataBytes) >= int(fileState.CurrentChunkSize) {
			// Already read full chunk size but still unable to parse events
			// Possibly event is too large, need to increase chunk size and retry
			if fileState.ConsecutiveFailures >= 3 && fileState.CurrentChunkSize < 10*1024*1024 {
				// 3 consecutive failures and chunk size < 10MB, double the chunk size
				oldSize := fileState.CurrentChunkSize
				fileState.CurrentChunkSize = fileState.CurrentChunkSize * 2
				if fileState.CurrentChunkSize > 10*1024*1024 {
					fileState.CurrentChunkSize = 10 * 1024 * 1024 // maximum 10MB
				}
				log.Warnf("Increasing chunk size for %s: %d -> %d bytes (consecutive failures: %d)",
					filePath, oldSize, fileState.CurrentChunkSize, fileState.ConsecutiveFailures)
				fileState.ConsecutiveFailures = 0 // reset count
			} else {
				log.Debugf("No events parsed from %d bytes at offset %d in %s (failure %d), waiting for more data",
					len(dataBytes), fileState.CurrentOffset, filePath, fileState.ConsecutiveFailures)
			}

			// If chunk size has reached maximum (10MB) and consecutive failures are many, data may be corrupted
			if fileState.CurrentChunkSize >= 10*1024*1024 && fileState.ConsecutiveFailures >= 10 {
				log.Errorf("Unable to parse events from %s at offset %d after %d attempts with 10MB chunks, likely corrupted",
					filePath, fileState.CurrentOffset, fileState.ConsecutiveFailures)
				// Skip a small segment of data (1KB) to attempt recovery
				fileState.CurrentOffset += 1024
				fileState.ConsecutiveFailures = 0
				return 0, fmt.Errorf("skipped corrupted data")
			}
		} else {
			// Data read is less than chunk size, reached end of file, wait for more data
			log.Debugf("Incomplete data at offset %d in %s (%d/%d bytes), waiting for more data",
				fileState.CurrentOffset, filePath, len(dataBytes), fileState.CurrentChunkSize)
			// Don't count as failure when waiting for more data
			fileState.ConsecutiveFailures = 0
		}

		// Offset unchanged, reread next time
		return 0, nil
	}
}

// aggregatedStepData aggregates all data for the same iteration
type aggregatedStepData struct {
	Step     int64              // iteration (unified primary dimension)
	WallTime float64            // timestamp
	Scalars  map[string]float32 // includes samples field
	Texts    map[string]string  // configuration metadata
	Tags     []string           // list of all tags
}

// extractBatchSize extracts batch_size from events
func (e *TensorBoardStreamExecutor) extractBatchSize(events []*tensorboard.ParsedEvent) int64 {
	// Search for batch-size or global_batch_size in text metadata
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

	// Search in scalars
	for _, event := range events {
		if batchSize, ok := event.Scalars["batch-size"]; ok && batchSize > 0 {
			return int64(batchSize)
		}
		if batchSize, ok := event.Scalars["global_batch_size"]; ok && batchSize > 0 {
			return int64(batchSize)
		}
	}

	// Default value: 128 (common for LLM training)
	return 128
}

// normalizeStep intelligently determines step type and converts to iteration
// Returns: (iteration, samples)
func (e *TensorBoardStreamExecutor) normalizeStep(step int64, batchSize int64) (int64, int64) {
	// Strategy 1: If step is small (< 100000), consider it as iteration
	if step < 100000 {
		return step, step * batchSize
	}

	// Strategy 2: If step is divisible by batch_size, consider it as samples
	if batchSize > 0 && step%batchSize == 0 {
		iteration := step / batchSize
		return iteration, step
	}

	// Strategy 3: step is large but not divisible, may be custom dimension, keep as is
	// This case is rare, assume it's iteration
	log.Debugf("Ambiguous step value %d (batch_size=%d), treating as iteration", step, batchSize)
	return step, step * batchSize
}

// cleanTagName cleans tag name, removes " vs samples" and similar suffixes
func (e *TensorBoardStreamExecutor) cleanTagName(tag string) string {
	// Remove " vs samples" suffix
	tag = strings.TrimSuffix(tag, " vs samples")

	// Remove " vs steps" suffix
	tag = strings.TrimSuffix(tag, " vs steps")

	return tag
}

// parseIntFromString parses integer from string
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

// storeEvents stores events to database
func (e *TensorBoardStreamExecutor) storeEvents(
	ctx context.Context,
	task *model.WorkloadTaskState,
	gpuPod *model.GpuPods,
	filePath string,
	events []*tensorboard.ParsedEvent,
) {
	// Step 0: Try to extract hyperparameters (only execute on first encounter with events containing hyperparameters)
	e.tryCollectHyperparameters(ctx, task, filePath, events)

	// Step 1: Extract batch_size (for determining and converting step)
	batchSize := e.extractBatchSize(events)

	// Step 2: Aggregate all event data by iteration (unify step dimension)
	iterationAggregated := make(map[int64]*aggregatedStepData)

	for _, event := range events {
		// Intelligently determine step type and convert to iteration
		iteration, samples := e.normalizeStep(event.Step, batchSize)

		if iterationAggregated[iteration] == nil {
			iterationAggregated[iteration] = &aggregatedStepData{
				Step:     iteration, // use iteration as primary step
				WallTime: event.WallTime,
				Scalars:  make(map[string]float32),
				Texts:    make(map[string]string),
				Tags:     make([]string, 0),
			}
		}

		agg := iterationAggregated[iteration]

		// Merge scalars (remove " vs samples" and similar suffixes)
		for tag, value := range event.Scalars {
			cleanTag := e.cleanTagName(tag)
			agg.Scalars[cleanTag] = value
			if !contains(agg.Tags, cleanTag) {
				agg.Tags = append(agg.Tags, cleanTag)
			}
		}

		// Merge texts (remove " vs samples" and similar suffixes)
		for tag, text := range event.Texts {
			cleanTag := e.cleanTagName(tag)
			agg.Texts[cleanTag] = text
			if !contains(agg.Tags, cleanTag) {
				agg.Tags = append(agg.Tags, cleanTag)
			}
		}

		// Update samples (if current event provides a larger value)
		if existingSamples, ok := agg.Scalars["samples"]; !ok || float32(samples) > existingSamples {
			agg.Scalars["samples"] = float32(samples)
		}

		// Use latest wall_time
		if event.WallTime > agg.WallTime {
			agg.WallTime = event.WallTime
		}
	}

	// Step 3: Store aggregated data
	successCount := 0
	duplicateCount := 0

	for iteration, agg := range iterationAggregated {
		// Only store iterations with data
		if len(agg.Scalars) == 0 && len(agg.Texts) == 0 {
			continue
		}

		// Build performance JSON, containing all dimensions and metrics
		performance := model.ExtType{
			"iteration": iteration,              // primary dimension
			"samples":   agg.Scalars["samples"], // cumulative sample count
			"wall_time": agg.WallTime,           // timestamp
			"file":      filePath,
		}

		// Flatten scalars and texts content to performance root level
		for tag, value := range agg.Scalars {
			// samples already processed separately, skip duplicate
			if tag != "samples" {
				performance[tag] = value
			}
		}
		for tag, text := range agg.Texts {
			performance[tag] = text
		}

		// Keep original scalars and texts structure (optional, for debugging)
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
			Iteration:   int32(iteration), // use unified iteration
			Serial:      0,
			DataSource:  "tensorflow",
			CreatedAt:   time.Now(),
		}

		// Check if already exists
		existing, err := e.trainingFacade.GetTrainingPerformanceByWorkloadIdSerialAndIteration(
			ctx, task.WorkloadUID, 0, int(iteration))

		if err != nil {
			log.Warnf("Failed to check existing record for iteration %d: %v", iteration, err)
			continue
		}

		if existing != nil {
			// Record already exists, merge new data into existing record
			existingPerf := existing.Performance
			if existingPerf == nil {
				existingPerf = make(model.ExtType)
			}

			mergedCount := 0
			newCount := 0

			// Merge new data (new values will overwrite old values)
			for key, value := range performance {
				if _, exists := existingPerf[key]; exists {
					mergedCount++
				} else {
					newCount++
				}
				existingPerf[key] = value
			}

			// Update record (UpdateTrainingPerformance will preserve original CreatedAt)
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
			// Record does not exist, create new record
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

// getFileOffsets gets current offset for all files
func (e *TensorBoardStreamExecutor) getFileOffsets() map[string]int64 {
	offsets := make(map[string]int64)
	for filePath, state := range e.fileStates {
		offsets[filePath] = state.CurrentOffset
	}
	return offsets
}

// updateCheckpoint updates checkpoint to database
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

// Cancel cancels task
func (e *TensorBoardStreamExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Cancelling TensorBoard stream for workload %s", task.WorkloadUID)
	// Task cancellation is controlled through context, no additional operation needed here
	return nil
}

// waitForTensorBoardFiles waits for TensorBoard files to appear
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
			// Check if timeout
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("timeout waiting for TensorBoard files after %v", timeout)
			}

			// Call FindTensorboardFiles to scan files
			log.Debugf("Scanning for TensorBoard files in pod %s", podUID)

			findResp, err := nodeExporterClient.FindTensorboardFiles(ctx, podUID, gpuPod.Name, gpuPod.Namespace)
			if err != nil {
				log.Warnf("Failed to scan for TensorBoard files: %v, will retry", err)
				continue
			}

			// Extract file paths and deduplicate
			fileSet := make(map[string]bool)
			var eventFiles []string
			for _, fileInfo := range findResp.Files {
				// Deduplicate: only add files not seen before
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

// parseEventFiles parses event files list
func (e *TensorBoardStreamExecutor) parseEventFiles(eventFilesRaw interface{}) ([]string, error) {
	var eventFiles []string

	// Try to convert interface type to string slice
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

// selectTargetPod selects target pod from all pods of a workload
// Prioritizes pods with names ending in master-0, otherwise returns the first one
func (e *TensorBoardStreamExecutor) selectTargetPod(ctx context.Context, workloadUID string) (*model.GpuPods, error) {
	// Method 1: Find pod through workload_pod_reference table (recommended, supports hierarchical relationship)
	workloadFacade := database.GetFacade().GetWorkload()
	podRefs, err := workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload_pod_reference for workload %s: %v", workloadUID, err)
	}

	var pods []*model.GpuPods
	if len(podRefs) > 0 {
		// Query pod details through pod UID list
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}

		// Get pod details
		db := database.GetFacade().GetSystemConfig().GetDB()
		err = db.WithContext(ctx).
			Where("uid IN ? AND deleted = ?", podUIDs, false).
			Find(&pods).Error
		if err != nil {
			return nil, fmt.Errorf("failed to query pods by references: %w", err)
		}

	}

	// Method 2: Find pods of child workload (recursively search hierarchical structure)
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

	// Prioritize pods ending with master-0
	for _, pod := range pods {
		if strings.HasSuffix(pod.Name, "master-0") {
			log.Infof("Selected master-0 pod: %s/%s for workload %s", pod.Namespace, pod.Name, workloadUID)
			return pod, nil
		}
	}

	// If no master-0, return first pod
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

// tryCollectHyperparameters tries to extract hyperparameters from TensorBoard events
// Only executes on first encounter with events containing hyperparameters (usually text events at step=0)
func (e *TensorBoardStreamExecutor) tryCollectHyperparameters(
	ctx context.Context,
	task *model.WorkloadTaskState,
	filePath string,
	events []*tensorboard.ParsedEvent,
) {
	// Quick check: whether already collected
	e.hpCollectedMutex.Lock()
	if e.hpCollected {
		e.hpCollectedMutex.Unlock()
		return
	}

	// Check if there are events containing hyperparameters
	hasHyperparams := false
	for _, event := range events {
		// Hyperparameters are usually at step=0 and contain text data
		if event.Step == 0 && len(event.Texts) > 0 {
			hasHyperparams = true
			break
		}
	}

	if !hasHyperparams {
		e.hpCollectedMutex.Unlock()
		return
	}

	// Mark as collected (avoid duplication)
	e.hpCollected = true
	e.hpCollectedMutex.Unlock()

	log.Infof("Detected hyperparameters in TensorBoard events, starting collection for workload %s", task.WorkloadUID)

	// Extract log_dir
	logDir := ""
	if logDirVal, ok := task.Ext["log_dir"]; ok {
		if logDirStr, ok := logDirVal.(string); ok {
			logDir = logDirStr
		}
	}

	// Prepare collection options
	opts := hyperparameters.CollectionOptions{
		TensorBoardEvents: events,
		TensorBoardLogDir: logDir,
	}

	// Collect hyperparameters
	hparams, err := e.hpCollector.CollectAll(ctx, task.WorkloadUID, opts)
	if err != nil {
		log.Errorf("Failed to collect hyperparameters for workload %s: %v", task.WorkloadUID, err)
		return
	}

	if len(hparams.Merged) == 0 {
		log.Warnf("No hyperparameters extracted for workload %s", task.WorkloadUID)
		return
	}

	log.Infof("Collected %d hyperparameters from %d sources for workload %s",
		len(hparams.Merged), len(hparams.Sources), task.WorkloadUID)

	// Store to workload annotations
	if err := e.hpStorage.Save(ctx, hparams); err != nil {
		log.Errorf("Failed to save hyperparameters for workload %s: %v", task.WorkloadUID, err)
		return
	}

	log.Infof("Successfully saved hyperparameters to workload %s annotations (version %d, %d parameters)",
		task.WorkloadUID, hparams.Version, len(hparams.Merged))

	// Log key hyperparameters
	if hparams.Summary.LearningRate != nil {
		log.Infof("  Learning Rate: %v", hparams.Summary.LearningRate)
	}
	if hparams.Summary.GlobalBatchSize != nil {
		log.Infof("  Global Batch Size: %v", hparams.Summary.GlobalBatchSize)
	}
	if hparams.Summary.NumLayers != nil {
		log.Infof("  Num Layers: %v", hparams.Summary.NumLayers)
	}
	if hparams.Summary.Framework != "" {
		log.Infof("  Framework: %s", hparams.Summary.Framework)
	}
}

// isWorkloadEnded checks if the workload has ended (Done, Deleted, or Failed status)
func (e *TensorBoardStreamExecutor) isWorkloadEnded(ctx context.Context, workloadUID string) bool {
	workload, err := e.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to get workload %s status: %v", workloadUID, err)
		return false
	}

	if workload == nil {
		// Workload not found, consider it as ended
		log.Infof("Workload %s not found in database, treating as ended", workloadUID)
		return true
	}

	// Check if workload status indicates it has ended
	status := workload.Status
	if status == metadata.WorkloadStatusDone ||
		status == metadata.WorkloadStatusDeleted ||
		status == metadata.WorkloadStatusFailed {
		log.Debugf("Workload %s has status %s, treating as ended", workloadUID, status)
		return true
	}

	// Also check EndAt field
	if !workload.EndAt.IsZero() {
		log.Debugf("Workload %s has EndAt set (%v), treating as ended", workloadUID, workload.EndAt)
		return true
	}

	return false
}
