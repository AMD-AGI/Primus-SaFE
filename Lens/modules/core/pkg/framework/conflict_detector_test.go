package framework

import (
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
	
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestConflictDetector_DetectConflicts_NoConflict(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{
			Source:     "log",
			Framework:  "primus",
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Framework:  "primus",
			DetectedAt: time.Now(),
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Empty(t, conflicts, "No conflicts should be detected")
}

func TestConflictDetector_DetectConflicts_WrapperAndBaseCompatible(t *testing.T) {
	detector := NewConflictDetector()
	
	// primus (wrapper) and deepspeed (base) should be compatible
	sources := []model.DetectionSource{
		{
			Source:     "log",
			Framework:  "primus",
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Framework:  "deepspeed",
			DetectedAt: time.Now(),
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Empty(t, conflicts, "Wrapper and base frameworks should not conflict")
}

func TestConflictDetector_DetectConflicts_SameLayerConflict(t *testing.T) {
	detector := NewConflictDetector()
	
	// Two different base frameworks should conflict
	sources := []model.DetectionSource{
		{
			Source:     "log",
			Framework:  "deepspeed",
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Framework:  "megatron",
			DetectedAt: time.Now(),
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Len(t, conflicts, 1, "Two different base frameworks should conflict")
	assert.Equal(t, "log", conflicts[0].Source1)
	assert.Equal(t, "component", conflicts[0].Source2)
	assert.Equal(t, "deepspeed", conflicts[0].Framework1)
	assert.Equal(t, "megatron", conflicts[0].Framework2)
}

func TestConflictDetector_DetectConflicts_MultipleConflicts(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{
			Source:     "log",
			Framework:  "primus", // wrapper
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Framework:  "deepspeed", // base
			DetectedAt: time.Now(),
		},
		{
			Source:     "wandb",
			Framework:  "megatron", // base
			DetectedAt: time.Now(),
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	// primus (wrapper) is compatible with both deepspeed and megatron (base)
	// but deepspeed and megatron conflict with each other (both base)
	assert.Len(t, conflicts, 1, "Only one conflict should be detected (deepspeed vs megatron)")
}

func TestConflictDetector_DetectConflicts_EmptySources(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Nil(t, conflicts, "Empty sources should return nil")
}

func TestConflictDetector_DetectConflicts_SingleSource(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{
			Source:     "log",
			Framework:  "primus",
			DetectedAt: time.Now(),
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Nil(t, conflicts, "Single source should have no conflicts")
}

func TestConflictDetector_HasConflict(t *testing.T) {
	detector := NewConflictDetector()
	
	tests := []struct {
		name     string
		sources  []model.DetectionSource
		expected bool
	}{
		{
			name:     "Empty sources",
			sources:  []model.DetectionSource{},
			expected: false,
		},
		{
			name: "Single source",
			sources: []model.DetectionSource{
				{Framework: "primus"},
			},
			expected: false,
		},
		{
			name: "No conflict - same framework",
			sources: []model.DetectionSource{
				{Framework: "primus"},
				{Framework: "primus"},
			},
			expected: false,
		},
		{
			name: "No conflict - wrapper and base compatible",
			sources: []model.DetectionSource{
				{Framework: "primus"},    // wrapper
				{Framework: "deepspeed"}, // base
			},
			expected: false,
		},
		{
			name: "Has conflict - two base frameworks",
			sources: []model.DetectionSource{
				{Framework: "deepspeed"}, // base
				{Framework: "megatron"},  // base
			},
			expected: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.HasConflict(tt.sources)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConflictDetector_GetConflictingSources(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{Source: "log", Framework: "primus"},
		{Source: "component", Framework: "deepspeed"},
		{Source: "wandb", Framework: "primus"},
	}
	
	result := detector.GetConflictingSources(sources)
	assert.Len(t, result, 2, "Should have 2 different frameworks")
	assert.Contains(t, result, "primus")
	assert.Contains(t, result, "deepspeed")
	assert.Len(t, result["primus"], 2, "Primus should have 2 sources")
	assert.Len(t, result["deepspeed"], 1, "Deepspeed should have 1 source")
}

func TestConflictDetector_GetConflictingSources_NoConflict(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{Source: "log", Framework: "primus"},
		{Source: "component", Framework: "primus"},
	}
	
	result := detector.GetConflictingSources(sources)
	assert.Nil(t, result, "No conflict should return nil")
}

func TestConflictDetector_CountConflicts(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{Framework: "primus"},    // wrapper
		{Framework: "deepspeed"}, // base
		{Framework: "megatron"},  // base
	}
	
	count := detector.CountConflicts(sources)
	// primus (wrapper) is compatible with deepspeed and megatron (base)
	// but deepspeed and megatron conflict with each other
	assert.Equal(t, 1, count, "Should count 1 conflict (deepspeed vs megatron)")
}

func TestConflictDetector_GetDistinctFrameworks(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{Framework: "primus"},
		{Framework: "deepspeed"},
		{Framework: "primus"},
		{Framework: "megatron"},
	}
	
	frameworks := detector.GetDistinctFrameworks(sources)
	assert.Len(t, frameworks, 3, "Should have 3 distinct frameworks")
	assert.Contains(t, frameworks, "primus")
	assert.Contains(t, frameworks, "deepspeed")
	assert.Contains(t, frameworks, "megatron")
}

// Test framework layer detection with evidence
func TestConflictDetector_DetectConflicts_WithLayerEvidence(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{
			Source:     "wandb",
			Framework:  "primus",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"wrapper_framework": "primus",
				"base_framework":    "deepspeed",
				"framework_layer":   "wrapper",
			},
		},
		{
			Source:     "log",
			Framework:  "deepspeed",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"base_framework":  "deepspeed",
				"framework_layer": "base",
			},
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Empty(t, conflicts, "Same base framework should not conflict even if one has wrapper")
}

// Test conflicting wrapper frameworks
func TestConflictDetector_DetectConflicts_WrapperConflict(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{
			Source:     "wandb",
			Framework:  "primus",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"wrapper_framework": "primus",
				"framework_layer":   "wrapper",
			},
		},
		{
			Source:     "log",
			Framework:  "lightning",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"wrapper_framework": "lightning",
				"framework_layer":   "wrapper",
			},
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Len(t, conflicts, 1, "Different wrapper frameworks should conflict")
	assert.Contains(t, conflicts[0].Resolution, "wrapper_layer_conflict")
}

// Test conflicting base frameworks with evidence
func TestConflictDetector_DetectConflicts_BaseConflictWithEvidence(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{
			Source:     "wandb",
			Framework:  "deepspeed",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"base_framework":  "deepspeed",
				"framework_layer": "base",
			},
		},
		{
			Source:     "log",
			Framework:  "megatron",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"base_framework":  "megatron",
				"framework_layer": "base",
			},
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Len(t, conflicts, 1, "Different base frameworks should conflict")
	assert.Contains(t, conflicts[0].Resolution, "base_layer_conflict")
}

// Test compatible wrapper and base with evidence
func TestConflictDetector_DetectConflicts_CompatibleWithEvidence(t *testing.T) {
	detector := NewConflictDetector()
	
	sources := []model.DetectionSource{
		{
			Source:     "wandb",
			Framework:  "primus",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"wrapper_framework": "primus",
				"base_framework":    "deepspeed",
				"framework_layer":   "wrapper",
			},
		},
		{
			Source:     "log",
			Framework:  "megatron",
			DetectedAt: time.Now(),
			Evidence: map[string]interface{}{
				"base_framework":  "megatron",
				"framework_layer": "base",
			},
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	assert.Len(t, conflicts, 1, "Different base frameworks (deepspeed vs megatron) should conflict")
}

