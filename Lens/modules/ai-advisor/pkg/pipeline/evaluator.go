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
//  1. Rule matching: match against promoted IntentRules from the DB
//  2. Heuristic scoring: pattern-based scoring on command lines, images, environment, etc.
//
// The combined result is returned with a confidence score.
type EvidenceEvaluator struct {
	// Pre-compiled regex patterns for common framework indicators
	imagePatterns   []imagePattern
	cmdlinePatterns []cmdlinePattern
	envPatterns     []envPattern
}

type imagePattern struct {
	re       *regexp.Regexp
	category intent.Category
	detail   string
	weight   float64
}

type cmdlinePattern struct {
	re       *regexp.Regexp
	category intent.Category
	detail   string
	weight   float64
}

type envPattern struct {
	key    string // exact match on env key
	re     *regexp.Regexp
	detail string
	weight float64
}

// NewEvidenceEvaluator creates an evaluator with pre-compiled heuristic patterns
func NewEvidenceEvaluator() *EvidenceEvaluator {
	e := &EvidenceEvaluator{}
	e.initPatterns()
	return e
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

	// Stage 2: heuristic patterns
	heuristicSignals := e.matchHeuristics(evidence)
	signals = append(signals, heuristicSignals...)

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
// Stage 2: Heuristic patterns
// ---------------------------------------------------------------------------

func (e *EvidenceEvaluator) matchHeuristics(evidence *intent.IntentEvidence) []evaluationSignal {
	var signals []evaluationSignal

	// Image-based heuristics
	if evidence.Image != "" {
		for _, p := range e.imagePatterns {
			if p.re.MatchString(evidence.Image) {
				signals = append(signals, evaluationSignal{
					category: p.category,
					field:    "category",
					value:    string(p.category),
					weight:   p.weight,
					source:   "image",
				})
			}
		}
	}

	// Command-line heuristics
	fullCmd := evidence.Command + " " + strings.Join(evidence.Args, " ")
	if fullCmd != " " {
		for _, p := range e.cmdlinePatterns {
			if p.re.MatchString(fullCmd) {
				signals = append(signals, evaluationSignal{
					category: p.category,
					field:    "category",
					value:    string(p.category),
					weight:   p.weight,
					source:   "cmdline",
				})
			}
		}
	}

	// Environment variable heuristics
	for k, v := range evidence.Env {
		for _, p := range e.envPatterns {
			if p.key != "" && k == p.key {
				signals = append(signals, evaluationSignal{
					category: "",
					field:    p.detail,
					value:    v,
					weight:   p.weight,
					source:   "env",
				})
			}
			if p.re != nil && p.re.MatchString(k+"="+v) {
				signals = append(signals, evaluationSignal{
					category: "",
					field:    p.detail,
					value:    v,
					weight:   p.weight,
					source:   "env",
				})
			}
		}
	}

	// Pip freeze heuristics (from code snapshot)
	if evidence.CodeSnapshot != nil && evidence.CodeSnapshot.PipFreeze != "" {
		pipSignals := e.analyzePipFreeze(evidence.CodeSnapshot.PipFreeze)
		signals = append(signals, pipSignals...)
	}

	// Image registry heuristics
	if evidence.ImageRegistry != nil {
		registrySignals := e.analyzeImageRegistry(evidence.ImageRegistry)
		signals = append(signals, registrySignals...)
	}

	// Detection system signals (already confirmed by DetectionCoordinator)
	if evidence.DetectedFramework != "" && evidence.DetectedWorkloadType != "" {
		detSignals := e.mapDetectionSignals(evidence.DetectedFramework, evidence.DetectedWorkloadType)
		signals = append(signals, detSignals...)
	}

	return signals
}

// analyzePipFreeze extracts framework information from pip freeze output
func (e *EvidenceEvaluator) analyzePipFreeze(pipFreeze string) []evaluationSignal {
	var signals []evaluationSignal

	pipLines := strings.Split(pipFreeze, "\n")

	frameworkPackages := map[string]struct {
		category intent.Category
		weight   float64
	}{
		"transformers":  {intent.CategoryFineTuning, 0.3},
		"trl":           {intent.CategoryFineTuning, 0.6},
		"peft":          {intent.CategoryFineTuning, 0.5},
		"vllm":          {intent.CategoryInference, 0.7},
		"text-generation-inference": {intent.CategoryInference, 0.7},
		"sglang":        {intent.CategoryInference, 0.6},
		"triton":        {intent.CategoryInference, 0.4},
		"deepspeed":     {intent.CategoryPreTraining, 0.4},
		"megatron-core": {intent.CategoryPreTraining, 0.6},
		"lightning":     {intent.CategoryFineTuning, 0.3},
		"evaluate":      {intent.CategoryEvaluation, 0.3},
		"lm-eval-harness": {intent.CategoryEvaluation, 0.6},
	}

	for _, line := range pipLines {
		line = strings.TrimSpace(line)
		parts := strings.SplitN(line, "==", 2)
		if len(parts) == 0 {
			continue
		}
		pkgName := strings.TrimSpace(parts[0])
		if info, ok := frameworkPackages[pkgName]; ok {
			signals = append(signals, evaluationSignal{
				category: info.category,
				field:    "category",
				value:    string(info.category),
				weight:   info.weight,
				source:   "pip",
			})
		}
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
	hasCmdlineMatch := false
	for _, sig := range signals {
		if sig.source == "rule" {
			hasRuleMatch = true
		}
		if sig.source == "cmdline" {
			hasCmdlineMatch = true
		}
	}
	if hasRuleMatch {
		result.AnalysisMode = intent.AnalysisModeCmdlineRich
		result.Source = intent.IntentSourceRule
	} else if hasCmdlineMatch {
		result.AnalysisMode = intent.AnalysisModeCmdlineRich
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
	case intent.CategoryEvaluation:
		result.ExpectedBehavior = intent.BehaviorBatch
	case intent.CategoryDataProcessing:
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

// ---------------------------------------------------------------------------
// Pattern initialization
// ---------------------------------------------------------------------------

func (e *EvidenceEvaluator) initPatterns() {
	// Image patterns
	e.imagePatterns = []imagePattern{
		// Inference / serving
		{re: regexp.MustCompile(`(?i)vllm`), category: intent.CategoryInference, detail: "vllm", weight: 0.7},
		{re: regexp.MustCompile(`(?i)text-generation-inference|tgi`), category: intent.CategoryInference, detail: "tgi", weight: 0.7},
		{re: regexp.MustCompile(`(?i)sglang`), category: intent.CategoryInference, detail: "sglang", weight: 0.6},
		{re: regexp.MustCompile(`(?i)triton.?server`), category: intent.CategoryInference, detail: "triton", weight: 0.6},
		{re: regexp.MustCompile(`(?i)torchserve`), category: intent.CategoryServing, detail: "torchserve", weight: 0.5},

		// Training
		{re: regexp.MustCompile(`(?i)megatron`), category: intent.CategoryPreTraining, detail: "megatron", weight: 0.6},
		{re: regexp.MustCompile(`(?i)nemo.*training`), category: intent.CategoryPreTraining, detail: "nemo", weight: 0.5},
		{re: regexp.MustCompile(`(?i)deepspeed`), category: intent.CategoryPreTraining, detail: "deepspeed", weight: 0.4},

		// Evaluation
		{re: regexp.MustCompile(`(?i)lm.?eval`), category: intent.CategoryEvaluation, detail: "lm_eval", weight: 0.7},
	}

	// Command-line patterns
	e.cmdlinePatterns = []cmdlinePattern{
		// Serving frameworks
		{re: regexp.MustCompile(`(?i)vllm\.entrypoints|python\s+-m\s+vllm`), category: intent.CategoryInference, weight: 0.8},
		{re: regexp.MustCompile(`(?i)text-generation-launcher`), category: intent.CategoryInference, weight: 0.8},
		{re: regexp.MustCompile(`(?i)sglang\.launch_server`), category: intent.CategoryInference, weight: 0.7},

		// Training frameworks
		{re: regexp.MustCompile(`(?i)torchrun|torch\.distributed\.launch`), category: intent.CategoryPreTraining, weight: 0.4},
		{re: regexp.MustCompile(`(?i)deepspeed\s`), category: intent.CategoryPreTraining, weight: 0.5},
		{re: regexp.MustCompile(`(?i)accelerate\s+launch`), category: intent.CategoryFineTuning, weight: 0.4},
		{re: regexp.MustCompile(`(?i)--do_train|--training_args`), category: intent.CategoryFineTuning, weight: 0.6},
		{re: regexp.MustCompile(`(?i)--do_eval\b`), category: intent.CategoryEvaluation, weight: 0.5},

		// Megatron-style commands (megatron pt, megatron sft, etc.)
		{re: regexp.MustCompile(`(?i)\bmegatron\s+(pt|pretrain|sft|finetune|train)\b`), category: intent.CategoryPreTraining, weight: 0.7},
		{re: regexp.MustCompile(`(?i)megatron.*--model\b`), category: intent.CategoryPreTraining, weight: 0.5},

		// Primus CLI (primus/cli/main.py train pretrain, etc.)
		{re: regexp.MustCompile(`(?i)primus/cli/main\.py\s+train\s+pretrain`), category: intent.CategoryPreTraining, weight: 0.7},
		{re: regexp.MustCompile(`(?i)primus/cli/main\.py\s+train\s+sft`), category: intent.CategoryFineTuning, weight: 0.7},
		{re: regexp.MustCompile(`(?i)primus/cli/main\.py\s+train`), category: intent.CategoryFineTuning, weight: 0.5},

		// ms-swift (swift sft, swift pt, swift infer, etc.)
		{re: regexp.MustCompile(`(?i)\bswift\s+(sft|pt|pretrain|finetune)\b`), category: intent.CategoryFineTuning, weight: 0.6},
		{re: regexp.MustCompile(`(?i)\bswift\s+infer\b`), category: intent.CategoryInference, weight: 0.6},

		// Fine-tuning indicators
		{re: regexp.MustCompile(`(?i)--lora_r|--use_peft|--peft_type`), category: intent.CategoryFineTuning, weight: 0.7},
		{re: regexp.MustCompile(`(?i)sft_trainer|dpo_trainer|rlhf`), category: intent.CategoryFineTuning, weight: 0.7},

		// HuggingFace training arguments
		{re: regexp.MustCompile(`(?i)--num_train_epochs|--per_device_train_batch_size`), category: intent.CategoryFineTuning, weight: 0.5},
		{re: regexp.MustCompile(`(?i)--gradient_accumulation_steps|--warmup_steps`), category: intent.CategoryFineTuning, weight: 0.4},

		// Evaluation
		{re: regexp.MustCompile(`(?i)lm_eval|lm-eval|evaluate\s+--model`), category: intent.CategoryEvaluation, weight: 0.7},

		// Data processing
		{re: regexp.MustCompile(`(?i)tokenize|preprocess.*dataset|data.*pipeline`), category: intent.CategoryDataProcessing, weight: 0.4},

		// Benchmark / profiling
		{re: regexp.MustCompile(`(?i)\bbench\b.*test|benchmark|superbench`), category: intent.CategoryEvaluation, weight: 0.4},
		{re: regexp.MustCompile(`(?i)primusbench|nccl.?test`), category: intent.CategoryEvaluation, weight: 0.5},
	}

	// Environment variable patterns
	e.envPatterns = []envPattern{
		{key: "MASTER_ADDR", detail: "parallelism", weight: 0.3},
		{key: "WORLD_SIZE", detail: "parallelism", weight: 0.3},
		{key: "NCCL_DEBUG", detail: "nccl", weight: 0.2},
		{key: "DEEPSPEED_ZERO_STAGE", detail: "deepspeed_zero", weight: 0.5},
	}
}
