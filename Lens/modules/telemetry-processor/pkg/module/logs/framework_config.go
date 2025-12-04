package logs

import "time"

// FrameworkLogPatterns defines log parsing patterns for a training framework
type FrameworkLogPatterns struct {
	// Framework identification
	Name        string `json:"name"`         // Framework name: primus, deepspeed, megatron
	DisplayName string `json:"display_name"` // Display name
	Version     string `json:"version"`      // Config version
	Priority    int    `json:"priority"`     // Priority for matching (higher = higher priority)
	Enabled     bool   `json:"enabled"`      // Whether this framework is enabled
	
	// Framework identification pattern (for auto-detection)
	IdentifyPatterns []PatternConfig `json:"identify_patterns"`
	
	// Performance log patterns
	PerformancePatterns []PatternConfig `json:"performance_patterns"`
	
	// Training lifecycle events
	TrainingEvents TrainingEventPatterns `json:"training_events"`
	
	// Checkpoint events
	CheckpointEvents CheckpointEventPatterns `json:"checkpoint_events"`
	
	// Extension fields
	Extensions map[string]interface{} `json:"extensions,omitempty"`
	
	// Metadata
	UpdatedAt time.Time `json:"updated_at"`
	CreatedAt time.Time `json:"created_at"`
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
	return nil
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

