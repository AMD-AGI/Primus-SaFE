/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

// ==================== SFT Training Types ====================

// CreateSftJobRequest represents the request to create an SFT training job.
type CreateSftJobRequest struct {
	// Required fields
	DisplayName string `json:"displayName" binding:"required"`
	Workspace   string `json:"workspace" binding:"required"`
	ModelSource string `json:"modelSource" binding:"required"` // "model_square" | "huggingface"
	DatasetId   string `json:"datasetId" binding:"required"`

	// Required conditionally: modelSource=model_square
	ModelId string `json:"modelId"`
	// Required conditionally: modelSource=huggingface
	HfModelName string `json:"hfModelName"`

	// Training configuration (all optional, smart defaults applied by backend)
	TrainConfig SftTrainConfig `json:"trainConfig"`

	// Resource configuration (optional, has defaults)
	Image            string            `json:"image"`            // default "rocm/primus:v26.1"
	NodeCount        int               `json:"nodeCount"`        // number of nodes, default 1 (single-node), >1 for multi-node
	GpuCount         int               `json:"gpuCount"`         // GPUs per node, default 8
	Cpu              string            `json:"cpu"`              // CPU per node, default "128"
	Memory           string            `json:"memory"`           // memory per node, default "1024Gi"
	EphemeralStorage string            `json:"ephemeralStorage"` // default "300Gi"
	Env              map[string]string `json:"env"`
	Hostpath         []string          `json:"hostpath"`
	Priority         int               `json:"priority"` // 0-2, default 0
	Timeout          int               `json:"timeout"`  // seconds
	SecretIds        []string          `json:"secretIds"`
	Description      string            `json:"description"`
}

// SftTrainConfig holds SFT-specific training hyperparameters.
// All fields are optional; the backend fills defaults based on model size and peft type.
type SftTrainConfig struct {
	Peft          string `json:"peft"`          // "none" | "lora", default "none"
	DatasetFormat string `json:"datasetFormat"` // "alpaca" | "squad", default "alpaca"

	TrainIters      int     `json:"trainIters"`
	GlobalBatchSize int     `json:"globalBatchSize"`
	MicroBatchSize  int     `json:"microBatchSize"`
	SeqLength       int     `json:"seqLength"`
	FinetuneLr      float64 `json:"finetuneLr"`
	MinLr           float64 `json:"minLr"`           // default 0.0
	LrWarmupIters   int     `json:"lrWarmupIters"`   // default 50
	EvalInterval    int     `json:"evalInterval"`    // default 30
	SaveInterval    int     `json:"saveInterval"`    // default 50
	PrecisionConfig string  `json:"precisionConfig"` // default "bf16_mixed"

	TensorModelParallelSize   int  `json:"tensorModelParallelSize"`
	PipelineModelParallelSize int  `json:"pipelineModelParallelSize"` // default 1
	ContextParallelSize       int  `json:"contextParallelSize"`       // default 1
	SequenceParallel          bool `json:"sequenceParallel"`          // default false

	// LoRA-specific (only used when peft="lora")
	PeftDim        int  `json:"peftDim"`        // default 16
	PeftAlpha      int  `json:"peftAlpha"`      // default 32
	PackedSequence bool `json:"packedSequence"` // default false
}

// ==================== SFT Response Types ====================

// SftJobResponse is the summary returned in list endpoints.
type SftJobResponse struct {
	WorkloadId      string `json:"workloadId"`
	DisplayName     string `json:"displayName"`
	Phase           string `json:"phase"`
	ModelSource     string `json:"modelSource"`
	ModelName       string `json:"modelName"`
	BaseModelId     string `json:"baseModelId,omitempty"`
	DatasetId       string `json:"datasetId"`
	Peft            string `json:"peft"`
	ExportedModelId string `json:"exportedModelId,omitempty"`
	CreatedAt       string `json:"createdAt"`
	Duration        string `json:"duration,omitempty"`
}

// SftJobDetailResponse is the full detail returned by GetSftJob.
type SftJobDetailResponse struct {
	SftJobResponse
	Image           string              `json:"image"`
	Env             map[string]string   `json:"env,omitempty"`
	Resource        interface{}         `json:"resource,omitempty"`
	Pods            interface{}         `json:"pods,omitempty"`
	Conditions      interface{}         `json:"conditions,omitempty"`
	EntryPoint      string              `json:"entryPoint,omitempty"`
	OutputPath      string              `json:"outputPath"`
	TrainConfig     SftTrainConfig      `json:"trainConfig"`
	TrainingMetrics *SftTrainingMetrics `json:"trainingMetrics,omitempty"`
}

// SftTrainingMetrics holds real-time training metrics from Lens.
type SftTrainingMetrics struct {
	CurrentIter    int     `json:"currentIter,omitempty"`
	TotalIters     int     `json:"totalIters"`
	TrainingLoss   float64 `json:"trainingLoss,omitempty"`
	ValidationLoss float64 `json:"validationLoss,omitempty"`
	Tflops         float64 `json:"tflops,omitempty"`
	IterTime       float64 `json:"iterTime,omitempty"` // ms per iteration
	Progress       float64 `json:"progress"`           // currentIter / totalIters
}

// ListSftJobsRequest represents query parameters for listing SFT jobs.
type ListSftJobsRequest struct {
	Workspace string `form:"workspace"`
	Phase     string `form:"phase"`
	Limit     int    `form:"limit,default=50"`
	Offset    int    `form:"offset,default=0"`
}

// ListSftJobsResponse is the paginated list response.
type ListSftJobsResponse struct {
	Items      []SftJobResponse `json:"items"`
	TotalCount int              `json:"totalCount"`
}

// ==================== SFT Label Constants ====================

const (
	SftLabelWorkloadType = "safe/workload-type"
	SftLabelModel        = "safe/sft-model"
	SftLabelDataset      = "safe/sft-dataset"
	SftLabelPeft         = "safe/sft-peft"
	SftLabelBaseModelId  = "safe/sft-base-model-id"

	SftWorkloadTypeValue = "sft"
)
