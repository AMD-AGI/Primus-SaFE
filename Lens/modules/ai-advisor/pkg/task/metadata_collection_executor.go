package task

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
	"gopkg.in/yaml.v3"
)

// MetadataCollectionExecutor metadata collection task executor
type MetadataCollectionExecutor struct {
	coreTask.BaseExecutor

	collector      *metadata.Collector
	podProber      *common.PodProber
	metadataFacade database.AiWorkloadMetadataFacadeInterface
	podFacade      database.PodFacadeInterface
	taskFacade     database.WorkloadTaskFacadeInterface
}

// NewMetadataCollectionExecutor creates metadata collection executor
func NewMetadataCollectionExecutor(collector *metadata.Collector) *MetadataCollectionExecutor {
	return &MetadataCollectionExecutor{
		collector:      collector,
		podProber:      common.NewPodProber(collector),
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
		podFacade:      database.NewPodFacade(),
		taskFacade:     database.NewWorkloadTaskFacade(),
	}
}

// GetTaskType returns task type
func (e *MetadataCollectionExecutor) GetTaskType() string {
	return constant.TaskTypeMetadataCollection
}

// Validate validates task parameters
func (e *MetadataCollectionExecutor) Validate(task *model.WorkloadTaskState) error {
	// Check required parameters
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}

	return nil
}

// Execute executes metadata collection task
func (e *MetadataCollectionExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task

	log.Infof("Starting metadata collection for workload %s", task.WorkloadUID)

	// 1. Get detection info from ai_workload_metadata table
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

	// 2. Get pod info from gpu_pods table
	// workload_uid corresponds to owner_uid field in gpu_pods table
	// Prioritize pods ending with master-0
	// Using shared PodProber for consistent pod selection logic
	gpuPod, err := e.podProber.SelectTargetPod(ctx, task.WorkloadUID)
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

	// 3. Get collection config from ext field (not currently used)
	// timeout := e.GetExtInt(task, "timeout")
	// if timeout == 0 {
	// 	timeout = 30 // default 30 seconds
	// }

	log.Infof("Detecting TensorBoard for pod %s/%s (node: %s)",
		gpuPod.Namespace, gpuPod.Name, gpuPod.NodeName)

	// 4. Get node-exporter client
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

	// 5. Get process tree, find first Python process
	// Using shared PodProber for consistent process tree handling
	processTree, err := e.podProber.GetProcessTree(ctx, gpuPod, common.DefaultProcessTreeOptions())
	var pythonProcess *types.ProcessInfo
	if err != nil {
		log.Warnf("Failed to get process tree for pod %s: %v", gpuPod.Name, err)
	} else {
		pythonProcess = e.podProber.FindPythonProcess(processTree)
		if pythonProcess != nil {
			// Remove children to avoid storing unnecessary data
			pythonProcess.Children = nil
			log.Infof("Found top-level Python process: PID=%d, cmdline=%s", pythonProcess.HostPID, pythonProcess.Cmdline)
		}
	}

	// 6. Call TensorBoard fd scan interface
	tensorboardResult, err := nodeExporterClient.FindTensorboardFiles(
		ctx,
		gpuPod.UID,
		gpuPod.Name,
		gpuPod.Namespace,
	)

	// 7. Build return result
	updates := map[string]interface{}{
		"completed_at":  time.Now().Format(time.RFC3339),
		"pod_name":      gpuPod.Name,
		"pod_namespace": gpuPod.Namespace,
		"node_name":     gpuPod.NodeName,
	}

	// Save Python process info to metadata (without children)
	if pythonProcess != nil {
		updates["python_process"] = e.serializeProcessInfo(pythonProcess)
	}

	if err != nil {
		errMsg := fmt.Sprintf("tensorboard detection failed: %v", err)
		log.Warnf("TensorBoard detection failed for workload %s: %v", task.WorkloadUID, err)
		updates["error"] = errMsg
		updates["tensorboard_enabled"] = false
		return coreTask.FailureResult(errMsg, updates), err
	}

	// 8. Parse TensorBoard result
	filesDetected := len(tensorboardResult.Files) > 0
	updates["tensorboard_files_detected"] = filesDetected
	updates["tensorboard_result"] = tensorboardResult

	// 9. Collect and save framework config (for profiler collection and other downstream tasks)
	framework := strings.ToLower(detectionInfo.Framework)
	var frameworkConfig *profiler.FrameworkConfig
	// Use strings.Contains to check for framework in comma-separated list (e.g., "primus,megatron,pytorch")
	if strings.Contains(framework, "primus") || strings.Contains(framework, "megatron") || strings.Contains(framework, "deepspeed") {
		// Determine primary framework for config collection (prioritize primus > megatron > deepspeed)
		primaryFramework := "pytorch"
		if strings.Contains(framework, "primus") {
			primaryFramework = "primus"
		} else if strings.Contains(framework, "megatron") {
			primaryFramework = "megatron"
		} else if strings.Contains(framework, "deepspeed") {
			primaryFramework = "deepspeed"
		}
		frameworkConfig, err = e.collectAndSaveFrameworkConfig(ctx, task.WorkloadUID, primaryFramework, pythonProcess, nodeExporterClient)
		if err != nil {
			log.Warnf("Failed to collect framework config for workload %s: %v", task.WorkloadUID, err)
		} else {
			updates["framework_config_collected"] = true
			if frameworkConfig.ExtractedPaths != nil {
				updates["profiler_dir"] = frameworkConfig.ExtractedPaths.ProfilerDir
			}
		}
	}

	// 10. Check if framework config has TensorBoard enabled (even if files haven't appeared yet)
	tensorboardConfigured, configLogDir := e.checkTensorBoardConfiguration(ctx, detectionInfo, pythonProcess, nodeExporterClient)
	updates["tensorboard_configured"] = tensorboardConfigured

	// Decide whether to enable TensorBoard stream
	tensorboardEnabled := filesDetected || tensorboardConfigured

	if tensorboardEnabled {
		var uniqueFilePaths []string
		var logDir string

		if filesDetected {
			// Files exist - use actually scanned files
			uniqueFilePaths = extractUniqueFilePaths(tensorboardResult.Files)
			if len(uniqueFilePaths) > 0 {
				logDir = extractLogDir(uniqueFilePaths[0])
			}

			updates["tensorboard_log_dir"] = logDir
			updates["tensorboard_event_files"] = uniqueFilePaths
			updates["tensorboard_files_count"] = len(uniqueFilePaths)
			updates["tensorboard_pids"] = extractUniquePIDs(tensorboardResult.Files)

			log.Infof("TensorBoard files detected for workload %s: log_dir=%s, files=%d",
				task.WorkloadUID, logDir, len(uniqueFilePaths))
		} else {
			// Files not appeared yet, but config shows it will be enabled - use path from config
			logDir = configLogDir
			updates["tensorboard_log_dir"] = logDir
			updates["tensorboard_event_files"] = []string{} // empty list, wait for files to appear
			updates["tensorboard_files_count"] = 0
			updates["tensorboard_detection_mode"] = "config_based"

			log.Infof("TensorBoard configured (not yet initialized) for workload %s: log_dir=%s",
				task.WorkloadUID, logDir)
		}

		// 10. Create TensorBoard streaming task (regardless of whether files have appeared)
		if err := e.createTensorBoardStreamTask(ctx, task.WorkloadUID, uniqueFilePaths, logDir, !filesDetected); err != nil {
			log.Warnf("Failed to create TensorBoard stream task for workload %s: %v", task.WorkloadUID, err)
			updates["stream_task_created"] = false
			updates["stream_task_error"] = err.Error()
		} else {
			log.Infof("TensorBoard stream task created for workload %s (wait_for_files=%v)", task.WorkloadUID, !filesDetected)
			updates["stream_task_created"] = true
		}
	} else {
		log.Infof("TensorBoard not enabled for workload %s (no files detected and not configured)", task.WorkloadUID)
		updates["tensorboard_enabled"] = false
	}

	return coreTask.SuccessResult(updates), nil
}

// Cancel cancels task
func (e *MetadataCollectionExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	// Metadata collection task can be cancelled directly (through context)
	log.Infof("Metadata collection task cancelled for workload %s", task.WorkloadUID)
	return nil
}

// contains checks if a string slice contains a specific item
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// createTensorBoardStreamTask creates TensorBoard streaming task
func (e *MetadataCollectionExecutor) createTensorBoardStreamTask(
	ctx context.Context,
	workloadUID string,
	eventFiles []string,
	logDir string,
	waitForFiles bool, // whether to wait for files to appear
) error {
	// Check if TensorBoard stream task already exists
	existingTask, err := e.taskFacade.GetTask(ctx, workloadUID, constant.TaskTypeTensorBoardStream)
	if err != nil {
		log.Debugf("Failed to check existing TensorBoard stream task: %v", err)
	}

	// If task already exists and is running, don't create new task
	if existingTask != nil && (existingTask.Status == constant.TaskStatusRunning || existingTask.Status == constant.TaskStatusPending) {
		log.Infof("TensorBoard stream task already exists for workload %s (status: %s)", workloadUID, existingTask.Status)
		return nil
	}

	// Create new task
	streamTask := &model.WorkloadTaskState{
		WorkloadUID: workloadUID,
		TaskType:    constant.TaskTypeTensorBoardStream,
		Status:      constant.TaskStatusPending,
		Ext: model.ExtType{
			// TensorBoard configuration
			"event_files":        eventFiles,   // precise event file list (may be empty)
			"log_dir":            logDir,       // log directory
			"wait_for_files":     waitForFiles, // whether to wait for files to appear
			"poll_interval":      5,            // 5 second polling interval
			"file_wait_timeout":  300,          // file wait timeout (5 minutes)
			"file_scan_interval": 10,           // file scan interval (10 seconds)

			// Task configuration
			"auto_restart": true,
			"priority":     90, // slightly lower than metadata collection task
			"max_retries":  5,
			"retry_count":  0,

			// Task metadata
			"created_by":   "metadata_collection",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "tensorboard_detection",
			"detection_mode": map[string]interface{}{
				"files_detected": len(eventFiles) > 0,
				"config_based":   waitForFiles,
			},
		},
	}

	// Use Upsert to create or update task
	if err := e.taskFacade.UpsertTask(ctx, streamTask); err != nil {
		return fmt.Errorf("failed to create TensorBoard stream task: %w", err)
	}

	if waitForFiles {
		log.Infof("TensorBoard stream task created for workload %s (waiting for files in %s)", workloadUID, logDir)
	} else {
		log.Infof("TensorBoard stream task created for workload %s with %d event files", workloadUID, len(eventFiles))
	}
	return nil
}

// checkTensorBoardConfiguration checks if framework config has TensorBoard enabled
func (e *MetadataCollectionExecutor) checkTensorBoardConfiguration(
	ctx context.Context,
	detectionInfo *model.AiWorkloadMetadata,
	pythonProcess *types.ProcessInfo,
	nodeExporterClient interface{},
) (bool, string) {
	if detectionInfo == nil || detectionInfo.Metadata == nil {
		return false, ""
	}

	framework := strings.ToLower(detectionInfo.Framework)
	log.Infof("Checking TensorBoard configuration for framework: %s", framework)

	// Check config based on different frameworks
	switch framework {
	case "primus":
		return e.checkPrimusTensorBoard(ctx, pythonProcess, nodeExporterClient)
	case "megatron", "megatron-lm":
		return e.checkMegatronTensorBoard(detectionInfo.Metadata)
	case "pytorch":
		return e.checkPyTorchTensorBoard(detectionInfo.Metadata)
	default:
		// For unknown frameworks, try to find generic tensorboard config from metadata
		return e.checkGenericTensorBoard(detectionInfo.Metadata)
	}
}

// checkPrimusTensorBoard checks Primus configuration
func (e *MetadataCollectionExecutor) checkPrimusTensorBoard(
	ctx context.Context,
	pythonProcess *types.ProcessInfo,
	nodeExporterClient interface{},
) (bool, string) {
	// Check if Python process is available
	if pythonProcess == nil {
		log.Warnf("No Python process available for TensorBoard config check")
		return false, ""
	}

	log.Infof("Checking Primus TensorBoard config from process: PID=%d", pythonProcess.HostPID)

	// Extract config file path from env or cmdline
	configPath := e.extractConfigPath(pythonProcess)
	if configPath == "" {
		log.Debugf("No config path found in process env or cmdline")
		return false, ""
	}

	// Convert relative path to absolute using cwd
	if !filepath.IsAbs(configPath) && pythonProcess.Cwd != "" {
		configPath = filepath.Join(pythonProcess.Cwd, configPath)
	}

	log.Infof("Reading config file from container: %s", configPath)

	// Read config file from container
	configContent, err := e.readContainerFile(ctx, pythonProcess.HostPID, configPath, nodeExporterClient)
	if err != nil {
		log.Warnf("Failed to read config file %s: %v", configPath, err)
		return false, ""
	}

	// Parse YAML and check tensorboard configuration
	enabled := e.parsePrimusConfig(configContent)
	if enabled {
		log.Infof("Primus: TensorBoard enabled from config file")
		return true, ""
	}

	return false, ""
}

// collectAndSaveFrameworkConfig collects framework config and saves to metadata
func (e *MetadataCollectionExecutor) collectAndSaveFrameworkConfig(
	ctx context.Context,
	workloadUID string,
	framework string,
	pythonProcess *types.ProcessInfo,
	nodeExporterClient interface{},
) (*profiler.FrameworkConfig, error) {
	if pythonProcess == nil {
		return nil, fmt.Errorf("no Python process available")
	}

	// Extract config file path
	configPath := e.extractConfigPath(pythonProcess)
	if configPath == "" {
		return nil, fmt.Errorf("no config path found")
	}

	// Convert relative path to absolute
	if !filepath.IsAbs(configPath) && pythonProcess.Cwd != "" {
		configPath = filepath.Join(pythonProcess.Cwd, configPath)
	}

	// Read config file
	configContent, err := e.readContainerFile(ctx, pythonProcess.HostPID, configPath, nodeExporterClient)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse config to map
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal([]byte(configContent), &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Extract environment variables from process using shared PodProber
	env := e.podProber.ExtractEnvMap(pythonProcess)

	// Build framework config using the service
	configService := profiler.NewFrameworkConfigService()
	frameworkConfig := configService.BuildFrameworkConfig(ctx, framework, rawConfig, configPath, env)

	// Save to metadata (including working directory for profiler collection)
	workingDir := ""
	if pythonProcess != nil && pythonProcess.Cwd != "" {
		workingDir = pythonProcess.Cwd
	}
	if err := e.saveFrameworkConfig(ctx, workloadUID, frameworkConfig, workingDir); err != nil {
		log.Warnf("Failed to save framework config for workload %s: %v", workloadUID, err)
		// Don't fail the entire operation, just log the error
	} else {
		log.Infof("Framework config saved for workload %s: profiler_dir=%s, working_dir=%s",
			workloadUID, frameworkConfig.ExtractedPaths.ProfilerDir, workingDir)
	}

	return frameworkConfig, nil
}

// saveFrameworkConfig saves framework config to ai_workload_metadata.metadata.framework_config
// Also saves working_dir for profiler collection to resolve relative paths
func (e *MetadataCollectionExecutor) saveFrameworkConfig(
	ctx context.Context,
	workloadUID string,
	config *profiler.FrameworkConfig,
	workingDir string,
) error {
	// Get current metadata
	currentMetadata, err := e.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		return fmt.Errorf("failed to get current metadata: %w", err)
	}

	if currentMetadata == nil {
		return fmt.Errorf("metadata not found for workload %s", workloadUID)
	}

	// Ensure metadata map exists
	if currentMetadata.Metadata == nil {
		currentMetadata.Metadata = make(model.ExtType)
	}

	// Convert FrameworkConfig to map for storage
	frameworkConfigMap := map[string]interface{}{
		"framework":    config.Framework,
		"version":      config.Version,
		"collected_at": config.CollectedAt.Format(time.RFC3339),
	}

	if config.Source != nil {
		frameworkConfigMap["source"] = map[string]interface{}{
			"type": config.Source.Type,
			"path": config.Source.Path,
		}
	}

	if config.ExtractedPaths != nil {
		extractedPaths := map[string]interface{}{
			"profiler_dir":    config.ExtractedPaths.ProfilerDir,
			"tensorboard_dir": config.ExtractedPaths.TensorBoardDir,
			"checkpoint_dir":  config.ExtractedPaths.CheckpointDir,
			"log_dir":         config.ExtractedPaths.LogDir,
			"workspace_dir":   config.ExtractedPaths.WorkspaceDir,
		}
		if len(config.ExtractedPaths.CustomPaths) > 0 {
			extractedPaths["custom_paths"] = config.ExtractedPaths.CustomPaths
		}
		frameworkConfigMap["extracted_paths"] = extractedPaths
	}

	// Optionally store raw config (can be large, consider limiting)
	// frameworkConfigMap["raw_config"] = config.RawConfig

	// Update metadata
	currentMetadata.Metadata["framework_config"] = frameworkConfigMap

	// Also save working_dir for profiler collection to resolve relative paths
	if workingDir != "" {
		currentMetadata.Metadata["working_dir"] = workingDir
	}

	// Save back to database
	if err := e.metadataFacade.UpdateAiWorkloadMetadata(ctx, currentMetadata); err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// serializeProcessInfo serializes process info without children
func (e *MetadataCollectionExecutor) serializeProcessInfo(proc *types.ProcessInfo) map[string]interface{} {
	if proc == nil {
		return nil
	}

	return map[string]interface{}{
		"host_pid":       proc.HostPID,
		"host_ppid":      proc.HostPPID,
		"container_pid":  proc.ContainerPID,
		"container_ppid": proc.ContainerPPID,
		"cmdline":        proc.Cmdline,
		"comm":           proc.Comm,
		"exe":            proc.Exe,
		"args":           proc.Args,
		"env":            proc.Env,
		"cwd":            proc.Cwd,
		"state":          proc.State,
		"threads":        proc.Threads,
		"cpu_time":       proc.CPUTime,
		"memory_rss":     proc.MemoryRSS,
		"memory_virtual": proc.MemoryVirtual,
		"container_id":   proc.ContainerID,
		"container_name": proc.ContainerName,
		"pod_uid":        proc.PodUID,
		"pod_name":       proc.PodName,
		"pod_namespace":  proc.PodNamespace,
		"is_python":      proc.IsPython,
		"is_java":        proc.IsJava,
		"start_time":     proc.StartTime,
	}
}

// extractConfigPath extracts config file path from env or cmdline
func (e *MetadataCollectionExecutor) extractConfigPath(proc *types.ProcessInfo) string {
	// First, check env for EXP variable
	for _, envVar := range proc.Env {
		if strings.HasPrefix(envVar, "EXP=") {
			return strings.TrimPrefix(envVar, "EXP=")
		}
	}

	// Second, check cmdline for --config flag
	configRegex := regexp.MustCompile(`--config\s+(\S+)`)
	if matches := configRegex.FindStringSubmatch(proc.Cmdline); len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// readContainerFile reads a file from container filesystem
func (e *MetadataCollectionExecutor) readContainerFile(
	ctx context.Context,
	pid int,
	path string,
	nodeExporterClient interface{},
) (string, error) {
	client, ok := nodeExporterClient.(interface {
		ReadContainerFile(ctx context.Context, req *types.ContainerFileReadRequest) (*types.ContainerFileReadResponse, error)
	})
	if !ok {
		return "", fmt.Errorf("invalid node-exporter client type")
	}

	req := &types.ContainerFileReadRequest{
		PID:            pid,
		Path:           path,
		FollowSymlinks: true,
	}

	resp, err := client.ReadContainerFile(ctx, req)
	if err != nil {
		return "", err
	}

	return resp.Content, nil
}

// parsePrimusConfig parses Primus YAML config and checks tensorboard setting
func (e *MetadataCollectionExecutor) parsePrimusConfig(content string) bool {
	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		log.Warnf("Failed to parse YAML config: %v", err)
		return false
	}

	// Navigate: modules.pre_trainer.overrides.disable_tensorboard
	modules, ok := config["modules"].(map[string]interface{})
	if !ok {
		log.Debugf("No 'modules' key in config")
		return false
	}

	preTrainer, ok := modules["pre_trainer"].(map[string]interface{})
	if !ok {
		log.Debugf("No 'modules.pre_trainer' key in config")
		return false
	}

	overrides, ok := preTrainer["overrides"].(map[string]interface{})
	if !ok {
		log.Debugf("No 'modules.pre_trainer.overrides' key in config")
		return false
	}

	disableTB, ok := overrides["disable_tensorboard"].(bool)
	if !ok {
		// If key doesn't exist, assume tensorboard is enabled
		log.Debugf("'disable_tensorboard' not found, assuming enabled")
		return true
	}

	if !disableTB {
		// tensorboard is enabled
		log.Debugf("disable_tensorboard=false, tensorboard enabled")
		return true
	}

	log.Debugf("disable_tensorboard=true, tensorboard disabled")
	return false
}

// checkMegatronTensorBoard checks Megatron configuration
func (e *MetadataCollectionExecutor) checkMegatronTensorBoard(metadata model.ExtType) (bool, string) {
	// Check tensorboard-dir parameter or environment variable
	if megatronConfig, ok := metadata["megatron_config"].(map[string]interface{}); ok {
		if tbDir, ok := megatronConfig["tensorboard_dir"].(string); ok && tbDir != "" {
			log.Infof("Megatron: TensorBoard enabled (tensorboard-dir=%s)", tbDir)
			return true, tbDir
		}
	}

	// Check environment variable
	if env, ok := metadata["environment"].(map[string]interface{}); ok {
		if tbDir, ok := env["TENSORBOARD_DIR"].(string); ok && tbDir != "" {
			log.Infof("Megatron: TensorBoard enabled (TENSORBOARD_DIR=%s)", tbDir)
			return true, tbDir
		}
	}

	return false, ""
}

// checkPyTorchTensorBoard checks PyTorch configuration
func (e *MetadataCollectionExecutor) checkPyTorchTensorBoard(metadata model.ExtType) (bool, string) {
	// PyTorch typically uses SummaryWriter, check if there's related config
	if tbInfo, ok := metadata["tensorboard_config"].(map[string]interface{}); ok {
		if logDir, ok := tbInfo["log_dir"].(string); ok && logDir != "" {
			log.Infof("PyTorch: TensorBoard enabled (log_dir=%s)", logDir)
			return true, logDir
		}
	}

	return false, ""
}

// checkGenericTensorBoard checks generic TensorBoard configuration
func (e *MetadataCollectionExecutor) checkGenericTensorBoard(metadata model.ExtType) (bool, string) {
	// Look for possible tensorboard related fields
	if tbConfig, ok := metadata["tensorboard"].(map[string]interface{}); ok {
		if enabled, ok := tbConfig["enabled"].(bool); ok && enabled {
			logDir, _ := tbConfig["log_dir"].(string)
			log.Infof("Generic: TensorBoard enabled (log_dir=%s)", logDir)
			return true, logDir
		}
		if logDir, ok := tbConfig["log_dir"].(string); ok && logDir != "" {
			log.Infof("Generic: TensorBoard enabled (log_dir=%s)", logDir)
			return true, logDir
		}
	}

	return false, ""
}

// extractLogDir extracts directory from TensorBoard event file path
func extractLogDir(filePath string) string {
	// Find position of last '/'
	lastSlash := strings.LastIndex(filePath, "/")
	if lastSlash > 0 {
		return filePath[:lastSlash]
	}
	return filePath
}

// extractUniqueFilePaths extracts unique file paths from TensorBoard file list (deduplication)
func extractUniqueFilePaths(files []*types.TensorboardFileInfo) []string {
	filePathMap := make(map[string]bool)
	var uniquePaths []string

	for _, file := range files {
		// Use file path as key for deduplication
		if !filePathMap[file.FilePath] {
			filePathMap[file.FilePath] = true
			uniquePaths = append(uniquePaths, file.FilePath)
		}
	}

	return uniquePaths
}

// extractUniquePIDs extracts unique PID list from TensorBoard file list
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
