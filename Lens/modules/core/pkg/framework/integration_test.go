package framework

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// TestIntegration_CompleteReuseFlow tests the complete reuse workflow
func TestIntegration_CompleteReuseFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Setup
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()

	// Create reuse engine
	engine := NewReuseEngine(mockDB, config)
	ctx := context.Background()

	// Step 1: Create historical workload with verified detection
	historicalWorkload := createHistoricalWorkload(
		"historical-uid-1",
		"registry.example.com/pytorch:v1.9.0",
		[]string{"python", "train.py"},
		map[string]string{"FRAMEWORK": "PyTorch", "WORLD_SIZE": "8"},
		0.95, // High confidence
	)

	// Step 2: Create new similar workload
	newWorkload := &Workload{
		UID:       "new-uid",
		Image:     "registry.example.com/pytorch:v1.9.1", // Similar version
		Command:   []string{"python", "train.py"},
		Args:      []string{"--epochs", "100"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch", "WORLD_SIZE": "8"},
		Labels:    map[string]string{"app": "training"},
		Namespace: "default",
	}

	// Mock database responses
	mockDB.On("FindCandidateWorkloads",
		ctx,
		"registry.example.com/pytorch",
		mock.Anything,
		config.MinConfidence,
		config.MaxCandidates,
	).Return([]*model.AiWorkloadMetadata{historicalWorkload}, nil)

	mockDB.On("GetAiWorkloadMetadata",
		ctx,
		"historical-uid-1",
	).Return(historicalWorkload, nil)

	// Step 3: Execute reuse
	detection, err := engine.TryReuse(ctx, newWorkload)

	// Step 4: Verify results
	require.NoError(t, err)
	require.NotNil(t, detection)

	// Verify framework detection
	assert.Equal(t, []string{"pytorch"}, detection.Frameworks)
	assert.Equal(t, "training", detection.Type)
	assert.Equal(t, coreModel.DetectionStatusReused, detection.Status)

	// Verify confidence decay
	expectedConfidence := 0.95 * config.ConfidenceDecayRate
	assert.InDelta(t, expectedConfidence, detection.Confidence, 0.001)

	// Verify reuse info
	require.NotNil(t, detection.ReuseInfo)
	assert.Equal(t, "historical-uid-1", detection.ReuseInfo.ReusedFrom)
	assert.Equal(t, 0.95, detection.ReuseInfo.OriginalConfidence)
	assert.Greater(t, detection.ReuseInfo.SimilarityScore, config.MinSimilarityScore)

	// Verify sources
	require.Len(t, detection.Sources, 1)
	assert.Equal(t, "reuse", detection.Sources[0].Source)
	assert.Equal(t, []string{"pytorch"}, detection.Sources[0].Frameworks)

	mockDB.AssertExpectations(t)
}

// TestIntegration_MultipleCandiatesSelection tests selecting best from multiple candidates
func TestIntegration_MultipleCandidatesSelection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	engine := NewReuseEngine(mockDB, config)
	ctx := context.Background()

	// Create multiple candidates with different similarity levels
	candidate1 := createHistoricalWorkload(
		"uid-1",
		"registry.example.com/pytorch:v1.9.0", // Exact match
		[]string{"python", "train.py"},
		map[string]string{"FRAMEWORK": "PyTorch", "WORLD_SIZE": "8"},
		0.90,
	)

	candidate2 := createHistoricalWorkload(
		"uid-2",
		"registry.example.com/pytorch:v1.8.0", // Different version
		[]string{"python", "train.py"},
		map[string]string{"FRAMEWORK": "PyTorch", "WORLD_SIZE": "4"},
		0.95, // Higher confidence but lower similarity
	)

	candidate3 := createHistoricalWorkload(
		"uid-3",
		"registry.example.com/pytorch:v1.9.0", // Exact match
		[]string{"python", "test.py"},         // Different script
		map[string]string{"FRAMEWORK": "PyTorch"},
		0.85,
	)

	newWorkload := &Workload{
		UID:       "new-uid",
		Image:     "registry.example.com/pytorch:v1.9.0",
		Command:   []string{"python", "train.py"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch", "WORLD_SIZE": "8"},
		Namespace: "default",
	}

	mockDB.On("FindCandidateWorkloads",
		ctx,
		"registry.example.com/pytorch",
		mock.Anything,
		config.MinConfidence,
		config.MaxCandidates,
	).Return([]*model.AiWorkloadMetadata{candidate1, candidate2, candidate3}, nil)

	mockDB.On("GetAiWorkloadMetadata",
		ctx,
		"uid-1", // Should select uid-1 (highest similarity)
	).Return(candidate1, nil)

	detection, err := engine.TryReuse(ctx, newWorkload)

	require.NoError(t, err)
	require.NotNil(t, detection)

	// Should reuse from candidate1 (highest similarity match)
	assert.Equal(t, "uid-1", detection.ReuseInfo.ReusedFrom)
	assert.Greater(t, detection.ReuseInfo.SimilarityScore, 0.95) // Very high similarity

	mockDB.AssertExpectations(t)
}

// TestIntegration_SignatureAndSimilarityPipeline tests the full signature and similarity pipeline
func TestIntegration_SignatureAndSimilarityPipeline(t *testing.T) {
	extractor := NewSignatureExtractor()
	calculator := NewSimilarityCalculator()

	// Create two workloads
	workload1 := &Workload{
		UID:     "wl-1",
		Image:   "registry.example.com/pytorch:v1.9.0",
		Command: []string{"/usr/bin/python3", "train.py"},
		Args:    []string{"--epochs", "100", "--lr", "0.001"},
		Env: map[string]string{
			"FRAMEWORK":    "PyTorch",
			"WORLD_SIZE":   "8",
			"API_PASSWORD": "secret", // Will be filtered
		},
		Labels: map[string]string{
			"app":               "training",
			"pod-template-hash": "abc", // Will be filtered
		},
		Namespace: "default",
	}

	workload2 := &Workload{
		UID:     "wl-2",
		Image:   "registry.example.com/pytorch:v1.9.1", // Different tag
		Command: []string{"/opt/python", "train.py"},   // Different path, same command
		Args:    []string{"--epochs", "100", "--lr", "0.001"},
		Env: map[string]string{
			"FRAMEWORK":  "PyTorch",
			"WORLD_SIZE": "8",
			"API_TOKEN":  "token", // Different sensitive var, will be filtered
		},
		Labels: map[string]string{
			"app":                      "training",
			"controller-revision-hash": "def", // Different dynamic label, will be filtered
		},
		Namespace: "default",
	}

	// Extract signatures
	sig1 := extractor.ExtractSignature(workload1)
	sig2 := extractor.ExtractSignature(workload2)

	// Verify filtering
	assert.NotContains(t, sig1.Env, "API_PASSWORD")
	assert.NotContains(t, sig1.Labels, "pod-template-hash")
	assert.NotContains(t, sig2.Env, "API_TOKEN")
	assert.NotContains(t, sig2.Labels, "controller-revision-hash")

	// Verify command normalization (python3/python -> python)
	assert.Equal(t, []string{"python", "train.py"}, sig1.Command)
	assert.Equal(t, []string{"python", "train.py"}, sig2.Command)

	// Calculate similarity
	result := calculator.CalculateSimilarity(sig1, sig2)

	// Should be highly similar despite minor differences
	assert.Greater(t, result.Score, 0.85)

	// Image should be similar (same repo, different tag)
	assert.InDelta(t, 0.9, result.Details.ImageScore, 0.1)

	// Env should match (after filtering sensitive vars)
	assert.Equal(t, 1.0, result.Details.EnvScore)

	// Args should match exactly
	assert.Equal(t, 1.0, result.Details.ArgsScore)

	// Labels should match (after filtering dynamic labels)
	assert.Equal(t, 1.0, result.Details.LabelScore)
}

// TestIntegration_CachingBehavior tests the caching mechanism
func TestIntegration_CachingBehavior(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	config.CacheTTLMinutes = 1 // Short TTL for testing
	engine := NewReuseEngine(mockDB, config)
	ctx := context.Background()

	candidate := createHistoricalWorkload(
		"cached-uid",
		"registry.example.com/pytorch:v1.9.0",
		[]string{"python", "train.py"},
		map[string]string{"FRAMEWORK": "PyTorch"},
		0.95,
	)

	workload := &Workload{
		UID:       "new-uid",
		Image:     "registry.example.com/pytorch:v1.9.0",
		Command:   []string{"python", "train.py"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch"},
		Namespace: "default",
	}

	// First call - should query database
	mockDB.On("FindCandidateWorkloads",
		ctx,
		"registry.example.com/pytorch",
		mock.Anything,
		config.MinConfidence,
		config.MaxCandidates,
	).Return([]*model.AiWorkloadMetadata{candidate}, nil).Once()

	mockDB.On("GetAiWorkloadMetadata",
		ctx,
		"cached-uid",
	).Return(candidate, nil).Once()

	detection1, err := engine.TryReuse(ctx, workload)
	require.NoError(t, err)
	require.NotNil(t, detection1)

	// Second call with same signature - should use cache (no DB call)
	detection2, err := engine.TryReuse(ctx, workload)
	require.NoError(t, err)
	require.NotNil(t, detection2)

	// Verify both detections are equivalent
	assert.Equal(t, detection1.Frameworks, detection2.Frameworks)
	assert.Equal(t, detection1.Confidence, detection2.Confidence)

	mockDB.AssertExpectations(t)
}

// Helper function to create historical workload with detection
func createHistoricalWorkload(
	uid string,
	image string,
	command []string,
	env map[string]string,
	confidence float64,
) *model.AiWorkloadMetadata {
	detection := coreModel.FrameworkDetection{
		Frameworks: []string{"pytorch"},
		Type:       "training",
		Confidence: confidence,
		Status:     coreModel.DetectionStatusVerified,
		Version:    "1.9.0",
		Sources: []coreModel.DetectionSource{
			{
				Source:     "logs",
				Frameworks: []string{"pytorch"},
				Type:       "training",
				Confidence: confidence,
				DetectedAt: time.Now().Add(-1 * time.Hour),
			},
		},
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	// Create signature
	extractor := NewSignatureExtractor()
	signature := extractor.ExtractSignature(&Workload{
		UID:       uid,
		Image:     image,
		Command:   command,
		Env:       env,
		Namespace: "default",
	})

	metadata := map[string]interface{}{
		"framework_detection": detection,
		"workload_signature":  signature,
	}

	metadataBytes, _ := json.Marshal(metadata)
	var metadataMap map[string]interface{}
	json.Unmarshal(metadataBytes, &metadataMap)

	return &model.AiWorkloadMetadata{
		WorkloadUID: uid,
		ImagePrefix: ExtractImageRepo(image),
		Metadata:    metadataMap,
		CreatedAt:   time.Now().Add(-2 * time.Hour),
	}
}
