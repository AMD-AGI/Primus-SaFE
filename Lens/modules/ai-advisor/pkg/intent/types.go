// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package intent provides types and interfaces for workload intent analysis.
// Intent analysis goes beyond framework detection to understand what workloads
// are actually doing (pre-training, fine-tuning, inference, evaluation, etc.)
// and extracts structured metadata about models, training methods, and configurations.
package intent

import "time"

// AnalysisMode indicates how the intent was determined
type AnalysisMode string

const (
	AnalysisModeCmdlineRich  AnalysisMode = "cmdline_rich"   // L1 cmdline extraction was sufficient
	AnalysisModeConfigBased  AnalysisMode = "config_based"   // L1+L2 config parsing was sufficient
	AnalysisModeCodeAnalyzed AnalysisMode = "code_analyzed"  // L3 LLM code analysis was needed
	AnalysisModeShellExpanded AnalysisMode = "shell_expanded" // Shell script was expanded before analysis
)

// IntentSource indicates the source of the intent analysis result
type IntentSource string

const (
	IntentSourceDeterministic IntentSource = "deterministic" // L1/L2 extraction only
	IntentSourceLLM           IntentSource = "llm"           // LLM analysis was used
	IntentSourceRule          IntentSource = "rule"          // Matched a distilled rule
	IntentSourceManual        IntentSource = "manual"        // Manual annotation
)

// Category represents the high-level workload intent
type Category string

const (
	CategoryPreTraining    Category = "pre_training"
	CategoryFineTuning     Category = "fine_tuning"
	CategoryInference      Category = "inference"
	CategoryEvaluation     Category = "evaluation"
	CategoryDataProcessing Category = "data_processing"
	CategoryServing        Category = "serving"
	CategoryProfiling      Category = "profiling"
)

// ExpectedBehavior represents the expected runtime behavior pattern
type ExpectedBehavior string

const (
	BehaviorLongRunning ExpectedBehavior = "long_running"
	BehaviorBatch       ExpectedBehavior = "batch"
	BehaviorPeriodic    ExpectedBehavior = "periodic"
	BehaviorElastic     ExpectedBehavior = "elastic"
)

// TrainingMethod represents specific training methodology
type TrainingMethod string

const (
	MethodPreTraining TrainingMethod = "pre_training"
	MethodSFT         TrainingMethod = "sft"
	MethodDPO         TrainingMethod = "dpo"
	MethodRLHF        TrainingMethod = "rlhf"
	MethodLoRA        TrainingMethod = "lora"
	MethodQLoRA       TrainingMethod = "qlora"
)

// ModelInfo contains parsed model metadata
type ModelInfo struct {
	Path    string `json:"path,omitempty"`    // Full model path or identifier
	Family  string `json:"family,omitempty"`  // Model family: llama, mixtral, qwen, etc.
	Scale   string `json:"scale,omitempty"`   // Parameter scale: 7B, 70B, 8x7B
	Variant string `json:"variant,omitempty"` // Variant: base, chat, instruct
}

// ParallelismConfig describes distributed training parallelism strategy
type ParallelismConfig struct {
	DataParallel   int    `json:"dp,omitempty"`         // Data parallel size
	TensorParallel int    `json:"tp,omitempty"`         // Tensor parallel size
	PipeParallel   int    `json:"pp,omitempty"`         // Pipeline parallel size
	ZeroStage      int    `json:"zero_stage,omitempty"` // DeepSpeed ZeRO stage (0-3)
	FSDP           bool   `json:"fsdp,omitempty"`       // Whether FSDP is used
	Strategy       string `json:"strategy,omitempty"`   // FSDP sharding strategy
}

// LoRAConfig describes LoRA adapter configuration
type LoRAConfig struct {
	Rank          int      `json:"rank,omitempty"`           // LoRA rank (r)
	Alpha         int      `json:"alpha,omitempty"`          // LoRA alpha
	TargetModules []string `json:"target_modules,omitempty"` // Target modules for LoRA
}

// HyperParams describes key training hyperparameters
type HyperParams struct {
	LearningRate float64 `json:"lr,omitempty"`
	Epochs       int     `json:"epochs,omitempty"`
	BatchSize    int     `json:"batch_size,omitempty"`
	Optimizer    string  `json:"optimizer,omitempty"`
	GradAccum    int     `json:"grad_accum,omitempty"` // Gradient accumulation steps
}

// DataInfo describes training data
type DataInfo struct {
	Path        string `json:"path,omitempty"`        // Data directory or file path
	DatasetName string `json:"dataset_name,omitempty"` // Dataset name (e.g., tatsu-lab/alpaca)
	FormatHint  string `json:"format_hint,omitempty"`  // Data format: instruction, completion, preference, raw_text
}

// TrainingDetail contains training-specific intent details
type TrainingDetail struct {
	Method      TrainingMethod    `json:"method,omitempty"`
	Parallelism *ParallelismConfig `json:"parallelism,omitempty"`
	Data        *DataInfo         `json:"data,omitempty"`
	LoRA        *LoRAConfig       `json:"lora_config,omitempty"`
	HyperParams *HyperParams     `json:"hyperparams,omitempty"`
}

// InferenceDetail contains inference-specific intent details
type InferenceDetail struct {
	ServingFramework string `json:"serving_framework,omitempty"` // vllm, tgi, triton, sglang, llama_cpp
	APIType          string `json:"api_type,omitempty"`          // openai, grpc, custom
	Quantization     string `json:"quantization,omitempty"`      // awq, gptq, squeezellm, none
	TensorParallel   int    `json:"tensor_parallel,omitempty"`
	MaxModelLength   int    `json:"max_model_length,omitempty"`
	Port             int    `json:"port,omitempty"`
}

// FrameworkStack describes the 3-layer framework stack
type FrameworkStack struct {
	Wrapper       string `json:"wrapper,omitempty"`       // Upper layer: primus, trl, lightning, hf_trainer
	Orchestration string `json:"orchestration,omitempty"` // Distributed: deepspeed, megatron, fsdp, colossalai
	Runtime       string `json:"runtime,omitempty"`       // Base: pytorch, jax, tensorflow
}

// IntentResult is the complete output of intent analysis for a single workload
type IntentResult struct {
	// Core classification
	Category         Category         `json:"category,omitempty"`
	ExpectedBehavior ExpectedBehavior `json:"expected_behavior,omitempty"`

	// Model information
	Model *ModelInfo `json:"model,omitempty"`

	// Framework stack
	FrameworkStack *FrameworkStack `json:"framework_stack,omitempty"`

	// Scenario-specific details (only one should be populated)
	Training  *TrainingDetail  `json:"training,omitempty"`
	Inference *InferenceDetail `json:"inference,omitempty"`

	// Analysis metadata
	Confidence   float64            `json:"confidence"`
	Source       IntentSource       `json:"source"`
	AnalysisMode AnalysisMode       `json:"analysis_mode"`
	Reasoning    string             `json:"reasoning,omitempty"`
	FieldSources map[string]string  `json:"field_sources,omitempty"` // Per-field provenance
	MatchedRules []int64            `json:"matched_rules,omitempty"` // Matched intent_rule IDs
}

// IntentEvidence is the structured evidence collected for intent analysis
type IntentEvidence struct {
	// From workload spec (always available)
	Image     string            `json:"image,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	GVK       string            `json:"gvk,omitempty"`       // GroupVersionKind
	Replicas  int               `json:"replicas,omitempty"`

	// From code snapshot (requires running pod)
	CodeSnapshot *CodeSnapshotEvidence `json:"code_snapshot,omitempty"`

	// From image registry (available anytime)
	ImageRegistry *ImageRegistryEvidence `json:"image_registry,omitempty"`
}

// CodeSnapshotEvidence contains evidence from code snapshot
type CodeSnapshotEvidence struct {
	EntryScript  *FileContent            `json:"entry_script,omitempty"`
	ConfigFiles  []*FileContent          `json:"config_files,omitempty"`
	LocalModules []*FileContent          `json:"local_modules,omitempty"`
	ImportGraph  map[string][]string     `json:"import_graph,omitempty"` // file -> imported files
	PipFreeze    string                  `json:"pip_freeze,omitempty"`
	Fingerprint  string                  `json:"fingerprint,omitempty"`
}

// FileContent represents a collected file
type FileContent struct {
	Path      string `json:"path"`
	Content   string `json:"content,omitempty"`
	Hash      string `json:"hash,omitempty"`
	Size      int    `json:"size,omitempty"`
	Truncated bool   `json:"truncated,omitempty"` // True if content was truncated due to size limit
}

// ImageRegistryEvidence contains evidence from Harbor registry
type ImageRegistryEvidence struct {
	Digest            string                 `json:"digest,omitempty"`
	BaseImage         string                 `json:"base_image,omitempty"`
	LayerHistory      []LayerInfo            `json:"layer_history,omitempty"`
	InstalledPackages []PackageInfo          `json:"installed_packages,omitempty"`
	FrameworkHints    map[string]interface{} `json:"framework_hints,omitempty"`
}

// LayerInfo describes a single image layer
type LayerInfo struct {
	CreatedBy string `json:"created_by,omitempty"` // Dockerfile instruction
	Size      int64  `json:"size,omitempty"`
	Comment   string `json:"comment,omitempty"`
}

// PackageInfo describes an installed package found in layer history
type PackageInfo struct {
	Manager string `json:"manager,omitempty"` // pip, apt, conda
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// CandidateRule represents a rule proposed by the LLM distillation process
type CandidateRule struct {
	DetectsField string  `json:"detects_field"` // category, model_family, training_method, etc.
	DetectsValue string  `json:"detects_value"` // The value this rule detects
	Dimension    string  `json:"dimension"`     // image, cmdline, env_key, env_value, config, code
	Pattern      string  `json:"pattern"`       // Regex pattern
	Confidence   float64 `json:"confidence"`    // Expected confidence when matched
	Reasoning    string  `json:"reasoning"`     // LLM explanation for why this rule works
}

// BacktestResult contains the metrics from backtesting a candidate rule
type BacktestResult struct {
	Precision    float64 `json:"precision"`
	Recall       float64 `json:"recall"`
	F1           float64 `json:"f1"`
	TruePositive int     `json:"tp"`
	FalsePositive int    `json:"fp"`
	FalseNegative int    `json:"fn"`
	SampleCount  int     `json:"sample_count"`
	BacktestedAt time.Time `json:"backtested_at"`
}

// AuditResult represents the outcome of a sampling audit on a single workload
type AuditResult struct {
	WorkloadUID       string       `json:"workload_uid"`
	Consistent        bool         `json:"consistent"`         // Whether rule result matches LLM deep analysis
	LLMResult         *IntentResult `json:"llm_result,omitempty"` // LLM deep analysis result
	DiscrepancyFields []string     `json:"discrepancy_fields,omitempty"` // Fields that differ
	Explanation       string       `json:"explanation,omitempty"`
}
