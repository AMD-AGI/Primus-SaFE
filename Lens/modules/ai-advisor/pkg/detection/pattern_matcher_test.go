package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewPatternMatcher_TrainingFramework tests creating a pattern matcher for training framework
func TestNewPatternMatcher_TrainingFramework(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name:        "pytorch",
		DisplayName: "PyTorch",
		Type:        FrameworkTypeTraining,
		Enabled:     true,
		Priority:    100,
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "pytorch-import",
				Pattern:    `import\s+torch`,
				Enabled:    true,
				Confidence: 0.9,
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)
	assert.NotNil(t, matcher)
	assert.Equal(t, "pytorch", matcher.GetFrameworkName())
	assert.True(t, matcher.IsTrainingFramework())
	assert.False(t, matcher.IsInferenceFramework())
}

// TestNewPatternMatcher_InferenceFramework tests creating a pattern matcher for inference framework
func TestNewPatternMatcher_InferenceFramework(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name:        "vllm",
		DisplayName: "vLLM",
		Type:        FrameworkTypeInference,
		Enabled:     true,
		Priority:    90,
		IdentifyPatterns: []PatternConfig{
			{
				Name:       "vllm-import",
				Pattern:    `from\s+vllm\s+import`,
				Enabled:    true,
				Confidence: 0.9,
			},
		},
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "vllm-process",
					Pattern:    `vllm\.entrypoints|python.*-m\s+vllm`,
					Enabled:    true,
					Confidence: 0.95,
				},
			},
			Ports: []int{8000},
			EnvPatterns: []PatternConfig{
				{
					Name:       "vllm-env",
					Pattern:    `VLLM_.*`,
					Enabled:    true,
					Confidence: 0.8,
				},
			},
			ImagePatterns: []PatternConfig{
				{
					Name:       "vllm-image",
					Pattern:    `vllm/vllm-openai|.*vllm.*`,
					Enabled:    true,
					Confidence: 0.85,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)
	assert.NotNil(t, matcher)
	assert.Equal(t, "vllm", matcher.GetFrameworkName())
	assert.False(t, matcher.IsTrainingFramework())
	assert.True(t, matcher.IsInferenceFramework())
	assert.Equal(t, FrameworkTypeInference, matcher.GetFrameworkType())
}

// TestMatchInference_VLLMDetection tests inference detection for vLLM
func TestMatchInference_VLLMDetection(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name:        "vllm",
		DisplayName: "vLLM",
		Type:        FrameworkTypeInference,
		Enabled:     true,
		Priority:    90,
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "vllm-process",
					Pattern:    `vllm\.entrypoints|python.*-m\s+vllm`,
					Enabled:    true,
					Confidence: 0.95,
				},
			},
			Ports: []int{8000},
			EnvPatterns: []PatternConfig{
				{
					Name:       "vllm-env",
					Pattern:    `VLLM_.*`,
					Enabled:    true,
					Confidence: 0.8,
				},
			},
			ImagePatterns: []PatternConfig{
				{
					Name:       "vllm-image",
					Pattern:    `vllm/vllm-openai`,
					Enabled:    true,
					Confidence: 0.85,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	// Test with matching context
	ctx := &InferenceMatchContext{
		ProcessNames:   []string{"python", "vllm.entrypoints.openai.api_server"},
		ImageName:      "vllm/vllm-openai:v0.4.0",
		ContainerPorts: []int{8000},
		EnvVars: map[string]string{
			"VLLM_HOST": "0.0.0.0",
		},
	}

	result := matcher.MatchInference(ctx)
	assert.True(t, result.Matched)
	assert.Equal(t, "vllm", result.FrameworkName)
	assert.Equal(t, FrameworkTypeInference, result.FrameworkType)
	assert.Greater(t, result.Confidence, 0.5)
	assert.GreaterOrEqual(t, len(result.MatchedSources), InferenceMinMatches)
}

// TestMatchInference_TritonDetection tests inference detection for Triton
func TestMatchInference_TritonDetection(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name:        "triton",
		DisplayName: "Triton Inference Server",
		Type:        FrameworkTypeInference,
		Enabled:     true,
		Priority:    85,
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "triton-process",
					Pattern:    `tritonserver`,
					Enabled:    true,
					Confidence: 0.95,
				},
			},
			Ports: []int{8000, 8001, 8002},
			ImagePatterns: []PatternConfig{
				{
					Name:       "triton-image",
					Pattern:    `nvcr\.io/nvidia/tritonserver`,
					Enabled:    true,
					Confidence: 0.9,
				},
			},
			EnvPatterns: []PatternConfig{
				{
					Name:       "triton-env",
					Pattern:    `TRITON_.*`,
					Enabled:    true,
					Confidence: 0.8,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	// Test with Triton context
	ctx := &InferenceMatchContext{
		ProcessNames:   []string{"tritonserver"},
		ImageName:      "nvcr.io/nvidia/tritonserver:23.10-py3",
		ContainerPorts: []int{8000, 8001, 8002},
	}

	result := matcher.MatchInference(ctx)
	assert.True(t, result.Matched)
	assert.Equal(t, "triton", result.FrameworkName)
	assert.Contains(t, result.MatchedSources, "process")
	assert.Contains(t, result.MatchedSources, "image")
	assert.Contains(t, result.MatchedSources, "port")
}

// TestMatchInference_NotEnoughMatches tests that detection fails with insufficient matches
func TestMatchInference_NotEnoughMatches(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name:        "vllm",
		DisplayName: "vLLM",
		Type:        FrameworkTypeInference,
		Enabled:     true,
		Priority:    90,
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "vllm-process",
					Pattern:    `vllm\.entrypoints`,
					Enabled:    true,
					Confidence: 0.95,
				},
			},
			Ports: []int{8000},
			ImagePatterns: []PatternConfig{
				{
					Name:       "vllm-image",
					Pattern:    `vllm/vllm-openai`,
					Enabled:    true,
					Confidence: 0.85,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	// Test with only one matching source (port only)
	ctx := &InferenceMatchContext{
		ProcessNames:   []string{"python"}, // Doesn't match vllm pattern
		ImageName:      "pytorch/pytorch:latest",
		ContainerPorts: []int{8000}, // Matches
	}

	result := matcher.MatchInference(ctx)
	assert.False(t, result.Matched, "Should not match with only one source")
}

// TestMatchInference_TrainingFrameworkReturnsNoMatch tests that training framework returns no match
func TestMatchInference_TrainingFrameworkReturnsNoMatch(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name:        "pytorch",
		DisplayName: "PyTorch",
		Type:        FrameworkTypeTraining, // Training, not inference
		Enabled:     true,
		Priority:    100,
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	ctx := &InferenceMatchContext{
		ProcessNames:   []string{"python"},
		ContainerPorts: []int{8000},
	}

	result := matcher.MatchInference(ctx)
	assert.False(t, result.Matched, "Training framework should not match inference")
}

// TestMatchInference_NilContext tests that nil context returns no match
func TestMatchInference_NilContext(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name:        "vllm",
		DisplayName: "vLLM",
		Type:        FrameworkTypeInference,
		Enabled:     true,
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "vllm-process",
					Pattern:    `vllm`,
					Enabled:    true,
					Confidence: 0.95,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	result := matcher.MatchInference(nil)
	assert.False(t, result.Matched)
}

// TestMatchProcessPatterns tests process pattern matching
func TestMatchProcessPatterns(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "vllm",
		Type: FrameworkTypeInference,
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "vllm-entrypoints",
					Pattern:    `vllm\.entrypoints`,
					Enabled:    true,
					Confidence: 0.95,
				},
				{
					Name:       "vllm-python",
					Pattern:    `python.*-m\s+vllm`,
					Enabled:    true,
					Confidence: 0.9,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	tests := []struct {
		name         string
		processNames []string
		cmdlines     []string
		wantMatch    bool
	}{
		{
			name:         "match process name",
			processNames: []string{"vllm.entrypoints.openai.api_server"},
			wantMatch:    true,
		},
		{
			name:      "match cmdline",
			cmdlines:  []string{"python -m vllm.entrypoints.openai.api_server"},
			wantMatch: true,
		},
		{
			name:         "no match",
			processNames: []string{"python", "gunicorn"},
			cmdlines:     []string{"python app.py"},
			wantMatch:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.matchProcessPatterns(tt.processNames, tt.cmdlines)
			assert.Equal(t, tt.wantMatch, result.Matched)
		})
	}
}

// TestMatchImagePattern tests image pattern matching
func TestMatchImagePattern(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "vllm",
		Type: FrameworkTypeInference,
		InferencePatterns: &InferencePatternConfig{
			ImagePatterns: []PatternConfig{
				{
					Name:       "vllm-official",
					Pattern:    `vllm/vllm-openai`,
					Enabled:    true,
					Confidence: 0.9,
				},
				{
					Name:       "vllm-generic",
					Pattern:    `.*vllm.*`,
					Enabled:    true,
					Confidence: 0.7,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	tests := []struct {
		name      string
		imageName string
		wantMatch bool
	}{
		{
			name:      "official image",
			imageName: "vllm/vllm-openai:v0.4.0",
			wantMatch: true,
		},
		{
			name:      "custom vllm image",
			imageName: "myregistry/my-vllm-server:latest",
			wantMatch: true,
		},
		{
			name:      "no match",
			imageName: "pytorch/pytorch:latest",
			wantMatch: false,
		},
		{
			name:      "empty image",
			imageName: "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.matchImagePattern(tt.imageName)
			assert.Equal(t, tt.wantMatch, result.Matched)
		})
	}
}

// TestMatchEnvPatterns tests environment variable pattern matching
func TestMatchEnvPatterns(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "vllm",
		Type: FrameworkTypeInference,
		InferencePatterns: &InferencePatternConfig{
			EnvPatterns: []PatternConfig{
				{
					Name:       "vllm-env",
					Pattern:    `VLLM_.*`,
					Enabled:    true,
					Confidence: 0.8,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	tests := []struct {
		name      string
		envVars   map[string]string
		wantMatch bool
	}{
		{
			name: "match vllm env",
			envVars: map[string]string{
				"VLLM_HOST": "0.0.0.0",
				"VLLM_PORT": "8000",
				"OTHER_VAR": "value",
			},
			wantMatch: true,
		},
		{
			name: "no vllm env",
			envVars: map[string]string{
				"HOME": "/root",
				"PATH": "/usr/bin",
			},
			wantMatch: false,
		},
		{
			name:      "empty env",
			envVars:   map[string]string{},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.matchEnvPatterns(tt.envVars)
			assert.Equal(t, tt.wantMatch, result.Matched)
		})
	}
}

// TestMatchPorts tests port matching
func TestMatchPorts(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "vllm",
		Type: FrameworkTypeInference,
		InferencePatterns: &InferencePatternConfig{
			Ports: []int{8000},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	tests := []struct {
		name           string
		containerPorts []int
		wantMatch      bool
	}{
		{
			name:           "match exact port",
			containerPorts: []int{8000},
			wantMatch:      true,
		},
		{
			name:           "match among multiple ports",
			containerPorts: []int{80, 443, 8000, 9090},
			wantMatch:      true,
		},
		{
			name:           "no match",
			containerPorts: []int{80, 443, 3000},
			wantMatch:      false,
		},
		{
			name:           "empty ports",
			containerPorts: []int{},
			wantMatch:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.matchPorts(tt.containerPorts)
			assert.Equal(t, tt.wantMatch, result.Matched)
		})
	}
}

// TestMatchCmdlinePatterns tests command line pattern matching
func TestMatchCmdlinePatterns(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "vllm",
		Type: FrameworkTypeInference,
		InferencePatterns: &InferencePatternConfig{
			CmdlinePatterns: []PatternConfig{
				{
					Name:       "vllm-serve",
					Pattern:    `--served-model-name`,
					Enabled:    true,
					Confidence: 0.85,
				},
				{
					Name:       "vllm-tensor-parallel",
					Pattern:    `--tensor-parallel-size`,
					Enabled:    true,
					Confidence: 0.8,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	tests := []struct {
		name      string
		cmdlines  []string
		wantMatch bool
	}{
		{
			name:      "match served model",
			cmdlines:  []string{"python -m vllm.entrypoints.openai.api_server --served-model-name llama"},
			wantMatch: true,
		},
		{
			name:      "match tensor parallel",
			cmdlines:  []string{"python app.py --tensor-parallel-size 4"},
			wantMatch: true,
		},
		{
			name:      "no match",
			cmdlines:  []string{"python train.py --epochs 100"},
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matcher.matchCmdlinePatterns(tt.cmdlines)
			assert.Equal(t, tt.wantMatch, result.Matched)
		})
	}
}

// TestIntToStr tests the integer to string helper
func TestIntToStr(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{8000, "8000"},
		{-1, "-1"},
		{12345, "12345"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := intToStr(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestConfidenceCalculation tests that confidence is calculated correctly
func TestConfidenceCalculation(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "vllm",
		Type: FrameworkTypeInference,
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "vllm-process",
					Pattern:    `vllm`,
					Enabled:    true,
					Confidence: 1.0, // Max confidence
				},
			},
			ImagePatterns: []PatternConfig{
				{
					Name:       "vllm-image",
					Pattern:    `vllm`,
					Enabled:    true,
					Confidence: 1.0,
				},
			},
			EnvPatterns: []PatternConfig{
				{
					Name:       "vllm-env",
					Pattern:    `VLLM`,
					Enabled:    true,
					Confidence: 1.0,
				},
			},
			Ports: []int{8000},
			CmdlinePatterns: []PatternConfig{
				{
					Name:       "vllm-cmd",
					Pattern:    `vllm`,
					Enabled:    true,
					Confidence: 1.0,
				},
			},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	// All sources match with max confidence
	ctx := &InferenceMatchContext{
		ProcessNames:    []string{"vllm.server"},
		ImageName:       "vllm/vllm:latest",
		EnvVars:         map[string]string{"VLLM_HOST": "0.0.0.0"},
		ContainerPorts:  []int{8000},
		ProcessCmdlines: []string{"python -m vllm.serve"},
	}

	result := matcher.MatchInference(ctx)
	assert.True(t, result.Matched)

	// Expected: 0.35 + 0.25 + 0.20 + 0.10 + 0.10 = 1.0
	assert.InDelta(t, 1.0, result.Confidence, 0.01)
	assert.Len(t, result.MatchedSources, 5)
}

// TestDisabledPatterns tests that disabled patterns are not matched
func TestDisabledPatterns(t *testing.T) {
	framework := &FrameworkLogPatterns{
		Name: "vllm",
		Type: FrameworkTypeInference,
		InferencePatterns: &InferencePatternConfig{
			ProcessPatterns: []PatternConfig{
				{
					Name:       "vllm-process",
					Pattern:    `vllm`,
					Enabled:    false, // Disabled
					Confidence: 0.95,
				},
			},
			ImagePatterns: []PatternConfig{
				{
					Name:       "vllm-image",
					Pattern:    `vllm`,
					Enabled:    true,
					Confidence: 0.85,
				},
			},
			Ports: []int{8000},
		},
	}

	matcher, err := NewPatternMatcher(framework)
	require.NoError(t, err)

	// Process pattern is disabled, so only image and port should match
	ctx := &InferenceMatchContext{
		ProcessNames:   []string{"vllm.server"},
		ImageName:      "vllm/vllm:latest",
		ContainerPorts: []int{8000},
	}

	result := matcher.MatchInference(ctx)
	assert.True(t, result.Matched)
	assert.NotContains(t, result.MatchedSources, "process")
	assert.Contains(t, result.MatchedSources, "image")
	assert.Contains(t, result.MatchedSources, "port")
}
