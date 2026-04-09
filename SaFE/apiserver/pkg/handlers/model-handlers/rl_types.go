/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

// ==================== RL Training Types ====================

// CreateRlJobRequest represents the request to create an RL training job (GRPO/PPO via verl).
// Entry point is Model Square — the base model must be a local HuggingFace-format model.
// The job is dispatched as a RayJob workload.
type CreateRlJobRequest struct {
	DisplayName string `json:"displayName" binding:"required"`
	Workspace   string `json:"workspace" binding:"required"`
	ModelId     string `json:"modelId" binding:"required"`
	DatasetId   string `json:"datasetId" binding:"required"`

	TrainConfig RlTrainConfig `json:"trainConfig"`

	ExportModel      *bool             `json:"exportModel"`
	Image            string            `json:"image"`
	NodeCount        int               `json:"nodeCount"`
	GpuCount         int               `json:"gpuCount"`
	Cpu              string            `json:"cpu"`
	Memory           string            `json:"memory"`
	SharedMemory     string            `json:"sharedMemory"`
	EphemeralStorage string            `json:"ephemeralStorage"`
	Env              map[string]string `json:"env"`
	Priority         int               `json:"priority"`
	Timeout          int               `json:"timeout"`
	Description      string            `json:"description"`
}

// RlTrainConfig holds RL-specific training hyperparameters for verl GRPO/PPO.
type RlTrainConfig struct {
	Algorithm string `json:"algorithm"` // "grpo" (default) | "ppo"
	Strategy  string `json:"strategy"`  // "fsdp2" (default) | "megatron"

	// Data
	TrainBatchSize    int `json:"trainBatchSize"`
	MaxPromptLength   int `json:"maxPromptLength"`
	MaxResponseLength int `json:"maxResponseLength"`

	// Actor
	ActorLr              float64 `json:"actorLr"`
	MiniPatchSize        int     `json:"miniPatchSize"`
	MicroBatchSizePerGpu int     `json:"microBatchSizePerGpu"`
	GradClip             float64 `json:"gradClip"`

	// FSDP2-specific
	ParamOffload          bool `json:"paramOffload"`
	OptimizerOffload      bool `json:"optimizerOffload"`
	GradientCheckpointing bool `json:"gradientCheckpointing"`
	UseTorchCompile       bool `json:"useTorchCompile"`

	// Megatron-specific
	MegatronTpSize int  `json:"megatronTpSize"` // Tensor parallel size for training
	MegatronPpSize int  `json:"megatronPpSize"` // Pipeline parallel size
	MegatronEpSize int  `json:"megatronEpSize"` // Expert parallel size (MoE models)
	MegatronCpSize int  `json:"megatronCpSize"` // Context parallel size
	GradOffload    bool `json:"gradOffload"`

	// KL
	UseKlLoss  bool    `json:"useKlLoss"`
	KlLossCoef float64 `json:"klLossCoef"`

	// Rollout (SGLang)
	RolloutN         int     `json:"rolloutN"`
	RolloutTpSize    int     `json:"rolloutTpSize"`
	RolloutGpuMemory float64 `json:"rolloutGpuMemory"`

	// Ref model
	RefParamOffload        bool `json:"refParamOffload"`
	RefReshardAfterForward bool `json:"refReshardAfterForward"`

	// Training schedule
	TotalEpochs int `json:"totalEpochs"`
	SaveFreq    int `json:"saveFreq"`
	TestFreq    int `json:"testFreq"`

	// Reward
	RewardType string `json:"rewardType"` // "math" | "custom"
}

type CreateRlJobResponse struct {
	WorkloadId string `json:"workloadId"`
}

// GetRlConfigQuery represents query parameters for fetching RL form defaults.
type GetRlConfigQuery struct {
	Workspace string `form:"workspace" binding:"required"`
	Strategy  string `form:"strategy"` // "fsdp2" (default) | "megatron"
}

// RlConfigResponse contains frontend-facing defaults and options for the RL form.
type RlConfigResponse struct {
	Supported     bool                   `json:"supported"`
	Reason        string                 `json:"reason,omitempty"`
	Model         RlConfigModelInfo      `json:"model"`
	DatasetFilter RlConfigDatasetFilter  `json:"datasetFilter"`
	Defaults      *RlConfigDefaults      `json:"defaults,omitempty"`
	Options       RlConfigOptions        `json:"options"`
}

type RlConfigModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
	ModelName   string `json:"modelName"`
	AccessMode  string `json:"accessMode"`
	Phase       string `json:"phase"`
	Workspace   string `json:"workspace"`
}

type RlConfigDatasetFilter struct {
	DatasetType string `json:"datasetType"`
	Workspace   string `json:"workspace"`
	Status      string `json:"status"`
}

type RlConfigDefaults struct {
	ExportModel      bool          `json:"exportModel"`
	Image            string        `json:"image"`
	NodeCount        int           `json:"nodeCount"`
	GpuCount         int           `json:"gpuCount"`
	Cpu              string        `json:"cpu"`
	Memory           string        `json:"memory"`
	SharedMemory     string        `json:"sharedMemory"`
	EphemeralStorage string        `json:"ephemeralStorage"`
	Priority         int           `json:"priority"`
	TrainConfig      RlTrainConfig `json:"trainConfig"`
}

type RlConfigOptions struct {
	AlgorithmOptions  []string               `json:"algorithmOptions"`
	StrategyOptions   []string               `json:"strategyOptions"`
	RewardTypeOptions []string               `json:"rewardTypeOptions"`
	PriorityOptions   []SftConfigPriorityRef `json:"priorityOptions"`
}

// ==================== RL Label Constants ====================

const (
	RlWorkloadTypeValue = "rl"

	RlLabelAlgorithm   = "safe/rl-algorithm"
	RlLabelRewardType  = "safe/rl-reward-type"
	RlLabelBaseModelId = "safe/rl-base-model-id"
	RlLabelUserId      = "safe/rl-user-id"
	RlLabelUserName    = "safe/rl-user-name"
)
