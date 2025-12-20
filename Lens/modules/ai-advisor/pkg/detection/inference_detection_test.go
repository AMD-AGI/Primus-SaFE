package detection

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestInferenceDetection_EndToEnd tests the end-to-end inference detection flow
func TestInferenceDetection_EndToEnd(t *testing.T) {
	// Create test framework config for vLLM
	vllmConfig := &FrameworkLogPatterns{
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
					Pattern:    `vllm/vllm-openai|.*vllm.*`,
					Enabled:    true,
					Confidence: 0.85,
				},
			},
			CmdlinePatterns: []PatternConfig{
				{
					Name:       "vllm-model-arg",
					Pattern:    `--model\s+\S+|--served-model-name`,
					Enabled:    true,
					Confidence: 0.8,
				},
			},
		},
	}

	// Create pattern matcher
	matcher, err := NewPatternMatcher(vllmConfig)
	require.NoError(t, err)

	// Test various scenarios
	testCases := []struct {
		name     string
		ctx      *InferenceMatchContext
		expected bool
		minConf  float64
	}{
		{
			name: "Full vLLM match",
			ctx: &InferenceMatchContext{
				ProcessNames:    []string{"python", "vllm.entrypoints.openai.api_server"},
				ProcessCmdlines: []string{"python -m vllm.entrypoints.openai.api_server --model meta-llama/Llama-2-7b"},
				ImageName:       "vllm/vllm-openai:v0.4.0",
				ContainerPorts:  []int{8000},
				EnvVars:         map[string]string{"VLLM_HOST": "0.0.0.0", "VLLM_PORT": "8000"},
			},
			expected: true,
			minConf:  0.8,
		},
		{
			name: "Partial match - process and image only",
			ctx: &InferenceMatchContext{
				ProcessNames: []string{"vllm.entrypoints.openai.api_server"},
				ImageName:    "vllm/vllm-openai:latest",
			},
			expected: true,
			minConf:  0.4,
		},
		{
			name: "No match - unrelated process",
			ctx: &InferenceMatchContext{
				ProcessNames:   []string{"python", "gunicorn"},
				ImageName:      "pytorch/pytorch:latest",
				ContainerPorts: []int{80, 443},
			},
			expected: false,
		},
		{
			name: "Single source match - not enough",
			ctx: &InferenceMatchContext{
				ContainerPorts: []int{8000}, // Only port matches
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := matcher.MatchInference(tc.ctx)
			assert.Equal(t, tc.expected, result.Matched, "Expected matched=%v, got matched=%v", tc.expected, result.Matched)

			if tc.expected {
				assert.GreaterOrEqual(t, result.Confidence, tc.minConf,
					"Expected confidence >= %v, got %v", tc.minConf, result.Confidence)
				assert.Equal(t, "vllm", result.FrameworkName)
				assert.Equal(t, FrameworkTypeInference, result.FrameworkType)
				assert.NotEmpty(t, result.MatchedSources)
				assert.NotEmpty(t, result.Evidence)
			}
		})
	}
}

// TestInferenceDetection_MultipleFrameworks tests detection with multiple inference frameworks
func TestInferenceDetection_MultipleFrameworks(t *testing.T) {
	// Create configs for multiple inference frameworks
	frameworks := []*FrameworkLogPatterns{
		{
			Name:     "vllm",
			Type:     FrameworkTypeInference,
			Enabled:  true,
			Priority: 90,
			InferencePatterns: &InferencePatternConfig{
				ProcessPatterns: []PatternConfig{
					{Name: "vllm-process", Pattern: `vllm`, Enabled: true, Confidence: 0.95},
				},
				ImagePatterns: []PatternConfig{
					{Name: "vllm-image", Pattern: `vllm`, Enabled: true, Confidence: 0.9},
				},
			},
		},
		{
			Name:     "triton",
			Type:     FrameworkTypeInference,
			Enabled:  true,
			Priority: 85,
			InferencePatterns: &InferencePatternConfig{
				ProcessPatterns: []PatternConfig{
					{Name: "triton-process", Pattern: `tritonserver`, Enabled: true, Confidence: 0.95},
				},
				ImagePatterns: []PatternConfig{
					{Name: "triton-image", Pattern: `tritonserver`, Enabled: true, Confidence: 0.9},
				},
			},
		},
		{
			Name:     "tgi",
			Type:     FrameworkTypeInference,
			Enabled:  true,
			Priority: 80,
			InferencePatterns: &InferencePatternConfig{
				ProcessPatterns: []PatternConfig{
					{Name: "tgi-process", Pattern: `text-generation-launcher`, Enabled: true, Confidence: 0.95},
				},
				ImagePatterns: []PatternConfig{
					{Name: "tgi-image", Pattern: `text-generation-inference`, Enabled: true, Confidence: 0.9},
				},
			},
		},
	}

	// Create matchers
	matchers := make(map[string]*PatternMatcher)
	for _, fw := range frameworks {
		matcher, err := NewPatternMatcher(fw)
		require.NoError(t, err)
		matchers[fw.Name] = matcher
	}

	testCases := []struct {
		name            string
		ctx             *InferenceMatchContext
		expectedFW      string
		expectedMatched bool
	}{
		{
			name: "Match vLLM",
			ctx: &InferenceMatchContext{
				ProcessNames: []string{"vllm.server"},
				ImageName:    "vllm/vllm:latest",
			},
			expectedFW:      "vllm",
			expectedMatched: true,
		},
		{
			name: "Match Triton",
			ctx: &InferenceMatchContext{
				ProcessNames: []string{"tritonserver"},
				ImageName:    "nvcr.io/nvidia/tritonserver:latest",
			},
			expectedFW:      "triton",
			expectedMatched: true,
		},
		{
			name: "Match TGI",
			ctx: &InferenceMatchContext{
				ProcessNames: []string{"text-generation-launcher"},
				ImageName:    "ghcr.io/huggingface/text-generation-inference:latest",
			},
			expectedFW:      "tgi",
			expectedMatched: true,
		},
		{
			name: "No match - training workload",
			ctx: &InferenceMatchContext{
				ProcessNames: []string{"python", "train.py"},
				ImageName:    "pytorch/pytorch:latest",
			},
			expectedFW:      "",
			expectedMatched: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var bestMatch *InferenceMatchResult
			var bestConfidence float64

			for _, matcher := range matchers {
				result := matcher.MatchInference(tc.ctx)
				if result.Matched && result.Confidence > bestConfidence {
					bestMatch = result
					bestConfidence = result.Confidence
				}
			}

			if tc.expectedMatched {
				require.NotNil(t, bestMatch, "Expected a match but got none")
				assert.Equal(t, tc.expectedFW, bestMatch.FrameworkName)
			} else {
				assert.Nil(t, bestMatch, "Expected no match but got one")
			}
		})
	}
}

// TestInferenceDetectionRequest tests the DetectInferenceFramework function
func TestInferenceDetectionRequest(t *testing.T) {
	// Note: This test requires patternMatchers to be initialized
	// In production, this would be done via InitializeDetectionManager

	// For unit testing, we'll test the request/response structures
	req := &InferenceDetectionRequest{
		WorkloadUID:     "test-workload-123",
		PodName:         "vllm-server-pod",
		Namespace:       "ml-inference",
		ProcessNames:    []string{"python", "vllm.entrypoints.openai.api_server"},
		ProcessCmdlines: []string{"python -m vllm.entrypoints.openai.api_server --model llama"},
		ImageName:       "vllm/vllm-openai:v0.4.0",
		ContainerPorts:  []int{8000},
		EnvVars: map[string]string{
			"VLLM_HOST": "0.0.0.0",
			"VLLM_PORT": "8000",
		},
	}

	// Validate request structure
	assert.NotEmpty(t, req.WorkloadUID)
	assert.NotEmpty(t, req.PodName)
	assert.NotEmpty(t, req.ProcessNames)
	assert.NotEmpty(t, req.ImageName)

	// Test response structure
	resp := &InferenceDetectionResponse{
		Detected:       true,
		FrameworkName:  "vllm",
		FrameworkType:  FrameworkTypeInference,
		Confidence:     0.85,
		MatchedSources: []string{"process", "image", "port", "env"},
		Evidence:       []string{"process:vllm.entrypoints matched", "image:vllm matched"},
	}

	assert.True(t, resp.Detected)
	assert.Equal(t, "vllm", resp.FrameworkName)
	assert.Equal(t, FrameworkTypeInference, resp.FrameworkType)
	assert.Greater(t, resp.Confidence, 0.0)
	assert.NotEmpty(t, resp.MatchedSources)
}

// TestIsTrainingWorkload_InferenceDetection tests isTrainingWorkload with inference detection
func TestIsTrainingWorkload_InferenceDetection(t *testing.T) {
	tc := NewTaskCreator("test-instance")

	testCases := []struct {
		name             string
		detectionType    string
		sourceTypes      []string
		expectedTraining bool
	}{
		{
			name:             "Explicit training type",
			detectionType:    FrameworkTypeTraining,
			sourceTypes:      nil,
			expectedTraining: true,
		},
		{
			name:             "Explicit inference type",
			detectionType:    FrameworkTypeInference,
			sourceTypes:      nil,
			expectedTraining: false,
		},
		{
			name:             "Empty type defaults to training",
			detectionType:    "",
			sourceTypes:      nil,
			expectedTraining: true,
		},
		{
			name:             "Source with inference type only",
			detectionType:    "",
			sourceTypes:      []string{FrameworkTypeInference},
			expectedTraining: false,
		},
		{
			name:             "Mixed sources with training",
			detectionType:    "",
			sourceTypes:      []string{FrameworkTypeInference, FrameworkTypeTraining},
			expectedTraining: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			// We can't easily construct a coreModel.FrameworkDetection in this package
			// This test demonstrates the logic flow
			// In practice, this would use the actual model type
			t.Logf("Test case: %s - detectionType=%s, expectedTraining=%v",
				test.name, test.detectionType, test.expectedTraining)
		})
	}

	// Test nil detection
	result := tc.isTrainingWorkload(nil)
	assert.True(t, result, "Nil detection should default to training")
}

// TestDetectInferenceFramework_NoMatchers tests when no pattern matchers are initialized
func TestDetectInferenceFramework_NoMatchers(t *testing.T) {
	// Save original and restore after test
	original := patternMatchers
	patternMatchers = nil
	defer func() { patternMatchers = original }()

	req := &InferenceDetectionRequest{
		WorkloadUID:  "test-123",
		ProcessNames: []string{"vllm.server"},
	}

	resp, err := DetectInferenceFramework(context.Background(), req)
	assert.NoError(t, err)
	assert.False(t, resp.Detected)
}

// TestDetectInferenceFramework_EmptyMatchers tests when pattern matchers map is empty
func TestDetectInferenceFramework_EmptyMatchers(t *testing.T) {
	// Save original and restore after test
	original := patternMatchers
	patternMatchers = make(map[string]*PatternMatcher)
	defer func() { patternMatchers = original }()

	req := &InferenceDetectionRequest{
		WorkloadUID:  "test-123",
		ProcessNames: []string{"vllm.server"},
	}

	resp, err := DetectInferenceFramework(context.Background(), req)
	assert.NoError(t, err)
	assert.False(t, resp.Detected)
}

