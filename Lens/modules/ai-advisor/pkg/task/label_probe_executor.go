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
)

// LabelProbeExecutor probes pod labels and annotations for framework detection
type LabelProbeExecutor struct {
	coreTask.BaseExecutor

	podProber       *common.PodProber
	evidenceStore   *detection.EvidenceStore
	coverageFacade  database.DetectionCoverageFacadeInterface
	workloadFacade  database.WorkloadFacadeInterface
}

// NewLabelProbeExecutor creates a new LabelProbeExecutor
func NewLabelProbeExecutor(collector *metadata.Collector) *LabelProbeExecutor {
	return &LabelProbeExecutor{
		podProber:      common.NewPodProber(collector),
		evidenceStore:  detection.NewEvidenceStore(),
		coverageFacade: database.NewDetectionCoverageFacade(),
		workloadFacade: database.GetFacade().GetWorkload(),
	}
}

// NewLabelProbeExecutorWithDeps creates executor with custom dependencies
func NewLabelProbeExecutorWithDeps(
	podProber *common.PodProber,
	evidenceStore *detection.EvidenceStore,
	coverageFacade database.DetectionCoverageFacadeInterface,
	workloadFacade database.WorkloadFacadeInterface,
) *LabelProbeExecutor {
	return &LabelProbeExecutor{
		podProber:      podProber,
		evidenceStore:  evidenceStore,
		coverageFacade: coverageFacade,
		workloadFacade: workloadFacade,
	}
}

// GetTaskType returns the task type
func (e *LabelProbeExecutor) GetTaskType() string {
	return constant.TaskTypeLabelProbe
}

// Validate validates task parameters
func (e *LabelProbeExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute executes label probing
func (e *LabelProbeExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID

	log.Infof("Starting label probe for workload %s", workloadUID)

	updates := map[string]interface{}{
		"started_at": time.Now().Format(time.RFC3339),
	}

	// Mark coverage as collecting
	if err := e.coverageFacade.MarkCollecting(ctx, workloadUID, constant.DetectionSourceLabel); err != nil {
		log.Warnf("Failed to mark label coverage as collecting: %v", err)
	}

	// Get labels and annotations from workload and pods
	labels, annotations := e.getLabelInfo(ctx, workloadUID)

	updates["labels_count"] = len(labels)
	updates["annotations_count"] = len(annotations)

	if len(labels) == 0 && len(annotations) == 0 {
		log.Infof("No labels or annotations found for workload %s", workloadUID)
		e.coverageFacade.MarkCollected(ctx, workloadUID, constant.DetectionSourceLabel, 0)
		return coreTask.SuccessResult(updates), nil
	}

	// Detect frameworks from labels and annotations
	evidenceCount := 0
	detectedFrameworks := make(map[string]bool)

	// Check labels
	for key, value := range labels {
		if fw, confidence := e.detectFrameworkFromLabel(key, value); fw != "" && !detectedFrameworks[fw] {
			detectedFrameworks[fw] = true
			if err := e.storeLabelEvidence(ctx, workloadUID, fw, confidence, key, value, "label"); err != nil {
				log.Warnf("Failed to store label evidence: %v", err)
			} else {
				evidenceCount++
			}
		}
	}

	// Check annotations
	for key, value := range annotations {
		if fw, confidence := e.detectFrameworkFromAnnotation(key, value); fw != "" && !detectedFrameworks[fw] {
			detectedFrameworks[fw] = true
			if err := e.storeLabelEvidence(ctx, workloadUID, fw, confidence, key, value, "annotation"); err != nil {
				log.Warnf("Failed to store annotation evidence: %v", err)
			} else {
				evidenceCount++
			}
		}
	}

	updates["evidence_count"] = evidenceCount
	updates["detected_frameworks"] = e.mapKeys(detectedFrameworks)
	updates["completed_at"] = time.Now().Format(time.RFC3339)

	// Mark coverage as collected
	if err := e.coverageFacade.MarkCollected(ctx, workloadUID, constant.DetectionSourceLabel, int32(evidenceCount)); err != nil {
		log.Warnf("Failed to mark label coverage as collected: %v", err)
	}

	log.Infof("Label probe completed for workload %s: found %d evidence", workloadUID, evidenceCount)
	return coreTask.SuccessResult(updates), nil
}

// getLabelInfo gets labels and annotations from workload and its pods
func (e *LabelProbeExecutor) getLabelInfo(ctx context.Context, workloadUID string) (map[string]string, map[string]string) {
	labels := make(map[string]string)
	annotations := make(map[string]string)

	// Get workload info
	workload, err := e.workloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	if err == nil && workload != nil {
		// Extract labels from workload's Labels field (ExtType)
		if workload.Labels != nil {
			for k, v := range workload.Labels {
				if str, ok := v.(string); ok {
					labels[k] = str
				}
			}
		}
		// Extract annotations from workload's Annotations field (ExtType)
		if workload.Annotations != nil {
			for k, v := range workload.Annotations {
				if str, ok := v.(string); ok {
					annotations[k] = str
				}
			}
		}
	}

	// Note: GpuPods model doesn't have labels/annotations fields
	// Pod labels would need to be fetched via K8s API if needed

	return labels, annotations
}

// detectFrameworkFromLabel detects framework from label key-value pair
func (e *LabelProbeExecutor) detectFrameworkFromLabel(key, value string) (string, float64) {
	keyLower := strings.ToLower(key)
	valueLower := strings.ToLower(value)

	// Standard Kubernetes labels
	if keyLower == "app.kubernetes.io/name" || keyLower == "app" {
		if fw := e.matchFrameworkName(valueLower); fw != "" {
			return fw, 0.7
		}
	}

	if keyLower == "app.kubernetes.io/component" || keyLower == "component" {
		if strings.Contains(valueLower, "training") || strings.Contains(valueLower, "inference") {
			if fw := e.matchFrameworkName(valueLower); fw != "" {
				return fw, 0.6
			}
		}
	}

	// Framework-specific labels
	if strings.Contains(keyLower, "framework") {
		if fw := e.matchFrameworkName(valueLower); fw != "" {
			return fw, 0.8
		}
	}

	// Training job labels
	if strings.Contains(keyLower, "training-job") || strings.Contains(keyLower, "trainingjob") {
		if fw := e.matchFrameworkName(valueLower); fw != "" {
			return fw, 0.7
		}
	}

	// Kubeflow/PyTorch operator labels
	if strings.Contains(keyLower, "pytorch-job") || strings.Contains(keyLower, "pytorchjob") {
		return "pytorch", 0.8
	}
	if strings.Contains(keyLower, "mpi-job") || strings.Contains(keyLower, "mpijob") {
		return "mpi", 0.7
	}

	return "", 0
}

// detectFrameworkFromAnnotation detects framework from annotation
func (e *LabelProbeExecutor) detectFrameworkFromAnnotation(key, value string) (string, float64) {
	keyLower := strings.ToLower(key)
	valueLower := strings.ToLower(value)

	// Framework annotations
	if strings.Contains(keyLower, "framework") {
		if fw := e.matchFrameworkName(valueLower); fw != "" {
			return fw, 0.75
		}
	}

	// Version annotations might reveal framework
	if strings.Contains(keyLower, "version") {
		if strings.Contains(valueLower, "primus") {
			return "primus", 0.7
		}
		if strings.Contains(valueLower, "megatron") {
			return "megatron", 0.7
		}
	}

	// Config annotations
	if strings.Contains(keyLower, "config") {
		if strings.Contains(valueLower, "deepspeed") {
			return "deepspeed", 0.65
		}
	}

	return "", 0
}

// matchFrameworkName tries to match a framework name from text
func (e *LabelProbeExecutor) matchFrameworkName(text string) string {
	frameworks := map[string][]string{
		"primus":    {"primus"},
		"megatron":  {"megatron", "megatron-lm"},
		"deepspeed": {"deepspeed"},
		"pytorch":   {"pytorch", "torch"},
		"vllm":      {"vllm"},
		"triton":    {"triton", "tritonserver"},
		"tgi":       {"tgi", "text-generation-inference"},
		"sglang":    {"sglang"},
	}

	for fw, keywords := range frameworks {
		for _, kw := range keywords {
			if strings.Contains(text, kw) {
				return fw
			}
		}
	}

	return ""
}

// storeLabelEvidence stores label/annotation detection evidence
func (e *LabelProbeExecutor) storeLabelEvidence(
	ctx context.Context,
	workloadUID string,
	framework string,
	confidence float64,
	key string,
	value string,
	labelType string,
) error {
	req := &detection.StoreEvidenceRequest{
		WorkloadUID:  workloadUID,
		Source:       constant.DetectionSourceLabel,
		SourceType:   "active",
		Framework:    framework,
		WorkloadType: e.inferWorkloadType(framework),
		Confidence:   confidence,
		Evidence: map[string]interface{}{
			"label_key":   key,
			"label_value": value,
			"label_type":  labelType,
			"method":      "label_pattern",
		},
	}

	// Set framework layer
	if e.isWrapperFramework(framework) {
		req.FrameworkLayer = "wrapper"
		req.WrapperFramework = framework
	} else {
		req.FrameworkLayer = "base"
		req.BaseFramework = framework
	}

	return e.evidenceStore.StoreEvidence(ctx, req)
}

// inferWorkloadType infers workload type from framework
func (e *LabelProbeExecutor) inferWorkloadType(framework string) string {
	inferenceFrameworks := map[string]bool{
		"vllm":   true,
		"triton": true,
		"tgi":    true,
		"sglang": true,
	}
	if inferenceFrameworks[framework] {
		return "inference"
	}
	return "training"
}

// isWrapperFramework checks if framework is a wrapper framework
func (e *LabelProbeExecutor) isWrapperFramework(framework string) bool {
	wrapperFrameworks := map[string]bool{
		"primus": true,
	}
	return wrapperFrameworks[framework]
}

// mapKeys returns keys of a map as slice
func (e *LabelProbeExecutor) mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Cancel cancels the task
func (e *LabelProbeExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Label probe task cancelled for workload %s", task.WorkloadUID)
	return nil
}

