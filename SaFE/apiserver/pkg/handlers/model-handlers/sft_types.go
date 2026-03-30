/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

// ==================== SFT Training Types ====================

// CreateSftJobRequest represents the request to create an SFT training job.
// Entry point is Model Square, so modelId is always required (no huggingface source).
type CreateSftJobRequest struct {
	DisplayName string `json:"displayName" binding:"required"`
	Workspace   string `json:"workspace" binding:"required"`
	ModelId     string `json:"modelId" binding:"required"`
	DatasetId   string `json:"datasetId" binding:"required"`

	TrainConfig SftTrainConfig `json:"trainConfig"`

	ExportModel      *bool             `json:"exportModel"`
	Image            string            `json:"image"`
	NodeCount        int               `json:"nodeCount"`
	GpuCount         int               `json:"gpuCount"`
	Cpu              string            `json:"cpu"`
	Memory           string            `json:"memory"`
	EphemeralStorage string            `json:"ephemeralStorage"`
	Env              map[string]string `json:"env"`
	Hostpath         []string          `json:"hostpath"`
	Priority         int               `json:"priority"`
	Timeout          int               `json:"timeout"`
	Description      string            `json:"description"`
	ForceHostNetwork bool              `json:"forceHostNetwork"`
}

// SftTrainConfig holds SFT-specific training hyperparameters.
// All fields are optional; the backend fills defaults based on model size and peft type.
type SftTrainConfig struct {
	Peft          string `json:"peft"`
	DatasetFormat string `json:"datasetFormat"`

	TrainIters      int     `json:"trainIters"`
	GlobalBatchSize int     `json:"globalBatchSize"`
	MicroBatchSize  int     `json:"microBatchSize"`
	SeqLength       int     `json:"seqLength"`
	FinetuneLr      float64 `json:"finetuneLr"`
	MinLr           float64 `json:"minLr"`
	LrWarmupIters   int     `json:"lrWarmupIters"`
	EvalInterval    int     `json:"evalInterval"`
	SaveInterval    int     `json:"saveInterval"`
	PrecisionConfig string  `json:"precisionConfig"`

	TensorModelParallelSize   int  `json:"tensorModelParallelSize"`
	PipelineModelParallelSize int  `json:"pipelineModelParallelSize"`
	ContextParallelSize       int  `json:"contextParallelSize"`
	SequenceParallel          bool `json:"sequenceParallel"`

	PeftDim        int  `json:"peftDim"`
	PeftAlpha      int  `json:"peftAlpha"`
	PackedSequence bool `json:"packedSequence"`
}

// CreateSftJobResponse is returned after successfully creating an SFT job.
// Frontend uses workloadId to redirect to /training/detail?id=xxx.
type CreateSftJobResponse struct {
	WorkloadId string `json:"workloadId"`
}

// GetSftConfigQuery represents query parameters for fetching SFT form defaults.
type GetSftConfigQuery struct {
	Workspace string `form:"workspace" binding:"required"`
}

// SftConfigResponse contains frontend-facing defaults and options for the SFT form.
type SftConfigResponse struct {
	Supported     bool                    `json:"supported"`
	Reason        string                  `json:"reason,omitempty"`
	Model         SftConfigModelInfo      `json:"model"`
	DatasetFilter SftConfigDatasetFilter  `json:"datasetFilter"`
	Defaults      *SftConfigDefaults      `json:"defaults,omitempty"`
	Options       SftConfigOptions        `json:"options"`
}

type SftConfigModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	ModelName   string `json:"modelName"`
	AccessMode  string `json:"accessMode"`
	Phase       string `json:"phase"`
	Workspace   string `json:"workspace"`
	MaxTokens   int    `json:"maxTokens"`
}

type SftConfigDatasetFilter struct {
	DatasetType string `json:"datasetType"`
	Workspace   string `json:"workspace"`
	Status      string `json:"status"`
}

type SftConfigDefaults struct {
	ExportModel      bool           `json:"exportModel"`
	Image            string         `json:"image"`
	NodeCount        int            `json:"nodeCount"`
	GpuCount         int            `json:"gpuCount"`
	Cpu              string         `json:"cpu"`
	Memory           string         `json:"memory"`
	EphemeralStorage string         `json:"ephemeralStorage"`
	Priority         int            `json:"priority"`
	TrainConfig      SftTrainConfig `json:"trainConfig"`
}

type SftConfigOptions struct {
	PeftOptions          []string               `json:"peftOptions"`
	DatasetFormatOptions []string               `json:"datasetFormatOptions"`
	PriorityOptions      []SftConfigPriorityRef `json:"priorityOptions"`
}

type SftConfigPriorityRef struct {
	Label string `json:"label"`
	Value int    `json:"value"`
}

// ==================== SFT Label Constants ====================

const (
	SftLabelWorkloadType = "safe/workload-type"
	SftLabelModel        = "safe/sft-model"
	SftLabelDataset      = "safe/sft-dataset"
	SftLabelPeft         = "safe/sft-peft"
	SftLabelBaseModelId  = "safe/sft-base-model-id"
	SftLabelUserId       = "safe/sft-user-id"
	SftLabelUserName     = "safe/sft-user-name"

	SftWorkloadTypeValue = "sft"
)
