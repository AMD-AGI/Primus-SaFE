package detection

import "time"

// FrameworkType constants
const (
	FrameworkTypeTraining  = "training"
	FrameworkTypeInference = "inference"
)

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

