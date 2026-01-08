// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

// ReuseConfig represents the configuration for the reuse engine
type ReuseConfig struct {
	Enabled             bool    `json:"enabled"`               // Whether to enable reuse
	MinSimilarityScore  float64 `json:"min_similarity_score"`  // Minimum similarity score (default: 0.85)
	TimeWindowDays      int     `json:"time_window_days"`      // Time window in days (default: 30)
	MinConfidence       float64 `json:"min_confidence"`        // Minimum confidence (default: 0.75)
	ConfidenceDecayRate float64 `json:"confidence_decay_rate"` // Confidence decay rate (default: 0.9)
	MaxCandidates       int     `json:"max_candidates"`        // Maximum number of candidates (default: 100)
	CacheTTLMinutes     int     `json:"cache_ttl_minutes"`     // Cache TTL in minutes (default: 10)
}

// DefaultReuseConfig returns the default configuration
func DefaultReuseConfig() ReuseConfig {
	return ReuseConfig{
		Enabled:             true,
		MinSimilarityScore:  0.85,
		TimeWindowDays:      30,
		MinConfidence:       0.75,
		ConfidenceDecayRate: 0.9,
		MaxCandidates:       100,
		CacheTTLMinutes:     10,
	}
}

// Validate validates the configuration parameters
func (c *ReuseConfig) Validate() error {
	if c.MinSimilarityScore < 0 || c.MinSimilarityScore > 1 {
		return ErrInvalidConfig("min_similarity_score must be between 0 and 1")
	}
	if c.TimeWindowDays <= 0 {
		return ErrInvalidConfig("time_window_days must be positive")
	}
	if c.MinConfidence < 0 || c.MinConfidence > 1 {
		return ErrInvalidConfig("min_confidence must be between 0 and 1")
	}
	if c.ConfidenceDecayRate <= 0 || c.ConfidenceDecayRate > 1 {
		return ErrInvalidConfig("confidence_decay_rate must be between 0 and 1")
	}
	if c.MaxCandidates <= 0 {
		return ErrInvalidConfig("max_candidates must be positive")
	}
	if c.CacheTTLMinutes < 0 {
		return ErrInvalidConfig("cache_ttl_minutes must be non-negative")
	}
	return nil
}

