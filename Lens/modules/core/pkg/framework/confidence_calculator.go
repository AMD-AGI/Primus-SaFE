package framework

import (
	"math"
	
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ConfidenceCalculator calculates overall confidence from multiple detection sources
type ConfidenceCalculator struct {
	config *DetectionConfig
}

// NewConfidenceCalculator creates a new confidence calculator
func NewConfidenceCalculator(config *DetectionConfig) *ConfidenceCalculator {
	return &ConfidenceCalculator{
		config: config,
	}
}

// Calculate computes the overall confidence score from multiple sources
// Algorithm:
// - Single source: return its confidence
// - Multiple consistent sources: weighted average + boost
// - Conflicting sources: max confidence - penalty
func (c *ConfidenceCalculator) Calculate(sources []model.DetectionSource) float64 {
	if len(sources) == 0 {
		return 0.0
	}
	
	// Scenario 1: Single source
	if len(sources) == 1 {
		return c.clamp(sources[0].Confidence)
	}
	
	// Check if all sources agree on the framework
	if c.allAgree(sources) {
		// Scenario 2: Multiple consistent sources
		return c.calculateConsistentConfidence(sources)
	}
	
	// Scenario 3: Conflicting sources
	return c.calculateConflictConfidence(sources)
}

// allAgree checks if all sources agree on the same framework
func (c *ConfidenceCalculator) allAgree(sources []model.DetectionSource) bool {
	if len(sources) <= 1 {
		return true
	}
	
	firstFramework := sources[0].Framework
	for i := 1; i < len(sources); i++ {
		if sources[i].Framework != firstFramework {
			return false
		}
	}
	return true
}

// calculateConsistentConfidence calculates confidence when all sources agree
// Formula: weighted_average + boost * (num_sources - 1)
func (c *ConfidenceCalculator) calculateConsistentConfidence(sources []model.DetectionSource) float64 {
	// Calculate weighted average based on source priorities
	totalWeight := 0.0
	weightedSum := 0.0
	
	for _, src := range sources {
		priority := float64(c.config.GetSourcePriority(src.Source))
		if priority == 0 {
			priority = 1.0 // Minimum weight
		}
		
		totalWeight += priority
		weightedSum += src.Confidence * priority
	}
	
	var avg float64
	if totalWeight > 0 {
		avg = weightedSum / totalWeight
	} else {
		// Fallback to simple average
		sum := 0.0
		for _, src := range sources {
			sum += src.Confidence
		}
		avg = sum / float64(len(sources))
	}
	
	// Apply boost for additional consistent sources
	boost := c.config.MultiSourceBoost * float64(len(sources)-1)
	result := avg + boost
	
	return c.clamp(result)
}

// calculateConflictConfidence calculates confidence when sources conflict
// Formula: max_confidence - penalty
func (c *ConfidenceCalculator) calculateConflictConfidence(sources []model.DetectionSource) float64 {
	// Find the source with highest priority
	maxPriority := -1
	maxConfidence := 0.0
	
	for _, src := range sources {
		priority := c.config.GetSourcePriority(src.Source)
		if priority > maxPriority {
			maxPriority = priority
			maxConfidence = src.Confidence
		} else if priority == maxPriority {
			// Same priority, take higher confidence
			if src.Confidence > maxConfidence {
				maxConfidence = src.Confidence
			}
		}
	}
	
	// Apply conflict penalty
	result := maxConfidence - c.config.ConflictPenalty
	
	// Ensure minimum confidence of 0.3
	return math.Max(c.clamp(result), 0.3)
}

// clamp ensures confidence is in valid range [0.0, 1.0]
func (c *ConfidenceCalculator) clamp(confidence float64) float64 {
	if confidence < 0.0 {
		return 0.0
	}
	if confidence > 1.0 {
		return 1.0
	}
	return confidence
}

// CalculateWeighted calculates weighted confidence based on source priorities
func (c *ConfidenceCalculator) CalculateWeighted(sources []model.DetectionSource) float64 {
	if len(sources) == 0 {
		return 0.0
	}
	
	totalWeight := 0.0
	weightedSum := 0.0
	
	for _, src := range sources {
		priority := float64(c.config.GetSourcePriority(src.Source))
		if priority == 0 {
			priority = 1.0
		}
		
		totalWeight += priority
		weightedSum += src.Confidence * priority
	}
	
	if totalWeight > 0 {
		return c.clamp(weightedSum / totalWeight)
	}
	
	return 0.0
}

