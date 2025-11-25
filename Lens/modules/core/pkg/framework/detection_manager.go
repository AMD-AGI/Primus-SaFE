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
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// FrameworkDetectionManager manages framework detection from multiple sources
type FrameworkDetectionManager struct {
	mu sync.RWMutex
	
	// Storage layer
	storage *database.FrameworkDetectionStorage
	
	// Detection components
	confidenceCalculator *ConfidenceCalculator
	statusManager        *StatusManager
	conflictResolver     *ConflictResolver
	conflictDetector     *ConflictDetector
	
	// Configuration
	config *DetectionConfig
	
	// Cache (optional)
	cache *cache.Cache
}

// NewFrameworkDetectionManager creates a new framework detection manager
func NewFrameworkDetectionManager(
	metadataFacade database.AiWorkloadMetadataFacadeInterface,
	config *DetectionConfig,
) *FrameworkDetectionManager {
	if config == nil {
		config = DefaultDetectionConfig()
	}
	
	storage := database.NewFrameworkDetectionStorage(metadataFacade)
	
	var cacheInstance *cache.Cache
	if config.EnableCache {
		cacheTTL := time.Duration(config.CacheTTLSec) * time.Second
		cacheInstance = cache.New(cacheTTL, cacheTTL*2)
	}
	
	return &FrameworkDetectionManager{
		storage:              storage,
		confidenceCalculator: NewConfidenceCalculator(config),
		statusManager:        NewStatusManager(config),
		conflictResolver:     NewConflictResolver(config),
		conflictDetector:     NewConflictDetector(),
		config:               config,
		cache:                cacheInstance,
	}
}

// ReportDetection reports a detection result from a data source
// This is the main entry point for adding detection information
func (m *FrameworkDetectionManager) ReportDetection(
	ctx context.Context,
	workloadUID string,
	source string,
	framework string,
	taskType string,
	confidence float64,
	evidence map[string]interface{},
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
	
	logrus.Infof("Reporting detection for workload %s: source=%s, framework=%s, confidence=%.2f",
		workloadUID, source, framework, confidence)
	
	// Load existing detection
	existing, err := m.loadDetection(ctx, workloadUID)
	if err != nil && err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to load detection: %w", err)
	}
	
	// Create new source entry
	newSource := model.DetectionSource{
		Source:     source,
		Framework:  framework,
		Type:       taskType,
		Confidence: confidence,
		DetectedAt: time.Now(),
		Evidence:   evidence,
	}
	
	// Merge with existing detection
	merged, err := m.MergeDetections(existing, &newSource)
	if err != nil {
		return fmt.Errorf("failed to merge detections: %w", err)
	}
	
	// Save to database
	if err := m.saveDetection(ctx, workloadUID, merged); err != nil {
		return fmt.Errorf("failed to save detection: %w", err)
	}
	
	// Record metrics
	m.recordMetrics(merged)
	
	// Invalidate cache
	if m.cache != nil {
		m.cache.Delete(workloadUID)
	}
	
	logrus.Infof("Detection reported successfully: workload=%s, framework=%s, status=%s, confidence=%.2f",
		workloadUID, merged.Framework, merged.Status, merged.Confidence)
	
	return nil
}

// GetDetection retrieves the current detection result for a workload
func (m *FrameworkDetectionManager) GetDetection(
	ctx context.Context,
	workloadUID string,
) (*model.FrameworkDetection, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	// Check cache first
	if m.cache != nil {
		if cached, found := m.cache.Get(workloadUID); found {
			RecordCacheHit()
			logrus.Debugf("Cache hit for workload %s", workloadUID)
			return cached.(*model.FrameworkDetection), nil
		}
		RecordCacheMiss()
	}
	
	// Load from storage
	detection, err := m.storage.GetDetection(ctx, workloadUID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	
	// Update cache
	if m.cache != nil && detection != nil {
		m.cache.Set(workloadUID, detection, cache.DefaultExpiration)
	}
	
	return detection, nil
}

// MergeDetections merges a new detection source with existing detection
// This is the core algorithm for multi-source fusion
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
			Framework:  newSource.Framework,
			Type:       newSource.Type,
			Confidence: newSource.Confidence,
			Status:     status,
			Sources:    []model.DetectionSource{*newSource},
			Conflicts:  []model.DetectionConflict{},
			Version:    "1.0",
			UpdatedAt:  time.Now(),
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
	existing.Framework = newSource.Framework
	existing.Type = newSource.Type
	existing.Confidence = m.confidenceCalculator.Calculate(existing.Sources)
	existing.Status = m.statusManager.DetermineStatus(existing.Confidence, existing.Sources)
	existing.UpdatedAt = time.Now()
	
	logrus.Debugf("Merged detection: framework=%s, confidence=%.2f, status=%s",
		existing.Framework, existing.Confidence, existing.Status)
	
	return existing, nil
}

// handleConflicts handles conflicting detection sources
func (m *FrameworkDetectionManager) handleConflicts(
	detection *model.FrameworkDetection,
	conflicts []model.DetectionConflict,
) (*model.FrameworkDetection, error) {
	// Resolve conflict using resolver
	resolved, reason, err := m.conflictResolver.ResolveWithReason(detection.Sources)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve conflict: %w", err)
	}
	
	logrus.Infof("Conflict resolved: chose %s from source %s (reason: %s)",
		resolved.Framework, resolved.Source, reason)
	
	// Update detection with resolved framework
	detection.Framework = resolved.Framework
	detection.Type = resolved.Type
	detection.Confidence = m.confidenceCalculator.Calculate(detection.Sources)
	detection.Status = model.DetectionStatusConflict
	
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
		RecordDetection(lastSource.Source, detection.Framework, detection.Status, detection.Confidence)
	}
	
	// Record conflicts
	if len(detection.Conflicts) > 0 {
		for _, conflict := range detection.Conflicts {
			RecordConflict(conflict.Source1, conflict.Source2)
		}
	}
	
	logrus.Debugf("Metrics recorded: framework=%s, status=%s, confidence=%.2f, sources=%d, conflicts=%d",
		detection.Framework, detection.Status, detection.Confidence,
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

// DetectionStatistics 框架检测统计信息
type DetectionStatistics struct {
	TotalWorkloads     int64                      `json:"total_workloads"`
	ByFramework        map[string]int64           `json:"by_framework"`
	ByStatus           map[string]int64           `json:"by_status"`
	BySource           map[string]int64           `json:"by_source"`
	AverageConfidence  float64                    `json:"average_confidence"`
	ConflictRate       float64                    `json:"conflict_rate"`
	ReuseRate          float64                    `json:"reuse_rate"`
	StartTime          string                     `json:"start_time,omitempty"`
	EndTime            string                     `json:"end_time,omitempty"`
	Namespace          string                     `json:"namespace,omitempty"`
}

// GetStatistics 获取详细的统计信息
func (m *FrameworkDetectionManager) GetStatistics(
	ctx context.Context,
	startTime string,
	endTime string,
	namespace string,
) (*DetectionStatistics, error) {
	
	logrus.Debugf("Getting statistics: startTime=%s, endTime=%s, namespace=%s",
		startTime, endTime, namespace)
	
	// 调用 storage 层的统计方法
	stats, err := m.storage.GetStatistics(ctx, startTime, endTime, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}
	
	// 构造返回结果
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

