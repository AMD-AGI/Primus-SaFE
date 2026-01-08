package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/types"
)

// ProcessProbeExecutor probes process information from pods
type ProcessProbeExecutor struct {
	coreTask.BaseExecutor

	podProber       *common.PodProber
	evidenceStore   *detection.EvidenceStore
	layerResolver   *detection.FrameworkLayerResolver
	coverageFacade  database.DetectionCoverageFacadeInterface
}

// NewProcessProbeExecutor creates a new ProcessProbeExecutor
func NewProcessProbeExecutor(collector *metadata.Collector) *ProcessProbeExecutor {
	return &ProcessProbeExecutor{
		podProber:      common.NewPodProber(collector),
		evidenceStore:  detection.NewEvidenceStore(),
		layerResolver:  detection.GetLayerResolver(),
		coverageFacade: database.NewDetectionCoverageFacade(),
	}
}

// NewProcessProbeExecutorWithDeps creates executor with custom dependencies
func NewProcessProbeExecutorWithDeps(
	podProber *common.PodProber,
	evidenceStore *detection.EvidenceStore,
	coverageFacade database.DetectionCoverageFacadeInterface,
) *ProcessProbeExecutor {
	return &ProcessProbeExecutor{
		podProber:      podProber,
		evidenceStore:  evidenceStore,
		layerResolver:  detection.GetLayerResolver(),
		coverageFacade: coverageFacade,
	}
}

// GetTaskType returns the task type
func (e *ProcessProbeExecutor) GetTaskType() string {
	return constant.TaskTypeProcessProbe
}

// Validate validates task parameters
func (e *ProcessProbeExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute executes process probing
func (e *ProcessProbeExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID

	log.Infof("Starting process probe for workload %s", workloadUID)

	updates := map[string]interface{}{
		"started_at": time.Now().Format(time.RFC3339),
	}

	// Mark coverage as collecting
	if err := e.coverageFacade.MarkCollecting(ctx, workloadUID, constant.DetectionSourceProcess); err != nil {
		log.Warnf("Failed to mark process coverage as collecting: %v", err)
	}

	// Select target pod
	pod, err := e.podProber.SelectTargetPod(ctx, workloadUID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to select target pod: %v", err)
		log.Warnf("Process probe failed for workload %s: %s", workloadUID, errMsg)
		e.coverageFacade.MarkFailed(ctx, workloadUID, constant.DetectionSourceProcess, errMsg)
		updates["error"] = errMsg
		return coreTask.FailureResult(errMsg, updates), err
	}

	updates["pod_name"] = pod.Name
	updates["pod_namespace"] = pod.Namespace
	updates["node_name"] = pod.NodeName

	// Check pod readiness
	if !e.podProber.IsPodReady(ctx, pod) {
		errMsg := fmt.Sprintf("pod %s/%s is not ready", pod.Namespace, pod.Name)
		log.Warnf("Process probe failed for workload %s: %s", workloadUID, errMsg)
		e.coverageFacade.MarkFailed(ctx, workloadUID, constant.DetectionSourceProcess, errMsg)
		updates["error"] = errMsg
		return coreTask.FailureResult(errMsg, updates), fmt.Errorf("%s", errMsg)
	}

	// Check pod age (at least 10 seconds)
	podAge := e.podProber.GetPodAge(ctx, pod)
	if podAge < 10*time.Second {
		errMsg := fmt.Sprintf("pod %s/%s is too young (age: %v), waiting for startup", pod.Namespace, pod.Name, podAge)
		log.Infof("Process probe deferred for workload %s: %s", workloadUID, errMsg)
		updates["deferred"] = true
		updates["pod_age"] = podAge.String()
		return coreTask.FailureResult(errMsg, updates), fmt.Errorf("%s", errMsg)
	}

	// Get process tree
	tree, err := e.podProber.GetProcessTree(ctx, pod, common.DefaultProcessTreeOptions())
	if err != nil {
		errMsg := fmt.Sprintf("failed to get process tree: %v", err)
		log.Warnf("Process probe failed for workload %s: %s", workloadUID, errMsg)
		e.coverageFacade.MarkFailed(ctx, workloadUID, constant.DetectionSourceProcess, errMsg)
		updates["error"] = errMsg
		return coreTask.FailureResult(errMsg, updates), err
	}

	// Find Python process
	pythonProc := e.podProber.FindPythonProcess(tree)
	if pythonProc == nil {
		errMsg := "no Python process found"
		log.Infof("Process probe for workload %s: %s", workloadUID, errMsg)
		e.coverageFacade.MarkCollected(ctx, workloadUID, constant.DetectionSourceProcess, 0)
		updates["python_found"] = false
		return coreTask.SuccessResult(updates), nil
	}

	updates["python_found"] = true
	updates["python_pid"] = pythonProc.HostPID
	updates["python_cmdline"] = pythonProc.Cmdline

	// Extract evidence
	result := e.extractProcessEvidence(pythonProc, tree)
	updates["cmdlines_count"] = len(result.Cmdlines)
	updates["env_vars_count"] = len(result.EnvVars)

	// Detect frameworks and store evidence
	evidenceCount, err := e.storeEvidence(ctx, workloadUID, result)
	if err != nil {
		log.Warnf("Failed to store process evidence: %v", err)
	}

	updates["evidence_count"] = evidenceCount
	updates["completed_at"] = time.Now().Format(time.RFC3339)

	// Mark coverage as collected
	if err := e.coverageFacade.MarkCollected(ctx, workloadUID, constant.DetectionSourceProcess, int32(evidenceCount)); err != nil {
		log.Warnf("Failed to mark process coverage as collected: %v", err)
	}

	log.Infof("Process probe completed for workload %s: found %d evidence", workloadUID, evidenceCount)
	return coreTask.SuccessResult(updates), nil
}

// ProcessProbeResult holds the result of process probing
type ProcessProbeResult struct {
	Cmdlines     []string
	ProcessNames []string
	EnvVars      map[string]string
	Cwd          string
}

// extractProcessEvidence extracts evidence from process tree
func (e *ProcessProbeExecutor) extractProcessEvidence(pythonProc *types.ProcessInfo, tree *types.PodProcessTree) *ProcessProbeResult {
	result := &ProcessProbeResult{
		Cmdlines:     []string{},
		ProcessNames: []string{},
		EnvVars:      make(map[string]string),
	}

	// Extract from main Python process
	if pythonProc != nil {
		result.Cmdlines = append(result.Cmdlines, pythonProc.Cmdline)
		result.ProcessNames = append(result.ProcessNames, pythonProc.Comm)
		result.Cwd = pythonProc.Cwd
		result.EnvVars = e.podProber.ExtractEnvMap(pythonProc)
	}

	// Also extract from all Python processes in tree
	if tree != nil {
		for _, container := range tree.Containers {
			if container.RootProcess != nil {
				e.extractAllPythonCmdlines(container.RootProcess, result)
			}
		}
	}

	return result
}

// extractAllPythonCmdlines recursively extracts cmdlines from all Python processes
func (e *ProcessProbeExecutor) extractAllPythonCmdlines(proc *types.ProcessInfo, result *ProcessProbeResult) {
	if proc == nil {
		return
	}

	if proc.IsPython && proc.Cmdline != "" {
		// Avoid duplicates
		found := false
		for _, cmd := range result.Cmdlines {
			if cmd == proc.Cmdline {
				found = true
				break
			}
		}
		if !found {
			result.Cmdlines = append(result.Cmdlines, proc.Cmdline)
			result.ProcessNames = append(result.ProcessNames, proc.Comm)
		}
	}

	for _, child := range proc.Children {
		e.extractAllPythonCmdlines(child, result)
	}
}

// storeEvidence analyzes and stores evidence
func (e *ProcessProbeExecutor) storeEvidence(ctx context.Context, workloadUID string, result *ProcessProbeResult) (int, error) {
	var requests []*detection.StoreEvidenceRequest
	detectedFrameworks := make(map[string]bool)

	// Detect from cmdlines
	for _, cmdline := range result.Cmdlines {
		fw, wlType := e.detectFrameworkFromCmdline(cmdline)
		if fw != "" && !detectedFrameworks[fw] {
			detectedFrameworks[fw] = true
			requests = append(requests, &detection.StoreEvidenceRequest{
				WorkloadUID:    workloadUID,
				Source:         constant.DetectionSourceProcess,
				SourceType:     "active",
				Framework:      fw,
				WorkloadType:   wlType,
				Confidence:     0.75,
				FrameworkLayer: e.getFrameworkLayer(fw),
				Evidence: map[string]interface{}{
					"cmdline": cmdline,
					"method":  "cmdline_pattern",
				},
			})
		}
	}

	// Detect from env vars
	if len(result.EnvVars) > 0 {
		if envResult := e.detectFrameworkFromEnv(result.EnvVars); envResult != nil && !detectedFrameworks[envResult.Framework] {
			detectedFrameworks[envResult.Framework] = true
			requests = append(requests, &detection.StoreEvidenceRequest{
				WorkloadUID:      workloadUID,
				Source:          constant.DetectionSourceProcess,
				SourceType:      "active",
				Framework:       envResult.Framework,
				WorkloadType:    "training",
				Confidence:      envResult.Confidence,
				FrameworkLayer:  envResult.Layer,
				WrapperFramework: envResult.WrapperFramework,
				BaseFramework:   envResult.BaseFramework,
				Evidence: map[string]interface{}{
					"matched_vars": e.getMatchedEnvVars(result.EnvVars),
					"method":       "env_pattern",
				},
			})
		}
	}

	if len(requests) > 0 {
		if err := e.evidenceStore.BatchStoreEvidence(ctx, requests); err != nil {
			return 0, err
		}
	}

	return len(requests), nil
}

// EnvDetectionResult holds env detection result
type EnvDetectionResult struct {
	Framework        string
	Confidence       float64
	Layer            string
	WrapperFramework string
	BaseFramework    string
}

// detectFrameworkFromCmdline detects framework from command line
func (e *ProcessProbeExecutor) detectFrameworkFromCmdline(cmdline string) (string, string) {
	cmdlineLower := strings.ToLower(cmdline)

	// Inference patterns
	inferencePatterns := map[string][]string{
		"vllm":   {"vllm", "vllm.entrypoints"},
		"triton": {"tritonserver"},
		"tgi":    {"text-generation-launcher"},
		"sglang": {"sglang"},
	}

	for fw, keywords := range inferencePatterns {
		for _, kw := range keywords {
			if strings.Contains(cmdlineLower, kw) {
				return fw, "inference"
			}
		}
	}

	// Training patterns
	trainingPatterns := map[string][]string{
		"primus":    {"primus", "primus-train", "primus.train"},
		"megatron":  {"megatron", "pretrain_gpt", "megatron-lm", "megatron_lm"},
		"deepspeed": {"deepspeed", "ds_config"},
		"pytorch":   {"torch.distributed", "torchrun", "torch.distributed.launch"},
	}

	for fw, keywords := range trainingPatterns {
		for _, kw := range keywords {
			if strings.Contains(cmdlineLower, kw) {
				return fw, "training"
			}
		}
	}

	return "", ""
}

// detectFrameworkFromEnv detects framework from environment variables
func (e *ProcessProbeExecutor) detectFrameworkFromEnv(envVars map[string]string) *EnvDetectionResult {
	// Primus
	if _, ok := envVars["PRIMUS_CONFIG"]; ok {
		result := &EnvDetectionResult{
			Framework:        "primus",
			Confidence:       0.85,
			Layer:            "wrapper",
			WrapperFramework: "primus",
		}
		if backend := envVars["PRIMUS_BACKEND"]; backend != "" {
			result.BaseFramework = strings.ToLower(backend)
		}
		return result
	}
	if _, ok := envVars["PRIMUS_VERSION"]; ok {
		return &EnvDetectionResult{
			Framework:        "primus",
			Confidence:       0.8,
			Layer:            "wrapper",
			WrapperFramework: "primus",
		}
	}

	// DeepSpeed
	if _, ok := envVars["DEEPSPEED_CONFIG"]; ok {
		return &EnvDetectionResult{
			Framework:     "deepspeed",
			Confidence:    0.85,
			Layer:         "base",
			BaseFramework: "deepspeed",
		}
	}
	if _, ok := envVars["DS_CONFIG"]; ok {
		return &EnvDetectionResult{
			Framework:     "deepspeed",
			Confidence:    0.8,
			Layer:         "base",
			BaseFramework: "deepspeed",
		}
	}

	// Megatron
	if _, ok := envVars["MEGATRON_CONFIG"]; ok {
		return &EnvDetectionResult{
			Framework:     "megatron",
			Confidence:    0.85,
			Layer:         "base",
			BaseFramework: "megatron",
		}
	}

	return nil
}

// getMatchedEnvVars returns framework-related env vars
func (e *ProcessProbeExecutor) getMatchedEnvVars(envVars map[string]string) map[string]string {
	matched := make(map[string]string)
	frameworkVars := []string{
		"PRIMUS_CONFIG", "PRIMUS_VERSION", "PRIMUS_BACKEND",
		"DEEPSPEED_CONFIG", "DS_CONFIG",
		"MEGATRON_CONFIG", "MEGATRON_LM_PATH",
		"FRAMEWORK", "TRAINING_FRAMEWORK",
	}

	for _, v := range frameworkVars {
		if val, ok := envVars[v]; ok {
			matched[v] = val
		}
	}
	return matched
}

// getFrameworkLayer returns the framework layer using config-based resolution
func (e *ProcessProbeExecutor) getFrameworkLayer(framework string) string {
	if e.layerResolver != nil {
		return e.layerResolver.GetLayer(framework)
	}
	// Fallback to runtime as default
	return detection.FrameworkLayerRuntime
}

// Cancel cancels the task
func (e *ProcessProbeExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Process probe task cancelled for workload %s", task.WorkloadUID)
	return nil
}

