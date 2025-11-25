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

// FrameworkDetection represents the framework detection result
type FrameworkDetection struct {
	Framework  string              `json:"framework"`  // Framework name (pytorch, tensorflow, primus, deepspeed, megatron, etc.)
	Type       string              `json:"type"`       // Task type (training, inference, etc.)
	Confidence float64             `json:"confidence"` // Confidence score [0.0-1.0]
	Status     DetectionStatus     `json:"status"`     // Detection status
	Sources    []DetectionSource   `json:"sources"`    // Data sources that contributed to detection
	Conflicts  []DetectionConflict `json:"conflicts"`  // Conflict records if any

	// Reuse information (only set when status is reused)
	ReuseInfo *ReuseInfo `json:"reuse_info,omitempty"` // Reuse metadata

	Version   string    `json:"version"`    // Schema version
	UpdatedAt time.Time `json:"updated_at"` // Last update timestamp
}

// DetectionSource represents a single detection data source
type DetectionSource struct {
	Source     string                 `json:"source"`      // Source identifier (component, log, wandb, user, reuse, image, default)
	Framework  string                 `json:"framework"`   // Framework detected by this source
	Type       string                 `json:"type"`        // Task type detected by this source
	Confidence float64                `json:"confidence"`  // Confidence of this source [0.0-1.0]
	DetectedAt time.Time              `json:"detected_at"` // Detection timestamp
	Evidence   map[string]interface{} `json:"evidence"`    // Evidence data (method, details, matched patterns, etc.)
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
