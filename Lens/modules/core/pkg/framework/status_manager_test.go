package framework

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestStatusManager_DetermineStatus_SingleLowConfidence(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.4,
			DetectedAt: time.Now(),
		},
	}

	status := manager.DetermineStatus(0.4, sources)
	assert.Equal(t, model.DetectionStatusSuspected, status,
		"Low confidence single source should be suspected")
}

func TestStatusManager_DetermineStatus_SingleHighConfidence(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	sources := []model.DetectionSource{
		{
			Source:     "component",
			Frameworks: []string{"primus"},
			Confidence: 0.9,
			DetectedAt: time.Now(),
		},
	}

	status := manager.DetermineStatus(0.9, sources)
	assert.Equal(t, model.DetectionStatusConfirmed, status,
		"High confidence single source should be confirmed")
}

func TestStatusManager_DetermineStatus_MultiSourceVerified(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.8,
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Frameworks: []string{"primus"},
			Confidence: 0.9,
			DetectedAt: time.Now(),
		},
	}

	status := manager.DetermineStatus(0.95, sources)
	assert.Equal(t, model.DetectionStatusVerified, status,
		"Multiple consistent high-confidence sources should be verified")
}

func TestStatusManager_DetermineStatus_Conflict(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.8,
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Frameworks: []string{"deepspeed"},
			Confidence: 0.9,
			DetectedAt: time.Now(),
		},
	}

	status := manager.DetermineStatus(0.7, sources)
	assert.Equal(t, model.DetectionStatusConflict, status,
		"Conflicting sources should result in conflict status")
}

func TestStatusManager_DetermineStatus_ReuseOnly(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	sources := []model.DetectionSource{
		{
			Source:     "reuse",
			Frameworks: []string{"primus"},
			Confidence: 0.85,
			DetectedAt: time.Now(),
		},
	}

	status := manager.DetermineStatus(0.85, sources)
	assert.Equal(t, model.DetectionStatusReused, status,
		"Reuse-only source should have reused status")
}

func TestStatusManager_DetermineStatus_Unknown(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.1,
			DetectedAt: time.Now(),
		},
	}

	status := manager.DetermineStatus(0.1, sources)
	assert.Equal(t, model.DetectionStatusUnknown, status,
		"Very low confidence should be unknown")
}

func TestStatusManager_ValidateTransition(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	validTransitions := []struct {
		name string
		from model.DetectionStatus
		to   model.DetectionStatus
	}{
		{"Unknown to Suspected", model.DetectionStatusUnknown, model.DetectionStatusSuspected},
		{"Unknown to Confirmed", model.DetectionStatusUnknown, model.DetectionStatusConfirmed},
		{"Suspected to Confirmed", model.DetectionStatusSuspected, model.DetectionStatusConfirmed},
		{"Suspected to Verified", model.DetectionStatusSuspected, model.DetectionStatusVerified},
		{"Confirmed to Verified", model.DetectionStatusConfirmed, model.DetectionStatusVerified},
		{"Reused to Verified", model.DetectionStatusReused, model.DetectionStatusVerified},
		{"Conflict to Verified", model.DetectionStatusConflict, model.DetectionStatusVerified},
	}

	for _, tt := range validTransitions {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateTransition(tt.from, tt.to)
			assert.NoError(t, err, "Transition should be valid")
		})
	}
}

func TestStatusManager_ValidateTransition_Invalid(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	invalidTransitions := []struct {
		name string
		from model.DetectionStatus
		to   model.DetectionStatus
	}{
		{"Verified to Unknown", model.DetectionStatusVerified, model.DetectionStatusUnknown},
		{"Confirmed to Suspected", model.DetectionStatusConfirmed, model.DetectionStatusSuspected},
		{"Verified to Suspected", model.DetectionStatusVerified, model.DetectionStatusSuspected},
	}

	for _, tt := range invalidTransitions {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.ValidateTransition(tt.from, tt.to)
			assert.Error(t, err, "Transition should be invalid")
		})
	}
}

func TestStatusManager_GetStatusPriority(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	tests := []struct {
		status   model.DetectionStatus
		expected int
	}{
		{model.DetectionStatusVerified, 5},
		{model.DetectionStatusConfirmed, 4},
		{model.DetectionStatusReused, 3},
		{model.DetectionStatusSuspected, 2},
		{model.DetectionStatusConflict, 1},
		{model.DetectionStatusUnknown, 0},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			priority := manager.GetStatusPriority(tt.status)
			assert.Equal(t, tt.expected, priority)
		})
	}
}

func TestStatusManager_ShouldUpdate(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	tests := []struct {
		name          string
		oldStatus     model.DetectionStatus
		newStatus     model.DetectionStatus
		oldConfidence float64
		newConfidence float64
		shouldUpdate  bool
	}{
		{
			name:          "Unknown to Suspected",
			oldStatus:     model.DetectionStatusUnknown,
			newStatus:     model.DetectionStatusSuspected,
			oldConfidence: 0.0,
			newConfidence: 0.4,
			shouldUpdate:  true,
		},
		{
			name:          "Confidence significantly higher",
			oldStatus:     model.DetectionStatusSuspected,
			newStatus:     model.DetectionStatusSuspected,
			oldConfidence: 0.4,
			newConfidence: 0.7,
			shouldUpdate:  true,
		},
		{
			name:          "Higher priority status",
			oldStatus:     model.DetectionStatusSuspected,
			newStatus:     model.DetectionStatusVerified,
			oldConfidence: 0.4,
			newConfidence: 0.5,
			shouldUpdate:  true,
		},
		{
			name:          "Lower priority status",
			oldStatus:     model.DetectionStatusVerified,
			newStatus:     model.DetectionStatusSuspected,
			oldConfidence: 0.9,
			newConfidence: 0.5,
			shouldUpdate:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.ShouldUpdate(
				tt.oldStatus, tt.newStatus,
				tt.oldConfidence, tt.newConfidence,
			)
			assert.Equal(t, tt.shouldUpdate, result)
		})
	}
}

func TestStatusManager_HasConflict(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	tests := []struct {
		name     string
		sources  []model.DetectionSource
		expected bool
	}{
		{
			name:     "No sources",
			sources:  []model.DetectionSource{},
			expected: false,
		},
		{
			name: "Single source",
			sources: []model.DetectionSource{
				{Frameworks: []string{"primus"}},
			},
			expected: false,
		},
		{
			name: "No conflict",
			sources: []model.DetectionSource{
				{Frameworks: []string{"primus"}},
				{Frameworks: []string{"primus"}},
			},
			expected: false,
		},
		{
			name: "Has conflict",
			sources: []model.DetectionSource{
				{Frameworks: []string{"primus"}},
				{Frameworks: []string{"deepspeed"}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.hasConflict(tt.sources)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestStatusManager_IsReusedOnly(t *testing.T) {
	config := DefaultDetectionConfig()
	manager := NewStatusManager(config)

	tests := []struct {
		name     string
		sources  []model.DetectionSource
		expected bool
	}{
		{
			name:     "No sources",
			sources:  []model.DetectionSource{},
			expected: false,
		},
		{
			name: "Reuse only",
			sources: []model.DetectionSource{
				{Source: "reuse"},
			},
			expected: true,
		},
		{
			name: "Mixed sources",
			sources: []model.DetectionSource{
				{Source: "reuse"},
				{Source: "component"},
			},
			expected: false,
		},
		{
			name: "No reuse",
			sources: []model.DetectionSource{
				{Source: "log"},
				{Source: "component"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isReusedOnly(tt.sources)
			assert.Equal(t, tt.expected, result)
		})
	}
}
