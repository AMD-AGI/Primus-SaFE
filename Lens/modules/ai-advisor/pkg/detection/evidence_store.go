// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package detection

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// EvidenceStore handles storage of detection evidence
type EvidenceStore struct {
	evidenceFacade database.WorkloadDetectionEvidenceFacadeInterface
	layerResolver  *FrameworkLayerResolver
}

// NewEvidenceStore creates a new EvidenceStore
func NewEvidenceStore() *EvidenceStore {
	return &EvidenceStore{
		evidenceFacade: database.NewWorkloadDetectionEvidenceFacade(),
		layerResolver:  GetLayerResolver(),
	}
}

// NewEvidenceStoreWithFacade creates a new EvidenceStore with custom facade
func NewEvidenceStoreWithFacade(facade database.WorkloadDetectionEvidenceFacadeInterface) *EvidenceStore {
	return &EvidenceStore{
		evidenceFacade: facade,
		layerResolver:  GetLayerResolver(),
	}
}

// NewEvidenceStoreWithLayerResolver creates a new EvidenceStore with custom layer resolver
func NewEvidenceStoreWithLayerResolver(facade database.WorkloadDetectionEvidenceFacadeInterface, layerResolver *FrameworkLayerResolver) *EvidenceStore {
	return &EvidenceStore{
		evidenceFacade: facade,
		layerResolver:  layerResolver,
	}
}

// resolveFrameworkLayer resolves the layer for a framework using config
func (s *EvidenceStore) resolveFrameworkLayer(framework string) string {
	if s.layerResolver != nil {
		return s.layerResolver.GetLayer(framework)
	}
	// Fallback to hardcoded logic
	if isWrapperFramework(framework) {
		return FrameworkLayerWrapper
	}
	if isInferenceFramework(framework) {
		return FrameworkLayerInference
	}
	return FrameworkLayerRuntime
}

// StoreEvidenceRequest holds the parameters for storing evidence
type StoreEvidenceRequest struct {
	WorkloadUID      string
	Source           string                 // e.g., "wandb", "process", "env", "image", "log", "label", "active_detection"
	SourceType       string                 // "passive" or "active"
	Framework        string                 // Primary detected framework
	Frameworks       []string               // All detected frameworks (optional)
	WorkloadType     string                 // "training" or "inference"
	Confidence       float64                // Detection confidence [0.0-1.0]
	FrameworkLayer   string                 // "wrapper" or "base"
	WrapperFramework string                 // Wrapper framework name (if applicable)
	BaseFramework    string                 // Base framework name (if applicable)
	Evidence         map[string]interface{} // Raw evidence data
	ExpiresAt        *time.Time             // Optional expiration time
}

// StoreEvidence stores a single evidence record
func (s *EvidenceStore) StoreEvidence(ctx context.Context, req *StoreEvidenceRequest) error {
	if req.WorkloadUID == "" || req.Source == "" {
		log.Warnf("Invalid evidence request: workloadUID or source is empty")
		return nil
	}

	// Convert evidence to ExtType
	evidence := model.ExtType{}
	for k, v := range req.Evidence {
		evidence[k] = v
	}

	// Convert frameworks to ExtJSON
	var frameworksJSON model.ExtJSON
	if len(req.Frameworks) > 0 {
		if err := frameworksJSON.MarshalFrom(req.Frameworks); err != nil {
			log.Warnf("Failed to marshal frameworks: %v", err)
		}
	}

	now := time.Now()
	record := &model.WorkloadDetectionEvidence{
		WorkloadUID:      req.WorkloadUID,
		Source:           req.Source,
		SourceType:       req.SourceType,
		Framework:        req.Framework,
		Frameworks:       frameworksJSON,
		WorkloadType:     req.WorkloadType,
		Confidence:       req.Confidence,
		FrameworkLayer:   req.FrameworkLayer,
		WrapperFramework: req.WrapperFramework,
		BaseFramework:    req.BaseFramework,
		Evidence:         evidence,
		Processed:        false,
		DetectedAt:       now,
		CreatedAt:        now,
	}

	if req.ExpiresAt != nil {
		record.ExpiresAt = *req.ExpiresAt
	}

	if err := s.evidenceFacade.CreateEvidence(ctx, record); err != nil {
		log.Errorf("Failed to store evidence for workload %s from source %s: %v",
			req.WorkloadUID, req.Source, err)
		return err
	}

	log.Debugf("Stored evidence for workload %s from source %s (framework=%s, confidence=%.2f)",
		req.WorkloadUID, req.Source, req.Framework, req.Confidence)

	return nil
}

// StoreWandBEvidence stores evidence from WandB detection
func (s *EvidenceStore) StoreWandBEvidence(ctx context.Context, workloadUID string, result *DetectionResult, rawEvidence map[string]interface{}) error {
	req := &StoreEvidenceRequest{
		WorkloadUID:      workloadUID,
		Source:           "wandb",
		SourceType:       "passive",
		Framework:        result.Framework,
		WorkloadType:     "training",
		Confidence:       result.Confidence,
		FrameworkLayer:   result.FrameworkLayer,
		WrapperFramework: result.WrapperFramework,
		BaseFramework:    result.BaseFramework,
		Evidence:         rawEvidence,
	}

	// Add detection method to evidence
	if req.Evidence == nil {
		req.Evidence = make(map[string]interface{})
	}
	req.Evidence["method"] = result.Method
	req.Evidence["matched_env_vars"] = result.MatchedEnvVars
	req.Evidence["matched_modules"] = result.MatchedModules

	return s.StoreEvidence(ctx, req)
}

// StoreProcessEvidence stores evidence from process probing
func (s *EvidenceStore) StoreProcessEvidence(ctx context.Context, workloadUID string, framework string, confidence float64, cmdlines []string, processNames []string) error {
	evidence := map[string]interface{}{
		"cmdlines":      cmdlines,
		"process_names": processNames,
		"method":        "cmdline_pattern",
	}

	// Resolve framework layer from config
	layer := s.resolveFrameworkLayer(framework)

	req := &StoreEvidenceRequest{
		WorkloadUID:    workloadUID,
		Source:         "process",
		SourceType:     "active",
		Framework:      framework,
		WorkloadType:   "training", // Default to training, can be overridden
		Confidence:     confidence,
		FrameworkLayer: layer,
		Evidence:       evidence,
	}

	// Set layer-specific fields
	switch layer {
	case FrameworkLayerWrapper:
		req.WrapperFramework = framework
	default:
		req.BaseFramework = framework
	}

	return s.StoreEvidence(ctx, req)
}

// StoreEnvEvidence stores evidence from environment variable detection
func (s *EvidenceStore) StoreEnvEvidence(ctx context.Context, workloadUID string, framework string, confidence float64, matchedVars map[string]string, wrapperFramework, baseFramework string) error {
	evidence := map[string]interface{}{
		"matched_vars": matchedVars,
		"method":       "env_pattern",
	}

	req := &StoreEvidenceRequest{
		WorkloadUID:     workloadUID,
		Source:          "env",
		SourceType:      "active",
		Framework:       framework,
		WorkloadType:    "training",
		Confidence:      confidence,
		WrapperFramework: wrapperFramework,
		BaseFramework:   baseFramework,
		Evidence:        evidence,
	}

	if wrapperFramework != "" {
		req.FrameworkLayer = "wrapper"
	} else {
		req.FrameworkLayer = "base"
	}

	return s.StoreEvidence(ctx, req)
}

// StoreImageEvidence stores evidence from container image detection
func (s *EvidenceStore) StoreImageEvidence(ctx context.Context, workloadUID string, framework string, confidence float64, imageName string, workloadType string) error {
	evidence := map[string]interface{}{
		"image_name": imageName,
		"method":     "image_pattern",
	}

	// Resolve framework layer from config
	layer := s.resolveFrameworkLayer(framework)

	req := &StoreEvidenceRequest{
		WorkloadUID:    workloadUID,
		Source:         "image",
		SourceType:     "active",
		Framework:      framework,
		WorkloadType:   workloadType,
		Confidence:     confidence,
		FrameworkLayer: layer,
		Evidence:       evidence,
	}

	// Set layer-specific fields
	switch layer {
	case FrameworkLayerWrapper:
		req.WrapperFramework = framework
	default:
		req.BaseFramework = framework
	}

	return s.StoreEvidence(ctx, req)
}

// StoreLabelEvidence stores evidence from pod label/annotation detection
func (s *EvidenceStore) StoreLabelEvidence(ctx context.Context, workloadUID string, framework string, confidence float64, labels map[string]string, annotations map[string]string) error {
	evidence := map[string]interface{}{
		"labels":      labels,
		"annotations": annotations,
		"method":      "label_pattern",
	}

	// Resolve framework layer from config
	layer := s.resolveFrameworkLayer(framework)

	req := &StoreEvidenceRequest{
		WorkloadUID:    workloadUID,
		Source:         "label",
		SourceType:     "passive",
		Framework:      framework,
		WorkloadType:   "training",
		Confidence:     confidence,
		FrameworkLayer: layer,
		Evidence:       evidence,
	}

	// Set layer-specific fields
	switch layer {
	case FrameworkLayerWrapper:
		req.WrapperFramework = framework
	default:
		req.BaseFramework = framework
	}

	return s.StoreEvidence(ctx, req)
}

// StoreLogEvidence stores evidence from log pattern detection
func (s *EvidenceStore) StoreLogEvidence(ctx context.Context, workloadUID string, framework string, confidence float64, matchedPatterns []string, logSnippet string) error {
	evidence := map[string]interface{}{
		"matched_patterns": matchedPatterns,
		"log_snippet":      logSnippet,
		"method":           "log_pattern",
	}

	// Resolve framework layer from config
	layer := s.resolveFrameworkLayer(framework)

	req := &StoreEvidenceRequest{
		WorkloadUID:    workloadUID,
		Source:         "log",
		SourceType:     "passive",
		Framework:      framework,
		WorkloadType:   "training",
		Confidence:     confidence,
		FrameworkLayer: layer,
		Evidence:       evidence,
	}

	// Set layer-specific fields
	switch layer {
	case FrameworkLayerWrapper:
		req.WrapperFramework = framework
	default:
		req.BaseFramework = framework
	}

	return s.StoreEvidence(ctx, req)
}

// StoreInferenceEvidence stores evidence from inference framework detection
func (s *EvidenceStore) StoreInferenceEvidence(ctx context.Context, workloadUID string, framework string, confidence float64, matchContext map[string]interface{}) error {
	evidence := map[string]interface{}{
		"match_context": matchContext,
		"method":        "inference_pattern",
	}

	req := &StoreEvidenceRequest{
		WorkloadUID:    workloadUID,
		Source:         "active_detection",
		SourceType:     "active",
		Framework:      framework,
		WorkloadType:   "inference",
		Confidence:     confidence,
		FrameworkLayer: FrameworkLayerInference,
		BaseFramework:  framework,
		Evidence:       evidence,
	}

	return s.StoreEvidence(ctx, req)
}

// BatchStoreEvidence stores multiple evidence records in a single transaction
func (s *EvidenceStore) BatchStoreEvidence(ctx context.Context, requests []*StoreEvidenceRequest) error {
	if len(requests) == 0 {
		return nil
	}

	records := make([]*model.WorkloadDetectionEvidence, 0, len(requests))
	now := time.Now()

	for _, req := range requests {
		if req.WorkloadUID == "" || req.Source == "" {
			continue
		}

		evidence := model.ExtType{}
		for k, v := range req.Evidence {
			evidence[k] = v
		}

		var frameworksJSON model.ExtJSON
		if len(req.Frameworks) > 0 {
			if err := frameworksJSON.MarshalFrom(req.Frameworks); err != nil {
				log.Warnf("Failed to marshal frameworks: %v", err)
			}
		}

		record := &model.WorkloadDetectionEvidence{
			WorkloadUID:      req.WorkloadUID,
			Source:           req.Source,
			SourceType:       req.SourceType,
			Framework:        req.Framework,
			Frameworks:       frameworksJSON,
			WorkloadType:     req.WorkloadType,
			Confidence:       req.Confidence,
			FrameworkLayer:   req.FrameworkLayer,
			WrapperFramework: req.WrapperFramework,
			BaseFramework:    req.BaseFramework,
			Evidence:         evidence,
			Processed:        false,
			DetectedAt:       now,
			CreatedAt:        now,
		}

		if req.ExpiresAt != nil {
			record.ExpiresAt = *req.ExpiresAt
		}

		records = append(records, record)
	}

	if len(records) == 0 {
		return nil
	}

	return s.evidenceFacade.BatchCreateEvidence(ctx, records)
}

// isInferenceFramework checks if a framework is an inference framework
func isInferenceFramework(framework string) bool {
	inferenceFrameworks := map[string]bool{
		"vllm":      true,
		"triton":    true,
		"tgi":       true,
		"sglang":    true,
		"trtllm":    true,
		"text-generation-inference": true,
	}
	return inferenceFrameworks[framework]
}

