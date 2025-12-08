package task

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
	"gopkg.in/yaml.v3"
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

	// 5. 获取进程树，找到第一个 Python 进程
	processTree, err := e.getProcessTree(ctx, gpuPod, nodeExporterClient)
	var pythonProcess *types.ProcessInfo
	if err != nil {
		log.Warnf("Failed to get process tree for pod %s: %v", gpuPod.Name, err)
	} else {
		pythonProcess = e.findTopLevelPythonProcess(processTree)
		if pythonProcess != nil {
			// Remove children to avoid storing unnecessary data
			pythonProcess.Children = nil
			log.Infof("Found top-level Python process: PID=%d, cmdline=%s", pythonProcess.HostPID, pythonProcess.Cmdline)
		}
	}

	// 6. 调用 TensorBoard fd 扫描接口
	tensorboardResult, err := nodeExporterClient.FindTensorboardFiles(
		ctx,
		gpuPod.UID,
		gpuPod.Name,
		gpuPod.Namespace,
	)

	// 7. 构建返回结果
	updates := map[string]interface{}{
		"completed_at":  time.Now().Format(time.RFC3339),
		"pod_name":      gpuPod.Name,
		"pod_namespace": gpuPod.Namespace,
		"node_name":     gpuPod.NodeName,
	}

	// 保存 Python 进程信息到 metadata（不含 children）
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

	// 8. 解析 TensorBoard 结果
	filesDetected := len(tensorboardResult.Files) > 0
	updates["tensorboard_files_detected"] = filesDetected
	updates["tensorboard_result"] = tensorboardResult

	// 9. 检查框架配置是否启用了 TensorBoard（即使文件还没出现）
	tensorboardConfigured, configLogDir := e.checkTensorBoardConfiguration(ctx, detectionInfo, pythonProcess, nodeExporterClient)
	updates["tensorboard_configured"] = tensorboardConfigured

	// 决定是否启用 TensorBoard stream
	tensorboardEnabled := filesDetected || tensorboardConfigured

	if tensorboardEnabled {
		var uniqueFilePaths []string
		var logDir string

		if filesDetected {
			// 文件已存在 - 使用实际扫描到的文件
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
			// 文件未出现，但配置显示会启用 - 使用配置中的路径
			logDir = configLogDir
			updates["tensorboard_log_dir"] = logDir
			updates["tensorboard_event_files"] = []string{} // 空列表，等待文件出现
			updates["tensorboard_files_count"] = 0
			updates["tensorboard_detection_mode"] = "config_based"

			log.Infof("TensorBoard configured (not yet initialized) for workload %s: log_dir=%s",
				task.WorkloadUID, logDir)
		}

		// 10. 创建 TensorBoard 流式读取任务（无论文件是否已出现）
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
	waitForFiles bool, // 是否等待文件出现
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
			"event_files":        eventFiles,   // 精确的事件文件列表（可能为空）
			"log_dir":            logDir,       // 日志目录
			"wait_for_files":     waitForFiles, // 是否等待文件出现
			"poll_interval":      5,            // 5 秒轮询间隔
			"file_wait_timeout":  300,          // 文件等待超时（5分钟）
			"file_scan_interval": 10,           // 文件扫描间隔（10秒）

			// 任务配置
			"auto_restart": true,
			"priority":     90, // 稍低于元数据收集任务
			"max_retries":  5,
			"retry_count":  0,

			// 任务元数据
			"created_by":   "metadata_collection",
			"created_at":   time.Now().Format(time.RFC3339),
			"triggered_by": "tensorboard_detection",
			"detection_mode": map[string]interface{}{
				"files_detected": len(eventFiles) > 0,
				"config_based":   waitForFiles,
			},
		},
	}

	// 使用 Upsert 创建或更新任务
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

// checkTensorBoardConfiguration 检查框架配置是否启用了 TensorBoard
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

	// 根据不同框架检查配置
	switch framework {
	case "primus":
		return e.checkPrimusTensorBoard(ctx, pythonProcess, nodeExporterClient)
	case "megatron", "megatron-lm":
		return e.checkMegatronTensorBoard(detectionInfo.Metadata)
	case "pytorch":
		return e.checkPyTorchTensorBoard(detectionInfo.Metadata)
	default:
		// 对于未知框架，尝试从 metadata 中查找通用的 tensorboard 配置
		return e.checkGenericTensorBoard(detectionInfo.Metadata)
	}
}

// checkPrimusTensorBoard 检查 Primus 配置
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

// getProcessTree retrieves process tree from node-exporter
func (e *MetadataCollectionExecutor) getProcessTree(
	ctx context.Context,
	gpuPod *model.GpuPods,
	nodeExporterClient interface{},
) (*types.PodProcessTree, error) {
	client, ok := nodeExporterClient.(interface {
		GetPodProcessTree(ctx context.Context, req *types.ProcessTreeRequest) (*types.PodProcessTree, error)
	})
	if !ok {
		return nil, fmt.Errorf("invalid node-exporter client type")
	}

	req := &types.ProcessTreeRequest{
		PodName:        gpuPod.Name,
		PodNamespace:   gpuPod.Namespace,
		PodUID:         gpuPod.UID,
		IncludeEnv:     true,
		IncludeCmdline: true,
	}

	return client.GetPodProcessTree(ctx, req)
}

// findTopLevelPythonProcess finds the top-level Python process in the tree
func (e *MetadataCollectionExecutor) findTopLevelPythonProcess(tree *types.PodProcessTree) *types.ProcessInfo {
	if tree == nil {
		return nil
	}

	for _, container := range tree.Containers {
		if container.RootProcess != nil {
			proc := e.findPythonProcessInTree(container.RootProcess)
			if proc != nil {
				return proc
			}
		}
	}

	return nil
}

// findPythonProcessInTree recursively searches for Python process
func (e *MetadataCollectionExecutor) findPythonProcessInTree(proc *types.ProcessInfo) *types.ProcessInfo {
	if proc == nil {
		return nil
	}

	if proc.IsPython {
		return proc
	}

	for _, child := range proc.Children {
		if result := e.findPythonProcessInTree(child); result != nil {
			return result
		}
	}

	return nil
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

// checkMegatronTensorBoard 检查 Megatron 配置
func (e *MetadataCollectionExecutor) checkMegatronTensorBoard(metadata model.ExtType) (bool, string) {
	// 检查 tensorboard-dir 参数或环境变量
	if megatronConfig, ok := metadata["megatron_config"].(map[string]interface{}); ok {
		if tbDir, ok := megatronConfig["tensorboard_dir"].(string); ok && tbDir != "" {
			log.Infof("Megatron: TensorBoard enabled (tensorboard-dir=%s)", tbDir)
			return true, tbDir
		}
	}

	// 检查环境变量
	if env, ok := metadata["environment"].(map[string]interface{}); ok {
		if tbDir, ok := env["TENSORBOARD_DIR"].(string); ok && tbDir != "" {
			log.Infof("Megatron: TensorBoard enabled (TENSORBOARD_DIR=%s)", tbDir)
			return true, tbDir
		}
	}

	return false, ""
}

// checkPyTorchTensorBoard 检查 PyTorch 配置
func (e *MetadataCollectionExecutor) checkPyTorchTensorBoard(metadata model.ExtType) (bool, string) {
	// PyTorch 通常通过 SummaryWriter 使用，检查是否有相关配置
	if tbInfo, ok := metadata["tensorboard_config"].(map[string]interface{}); ok {
		if logDir, ok := tbInfo["log_dir"].(string); ok && logDir != "" {
			log.Infof("PyTorch: TensorBoard enabled (log_dir=%s)", logDir)
			return true, logDir
		}
	}

	return false, ""
}

// checkGenericTensorBoard 检查通用 TensorBoard 配置
func (e *MetadataCollectionExecutor) checkGenericTensorBoard(metadata model.ExtType) (bool, string) {
	// 查找可能的 tensorboard 相关字段
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
