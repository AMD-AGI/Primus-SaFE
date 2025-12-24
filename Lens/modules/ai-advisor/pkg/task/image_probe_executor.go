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

// ImageProbeExecutor probes container image information for framework detection
type ImageProbeExecutor struct {
	coreTask.BaseExecutor

	podProber        *common.PodProber
	evidenceStore    *detection.EvidenceStore
	layerResolver    *detection.FrameworkLayerResolver
	coverageFacade   database.DetectionCoverageFacadeInterface
	metadataFacade   database.AiWorkloadMetadataFacadeInterface
}

// NewImageProbeExecutor creates a new ImageProbeExecutor
func NewImageProbeExecutor(collector *metadata.Collector) *ImageProbeExecutor {
	return &ImageProbeExecutor{
		podProber:      common.NewPodProber(collector),
		evidenceStore:  detection.NewEvidenceStore(),
		layerResolver:  detection.GetLayerResolver(),
		coverageFacade: database.NewDetectionCoverageFacade(),
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
	}
}

// NewImageProbeExecutorWithDeps creates executor with custom dependencies
func NewImageProbeExecutorWithDeps(
	podProber *common.PodProber,
	evidenceStore *detection.EvidenceStore,
	coverageFacade database.DetectionCoverageFacadeInterface,
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
) *ImageProbeExecutor {
	return &ImageProbeExecutor{
		podProber:      podProber,
		evidenceStore:  evidenceStore,
		layerResolver:  detection.GetLayerResolver(),
		coverageFacade: coverageFacade,
		metadataFacade: metadataFacade,
	}
}

// GetTaskType returns the task type
func (e *ImageProbeExecutor) GetTaskType() string {
	return constant.TaskTypeImageProbe
}

// Validate validates task parameters
func (e *ImageProbeExecutor) Validate(task *model.WorkloadTaskState) error {
	if task.WorkloadUID == "" {
		return fmt.Errorf("workload_uid is required")
	}
	return nil
}

// Execute executes image probing
func (e *ImageProbeExecutor) Execute(
	ctx context.Context,
	execCtx *coreTask.ExecutionContext,
) (*coreTask.ExecutionResult, error) {
	task := execCtx.Task
	workloadUID := task.WorkloadUID

	log.Infof("Starting image probe for workload %s", workloadUID)

	updates := map[string]interface{}{
		"started_at": time.Now().Format(time.RFC3339),
	}

	// Mark coverage as collecting
	if err := e.coverageFacade.MarkCollecting(ctx, workloadUID, constant.DetectionSourceImage); err != nil {
		log.Warnf("Failed to mark image coverage as collecting: %v", err)
	}

	// Try to get image from multiple sources
	imageName, imageTag := e.getImageInfo(ctx, workloadUID)

	if imageName == "" {
		log.Infof("No image information found for workload %s", workloadUID)
		updates["image_found"] = false
		e.coverageFacade.MarkCollected(ctx, workloadUID, constant.DetectionSourceImage, 0)
		return coreTask.SuccessResult(updates), nil
	}

	updates["image_found"] = true
	updates["image_name"] = imageName
	updates["image_tag"] = imageTag

	// Detect framework from image
	framework, workloadType := e.detectFrameworkFromImage(imageName)
	updates["detected_framework"] = framework
	updates["workload_type"] = workloadType

	evidenceCount := 0
	if framework != "" {
		// Store evidence
		if err := e.storeImageEvidence(ctx, workloadUID, framework, workloadType, imageName, imageTag); err != nil {
			log.Warnf("Failed to store image evidence: %v", err)
		} else {
			evidenceCount = 1
		}
	}

	updates["evidence_count"] = evidenceCount
	updates["completed_at"] = time.Now().Format(time.RFC3339)

	// Mark coverage as collected
	if err := e.coverageFacade.MarkCollected(ctx, workloadUID, constant.DetectionSourceImage, int32(evidenceCount)); err != nil {
		log.Warnf("Failed to mark image coverage as collected: %v", err)
	}

	log.Infof("Image probe completed for workload %s: image=%s, framework=%s", workloadUID, imageName, framework)
	return coreTask.SuccessResult(updates), nil
}

// getImageInfo tries to get image info from multiple sources
func (e *ImageProbeExecutor) getImageInfo(ctx context.Context, workloadUID string) (string, string) {
	// Try 1: Get from ai_workload_metadata
	metadata, err := e.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err == nil && metadata != nil && metadata.Metadata != nil {
		// Check workload_signature
		if signatureData, ok := metadata.Metadata["workload_signature"].(map[string]interface{}); ok {
			if imageName, ok := signatureData["image"].(string); ok && imageName != "" {
				return e.parseImageNameAndTag(imageName)
			}
		}

		// Check container_image field
		if imageName, ok := metadata.Metadata["container_image"].(string); ok && imageName != "" {
			return e.parseImageNameAndTag(imageName)
		}
	}

	// Try 2: Get from pod info via pod prober
	pod, err := e.podProber.SelectTargetPod(ctx, workloadUID)
	if err == nil && pod != nil {
		// GpuPods might have image info in ext or other fields
		// For now, return empty as GpuPods model doesn't have direct image field
		// In production, this would query container info from K8s API
	}

	return "", ""
}

// parseImageNameAndTag splits image name and tag
func (e *ImageProbeExecutor) parseImageNameAndTag(fullImage string) (string, string) {
	// Handle digest format (image@sha256:...)
	if idx := strings.Index(fullImage, "@"); idx > 0 {
		return fullImage[:idx], fullImage[idx+1:]
	}

	// Handle tag format (image:tag)
	parts := strings.Split(fullImage, ":")
	if len(parts) > 1 {
		// Handle port in registry (e.g., registry:5000/image:tag)
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
		// Registry with port
		tag := parts[len(parts)-1]
		name := strings.Join(parts[:len(parts)-1], ":")
		return name, tag
	}

	return fullImage, ""
}

// detectFrameworkFromImage detects framework from container image name
func (e *ImageProbeExecutor) detectFrameworkFromImage(imageName string) (string, string) {
	imageLower := strings.ToLower(imageName)

	// Inference frameworks (higher specificity)
	inferencePatterns := map[string][]string{
		"vllm":      {"vllm", "/vllm-"},
		"triton":    {"triton", "tritonserver"},
		"tgi":       {"text-generation-inference", "tgi", "huggingface/text-generation"},
		"sglang":    {"sglang"},
		"trtllm":    {"tensorrt-llm", "trt-llm"},
		"ollama":    {"ollama"},
		"llamacpp":  {"llama.cpp", "llama-cpp"},
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
		"primus":    {"primus", "primus-training"},
		"megatron":  {"megatron", "megatron-lm", "nemo"},
		"deepspeed": {"deepspeed"},
		"pytorch":   {"pytorch", "torch"},
		"jax":       {"jax"},
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

// storeImageEvidence stores image detection evidence
func (e *ImageProbeExecutor) storeImageEvidence(
	ctx context.Context,
	workloadUID string,
	framework string,
	workloadType string,
	imageName string,
	imageTag string,
) error {
	// Resolve layer from config
	layer := e.resolveFrameworkLayer(framework)

	req := &detection.StoreEvidenceRequest{
		WorkloadUID:    workloadUID,
		Source:         constant.DetectionSourceImage,
		SourceType:     "active",
		Framework:      framework,
		WorkloadType:   workloadType,
		Confidence:     0.6, // Image-based detection has lower confidence
		FrameworkLayer: layer,
		Evidence: map[string]interface{}{
			"image_name": imageName,
			"image_tag":  imageTag,
			"method":     "image_pattern",
		},
	}

	// Set layer-specific fields
	if layer == detection.FrameworkLayerWrapper {
		req.WrapperFramework = framework
	} else {
		req.BaseFramework = framework
	}

	return e.evidenceStore.StoreEvidence(ctx, req)
}

// resolveFrameworkLayer resolves the layer for a framework using config
func (e *ImageProbeExecutor) resolveFrameworkLayer(framework string) string {
	if e.layerResolver != nil {
		return e.layerResolver.GetLayer(framework)
	}
	// Fallback to runtime as default
	return detection.FrameworkLayerRuntime
}

// isInferenceFramework checks if framework is an inference framework
func (e *ImageProbeExecutor) isInferenceFramework(framework string) bool {
	inferenceFrameworks := map[string]bool{
		"vllm":     true,
		"triton":   true,
		"tgi":      true,
		"sglang":   true,
		"trtllm":   true,
		"ollama":   true,
		"llamacpp": true,
	}
	return inferenceFrameworks[framework]
}

// isBaseFramework checks if framework is a base framework
func (e *ImageProbeExecutor) isBaseFramework(framework string) bool {
	baseFrameworks := map[string]bool{
		"pytorch":   true,
		"jax":       true,
		"megatron":  true,
		"deepspeed": true,
	}
	return baseFrameworks[framework]
}

// Cancel cancels the task
func (e *ImageProbeExecutor) Cancel(ctx context.Context, task *model.WorkloadTaskState) error {
	log.Infof("Image probe task cancelled for workload %s", task.WorkloadUID)
	return nil
}

