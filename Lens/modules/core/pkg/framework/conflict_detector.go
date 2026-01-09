// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package framework

import (
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// ConflictDetector detects conflicts between detection sources
type ConflictDetector struct{}

// NewConflictDetector creates a new conflict detector
func NewConflictDetector() *ConflictDetector {
	return &ConflictDetector{}
}

// isWrapperFramework determines if the framework is a wrapper framework
func isWrapperFramework(framework string) bool {
	wrapperFrameworks := map[string]bool{
		"primus":               true,
		"lightning":            true,
		"pytorch_lightning":    true,
		"transformers_trainer": true,
	}
	return wrapperFrameworks[framework]
}

// isCompatibleFrameworkPair determines if two frameworks are compatible (not conflicting)
// If one is a wrapper framework and the other is a base framework, they are considered compatible
func isCompatibleFrameworkPair(fw1, fw2 string) bool {
	if fw1 == fw2 {
		return true // Same framework, compatible
	}

	isWrapper1 := isWrapperFramework(fw1)
	isWrapper2 := isWrapperFramework(fw2)

	// If one is wrapper and the other is base, they are compatible
	if isWrapper1 != isWrapper2 {
		return true
	}

	// If both are wrappers or both are base frameworks but different, they conflict
	return false
}

// extractFrameworkLayer extracts framework layer information from evidence
func extractFrameworkLayer(evidence map[string]interface{}) (wrapper string, base string) {
	if evidence == nil {
		return "", ""
	}

	// Try to extract wrapper_framework and base_framework from evidence
	if wf, ok := evidence["wrapper_framework"].(string); ok {
		wrapper = wf
	}
	if bf, ok := evidence["base_framework"].(string); ok {
		base = bf
	}

	return wrapper, base
}

// DetectConflicts identifies all conflicts between sources
// A conflict exists when two sources report different frameworks at the same layer
// Wrapper frameworks and base frameworks are considered compatible (not conflicting)
func (d *ConflictDetector) DetectConflicts(sources []model.DetectionSource) []model.DetectionConflict {
	if len(sources) <= 1 {
		return nil
	}

	var conflicts []model.DetectionConflict

	// Compare each pair of sources
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			// Extract framework layer information
			wrapper1, base1 := extractFrameworkLayer(sources[i].Evidence)
			wrapper2, base2 := extractFrameworkLayer(sources[j].Evidence)

			// Check if there is a conflict
			hasConflict := false
			conflictType := ""

			// If both sources have framework layer information, compare by layer
			if (wrapper1 != "" || base1 != "") && (wrapper2 != "" || base2 != "") {
				// Compare wrapper layer
				if wrapper1 != "" && wrapper2 != "" && wrapper1 != wrapper2 {
					hasConflict = true
					conflictType = "wrapper_layer_conflict"
				}
				// Compare base layer
				if base1 != "" && base2 != "" && base1 != base2 {
					hasConflict = true
					conflictType = "base_layer_conflict"
				}
			} else {
				// If no framework layer info, compare primary frameworks
				// Consider wrapper and base compatibility
				var frameworkI, frameworkJ string
				if len(sources[i].Frameworks) > 0 {
					frameworkI = sources[i].Frameworks[0]
				}
				if len(sources[j].Frameworks) > 0 {
					frameworkJ = sources[j].Frameworks[0]
				}

				if frameworkI != frameworkJ {
					if !isCompatibleFrameworkPair(frameworkI, frameworkJ) {
						hasConflict = true
						conflictType = "framework_conflict"
					}
				}
			}

			if hasConflict {
				// Found a conflict
				var framework1, framework2 string
				if len(sources[i].Frameworks) > 0 {
					framework1 = sources[i].Frameworks[0]
				}
				if len(sources[j].Frameworks) > 0 {
					framework2 = sources[j].Frameworks[0]
				}

				conflict := model.DetectionConflict{
					Source1:    sources[i].Source,
					Source2:    sources[j].Source,
					Framework1: framework1,
					Framework2: framework2,
					Resolution: conflictType,
					ResolvedAt: time.Time{}, // Will be set when resolved
				}
				conflicts = append(conflicts, conflict)
			}
		}
	}

	return conflicts
}

// HasConflict checks if there's any conflict between sources
// Returns true only if there are actual conflicts (incompatible frameworks)
func (d *ConflictDetector) HasConflict(sources []model.DetectionSource) bool {
	if len(sources) <= 1 {
		return false
	}

	// Use DetectConflicts to check for conflicts
	// This ensures consistency with the conflict detection logic
	conflicts := d.DetectConflicts(sources)
	return len(conflicts) > 0
}

// GetConflictingSources returns sources that are in conflict
// Returns map[framework][]source
func (d *ConflictDetector) GetConflictingSources(sources []model.DetectionSource) map[string][]string {
	result := make(map[string][]string)

	for _, src := range sources {
		// Use first framework from Frameworks array
		var framework string
		if len(src.Frameworks) > 0 {
			framework = src.Frameworks[0]
		}
		result[framework] = append(result[framework], src.Source)
	}

	// Only return if there's actual conflict (more than one framework)
	if len(result) <= 1 {
		return nil
	}

	return result
}

// CountConflicts returns the number of conflicts
func (d *ConflictDetector) CountConflicts(sources []model.DetectionSource) int {
	conflicts := d.DetectConflicts(sources)
	return len(conflicts)
}

// GetDistinctFrameworks returns all distinct frameworks from sources
func (d *ConflictDetector) GetDistinctFrameworks(sources []model.DetectionSource) []string {
	frameworkSet := make(map[string]bool)

	for _, src := range sources {
		// Collect all frameworks from Frameworks array
		for _, fw := range src.Frameworks {
			frameworkSet[fw] = true
		}
	}

	var frameworks []string
	for fw := range frameworkSet {
		frameworks = append(frameworks, fw)
	}

	return frameworks
}
