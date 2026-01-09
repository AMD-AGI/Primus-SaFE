// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package profiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// PrimusExtractor Tests
// ============================================================================

func TestNewPrimusExtractor(t *testing.T) {
	extractor := NewPrimusExtractor()
	assert.NotNil(t, extractor)
}

func TestPrimusExtractor_FrameworkType(t *testing.T) {
	extractor := NewPrimusExtractor()
	assert.Equal(t, "primus", extractor.FrameworkType())
}

func TestPrimusExtractor_Priority(t *testing.T) {
	extractor := NewPrimusExtractor()
	assert.Equal(t, 10, extractor.Priority())
}

func TestPrimusExtractor_CanHandle(t *testing.T) {
	extractor := NewPrimusExtractor()

	tests := []struct {
		name      string
		rawConfig map[string]interface{}
		expected  bool
	}{
		{
			name: "valid primus config with pre_trainer",
			rawConfig: map[string]interface{}{
				"modules": map[string]interface{}{
					"pre_trainer": map[string]interface{}{
						"enabled": true,
					},
				},
			},
			expected: true,
		},
		{
			name: "primus config with empty pre_trainer",
			rawConfig: map[string]interface{}{
				"modules": map[string]interface{}{
					"pre_trainer": map[string]interface{}{},
				},
			},
			expected: true,
		},
		{
			name: "config without modules",
			rawConfig: map[string]interface{}{
				"workspace": "/output",
			},
			expected: false,
		},
		{
			name: "config with modules but no pre_trainer",
			rawConfig: map[string]interface{}{
				"modules": map[string]interface{}{
					"other_module": map[string]interface{}{},
				},
			},
			expected: false,
		},
		{
			name:      "empty config",
			rawConfig: map[string]interface{}{},
			expected:  false,
		},
		{
			name:      "nil config",
			rawConfig: nil,
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.CanHandle(tt.rawConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrimusExtractor_ExtractPaths(t *testing.T) {
	extractor := NewPrimusExtractor()

	tests := []struct {
		name           string
		rawConfig      map[string]interface{}
		env            map[string]string
		expectedPaths  *ExtractedPaths
		checkProfiler  bool
		profilerPrefix string
	}{
		{
			name: "full primus config",
			rawConfig: map[string]interface{}{
				"workspace":  "/output",
				"work_group": "amd-team",
				"user_name":  "testuser",
				"exp_name":   "my-experiment",
			},
			env:            nil,
			checkProfiler:  true,
			profilerPrefix: "/output/amd-team/testuser/my-experiment/tensorboard",
		},
		{
			name: "config with environment variables",
			rawConfig: map[string]interface{}{
				"workspace":  "/output",
				"work_group": "${TEAM:default-team}",
				"user_name":  "${USER:root}",
				"exp_name":   "test-exp",
			},
			env: map[string]string{
				"TEAM": "my-team",
				"USER": "myuser",
			},
			checkProfiler:  true,
			profilerPrefix: "/output/my-team/myuser/test-exp/tensorboard",
		},
		{
			name: "config with profiler settings",
			rawConfig: map[string]interface{}{
				"workspace":  "/output",
				"work_group": "team",
				"user_name":  "user",
				"exp_name":   "exp",
				"modules": map[string]interface{}{
					"pre_trainer": map[string]interface{}{
						"overrides": map[string]interface{}{
							"profile":              true,
							"use_pytorch_profiler": true,
							"torch_profiler_use_gzip": true,
						},
					},
				},
			},
			env:            nil,
			checkProfiler:  true,
			profilerPrefix: "/output/team/user/exp/tensorboard",
		},
		{
			name:           "empty config uses defaults",
			rawConfig:      map[string]interface{}{},
			env:            nil,
			checkProfiler:  true,
			profilerPrefix: "./output",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := extractor.ExtractPaths(tt.rawConfig, tt.env)
			require.NotNil(t, paths)

			if tt.checkProfiler {
				assert.Contains(t, paths.ProfilerDir, "tensorboard")
			}
			assert.NotEmpty(t, paths.TensorBoardDir)
		})
	}
}

func TestPrimusExtractor_GetProfilerLocations(t *testing.T) {
	extractor := NewPrimusExtractor()

	tests := []struct {
		name           string
		config         *FrameworkConfig
		expectedLen    int
		expectedSource string
	}{
		{
			name:        "nil config",
			config:      nil,
			expectedLen: 0,
		},
		{
			name: "config with nil extracted paths",
			config: &FrameworkConfig{
				Framework:      "primus",
				ExtractedPaths: nil,
			},
			expectedLen: 0,
		},
		{
			name: "config with empty profiler dir",
			config: &FrameworkConfig{
				Framework: "primus",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "",
				},
			},
			expectedLen: 0,
		},
		{
			name: "config with valid profiler dir",
			config: &FrameworkConfig{
				Framework: "primus",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "/output/tensorboard",
					CustomPaths: map[string]string{
						"exp_name": "test-exp",
					},
				},
			},
			expectedLen:    1,
			expectedSource: "primus_config",
		},
		{
			name: "config with gzip enabled",
			config: &FrameworkConfig{
				Framework: "primus",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "/output/tensorboard",
					CustomPaths: map[string]string{
						"exp_name": "test-exp",
						"use_gzip": "true",
					},
				},
			},
			expectedLen:    1,
			expectedSource: "primus_config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := extractor.GetProfilerLocations(tt.config)
			assert.Len(t, locations, tt.expectedLen)

			if tt.expectedLen > 0 {
				assert.Equal(t, tt.expectedSource, locations[0].Source)
				assert.NotEmpty(t, locations[0].Patterns)
			}
		})
	}
}

func TestPrimusExtractor_GetTensorBoardDir(t *testing.T) {
	extractor := NewPrimusExtractor()

	tests := []struct {
		name     string
		config   *FrameworkConfig
		expected string
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: "",
		},
		{
			name: "config with tensorboard dir",
			config: &FrameworkConfig{
				ExtractedPaths: &ExtractedPaths{
					TensorBoardDir: "/output/tensorboard",
				},
			},
			expected: "/output/tensorboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.GetTensorBoardDir(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrimusExtractor_GetCheckpointDir(t *testing.T) {
	extractor := NewPrimusExtractor()

	tests := []struct {
		name     string
		config   *FrameworkConfig
		expected string
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: "",
		},
		{
			name: "config with checkpoint dir",
			config: &FrameworkConfig{
				ExtractedPaths: &ExtractedPaths{
					CheckpointDir: "/output/checkpoints",
				},
			},
			expected: "/output/checkpoints",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.GetCheckpointDir(tt.config)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// MegatronExtractor Tests
// ============================================================================

func TestNewMegatronExtractor(t *testing.T) {
	extractor := NewMegatronExtractor()
	assert.NotNil(t, extractor)
}

func TestMegatronExtractor_FrameworkType(t *testing.T) {
	extractor := NewMegatronExtractor()
	assert.Equal(t, "megatron", extractor.FrameworkType())
}

func TestMegatronExtractor_Priority(t *testing.T) {
	extractor := NewMegatronExtractor()
	assert.Equal(t, 20, extractor.Priority())
}

func TestMegatronExtractor_CanHandle(t *testing.T) {
	extractor := NewMegatronExtractor()

	tests := []struct {
		name      string
		rawConfig map[string]interface{}
		expected  bool
	}{
		{
			name: "config with tensorboard-dir",
			rawConfig: map[string]interface{}{
				"tensorboard-dir": "/output/tensorboard",
			},
			expected: true,
		},
		{
			name: "config with save and load",
			rawConfig: map[string]interface{}{
				"save": "/output/checkpoints",
				"load": "/output/checkpoints",
			},
			expected: true,
		},
		{
			name: "config with only save",
			rawConfig: map[string]interface{}{
				"save": "/output/checkpoints",
			},
			expected: false,
		},
		{
			name:      "empty config",
			rawConfig: map[string]interface{}{},
			expected:  false,
		},
		{
			name: "unrelated config",
			rawConfig: map[string]interface{}{
				"learning_rate": 0.001,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.CanHandle(tt.rawConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMegatronExtractor_ExtractPaths(t *testing.T) {
	extractor := NewMegatronExtractor()

	tests := []struct {
		name         string
		rawConfig    map[string]interface{}
		env          map[string]string
		expectedTB   string
		expectedSave string
	}{
		{
			name: "full megatron config",
			rawConfig: map[string]interface{}{
				"tensorboard-dir": "/output/tensorboard",
				"save":            "/output/checkpoints",
			},
			env:          nil,
			expectedTB:   "/output/tensorboard",
			expectedSave: "/output/checkpoints",
		},
		{
			name: "config with profile-dir",
			rawConfig: map[string]interface{}{
				"tensorboard-dir": "/output/tensorboard",
				"profile-dir":     "/output/profiler",
				"save":            "/output/checkpoints",
			},
			env:          nil,
			expectedTB:   "/output/tensorboard",
			expectedSave: "/output/checkpoints",
		},
		{
			name: "config with environment variables",
			rawConfig: map[string]interface{}{
				"tensorboard-dir": "${OUTPUT_DIR}/tensorboard",
				"save":            "${OUTPUT_DIR}/checkpoints",
			},
			env: map[string]string{
				"OUTPUT_DIR": "/data",
			},
			expectedTB:   "/data/tensorboard",
			expectedSave: "/data/checkpoints",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := extractor.ExtractPaths(tt.rawConfig, tt.env)
			require.NotNil(t, paths)

			if tt.expectedTB != "" {
				assert.Equal(t, tt.expectedTB, paths.TensorBoardDir)
			}
			if tt.expectedSave != "" {
				assert.Equal(t, tt.expectedSave, paths.CheckpointDir)
			}
		})
	}
}

func TestMegatronExtractor_GetProfilerLocations(t *testing.T) {
	extractor := NewMegatronExtractor()

	tests := []struct {
		name        string
		config      *FrameworkConfig
		expectedLen int
	}{
		{
			name:        "nil config",
			config:      nil,
			expectedLen: 0,
		},
		{
			name: "config with profiler dir",
			config: &FrameworkConfig{
				Framework: "megatron",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "/output/profiler",
				},
			},
			expectedLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := extractor.GetProfilerLocations(tt.config)
			assert.Len(t, locations, tt.expectedLen)

			if tt.expectedLen > 0 {
				assert.Equal(t, "megatron_config", locations[0].Source)
				assert.True(t, locations[0].Recursive)
				assert.Equal(t, 2, locations[0].MaxDepth)
			}
		})
	}
}

// ============================================================================
// DeepSpeedExtractor Tests
// ============================================================================

func TestNewDeepSpeedExtractor(t *testing.T) {
	extractor := NewDeepSpeedExtractor()
	assert.NotNil(t, extractor)
}

func TestDeepSpeedExtractor_FrameworkType(t *testing.T) {
	extractor := NewDeepSpeedExtractor()
	assert.Equal(t, "deepspeed", extractor.FrameworkType())
}

func TestDeepSpeedExtractor_Priority(t *testing.T) {
	extractor := NewDeepSpeedExtractor()
	assert.Equal(t, 30, extractor.Priority())
}

func TestDeepSpeedExtractor_CanHandle(t *testing.T) {
	extractor := NewDeepSpeedExtractor()

	tests := []struct {
		name      string
		rawConfig map[string]interface{}
		expected  bool
	}{
		{
			name: "config with zero_optimization",
			rawConfig: map[string]interface{}{
				"zero_optimization": map[string]interface{}{
					"stage": 2,
				},
			},
			expected: true,
		},
		{
			name: "config with fp16",
			rawConfig: map[string]interface{}{
				"fp16": map[string]interface{}{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name: "config with flops_profiler",
			rawConfig: map[string]interface{}{
				"flops_profiler": map[string]interface{}{
					"enabled": true,
				},
			},
			expected: true,
		},
		{
			name:      "empty config",
			rawConfig: map[string]interface{}{},
			expected:  false,
		},
		{
			name: "unrelated config",
			rawConfig: map[string]interface{}{
				"learning_rate": 0.001,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.CanHandle(tt.rawConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeepSpeedExtractor_ExtractPaths(t *testing.T) {
	extractor := NewDeepSpeedExtractor()

	tests := []struct {
		name             string
		rawConfig        map[string]interface{}
		env              map[string]string
		expectedProfiler string
		expectedTB       string
	}{
		{
			name: "config with flops_profiler",
			rawConfig: map[string]interface{}{
				"flops_profiler": map[string]interface{}{
					"output_file": "/output/profiler",
				},
			},
			env:              nil,
			expectedProfiler: "/output/profiler",
		},
		{
			name: "config with tensorboard",
			rawConfig: map[string]interface{}{
				"tensorboard": map[string]interface{}{
					"output_path": "/output/tensorboard",
				},
			},
			env:        nil,
			expectedTB: "/output/tensorboard",
		},
		{
			name: "full deepspeed config",
			rawConfig: map[string]interface{}{
				"flops_profiler": map[string]interface{}{
					"output_file": "/output/profiler",
				},
				"tensorboard": map[string]interface{}{
					"output_path": "/output/tensorboard",
				},
				"checkpoint": map[string]interface{}{
					"save_path": "/output/checkpoints",
				},
			},
			env:              nil,
			expectedProfiler: "/output/profiler",
			expectedTB:       "/output/tensorboard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := extractor.ExtractPaths(tt.rawConfig, tt.env)
			require.NotNil(t, paths)

			if tt.expectedProfiler != "" {
				assert.Equal(t, tt.expectedProfiler, paths.ProfilerDir)
			}
			if tt.expectedTB != "" {
				assert.Equal(t, tt.expectedTB, paths.TensorBoardDir)
			}
		})
	}
}

func TestDeepSpeedExtractor_GetProfilerLocations(t *testing.T) {
	extractor := NewDeepSpeedExtractor()

	tests := []struct {
		name        string
		config      *FrameworkConfig
		expectedLen int
		expectedDir string
	}{
		{
			name:        "nil config returns empty",
			config:      nil,
			expectedLen: 0, // Nil config returns no locations
			expectedDir: "",
		},
		{
			name: "config with profiler dir",
			config: &FrameworkConfig{
				Framework: "deepspeed",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "/custom/profiler",
				},
			},
			expectedLen: 1,
			expectedDir: "/custom/profiler",
		},
		{
			name: "config with empty profiler dir uses default",
			config: &FrameworkConfig{
				Framework: "deepspeed",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "",
				},
			},
			expectedLen: 1, // Empty profiler dir uses default location
			expectedDir: "/tmp/deepspeed_profiler",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := extractor.GetProfilerLocations(tt.config)
			assert.Len(t, locations, tt.expectedLen)

			if tt.expectedLen > 0 {
				assert.Equal(t, tt.expectedDir, locations[0].Directory)
				assert.Equal(t, "deepspeed_config", locations[0].Source)
			}
		})
	}
}

// ============================================================================
// GenericPyTorchExtractor Tests
// ============================================================================

func TestNewGenericPyTorchExtractor(t *testing.T) {
	extractor := NewGenericPyTorchExtractor()
	assert.NotNil(t, extractor)
}

func TestGenericPyTorchExtractor_FrameworkType(t *testing.T) {
	extractor := NewGenericPyTorchExtractor()
	assert.Equal(t, "pytorch", extractor.FrameworkType())
}

func TestGenericPyTorchExtractor_Priority(t *testing.T) {
	extractor := NewGenericPyTorchExtractor()
	assert.Equal(t, 100, extractor.Priority()) // Lowest priority (fallback)
}

func TestGenericPyTorchExtractor_CanHandle(t *testing.T) {
	extractor := NewGenericPyTorchExtractor()

	// GenericPyTorchExtractor always returns true (fallback)
	tests := []struct {
		name      string
		rawConfig map[string]interface{}
		expected  bool
	}{
		{
			name:      "empty config",
			rawConfig: map[string]interface{}{},
			expected:  true,
		},
		{
			name: "any config",
			rawConfig: map[string]interface{}{
				"some_key": "some_value",
			},
			expected: true,
		},
		{
			name:      "nil config",
			rawConfig: nil,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractor.CanHandle(tt.rawConfig)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenericPyTorchExtractor_ExtractPaths(t *testing.T) {
	extractor := NewGenericPyTorchExtractor()

	tests := []struct {
		name         string
		rawConfig    map[string]interface{}
		env          map[string]string
		checkPaths   bool
		expectedTB   string
	}{
		{
			name: "config with output_dir",
			rawConfig: map[string]interface{}{
				"output_dir": "/output",
			},
			env:        nil,
			checkPaths: true,
		},
		{
			name: "config with log_dir",
			rawConfig: map[string]interface{}{
				"log_dir": "/logs",
			},
			env:        nil,
			expectedTB: "/logs",
		},
		{
			name: "config with profiler_dir",
			rawConfig: map[string]interface{}{
				"profiler_dir": "/profiler",
			},
			env:        nil,
			checkPaths: true,
		},
		{
			name:       "empty config",
			rawConfig:  map[string]interface{}{},
			env:        nil,
			checkPaths: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			paths := extractor.ExtractPaths(tt.rawConfig, tt.env)
			require.NotNil(t, paths)

			if tt.expectedTB != "" {
				assert.Equal(t, tt.expectedTB, paths.TensorBoardDir)
			}
		})
	}
}

func TestGenericPyTorchExtractor_GetProfilerLocations(t *testing.T) {
	extractor := NewGenericPyTorchExtractor()

	tests := []struct {
		name        string
		config      *FrameworkConfig
		minExpected int // At least this many locations (includes defaults)
	}{
		{
			name:        "nil config returns defaults",
			config:      nil,
			minExpected: 4, // Default locations
		},
		{
			name: "config with profiler dir",
			config: &FrameworkConfig{
				Framework: "pytorch",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "/custom/profiler",
				},
			},
			minExpected: 5, // Config location + defaults
		},
		{
			name: "config with empty profiler dir",
			config: &FrameworkConfig{
				Framework: "pytorch",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "",
				},
			},
			minExpected: 4, // Only defaults
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := extractor.GetProfilerLocations(tt.config)
			assert.GreaterOrEqual(t, len(locations), tt.minExpected)

			// Check that all locations have required fields
			for _, loc := range locations {
				assert.NotEmpty(t, loc.Directory)
				assert.NotEmpty(t, loc.Patterns)
				assert.NotEmpty(t, loc.Source)
			}
		})
	}
}

// ============================================================================
// Extractor Priority Order Tests
// ============================================================================

func TestExtractorPriorityOrder(t *testing.T) {
	primus := NewPrimusExtractor()
	megatron := NewMegatronExtractor()
	deepspeed := NewDeepSpeedExtractor()
	generic := NewGenericPyTorchExtractor()

	// Verify priority order: primus < megatron < deepspeed < generic
	assert.Less(t, primus.Priority(), megatron.Priority())
	assert.Less(t, megatron.Priority(), deepspeed.Priority())
	assert.Less(t, deepspeed.Priority(), generic.Priority())
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestExtractorIntegration_PrimusFullWorkflow(t *testing.T) {
	extractor := NewPrimusExtractor()

	rawConfig := map[string]interface{}{
		"workspace":  "/output",
		"work_group": "amd",
		"user_name":  "researcher",
		"exp_name":   "llm-training",
		"modules": map[string]interface{}{
			"pre_trainer": map[string]interface{}{
				"overrides": map[string]interface{}{
					"profile":              true,
					"use_pytorch_profiler": true,
				},
			},
		},
	}

	// 1. Check can handle
	assert.True(t, extractor.CanHandle(rawConfig))

	// 2. Extract paths
	paths := extractor.ExtractPaths(rawConfig, nil)
	require.NotNil(t, paths)
	assert.Contains(t, paths.ProfilerDir, "tensorboard")
	assert.Equal(t, "llm-training", paths.CustomPaths["exp_name"])

	// 3. Build config and get locations
	config := &FrameworkConfig{
		Framework:      "primus",
		ExtractedPaths: paths,
	}
	locations := extractor.GetProfilerLocations(config)
	assert.NotEmpty(t, locations)
}

