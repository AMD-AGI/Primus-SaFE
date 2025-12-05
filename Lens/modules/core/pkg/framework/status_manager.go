package framework

import (
	"fmt"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// StatusManager manages detection status transitions
type StatusManager struct {
	config *DetectionConfig
}

// NewStatusManager creates a new status manager
func NewStatusManager(config *DetectionConfig) *StatusManager {
	return &StatusManager{
		config: config,
	}
}

// DetermineStatus determines the detection status based on confidence and sources
// Rules:
// 1. If conflict exists -> conflict
// 2. If only reuse source -> reused
// 3. If confidence >= verified threshold and multiple sources -> verified
// 4. If confidence >= confirmed threshold -> confirmed
// 5. If confidence >= suspected threshold -> suspected
// 6. Otherwise -> unknown
func (s *StatusManager) DetermineStatus(
	confidence float64,
	sources []model.DetectionSource,
) model.DetectionStatus {
	// Check for conflicts
	if s.hasConflict(sources) {
		return model.DetectionStatusConflict
	}

	// Check if only reuse source
	if s.isReusedOnly(sources) {
		return model.DetectionStatusReused
	}

	// Determine status based on confidence and source count
	if confidence >= s.config.VerifiedThreshold {
		if len(sources) > 1 {
			return model.DetectionStatusVerified
		}
		return model.DetectionStatusConfirmed
	} else if confidence >= s.config.ConfirmedThreshold {
		return model.DetectionStatusConfirmed
	} else if confidence >= s.config.SuspectedThreshold {
		return model.DetectionStatusSuspected
	}

	return model.DetectionStatusUnknown
}

// hasConflict checks if sources have conflicting frameworks
func (s *StatusManager) hasConflict(sources []model.DetectionSource) bool {
	if len(sources) <= 1 {
		return false
	}

	// Compare first framework from each source's Frameworks array
	var firstFramework string
	if len(sources[0].Frameworks) > 0 {
		firstFramework = sources[0].Frameworks[0]
	}

	for i := 1; i < len(sources); i++ {
		var currentFramework string
		if len(sources[i].Frameworks) > 0 {
			currentFramework = sources[i].Frameworks[0]
		}
		if currentFramework != firstFramework {
			return true
		}
	}
	return false
}

// isReusedOnly checks if sources contain only reuse
func (s *StatusManager) isReusedOnly(sources []model.DetectionSource) bool {
	if len(sources) == 0 {
		return false
	}

	for _, src := range sources {
		if src.Source != "reuse" {
			return false
		}
	}
	return true
}

// ValidateTransition validates if a status transition is valid
// Valid transitions:
// unknown -> suspected/confirmed/verified/reused
// suspected -> confirmed/verified/conflict
// confirmed -> verified/conflict
// verified -> conflict (unusual but possible)
// reused -> confirmed/verified (when additional sources confirm)
// conflict -> verified (when conflict is resolved)
func (s *StatusManager) ValidateTransition(
	from, to model.DetectionStatus,
) error {
	// Define valid transitions
	validTransitions := map[model.DetectionStatus][]model.DetectionStatus{
		model.DetectionStatusUnknown: {
			model.DetectionStatusSuspected,
			model.DetectionStatusConfirmed,
			model.DetectionStatusVerified,
			model.DetectionStatusReused,
		},
		model.DetectionStatusSuspected: {
			model.DetectionStatusConfirmed,
			model.DetectionStatusVerified,
			model.DetectionStatusConflict,
		},
		model.DetectionStatusConfirmed: {
			model.DetectionStatusVerified,
			model.DetectionStatusConflict,
		},
		model.DetectionStatusVerified: {
			model.DetectionStatusConflict,
		},
		model.DetectionStatusReused: {
			model.DetectionStatusConfirmed,
			model.DetectionStatusVerified,
			model.DetectionStatusConflict,
		},
		model.DetectionStatusConflict: {
			model.DetectionStatusVerified,
			model.DetectionStatusConfirmed,
		},
	}

	// Check if transition is valid
	allowedTargets, ok := validTransitions[from]
	if !ok {
		return fmt.Errorf("invalid source status: %s", from)
	}

	for _, allowed := range allowedTargets {
		if to == allowed {
			return nil // Valid transition
		}
	}

	return fmt.Errorf("invalid transition from %s to %s", from, to)
}

// GetStatusPriority returns the priority level of a status (higher = more certain)
func (s *StatusManager) GetStatusPriority(status model.DetectionStatus) int {
	priorities := map[model.DetectionStatus]int{
		model.DetectionStatusVerified:  5,
		model.DetectionStatusConfirmed: 4,
		model.DetectionStatusReused:    3,
		model.DetectionStatusSuspected: 2,
		model.DetectionStatusConflict:  1,
		model.DetectionStatusUnknown:   0,
	}

	if priority, ok := priorities[status]; ok {
		return priority
	}
	return 0
}

// ShouldUpdate determines if new status should replace old status
func (s *StatusManager) ShouldUpdate(
	oldStatus, newStatus model.DetectionStatus,
	oldConfidence, newConfidence float64,
) bool {
	// Always update if old status is unknown
	if oldStatus == model.DetectionStatusUnknown {
		return true
	}

	// Always update if new confidence is significantly higher (>0.2 difference)
	if newConfidence-oldConfidence > 0.2 {
		return true
	}

	// Update if new status has higher priority
	oldPriority := s.GetStatusPriority(oldStatus)
	newPriority := s.GetStatusPriority(newStatus)

	return newPriority > oldPriority
}
