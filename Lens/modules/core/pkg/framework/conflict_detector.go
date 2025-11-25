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

// DetectConflicts identifies all conflicts between sources
// A conflict exists when two sources report different frameworks
func (d *ConflictDetector) DetectConflicts(sources []model.DetectionSource) []model.DetectionConflict {
	if len(sources) <= 1 {
		return nil
	}
	
	var conflicts []model.DetectionConflict
	
	// Compare each pair of sources
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			if sources[i].Framework != sources[j].Framework {
				// Found a conflict
				conflict := model.DetectionConflict{
					Source1:    sources[i].Source,
					Source2:    sources[j].Source,
					Framework1: sources[i].Framework,
					Framework2: sources[j].Framework,
					Resolution: "pending",
					ResolvedAt: time.Time{}, // Will be set when resolved
				}
				conflicts = append(conflicts, conflict)
			}
		}
	}
	
	return conflicts
}

// HasConflict checks if there's any conflict between sources
func (d *ConflictDetector) HasConflict(sources []model.DetectionSource) bool {
	if len(sources) <= 1 {
		return false
	}
	
	firstFramework := sources[0].Framework
	for i := 1; i < len(sources); i++ {
		if sources[i].Framework != firstFramework {
			return true
		}
	}
	return false
}

// GetConflictingSources returns sources that are in conflict
// Returns map[framework][]source
func (d *ConflictDetector) GetConflictingSources(sources []model.DetectionSource) map[string][]string {
	result := make(map[string][]string)
	
	for _, src := range sources {
		result[src.Framework] = append(result[src.Framework], src.Source)
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
		frameworkSet[src.Framework] = true
	}
	
	var frameworks []string
	for fw := range frameworkSet {
		frameworks = append(frameworks, fw)
	}
	
	return frameworks
}

