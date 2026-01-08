// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy

import (
	"context"
	"sync"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

var (
	instance *PySpyCollector
	once     sync.Once
)

// PySpyCollector manages py-spy profiling operations
type PySpyCollector struct {
	config    *config.PySpyConfig
	executor  *Executor
	fileStore *FileStore
	detector  *Detector
}

// InitCollector initializes the py-spy collector
func InitCollector(ctx context.Context, cfg *config.PySpyConfig) error {
	var initErr error

	once.Do(func() {
		// Create file store
		fileStore, err := NewFileStore(cfg)
		if err != nil {
			initErr = err
			return
		}

		// Create executor
		executor := NewExecutor(cfg, fileStore)

		// Check if py-spy is available
		if err := executor.CheckPySpyAvailable(); err != nil {
			log.Warnf("py-spy not available: %v", err)
			// Don't fail initialization, just warn
		}

		// Create detector
		detector := NewDetector(cfg)

		instance = &PySpyCollector{
			config:    cfg,
			executor:  executor,
			fileStore: fileStore,
			detector:  detector,
		}

		// Start background cleanup routine
		go fileStore.StartCleanupRoutine(ctx)

		log.Infof("py-spy collector initialized (storage: %s)", cfg.StoragePath)
	})

	return initErr
}

// GetCollector returns the global py-spy collector instance
func GetCollector() *PySpyCollector {
	return instance
}

// Execute runs py-spy profiling
func (c *PySpyCollector) Execute(ctx context.Context, req *ExecuteRequest) (*ExecuteResponse, error) {
	return c.executor.Execute(ctx, req)
}

// Check checks py-spy compatibility
func (c *PySpyCollector) Check(ctx context.Context, req *CheckRequest) (*CheckResponse, error) {
	return c.detector.Check(ctx, req)
}

// ListFiles lists profiling files
func (c *PySpyCollector) ListFiles(req *FileListRequest) *FileListResponse {
	return c.fileStore.ListFiles(req)
}

// GetFile gets a file by task ID
func (c *PySpyCollector) GetFile(taskID string) (*FileInfo, bool) {
	return c.fileStore.GetFile(taskID)
}

// ReadFile reads a file's content
func (c *PySpyCollector) ReadFile(taskID, fileName string) (interface{}, *FileInfo, error) {
	return c.fileStore.ReadFile(taskID, fileName)
}

// DeleteFile deletes a file
func (c *PySpyCollector) DeleteFile(taskID string) error {
	return c.fileStore.DeleteFile(taskID)
}

// CancelTask cancels a running task
func (c *PySpyCollector) CancelTask(taskID string) bool {
	return c.executor.CancelTask(taskID)
}

// IsTaskRunning checks if a task is running
func (c *PySpyCollector) IsTaskRunning(taskID string) bool {
	return c.executor.IsTaskRunning(taskID)
}

// GetStorageStats returns storage statistics
func (c *PySpyCollector) GetStorageStats() (fileCount int, totalSize int64) {
	return c.fileStore.GetStorageStats()
}

// IsEnabled returns whether py-spy is enabled
func (c *PySpyCollector) IsEnabled() bool {
	return c.config.Enabled
}

