package task

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// ActiveDetectionExecutor proactively detects framework for workloads
//
// Deprecated: This executor is being replaced by DetectionCoordinator which provides
// a more modular architecture with separate sub-task executors (ProcessProbeExecutor,
// ImageProbeExecutor, LabelProbeExecutor, LogDetectionExecutor) and a state machine
// for coordinating evidence collection. Use DetectionCoordinator for new workloads.
// This executor is kept for backward compatibility with existing active_detection tasks.
type ActiveDetectionExecutor struct {
	coreTask.BaseExecutor

	collector         *metadata.Collector
	aggregator        *detection.EvidenceAggregator
	evidenceStore     *detection.EvidenceStore
	layerResolver     *detection.FrameworkLayerResolver
	podFacade         database.PodFacadeInterface
	workloadFacade    database.WorkloadFacadeInterface
	taskFacade        database.WorkloadTaskFacadeInterface
	detectionFacade   database.WorkloadDetectionFacadeInterface
}

// NewActiveDetectionExecutor creates new executor
//
// Deprecated: Use NewDetectionCoordinator instead. See ActiveDetectionExecutor for details.
func NewActiveDetectionExecutor(collector *metadata.Collector) *ActiveDetectionExecutor {
	return &ActiveDetectionExecutor{
		collector:         collector,
		aggregator:        detection.NewEvidenceAggregator(),
		evidenceStore:     detection.NewEvidenceStore(),
		layerResolver:     detection.GetLayerResolver(),
		podFacade:         database.NewPodFacade(),
		workloadFacade:    database.GetFacade().GetWorkload(),
		taskFacade:        database.NewWorkloadTaskFacade(),
		detectionFacade:   database.NewWorkloadDetectionFacade(),
	}
}

// NewActiveDetectionExecutorWithDeps creates executor with custom dependencies
func NewActiveDetectionExecutorWithDeps(
	collector *metadata.Collector,
	aggregator *detection.EvidenceAggregator,
	evidenceStore *detection.EvidenceStore,
) *ActiveDetectionExecutor {
	return &ActiveDetectionExecutor{
		collector:         collector,
		aggregator:        aggregator,
		evidenceStore:     evidenceStore,
		layerResolver:     detection.GetLayerResolver(),
		podFacade:         database.NewPodFacade(),
		workloadFacade:    database.GetFacade().GetWorkload(),
		taskFacade:        database.NewWorkloadTaskFacade(),
		detectionFacade:   database.NewWorkloadDetectionFacade(),
	}
}

// GetTaskType returns task type
func (e *ActiveDetectionExecutor) GetTaskType() string {
	return constant.TaskTypeActiveDetection
}

// Validate validates task parameters
func (e *ActiveDetectionExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute performs active framework detection with evidence aggregation
func (e *ActiveDetectionExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID

	log.Infof("Starting active detection for workload %s", workloadUID)

	// Get attempt count (no max limit - runs until confirmed or workload terminates)
	attemptCount := e.GetExtInt(task, "attempt_count")
	attemptCount++

	updates := map[string]interface{}{
		"attempt_count": attemptCount,
		"last_attempt":  time.Now().Format(time.RFC3339),
	}

	// Update detection state to in_progress
	if err := e.ensureDetectionRecord(ctx, workloadUID); err != nil {
		log.Warnf("Failed to ensure detection record: %v", err)
	}

	// Step 1: Aggregate ALL existing evidence (from passive + previous active attempts)
	aggregationResult, err := e.aggregator.AggregateEvidence(ctx, workloadUID)
	if err != nil {
		log.Warnf("Evidence aggregation failed: %v", err)
	}

	// Step 2: Check if aggregated result meets threshold
	if aggregationResult != nil && aggregationResult.IsConfirmed() {
		log.Infof("Detection confirmed via evidence aggregation: framework=%s, confidence=%.2f, sources=%v",
			aggregationResult.Framework, aggregationResult.Confidence, aggregationResult.Sources)

		updates["result"] = "confirmed_via_aggregation"
		updates["framework"] = aggregationResult.Framework
		updates["confidence"] = aggregationResult.Confidence
		updates["status"] = string(aggregationResult.Status)
		updates["evidence_count"] = aggregationResult.EvidenceCount

		return coreTask.SuccessResult(updates), nil
	}

	// Record pre-probe aggregation state
	if aggregationResult != nil {
		updates["pre_probe_aggregation"] = map[string]interface{}{
			"evidence_count": aggregationResult.EvidenceCount,
			"sources":        aggregationResult.Sources,
			"confidence":     aggregationResult.Confidence,
			"status":         string(aggregationResult.Status),
		}
	}

	// Step 3: Perform active probing to collect new evidence
	pod, err := e.selectTargetPod(ctx, workloadUID)
	if err != nil || pod == nil {
		log.Warnf("Failed to get pod for workload %s: %v", workloadUID, err)
		updates["probe_error"] = fmt.Sprintf("failed to get pod: %v", err)
		return e.handleRetryOrComplete(ctx, task, updates, attemptCount, 0)
	}

	updates["pod_name"] = pod.Name
	updates["pod_namespace"] = pod.Namespace
	updates["node_name"] = pod.NodeName

	// Step 4: Collect evidence from multiple sources
	evidence := e.collectEvidence(ctx, task, pod)
	updates["evidence_sources_probed"] = evidence.Sources

	// Step 5: Store collected evidence
	if err := e.storeCollectedEvidence(ctx, workloadUID, evidence); err != nil {
		log.Warnf("Failed to store evidence: %v", err)
	}

	// Step 6: Re-aggregate with new evidence
	aggregationResult, err = e.aggregator.AggregateEvidence(ctx, workloadUID)
	if err != nil {
		log.Warnf("Re-aggregation failed: %v", err)
	}

	if aggregationResult != nil {
		updates["post_probe_aggregation"] = map[string]interface{}{
			"evidence_count": aggregationResult.EvidenceCount,
			"sources":        aggregationResult.Sources,
			"confidence":     aggregationResult.Confidence,
			"status":         string(aggregationResult.Status),
		}

		if aggregationResult.IsConfirmed() {
			log.Infof("Detection confirmed after probing: framework=%s, confidence=%.2f",
				aggregationResult.Framework, aggregationResult.Confidence)

			updates["result"] = "confirmed_after_probe"
			updates["framework"] = aggregationResult.Framework
			updates["confidence"] = aggregationResult.Confidence
			updates["status"] = string(aggregationResult.Status)

			return coreTask.SuccessResult(updates), nil
		}
	}

	// Step 7: Not confirmed yet, schedule retry or complete
	currentConfidence := float64(0)
	if aggregationResult != nil {
		currentConfidence = aggregationResult.Confidence
	}
	log.Infof("Framework not confirmed for workload %s (attempt %d, confidence=%.2f)",
		workloadUID, attemptCount, currentConfidence)

	return e.handleRetryOrComplete(ctx, task, updates, attemptCount, 0)
}

// EvidenceCollection holds collected evidence
type EvidenceCollection struct {
	Sources     []string
	ProcessInfo *ProcessEvidence
	ImageInfo   *ImageEvidence
	EnvInfo     *EnvEvidence
	LabelInfo   *LabelEvidence
}

// ProcessEvidence evidence from process probing
type ProcessEvidence struct {
	Cmdlines     []string
	ProcessNames []string
	Cwd          string
}

// ImageEvidence evidence from container image
type ImageEvidence struct {
	ImageName string
	ImageTag  string
}

// EnvEvidence evidence from environment variables
type EnvEvidence struct {
	EnvVars map[string]string
}

// LabelEvidence evidence from pod labels/annotations
type LabelEvidence struct {
	Labels      map[string]string
	Annotations map[string]string
}

// collectEvidence collects evidence from configured sources
func (e *ActiveDetectionExecutor) collectEvidence(
	ctx context.Context,
	task *model.WorkloadTaskState,
	pod *model.GpuPods,
) *EvidenceCollection {
	evidence := &EvidenceCollection{
		Sources: []string{},
	}

	// 1. Probe process info
	if e.GetExtBool(task, "probe_process") {
		if procEvidence := e.probeProcessInfo(ctx, pod); procEvidence != nil {
			evidence.ProcessInfo = procEvidence
			evidence.Sources = append(evidence.Sources, "process")
		}
	}

	// 2. Check container image
	if e.GetExtBool(task, "probe_image") {
		if imageEvidence := e.probeImageInfo(pod); imageEvidence != nil {
			evidence.ImageInfo = imageEvidence
			evidence.Sources = append(evidence.Sources, "image")
		}
	}

	// 3. Probe environment variables
	if e.GetExtBool(task, "probe_env") {
		if envEvidence := e.probeEnvInfo(ctx, pod); envEvidence != nil {
			evidence.EnvInfo = envEvidence
			evidence.Sources = append(evidence.Sources, "env")
		}
	}

	// 4. Check pod labels (if we have K8s API access)
	if e.GetExtBool(task, "probe_labels") {
		if labelEvidence := e.probeLabelInfo(pod); labelEvidence != nil {
			evidence.LabelInfo = labelEvidence
			evidence.Sources = append(evidence.Sources, "labels")
		}
	}

	return evidence
}

// probeProcessInfo gets process info from node-exporter
func (e *ActiveDetectionExecutor) probeProcessInfo(
	ctx context.Context,
	pod *model.GpuPods,
) *ProcessEvidence {
	if e.collector == nil {
		return nil
	}

	client, err := e.collector.GetNodeExporterClientForPod(ctx, pod.NodeName)
	if err != nil {
		log.Debugf("Failed to get node-exporter client: %v", err)
		return nil
	}

	processTree, err := client.GetPodProcessTree(ctx, &types.ProcessTreeRequest{
		PodName:        pod.Name,
		PodNamespace:   pod.Namespace,
		PodUID:         pod.UID,
		IncludeCmdline: true,
		IncludeEnv:     false,
	})

	if err != nil {
		log.Debugf("Failed to get process tree: %v", err)
		return nil
	}

	evidence := &ProcessEvidence{
		Cmdlines:     []string{},
		ProcessNames: []string{},
	}

	// Extract cmdlines from Python processes
	e.extractProcessInfo(processTree, evidence)

	return evidence
}

// extractProcessInfo extracts process info from process tree
func (e *ActiveDetectionExecutor) extractProcessInfo(tree *types.PodProcessTree, evidence *ProcessEvidence) {
	if tree == nil {
		return
	}

	for _, container := range tree.Containers {
		if container.RootProcess != nil {
			e.extractFromProcess(container.RootProcess, evidence)
		}
	}
}

// extractFromProcess recursively extracts info from process
func (e *ActiveDetectionExecutor) extractFromProcess(proc *types.ProcessInfo, evidence *ProcessEvidence) {
	if proc == nil {
		return
	}

	if proc.IsPython {
		evidence.Cmdlines = append(evidence.Cmdlines, proc.Cmdline)
		evidence.ProcessNames = append(evidence.ProcessNames, proc.Comm)
		if evidence.Cwd == "" && proc.Cwd != "" {
			evidence.Cwd = proc.Cwd
		}
	}

	for _, child := range proc.Children {
		e.extractFromProcess(child, evidence)
	}
}

// probeImageInfo extracts image information from workload metadata
func (e *ActiveDetectionExecutor) probeImageInfo(pod *model.GpuPods) *ImageEvidence {
	// Note: GpuPods doesn't have Image field
	// We need to get it from workload metadata or container info
	// For now, we'll try to get it from ai_workload_metadata table
	if pod == nil {
		return nil
	}

	ctx := context.Background()
	metadataFacade := database.NewAiWorkloadMetadataFacade()
	
	metadata, err := metadataFacade.GetAiWorkloadMetadata(ctx, pod.OwnerUID)
	if err != nil || metadata == nil {
		return nil
	}

	// Try to extract image from workload_signature in metadata
	if metadata.Metadata != nil {
		if signatureData, ok := metadata.Metadata["workload_signature"].(map[string]interface{}); ok {
			if imageName, ok := signatureData["image"].(string); ok && imageName != "" {
				evidence := &ImageEvidence{
					ImageName: imageName,
				}
				// Extract tag
				parts := strings.Split(imageName, ":")
				if len(parts) > 1 {
					evidence.ImageTag = parts[len(parts)-1]
				}
				return evidence
			}
		}
	}

	return nil
}

// probeEnvInfo gets environment variables from process
func (e *ActiveDetectionExecutor) probeEnvInfo(
	ctx context.Context,
	pod *model.GpuPods,
) *EnvEvidence {
	if e.collector == nil {
		return nil
	}

	client, err := e.collector.GetNodeExporterClientForPod(ctx, pod.NodeName)
	if err != nil {
		return nil
	}

	processTree, err := client.GetPodProcessTree(ctx, &types.ProcessTreeRequest{
		PodName:      pod.Name,
		PodNamespace: pod.Namespace,
		PodUID:       pod.UID,
		IncludeEnv:   true,
	})

	if err != nil {
		return nil
	}

	evidence := &EnvEvidence{
		EnvVars: make(map[string]string),
	}

	// Extract environment variables from first Python process
	if proc := e.findFirstPythonProcess(processTree); proc != nil {
		for _, env := range proc.Env {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				evidence.EnvVars[parts[0]] = parts[1]
			}
		}
	}

	return evidence
}

// findFirstPythonProcess finds the first Python process in tree
func (e *ActiveDetectionExecutor) findFirstPythonProcess(tree *types.PodProcessTree) *types.ProcessInfo {
	if tree == nil {
		return nil
	}

	for _, container := range tree.Containers {
		if container.RootProcess != nil {
			if proc := e.findPythonInTree(container.RootProcess); proc != nil {
				return proc
			}
		}
	}
	return nil
}

// findPythonInTree recursively finds Python process
func (e *ActiveDetectionExecutor) findPythonInTree(proc *types.ProcessInfo) *types.ProcessInfo {
	if proc == nil {
		return nil
	}
	if proc.IsPython {
		return proc
	}
	for _, child := range proc.Children {
		if found := e.findPythonInTree(child); found != nil {
			return found
		}
	}
	return nil
}

// probeLabelInfo gets pod labels (currently limited implementation)
func (e *ActiveDetectionExecutor) probeLabelInfo(pod *model.GpuPods) *LabelEvidence {
	// Note: In full implementation, this would query K8s API for labels/annotations
	// For now, return nil as we don't have direct K8s API access here
	return nil
}

// storeCollectedEvidence stores evidence from active probing
func (e *ActiveDetectionExecutor) storeCollectedEvidence(
	ctx context.Context,
	workloadUID string,
	evidence *EvidenceCollection,
) error {
	var requests []*detection.StoreEvidenceRequest

	// Store process evidence
	if evidence.ProcessInfo != nil {
		for _, cmdline := range evidence.ProcessInfo.Cmdlines {
			if fw := e.detectTrainingFrameworkFromCmdline(cmdline); fw != "" {
				// Resolve layer from config
				layer := e.resolveFrameworkLayer(fw)
				req := &detection.StoreEvidenceRequest{
					WorkloadUID:    workloadUID,
					Source:         "process",
					SourceType:     "active",
					Framework:      fw,
					WorkloadType:   "training",
					Confidence:     0.7,
					FrameworkLayer: layer,
					Evidence: map[string]interface{}{
						"cmdline": cmdline,
						"method":  "cmdline_pattern",
					},
				}
				// Set layer-specific fields
				if layer == detection.FrameworkLayerWrapper {
					req.WrapperFramework = fw
				} else {
					req.BaseFramework = fw
				}
				requests = append(requests, req)
			}
		}
	}

	// Store env evidence
	if evidence.EnvInfo != nil {
		if fw := e.detectFrameworkFromEnv(evidence.EnvInfo.EnvVars); fw != nil {
			// Resolve layer from config
			layer := e.resolveFrameworkLayer(fw.Framework)
			requests = append(requests, &detection.StoreEvidenceRequest{
				WorkloadUID:      workloadUID,
				Source:           "env",
				SourceType:       "active",
				Framework:        fw.Framework,
				WorkloadType:     "training",
				Confidence:       fw.Confidence,
				FrameworkLayer:   layer,
				WrapperFramework: fw.WrapperFramework,
				BaseFramework:    fw.BaseFramework,
				Evidence: map[string]interface{}{
					"matched_vars": e.getMatchedEnvVars(evidence.EnvInfo.EnvVars),
					"method":       "env_pattern",
				},
			})
		}
	}

	// Store image evidence
	if evidence.ImageInfo != nil {
		if fw, wlType := e.detectFrameworkFromImage(evidence.ImageInfo.ImageName); fw != "" {
			// Resolve layer from config
			layer := e.resolveFrameworkLayer(fw)
			req := &detection.StoreEvidenceRequest{
				WorkloadUID:    workloadUID,
				Source:         "image",
				SourceType:     "active",
				Framework:      fw,
				WorkloadType:   wlType,
				Confidence:     0.6,
				FrameworkLayer: layer,
				Evidence: map[string]interface{}{
					"image_name": evidence.ImageInfo.ImageName,
					"method":     "image_pattern",
				},
			}
			// Set layer-specific fields
			if layer == detection.FrameworkLayerWrapper {
				req.WrapperFramework = fw
			} else {
				req.BaseFramework = fw
			}
			requests = append(requests, req)
		}
	}

	// Batch store evidence
	if len(requests) > 0 {
		return e.evidenceStore.BatchStoreEvidence(ctx, requests)
	}

	return nil
}

// resolveFrameworkLayer resolves the layer for a framework using config
func (e *ActiveDetectionExecutor) resolveFrameworkLayer(framework string) string {
	if e.layerResolver != nil {
		return e.layerResolver.GetLayer(framework)
	}
	// Fallback to runtime as default
	return detection.FrameworkLayerRuntime
}

// FrameworkFromEnv result from env detection
type FrameworkFromEnv struct {
	Framework        string
	Confidence       float64
	WrapperFramework string
	BaseFramework    string
}

// detectTrainingFrameworkFromCmdline detects framework from command line
func (e *ActiveDetectionExecutor) detectTrainingFrameworkFromCmdline(cmdline string) string {
	cmdlineLower := strings.ToLower(cmdline)

	patterns := map[string][]string{
		"primus":    {"primus", "primus-train", "primus.train"},
		"megatron":  {"megatron", "pretrain_gpt", "megatron-lm"},
		"deepspeed": {"deepspeed", "ds_config", "deepspeed_config"},
		"pytorch":   {"torch.distributed", "torchrun"},
	}

	for fw, keywords := range patterns {
		for _, kw := range keywords {
			if strings.Contains(cmdlineLower, kw) {
				return fw
			}
		}
	}
	return ""
}

// detectFrameworkFromEnv detects framework from environment variables
func (e *ActiveDetectionExecutor) detectFrameworkFromEnv(envVars map[string]string) *FrameworkFromEnv {
	// Check for specific framework environment variables
	if _, ok := envVars["PRIMUS_CONFIG"]; ok {
		result := &FrameworkFromEnv{Framework: "primus", Confidence: 0.8, WrapperFramework: "primus"}
		if backend := envVars["PRIMUS_BACKEND"]; backend != "" {
			result.BaseFramework = strings.ToLower(backend)
		}
		return result
	}
	if _, ok := envVars["PRIMUS_VERSION"]; ok {
		return &FrameworkFromEnv{Framework: "primus", Confidence: 0.8, WrapperFramework: "primus"}
	}
	if _, ok := envVars["DEEPSPEED_CONFIG"]; ok {
		return &FrameworkFromEnv{Framework: "deepspeed", Confidence: 0.8, BaseFramework: "deepspeed"}
	}
	if _, ok := envVars["DS_CONFIG"]; ok {
		return &FrameworkFromEnv{Framework: "deepspeed", Confidence: 0.8, BaseFramework: "deepspeed"}
	}
	if _, ok := envVars["MEGATRON_CONFIG"]; ok {
		return &FrameworkFromEnv{Framework: "megatron", Confidence: 0.8, BaseFramework: "megatron"}
	}
	return nil
}

// getMatchedEnvVars returns matched environment variables
func (e *ActiveDetectionExecutor) getMatchedEnvVars(envVars map[string]string) map[string]string {
	matched := make(map[string]string)
	frameworkVars := []string{
		"PRIMUS_CONFIG", "PRIMUS_VERSION", "PRIMUS_BACKEND",
		"DEEPSPEED_CONFIG", "DS_CONFIG",
		"MEGATRON_CONFIG", "MEGATRON_LM_PATH",
		"FRAMEWORK",
	}

	for _, v := range frameworkVars {
		if val, ok := envVars[v]; ok {
			matched[v] = val
		}
	}
	return matched
}

// detectFrameworkFromImage detects framework from container image name
func (e *ActiveDetectionExecutor) detectFrameworkFromImage(imageName string) (string, string) {
	imageLower := strings.ToLower(imageName)

	// Inference frameworks
	inferencePatterns := map[string][]string{
		"vllm":   {"vllm"},
		"triton": {"triton", "tritonserver"},
		"tgi":    {"text-generation-inference", "tgi"},
		"sglang": {"sglang"},
	}

	for fw, keywords := range inferencePatterns {
		for _, kw := range keywords {
			if strings.Contains(imageLower, kw) {
				return fw, "inference"
			}
		}
	}

	// Training frameworks
	trainingPatterns := map[string][]string{
		"primus":    {"primus"},
		"megatron":  {"megatron"},
		"deepspeed": {"deepspeed"},
	}

	for fw, keywords := range trainingPatterns {
		for _, kw := range keywords {
			if strings.Contains(imageLower, kw) {
				return fw, "training"
			}
		}
	}

	return "", ""
}

// handleRetryOrComplete decides whether to retry or complete the task
// Note: This task runs continuously until detection is confirmed or workload terminates
func (e *ActiveDetectionExecutor) handleRetryOrComplete(
	ctx context.Context,
	task *model.WorkloadTaskState,
	updates map[string]interface{},
	attemptCount int,
	_ int, // unused, kept for compatibility
) (*coreTask.ExecutionResult, error) {
	// Check if workload is still running before scheduling retry
	workloadRunning := e.isWorkloadRunning(ctx, task.WorkloadUID)
	if !workloadRunning {
		log.Infof("Workload %s is no longer running, completing active detection task", task.WorkloadUID)
		updates["final_result"] = "workload_terminated"
		updates["detection_completed"] = true

		if err := e.detectionFacade.UpdateDetectionState(ctx, task.WorkloadUID, "completed"); err != nil {
			log.Warnf("Failed to update detection state: %v", err)
		}

		return coreTask.SuccessResult(updates), nil
	}

	// No max attempts limit - continue retrying as long as workload is running
	// Only stop when detection is confirmed or workload terminates

	// Calculate next retry time with exponential backoff
	baseInterval := e.GetExtInt(task, "retry_interval")
	if baseInterval == 0 {
		baseInterval = 10 // Start with 10 seconds
	}

	// Exponential backoff: baseInterval * 2^(attemptCount-1), capped at 60 seconds (1 minute)
	backoff := float64(baseInterval) * math.Pow(2, float64(attemptCount-1))
	if backoff > 60 {
		backoff = 60 // Cap at 1 minute
	}

	nextAttemptAt := time.Now().Add(time.Duration(backoff) * time.Second)
	updates["next_attempt_at"] = nextAttemptAt.Format(time.RFC3339)
	updates["retry_backoff_seconds"] = int(backoff)

	// Update task ext with new attempt count and next attempt time
	if err := e.taskFacade.UpdateTaskExt(ctx, task.WorkloadUID, task.TaskType, updates); err != nil {
		log.Warnf("Failed to update task ext: %v", err)
	}

	// Set next attempt time in detection table
	if err := e.detectionFacade.SetNextAttemptAt(ctx, task.WorkloadUID, nextAttemptAt); err != nil {
		log.Warnf("Failed to set next attempt time: %v", err)
	}

	// Return progress result - task will be re-picked by scheduler after retry interval
	return coreTask.ProgressResult(updates), nil
}

// isWorkloadRunning checks if the workload is still running
func (e *ActiveDetectionExecutor) isWorkloadRunning(ctx context.Context, workloadUID string) bool {
	// Check if there are any running pods for this workload
	podRefs, err := e.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil || len(podRefs) == 0 {
		return false
	}

	// Check pod status
	db := database.GetFacade().GetSystemConfig().GetDB()
	var runningCount int64
	err = db.WithContext(ctx).
		Table("gpu_pods").
		Where("owner_uid = ? AND deleted = ? AND running = ?", workloadUID, false, true).
		Count(&runningCount).Error

	if err != nil {
		log.Warnf("Failed to check workload running status: %v", err)
		return true // Assume running on error to avoid premature termination
	}

	return runningCount > 0
}

// selectTargetPod selects target pod for a workload
func (e *ActiveDetectionExecutor) selectTargetPod(ctx context.Context, workloadUID string) (*model.GpuPods, error) {
	// Method 1: Find pod through workload_pod_reference table
	podRefs, err := e.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
	if err != nil {
		log.Warnf("Failed to query workload_pod_reference for workload %s: %v", workloadUID, err)
	}

	var pods []*model.GpuPods
	if len(podRefs) > 0 {
		podUIDs := make([]string, 0, len(podRefs))
		for _, ref := range podRefs {
			podUIDs = append(podUIDs, ref.PodUID)
		}

		db := database.GetFacade().GetSystemConfig().GetDB()
		err = db.WithContext(ctx).
			Where("uid IN ? AND deleted = ?", podUIDs, false).
			Find(&pods).Error
		if err != nil {
			return nil, fmt.Errorf("failed to query pods by references: %w", err)
		}
	}

	// Method 2: Find pods of child workload
	if len(pods) == 0 {
		childWorkloads, err := e.workloadFacade.ListChildrenWorkloadByParentUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to query child workloads for %s: %v", workloadUID, err)
		} else if len(childWorkloads) > 0 {
			for _, child := range childWorkloads {
				childPod, err := e.selectTargetPod(ctx, child.UID)
				if err == nil && childPod != nil {
					return childPod, nil
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
			return pod, nil
		}
	}

	return pods[0], nil
}

// ensureDetectionRecord ensures detection record exists
func (e *ActiveDetectionExecutor) ensureDetectionRecord(ctx context.Context, workloadUID string) error {
	existing, err := e.detectionFacade.GetDetection(ctx, workloadUID)
	if err != nil {
		return err
	}

	if existing == nil {
		detection := &model.WorkloadDetection{
			WorkloadUID:    workloadUID,
			Status:         "unknown",
			DetectionState: "in_progress",
			AttemptCount:   0,
			MaxAttempts:    5,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}
		return e.detectionFacade.CreateDetection(ctx, detection)
	}

	// Update detection state if needed
	if existing.DetectionState != "in_progress" && existing.DetectionState != "completed" {
		return e.detectionFacade.UpdateDetectionState(ctx, workloadUID, "in_progress")
	}

	return nil
}

// Cancel cancels the task
func (e *ActiveDetectionExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Active detection task cancelled for workload %s", task.WorkloadUID)
	return nil
}

