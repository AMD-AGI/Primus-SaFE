// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pipeline

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/intent"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// EvidenceEvaluator performs deterministic intent analysis from collected evidence.
// It runs two stages:
//  1. Rule matching: match against promoted IntentRules from the DB (image, cmdline, env, pip patterns)
//  2. Structural heuristics: detection-system signals, image-registry analysis, no-cmdline fallback
//
// All regex-based detection patterns are stored in the intent_rule table, NOT hardcoded.
// The combined result is returned with a confidence score.
type EvidenceEvaluator struct{}

// NewEvidenceEvaluator creates a new evaluator instance
func NewEvidenceEvaluator() *EvidenceEvaluator {
	return &EvidenceEvaluator{}
}

// Evaluate runs deterministic analysis on the provided evidence
func (e *EvidenceEvaluator) Evaluate(
	evidence *intent.IntentEvidence,
	promotedRules []*model.IntentRule,
) *intent.IntentResult {
	result := &intent.IntentResult{
		FieldSources: make(map[string]string),
		Source:       intent.IntentSourceDeterministic,
	}

	var signals []evaluationSignal

	// Stage 1: match promoted rules from DB
	ruleSignals := e.matchRules(evidence, promotedRules)
	signals = append(signals, ruleSignals...)

	// Stage 2: structural heuristics (detection signals, image registry, no-cmdline fallback)
	structuralSignals := e.matchStructuralHeuristics(evidence)
	signals = append(signals, structuralSignals...)

	if len(signals) == 0 {
		result.Confidence = 0
		return result
	}

	// Aggregate signals into final result
	e.aggregateSignals(signals, result)

	return result
}

// evaluationSignal represents a single piece of evidence pointing to an intent
type evaluationSignal struct {
	category intent.Category
	field    string // which result field this signal contributes to
	value    string // the value for this field
	weight   float64
	source   string // provenance: "rule", "image", "cmdline", "env", "code", "pip"
	ruleID   int64  // if matched from a DB rule
}

// ---------------------------------------------------------------------------
// Stage 1: Rule matching
// ---------------------------------------------------------------------------

func (e *EvidenceEvaluator) matchRules(
	evidence *intent.IntentEvidence,
	rules []*model.IntentRule,
) []evaluationSignal {
	var signals []evaluationSignal

	for _, rule := range rules {
		if rule == nil || rule.Status != "promoted" {
			continue
		}

		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			log.Warnf("Invalid regex in intent_rule %d: %s", rule.ID, rule.Pattern)
			continue
		}

		var target string
		switch rule.Dimension {
		case "image":
			target = evidence.Image
		case "cmdline":
			target = evidence.Command + " " + strings.Join(evidence.Args, " ")
		case "env_key":
			// Check if any env key matches
			for k := range evidence.Env {
				if re.MatchString(k) {
					target = k
					break
				}
			}
		case "env_value":
			for _, v := range evidence.Env {
				if re.MatchString(v) {
					target = v
					break
				}
			}
		case "pip":
			if evidence.CodeSnapshot != nil {
				target = evidence.CodeSnapshot.PipFreeze
			}
		case "code":
			if evidence.CodeSnapshot != nil && evidence.CodeSnapshot.EntryScript != nil {
				target = evidence.CodeSnapshot.EntryScript.Content
			}
		default:
			continue
		}

		if target == "" {
			continue
		}

		if re.MatchString(target) {
			signals = append(signals, evaluationSignal{
				category: intent.Category(rule.DetectsValue),
				field:    rule.DetectsField,
				value:    rule.DetectsValue,
				weight:   rule.Confidence,
				source:   "rule",
				ruleID:   rule.ID,
			})
		}
	}

	return signals
}

// ---------------------------------------------------------------------------
// Stage 2: Structural heuristics (non-regex logic)
// ---------------------------------------------------------------------------

// matchStructuralHeuristics produces signals that cannot be expressed as simple
// regex rules in the intent_rule table. These include:
//   - Detection-system mapping (framework + workload_type -> category)
//   - Image-registry package analysis (structured data, not raw text)
//   - No-cmdline fallback (absence of evidence)
func (e *EvidenceEvaluator) matchStructuralHeuristics(evidence *intent.IntentEvidence) []evaluationSignal {
	var signals []evaluationSignal

	// Image registry heuristics (structured data from registry inspection)
	if evidence.ImageRegistry != nil {
		registrySignals := e.analyzeImageRegistry(evidence.ImageRegistry)
		signals = append(signals, registrySignals...)
	}

	// Detection system signals (already confirmed by DetectionCoordinator)
	if evidence.DetectedFramework != "" && evidence.DetectedWorkloadType != "" {
		detSignals := e.mapDetectionSignals(evidence.DetectedFramework, evidence.DetectedWorkloadType)
		signals = append(signals, detSignals...)
	}

	// If no meaningful cmdline was found (empty Command after spec + process collection),
	// the workload is likely idle / interactive development: someone requested GPU resources
	// but is running ad-hoc commands manually rather than a structured job.
	if evidence.Command == "" && evidence.DetectedFramework != "" {
		signals = append(signals, evaluationSignal{
			category: intent.CategoryInteractiveDevelopment,
			field:    "category",
			value:    string(intent.CategoryInteractiveDevelopment),
			weight:   0.5,
			source:   "no_cmdline",
		})
	}

	return signals
}

// mapDetectionSignals converts already-confirmed framework/workload_type from the
// detection system into evaluation signals. These carry moderate weight because
// the detection coordinator has already validated them with process probes and log
// analysis, but they don't provide fine-grained intent (category/model) details.
func (e *EvidenceEvaluator) mapDetectionSignals(framework, workloadType string) []evaluationSignal {
	var signals []evaluationSignal

	fw := strings.ToLower(framework)
	wt := strings.ToLower(workloadType)

	// Map framework + workload_type to category
	var cat intent.Category
	switch {
	case (fw == "vllm" || fw == "sglang" || fw == "tgi") && wt == "inference":
		cat = intent.CategoryInference
	case fw == "megatron" && wt == "training":
		cat = intent.CategoryPreTraining
	case fw == "deepspeed" && wt == "training":
		cat = intent.CategoryPreTraining
	case (fw == "primus" || fw == "pytorch" || fw == "lightning") && wt == "training":
		// Could be pre-training or fine-tuning; need more evidence to distinguish
		cat = intent.CategoryFineTuning
	default:
		if wt == "training" {
			cat = intent.CategoryFineTuning
		} else if wt == "inference" {
			cat = intent.CategoryInference
		}
	}

	if cat != "" {
		signals = append(signals, evaluationSignal{
			category: cat,
			field:    "category",
			value:    string(cat),
			weight:   0.6,
			source:   "detection",
		})
	}

	return signals
}

// analyzeImageRegistry extracts signals from image registry metadata
func (e *EvidenceEvaluator) analyzeImageRegistry(reg *intent.ImageRegistryEvidence) []evaluationSignal {
	var signals []evaluationSignal

	if reg.FrameworkHints != nil {
		for key, val := range reg.FrameworkHints {
			switch key {
			case "serving_framework":
				signals = append(signals, evaluationSignal{
					category: intent.CategoryInference,
					field:    "serving_framework",
					value:    fmt.Sprint(val),
					weight:   0.5,
					source:   "image_registry",
				})
			case "training_framework":
				signals = append(signals, evaluationSignal{
					category: intent.CategoryFineTuning,
					field:    "training_framework",
					value:    fmt.Sprint(val),
					weight:   0.5,
					source:   "image_registry",
				})
			}
		}
	}

	// Analyze installed packages from layer history
	for _, pkg := range reg.InstalledPackages {
		switch pkg.Name {
		case "vllm", "text-generation-inference":
			signals = append(signals, evaluationSignal{
				category: intent.CategoryInference,
				field:    "category",
				value:    string(intent.CategoryInference),
				weight:   0.5,
				source:   "image_registry",
			})
		case "deepspeed", "megatron-core":
			signals = append(signals, evaluationSignal{
				category: intent.CategoryPreTraining,
				field:    "category",
				value:    string(intent.CategoryPreTraining),
				weight:   0.5,
				source:   "image_registry",
			})
		}
	}

	return signals
}

// ---------------------------------------------------------------------------
// Signal aggregation
// ---------------------------------------------------------------------------

func (e *EvidenceEvaluator) aggregateSignals(
	signals []evaluationSignal,
	result *intent.IntentResult,
) {
	// Score by category
	categoryScores := make(map[intent.Category]float64)
	var matchedRuleIDs []int64

	for _, sig := range signals {
		if sig.category != "" {
			categoryScores[sig.category] += sig.weight
		}
		if sig.ruleID > 0 {
			matchedRuleIDs = append(matchedRuleIDs, sig.ruleID)
		}

		// Record field sources
		if sig.field != "" && sig.source != "" {
			result.FieldSources[sig.field] = sig.source
		}
	}

	// Pick the category with the highest score
	var bestCategory intent.Category
	var bestScore float64
	var totalScore float64

	for cat, score := range categoryScores {
		totalScore += score
		if score > bestScore {
			bestScore = score
			bestCategory = cat
		}
	}

	result.Category = bestCategory
	result.MatchedRules = matchedRuleIDs

	// Normalize confidence: best score vs total, capped at 1.0
	if totalScore > 0 {
		dominance := bestScore / totalScore
		// Scale: single strong signal = ~0.6, multiple agreeing signals = ~0.9+
		result.Confidence = clamp(dominance*0.5+bestScore*0.3, 0, 1.0)
	}

	// Determine analysis mode based on what matched
	hasRuleMatch := false
	for _, sig := range signals {
		if sig.source == "rule" {
			hasRuleMatch = true
			break
		}
	}
	if hasRuleMatch {
		result.AnalysisMode = intent.AnalysisModeCmdlineRich
		result.Source = intent.IntentSourceRule
	} else {
		result.AnalysisMode = intent.AnalysisModeConfigBased
	}

	// Infer expected behavior from category
	switch bestCategory {
	case intent.CategoryPreTraining:
		result.ExpectedBehavior = intent.BehaviorLongRunning
	case intent.CategoryFineTuning:
		result.ExpectedBehavior = intent.BehaviorBatch
	case intent.CategoryInference, intent.CategoryServing:
		result.ExpectedBehavior = intent.BehaviorLongRunning
	case intent.CategoryEvaluation, intent.CategoryBenchmark:
		result.ExpectedBehavior = intent.BehaviorBatch
	case intent.CategoryDataProcessing:
		result.ExpectedBehavior = intent.BehaviorBatch
	case intent.CategoryInteractiveDevelopment:
		result.ExpectedBehavior = intent.BehaviorLongRunning
	case intent.CategoryCICD:
		result.ExpectedBehavior = intent.BehaviorBatch
	}
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

