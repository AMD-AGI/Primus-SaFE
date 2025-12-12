package database

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"gorm.io/gorm"
)

// FrameworkDetectionStorage provides storage operations for framework detection
type FrameworkDetectionStorage struct {
	metadataFacade AiWorkloadMetadataFacadeInterface
}

// NewFrameworkDetectionStorage creates a new framework detection storage
func NewFrameworkDetectionStorage(facade AiWorkloadMetadataFacadeInterface) *FrameworkDetectionStorage {
	return &FrameworkDetectionStorage{
		metadataFacade: facade,
	}
}

// GetDetection retrieves framework detection result for a workload
func (s *FrameworkDetectionStorage) GetDetection(
	ctx context.Context,
	workloadUID string,
) (*coreModel.FrameworkDetection, error) {
	metadata, err := s.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	if metadata == nil {
		return nil, gorm.ErrRecordNotFound
	}

	// Extract framework_detection from metadata JSONB
	detection, err := s.extractDetectionFromMetadata(metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to extract detection: %w", err)
	}

	return detection, nil
}

// SaveDetection saves a new framework detection result
func (s *FrameworkDetectionStorage) SaveDetection(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Build metadata map
	metadataMap, err := s.buildMetadataMap(detection, nil)
	if err != nil {
		return fmt.Errorf("failed to build metadata: %w", err)
	}

	// Join all frameworks with comma as the framework value
	frameworkValue := strings.Join(detection.Frameworks, ",")

	metadata := &model.AiWorkloadMetadata{
		WorkloadUID: workloadUID,
		Type:        detection.Type,
		Framework:   frameworkValue,
		Metadata:    metadataMap,
		CreatedAt:   time.Now(),
	}

	return s.metadataFacade.CreateAiWorkloadMetadata(ctx, metadata)
}

// UpdateDetection updates an existing framework detection result
func (s *FrameworkDetectionStorage) UpdateDetection(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Load existing metadata
	existingMetadata, err := s.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		return fmt.Errorf("failed to get existing metadata: %w", err)
	}

	if existingMetadata == nil {
		// No existing metadata, create new one
		return s.SaveDetection(ctx, workloadUID, detection)
	}

	// Build updated metadata map
	metadataMap, err := s.buildMetadataMap(detection, existingMetadata)
	if err != nil {
		return fmt.Errorf("failed to build metadata: %w", err)
	}

	// Join all frameworks with comma as the framework value
	frameworkValue := strings.Join(detection.Frameworks, ",")

	existingMetadata.Type = detection.Type
	existingMetadata.Framework = frameworkValue
	existingMetadata.Metadata = metadataMap

	return s.metadataFacade.UpdateAiWorkloadMetadata(ctx, existingMetadata)
}

// UpsertDetection creates or updates framework detection result
func (s *FrameworkDetectionStorage) UpsertDetection(
	ctx context.Context,
	workloadUID string,
	detection *coreModel.FrameworkDetection,
) error {
	// Try to get existing metadata
	existing, err := s.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if existing == nil {
		return s.SaveDetection(ctx, workloadUID, detection)
	}

	return s.UpdateDetection(ctx, workloadUID, detection)
}

// ListDetections lists framework detection results with filters
func (s *FrameworkDetectionStorage) ListDetections(
	ctx context.Context,
	filters map[string]interface{},
) ([]*coreModel.FrameworkDetection, error) {
	// Extract filter parameters
	framework, _ := filters["framework"].(string)
	status, _ := filters["status"].(string)
	minConfidence, _ := filters["min_confidence"].(float64)
	limit, _ := filters["limit"].(int)

	if limit == 0 {
		limit = 100
	}

	// Build query - this is a simplified version
	// In production, you'd want more sophisticated filtering
	metadata, err := s.metadataFacade.GetAiWorkloadMetadata(ctx, "")
	if err != nil {
		return nil, err
	}

	// For now, return empty list
	// TODO: Implement proper filtering with database queries
	_ = framework
	_ = status
	_ = minConfidence
	_ = metadata

	return []*coreModel.FrameworkDetection{}, nil
}

// extractDetectionFromMetadata extracts FrameworkDetection from metadata
func (s *FrameworkDetectionStorage) extractDetectionFromMetadata(
	metadata *model.AiWorkloadMetadata,
) (*coreModel.FrameworkDetection, error) {
	metadataMap := metadata.Metadata

	detectionData, ok := metadataMap["framework_detection"]
	if !ok {
		return nil, nil
	}

	detectionJSON, err := json.Marshal(detectionData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal detection data: %w", err)
	}

	var detection coreModel.FrameworkDetection
	if err := json.Unmarshal(detectionJSON, &detection); err != nil {
		return nil, fmt.Errorf("failed to unmarshal detection: %w", err)
	}

	return &detection, nil
}

// buildMetadataMap builds the metadata map including framework_detection
func (s *FrameworkDetectionStorage) buildMetadataMap(
	detection *coreModel.FrameworkDetection,
	existingMetadata *model.AiWorkloadMetadata,
) (model.ExtType, error) {
	var metadataMap model.ExtType

	if existingMetadata != nil && len(existingMetadata.Metadata) > 0 {
		// Preserve existing metadata fields
		metadataMap = existingMetadata.Metadata
	} else {
		metadataMap = make(model.ExtType)
	}

	// Update framework_detection field
	metadataMap["framework_detection"] = detection

	// Extract and store WandB information from the latest wandb source
	// This makes it easier to access WandB metadata without parsing through sources
	if len(detection.Sources) > 0 {
		for i := len(detection.Sources) - 1; i >= 0; i-- {
			source := detection.Sources[i]
			if source.Source == "wandb" && source.Evidence != nil {
				// Save complete wandb information
				if wandbInfo, ok := source.Evidence["wandb"]; ok {
					metadataMap["wandb"] = wandbInfo
				}

				// Save environment information
				if envInfo, ok := source.Evidence["environment"]; ok {
					metadataMap["environment"] = envInfo
				}

				// Save pytorch information
				if pytorchInfo, ok := source.Evidence["pytorch"]; ok {
					metadataMap["pytorch"] = pytorchInfo
				}

				// Save system information
				if systemInfo, ok := source.Evidence["system"]; ok {
					metadataMap["system"] = systemInfo
				}

				// Save detailed information of wrapper and base frameworks
				if wrapperInfo, ok := source.Evidence["wrapper_frameworks_detail"]; ok {
					metadataMap["wrapper_frameworks_detail"] = wrapperInfo
				}
				if baseInfo, ok := source.Evidence["base_frameworks_detail"]; ok {
					metadataMap["base_frameworks_detail"] = baseInfo
				}

				// Save framework layer information
				if layer, ok := source.Evidence["framework_layer"]; ok {
					metadataMap["framework_layer"] = layer
				}
				if wrapperFw, ok := source.Evidence["wrapper_framework"]; ok {
					metadataMap["wrapper_framework"] = wrapperFw
				}
				if baseFw, ok := source.Evidence["base_framework"]; ok {
					metadataMap["base_framework"] = baseFw
				}

				break
			}
		}
	}

	return metadataMap, nil
}

// StatisticsResult represents statistics results
type StatisticsResult struct {
	TotalWorkloads    int64            `json:"total_workloads"`
	ByFramework       map[string]int64 `json:"by_framework"`
	ByStatus          map[string]int64 `json:"by_status"`
	BySource          map[string]int64 `json:"by_source"`
	AverageConfidence float64          `json:"average_confidence"`
	ConflictRate      float64          `json:"conflict_rate"`
	ReuseRate         float64          `json:"reuse_rate"`
}

// GetStatistics retrieves framework detection statistics
func (s *FrameworkDetectionStorage) GetStatistics(
	ctx context.Context,
	startTime string,
	endTime string,
	namespace string,
) (*StatisticsResult, error) {

	// Note: Since the facade interface does not provide direct DB access, this uses a simplified implementation
	// In production environment, statistics methods should be added to the facade interface, or use a dedicated statistics service

	// Simplified implementation: return basic statistics
	// TODO: Add more comprehensive statistics query methods to facade interface

	logrus.Warn("GetStatistics using simplified implementation. Consider adding statistics methods to facade interface.")

	return &StatisticsResult{
		TotalWorkloads:    0,
		ByFramework:       make(map[string]int64),
		ByStatus:          make(map[string]int64),
		BySource:          make(map[string]int64),
		AverageConfidence: 0,
		ConflictRate:      0,
		ReuseRate:         0,
	}, nil
}
