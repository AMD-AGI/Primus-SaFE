package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDetectionDimensionConstants(t *testing.T) {
	// Verify dimension constants are defined correctly
	assert.Equal(t, DetectionDimension("wrapper_framework"), DimensionWrapperFramework)
	assert.Equal(t, DetectionDimension("base_framework"), DimensionBaseFramework)
	assert.Equal(t, DetectionDimension("runtime"), DimensionRuntime)
	assert.Equal(t, DetectionDimension("language"), DimensionLanguage)
	assert.Equal(t, DetectionDimension("behavior"), DimensionBehavior)
	assert.Equal(t, DetectionDimension("accelerator"), DimensionAccelerator)
	assert.Equal(t, DetectionDimension("distributed"), DimensionDistributed)
}

func TestGetDimensionPriorities(t *testing.T) {
	priorities := GetDimensionPriorities()

	// Verify we have all expected dimensions
	assert.NotEmpty(t, priorities)
	assert.GreaterOrEqual(t, len(priorities), 7)

	// Create a map for easier testing
	priorityMap := make(map[DetectionDimension]int)
	for _, p := range priorities {
		priorityMap[p.Dimension] = p.Priority
	}

	// Verify priority order (language should be most fundamental)
	assert.Less(t, priorityMap[DimensionLanguage], priorityMap[DimensionRuntime])
	assert.Less(t, priorityMap[DimensionRuntime], priorityMap[DimensionBaseFramework])
	assert.Less(t, priorityMap[DimensionBaseFramework], priorityMap[DimensionWrapperFramework])

	// Verify language has highest priority (lowest number)
	assert.Equal(t, 1, priorityMap[DimensionLanguage])
}

func TestGetDimensionPrioritiesDependencies(t *testing.T) {
	priorities := GetDimensionPriorities()

	// Create a map for easier testing
	priorityMap := make(map[DetectionDimension]DimensionPriority)
	for _, p := range priorities {
		priorityMap[p.Dimension] = p
	}

	// Verify language has dependencies on higher-level dimensions
	languagePriority := priorityMap[DimensionLanguage]
	assert.Contains(t, languagePriority.Dependencies, DimensionRuntime)
	assert.Contains(t, languagePriority.Dependencies, DimensionWrapperFramework)
	assert.Contains(t, languagePriority.Dependencies, DimensionBaseFramework)

	// Verify runtime depends on frameworks
	runtimePriority := priorityMap[DimensionRuntime]
	assert.Contains(t, runtimePriority.Dependencies, DimensionBaseFramework)
	assert.Contains(t, runtimePriority.Dependencies, DimensionWrapperFramework)

	// Verify wrapper framework has no dependencies (highest level)
	wrapperPriority := priorityMap[DimensionWrapperFramework]
	assert.Empty(t, wrapperPriority.Dependencies)
}

func TestGetCompatibilityRules(t *testing.T) {
	rules := GetCompatibilityRules()

	assert.NotEmpty(t, rules)

	// Find pytorch runtime rule
	var pytorchRule *DimensionCompatibilityRule
	for i := range rules {
		if rules[i].Dimension == DimensionRuntime && rules[i].Value == "pytorch" {
			pytorchRule = &rules[i]
			break
		}
	}

	assert.NotNil(t, pytorchRule, "Should have pytorch compatibility rule")

	// Verify pytorch is compatible with python
	assert.Contains(t, pytorchRule.CompatibleWith[DimensionLanguage], "python")

	// Verify pytorch is compatible with megatron and deepspeed
	assert.Contains(t, pytorchRule.CompatibleWith[DimensionBaseFramework], "megatron")
	assert.Contains(t, pytorchRule.CompatibleWith[DimensionBaseFramework], "deepspeed")

	// Verify pytorch is incompatible with tensorflow and jax
	assert.Contains(t, pytorchRule.IncompatibleWith[DimensionRuntime], "tensorflow")
	assert.Contains(t, pytorchRule.IncompatibleWith[DimensionRuntime], "jax")
}

func TestGetCompatibilityRulesMegatron(t *testing.T) {
	rules := GetCompatibilityRules()

	// Find megatron base framework rule
	var megatronRule *DimensionCompatibilityRule
	for i := range rules {
		if rules[i].Dimension == DimensionBaseFramework && rules[i].Value == "megatron" {
			megatronRule = &rules[i]
			break
		}
	}

	assert.NotNil(t, megatronRule, "Should have megatron compatibility rule")

	// Verify megatron is compatible with pytorch
	assert.Contains(t, megatronRule.CompatibleWith[DimensionRuntime], "pytorch")

	// Verify megatron is compatible with python
	assert.Contains(t, megatronRule.CompatibleWith[DimensionLanguage], "python")
}

func TestDimensionValueStruct(t *testing.T) {
	now := time.Now()
	value := DimensionValue{
		Value:      "pytorch",
		Confidence: 0.95,
		Source:     "log_analysis",
		DetectedAt: now,
		Evidence: map[string]interface{}{
			"pattern": "import torch",
			"count":   5,
		},
	}

	assert.Equal(t, "pytorch", value.Value)
	assert.Equal(t, 0.95, value.Confidence)
	assert.Equal(t, "log_analysis", value.Source)
	assert.Equal(t, now, value.DetectedAt)
	assert.Contains(t, value.Evidence, "pattern")
	assert.Contains(t, value.Evidence, "count")
}

func TestMultiDimensionalDetectionStruct(t *testing.T) {
	now := time.Now()
	detection := MultiDimensionalDetection{
		WorkloadUID: "workload-123",
		Version:     "2.0",
		UpdatedAt:   now,
		Dimensions: map[DetectionDimension][]DimensionValue{
			DimensionRuntime: {
				{Value: "pytorch", Confidence: 0.9, Source: "log", DetectedAt: now},
			},
			DimensionLanguage: {
				{Value: "python", Confidence: 0.95, Source: "process", DetectedAt: now},
			},
		},
		Confidence: 0.9,
		Status:     DetectionStatusConfirmed,
		Sources: []DetectionSource{
			{Source: "log", Frameworks: []string{"pytorch"}, Type: "training", Confidence: 0.9, DetectedAt: now},
		},
		Conflicts: make(map[DetectionDimension][]DetectionConflict),
	}

	assert.Equal(t, "workload-123", detection.WorkloadUID)
	assert.Equal(t, "2.0", detection.Version)
	assert.Equal(t, 0.9, detection.Confidence)
	assert.Equal(t, DetectionStatusConfirmed, detection.Status)
	assert.Len(t, detection.Dimensions, 2)
	assert.Len(t, detection.Sources, 1)
	assert.Contains(t, detection.Dimensions, DimensionRuntime)
	assert.Contains(t, detection.Dimensions, DimensionLanguage)
}

func TestDimensionPriorityStruct(t *testing.T) {
	priority := DimensionPriority{
		Dimension:    DimensionRuntime,
		Priority:     2,
		Dependencies: []DetectionDimension{DimensionBaseFramework, DimensionWrapperFramework},
	}

	assert.Equal(t, DimensionRuntime, priority.Dimension)
	assert.Equal(t, 2, priority.Priority)
	assert.Len(t, priority.Dependencies, 2)
	assert.Contains(t, priority.Dependencies, DimensionBaseFramework)
	assert.Contains(t, priority.Dependencies, DimensionWrapperFramework)
}

func TestDimensionCompatibilityRuleStruct(t *testing.T) {
	rule := DimensionCompatibilityRule{
		Dimension: DimensionRuntime,
		Value:     "tensorflow",
		CompatibleWith: map[DetectionDimension][]string{
			DimensionLanguage: {"python"},
		},
		IncompatibleWith: map[DetectionDimension][]string{
			DimensionRuntime: {"pytorch", "jax"},
		},
	}

	assert.Equal(t, DimensionRuntime, rule.Dimension)
	assert.Equal(t, "tensorflow", rule.Value)
	assert.Contains(t, rule.CompatibleWith[DimensionLanguage], "python")
	assert.Contains(t, rule.IncompatibleWith[DimensionRuntime], "pytorch")
	assert.Contains(t, rule.IncompatibleWith[DimensionRuntime], "jax")
}
