// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package intent

import (
	"regexp"
	"strings"
)

// CmdlineExtractor is the Layer 1 deterministic extractor that operates on
// the raw command line (entrypoint + args) of a workload. It uses regex
// patterns to extract:
//   - Serving/inference framework
//   - Model path and identity
//   - Training method (SFT, DPO, LoRA, pre-training, etc.)
//   - Parallelism hints (world size, tensor parallel, etc.)
//   - Hyperparameter hints (batch size, learning rate, epochs)
//
// L1 extraction is zero-cost (pure regex, <1ms) and covers ~30% of workloads
// fully and ~25% partially.
type CmdlineExtractor struct {
	servingPatterns  []servingPattern
	modelPathRules   []modelPathRule
	trainingPatterns []trainingMethodPattern
	parallelRules    []parallelismRule
	hyperparamRules  []hyperparamRule
	scriptNameRules  []scriptNameRule
}

type servingPattern struct {
	re        *regexp.Regexp
	framework string
}

type modelPathRule struct {
	re    *regexp.Regexp
	label string // which cmdline arg this matches (--model, --model-id, etc.)
}

type trainingMethodPattern struct {
	re     *regexp.Regexp
	method TrainingMethod
}

type parallelismRule struct {
	re    *regexp.Regexp
	field string // dp, tp, pp, zero_stage, etc.
}

type hyperparamRule struct {
	re    *regexp.Regexp
	field string // lr, batch_size, epochs, etc.
}

type scriptNameRule struct {
	re       *regexp.Regexp
	category Category
	method   TrainingMethod
}

// CmdlineExtractionResult holds the result of L1 extraction
type CmdlineExtractionResult struct {
	// Serving framework detection
	ServingFramework string  `json:"serving_framework,omitempty"`

	// Model information
	ModelPath string `json:"model_path,omitempty"`

	// Training method
	Method TrainingMethod `json:"method,omitempty"`

	// Category inference
	Category Category `json:"category,omitempty"`

	// Parallelism hints from cmdline
	Parallelism *ParallelismConfig `json:"parallelism,omitempty"`

	// Hyperparameter hints from cmdline
	HyperParams *HyperParams `json:"hyperparams,omitempty"`

	// Additional extracted fields
	ConfigPaths  []string `json:"config_paths,omitempty"`
	DataPath     string   `json:"data_path,omitempty"`
	OutputDir    string   `json:"output_dir,omitempty"`

	// Provenance tracking
	FieldSources map[string]string `json:"field_sources,omitempty"`

	// Confidence: how much of the intent could be determined from cmdline alone
	Coverage float64 `json:"coverage"`
}

// NewCmdlineExtractor creates a new L1 extractor with all built-in patterns
func NewCmdlineExtractor() *CmdlineExtractor {
	e := &CmdlineExtractor{}
	e.initPatterns()
	return e
}

// Extract performs L1 extraction on a command line
func (e *CmdlineExtractor) Extract(cmdline string, args []string) *CmdlineExtractionResult {
	result := &CmdlineExtractionResult{
		FieldSources: make(map[string]string),
	}

	// Build full command string
	fullCmd := cmdline
	if len(args) > 0 {
		fullCmd = cmdline + " " + strings.Join(args, " ")
	}
	fullCmd = strings.TrimSpace(fullCmd)

	if fullCmd == "" {
		return result
	}

	// 1. Detect serving framework
	e.extractServingFramework(fullCmd, result)

	// 2. Extract model path
	e.extractModelPath(fullCmd, result)

	// 3. Detect training method
	e.extractTrainingMethod(fullCmd, result)

	// 4. Detect category from script name
	e.extractFromScriptName(fullCmd, result)

	// 5. Extract parallelism hints
	e.extractParallelism(fullCmd, result)

	// 6. Extract hyperparameter hints
	e.extractHyperParams(fullCmd, result)

	// 7. Extract config file paths
	e.extractConfigPaths(fullCmd, result)

	// 8. Extract data/output paths
	e.extractPaths(fullCmd, result)

	// Infer category if not already set
	e.inferCategory(result)

	// Calculate coverage
	result.Coverage = e.calculateCoverage(result)

	return result
}

// ---------------------------------------------------------------------------
// Extraction methods
// ---------------------------------------------------------------------------

func (e *CmdlineExtractor) extractServingFramework(cmd string, result *CmdlineExtractionResult) {
	for _, p := range e.servingPatterns {
		if p.re.MatchString(cmd) {
			result.ServingFramework = p.framework
			result.FieldSources["serving_framework"] = "cmdline:L1"
			return
		}
	}
}

func (e *CmdlineExtractor) extractModelPath(cmd string, result *CmdlineExtractionResult) {
	for _, rule := range e.modelPathRules {
		matches := rule.re.FindStringSubmatch(cmd)
		if len(matches) > 1 {
			result.ModelPath = matches[1]
			result.FieldSources["model_path"] = "cmdline:L1:" + rule.label
			return
		}
	}
}

func (e *CmdlineExtractor) extractTrainingMethod(cmd string, result *CmdlineExtractionResult) {
	for _, p := range e.trainingPatterns {
		if p.re.MatchString(cmd) {
			result.Method = p.method
			result.FieldSources["training_method"] = "cmdline:L1"
			return
		}
	}
}

func (e *CmdlineExtractor) extractFromScriptName(cmd string, result *CmdlineExtractionResult) {
	for _, rule := range e.scriptNameRules {
		if rule.re.MatchString(cmd) {
			if result.Category == "" {
				result.Category = rule.category
				result.FieldSources["category"] = "cmdline:L1:script_name"
			}
			if result.Method == "" && rule.method != "" {
				result.Method = rule.method
				result.FieldSources["training_method"] = "cmdline:L1:script_name"
			}
			return
		}
	}
}

func (e *CmdlineExtractor) extractParallelism(cmd string, result *CmdlineExtractionResult) {
	p := &ParallelismConfig{}
	found := false

	for _, rule := range e.parallelRules {
		matches := rule.re.FindStringSubmatch(cmd)
		if len(matches) > 1 {
			val := parseIntFromString(matches[1])
			if val > 0 {
				found = true
				switch rule.field {
				case "tp":
					p.TensorParallel = val
				case "pp":
					p.PipeParallel = val
				case "dp":
					p.DataParallel = val
				case "world_size":
					// world_size is total, don't set directly
				case "zero_stage":
					p.ZeroStage = val
				}
				result.FieldSources["parallelism."+rule.field] = "cmdline:L1"
			}
		}
	}

	// FSDP detection
	if regexp.MustCompile(`(?i)--fsdp\b|--fsdp_config`).MatchString(cmd) {
		p.FSDP = true
		found = true
		result.FieldSources["parallelism.fsdp"] = "cmdline:L1"
	}

	if found {
		result.Parallelism = p
	}
}

func (e *CmdlineExtractor) extractHyperParams(cmd string, result *CmdlineExtractionResult) {
	hp := &HyperParams{}
	found := false

	for _, rule := range e.hyperparamRules {
		matches := rule.re.FindStringSubmatch(cmd)
		if len(matches) > 1 {
			found = true
			switch rule.field {
			case "lr":
				hp.LearningRate = parseFloatFromString(matches[1])
			case "batch_size":
				hp.BatchSize = parseIntFromString(matches[1])
			case "epochs":
				hp.Epochs = parseIntFromString(matches[1])
			case "grad_accum":
				hp.GradAccum = parseIntFromString(matches[1])
			case "optimizer":
				hp.Optimizer = matches[1]
			}
			result.FieldSources["hyperparams."+rule.field] = "cmdline:L1"
		}
	}

	if found {
		result.HyperParams = hp
	}
}

func (e *CmdlineExtractor) extractConfigPaths(cmd string, result *CmdlineExtractionResult) {
	configArgRe := regexp.MustCompile(`(?:--config(?:_file)?|--ds_config|--deepspeed(?:_config)?|--training_args|--model_config|--fsdp_config)\s+(\S+)`)
	matches := configArgRe.FindAllStringSubmatch(cmd, -1)
	for _, m := range matches {
		if len(m) > 1 {
			result.ConfigPaths = append(result.ConfigPaths, m[1])
		}
	}
}

func (e *CmdlineExtractor) extractPaths(cmd string, result *CmdlineExtractionResult) {
	// Data path
	dataPathRe := regexp.MustCompile(`(?:--data_path|--dataset|--data_dir|--train_file)\s+(\S+)`)
	if m := dataPathRe.FindStringSubmatch(cmd); len(m) > 1 {
		result.DataPath = m[1]
		result.FieldSources["data_path"] = "cmdline:L1"
	}

	// Output dir
	outputRe := regexp.MustCompile(`(?:--output_dir|--save_dir|--checkpoint_dir)\s+(\S+)`)
	if m := outputRe.FindStringSubmatch(cmd); len(m) > 1 {
		result.OutputDir = m[1]
		result.FieldSources["output_dir"] = "cmdline:L1"
	}
}

func (e *CmdlineExtractor) inferCategory(result *CmdlineExtractionResult) {
	if result.Category != "" {
		return
	}

	// Infer from serving framework
	if result.ServingFramework != "" {
		result.Category = CategoryInference
		result.FieldSources["category"] = "cmdline:L1:inferred_from_serving"
		return
	}

	// Infer from training method
	switch result.Method {
	case MethodPreTraining:
		result.Category = CategoryPreTraining
	case MethodSFT, MethodDPO, MethodRLHF, MethodLoRA, MethodQLoRA:
		result.Category = CategoryFineTuning
	}
	if result.Category != "" {
		result.FieldSources["category"] = "cmdline:L1:inferred_from_method"
	}
}

func (e *CmdlineExtractor) calculateCoverage(result *CmdlineExtractionResult) float64 {
	// Score: how many core fields were extracted
	total := 5.0 // category, model, method, parallelism, hyperparams
	extracted := 0.0

	if result.Category != "" {
		extracted++
	}
	if result.ModelPath != "" {
		extracted++
	}
	if result.Method != "" {
		extracted++
	}
	if result.Parallelism != nil {
		extracted++
	}
	if result.HyperParams != nil {
		extracted += 0.5 // partial
	}

	return extracted / total
}

// ---------------------------------------------------------------------------
// Pattern initialization
// ---------------------------------------------------------------------------

func (e *CmdlineExtractor) initPatterns() {
	// Serving framework detection
	e.servingPatterns = []servingPattern{
		{re: regexp.MustCompile(`(?i)vllm\.entrypoints|python\s+-m\s+vllm`), framework: "vllm"},
		{re: regexp.MustCompile(`(?i)text-generation-launcher`), framework: "tgi"},
		{re: regexp.MustCompile(`(?i)tritonserver`), framework: "triton"},
		{re: regexp.MustCompile(`(?i)sglang\.launch_server`), framework: "sglang"},
		{re: regexp.MustCompile(`(?i)llama-server`), framework: "llama_cpp"},
		{re: regexp.MustCompile(`(?i)torchserve`), framework: "torchserve"},
	}

	// Model path extraction (ordered by specificity)
	e.modelPathRules = []modelPathRule{
		{re: regexp.MustCompile(`--model_name_or_path\s+(\S+)`), label: "model_name_or_path"},
		{re: regexp.MustCompile(`--model-id\s+(\S+)`), label: "model-id"},
		{re: regexp.MustCompile(`--model-path\s+(\S+)`), label: "model-path"},
		{re: regexp.MustCompile(`--model\s+(\S+)`), label: "model"},
		{re: regexp.MustCompile(`--tokenizer\s+(\S+)`), label: "tokenizer"},
		{re: regexp.MustCompile(`-m\s+(\S+\.gguf)`), label: "-m_gguf"},
	}

	// Training method detection (ordered by specificity)
	e.trainingPatterns = []trainingMethodPattern{
		// LoRA / QLoRA (most specific first)
		{re: regexp.MustCompile(`(?i)--use_qlora|--qlora|--bits\s+4`), method: MethodQLoRA},
		{re: regexp.MustCompile(`(?i)--lora_r\s+\d+|--use_lora|--use_peft|--peft_type`), method: MethodLoRA},

		// DPO / RLHF
		{re: regexp.MustCompile(`(?i)--dpo_beta|dpo_trainer`), method: MethodDPO},
		{re: regexp.MustCompile(`(?i)--reward_model|rlhf_trainer`), method: MethodRLHF},

		// SFT
		{re: regexp.MustCompile(`(?i)sft_trainer|--do_train\b`), method: MethodSFT},

		// Pre-training
		{re: regexp.MustCompile(`(?i)--do_pretrain|pretrain_gpt|megatron.*pretrain`), method: MethodPreTraining},
	}

	// Script name rules
	e.scriptNameRules = []scriptNameRule{
		{re: regexp.MustCompile(`(?i)pretrain[_.]`), category: CategoryPreTraining, method: MethodPreTraining},
		{re: regexp.MustCompile(`(?i)sft[_.]|supervised_fine`), category: CategoryFineTuning, method: MethodSFT},
		{re: regexp.MustCompile(`(?i)dpo[_.]`), category: CategoryFineTuning, method: MethodDPO},
		{re: regexp.MustCompile(`(?i)rlhf[_.]`), category: CategoryFineTuning, method: MethodRLHF},
		{re: regexp.MustCompile(`(?i)lora[_.]|qlora[_.]`), category: CategoryFineTuning, method: MethodLoRA},
		{re: regexp.MustCompile(`(?i)eval[_.]|evaluate[_.]|benchmark`), category: CategoryEvaluation},
		{re: regexp.MustCompile(`(?i)serve[_.]|server[_.]|api[_.]`), category: CategoryServing},
		{re: regexp.MustCompile(`(?i)tokenize|preprocess|data_prep`), category: CategoryDataProcessing},
	}

	// Parallelism hints
	e.parallelRules = []parallelismRule{
		{re: regexp.MustCompile(`--tensor[_-]?model[_-]?parallel[_-]?size\s+(\d+)`), field: "tp"},
		{re: regexp.MustCompile(`--tp\s+(\d+)`), field: "tp"},
		{re: regexp.MustCompile(`--pipeline[_-]?model[_-]?parallel[_-]?size\s+(\d+)`), field: "pp"},
		{re: regexp.MustCompile(`--pp\s+(\d+)`), field: "pp"},
		{re: regexp.MustCompile(`--nproc[_-]per[_-]node\s+(\d+)`), field: "dp"},
		{re: regexp.MustCompile(`--zero[_-]?stage\s+(\d+)`), field: "zero_stage"},
		{re: regexp.MustCompile(`WORLD_SIZE=(\d+)`), field: "world_size"},
	}

	// Hyperparameter hints
	e.hyperparamRules = []hyperparamRule{
		{re: regexp.MustCompile(`--learning[_-]?rate\s+([\d.eE+-]+)`), field: "lr"},
		{re: regexp.MustCompile(`--lr\s+([\d.eE+-]+)`), field: "lr"},
		{re: regexp.MustCompile(`--per[_-]device[_-]train[_-]batch[_-]size\s+(\d+)`), field: "batch_size"},
		{re: regexp.MustCompile(`--batch[_-]?size\s+(\d+)`), field: "batch_size"},
		{re: regexp.MustCompile(`--num[_-]train[_-]epochs?\s+(\d+)`), field: "epochs"},
		{re: regexp.MustCompile(`--epochs?\s+(\d+)`), field: "epochs"},
		{re: regexp.MustCompile(`--gradient[_-]accumulation[_-]steps?\s+(\d+)`), field: "grad_accum"},
		{re: regexp.MustCompile(`--optim(?:izer)?\s+(\S+)`), field: "optimizer"},
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseIntFromString(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		} else {
			break
		}
	}
	return n
}

func parseFloatFromString(s string) float64 {
	// Simple float parser for common patterns like "2e-5", "0.001", "1.5e-4"
	var result float64
	var decimal float64
	var exponent int
	var expSign float64 = 1
	inDecimal := false
	inExponent := false
	decimalPlace := 0.1

	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			if inExponent {
				exponent = exponent*10 + int(c-'0')
			} else if inDecimal {
				result += float64(c-'0') * decimalPlace
				decimalPlace *= 0.1
			} else {
				result = result*10 + float64(c-'0')
			}
		case c == '.':
			inDecimal = true
		case c == 'e' || c == 'E':
			inExponent = true
		case c == '-' && inExponent:
			expSign = -1
		case c == '+' && inExponent:
			// positive exponent, default
		default:
			break
		}
	}

	if inExponent {
		_ = decimal
		exp := expSign * float64(exponent)
		multiplier := 1.0
		if exp > 0 {
			for i := 0; i < int(exp); i++ {
				multiplier *= 10
			}
		} else {
			for i := 0; i < int(-exp); i++ {
				multiplier /= 10
			}
		}
		result *= multiplier
	}

	return result
}
