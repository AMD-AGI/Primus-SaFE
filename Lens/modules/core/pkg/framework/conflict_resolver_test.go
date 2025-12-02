package framework

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestConflictResolver_Resolve_ByPriority(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	sources := []model.DetectionSource{
		{
			Source:     "log", // Priority 60
			Frameworks: []string{"primus"},
			Confidence: 0.9,
			DetectedAt: time.Now(),
		},
		{
			Source:     "component", // Priority 80 (higher)
			Frameworks: []string{"deepspeed"},
			Confidence: 0.7,
			DetectedAt: time.Now(),
		},
	}

	result, err := resolver.Resolve(sources)
	require.NoError(t, err)
	assert.Equal(t, "component", result.Source, "Should choose higher priority source")
	assert.Equal(t, []string{"deepspeed"}, result.Frameworks)
}

func TestConflictResolver_Resolve_ByConfidence(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	now := time.Now()
	sources := []model.DetectionSource{
		{
			Source:     "log", // Same priority
			Frameworks: []string{"primus"},
			Confidence: 0.9, // Higher confidence
			DetectedAt: now,
		},
		{
			Source:     "log", // Same priority and source type
			Frameworks: []string{"deepspeed"},
			Confidence: 0.7,
			DetectedAt: now,
		},
	}

	result, err := resolver.Resolve(sources)
	require.NoError(t, err)
	assert.Equal(t, []string{"primus"}, result.Frameworks, "Should choose higher confidence when priority is same")
}

func TestConflictResolver_Resolve_ByTime(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	older := time.Now().Add(-1 * time.Hour)
	newer := time.Now()

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.8,
			DetectedAt: older,
		},
		{
			Source:     "log",
			Frameworks: []string{"deepspeed"},
			Confidence: 0.8, // Same confidence
			DetectedAt: newer,
		},
	}

	result, err := resolver.Resolve(sources)
	require.NoError(t, err)
	assert.Equal(t, []string{"deepspeed"}, result.Frameworks, "Should choose more recent when priority and confidence are same")
}

func TestConflictResolver_Resolve_EmptySources(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	sources := []model.DetectionSource{}

	result, err := resolver.Resolve(sources)
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestConflictResolver_Resolve_SingleSource(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.8,
			DetectedAt: time.Now(),
		},
	}

	result, err := resolver.Resolve(sources)
	require.NoError(t, err)
	assert.Equal(t, []string{"primus"}, result.Frameworks)
}

func TestConflictResolver_ResolveWithReason_Priority(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.9,
			DetectedAt: time.Now(),
		},
		{
			Source:     "component",
			Frameworks: []string{"deepspeed"},
			Confidence: 0.7,
			DetectedAt: time.Now(),
		},
	}

	result, reason, err := resolver.ResolveWithReason(sources)
	require.NoError(t, err)
	assert.Equal(t, []string{"deepspeed"}, result.Frameworks)
	assert.Contains(t, reason, "priority", "Reason should mention priority")
}

func TestConflictResolver_ResolveWithReason_Confidence(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	now := time.Now()
	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.9,
			DetectedAt: now,
		},
		{
			Source:     "log",
			Frameworks: []string{"deepspeed"},
			Confidence: 0.7,
			DetectedAt: now,
		},
	}

	result, reason, err := resolver.ResolveWithReason(sources)
	require.NoError(t, err)
	assert.Equal(t, []string{"primus"}, result.Frameworks)
	assert.Contains(t, reason, "confidence", "Reason should mention confidence")
}

func TestConflictResolver_ResolveWithReason_Time(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	older := time.Now().Add(-1 * time.Hour)
	newer := time.Now()

	sources := []model.DetectionSource{
		{
			Source:     "log",
			Frameworks: []string{"primus"},
			Confidence: 0.8,
			DetectedAt: older,
		},
		{
			Source:     "log",
			Frameworks: []string{"deepspeed"},
			Confidence: 0.8,
			DetectedAt: newer,
		},
	}

	result, reason, err := resolver.ResolveWithReason(sources)
	require.NoError(t, err)
	assert.Equal(t, []string{"deepspeed"}, result.Frameworks)
	assert.Contains(t, reason, "recent", "Reason should mention time")
}

func TestConflictResolver_GetWinningFramework(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	sources := []model.DetectionSource{
		{
			Source:     "component",
			Frameworks: []string{"primus"},
			Confidence: 0.9,
			DetectedAt: time.Now(),
		},
		{
			Source:     "log",
			Frameworks: []string{"deepspeed"},
			Confidence: 0.8,
			DetectedAt: time.Now(),
		},
	}

	framework, err := resolver.GetWinningFramework(sources)
	require.NoError(t, err)
	assert.Equal(t, "primus", framework, "Component priority is higher")
}

func TestConflictResolver_Compare(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	now := time.Now()

	tests := []struct {
		name     string
		sourceA  model.DetectionSource
		sourceB  model.DetectionSource
		expected int // >0 if A better, <0 if B better, 0 if equal
	}{
		{
			name: "A has higher priority",
			sourceA: model.DetectionSource{
				Source:     "component", // Priority 80
				Confidence: 0.7,
				DetectedAt: now,
			},
			sourceB: model.DetectionSource{
				Source:     "log", // Priority 60
				Confidence: 0.9,
				DetectedAt: now,
			},
			expected: 1,
		},
		{
			name: "B has higher priority",
			sourceA: model.DetectionSource{
				Source:     "log", // Priority 60
				Confidence: 0.9,
				DetectedAt: now,
			},
			sourceB: model.DetectionSource{
				Source:     "component", // Priority 80
				Confidence: 0.7,
				DetectedAt: now,
			},
			expected: -1,
		},
		{
			name: "Same priority, A has higher confidence",
			sourceA: model.DetectionSource{
				Source:     "log",
				Confidence: 0.9,
				DetectedAt: now,
			},
			sourceB: model.DetectionSource{
				Source:     "log",
				Confidence: 0.7,
				DetectedAt: now,
			},
			expected: 1,
		},
		{
			name: "Same priority and confidence, A is newer",
			sourceA: model.DetectionSource{
				Source:     "log",
				Confidence: 0.8,
				DetectedAt: now.Add(1 * time.Second),
			},
			sourceB: model.DetectionSource{
				Source:     "log",
				Confidence: 0.8,
				DetectedAt: now,
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.compare(&tt.sourceA, &tt.sourceB)
			if tt.expected > 0 {
				assert.True(t, result > 0, "A should be better than B")
			} else if tt.expected < 0 {
				assert.True(t, result < 0, "B should be better than A")
			} else {
				assert.Equal(t, 0, result, "A and B should be equal")
			}
		})
	}
}

func TestConflictResolver_UserSourceAlwaysWins(t *testing.T) {
	config := DefaultDetectionConfig()
	resolver := NewConflictResolver(config)

	sources := []model.DetectionSource{
		{
			Source:     "component",
			Frameworks: []string{"primus"},
			Confidence: 0.95,
			DetectedAt: time.Now(),
		},
		{
			Source:     "user", // Highest priority (100)
			Frameworks: []string{"deepspeed"},
			Confidence: 0.5,                            // Even with lower confidence
			DetectedAt: time.Now().Add(-1 * time.Hour), // Even if older
		},
	}

	result, err := resolver.Resolve(sources)
	require.NoError(t, err)
	assert.Equal(t, "user", result.Source, "User source should always win")
	assert.Equal(t, []string{"deepspeed"}, result.Frameworks)
}
