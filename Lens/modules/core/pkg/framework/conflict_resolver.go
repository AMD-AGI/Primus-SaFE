package framework

import (
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ConflictResolver resolves conflicts between detection sources
type ConflictResolver struct {
	config *DetectionConfig
}

// NewConflictResolver creates a new conflict resolver
func NewConflictResolver(config *DetectionConfig) *ConflictResolver {
	return &ConflictResolver{
		config: config,
	}
}

// Resolve resolves conflict and returns the winning source
// Resolution strategy:
// 1. Priority: Choose source with highest priority
// 2. Confidence: If same priority, choose highest confidence
// 3. Time: If same priority and confidence, choose most recent
func (r *ConflictResolver) Resolve(sources []model.DetectionSource) (*model.DetectionSource, error) {
	if len(sources) == 0 {
		return nil, fmt.Errorf("no sources to resolve")
	}

	if len(sources) == 1 {
		return &sources[0], nil
	}

	// Find the best source using resolution strategy
	bestIndex := 0

	for i := 1; i < len(sources); i++ {
		if r.isBetter(&sources[i], &sources[bestIndex]) {
			bestIndex = i
		}
	}

	return &sources[bestIndex], nil
}

// ResolveWithReason resolves conflict and returns the winning source with explanation
func (r *ConflictResolver) ResolveWithReason(sources []model.DetectionSource) (*model.DetectionSource, string, error) {
	if len(sources) == 0 {
		return nil, "", fmt.Errorf("no sources to resolve")
	}

	if len(sources) == 1 {
		return &sources[0], "single_source", nil
	}

	// Find the best source and track reason
	bestIndex := 0
	reason := "equal"

	for i := 1; i < len(sources); i++ {
		comparison := r.compare(&sources[i], &sources[bestIndex])
		if comparison > 0 {
			// Get reason before updating bestIndex
			reason = r.getComparisonReason(&sources[i], &sources[bestIndex])
			bestIndex = i
		}
	}

	// If bestIndex is still 0, find a reason by comparing with another source
	if bestIndex == 0 && len(sources) > 1 {
		// Compare the winning source with the first different source
		for i := 1; i < len(sources); i++ {
			comparison := r.compare(&sources[0], &sources[i])
			if comparison != 0 {
				reason = r.getComparisonReason(&sources[0], &sources[i])
				break
			}
		}
	}

	return &sources[bestIndex], reason, nil
}

// isBetter determines if source a is better than source b
func (r *ConflictResolver) isBetter(a, b *model.DetectionSource) bool {
	return r.compare(a, b) > 0
}

// compare compares two sources
// Returns: >0 if a is better, <0 if b is better, 0 if equal
func (r *ConflictResolver) compare(a, b *model.DetectionSource) int {
	// Strategy 1: Compare priority
	priorityA := r.config.GetSourcePriority(a.Source)
	priorityB := r.config.GetSourcePriority(b.Source)

	if priorityA > priorityB {
		return 1
	} else if priorityA < priorityB {
		return -1
	}

	// Strategy 2: Same priority, compare confidence
	if a.Confidence > b.Confidence {
		return 1
	} else if a.Confidence < b.Confidence {
		return -1
	}

	// Strategy 3: Same priority and confidence, compare time (newer is better)
	if a.DetectedAt.After(b.DetectedAt) {
		return 1
	} else if a.DetectedAt.Before(b.DetectedAt) {
		return -1
	}

	return 0
}

// getComparisonReason returns the reason why source a is better than b
func (r *ConflictResolver) getComparisonReason(a, b *model.DetectionSource) string {
	priorityA := r.config.GetSourcePriority(a.Source)
	priorityB := r.config.GetSourcePriority(b.Source)

	if priorityA > priorityB {
		return fmt.Sprintf("higher_priority_%s(%d)_vs_%s(%d)",
			a.Source, priorityA, b.Source, priorityB)
	} else if priorityA < priorityB {
		return fmt.Sprintf("higher_priority_%s(%d)_vs_%s(%d)",
			b.Source, priorityB, a.Source, priorityA)
	}

	if a.Confidence > b.Confidence {
		return fmt.Sprintf("higher_confidence_%.2f_vs_%.2f", a.Confidence, b.Confidence)
	} else if a.Confidence < b.Confidence {
		return fmt.Sprintf("higher_confidence_%.2f_vs_%.2f", b.Confidence, a.Confidence)
	}

	if a.DetectedAt.After(b.DetectedAt) {
		return "more_recent"
	} else if a.DetectedAt.Before(b.DetectedAt) {
		return "more_recent"
	}

	return "equal"
}

// ResolveAll resolves all conflicts and returns frameworks grouped by priority
func (r *ConflictResolver) ResolveAll(sources []model.DetectionSource) map[string][]string {
	result := make(map[string][]string)

	for _, src := range sources {
		// Use first framework from Frameworks array
		var framework string
		if len(src.Frameworks) > 0 {
			framework = src.Frameworks[0]
		}
		result[framework] = append(result[framework], src.Source)
	}

	return result
}

// GetWinningFramework returns the framework that wins the conflict resolution
func (r *ConflictResolver) GetWinningFramework(sources []model.DetectionSource) (string, error) {
	winningSource, err := r.Resolve(sources)
	if err != nil {
		return "", err
	}
	// Return first framework from winning source's Frameworks array
	if len(winningSource.Frameworks) > 0 {
		return winningSource.Frameworks[0], nil
	}
	return "", nil
}
