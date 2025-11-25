package framework

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

func TestReuseEngine_TryReuse_Disabled(t *testing.T) {
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	config.Enabled = false

	engine := NewReuseEngine(mockDB, config)

	workload := &Workload{
		UID:       "test-uid",
		Image:     "registry.example.com/primus:v1.2.3",
		Namespace: "default",
	}

	result, err := engine.TryReuse(context.Background(), workload)

	assert.NoError(t, err)
	assert.Nil(t, result)
	mockDB.AssertNotCalled(t, "FindCandidateWorkloads")
}

func TestReuseEngine_TryReuse_NoCandidates(t *testing.T) {
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	engine := NewReuseEngine(mockDB, config)

	workload := &Workload{
		UID:       "test-uid",
		Image:     "registry.example.com/primus:v1.2.3",
		Command:   []string{"python", "train.py"},
		Namespace: "default",
	}

	mockDB.On("FindCandidateWorkloads",
		mock.Anything,
		"registry.example.com/primus",
		mock.Anything,
		config.MinConfidence,
		config.MaxCandidates,
	).Return([]*model.AiWorkloadMetadata{}, nil)

	result, err := engine.TryReuse(context.Background(), workload)

	assert.NoError(t, err)
	assert.Nil(t, result)
	mockDB.AssertExpectations(t)
}

func TestReuseEngine_TryReuse_BelowThreshold(t *testing.T) {
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	config.MinSimilarityScore = 0.90 // High threshold
	engine := NewReuseEngine(mockDB, config)

	newWorkload := &Workload{
		UID:       "new-uid",
		Image:     "registry.example.com/primus:v2.0.0", // Different version
		Command:   []string{"python", "test.py"},        // Different command
		Namespace: "default",
	}

	// Create candidate with different characteristics
	candidateWorkload := createMockCandidate("old-uid", "registry.example.com/primus:v1.0.0", []string{"python", "train.py"})

	mockDB.On("FindCandidateWorkloads",
		mock.Anything,
		"registry.example.com/primus",
		mock.Anything,
		config.MinConfidence,
		config.MaxCandidates,
	).Return([]*model.AiWorkloadMetadata{candidateWorkload}, nil)

	result, err := engine.TryReuse(context.Background(), newWorkload)

	assert.NoError(t, err)
	assert.Nil(t, result) // Should not reuse due to low similarity
	mockDB.AssertExpectations(t)
}

func TestReuseEngine_TryReuse_Success(t *testing.T) {
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	config.MinSimilarityScore = 0.85
	config.ConfidenceDecayRate = 0.9
	engine := NewReuseEngine(mockDB, config)

	newWorkload := &Workload{
		UID:       "new-uid",
		Image:     "registry.example.com/primus:v1.2.4", // Similar version
		Command:   []string{"python", "train.py"},
		Args:      []string{"--epochs", "100"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch"},
		Labels:    map[string]string{"app": "training"},
		Namespace: "default",
	}

	// Create highly similar candidate
	candidateWorkload := createMockCandidate(
		"old-uid",
		"registry.example.com/primus:v1.2.3",
		[]string{"python", "train.py"},
	)

	// Create detailed metadata with framework detection
	detectionData := coreModel.FrameworkDetection{
		Framework:  "pytorch",
		Type:       "training",
		Confidence: 0.95,
		Version:    "1.9.0",
		Status:     coreModel.DetectionStatusVerified,
		Sources: []coreModel.DetectionSource{
			{
				Source:     "logs",
				Framework:  "pytorch",
				Type:       "training",
				Confidence: 0.95,
			},
		},
	}

	candidateMetadata := &model.AiWorkloadMetadata{
		WorkloadUID: "old-uid",
		ImagePrefix: "registry.example.com/primus",
		Metadata: map[string]interface{}{
			"framework_detection": detectionData,
		},
		CreatedAt: time.Now().Add(-1 * time.Hour),
	}

	mockDB.On("FindCandidateWorkloads",
		mock.Anything,
		"registry.example.com/primus",
		mock.Anything,
		config.MinConfidence,
		config.MaxCandidates,
	).Return([]*model.AiWorkloadMetadata{candidateWorkload}, nil)

	mockDB.On("GetAiWorkloadMetadata",
		mock.Anything,
		"old-uid",
	).Return(candidateMetadata, nil)

	result, err := engine.TryReuse(context.Background(), newWorkload)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "pytorch", result.Framework)
	assert.Equal(t, "training", result.Type)
	assert.Equal(t, coreModel.DetectionStatusReused, result.Status)
	
	// Check confidence decay
	expectedConfidence := 0.95 * config.ConfidenceDecayRate
	assert.InDelta(t, expectedConfidence, result.Confidence, 0.001)

	// Check reuse info
	assert.NotNil(t, result.ReuseInfo)
	assert.Equal(t, "old-uid", result.ReuseInfo.ReusedFrom)
	assert.Equal(t, 0.95, result.ReuseInfo.OriginalConfidence)
	assert.Greater(t, result.ReuseInfo.SimilarityScore, 0.85)

	// Check sources
	assert.Len(t, result.Sources, 1)
	assert.Equal(t, "reuse", result.Sources[0].Source)

	mockDB.AssertExpectations(t)
}

func TestReuseEngine_FindCandidates(t *testing.T) {
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	engine := NewReuseEngine(mockDB, config)

	signature := &coreModel.WorkloadSignature{
		Image:     "registry.example.com/primus:v1.2.3",
		Namespace: "default",
	}

	candidates := []*model.AiWorkloadMetadata{
		createMockCandidate("uid-1", "registry.example.com/primus:v1.2.2", []string{"python", "train.py"}),
		createMockCandidate("uid-2", "registry.example.com/primus:v1.2.1", []string{"python", "train.py"}),
	}

	mockDB.On("FindCandidateWorkloads",
		mock.Anything,
		"registry.example.com/primus",
		mock.Anything,
		config.MinConfidence,
		config.MaxCandidates,
	).Return(candidates, nil)

	results, err := engine.findCandidates(context.Background(), signature)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "uid-1", results[0].WorkloadUID)
	assert.Equal(t, "uid-2", results[1].WorkloadUID)
	mockDB.AssertExpectations(t)
}

func TestReuseEngine_CalculateSimilarities(t *testing.T) {
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	engine := NewReuseEngine(mockDB, config)

	signature := &coreModel.WorkloadSignature{
		Image:     "registry.example.com/primus:v1.2.3",
		Command:   []string{"python", "train.py"},
		Args:      []string{"--epochs", "100"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch"},
		Labels:    map[string]string{"app": "training"},
		Namespace: "default",
	}

	candidates := []*coreModel.CandidateWorkload{
		{
			WorkloadUID: "uid-1",
			Signature: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.3",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "100"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
		},
		{
			WorkloadUID: "uid-2",
			Signature: &coreModel.WorkloadSignature{
				Image:     "registry.example.com/primus:v1.2.4",
				Command:   []string{"python", "train.py"},
				Args:      []string{"--epochs", "50"},
				Env:       map[string]string{"FRAMEWORK": "PyTorch"},
				Labels:    map[string]string{"app": "training"},
				Namespace: "default",
			},
		},
	}

	results := engine.calculateSimilarities(signature, candidates)

	assert.Len(t, results, 2)
	assert.Equal(t, "uid-1", results[0].WorkloadUID)
	assert.Equal(t, "uid-2", results[1].WorkloadUID)
	
	// First candidate should have higher similarity (exact match)
	assert.Greater(t, results[0].Score, results[1].Score)
	assert.Equal(t, 1.0, results[0].Score)
}

func TestReuseEngine_BuildCacheKey(t *testing.T) {
	mockDB := new(MockAiWorkloadMetadataFacade)
	config := coreModel.DefaultReuseConfig()
	engine := NewReuseEngine(mockDB, config)

	signature := &coreModel.WorkloadSignature{
		ImageHash:   "abc123",
		CommandHash: "def456",
		EnvHash:     "ghi789",
	}

	key := engine.buildCacheKey(signature)
	assert.Equal(t, "sig:abc123:def456:ghi789", key)
}

// Helper function to create mock candidate workload
func createMockCandidate(uid, image string, command []string) *model.AiWorkloadMetadata {
	detection := coreModel.FrameworkDetection{
		Framework:  "pytorch",
		Type:       "training",
		Confidence: 0.90,
		Status:     coreModel.DetectionStatusVerified,
	}

	signature := coreModel.WorkloadSignature{
		Image:     image,
		Command:   command,
		Args:      []string{"--epochs", "100"},
		Env:       map[string]string{"FRAMEWORK": "PyTorch"},
		Labels:    map[string]string{"app": "training"},
		Namespace: "default",
	}

	// Create signature extractor to calculate hashes
	extractor := NewSignatureExtractor()
	fullSignature := extractor.ExtractSignature(&Workload{
		UID:       uid,
		Image:     image,
		Command:   command,
		Args:      signature.Args,
		Env:       signature.Env,
		Labels:    signature.Labels,
		Namespace: signature.Namespace,
	})

	metadata := map[string]interface{}{
		"framework_detection": detection,
		"workload_signature":  fullSignature,
	}

	return &model.AiWorkloadMetadata{
		WorkloadUID: uid,
		ImagePrefix: ExtractImageRepo(image),
		Metadata:    metadata,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
	}
}

func TestReuseConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  coreModel.ReuseConfig
		wantErr bool
	}{
		{
			name:    "valid default config",
			config:  coreModel.DefaultReuseConfig(),
			wantErr: false,
		},
		{
			name: "invalid min similarity score - too low",
			config: coreModel.ReuseConfig{
				MinSimilarityScore: -0.1,
				TimeWindowDays:     30,
				MinConfidence:      0.75,
				ConfidenceDecayRate: 0.9,
				MaxCandidates:      100,
				CacheTTLMinutes:    10,
			},
			wantErr: true,
		},
		{
			name: "invalid min similarity score - too high",
			config: coreModel.ReuseConfig{
				MinSimilarityScore: 1.5,
				TimeWindowDays:     30,
				MinConfidence:      0.75,
				ConfidenceDecayRate: 0.9,
				MaxCandidates:      100,
				CacheTTLMinutes:    10,
			},
			wantErr: true,
		},
		{
			name: "invalid time window days",
			config: coreModel.ReuseConfig{
				MinSimilarityScore: 0.85,
				TimeWindowDays:     0,
				MinConfidence:      0.75,
				ConfidenceDecayRate: 0.9,
				MaxCandidates:      100,
				CacheTTLMinutes:    10,
			},
			wantErr: true,
		},
		{
			name: "invalid confidence decay rate",
			config: coreModel.ReuseConfig{
				MinSimilarityScore: 0.85,
				TimeWindowDays:     30,
				MinConfidence:      0.75,
				ConfidenceDecayRate: 1.5,
				MaxCandidates:      100,
				CacheTTLMinutes:    10,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Test serialization/deserialization of detection data
func TestFrameworkDetection_JSON(t *testing.T) {
	detection := coreModel.FrameworkDetection{
		Framework:  "pytorch",
		Type:       "training",
		Confidence: 0.95,
		Version:    "1.9.0",
		Status:     coreModel.DetectionStatusVerified,
		ReuseInfo: &coreModel.ReuseInfo{
			ReusedFrom:         "old-uid",
			ReusedAt:           time.Now(),
			SimilarityScore:    0.92,
			OriginalConfidence: 0.95,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(detection)
	assert.NoError(t, err)
	assert.NotEmpty(t, data)

	// Unmarshal from JSON
	var decoded coreModel.FrameworkDetection
	err = json.Unmarshal(data, &decoded)
	assert.NoError(t, err)
	assert.Equal(t, detection.Framework, decoded.Framework)
	assert.Equal(t, detection.Type, decoded.Type)
	assert.Equal(t, detection.Confidence, decoded.Confidence)
	assert.NotNil(t, decoded.ReuseInfo)
	assert.Equal(t, detection.ReuseInfo.ReusedFrom, decoded.ReuseInfo.ReusedFrom)
}

