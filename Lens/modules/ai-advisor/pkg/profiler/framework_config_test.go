package profiler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ResolveEnvVar Tests
// ============================================================================

func TestResolveEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		env      map[string]string
		osEnv    map[string]string
		expected string
	}{
		{
			name:     "no env vars",
			input:    "/output/tensorboard",
			env:      nil,
			expected: "/output/tensorboard",
		},
		{
			name:     "simple env var with default",
			input:    "${VAR:default}",
			env:      nil,
			expected: "default",
		},
		{
			name:     "env var from map",
			input:    "${VAR:default}",
			env:      map[string]string{"VAR": "custom"},
			expected: "custom",
		},
		{
			name:     "env var without default",
			input:    "${VAR}",
			env:      map[string]string{"VAR": "value"},
			expected: "value",
		},
		{
			name:     "env var without default, not found",
			input:    "${VAR}",
			env:      nil,
			expected: "",
		},
		{
			name:     "multiple env vars",
			input:    "${DIR}/${USER}/${FILE}",
			env:      map[string]string{"DIR": "output", "USER": "test", "FILE": "data"},
			expected: "output/test/data",
		},
		{
			name:     "nested path with env var",
			input:    "/data/${TEAM:default-team}/${USER:root}/output",
			env:      map[string]string{"TEAM": "my-team"},
			expected: "/data/my-team/root/output",
		},
		{
			name:     "empty env var uses default",
			input:    "${EMPTY_VAR:fallback}",
			env:      map[string]string{"EMPTY_VAR": ""},
			expected: "fallback",
		},
		{
			name:     "colon in default value",
			input:    "${VAR:http://localhost:8080}",
			env:      nil,
			expected: "http://localhost:8080",
		},
		{
			name:     "no closing brace - returns as is after first var",
			input:    "${VAR",
			env:      nil,
			expected: "${VAR",
		},
		{
			name:     "mixed text and env vars",
			input:    "prefix_${VAR:val}_suffix",
			env:      nil,
			expected: "prefix_val_suffix",
		},
		{
			name:     "env var at start",
			input:    "${PREFIX:}/data",
			env:      map[string]string{"PREFIX": "/mnt"},
			expected: "/mnt/data",
		},
		{
			name:     "env var at end",
			input:    "/data/${SUFFIX:logs}",
			env:      nil,
			expected: "/data/logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set OS env vars if specified
			for k, v := range tt.osEnv {
				os.Setenv(k, v)
				defer os.Unsetenv(k)
			}

			result := ResolveEnvVar(tt.input, tt.env)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveEnvVar_WithOSEnv(t *testing.T) {
	// Test that OS environment variables are used as fallback
	envKey := "TEST_RESOLVE_ENV_VAR_" + time.Now().Format("20060102150405")
	os.Setenv(envKey, "os-value")
	defer os.Unsetenv(envKey)

	input := "${" + envKey + ":default}"

	// Without map env, should use OS env
	result := ResolveEnvVar(input, nil)
	assert.Equal(t, "os-value", result)

	// Map env takes precedence
	result = ResolveEnvVar(input, map[string]string{envKey: "map-value"})
	assert.Equal(t, "map-value", result)
}

// ============================================================================
// JoinPaths Tests
// ============================================================================

func TestJoinPaths(t *testing.T) {
	tests := []struct {
		name     string
		base     string
		paths    []string
		expected string
	}{
		{
			name:     "simple join",
			base:     "/output",
			paths:    []string{"tensorboard"},
			expected: "/output/tensorboard",
		},
		{
			name:     "multiple paths",
			base:     "/output",
			paths:    []string{"team", "user", "exp"},
			expected: "/output/team/user/exp",
		},
		{
			name:     "absolute path in middle resets",
			base:     "/output",
			paths:    []string{"relative", "/absolute", "after"},
			expected: "/absolute/after",
		},
		{
			name:     "empty paths ignored",
			base:     "/output",
			paths:    []string{"", "data", ""},
			expected: "/output/data",
		},
		{
			name:     "no additional paths",
			base:     "/output",
			paths:    []string{},
			expected: "/output",
		},
		{
			name:     "relative base",
			base:     "./output",
			paths:    []string{"data"},
			expected: "output/data",
		},
		{
			name:     "all empty paths",
			base:     "/base",
			paths:    []string{"", "", ""},
			expected: "/base",
		},
		{
			name:     "with dots in path",
			base:     "/output",
			paths:    []string{"../sibling", "data"},
			expected: "/sibling/data",
		},
		{
			name:     "trailing slashes handled",
			base:     "/output/",
			paths:    []string{"data/"},
			expected: "/output/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := JoinPaths(tt.base, tt.paths...)
			// Normalize for comparison
			expected := filepath.Clean(tt.expected)
			assert.Equal(t, expected, result)
		})
	}
}

// ============================================================================
// BuildProfilerFilenamePattern Tests
// ============================================================================

func TestBuildProfilerFilenamePattern(t *testing.T) {
	tests := []struct {
		name        string
		expName     string
		ranks       []int
		useGzip     bool
		minPatterns int
		checkGzip   bool
	}{
		{
			name:        "basic pattern without gzip",
			expName:     "my-experiment",
			ranks:       nil,
			useGzip:     false,
			minPatterns: 1,
			checkGzip:   false,
		},
		{
			name:        "basic pattern with gzip",
			expName:     "my-experiment",
			ranks:       nil,
			useGzip:     true,
			minPatterns: 2,
			checkGzip:   true,
		},
		{
			name:        "specific ranks without gzip",
			expName:     "test-exp",
			ranks:       []int{0, 1},
			useGzip:     false,
			minPatterns: 2,
			checkGzip:   false,
		},
		{
			name:        "specific ranks with gzip",
			expName:     "test-exp",
			ranks:       []int{0, 1},
			useGzip:     true,
			minPatterns: 4, // 2 ranks * 2 (json + gz)
			checkGzip:   true,
		},
		{
			name:        "single rank",
			expName:     "single-rank-exp",
			ranks:       []int{0},
			useGzip:     false,
			minPatterns: 1,
			checkGzip:   false,
		},
		{
			name:        "empty exp name",
			expName:     "",
			ranks:       nil,
			useGzip:     false,
			minPatterns: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patterns := BuildProfilerFilenamePattern(tt.expName, tt.ranks, tt.useGzip)
			assert.GreaterOrEqual(t, len(patterns), tt.minPatterns)

			// Check for gzip patterns if expected
			if tt.checkGzip {
				hasGzip := false
				for _, p := range patterns {
					if filepath.Ext(p) == ".gz" || len(p) > 3 && p[len(p)-3:] == ".gz" {
						hasGzip = true
						break
					}
				}
				assert.True(t, hasGzip, "Expected gzip pattern")
			}

			// All patterns should end with .json or .json.gz
			for _, p := range patterns {
				assert.True(t,
					len(p) > 5 && (p[len(p)-5:] == ".json" || p[len(p)-8:] == ".json.gz"),
					"Pattern should end with .json or .json.gz: %s", p)
			}
		})
	}
}

// ============================================================================
// ExtractorRegistry Tests
// ============================================================================

func TestNewExtractorRegistry(t *testing.T) {
	registry := NewExtractorRegistry()
	assert.NotNil(t, registry)
	assert.NotNil(t, registry.extractors)
	assert.NotNil(t, registry.ordered)
}

func TestExtractorRegistry_Register(t *testing.T) {
	registry := NewExtractorRegistry()

	// Register extractors
	registry.Register(NewPrimusExtractor())
	registry.Register(NewMegatronExtractor())

	assert.Len(t, registry.extractors, 2)
	assert.Len(t, registry.ordered, 2)

	// Check priority order (primus should come first with priority 10)
	assert.Equal(t, "primus", registry.ordered[0].FrameworkType())
	assert.Equal(t, "megatron", registry.ordered[1].FrameworkType())
}

func TestExtractorRegistry_Register_PriorityOrder(t *testing.T) {
	registry := NewExtractorRegistry()

	// Register in reverse priority order
	registry.Register(NewGenericPyTorchExtractor()) // Priority 100
	registry.Register(NewDeepSpeedExtractor())       // Priority 30
	registry.Register(NewPrimusExtractor())          // Priority 10

	// Should be ordered by priority
	assert.Equal(t, "primus", registry.ordered[0].FrameworkType())
	assert.Equal(t, "deepspeed", registry.ordered[1].FrameworkType())
	assert.Equal(t, "pytorch", registry.ordered[2].FrameworkType())
}

func TestExtractorRegistry_GetExtractor(t *testing.T) {
	registry := NewExtractorRegistry()
	registry.Register(NewPrimusExtractor())
	registry.Register(NewMegatronExtractor())

	tests := []struct {
		name          string
		frameworkType string
		expectNil     bool
	}{
		{
			name:          "get primus",
			frameworkType: "primus",
			expectNil:     false,
		},
		{
			name:          "get megatron",
			frameworkType: "megatron",
			expectNil:     false,
		},
		{
			name:          "get uppercase (should work due to lowercase)",
			frameworkType: "PRIMUS",
			expectNil:     false,
		},
		{
			name:          "get nonexistent",
			frameworkType: "tensorflow",
			expectNil:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := registry.GetExtractor(tt.frameworkType)
			if tt.expectNil {
				assert.Nil(t, extractor)
			} else {
				assert.NotNil(t, extractor)
			}
		})
	}
}

func TestExtractorRegistry_GetAllExtractors(t *testing.T) {
	registry := NewExtractorRegistry()
	registry.Register(NewPrimusExtractor())
	registry.Register(NewMegatronExtractor())

	extractors := registry.GetAllExtractors()
	assert.Len(t, extractors, 2)

	// Should be a copy (modifying returned slice shouldn't affect registry)
	extractors[0] = nil
	originalExtractors := registry.GetAllExtractors()
	assert.NotNil(t, originalExtractors[0])
}

func TestExtractorRegistry_FindExtractor(t *testing.T) {
	registry := NewExtractorRegistry()
	registry.Register(NewPrimusExtractor())
	registry.Register(NewMegatronExtractor())
	registry.Register(NewGenericPyTorchExtractor())

	tests := []struct {
		name          string
		rawConfig     map[string]interface{}
		expectedType  string
		expectNil     bool
	}{
		{
			name: "find primus extractor",
			rawConfig: map[string]interface{}{
				"modules": map[string]interface{}{
					"pre_trainer": map[string]interface{}{},
				},
			},
			expectedType: "primus",
			expectNil:    false,
		},
		{
			name: "find megatron extractor",
			rawConfig: map[string]interface{}{
				"tensorboard-dir": "/output/tb",
			},
			expectedType: "megatron",
			expectNil:    false,
		},
		{
			name: "fallback to generic",
			rawConfig: map[string]interface{}{
				"learning_rate": 0.001,
			},
			expectedType: "pytorch", // Generic fallback
			expectNil:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			extractor := registry.FindExtractor(tt.rawConfig)
			if tt.expectNil {
				assert.Nil(t, extractor)
			} else {
				require.NotNil(t, extractor)
				assert.Equal(t, tt.expectedType, extractor.FrameworkType())
			}
		})
	}
}

// ============================================================================
// FrameworkConfigService Tests
// ============================================================================

func TestNewFrameworkConfigService(t *testing.T) {
	service := NewFrameworkConfigService()
	assert.NotNil(t, service)
	assert.NotNil(t, service.registry)
}

func TestFrameworkConfigService_BuildFrameworkConfig(t *testing.T) {
	service := NewFrameworkConfigService()
	ctx := context.Background()

	tests := []struct {
		name       string
		framework  string
		rawConfig  map[string]interface{}
		configPath string
		env        map[string]string
	}{
		{
			name:      "build primus config",
			framework: "primus",
			rawConfig: map[string]interface{}{
				"workspace": "/output",
				"exp_name":  "test",
			},
			configPath: "/config/primus.yaml",
			env:        nil,
		},
		{
			name:      "build megatron config",
			framework: "megatron",
			rawConfig: map[string]interface{}{
				"tensorboard-dir": "/output/tb",
				"save":            "/output/ckpt",
				"load":            "/output/ckpt",
			},
			configPath: "/config/megatron.yaml",
			env:        nil,
		},
		{
			name:       "build with unknown framework",
			framework:  "unknown",
			rawConfig:  map[string]interface{}{},
			configPath: "/config/unknown.yaml",
			env:        nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := service.BuildFrameworkConfig(ctx, tt.framework, tt.rawConfig, tt.configPath, tt.env)
			require.NotNil(t, config)
			assert.Equal(t, tt.framework, config.Framework)
			assert.Equal(t, "1.0", config.Version)
			assert.NotNil(t, config.Source)
			assert.Equal(t, tt.configPath, config.Source.Path)
			assert.NotNil(t, config.ExtractedPaths)
			assert.False(t, config.CollectedAt.IsZero())
		})
	}
}

func TestFrameworkConfigService_GetProfilerLocations(t *testing.T) {
	service := NewFrameworkConfigService()

	tests := []struct {
		name        string
		config      *FrameworkConfig
		minExpected int
	}{
		{
			name:        "nil config returns defaults",
			config:      nil,
			minExpected: 2, // Default locations
		},
		{
			name: "primus config with profiler dir",
			config: &FrameworkConfig{
				Framework: "primus",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "/output/tensorboard",
				},
			},
			minExpected: 1,
		},
		{
			name: "config with empty extracted paths",
			config: &FrameworkConfig{
				Framework:      "pytorch",
				ExtractedPaths: nil,
			},
			minExpected: 2, // Falls back to defaults
		},
		{
			name: "config with empty profiler dir",
			config: &FrameworkConfig{
				Framework: "pytorch",
				ExtractedPaths: &ExtractedPaths{
					ProfilerDir: "",
				},
			},
			minExpected: 2, // Falls back to defaults
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locations := service.GetProfilerLocations(tt.config)
			assert.GreaterOrEqual(t, len(locations), tt.minExpected)

			// All locations should have required fields
			for _, loc := range locations {
				assert.NotEmpty(t, loc.Directory)
				assert.NotEmpty(t, loc.Patterns)
				assert.NotEmpty(t, loc.Source)
			}
		})
	}
}

// ============================================================================
// FrameworkConfig Struct Tests
// ============================================================================

func TestFrameworkConfig_Fields(t *testing.T) {
	now := time.Now()
	config := &FrameworkConfig{
		Framework: "primus",
		Version:   "1.0",
		Source: &ConfigSource{
			Type: "config_file",
			Path: "/config/primus.yaml",
		},
		RawConfig: map[string]interface{}{
			"workspace": "/output",
		},
		ExtractedPaths: &ExtractedPaths{
			ProfilerDir:    "/output/profiler",
			TensorBoardDir: "/output/tensorboard",
			CheckpointDir:  "/output/checkpoints",
			LogDir:         "/output/logs",
			WorkspaceDir:   "/output",
			CustomPaths: map[string]string{
				"exp_name": "test",
			},
		},
		CollectedAt: now,
	}

	assert.Equal(t, "primus", config.Framework)
	assert.Equal(t, "1.0", config.Version)
	assert.Equal(t, "config_file", config.Source.Type)
	assert.Equal(t, "/config/primus.yaml", config.Source.Path)
	assert.NotNil(t, config.RawConfig)
	assert.Equal(t, "/output/profiler", config.ExtractedPaths.ProfilerDir)
	assert.Equal(t, "test", config.ExtractedPaths.CustomPaths["exp_name"])
	assert.Equal(t, now, config.CollectedAt)
}

// ============================================================================
// ProfilerLocation Struct Tests
// ============================================================================

func TestProfilerLocation_Fields(t *testing.T) {
	loc := ProfilerLocation{
		Directory:    "/output/profiler",
		Patterns:     []string{"*.pt.trace.json", "*.pt.trace.json.gz"},
		Recursive:    true,
		MaxDepth:     3,
		Priority:     1,
		Source:       "config",
		ProfileRanks: []int{0, 1, 2},
	}

	assert.Equal(t, "/output/profiler", loc.Directory)
	assert.Len(t, loc.Patterns, 2)
	assert.True(t, loc.Recursive)
	assert.Equal(t, 3, loc.MaxDepth)
	assert.Equal(t, 1, loc.Priority)
	assert.Equal(t, "config", loc.Source)
	assert.Equal(t, []int{0, 1, 2}, loc.ProfileRanks)
}

// ============================================================================
// Global Registry Tests
// ============================================================================

func TestGetExtractorRegistry(t *testing.T) {
	// Should return the same instance
	registry1 := GetExtractorRegistry()
	registry2 := GetExtractorRegistry()
	assert.Same(t, registry1, registry2)
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkResolveEnvVar(b *testing.B) {
	input := "/data/${TEAM:default-team}/${USER:root}/output"
	env := map[string]string{"TEAM": "my-team", "USER": "myuser"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ResolveEnvVar(input, env)
	}
}

func BenchmarkJoinPaths(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = JoinPaths("/output", "team", "user", "experiment", "tensorboard")
	}
}

func BenchmarkBuildProfilerFilenamePattern(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = BuildProfilerFilenamePattern("my-experiment", []int{0, 1, 2, 3}, true)
	}
}

func BenchmarkExtractorRegistry_FindExtractor(b *testing.B) {
	registry := NewExtractorRegistry()
	registry.Register(NewPrimusExtractor())
	registry.Register(NewMegatronExtractor())
	registry.Register(NewDeepSpeedExtractor())
	registry.Register(NewGenericPyTorchExtractor())

	config := map[string]interface{}{
		"modules": map[string]interface{}{
			"pre_trainer": map[string]interface{}{},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = registry.FindExtractor(config)
	}
}

