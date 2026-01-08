// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package reconciler

import (
	"context"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	primusSafeV1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
)

// FrameworkDetectionIntegration Framework detection integration point
type FrameworkDetectionIntegration struct {
	reuseEngine      *framework.ReuseEngine
	detectionManager *framework.FrameworkDetectionManager
	workloadAnalyzer *WorkloadAnalyzer
}

// NewFrameworkDetectionIntegration Create framework detection integrator
func NewFrameworkDetectionIntegration(
	db database.AiWorkloadMetadataFacadeInterface,
) (*FrameworkDetectionIntegration, error) {

	// Initialize ReuseEngine
	reuseConfig := model.ReuseConfig{
		Enabled:             true,
		MinSimilarityScore:  0.85,
		TimeWindowDays:      30,
		MinConfidence:       0.75,
		ConfidenceDecayRate: 0.9,
		MaxCandidates:       100,
		CacheTTLMinutes:     10,
	}

	reuseEngine := framework.NewReuseEngine(db, reuseConfig)

	// Initialize DetectionManager
	detectionConfig := &framework.DetectionConfig{
		SuspectedThreshold: 0.3,
		ConfirmedThreshold: 0.6,
		VerifiedThreshold:  0.85,
		ConflictPenalty:    0.2,
		MultiSourceBoost:   0.1,
		EnableCache:        true,
		CacheTTLSec:        300,
	}

	detectionManager := framework.NewFrameworkDetectionManager(db, detectionConfig)

	// Initialize WorkloadAnalyzer
	analyzer := NewWorkloadAnalyzer()

	return &FrameworkDetectionIntegration{
		reuseEngine:      reuseEngine,
		detectionManager: detectionManager,
		workloadAnalyzer: analyzer,
	}, nil
}

// OnWorkloadCreated Triggered when Workload is created
func (f *FrameworkDetectionIntegration) OnWorkloadCreated(
	ctx context.Context,
	workload *primusSafeV1.Workload,
) error {
	log.Infof("Starting framework detection for workload %s", workload.UID)

	// ========== Step 1: Try to reuse Metadata ==========
	reusedDetection, err := f.tryReuseMetadata(ctx, workload)
	if err != nil {
		log.Errorf("Failed to try reuse: %v", err)
		// Don't block, continue execution
	}

	if reusedDetection != nil {
		log.Infof("✓ Reused metadata from %s, framework=%v, confidence=%.2f",
			reusedDetection.ReuseInfo.ReusedFrom,
			reusedDetection.Frameworks,
			reusedDetection.Confidence)
	}

	// ========== Step 2: Execute component analysis ==========
	componentDetection, err := f.analyzeWorkloadComponents(ctx, workload)
	if err != nil {
		log.Errorf("Failed to analyze components: %v", err)
		// Don't block, continue execution
	}

	if componentDetection != nil {
		log.Infof("✓ Component detection: framework=%s, confidence=%.2f",
			componentDetection.Framework,
			componentDetection.Confidence)
	}

	// ========== Step 3: Get final detection result ==========
	finalDetection, err := f.detectionManager.GetDetection(ctx, string(workload.UID))
	if err != nil {
		return fmt.Errorf("failed to get final detection: %w", err)
	}

	if finalDetection != nil {
		log.Infof("✓ Framework detection completed: framework=%v, confidence=%.2f, status=%s, sources=%d",
			finalDetection.Frameworks,
			finalDetection.Confidence,
			finalDetection.Status,
			len(finalDetection.Sources))
	}

	return nil
}

// tryReuseMetadata Try to reuse existing Metadata
func (f *FrameworkDetectionIntegration) tryReuseMetadata(
	ctx context.Context,
	workload *primusSafeV1.Workload,
) (*model.FrameworkDetection, error) {

	// Convert to internal workload type for ReuseEngine
	internalWorkload := convertToInternalWorkload(workload)

	// Call ReuseEngine
	detection, err := f.reuseEngine.TryReuse(ctx, internalWorkload)
	if err != nil {
		return nil, err
	}

	if detection == nil {
		log.Debug("No similar workload found for reuse")
		return nil, nil
	}

	// Report reuse result to DetectionManager
	evidence := map[string]interface{}{
		"method":           "workload_similarity",
		"reused_from":      detection.ReuseInfo.ReusedFrom,
		"similarity_score": detection.ReuseInfo.SimilarityScore,
		"reuse_reason":     "high_similarity_match",
	}

	// Get primary framework from Frameworks array
	primaryFramework := "unknown"
	if len(detection.Frameworks) > 0 {
		primaryFramework = detection.Frameworks[0]
	}

	err = f.detectionManager.ReportDetection(
		ctx,
		string(workload.UID),
		"reuse",
		primaryFramework,
		detection.Type,
		detection.Confidence,
		evidence,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to report reuse detection: %w", err)
	}

	return detection, nil
}

// analyzeWorkloadComponents Analyze Workload component features
func (f *FrameworkDetectionIntegration) analyzeWorkloadComponents(
	ctx context.Context,
	workload *primusSafeV1.Workload,
) (*ComponentDetectionResult, error) {

	// Call WorkloadAnalyzer
	result := f.workloadAnalyzer.Analyze(workload)
	if result.Framework == "" || result.Framework == "unknown" {
		log.Debug("Component analysis did not detect framework")
		return nil, nil
	}

	// Report component detection result
	evidence := map[string]interface{}{
		"method":           "component_analysis",
		"image":            extractImage(workload),
		"command":          extractCommand(workload),
		"args":             extractArgs(workload),
		"matched_env_vars": result.MatchedEnvVars,
		"reason":           result.Reason,
	}

	err := f.detectionManager.ReportDetection(
		ctx,
		string(workload.UID),
		"component",
		result.Framework,
		result.Type,
		result.Confidence,
		evidence,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to report component detection: %w", err)
	}

	return result, nil
}

// convertToInternalWorkload Convert PrimusSafe Workload to internal workload type
func convertToInternalWorkload(w *primusSafeV1.Workload) *framework.Workload {
	// Extract image from workload spec
	image := w.Spec.Image

	// Extract command and args from EntryPoint
	command := []string{}
	args := []string{}
	if w.Spec.EntryPoint != "" {
		// EntryPoint is base64 encoded, we can use it as a single command
		command = []string{"sh", "-c"}
		args = []string{w.Spec.EntryPoint}
	}

	// Extract environment variables
	env := make(map[string]string)
	if w.Spec.Env != nil {
		env = w.Spec.Env
	}

	return &framework.Workload{
		UID:       string(w.UID),
		Namespace: w.Namespace,
		Image:     image,
		Command:   command,
		Args:      args,
		Env:       env,
		Labels:    w.Labels,
	}
}

// extractImage Extract image from workload
func extractImage(w *primusSafeV1.Workload) string {
	return w.Spec.Image
}

// extractCommand Extract command from workload
func extractCommand(w *primusSafeV1.Workload) []string {
	if w.Spec.EntryPoint != "" {
		return []string{"sh", "-c"}
	}
	return []string{}
}

// extractArgs Extract args from workload
func extractArgs(w *primusSafeV1.Workload) []string {
	if w.Spec.EntryPoint != "" {
		return []string{w.Spec.EntryPoint}
	}
	return []string{}
}
