// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package framework

// SourcePriority defines priority configuration for a detection source
type SourcePriority struct {
	Source         string  // Source name
	Priority       int     // Priority (higher value = higher priority)
	BaseConfidence float64 // Base confidence range for this source
}

// DefaultSourcePriorities defines the default priority for each detection source
var DefaultSourcePriorities = []SourcePriority{
	{Source: "user", Priority: 100, BaseConfidence: 1.0},       // User manual annotation (highest priority)
	{Source: "component", Priority: 80, BaseConfidence: 0.85},  // Component analysis (high trust)
	{Source: "wandb", Priority: 70, BaseConfidence: 0.75},      // WandB detection (medium-high trust)
	{Source: "log", Priority: 60, BaseConfidence: 0.70},        // Log pattern matching (medium trust)
	{Source: "reuse", Priority: 50, BaseConfidence: 0.85},      // Reuse from similar workload
	{Source: "image", Priority: 40, BaseConfidence: 0.50},      // Image name inference (low trust)
	{Source: "default", Priority: 20, BaseConfidence: 0.30},    // Default inference (lowest)
}

// DetectionConfig holds configuration for framework detection
type DetectionConfig struct {
	// Confidence thresholds for different detection statuses
	SuspectedThreshold float64 // Suspected status threshold (default: 0.3)
	ConfirmedThreshold float64 // Confirmed status threshold (default: 0.6)
	VerifiedThreshold  float64 // Verified status threshold (default: 0.85)
	
	// Multi-source merging strategy
	MultiSourceBoost float64 // Confidence boost per additional consistent source (default: 0.1)
	ConflictPenalty  float64 // Confidence penalty when conflict exists (default: 0.2)
	
	// Source priorities mapping
	SourcePriorities map[string]int // Map from source name to priority value
	
	// Cache settings
	EnableCache  bool // Enable detection result caching
	CacheTTLSec  int  // Cache TTL in seconds (default: 300)
}

// DefaultDetectionConfig returns the default detection configuration
func DefaultDetectionConfig() *DetectionConfig {
	priorities := make(map[string]int)
	for _, sp := range DefaultSourcePriorities {
		priorities[sp.Source] = sp.Priority
	}
	
	return &DetectionConfig{
		SuspectedThreshold: 0.3,
		ConfirmedThreshold: 0.6,
		VerifiedThreshold:  0.85,
		MultiSourceBoost:   0.1,
		ConflictPenalty:    0.2,
		SourcePriorities:   priorities,
		EnableCache:        true,
		CacheTTLSec:        300, // 5 minutes
	}
}

// GetSourcePriority returns the priority for a given source
func (c *DetectionConfig) GetSourcePriority(source string) int {
	if priority, ok := c.SourcePriorities[source]; ok {
		return priority
	}
	return 0 // Unknown source has lowest priority
}

// GetBaseConfidence returns the base confidence for a given source
func GetBaseConfidence(source string) float64 {
	for _, sp := range DefaultSourcePriorities {
		if sp.Source == source {
			return sp.BaseConfidence
		}
	}
	return 0.3 // Default for unknown sources
}

