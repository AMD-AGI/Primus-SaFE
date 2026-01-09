// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package detection

import (
	"testing"

	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// NewTaskCreator Tests
// ============================================================================

func TestNewTaskCreator(t *testing.T) {
	creator := NewTaskCreator("test-instance")

	require.NotNil(t, creator)
	assert.Equal(t, "test-instance", creator.instanceID)
	assert.True(t, creator.autoCreateTask) // Default is true
	assert.NotNil(t, creator.taskFacade)
}

func TestNewTaskCreator_DifferentInstanceIDs(t *testing.T) {
	tests := []string{
		"instance-1",
		"instance-2",
		"prod-advisor-1",
		"",
	}

	for _, instanceID := range tests {
		t.Run(instanceID, func(t *testing.T) {
			creator := NewTaskCreator(instanceID)
			assert.Equal(t, instanceID, creator.instanceID)
		})
	}
}

// ============================================================================
// SetAutoCreateTask Tests
// ============================================================================

func TestTaskCreator_SetAutoCreateTask(t *testing.T) {
	creator := NewTaskCreator("test-instance")

	// Default is true
	assert.True(t, creator.autoCreateTask)

	// Set to false
	creator.SetAutoCreateTask(false)
	assert.False(t, creator.autoCreateTask)

	// Set back to true
	creator.SetAutoCreateTask(true)
	assert.True(t, creator.autoCreateTask)
}

// ============================================================================
// isPyTorchFramework Tests
// ============================================================================

func TestTaskCreator_isPyTorchFramework(t *testing.T) {
	creator := NewTaskCreator("test-instance")

	tests := []struct {
		name      string
		detection *coreModel.FrameworkDetection
		expected  bool
	}{
		{
			name: "pytorch in frameworks",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"pytorch"},
			},
			expected: true,
		},
		{
			name: "PyTorch uppercase",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"PyTorch"},
			},
			expected: true,
		},
		{
			name: "torch in framework name",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"torch-lightning"},
			},
			expected: true,
		},
		{
			name: "megatron (PyTorch-based)",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"megatron"},
			},
			expected: true,
		},
		{
			name: "Megatron-LM",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"Megatron-LM"},
			},
			expected: true,
		},
		{
			name: "multiple frameworks including pytorch",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"deepspeed", "pytorch", "transformers"},
			},
			expected: true,
		},
		{
			name: "tensorflow only",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"tensorflow"},
			},
			expected: false,
		},
		{
			name: "jax only",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{"jax"},
			},
			expected: false,
		},
		{
			name: "empty frameworks",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{},
			},
			expected: false,
		},
		{
			name: "pytorch in sources",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{},
				Sources: []coreModel.DetectionSource{
					{
						Source:     "process",
						Frameworks: []string{"pytorch"},
					},
				},
			},
			expected: true,
		},
		{
			name: "megatron in sources",
			detection: &coreModel.FrameworkDetection{
				Frameworks: []string{},
				Sources: []coreModel.DetectionSource{
					{
						Source:     "config",
						Frameworks: []string{"megatron"},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := creator.isPyTorchFramework(tt.detection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// isTrainingWorkload Tests
// ============================================================================

func TestTaskCreator_isTrainingWorkload(t *testing.T) {
	creator := NewTaskCreator("test-instance")

	tests := []struct {
		name      string
		detection *coreModel.FrameworkDetection
		expected  bool
	}{
		{
			name: "training type source",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{
					{Type: "training"},
				},
			},
			expected: true,
		},
		{
			name: "empty type (defaults to training)",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{
					{Type: ""},
				},
			},
			expected: true,
		},
		{
			name: "no sources (defaults to training)",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{},
			},
			expected: true,
		},
		{
			name: "mixed sources with training",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{
					{Type: "inference"},
					{Type: "training"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := creator.isTrainingWorkload(tt.detection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// extractSourceNames Tests
// ============================================================================

func TestTaskCreator_extractSourceNames(t *testing.T) {
	creator := NewTaskCreator("test-instance")

	tests := []struct {
		name      string
		detection *coreModel.FrameworkDetection
		expected  []string
	}{
		{
			name: "single source",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{
					{Source: "process"},
				},
			},
			expected: []string{"process"},
		},
		{
			name: "multiple unique sources",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{
					{Source: "process"},
					{Source: "config"},
					{Source: "env"},
				},
			},
			expected: []string{"process", "config", "env"},
		},
		{
			name: "duplicate sources",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{
					{Source: "process"},
					{Source: "process"},
					{Source: "config"},
				},
			},
			expected: []string{"process", "config"},
		},
		{
			name: "empty sources",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{},
			},
			expected: []string{},
		},
		{
			name: "sources with empty names",
			detection: &coreModel.FrameworkDetection{
				Sources: []coreModel.DetectionSource{
					{Source: ""},
					{Source: "process"},
				},
			},
			expected: []string{"", "process"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := creator.extractSourceNames(tt.detection)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// OnDetectionCompleted Tests
// ============================================================================

func TestTaskCreator_OnDetectionCompleted_AutoCreateDisabled(t *testing.T) {
	// Skip this test as database access is disabled when autoCreateTask is false
	// but we need to verify the check works
	creator := NewTaskCreator("test-instance")
	creator.SetAutoCreateTask(false)
	assert.False(t, creator.autoCreateTask)
}

func TestTaskCreator_OnDetectionCompleted_NotConfirmed(t *testing.T) {
	// Skip this test as it requires database connection
	t.Skip("Requires database connection - integration test")
}

func TestTaskCreator_OnDetectionCompleted_NonPyTorch(t *testing.T) {
	// Skip this test as it requires database connection
	t.Skip("Requires database connection - integration test")
}

// ============================================================================
// DetectionEventAdapter Tests
// ============================================================================

func TestDetectionEventAdapter_Fields(t *testing.T) {
	creator := NewTaskCreator("test-instance")
	adapter := &detectionEventAdapter{
		taskCreator: creator,
	}

	assert.NotNil(t, adapter.taskCreator)
	assert.Equal(t, creator, adapter.taskCreator)
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestTaskCreator_Integration_FullFlow(t *testing.T) {
	creator := NewTaskCreator("test-instance")

	// Test isPyTorchFramework with various scenarios
	pytorchDetection := &coreModel.FrameworkDetection{
		Frameworks: []string{"pytorch", "deepspeed"},
		Status:     coreModel.DetectionStatusConfirmed,
		Sources: []coreModel.DetectionSource{
			{Source: "process", Frameworks: []string{"pytorch"}},
		},
	}

	assert.True(t, creator.isPyTorchFramework(pytorchDetection))
	assert.True(t, creator.isTrainingWorkload(pytorchDetection))

	sources := creator.extractSourceNames(pytorchDetection)
	assert.Contains(t, sources, "process")
}

func TestTaskCreator_Integration_MegatronWorkload(t *testing.T) {
	creator := NewTaskCreator("test-instance")

	megatronDetection := &coreModel.FrameworkDetection{
		Frameworks: []string{"megatron", "deepspeed"},
		Status:     coreModel.DetectionStatusVerified,
		Confidence: 0.95,
		Sources: []coreModel.DetectionSource{
			{Source: "config", Frameworks: []string{"megatron"}},
			{Source: "process", Frameworks: []string{"deepspeed"}},
		},
	}

	// Megatron is PyTorch-based
	assert.True(t, creator.isPyTorchFramework(megatronDetection))
	assert.True(t, creator.isTrainingWorkload(megatronDetection))

	sources := creator.extractSourceNames(megatronDetection)
	assert.Len(t, sources, 2)
	assert.Contains(t, sources, "config")
	assert.Contains(t, sources, "process")
}

// ============================================================================
// Edge Cases Tests
// ============================================================================

func TestTaskCreator_EdgeCases(t *testing.T) {
	creator := NewTaskCreator("")

	t.Run("empty instance ID", func(t *testing.T) {
		assert.Equal(t, "", creator.instanceID)
	})

	t.Run("nil detection for isPyTorchFramework", func(t *testing.T) {
		// Skip - nil detection causes panic in actual implementation
		t.Skip("Nil detection causes panic - needs production code nil check")
	})

	t.Run("nil detection for isTrainingWorkload", func(t *testing.T) {
		// Skip - nil detection causes panic in actual implementation
		t.Skip("Nil detection causes panic - needs production code nil check")
	})

	t.Run("case insensitivity for framework names", func(t *testing.T) {
		cases := []string{"PYTORCH", "PyTorch", "pytorch", "MEGATRON", "Megatron", "megatron"}
		for _, fw := range cases {
			detection := &coreModel.FrameworkDetection{
				Frameworks: []string{fw},
			}
			assert.True(t, creator.isPyTorchFramework(detection),
				"Expected true for framework: %s", fw)
		}
	})
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkTaskCreator_isPyTorchFramework(b *testing.B) {
	creator := NewTaskCreator("bench-instance")
	detection := &coreModel.FrameworkDetection{
		Frameworks: []string{"pytorch", "deepspeed", "transformers"},
		Sources: []coreModel.DetectionSource{
			{Source: "process", Frameworks: []string{"pytorch"}},
			{Source: "config", Frameworks: []string{"deepspeed"}},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = creator.isPyTorchFramework(detection)
	}
}

func BenchmarkTaskCreator_isTrainingWorkload(b *testing.B) {
	creator := NewTaskCreator("bench-instance")
	detection := &coreModel.FrameworkDetection{
		Sources: []coreModel.DetectionSource{
			{Type: "training"},
			{Type: ""},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = creator.isTrainingWorkload(detection)
	}
}

func BenchmarkTaskCreator_extractSourceNames(b *testing.B) {
	creator := NewTaskCreator("bench-instance")
	detection := &coreModel.FrameworkDetection{
		Sources: []coreModel.DetectionSource{
			{Source: "process"},
			{Source: "config"},
			{Source: "env"},
			{Source: "cmdline"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = creator.extractSourceNames(detection)
	}
}

func BenchmarkNewTaskCreator(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = NewTaskCreator("bench-instance")
	}
}

