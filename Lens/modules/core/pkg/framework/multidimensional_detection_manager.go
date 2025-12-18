package framework

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// MultiDimensionalDetectionManager manages detection across multiple dimensions
type MultiDimensionalDetectionManager struct {
	mu sync.RWMutex

	// Dimension-specific detectors
	detectors map[model.DetectionDimension]model.DimensionDetector

	// Storage for multi-dimensional detection results
	storage *MultiDimensionalDetectionStorage

	// Configuration
	config *DetectionConfig

	// Event dispatcher
	eventDispatcher *EventDispatcher

	// Compatibility validator
	validator *DimensionCompatibilityValidator
}

// NewMultiDimensionalDetectionManager creates a new multi-dimensional detection manager
func NewMultiDimensionalDetectionManager(config *DetectionConfig) *MultiDimensionalDetectionManager {
	if config == nil {
		config = DefaultDetectionConfig()
	}

	return &MultiDimensionalDetectionManager{
		detectors:       make(map[model.DetectionDimension]model.DimensionDetector),
		storage:         NewMultiDimensionalDetectionStorage(),
		config:          config,
		eventDispatcher: NewEventDispatcher(),
		validator:       NewDimensionCompatibilityValidator(),
	}
}

// RegisterDetector registers a dimension-specific detector
func (m *MultiDimensionalDetectionManager) RegisterDetector(detector model.DimensionDetector) {
	m.mu.Lock()
	defer m.mu.Unlock()

	dimension := detector.GetDimension()
	m.detectors[dimension] = detector
	log.Infof("Registered detector for dimension: %s", dimension)
}

// ReportDimensionDetection reports detection for a specific dimension
func (m *MultiDimensionalDetectionManager) ReportDimensionDetection(
	ctx context.Context,
	workloadUID string,
	dimension model.DetectionDimension,
	value string,
	source string,
	confidence float64,
	evidence map[string]interface{},
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create detection
	detection := m.storage.Get(workloadUID)
	if detection == nil {
		detection = &model.MultiDimensionalDetection{
			WorkloadUID: workloadUID,
			Version:     "2.0",
			Dimensions:  make(map[model.DetectionDimension][]model.DimensionValue),
			Conflicts:   make(map[model.DetectionDimension][]model.DetectionConflict),
			UpdatedAt:   time.Now(),
		}
	}

	// Create new dimension value
	newValue := model.DimensionValue{
		Value:      value,
		Confidence: confidence,
		Source:     source,
		DetectedAt: time.Now(),
		Evidence:   evidence,
	}

	// Add or update dimension value
	dimensionValues := detection.Dimensions[dimension]

	// Check if this source already reported for this dimension
	sourceExists := false
	for i, dv := range dimensionValues {
		if dv.Source == source {
			dimensionValues[i] = newValue
			sourceExists = true
			break
		}
	}

	if !sourceExists {
		dimensionValues = append(dimensionValues, newValue)
	}

	detection.Dimensions[dimension] = dimensionValues

	// Detect conflicts within this dimension
	conflicts := m.detectDimensionConflicts(dimension, dimensionValues)
	if len(conflicts) > 0 {
		detection.Conflicts[dimension] = conflicts
		log.Warnf("Detected %d conflicts in dimension %s for workload %s",
			len(conflicts), dimension, workloadUID)
	}

	// Validate cross-dimension compatibility
	if err := m.validator.Validate(detection); err != nil {
		log.Warnf("Cross-dimension validation warning for workload %s: %v", workloadUID, err)
		// Don't fail, just log warning
	}

	// Update overall confidence and status
	detection.Confidence = m.calculateOverallConfidence(detection)
	detection.Status = m.determineStatus(detection)
	detection.UpdatedAt = time.Now()

	// Save detection
	m.storage.Save(workloadUID, detection)

	log.Infof("Dimension detection reported: workload=%s, dimension=%s, value=%s, confidence=%.2f",
		workloadUID, dimension, value, confidence)

	// Dispatch event
	m.dispatchDimensionEvent(ctx, workloadUID, dimension, detection)

	return nil
}

// GetDetection retrieves multi-dimensional detection for a workload
func (m *MultiDimensionalDetectionManager) GetDetection(workloadUID string) *model.MultiDimensionalDetection {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.storage.Get(workloadUID)
}

// GetDimensionValue gets the resolved value for a specific dimension
// Returns the highest confidence value, or resolved value if there's conflict
func (m *MultiDimensionalDetectionManager) GetDimensionValue(
	workloadUID string,
	dimension model.DetectionDimension,
) (string, float64, error) {
	detection := m.GetDetection(workloadUID)
	if detection == nil {
		return "", 0, fmt.Errorf("no detection found for workload %s", workloadUID)
	}

	values, exists := detection.Dimensions[dimension]
	if !exists || len(values) == 0 {
		return "", 0, fmt.Errorf("no value detected for dimension %s", dimension)
	}

	// If no conflicts, return highest confidence value
	if len(detection.Conflicts[dimension]) == 0 {
		return m.getHighestConfidenceValue(values)
	}

	// If conflicts exist, apply resolution logic
	return m.resolveConflictedValue(dimension, values)
}

// HasDimensionValue checks if a specific value is detected in a dimension
func (m *MultiDimensionalDetectionManager) HasDimensionValue(
	workloadUID string,
	dimension model.DetectionDimension,
	value string,
) bool {
	detection := m.GetDetection(workloadUID)
	if detection == nil {
		return false
	}

	values, exists := detection.Dimensions[dimension]
	if !exists {
		return false
	}

	for _, dv := range values {
		if dv.Value == value {
			return true
		}
	}

	return false
}

// detectDimensionConflicts detects conflicts within a single dimension
func (m *MultiDimensionalDetectionManager) detectDimensionConflicts(
	dimension model.DetectionDimension,
	values []model.DimensionValue,
) []model.DetectionConflict {
	if len(values) <= 1 {
		return nil
	}

	var conflicts []model.DetectionConflict

	// Check for different values from different sources
	valueMap := make(map[string][]string) // value -> sources
	for _, dv := range values {
		valueMap[dv.Value] = append(valueMap[dv.Value], dv.Source)
	}

	// If more than one value detected, there's a conflict
	if len(valueMap) > 1 {
		// Create conflicts for each pair
		valueList := make([]string, 0, len(valueMap))
		for val := range valueMap {
			valueList = append(valueList, val)
		}

		for i := 0; i < len(valueList); i++ {
			for j := i + 1; j < len(valueList); j++ {
				val1 := valueList[i]
				val2 := valueList[j]

				conflict := model.DetectionConflict{
					Source1:    valueMap[val1][0], // First source for this value
					Source2:    valueMap[val2][0],
					Framework1: val1,
					Framework2: val2,
					Resolution: fmt.Sprintf("dimension_%s_conflict", dimension),
					ResolvedAt: time.Time{},
				}
				conflicts = append(conflicts, conflict)
			}
		}
	}

	return conflicts
}

// getHighestConfidenceValue returns value with highest confidence
func (m *MultiDimensionalDetectionManager) getHighestConfidenceValue(
	values []model.DimensionValue,
) (string, float64, error) {
	if len(values) == 0 {
		return "", 0, fmt.Errorf("no values provided")
	}

	// Sort by confidence descending
	sortedValues := make([]model.DimensionValue, len(values))
	copy(sortedValues, values)
	sort.Slice(sortedValues, func(i, j int) bool {
		return sortedValues[i].Confidence > sortedValues[j].Confidence
	})

	highest := sortedValues[0]
	return highest.Value, highest.Confidence, nil
}

// resolveConflictedValue resolves conflicts and returns the best value
func (m *MultiDimensionalDetectionManager) resolveConflictedValue(
	dimension model.DetectionDimension,
	values []model.DimensionValue,
) (string, float64, error) {
	// Simple resolution: highest confidence wins
	// TODO: Implement more sophisticated resolution logic
	return m.getHighestConfidenceValue(values)
}

// calculateOverallConfidence calculates overall confidence across all dimensions
func (m *MultiDimensionalDetectionManager) calculateOverallConfidence(
	detection *model.MultiDimensionalDetection,
) float64 {
	if len(detection.Dimensions) == 0 {
		return 0.0
	}

	totalConfidence := 0.0
	totalWeight := 0.0

	// Weight dimensions by their priority
	priorities := model.GetDimensionPriorities()
	priorityMap := make(map[model.DetectionDimension]int)
	for _, p := range priorities {
		priorityMap[p.Dimension] = p.Priority
	}

	for dimension, values := range detection.Dimensions {
		if len(values) == 0 {
			continue
		}

		// Get highest confidence in this dimension
		maxConf := 0.0
		for _, v := range values {
			if v.Confidence > maxConf {
				maxConf = v.Confidence
			}
		}

		// Weight by priority (lower priority number = higher weight)
		priority := priorityMap[dimension]
		if priority == 0 {
			priority = 10 // Default low weight
		}
		weight := 1.0 / float64(priority)

		totalConfidence += maxConf * weight
		totalWeight += weight
	}

	if totalWeight == 0 {
		return 0.0
	}

	return totalConfidence / totalWeight
}

// determineStatus determines overall detection status
func (m *MultiDimensionalDetectionManager) determineStatus(
	detection *model.MultiDimensionalDetection,
) model.DetectionStatus {
	// If any dimension has conflicts, status is conflict
	for _, conflicts := range detection.Conflicts {
		if len(conflicts) > 0 {
			return model.DetectionStatusConflict
		}
	}

	// Based on overall confidence
	if detection.Confidence >= 0.8 {
		return model.DetectionStatusVerified
	} else if detection.Confidence >= 0.6 {
		return model.DetectionStatusConfirmed
	} else if detection.Confidence >= 0.4 {
		return model.DetectionStatusSuspected
	}

	return model.DetectionStatusUnknown
}

// dispatchDimensionEvent dispatches detection event
func (m *MultiDimensionalDetectionManager) dispatchDimensionEvent(
	ctx context.Context,
	workloadUID string,
	dimension model.DetectionDimension,
	detection *model.MultiDimensionalDetection,
) {
	// TODO: Implement event dispatching for multi-dimensional detection
	log.Debugf("Dimension event: workload=%s, dimension=%s", workloadUID, dimension)
}

// MultiDimensionalDetectionStorage in-memory storage for multi-dimensional detection
type MultiDimensionalDetectionStorage struct {
	mu   sync.RWMutex
	data map[string]*model.MultiDimensionalDetection
}

// NewMultiDimensionalDetectionStorage creates new storage
func NewMultiDimensionalDetectionStorage() *MultiDimensionalDetectionStorage {
	return &MultiDimensionalDetectionStorage{
		data: make(map[string]*model.MultiDimensionalDetection),
	}
}

// Get retrieves detection
func (s *MultiDimensionalDetectionStorage) Get(workloadUID string) *model.MultiDimensionalDetection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.data[workloadUID]
}

// Save saves detection
func (s *MultiDimensionalDetectionStorage) Save(workloadUID string, detection *model.MultiDimensionalDetection) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[workloadUID] = detection
}

// DimensionCompatibilityValidator validates cross-dimension compatibility
type DimensionCompatibilityValidator struct{}

// NewDimensionCompatibilityValidator creates new validator
func NewDimensionCompatibilityValidator() *DimensionCompatibilityValidator {
	return &DimensionCompatibilityValidator{}
}

// Validate validates cross-dimension compatibility
func (v *DimensionCompatibilityValidator) Validate(detection *model.MultiDimensionalDetection) error {
	// TODO: Implement compatibility validation based on rules
	// For now, just return nil
	return nil
}
