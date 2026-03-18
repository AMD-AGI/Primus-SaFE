// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package intent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// L1L2CoverageThreshold is the minimum combined L1+L2 coverage to skip L3
	L1L2CoverageThreshold = 0.80

	// L1L2ConfidenceThreshold is the minimum confidence to skip L3
	L1L2ConfidenceThreshold = 0.75
)

// Analyzer is the top-level intent analysis dispatcher.
// It orchestrates the three analysis layers:
//
//	L1 (CmdlineExtractor) → L2 (ConfigExtractor) → confidence check → L3 (Conductor LLM)
//
// The analysis stops as soon as sufficient confidence is reached, minimizing
// LLM usage and cost.
type Analyzer struct {
	cmdlineExtractor *CmdlineExtractor
	configExtractor  *ConfigExtractor
	modelParser      *ModelNameParser
	reuseChecker     *ReuseChecker

	// Conductor client for L3 (nil if not configured)
	conductorURL string
}

// AnalysisResult holds the final merged result of all analysis layers
type AnalysisResult struct {
	Intent       *IntentResult `json:"intent"`
	LayersUsed   []string      `json:"layers_used"`   // e.g., ["L1", "L2"] or ["L1", "L2", "L3"]
	Reused       bool          `json:"reused"`        // Whether the result was reused
	ReuseSource  string        `json:"reuse_source"`  // "fingerprint" or "digest"
	L1Coverage   float64       `json:"l1_coverage"`
	L2Coverage   float64       `json:"l2_coverage"`
	TotalCoverage float64      `json:"total_coverage"`
}

// NewAnalyzer creates a new intent analyzer
func NewAnalyzer(conductorURL string) *Analyzer {
	return &Analyzer{
		cmdlineExtractor: NewCmdlineExtractor(),
		configExtractor:  NewConfigExtractor(),
		modelParser:      NewModelNameParser(),
		reuseChecker:     NewReuseChecker(),
		conductorURL:     conductorURL,
	}
}

// Analyze performs layered intent analysis on the given evidence
func (a *Analyzer) Analyze(ctx context.Context, workloadUID string, evidence *IntentEvidence) (*AnalysisResult, error) {
	result := &AnalysisResult{
		Intent: &IntentResult{
			FieldSources: make(map[string]string),
		},
	}

	// Step 0: Check for reusable results
	reuseResult := a.reuseChecker.Check(ctx, workloadUID, evidence)
	if reuseResult.Found {
		log.Infof("Analyzer: reusing intent for workload %s from %s (source: %s)",
			workloadUID, reuseResult.SourceWorkloadUID, reuseResult.ReuseSource)
		result.Intent = reuseResult.Result
		result.Reused = true
		result.ReuseSource = reuseResult.ReuseSource
		return result, nil
	}

	// Step 1: L1 Cmdline Extraction
	l1Result := a.cmdlineExtractor.Extract(evidence.Command, evidence.Args)
	result.L1Coverage = l1Result.Coverage
	result.LayersUsed = append(result.LayersUsed, "L1")

	// Merge L1 into intent
	a.mergeL1(l1Result, result.Intent)

	// Step 2: L2 Config Extraction (if config files available)
	configs := a.gatherConfigs(evidence, l1Result)
	if len(configs) > 0 {
		l2Result := a.configExtractor.ExtractFromConfigs(configs)
		result.L2Coverage = l2Result.Coverage
		result.LayersUsed = append(result.LayersUsed, "L2")

		// Merge L2 into intent
		a.mergeL2(l2Result, result.Intent)
	}

	// Calculate total coverage
	result.TotalCoverage = a.calculateTotalCoverage(result.Intent)

	// Step 3: Check if L1+L2 is sufficient
	if result.TotalCoverage >= L1L2CoverageThreshold && result.Intent.Confidence >= L1L2ConfidenceThreshold {
		log.Infof("Analyzer: L1+L2 sufficient for workload %s (coverage=%.2f, confidence=%.2f)",
			workloadUID, result.TotalCoverage, result.Intent.Confidence)
		return result, nil
	}

	// Step 4: L3 - Call Conductor for LLM analysis
	if a.conductorURL != "" {
		l3Result, err := a.callConductorL3(ctx, workloadUID, evidence, result.Intent)
		if err != nil {
			log.Warnf("Analyzer: L3 call failed for workload %s: %v", workloadUID, err)
			// Continue with L1+L2 results
		} else if l3Result != nil {
			result.LayersUsed = append(result.LayersUsed, "L3")
			a.mergeL3(l3Result, result.Intent)
			result.TotalCoverage = a.calculateTotalCoverage(result.Intent)
		}
	}

	return result, nil
}

// mergeL1 merges L1 cmdline extraction results into the intent
func (a *Analyzer) mergeL1(l1 *CmdlineExtractionResult, intent *IntentResult) {
	if l1.Category != "" && intent.Category == "" {
		intent.Category = l1.Category
	}

	if l1.ServingFramework != "" {
		if intent.Inference == nil {
			intent.Inference = &InferenceDetail{}
		}
		intent.Inference.ServingFramework = l1.ServingFramework
	}

	if l1.ModelPath != "" {
		modelInfo := a.modelParser.Parse(l1.ModelPath)
		if modelInfo != nil {
			intent.Model = modelInfo
		} else {
			intent.Model = &ModelInfo{Path: l1.ModelPath}
		}
	}

	if l1.Method != "" {
		if intent.Training == nil {
			intent.Training = &TrainingDetail{}
		}
		intent.Training.Method = l1.Method
	}

	if l1.Parallelism != nil {
		if intent.Training == nil {
			intent.Training = &TrainingDetail{}
		}
		intent.Training.Parallelism = l1.Parallelism
	}

	if l1.HyperParams != nil {
		if intent.Training == nil {
			intent.Training = &TrainingDetail{}
		}
		intent.Training.HyperParams = l1.HyperParams
	}

	if l1.DataPath != "" {
		if intent.Training == nil {
			intent.Training = &TrainingDetail{}
		}
		if intent.Training.Data == nil {
			intent.Training.Data = &DataInfo{}
		}
		intent.Training.Data.Path = l1.DataPath
	}

	// Merge field sources
	for k, v := range l1.FieldSources {
		intent.FieldSources[k] = v
	}

	// Set analysis mode
	if l1.Coverage >= 0.6 {
		intent.AnalysisMode = AnalysisModeCmdlineRich
	}

	// Calculate L1 confidence
	intent.Confidence = l1.Coverage * 0.8 // L1 alone caps at 0.8
	intent.Source = IntentSourceDeterministic
}

// mergeL2 merges L2 config extraction results into the intent
func (a *Analyzer) mergeL2(l2 *ConfigExtractionResult, intent *IntentResult) {
	// L2 provides higher confidence when available
	if l2.Category != "" && intent.Category == "" {
		intent.Category = l2.Category
	}

	if l2.Model != nil && intent.Model == nil {
		intent.Model = l2.Model
	}

	if l2.Training != nil {
		if intent.Training == nil {
			intent.Training = l2.Training
		} else {
			// Merge training details (L2 takes priority for detailed fields)
			if l2.Training.Method != "" && intent.Training.Method == "" {
				intent.Training.Method = l2.Training.Method
			}
			if l2.Training.Parallelism != nil {
				intent.Training.Parallelism = l2.Training.Parallelism
			}
			if l2.Training.HyperParams != nil {
				a.mergeHyperParams(l2.Training.HyperParams, intent.Training)
			}
			if l2.Training.LoRA != nil {
				intent.Training.LoRA = l2.Training.LoRA
			}
			if l2.Training.Data != nil {
				intent.Training.Data = l2.Training.Data
			}
		}
	}

	if l2.FrameworkStack != nil {
		intent.FrameworkStack = l2.FrameworkStack
	}

	// Merge field sources
	for k, v := range l2.FieldSources {
		intent.FieldSources[k] = v
	}

	// Update analysis mode
	if l2.Coverage >= 0.4 {
		intent.AnalysisMode = AnalysisModeConfigBased
	}

	// Boost confidence with L2
	if l2.Coverage > 0 {
		intent.Confidence = clampFloat(intent.Confidence+l2.Coverage*0.3, 0, 1.0)
	}
}

// mergeL3 merges Conductor L3 results into the intent
func (a *Analyzer) mergeL3(l3 *IntentResult, intent *IntentResult) {
	if l3 == nil {
		return
	}

	// L3 (LLM) takes priority when it has higher confidence
	if l3.Category != "" && (intent.Category == "" || l3.Confidence > intent.Confidence) {
		intent.Category = l3.Category
	}

	if l3.Model != nil && (intent.Model == nil || intent.Model.Family == "") {
		intent.Model = l3.Model
	}

	if l3.Training != nil && intent.Training == nil {
		intent.Training = l3.Training
	}

	if l3.Inference != nil && intent.Inference == nil {
		intent.Inference = l3.Inference
	}

	if l3.FrameworkStack != nil {
		intent.FrameworkStack = l3.FrameworkStack
	}

	if l3.Reasoning != "" {
		intent.Reasoning = l3.Reasoning
	}

	// Merge field sources
	for k, v := range l3.FieldSources {
		intent.FieldSources[k] = v
	}

	intent.AnalysisMode = AnalysisModeCodeAnalyzed
	intent.Source = IntentSourceLLM
	intent.Confidence = clampFloat(l3.Confidence, intent.Confidence, 1.0)
}

// mergeHyperParams merges L2 hyperparams into existing training detail
func (a *Analyzer) mergeHyperParams(src *HyperParams, dst *TrainingDetail) {
	if dst.HyperParams == nil {
		dst.HyperParams = src
		return
	}
	// L2 provides more precise values
	if src.LearningRate > 0 {
		dst.HyperParams.LearningRate = src.LearningRate
	}
	if src.BatchSize > 0 {
		dst.HyperParams.BatchSize = src.BatchSize
	}
	if src.Epochs > 0 {
		dst.HyperParams.Epochs = src.Epochs
	}
	if src.GradAccum > 0 {
		dst.HyperParams.GradAccum = src.GradAccum
	}
	if src.Optimizer != "" {
		dst.HyperParams.Optimizer = src.Optimizer
	}
}

// gatherConfigs collects config file contents from evidence and L1 result
func (a *Analyzer) gatherConfigs(evidence *IntentEvidence, l1 *CmdlineExtractionResult) map[string]string {
	configs := make(map[string]string)

	// From code snapshot config files
	if evidence.CodeSnapshot != nil {
		for _, cf := range evidence.CodeSnapshot.ConfigFiles {
			if cf != nil && cf.Content != "" {
				configs[cf.Path] = cf.Content
			}
		}
	}

	return configs
}

// calculateTotalCoverage estimates how complete the analysis is
func (a *Analyzer) calculateTotalCoverage(intent *IntentResult) float64 {
	total := 6.0 // category, model, method, parallelism, hyperparams, framework_stack
	covered := 0.0

	if intent.Category != "" {
		covered++
	}
	if intent.Model != nil && intent.Model.Family != "" {
		covered++
	}
	if intent.Training != nil && intent.Training.Method != "" {
		covered++
	}
	if intent.Training != nil && intent.Training.Parallelism != nil {
		covered++
	}
	if intent.Training != nil && intent.Training.HyperParams != nil {
		covered += 0.5
	}
	if intent.FrameworkStack != nil {
		covered += 0.5
	}

	return covered / total
}

// callConductorL3 sends analysis request to Conductor
func (a *Analyzer) callConductorL3(
	ctx context.Context,
	workloadUID string,
	evidence *IntentEvidence,
	partialResult *IntentResult,
) (*IntentResult, error) {
	if a.conductorURL == "" {
		return nil, fmt.Errorf("conductor URL not configured")
	}

	// Build request
	evidenceJSON, _ := json.Marshal(evidence)
	partialJSON, _ := json.Marshal(partialResult)

	log.Infof("Analyzer: calling Conductor L3 for workload %s (evidence=%d bytes)", workloadUID, len(evidenceJSON))

	// TODO: implement HTTP call to POST /api/v1/intent/analyze
	_ = partialJSON
	_ = evidenceJSON

	return nil, fmt.Errorf("conductor L3 integration pending (M2 implementation)")
}

func clampFloat(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
