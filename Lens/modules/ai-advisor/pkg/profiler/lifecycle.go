// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package profiler

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// LifecycleManager manages profiler file lifecycle
type LifecycleManager struct {
	metadataMgr    *MetadataManager
	storageBackend storage.StorageBackend
	config         *LifecycleConfig
}

// LifecycleConfig lifecycle configuration
type LifecycleConfig struct {
	// Default retention days
	DefaultRetentionDays int

	// Retention policy by file type
	RetentionByType map[string]int

	// Retention policy by workload
	RetentionByWorkload map[string]int

	// Storage space threshold (percentage)
	StorageThreshold float64

	// Enable safe delete (mark then delete)
	SafeDelete bool

	// Safe delete wait days
	SafeDeleteWaitDays int
}

// DefaultLifecycleConfig returns default configuration
func DefaultLifecycleConfig() *LifecycleConfig {
	return &LifecycleConfig{
		DefaultRetentionDays: 30,
		RetentionByType: map[string]int{
			"chrome_trace":  30,
			"stack_trace":   60, // stack trace kept longer
			"memory_dump":   7,  // memory dump kept shorter (large files)
			"kineto":        30,
		},
		StorageThreshold:   0.9, // 90%
		SafeDelete:         true,
		SafeDeleteWaitDays: 1,
	}
}

// NewLifecycleManager creates lifecycle manager
func NewLifecycleManager(
	metadataMgr *MetadataManager,
	storageBackend storage.StorageBackend,
	config *LifecycleConfig,
) *LifecycleManager {
	if config == nil {
		config = DefaultLifecycleConfig()
	}

	return &LifecycleManager{
		metadataMgr:    metadataMgr,
		storageBackend: storageBackend,
		config:         config,
	}
}

// CleanupResult cleanup operation result
type CleanupResult struct {
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	TotalScanned int
	DeletedCount int
	FreedSpace   int64
	Errors       []string
}

// CleanupExpiredFiles cleans up expired files
func (m *LifecycleManager) CleanupExpiredFiles(ctx context.Context) (*CleanupResult, error) {
	log.Info("Starting expired files cleanup")

	result := &CleanupResult{
		StartTime:    time.Now(),
		TotalScanned: 0,
		DeletedCount: 0,
		FreedSpace:   0,
		Errors:       []string{},
	}

	// 1. Query all files
	// TODO: Implement ListAllFiles in MetadataManager
	files := []*ProfilerFileMetadata{} // Placeholder
	// files, err := m.metadataMgr.ListAllFiles(ctx)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to list files: %w", err)
	// }

	result.TotalScanned = len(files)
	log.Infof("Found %d files to check", len(files))

	// 2. Check each file if expired
	for _, file := range files {
		expired := m.isFileExpired(file)
		if !expired {
			continue
		}

		// 3. Delete file
		if err := m.deleteFile(ctx, file); err != nil {
			log.Errorf("Failed to delete file %d: %v", file.ID, err)
			result.Errors = append(result.Errors, fmt.Sprintf("file %d: %v", file.ID, err))
			continue
		}

		result.DeletedCount++
		result.FreedSpace += file.FileSize
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	log.Infof("Cleanup completed: deleted %d files, freed %d bytes in %v",
		result.DeletedCount, result.FreedSpace, result.Duration)

	// Record metrics
	RecordCleanup(result.DeletedCount, result.FreedSpace, result.Duration.Seconds())

	return result, nil
}

// CleanupByStorageUsage cleans up based on storage usage
func (m *LifecycleManager) CleanupByStorageUsage(
	ctx context.Context,
	currentUsage float64,
) (*CleanupResult, error) {
	if currentUsage < m.config.StorageThreshold {
		log.Debugf("Storage usage %.2f%% below threshold %.2f%%, skipping cleanup",
			currentUsage*100, m.config.StorageThreshold*100)
		return &CleanupResult{}, nil
	}

	log.Warnf("Storage usage %.2f%% exceeds threshold %.2f%%, starting aggressive cleanup",
		currentUsage*100, m.config.StorageThreshold*100)

	// Query files ordered by creation time (oldest first)
	// TODO: Implement ListFilesByAge in MetadataManager
	files := []*ProfilerFileMetadata{} // Placeholder
	// files, err := m.metadataMgr.ListFilesByAge(ctx, 1000)
	// if err != nil {
	// 	return nil, err
	// }

	result := &CleanupResult{
		StartTime:    time.Now(),
		TotalScanned: len(files),
	}

	// Calculate space to free (20% of total)
	targetFreedSpace := int64(float64(m.getTotalStorageSize(files)) * 0.2)

	for _, file := range files {
		if result.FreedSpace >= targetFreedSpace {
			break
		}

		// Delete file
		if err := m.deleteFile(ctx, file); err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}

		result.DeletedCount++
		result.FreedSpace += file.FileSize
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Record metrics
	RecordCleanup(result.DeletedCount, result.FreedSpace, result.Duration.Seconds())

	return result, nil
}

// MarkForDeletion marks file for deletion (safe delete)
func (m *LifecycleManager) MarkForDeletion(
	ctx context.Context,
	fileID int64,
) error {
	// TODO: Implement UpdateFileMetadata in MetadataManager
	_ = fileID
	return fmt.Errorf("not implemented yet")
	// return m.metadataMgr.UpdateFileMetadata(ctx, fileID, map[string]interface{}{
	// 	"marked_for_deletion": true,
	// 	"mark_deletion_at":    time.Now(),
	// })
}

// DeleteMarkedFiles deletes files marked for deletion
func (m *LifecycleManager) DeleteMarkedFiles(ctx context.Context) (*CleanupResult, error) {
	waitDuration := time.Duration(m.config.SafeDeleteWaitDays) * 24 * time.Hour
	cutoffTime := time.Now().Add(-waitDuration)
	_ = cutoffTime // Suppress unused variable warning

	// TODO: Implement ListMarkedForDeletion in MetadataManager
	files := []*ProfilerFileMetadata{} // Placeholder
	// files, err := m.metadataMgr.ListMarkedForDeletion(ctx, cutoffTime)
	// if err != nil {
	// 	return nil, err
	// }

	result := &CleanupResult{
		StartTime:    time.Now(),
		TotalScanned: len(files),
	}

	for _, file := range files {
		if err := m.deleteFile(ctx, file); err != nil {
			result.Errors = append(result.Errors, err.Error())
			continue
		}

		result.DeletedCount++
		result.FreedSpace += file.FileSize
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Record metrics
	RecordCleanup(result.DeletedCount, result.FreedSpace, result.Duration.Seconds())

	return result, nil
}

// isFileExpired checks if file is expired
func (m *LifecycleManager) isFileExpired(file *ProfilerFileMetadata) bool {
	// 1. Check workload-specific retention policy
	if retentionDays, ok := m.config.RetentionByWorkload[file.WorkloadUID]; ok {
		expiresAt := file.CollectedAt.AddDate(0, 0, retentionDays)
		return time.Now().After(expiresAt)
	}

	// 2. Check file type-specific retention policy
	if retentionDays, ok := m.config.RetentionByType[file.FileType]; ok {
		expiresAt := file.CollectedAt.AddDate(0, 0, retentionDays)
		return time.Now().After(expiresAt)
	}

	// 3. Use default retention policy
	expiresAt := file.CollectedAt.AddDate(0, 0, m.config.DefaultRetentionDays)
	return time.Now().After(expiresAt)
}

// deleteFile deletes file from storage and metadata
func (m *LifecycleManager) deleteFile(ctx context.Context, file *ProfilerFileMetadata) error {
	// 1. Delete from storage backend
	if err := m.storageBackend.Delete(ctx, file.StoragePath); err != nil {
		return fmt.Errorf("failed to delete from storage: %w", err)
	}

	// 2. Delete metadata from database
	// TODO: Implement DeleteFile in MetadataManager
	_ = file.ID // Suppress unused variable warning
	// if err := m.metadataMgr.DeleteFile(ctx, file.ID); err != nil {
	// 	return fmt.Errorf("failed to delete metadata: %w", err)
	// }

	log.Infof("Deleted file %d (%s, %d bytes)", file.ID, file.FileName, file.FileSize)

	return nil
}

// getTotalStorageSize calculates total storage size
func (m *LifecycleManager) getTotalStorageSize(files []*ProfilerFileMetadata) int64 {
	var total int64
	for _, file := range files {
		total += file.FileSize
	}
	return total
}

