package framework

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// FrameworkDetectionManager manages framework detection from multiple sources
type FrameworkDetectionManager struct {
	mu sync.RWMutex

	// Storage layer
	storage           *database.FrameworkDetectionStorage
	workloadFacade    database.AiWorkloadMetadataFacadeInterface
	gpuWorkloadFacade database.WorkloadFacadeInterface // For hierarchy queries

	// Detection components
	confidenceCalculator *ConfidenceCalculator
	statusManager        *StatusManager
	conflictResolver     *ConflictResolver
	conflictDetector     *ConflictDetector

	// Configuration
	config *DetectionConfig

	// Cache (optional)
	cache *cache.Cache

	// Workload hierarchy cache: workloadUID -> parentUID
	hierarchyCache *cache.Cache

	// Event dispatcher for detection events
	eventDispatcher *EventDispatcher
}

// NewFrameworkDetectionManager creates a new framework detection manager
func NewFrameworkDetectionManager(
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
	config *DetectionConfig,
) *FrameworkDetectionManager {
	return NewFrameworkDetectionManagerWithFacades(metadataFacade, nil, config)
}

// NewFrameworkDetectionManagerWithFacades creates a new framework detection manager with custom facades
// This is useful for testing with mock facades
func NewFrameworkDetectionManagerWithFacades(
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
	gpuWorkloadFacade database.WorkloadFacadeInterface,
	config *DetectionConfig,
) *FrameworkDetectionManager {
	if config == nil {
		config = DefaultDetectionConfig()
	}

	storage := database.NewFrameworkDetectionStorage(metadataFacade)

	var cacheInstance *cache.Cache
	var hierarchyCacheInstance *cache.Cache
	if config.EnableCache {
		cacheTTL := time.Duration(config.CacheTTLSec) * time.Second
		cacheInstance = cache.New(cacheTTL, cacheTTL*2)
		// Hierarchy cache with longer TTL (5 minutes)
		hierarchyCacheInstance = cache.New(5*time.Minute, 10*time.Minute)
	}

	return &FrameworkDetectionManager{
		storage:              storage,
		workloadFacade:       metadataFacade,
		gpuWorkloadFacade:    gpuWorkloadFacade, // Can be nil, will use global facade
		confidenceCalculator: NewConfidenceCalculator(config),
		statusManager:        NewStatusManager(config),
		conflictResolver:     NewConflictResolver(config),
		conflictDetector:     NewConflictDetector(),
		config:               config,
		cache:                cacheInstance,
		hierarchyCache:       hierarchyCacheInstance,
		eventDispatcher:      NewEventDispatcher(),
	}
}

// ReportDetection reports a detection result from a data source
// This is the main entry point for adding detection information
// It propagates the detection to the root of the workload hierarchy
// Deprecated: Use ReportDetectionWithLayers for dual-layer framework support
func (m *FrameworkDetectionManager) ReportDetection(
	ctx context.Context,
	workloadUID string,
	source string,
	framework string,
	taskType string,
	confidence float64,
	evidence map[string]interface{},
) error {
	// Extract dual-layer framework info from evidence if exists
	var frameworkLayer, wrapperFramework, baseFramework string
	if evidence != nil {
		if layer, ok := evidence["framework_layer"].(string); ok {
			frameworkLayer = layer
		}
		if wrapper, ok := evidence["wrapper_framework"].(string); ok {
			wrapperFramework = wrapper
		}
		if base, ok := evidence["base_framework"].(string); ok {
			baseFramework = base
		}
	}

	return m.ReportDetectionWithLayers(ctx, workloadUID, source, framework, taskType, confidence, evidence, frameworkLayer, wrapperFramework, baseFramework)
}

// ReportDetectionWithLayers reports a detection result with dual-layer framework support
// This is the new entry point for adding detection information with wrapper/base framework distinction
func (m *FrameworkDetectionManager) ReportDetectionWithLayers(
	ctx context.Context,
	workloadUID string,
	source string,
	framework string,
	taskType string,
	confidence float64,
	evidence map[string]interface{},
	frameworkLayer string,
	wrapperFramework string,
	baseFramework string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	startTime := time.Now()
	defer func() {
		logrus.Debugf("ReportDetection took %v", time.Since(startTime))
	}()

	// Validate input
	if err := m.validateInput(source, framework, confidence); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}

	// Log detection report with dual-layer info
	if wrapperFramework != "" && baseFramework != "" {
		logrus.Infof("Reporting detection for workload %s: source=%s, framework=%s/%s (wrapper/base), confidence=%.2f",
			workloadUID, source, wrapperFramework, baseFramework, confidence)
	} else {
		logrus.Infof("Reporting detection for workload %s: source=%s, framework=%s, confidence=%.2f",
			workloadUID, source, framework, confidence)
	}

	// Get root workload in hierarchy
	rootUID, err := m.getRootWorkload(ctx, workloadUID)
	if err != nil {
		logrus.Warnf("Failed to get root workload for %s: %v, using current workload", workloadUID, err)
		rootUID = workloadUID
	}

	// If child workload differs from root, log it
	if rootUID != workloadUID {
		logrus.Infof("Propagating detection from child workload %s to root workload %s", workloadUID, rootUID)
	}

	// Load existing detection from root
	existing, err := m.loadDetection(ctx, rootUID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to load detection: %w", err)
	}

	// Build frameworks array: [wrapper, base] for dual-layer, [framework] for single-layer
	var frameworks []string
	if wrapperFramework != "" && baseFramework != "" {
		frameworks = []string{wrapperFramework, baseFramework}
	} else if wrapperFramework != "" {
		frameworks = []string{wrapperFramework}
	} else if baseFramework != "" {
		frameworks = []string{baseFramework}
	} else {
		frameworks = []string{framework}
	}

	// Create new source entry with dual-layer framework info
	newSource := model.DetectionSource{
		Source:           source,
		Frameworks:       frameworks,
		Type:             taskType,
		Confidence:       confidence,
		DetectedAt:       time.Now(),
		Evidence:         evidence,
		FrameworkLayer:   frameworkLayer,
		WrapperFramework: wrapperFramework,
		BaseFramework:    baseFramework,
	}

	// Merge with existing detection
	merged, err := m.MergeDetections(existing, &newSource)
	if err != nil {
		return fmt.Errorf("failed to merge detections: %w", err)
	}

	// Save to database for root workload
	if err := m.saveDetection(ctx, rootUID, merged); err != nil {
		return fmt.Errorf("failed to save detection: %w", err)
	}

	// Record metrics
	m.recordMetrics(merged)

	// Invalidate cache for both child and root
	if m.cache != nil {
		m.cache.Delete(workloadUID)
		if rootUID != workloadUID {
			m.cache.Delete(rootUID)
		}
	}

	if merged.WrapperFramework != "" && merged.BaseFramework != "" {
		logrus.Infof("Detection reported successfully: workload=%s (root=%s), frameworks=%v (wrapper/base), status=%s, confidence=%.2f",
			workloadUID, rootUID, merged.Frameworks, merged.Status, merged.Confidence)
	} else {
		logrus.Infof("Detection reported successfully: workload=%s (root=%s), frameworks=%v, status=%s, confidence=%.2f",
			workloadUID, rootUID, merged.Frameworks, merged.Status, merged.Confidence)
	}

	// Dispatch detection event
	eventType := m.determineEventType(merged, existing)
	m.dispatchDetectionEvent(ctx, eventType, workloadUID, merged)

	return nil
}

// GetDetection retrieves the current detection result for a workload
// It searches along the workload inheritance chain (from child to parent)
func (m *FrameworkDetectionManager) GetDetection(
	ctx context.Context,
	workloadUID string,
) (*model.FrameworkDetection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Get workload inheritance chain (including self)
	chain, err := m.getWorkloadHierarchyChain(ctx, workloadUID)
	if err != nil {
		logrus.Warnf("Failed to get workload hierarchy chain for %s: %v", workloadUID, err)
		// Fall back to single workload lookup
		chain = []string{workloadUID}
	}

	// Search along the hierarchy chain
	for _, uid := range chain {
		// Check cache first
		if m.cache != nil {
			if cached, found := m.cache.Get(uid); found {
				RecordCacheHit()
				logrus.Debugf("Cache hit for workload %s (searched from %s)", uid, workloadUID)
				return cached.(*model.FrameworkDetection), nil
			}
		}

		// Load from storage
		detection, err := m.storage.GetDetection(ctx, uid)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				// Try next in chain
				continue
			}
			return nil, err
		}

		if detection != nil {
			// Found detection in hierarchy
			if uid != workloadUID {
				logrus.Infof("Found detection for workload %s from parent %s", workloadUID, uid)
			}

			// Update cache for the queried workload
			if m.cache != nil {
				m.cache.Set(workloadUID, detection, cache.DefaultExpiration)
			}

			return detection, nil
		}
	}

	RecordCacheMiss()
	return nil, nil
}

// MergeDetections merges a new detection source with existing detection
// This is the core algorithm for multi-source fusion with dual-layer framework support
func (m *FrameworkDetectionManager) MergeDetections(
	existing *model.FrameworkDetection,
	newSource *model.DetectionSource,
) (*model.FrameworkDetection, error) {
	// Scenario 1: First detection
	if existing == nil {
		status := m.statusManager.DetermineStatus(
			newSource.Confidence,
			[]model.DetectionSource{*newSource},
		)

		return &model.FrameworkDetection{
			Frameworks:       newSource.Frameworks,
			Type:             newSource.Type,
			Confidence:       newSource.Confidence,
			Status:           status,
			Sources:          []model.DetectionSource{*newSource},
			Conflicts:        []model.DetectionConflict{},
			FrameworkLayer:   newSource.FrameworkLayer,
			WrapperFramework: newSource.WrapperFramework,
			BaseFramework:    newSource.BaseFramework,
			Version:          "1.0",
			UpdatedAt:        time.Now(),
		}, nil
	}

	// Scenario 2: Update existing detection

	// Check if source already exists (update case)
	sourceExists := false
	for i, src := range existing.Sources {
		if src.Source == newSource.Source {
			existing.Sources[i] = *newSource
			sourceExists = true
			logrus.Debugf("Updated existing source: %s", newSource.Source)
			break
		}
	}

	// Add new source if not exists
	if !sourceExists {
		existing.Sources = append(existing.Sources, *newSource)
		logrus.Debugf("Added new source: %s", newSource.Source)
	}

	// Detect conflicts
	conflicts := m.conflictDetector.DetectConflicts(existing.Sources)

	if len(conflicts) > 0 {
		// Handle conflicts
		logrus.Warnf("Detected %d conflicts for workload", len(conflicts))
		return m.handleConflicts(existing, conflicts)
	}

	// No conflicts: all sources agree
	existing.Frameworks = newSource.Frameworks
	existing.Type = newSource.Type
	existing.Confidence = m.confidenceCalculator.Calculate(existing.Sources)
	existing.Status = m.statusManager.DetermineStatus(existing.Confidence, existing.Sources)
	existing.UpdatedAt = time.Now()

	// Update dual-layer framework info
	existing.FrameworkLayer = newSource.FrameworkLayer
	existing.WrapperFramework = newSource.WrapperFramework
	existing.BaseFramework = newSource.BaseFramework

	if existing.WrapperFramework != "" && existing.BaseFramework != "" {
		logrus.Debugf("Merged detection: frameworks=%v (wrapper/base), confidence=%.2f, status=%s",
			existing.Frameworks, existing.Confidence, existing.Status)
	} else {
		logrus.Debugf("Merged detection: frameworks=%v, confidence=%.2f, status=%s",
			existing.Frameworks, existing.Confidence, existing.Status)
	}

	return existing, nil
}

// handleConflicts handles conflicting detection sources with dual-layer framework support
func (m *FrameworkDetectionManager) handleConflicts(
	detection *model.FrameworkDetection,
	conflicts []model.DetectionConflict,
) (*model.FrameworkDetection, error) {
	// Resolve conflict using resolver
	resolved, reason, err := m.conflictResolver.ResolveWithReason(detection.Sources)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve conflict: %w", err)
	}

	if resolved.WrapperFramework != "" && resolved.BaseFramework != "" {
		logrus.Infof("Conflict resolved: chose %v (wrapper/base) from source %s (reason: %s)",
			resolved.Frameworks, resolved.Source, reason)
	} else {
		logrus.Infof("Conflict resolved: chose %v from source %s (reason: %s)",
			resolved.Frameworks, resolved.Source, reason)
	}

	// Update detection with resolved framework (including dual-layer info)
	detection.Frameworks = resolved.Frameworks
	detection.Type = resolved.Type
	detection.Confidence = m.confidenceCalculator.Calculate(detection.Sources)
	detection.Status = model.DetectionStatusConflict
	detection.FrameworkLayer = resolved.FrameworkLayer
	detection.WrapperFramework = resolved.WrapperFramework
	detection.BaseFramework = resolved.BaseFramework

	// Update conflict records
	for i := range conflicts {
		conflicts[i].Resolution = reason
		conflicts[i].ResolvedAt = time.Now()
	}
	detection.Conflicts = append(detection.Conflicts, conflicts...)
	detection.UpdatedAt = time.Now()

	return detection, nil
}

// validateInput validates detection input parameters
func (m *FrameworkDetectionManager) validateInput(
	source, framework string,
	confidence float64,
) error {
	if source == "" {
		return fmt.Errorf("source cannot be empty")
	}
	if framework == "" {
		return fmt.Errorf("framework cannot be empty")
	}
	if confidence < 0.0 || confidence > 1.0 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got %.2f", confidence)
	}
	return nil
}

// loadDetection loads detection from storage
func (m *FrameworkDetectionManager) loadDetection(
	ctx context.Context,
	workloadUID string,
) (*model.FrameworkDetection, error) {
	return m.storage.GetDetection(ctx, workloadUID)
}

// saveDetection saves detection to storage
func (m *FrameworkDetectionManager) saveDetection(
	ctx context.Context,
	workloadUID string,
	detection *model.FrameworkDetection,
) error {
	return m.storage.UpsertDetection(ctx, workloadUID, detection)
}

// recordMetrics records detection metrics
func (m *FrameworkDetectionManager) recordMetrics(detection *model.FrameworkDetection) {
	// Record detection event
	if len(detection.Sources) > 0 {
		lastSource := detection.Sources[len(detection.Sources)-1]
		// Use first framework from array for metrics
		var primaryFramework string
		if len(detection.Frameworks) > 0 {
			primaryFramework = detection.Frameworks[0]
		}
		RecordDetection(lastSource.Source, primaryFramework, detection.Status, detection.Confidence)
	}

	// Record conflicts
	if len(detection.Conflicts) > 0 {
		for _, conflict := range detection.Conflicts {
			RecordConflict(conflict.Source1, conflict.Source2)
		}
	}

	logrus.Debugf("Metrics recorded: frameworks=%v, status=%s, confidence=%.2f, sources=%d, conflicts=%d",
		detection.Frameworks, detection.Status, detection.Confidence,
		len(detection.Sources), len(detection.Conflicts))
}

// GetConfig returns the current configuration
func (m *FrameworkDetectionManager) GetConfig() *DetectionConfig {
	return m.config
}

// GetStats returns statistics about detection operations
func (m *FrameworkDetectionManager) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"cache_enabled": m.config.EnableCache,
	}

	if m.cache != nil {
		stats["cache_items"] = m.cache.ItemCount()
	}

	return stats
}

// DetectionStatistics represents framework detection statistics information
type DetectionStatistics struct {
	TotalWorkloads    int64            `json:"total_workloads"`
	ByFramework       map[string]int64 `json:"by_framework"`
	ByStatus          map[string]int64 `json:"by_status"`
	BySource          map[string]int64 `json:"by_source"`
	AverageConfidence float64          `json:"average_confidence"`
	ConflictRate      float64          `json:"conflict_rate"`
	ReuseRate         float64          `json:"reuse_rate"`
	StartTime         string           `json:"start_time,omitempty"`
	EndTime           string           `json:"end_time,omitempty"`
	Namespace         string           `json:"namespace,omitempty"`
}

// GetStatistics retrieves detailed statistical information
func (m *FrameworkDetectionManager) GetStatistics(
	ctx context.Context,
	startTime string,
	endTime string,
	namespace string,
) (*DetectionStatistics, error) {

	logrus.Debugf("Getting statistics: startTime=%s, endTime=%s, namespace=%s",
		startTime, endTime, namespace)

	// Call storage layer statistics method
	stats, err := m.storage.GetStatistics(ctx, startTime, endTime, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	// Construct return result
	result := &DetectionStatistics{
		TotalWorkloads:    stats.TotalWorkloads,
		ByFramework:       stats.ByFramework,
		ByStatus:          stats.ByStatus,
		BySource:          stats.BySource,
		AverageConfidence: stats.AverageConfidence,
		ConflictRate:      stats.ConflictRate,
		ReuseRate:         stats.ReuseRate,
		StartTime:         startTime,
		EndTime:           endTime,
		Namespace:         namespace,
	}

	return result, nil
}

// getWorkloadHierarchyChain returns the workload hierarchy chain from child to root
// Returns: [workloadUID, parentUID, grandparentUID, ..., rootUID]
func (m *FrameworkDetectionManager) getWorkloadHierarchyChain(
	ctx context.Context,
	workloadUID string,
) ([]string, error) {
	chain := []string{workloadUID}
	visited := make(map[string]bool)
	visited[workloadUID] = true

	currentUID := workloadUID
	maxDepth := 10 // Prevent infinite loops

	for i := 0; i < maxDepth; i++ {
		parentUID, err := m.getParentWorkload(ctx, currentUID)
		if err != nil {
			return chain, err
		}

		// No parent found, reached root
		if parentUID == "" {
			break
		}

		// Check for cycles
		if visited[parentUID] {
			logrus.Warnf("Detected cycle in workload hierarchy at %s", parentUID)
			break
		}

		chain = append(chain, parentUID)
		visited[parentUID] = true
		currentUID = parentUID
	}

	return chain, nil
}

// getRootWorkload returns the root workload UID in the hierarchy
func (m *FrameworkDetectionManager) getRootWorkload(
	ctx context.Context,
	workloadUID string,
) (string, error) {
	chain, err := m.getWorkloadHierarchyChain(ctx, workloadUID)
	if err != nil {
		return workloadUID, err
	}

	// Return the last element (root)
	if len(chain) > 0 {
		return chain[len(chain)-1], nil
	}

	return workloadUID, nil
}

// getParentWorkload returns the parent workload UID, or empty string if no parent
func (m *FrameworkDetectionManager) getParentWorkload(
	ctx context.Context,
	workloadUID string,
) (string, error) {
	// Check hierarchy cache first
	if m.hierarchyCache != nil {
		if cached, found := m.hierarchyCache.Get(workloadUID); found {
			return cached.(string), nil
		}
	}

	// Query workload from database using injected facade or global facade
	var workload *dbModel.GpuWorkload
	var err error
	if m.gpuWorkloadFacade != nil {
		workload, err = m.gpuWorkloadFacade.GetGpuWorkloadByUid(ctx, workloadUID)
	} else {
		workload, err = database.GetFacade().GetWorkload().GetGpuWorkloadByUid(ctx, workloadUID)
	}
	if err != nil {
		return "", err
	}

	if workload == nil {
		return "", nil
	}

	parentUID := workload.ParentUID

	// Update hierarchy cache
	if m.hierarchyCache != nil {
		m.hierarchyCache.Set(workloadUID, parentUID, cache.DefaultExpiration)
	}

	return parentUID, nil
}

// RegisterListener registers a detection event listener
func (m *FrameworkDetectionManager) RegisterListener(listener DetectionEventListener) {
	m.eventDispatcher.RegisterListener(listener)
}

// UnregisterListener removes a detection event listener
func (m *FrameworkDetectionManager) UnregisterListener(listener DetectionEventListener) {
	m.eventDispatcher.UnregisterListener(listener)
}

// GetListenerCount returns the number of registered event listeners
func (m *FrameworkDetectionManager) GetListenerCount() int {
	return m.eventDispatcher.GetListenerCount()
}

// dispatchDetectionEvent dispatches a detection event to all registered listeners
func (m *FrameworkDetectionManager) dispatchDetectionEvent(
	ctx context.Context,
	eventType DetectionEventType,
	workloadUID string,
	detection *model.FrameworkDetection,
) {
	event := &DetectionEvent{
		Type:        eventType,
		WorkloadUID: workloadUID,
		Detection:   detection,
	}

	m.eventDispatcher.Dispatch(ctx, event)
}

// determineEventType determines the appropriate event type based on detection state
func (m *FrameworkDetectionManager) determineEventType(
	merged *model.FrameworkDetection,
	existing *model.FrameworkDetection,
) DetectionEventType {
	// New detection
	if existing == nil {
		return DetectionEventTypeCompleted
	}

	// Conflict detected
	if merged.Status == model.DetectionStatusConflict {
		return DetectionEventTypeConflict
	}

	// Updated detection
	return DetectionEventTypeUpdated
}
