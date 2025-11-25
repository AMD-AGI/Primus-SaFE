package framework

import (
	"context"
	"testing"
	"time"
	
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"gorm.io/gorm"
)

func setupTestManager(t *testing.T) (*FrameworkDetectionManager, *MockAiWorkloadMetadataFacade) {
	mockFacade := new(MockAiWorkloadMetadataFacade)
	config := DefaultDetectionConfig()
	config.EnableCache = false // Disable cache for testing
	
	manager := NewFrameworkDetectionManager(mockFacade, config)
	return manager, mockFacade
}

// TestScenario1_LogFirst_ThenComponent tests the scenario where log detection arrives first
func TestScenario1_LogFirst_ThenComponent(t *testing.T) {
	manager, mockFacade := setupTestManager(t)
	ctx := context.Background()
	workloadUID := "test-workload-1"
	
	// Setup: No existing metadata
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(nil, gorm.ErrRecordNotFound).Once()
	mockFacade.On("CreateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 1: Log detection arrives
	err := manager.ReportDetection(ctx, workloadUID,
		"log", "primus", "training", 0.7, map[string]interface{}{
			"method": "log_pattern",
		})
	require.NoError(t, err)
	
	// Verify first report
	mockFacade.AssertExpectations(t)
	
	// Setup for second report: return existing metadata
	existingMetadata := &model.AiWorkloadMetadata{
		WorkloadUID: workloadUID,
		Framework:   "primus",
		Type:        "training",
		Metadata: model.ExtType{
			"framework_detection": map[string]interface{}{
				"framework":  "primus",
				"type":       "training",
				"confidence": 0.7,
				"status":     "confirmed",
				"sources": []interface{}{
					map[string]interface{}{
						"source":     "log",
						"framework":  "primus",
						"confidence": 0.7,
					},
				},
			},
		},
	}
	
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(existingMetadata, nil).Once()
	mockFacade.On("UpdateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 2: Component detection arrives
	err = manager.ReportDetection(ctx, workloadUID,
		"component", "primus", "training", 0.9, map[string]interface{}{
			"method": "image_analysis",
		})
	require.NoError(t, err)
	
	mockFacade.AssertExpectations(t)
}

// TestScenario2_ConflictResolution tests conflict resolution by priority
func TestScenario2_ConflictResolution(t *testing.T) {
	manager, mockFacade := setupTestManager(t)
	ctx := context.Background()
	workloadUID := "test-workload-2"
	
	// Setup: No existing metadata
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(nil, gorm.ErrRecordNotFound).Once()
	mockFacade.On("CreateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 1: Log detection for primus
	err := manager.ReportDetection(ctx, workloadUID,
		"log", "primus", "training", 0.8, nil)
	require.NoError(t, err)
	
	// Setup existing metadata with log detection
	existingMetadata := &model.AiWorkloadMetadata{
		WorkloadUID: workloadUID,
		Framework:   "primus",
		Type:        "training",
		Metadata: model.ExtType{
			"framework_detection": map[string]interface{}{
				"framework":  "primus",
				"type":       "training",
				"confidence": 0.8,
				"status":     "confirmed",
				"sources": []interface{}{
					map[string]interface{}{
						"source":     "log",
						"framework":  "primus",
						"confidence": 0.8,
					},
				},
			},
		},
	}
	
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(existingMetadata, nil).Once()
	mockFacade.On("UpdateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 2: Component detection for deepspeed (conflict)
	err = manager.ReportDetection(ctx, workloadUID,
		"component", "deepspeed", "training", 0.85, nil)
	require.NoError(t, err)
	
	mockFacade.AssertExpectations(t)
}

// TestScenario3_ReuseWithVerification tests reuse followed by verification
func TestScenario3_ReuseWithVerification(t *testing.T) {
	manager, mockFacade := setupTestManager(t)
	ctx := context.Background()
	workloadUID := "test-workload-3"
	
	// Setup: No existing metadata
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(nil, gorm.ErrRecordNotFound).Once()
	mockFacade.On("CreateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 1: Reuse detection
	err := manager.ReportDetection(ctx, workloadUID,
		"reuse", "primus", "training", 0.85, map[string]interface{}{
			"reused_from": "workload-xyz",
		})
	require.NoError(t, err)
	
	// Setup existing metadata with reuse
	existingMetadata := &model.AiWorkloadMetadata{
		WorkloadUID: workloadUID,
		Framework:   "primus",
		Type:        "training",
		Metadata: model.ExtType{
			"framework_detection": map[string]interface{}{
				"framework":  "primus",
				"type":       "training",
				"confidence": 0.85,
				"status":     "reused",
				"sources": []interface{}{
					map[string]interface{}{
						"source":     "reuse",
						"framework":  "primus",
						"confidence": 0.85,
					},
				},
			},
		},
	}
	
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(existingMetadata, nil).Once()
	mockFacade.On("UpdateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 2: Component verification
	err = manager.ReportDetection(ctx, workloadUID,
		"component", "primus", "training", 0.9, nil)
	require.NoError(t, err)
	
	mockFacade.AssertExpectations(t)
}

// TestScenario4_UserOverride tests user manual correction
func TestScenario4_UserOverride(t *testing.T) {
	manager, mockFacade := setupTestManager(t)
	ctx := context.Background()
	workloadUID := "test-workload-4"
	
	// Setup: No existing metadata
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(nil, gorm.ErrRecordNotFound).Once()
	mockFacade.On("CreateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 1: Incorrect detection
	err := manager.ReportDetection(ctx, workloadUID,
		"log", "deepspeed", "training", 0.7, nil)
	require.NoError(t, err)
	
	// Setup existing metadata with wrong detection
	existingMetadata := &model.AiWorkloadMetadata{
		WorkloadUID: workloadUID,
		Framework:   "deepspeed",
		Type:        "training",
		Metadata: model.ExtType{
			"framework_detection": map[string]interface{}{
				"framework":  "deepspeed",
				"type":       "training",
				"confidence": 0.7,
				"status":     "confirmed",
				"sources": []interface{}{
					map[string]interface{}{
						"source":     "log",
						"framework":  "deepspeed",
						"confidence": 0.7,
					},
				},
			},
		},
	}
	
	mockFacade.On("GetAiWorkloadMetadata", ctx, workloadUID).
		Return(existingMetadata, nil).Once()
	mockFacade.On("UpdateAiWorkloadMetadata", ctx, mock.Anything).
		Return(nil).Once()
	
	// Step 2: User correction (should win due to highest priority)
	err = manager.ReportDetection(ctx, workloadUID,
		"user", "primus", "training", 1.0, map[string]interface{}{
			"reason": "manual correction",
		})
	require.NoError(t, err)
	
	mockFacade.AssertExpectations(t)
}

// TestValidateInput tests input validation
func TestValidateInput(t *testing.T) {
	manager, _ := setupTestManager(t)
	
	tests := []struct {
		name        string
		source      string
		framework   string
		confidence  float64
		shouldError bool
	}{
		{"Valid input", "log", "primus", 0.7, false},
		{"Empty source", "", "primus", 0.7, true},
		{"Empty framework", "log", "", 0.7, true},
		{"Confidence too low", "log", "primus", -0.1, true},
		{"Confidence too high", "log", "primus", 1.5, true},
		{"Confidence at boundary low", "log", "primus", 0.0, false},
		{"Confidence at boundary high", "log", "primus", 1.0, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := manager.validateInput(tt.source, tt.framework, tt.confidence)
			if tt.shouldError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestMergeDetections_FirstDetection tests merging when no existing detection
func TestMergeDetections_FirstDetection(t *testing.T) {
	manager, _ := setupTestManager(t)
	
	newSource := &coreModel.DetectionSource{
		Source:     "log",
		Framework:  "primus",
		Type:       "training",
		Confidence: 0.7,
		DetectedAt: time.Now(),
		Evidence:   map[string]interface{}{"method": "log_pattern"},
	}
	
	result, err := manager.MergeDetections(nil, newSource)
	require.NoError(t, err)
	
	assert.Equal(t, "primus", result.Framework)
	assert.Equal(t, "training", result.Type)
	assert.Equal(t, 0.7, result.Confidence)
	assert.Len(t, result.Sources, 1)
	assert.Empty(t, result.Conflicts)
	assert.Equal(t, coreModel.DetectionStatusConfirmed, result.Status)
}

// TestMergeDetections_UpdateExistingSource tests updating an existing source
func TestMergeDetections_UpdateExistingSource(t *testing.T) {
	manager, _ := setupTestManager(t)
	
	existing := &coreModel.FrameworkDetection{
		Framework:  "primus",
		Type:       "training",
		Confidence: 0.6,
		Status:     coreModel.DetectionStatusSuspected,
		Sources: []coreModel.DetectionSource{
			{
				Source:     "log",
				Framework:  "primus",
				Confidence: 0.6,
				DetectedAt: time.Now().Add(-1 * time.Hour),
			},
		},
	}
	
	newSource := &coreModel.DetectionSource{
		Source:     "log",  // Same source, should update
		Framework:  "primus",
		Type:       "training",
		Confidence: 0.8,  // Increased confidence
		DetectedAt: time.Now(),
	}
	
	result, err := manager.MergeDetections(existing, newSource)
	require.NoError(t, err)
	
	assert.Equal(t, "primus", result.Framework)
	assert.Len(t, result.Sources, 1, "Should still have only one source (updated)")
	assert.Equal(t, 0.8, result.Sources[0].Confidence, "Confidence should be updated")
}

// TestGetStats tests statistics retrieval
func TestGetStats(t *testing.T) {
	manager, _ := setupTestManager(t)
	
	stats := manager.GetStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "cache_enabled")
	assert.Equal(t, false, stats["cache_enabled"])
}

// TestGetConfig tests configuration retrieval
func TestGetConfig(t *testing.T) {
	manager, _ := setupTestManager(t)
	
	config := manager.GetConfig()
	assert.NotNil(t, config)
	assert.Equal(t, 0.3, config.SuspectedThreshold)
	assert.Equal(t, 0.6, config.ConfirmedThreshold)
	assert.Equal(t, 0.85, config.VerifiedThreshold)
}

