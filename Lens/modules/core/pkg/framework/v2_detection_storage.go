package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"gorm.io/gorm"
)

// V2DetectionStorage stores MultiDimensionalDetection (V2) in ai_workload_metadata
// All new detection data is stored in V2 format
type V2DetectionStorage struct {
	metadataFacade database.AiWorkloadMetadataFacadeInterface
}

// NewV2DetectionStorage creates a new V2 detection storage
func NewV2DetectionStorage() *V2DetectionStorage {
	return &V2DetectionStorage{
		metadataFacade: database.NewAiWorkloadMetadataFacade(),
	}
}

// LoadDetection loads V2 detection from ai_workload_metadata.metadata.framework_detection
func (s *V2DetectionStorage) LoadDetection(
	ctx context.Context,
	workloadUID string,
) (*model.MultiDimensionalDetection, error) {
	metadata, err := s.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get metadata: %w", err)
	}

	if metadata == nil {
		return nil, nil
	}

	// Extract framework_detection from metadata JSONB
	detectionData, ok := metadata.Metadata["framework_detection"]
	if !ok {
		return nil, nil
	}

	// Marshal to JSON for parsing
	detectionJSON, err := json.Marshal(detectionData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal detection data: %w", err)
	}

	// Parse as V2
	var detection model.MultiDimensionalDetection
	if err := json.Unmarshal(detectionJSON, &detection); err != nil {
		return nil, fmt.Errorf("failed to unmarshal V2 detection: %w", err)
	}

	// Verify it's V2 format
	if detection.Version != "2.0" {
		return nil, fmt.Errorf("unexpected detection version %s (expected 2.0)", detection.Version)
	}

	return &detection, nil
}

// SaveDetection saves V2 detection to ai_workload_metadata.metadata.framework_detection
// Always saves in V2 format
func (s *V2DetectionStorage) SaveDetection(
	ctx context.Context,
	detection *model.MultiDimensionalDetection,
) error {
	if detection == nil {
		return fmt.Errorf("detection is nil")
	}

	// Ensure V2 version
	detection.Version = "2.0"
	detection.UpdatedAt = time.Now()

	workloadUID := detection.WorkloadUID
	if workloadUID == "" {
		return fmt.Errorf("workload_uid is empty")
	}

	// Load existing metadata or create new
	metadata, err := s.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	if metadata == nil {
		// Create new metadata
		metadata = &dbModel.AiWorkloadMetadata{
			WorkloadUID: workloadUID,
			Type:        inferTypeFromDetection(detection),
			Framework:   inferFrameworkString(detection),
			Metadata:    make(dbModel.ExtType),
			CreatedAt:   time.Now(),
		}
	}

	// Update framework_detection field
	metadata.Metadata["framework_detection"] = detection

	// Update simplified fields for querying
	metadata.Type = inferTypeFromDetection(detection)
	metadata.Framework = inferFrameworkString(detection)

	// Save or update
	if metadata.ID == 0 {
		err = s.metadataFacade.CreateAiWorkloadMetadata(ctx, metadata)
	} else {
		err = s.metadataFacade.UpdateAiWorkloadMetadata(ctx, metadata)
	}

	if err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	log.Debugf("Saved V2 detection for workload %s", workloadUID)
	return nil
}

// UpsertDetection creates or updates V2 detection
func (s *V2DetectionStorage) UpsertDetection(
	ctx context.Context,
	detection *model.MultiDimensionalDetection,
) error {
	return s.SaveDetection(ctx, detection)
}

// DeleteDetection deletes detection data for a workload
func (s *V2DetectionStorage) DeleteDetection(
	ctx context.Context,
	workloadUID string,
) error {
	metadata, err := s.metadataFacade.GetAiWorkloadMetadata(ctx, workloadUID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to get metadata: %w", err)
	}

	if metadata == nil {
		return nil
	}

	// Remove framework_detection field
	delete(metadata.Metadata, "framework_detection")

	return s.metadataFacade.UpdateAiWorkloadMetadata(ctx, metadata)
}

// inferTypeFromDetection infers workload type from detection
func inferTypeFromDetection(detection *model.MultiDimensionalDetection) string {
	// Check behavior dimension
	if behaviorValues, ok := detection.Dimensions[model.DimensionBehavior]; ok && len(behaviorValues) > 0 {
		return behaviorValues[0].Value // "training", "inference", etc.
	}
	return "unknown"
}

// inferFrameworkString infers framework string for simplified querying
func inferFrameworkString(detection *model.MultiDimensionalDetection) string {
	var frameworks []string

	// Add wrapper framework
	if wrapperValues, ok := detection.Dimensions[model.DimensionWrapperFramework]; ok {
		for _, v := range wrapperValues {
			frameworks = append(frameworks, v.Value)
		}
	}

	// Add base framework
	if baseValues, ok := detection.Dimensions[model.DimensionBaseFramework]; ok {
		for _, v := range baseValues {
			frameworks = append(frameworks, v.Value)
		}
	}

	// Add runtime
	if runtimeValues, ok := detection.Dimensions[model.DimensionRuntime]; ok {
		for _, v := range runtimeValues {
			frameworks = append(frameworks, v.Value)
		}
	}

	if len(frameworks) == 0 {
		return "unknown"
	}

	// Deduplicate
	seen := make(map[string]bool)
	unique := []string{}
	for _, fw := range frameworks {
		if !seen[strings.ToLower(fw)] {
			seen[strings.ToLower(fw)] = true
			unique = append(unique, fw)
		}
	}

	return strings.Join(unique, ",")
}
