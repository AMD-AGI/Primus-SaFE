package framework

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestConfidenceCalculator_Calculate_SingleSource(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.7,
			DetectedAt: time.Now(),
		},
	}

	result := calc.Calculate(sources)
	assert.Equal(t, 0.7, result, "Single source should return its own confidence")
}

func TestConfidenceCalculator_Calculate_MultiSourceConsistent(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.7,
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Frameworks: []string{"primus"},
			Confidence: 0.8,
			DetectedAt: time.Now(),
		},
	}

	result := calc.Calculate(sources)
	// Weighted average should be higher than simple average due to component priority
	// Plus 0.1 boost for second source
	assert.True(t, result > 0.75, "Multi-source consistent should boost confidence")
	assert.True(t, result <= 1.0, "Confidence should not exceed 1.0")
}

func TestConfidenceCalculator_Calculate_MultiSourceConflict(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.9,
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Frameworks: []string{"deepspeed"},
			Confidence: 0.7,
			DetectedAt: time.Now(),
		},
	}

	result := calc.Calculate(sources)
	// Should take component's confidence (0.7) and apply penalty
	// 0.7 - 0.2 = 0.5, but minimum is 0.3
	expected := 0.7 - config.ConflictPenalty
	assert.InDelta(t, expected, result, 0.01, "Conflicting sources should apply penalty")
}

func TestConfidenceCalculator_Calculate_EmptySources(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	sources := []model.DetectionSource{}

	result := calc.Calculate(sources)
	assert.Equal(t, 0.0, result, "Empty sources should return 0.0")
}

func TestConfidenceCalculator_Calculate_ThreeConsistentSources(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.7,
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Frameworks: []string{"primus"},
			Confidence: 0.8,
			DetectedAt: time.Now(),
		},
		{
			Source:     "wandb",
			Frameworks: []string{"primus"},
			Confidence: 0.75,
			DetectedAt: time.Now(),
		},
	}

	result := calc.Calculate(sources)
	// Three consistent sources should have high confidence
	assert.True(t, result > 0.85, "Three consistent sources should have high confidence")
	assert.True(t, result <= 1.0, "Confidence should not exceed 1.0")
}

func TestConfidenceCalculator_Clamp(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"Below zero", -0.5, 0.0},
		{"Above one", 1.5, 1.0},
		{"Valid low", 0.3, 0.3},
		{"Valid high", 0.9, 0.9},
		{"Exactly zero", 0.0, 0.0},
		{"Exactly one", 1.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.clamp(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfidenceCalculator_AllAgree(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	tests := []struct {
		name     string
		sources  []model.DetectionSource
		expected bool
	}{
		{
			name:     "Empty sources",
			sources:  []model.DetectionSource{},
			expected: true,
		},
		{
			name: "Single source",
			sources: []model.DetectionSource{
				{Frameworks: []string{"primus"}},
			},
			expected: true,
		},
		{
			name: "All agree",
			sources: []model.DetectionSource{
				{Frameworks: []string{"primus"}},
				{Frameworks: []string{"primus"}},
				{Frameworks: []string{"primus"}},
			},
			expected: true,
		},
		{
			name: "Conflict",
			sources: []model.DetectionSource{
				{Frameworks: []string{"primus"}},
				{Frameworks: []string{"deepspeed"}},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calc.allAgree(tt.sources)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfidenceCalculator_CalculateWeighted(t *testing.T) {
	config := DefaultDetectionConfig()
	calc := NewConfidenceCalculator(config)

	sources := []model.DetectionSource{
		{
			Source:     "log", // Priority 60
			Frameworks: []string{"primus"},
			Confidence: 0.7,
		},
		{
			Source:     "component", // Priority 80
			Frameworks: []string{"primus"},
			Confidence: 0.9,
		},
	}

	result := calc.CalculateWeighted(sources)
	// Component has higher priority, so weighted average should be closer to 0.9
	assert.True(t, result > 0.7, "Weighted average should be higher than lowest confidence")
	assert.True(t, result < 0.9, "Weighted average should be lower than highest confidence")
	assert.True(t, result > 0.8, "Weighted average should favor higher priority source")
}
