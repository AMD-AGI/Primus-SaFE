// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import "time"

// DetectionStatus represents the framework detection status
type DetectionStatus string

const (
	DetectionStatusUnknown   DetectionStatus = "unknown"   // Unknown status, no detection yet
	DetectionStatusSuspected DetectionStatus = "suspected" // Suspected, single weak signal
	DetectionStatusConfirmed DetectionStatus = "confirmed" // Confirmed, single strong signal
	DetectionStatusVerified  DetectionStatus = "verified"  // Verified, multiple consistent sources
	DetectionStatusReused    DetectionStatus = "reused"    // Reused from similar workload
	DetectionStatusConflict  DetectionStatus = "conflict"  // Conflict between sources
)

// FrameworkDetection represents the framework detection result with dual-layer support
type FrameworkDetection struct {
	Frameworks []string            `json:"frameworks"` // Detected frameworks: [wrapper, base] for dual-layer, [framework] for single-layer
	Type       string              `json:"type"`       // Task type (training, inference, etc.)
	Confidence float64             `json:"confidence"` // Confidence score [0.0-1.0]
	Status     DetectionStatus     `json:"status"`     // Detection status
	Sources    []DetectionSource   `json:"sources"`    // Data sources that contributed to detection
	Conflicts  []DetectionConflict `json:"conflicts"`  // Conflict records if any

	// Dual-layer framework support
	FrameworkLayer   string `json:"framework_layer,omitempty"`   // Framework layer: "wrapper" or "base"
	WrapperFramework string `json:"wrapper_framework,omitempty"` // Wrapper framework (e.g., primus, lightning)
	BaseFramework    string `json:"base_framework,omitempty"`    // Base framework (e.g., megatron, deepspeed)

	// Reuse information (only set when status is reused)
	ReuseInfo *ReuseInfo `json:"reuse_info,omitempty"` // Reuse metadata

	Version   string    `json:"version"`    // Schema version
	UpdatedAt time.Time `json:"updated_at"` // Last update timestamp
}

// DetectionSource represents a single detection data source with dual-layer support
type DetectionSource struct {
	Source     string                 `json:"source"`      // Source identifier (component, log, wandb, user, reuse, image, default)
	Frameworks []string               `json:"frameworks"`  // Detected frameworks: [wrapper, base] or [framework]
	Type       string                 `json:"type"`        // Task type detected by this source
	Confidence float64                `json:"confidence"`  // Confidence of this source [0.0-1.0]
	DetectedAt time.Time              `json:"detected_at"` // Detection timestamp
	Evidence   map[string]interface{} `json:"evidence"`    // Evidence data (method, details, matched patterns, etc.)

	// Dual-layer framework support
	FrameworkLayer   string `json:"framework_layer,omitempty"`   // Framework layer: "wrapper" or "base"
	WrapperFramework string `json:"wrapper_framework,omitempty"` // Wrapper framework if detected
	BaseFramework    string `json:"base_framework,omitempty"`    // Base framework if detected
}

// DetectionConflict represents a conflict between two detection sources
type DetectionConflict struct {
	Source1    string    `json:"source1"`     // First conflicting source
	Source2    string    `json:"source2"`     // Second conflicting source
	Framework1 string    `json:"framework1"`  // Framework from source1
	Framework2 string    `json:"framework2"`  // Framework from source2
	Resolution string    `json:"resolution"`  // Resolution method (priority, confidence, time, etc.)
	ResolvedAt time.Time `json:"resolved_at"` // Resolution timestamp
}
