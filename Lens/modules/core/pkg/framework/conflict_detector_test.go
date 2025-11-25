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

func TestConflictDetector_DetectConflicts_OneConflict(t *testing.T) {
	detector := NewConflictDetector()
	
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
	assert.Len(t, conflicts, 1, "One conflict should be detected")
	assert.Equal(t, "log", conflicts[0].Source1)
	assert.Equal(t, "component", conflicts[0].Source2)
	assert.Equal(t, "primus", conflicts[0].Framework1)
	assert.Equal(t, "deepspeed", conflicts[0].Framework2)
}

func TestConflictDetector_DetectConflicts_MultipleConflicts(t *testing.T) {
	detector := NewConflictDetector()
	
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
		{
			Source:     "wandb",
			Framework:  "megatron",
			DetectedAt: time.Now(),
		},
	}
	
	conflicts := detector.DetectConflicts(sources)
	// With 3 different frameworks, we should have 3 conflicts (pairwise)
	assert.Len(t, conflicts, 3, "Three conflicts should be detected")
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
			name: "No conflict",
			sources: []model.DetectionSource{
				{Framework: "primus"},
				{Framework: "primus"},
			},
			expected: false,
		},
		{
			name: "Has conflict",
			sources: []model.DetectionSource{
				{Framework: "primus"},
				{Framework: "deepspeed"},
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
		{Framework: "primus"},
		{Framework: "deepspeed"},
		{Framework: "megatron"},
	}
	
	count := detector.CountConflicts(sources)
	assert.Equal(t, 3, count, "Should count 3 pairwise conflicts")
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

