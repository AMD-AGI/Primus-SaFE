// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

// DetectionDimension represents a detection dimension type
type DetectionDimension string

const (
	// Framework layers
	DimensionWrapperFramework DetectionDimension = "wrapper_framework" // e.g. primus, lightning
	DimensionBaseFramework    DetectionDimension = "base_framework"    // e.g. megatron, deepspeed

	// Runtime/Capability layer (more fundamental than frameworks)
	DimensionRuntime DetectionDimension = "runtime" // e.g. pytorch, tensorflow, jax

	// Language layer (most fundamental)
	DimensionLanguage DetectionDimension = "language" // e.g. python, java, go

	// Behavior/Task type
	DimensionBehavior DetectionDimension = "behavior" // e.g. training, inference, evaluation

	// Additional dimensions (extensible)
	DimensionAccelerator DetectionDimension = "accelerator" // e.g. cuda, rocm, tpu
	DimensionDistributed DetectionDimension = "distributed" // e.g. ddp, fsdp, horovod
)

// DimensionValue represents a detected value in a specific dimension
type DimensionValue struct {
	Value      string                 `json:"value"`       // e.g. "pytorch", "python", "training"
	Confidence float64                `json:"confidence"`  // 0.0 - 1.0
	Source     string                 `json:"source"`      // Detection source
	DetectedAt time.Time              `json:"detected_at"` // Detection timestamp
	Evidence   map[string]interface{} `json:"evidence"`    // Supporting evidence
}

// MultiDimensionalDetection represents detection results across multiple dimensions
type MultiDimensionalDetection struct {
	WorkloadUID string    `json:"workload_uid"`
	Version     string    `json:"version"` // Schema version
	UpdatedAt   time.Time `json:"updated_at"`

	// Multi-dimensional detection results
	// Key: dimension type, Value: list of detected values
	Dimensions map[DetectionDimension][]DimensionValue `json:"dimensions"`

	// Overall confidence and status
	Confidence float64         `json:"confidence"`
	Status     DetectionStatus `json:"status"`

	// Sources that contributed to this detection
	Sources []DetectionSource `json:"sources"`

	// Conflicts (per dimension)
	Conflicts map[DetectionDimension][]DetectionConflict `json:"conflicts"`
}

// DimensionPriority defines the hierarchical relationship between dimensions
// Used for determining dependencies and validation
type DimensionPriority struct {
	Dimension    DetectionDimension
	Priority     int                  // Lower number = more fundamental
	Dependencies []DetectionDimension // Dimensions that depend on this one
}

// GetDimensionPriorities returns the hierarchy of detection dimensions
// Lower priority number = more fundamental/basic
func GetDimensionPriorities() []DimensionPriority {
	return []DimensionPriority{
		{
			Dimension:    DimensionLanguage,
			Priority:     1, // Most fundamental
			Dependencies: []DetectionDimension{DimensionRuntime, DimensionWrapperFramework, DimensionBaseFramework},
		},
		{
			Dimension:    DimensionRuntime,
			Priority:     2,
			Dependencies: []DetectionDimension{DimensionBaseFramework, DimensionWrapperFramework},
		},
		{
			Dimension:    DimensionBaseFramework,
			Priority:     3,
			Dependencies: []DetectionDimension{DimensionWrapperFramework},
		},
		{
			Dimension:    DimensionWrapperFramework,
			Priority:     4,
			Dependencies: []DetectionDimension{},
		},
		{
			Dimension:    DimensionBehavior,
			Priority:     5, // Orthogonal, but can be detected
			Dependencies: []DetectionDimension{},
		},
		{
			Dimension:    DimensionAccelerator,
			Priority:     6,
			Dependencies: []DetectionDimension{},
		},
		{
			Dimension:    DimensionDistributed,
			Priority:     7,
			Dependencies: []DetectionDimension{},
		},
	}
}

// DimensionCompatibilityRule defines compatibility rules between dimensions
type DimensionCompatibilityRule struct {
	Dimension        DetectionDimension
	Value            string
	CompatibleWith   map[DetectionDimension][]string // Compatible values in other dimensions
	IncompatibleWith map[DetectionDimension][]string // Incompatible values
}

// GetCompatibilityRules returns compatibility rules for validation
func GetCompatibilityRules() []DimensionCompatibilityRule {
	return []DimensionCompatibilityRule{
		{
			Dimension: DimensionRuntime,
			Value:     "pytorch",
			CompatibleWith: map[DetectionDimension][]string{
				DimensionLanguage:      {"python"},
				DimensionBaseFramework: {"megatron", "deepspeed", "fairscale"},
			},
			IncompatibleWith: map[DetectionDimension][]string{
				DimensionRuntime: {"tensorflow", "jax"}, // Cannot have multiple runtimes
			},
		},
		{
			Dimension: DimensionBaseFramework,
			Value:     "megatron",
			CompatibleWith: map[DetectionDimension][]string{
				DimensionRuntime:  {"pytorch"},
				DimensionLanguage: {"python"},
			},
		},
		// Add more rules as needed
	}
}

// DimensionDetector interface for dimension-specific detectors
type DimensionDetector interface {
	// GetDimension returns the dimension this detector handles
	GetDimension() DetectionDimension

	// Detect performs detection for this dimension
	Detect(sources []DetectionSource) ([]DimensionValue, error)

	// Validate checks if detected value is valid in this dimension
	Validate(value string) bool
}
