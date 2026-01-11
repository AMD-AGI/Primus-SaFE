// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package detection

import "time"

// FrameworkType constants
const (
	FrameworkTypeTraining  = "training"
	FrameworkTypeInference = "inference"
)

// FrameworkLayer constants
const (
	// Training layers (hierarchical)
	FrameworkLayerWrapper       = "wrapper"       // L1: Training abstraction (primus, lightning)
	FrameworkLayerOrchestration = "orchestration" // L2: Distributed training (megatron, deepspeed)
	FrameworkLayerRuntime       = "runtime"       // L3: Base DL framework (pytorch, tensorflow, jax)

	// Inference layer
	FrameworkLayerInference = "inference" // Inference serving (vllm, triton, tgi)
)

// FrameworkLayerPriority defines layer priority for winner selection
// Higher priority = higher layer (wrapper > orchestration > runtime)
var FrameworkLayerPriority = map[string]int{
	FrameworkLayerWrapper:       3,
	FrameworkLayerOrchestration: 2,
	FrameworkLayerRuntime:       1,
	FrameworkLayerInference:     0, // Separate track
}

// FrameworkLogPatterns defines log parsing patterns for a framework (training or inference)
type FrameworkLogPatterns struct {
	// Framework identification
	Name        string `json:"name"`         // Framework name: primus, deepspeed, megatron, vllm, tgi, triton
	DisplayName string `json:"display_name"` // Display name
	Version     string `json:"version"`      // Config version
	Priority    int    `json:"priority"`     // Priority for matching (higher = higher priority)
	Enabled     bool   `json:"enabled"`      // Whether this framework is enabled

	// Framework type: "training" or "inference"
	// Empty or unset defaults to "training" for backward compatibility
	Type string `json:"type,omitempty"`

	// Framework layer: "wrapper", "orchestration", "runtime", "inference"
	// - wrapper: High-level training abstraction (primus, lightning)
	// - orchestration: Distributed training / optimization (megatron, deepspeed)
	// - runtime: Base DL framework (pytorch, tensorflow, jax)
	// - inference: Inference serving (vllm, triton, tgi)
	// Default to "runtime" for backward compatibility
	Layer string `json:"layer,omitempty"`

	// Framework identification pattern (for auto-detection)
	IdentifyPatterns []PatternConfig `json:"identify_patterns"`

	// Performance log patterns
	PerformancePatterns []PatternConfig `json:"performance_patterns"`

	// Training lifecycle events (for training frameworks)
	TrainingEvents TrainingEventPatterns `json:"training_events,omitempty"`

	// Checkpoint events (for training frameworks)
	CheckpointEvents CheckpointEventPatterns `json:"checkpoint_events,omitempty"`

	// Inference patterns (for inference frameworks)
	InferencePatterns *InferencePatternConfig `json:"inference_patterns,omitempty"`

	// Extension fields
	Extensions map[string]interface{} `json:"extensions,omitempty"`

	// Metadata
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
}

// InferencePatternConfig defines patterns for inference framework detection
type InferencePatternConfig struct {
	// Process name patterns
	ProcessPatterns []PatternConfig `json:"process_patterns,omitempty"`

	// Port patterns (common ports for this inference service)
	Ports []int `json:"ports,omitempty"`

	// Environment variable patterns
	EnvPatterns []PatternConfig `json:"env_patterns,omitempty"`

	// Image name patterns
	ImagePatterns []PatternConfig `json:"image_patterns,omitempty"`

	// Command line patterns
	CmdlinePatterns []PatternConfig `json:"cmdline_patterns,omitempty"`

	// Health check endpoint
	HealthEndpoint string `json:"health_endpoint,omitempty"`
}

// PatternConfig defines a regex pattern configuration
type PatternConfig struct {
	Name        string   `json:"name"`        // Pattern name
	Pattern     string   `json:"pattern"`     // Regex pattern
	Description string   `json:"description"` // Description
	Enabled     bool     `json:"enabled"`     // Enabled flag
	Tags        []string `json:"tags"`        // Tags for categorization
	Confidence  float64  `json:"confidence"`  // Detection confidence (0.0-1.0)
}

// TrainingEventPatterns defines patterns for training lifecycle events
type TrainingEventPatterns struct {
	StartTraining  []PatternConfig `json:"start_training"`
	EndTraining    []PatternConfig `json:"end_training,omitempty"`
	PauseTraining  []PatternConfig `json:"pause_training,omitempty"`
	ResumeTraining []PatternConfig `json:"resume_training,omitempty"`
}

// CheckpointEventPatterns defines patterns for checkpoint events
type CheckpointEventPatterns struct {
	StartSaving []PatternConfig `json:"start_saving"`
	EndSaving   []PatternConfig `json:"end_saving"`
	Loading     []PatternConfig `json:"loading,omitempty"`
}

// Validate validates the framework log patterns configuration
func (f *FrameworkLogPatterns) Validate() error {
	if f.Name == "" {
		return ErrInvalidFrameworkName
	}
	if f.Priority < 0 {
		return ErrInvalidPriority
	}
	// Validate type if specified
	if f.Type != "" && f.Type != FrameworkTypeTraining && f.Type != FrameworkTypeInference {
		return ErrInvalidFrameworkType
	}
	// Validate layer if specified
	if f.Layer != "" {
		validLayers := map[string]bool{
			FrameworkLayerWrapper:       true,
			FrameworkLayerOrchestration: true,
			FrameworkLayerRuntime:       true,
			FrameworkLayerInference:     true,
		}
		if !validLayers[f.Layer] {
			return ErrInvalidFrameworkLayer
		}
	}
	return nil
}

// GetType returns the framework type, defaults to "training" for backward compatibility
func (f *FrameworkLogPatterns) GetType() string {
	if f.Type == "" {
		return FrameworkTypeTraining
	}
	return f.Type
}

// IsTraining returns true if this is a training framework
func (f *FrameworkLogPatterns) IsTraining() bool {
	return f.GetType() == FrameworkTypeTraining
}

// IsInference returns true if this is an inference framework
func (f *FrameworkLogPatterns) IsInference() bool {
	return f.GetType() == FrameworkTypeInference
}

// GetLayer returns the framework layer, defaults to "runtime" for backward compatibility
func (f *FrameworkLogPatterns) GetLayer() string {
	if f.Layer == "" {
		// Default based on type for backward compatibility
		if f.GetType() == FrameworkTypeInference {
			return FrameworkLayerInference
		}
		return FrameworkLayerRuntime
	}
	return f.Layer
}

// IsWrapper returns true if this is a wrapper (L1) framework
func (f *FrameworkLogPatterns) IsWrapper() bool {
	return f.GetLayer() == FrameworkLayerWrapper
}

// IsOrchestration returns true if this is an orchestration (L2) framework
func (f *FrameworkLogPatterns) IsOrchestration() bool {
	return f.GetLayer() == FrameworkLayerOrchestration
}

// IsRuntime returns true if this is a runtime (L3) framework
func (f *FrameworkLogPatterns) IsRuntime() bool {
	return f.GetLayer() == FrameworkLayerRuntime
}

// IsInferenceLayer returns true if this is an inference layer framework
func (f *FrameworkLogPatterns) IsInferenceLayer() bool {
	return f.GetLayer() == FrameworkLayerInference
}

// GetLayerPriority returns the layer priority for winner selection
func (f *FrameworkLogPatterns) GetLayerPriority() int {
	if priority, ok := FrameworkLayerPriority[f.GetLayer()]; ok {
		return priority
	}
	return 0
}

// GetEnabledInferenceProcessPatterns returns enabled inference process patterns
func (f *FrameworkLogPatterns) GetEnabledInferenceProcessPatterns() []PatternConfig {
	if f.InferencePatterns == nil {
		return nil
	}
	var enabled []PatternConfig
	for _, p := range f.InferencePatterns.ProcessPatterns {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetEnabledInferenceEnvPatterns returns enabled inference environment patterns
func (f *FrameworkLogPatterns) GetEnabledInferenceEnvPatterns() []PatternConfig {
	if f.InferencePatterns == nil {
		return nil
	}
	var enabled []PatternConfig
	for _, p := range f.InferencePatterns.EnvPatterns {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetEnabledInferenceImagePatterns returns enabled inference image patterns
func (f *FrameworkLogPatterns) GetEnabledInferenceImagePatterns() []PatternConfig {
	if f.InferencePatterns == nil {
		return nil
	}
	var enabled []PatternConfig
	for _, p := range f.InferencePatterns.ImagePatterns {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetEnabledInferenceCmdlinePatterns returns enabled inference command line patterns
func (f *FrameworkLogPatterns) GetEnabledInferenceCmdlinePatterns() []PatternConfig {
	if f.InferencePatterns == nil {
		return nil
	}
	var enabled []PatternConfig
	for _, p := range f.InferencePatterns.CmdlinePatterns {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetEnabledIdentifyPatterns returns enabled identify patterns
func (f *FrameworkLogPatterns) GetEnabledIdentifyPatterns() []PatternConfig {
	var enabled []PatternConfig
	for _, p := range f.IdentifyPatterns {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// GetEnabledPerformancePatterns returns enabled performance patterns
func (f *FrameworkLogPatterns) GetEnabledPerformancePatterns() []PatternConfig {
	var enabled []PatternConfig
	for _, p := range f.PerformancePatterns {
		if p.Enabled {
			enabled = append(enabled, p)
		}
	}
	return enabled
}

// Validate validates pattern configuration
func (p *PatternConfig) Validate() error {
	if p.Name == "" {
		return ErrInvalidPatternName
	}
	if p.Pattern == "" {
		return ErrInvalidPattern
	}
	if p.Confidence < 0.0 || p.Confidence > 1.0 {
		return ErrInvalidConfidence
	}
	return nil
}

