// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	metadataCollector "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/profiler"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
)

// InitProfilerServices initializes profiler-related services
// This is a simplified version for Phase 3 implementation
func InitProfilerServices(
	ctx context.Context,
	scheduler *task.TaskScheduler,
	metaCollector *metadataCollector.Collector,
) error {
	log.Info("Initializing Profiler services...")

	// 1. Get database connection
	gormDB := database.GetFacade().GetSystemConfig().GetDB()
	if gormDB == nil {
		return fmt.Errorf("failed to get database connection")
	}

	// Get underlying sql.DB from gorm.DB
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB from gorm.DB: %w", err)
	}

	// 2. Create storage backend (using database storage for simplicity)
	// TODO: Make this configurable
	storageBackend, err := storage.NewDatabaseStorageBackend(sqlDB, &storage.DatabaseConfig{
		Compression:         true,
		ChunkSize:           10 * 1024 * 1024,  // 10MB
		MaxFileSize:         200 * 1024 * 1024, // 200MB
		MaxConcurrentChunks: 5,
	})
	if err != nil {
		return fmt.Errorf("failed to create storage backend: %w", err)
	}

	// 3. Create metadata manager
	metadataMgr, err := profiler.NewMetadataManager(sqlDB)
	if err != nil {
		return fmt.Errorf("failed to create metadata manager: %w", err)
	}

	// 4. Create collector
	collector, err := profiler.NewCollector(&profiler.CollectorConfig{
		AutoCollect: true,
	}, storageBackend, "http://node-exporter:8080")
	if err != nil {
		return fmt.Errorf("failed to create collector: %w", err)
	}

	// 5. Create lifecycle manager
	lifecycleMgr := profiler.NewLifecycleManager(
		metadataMgr,
		storageBackend,
		profiler.DefaultLifecycleConfig(),
	)

	// 6. Create task executor with metadata collector for node-exporter client access
	executor := profiler.NewProfilerCollectionExecutor(
		collector,
		metadataMgr,
		metaCollector,
	)

	// 7. Register executor to scheduler
	if err := scheduler.RegisterExecutor(executor); err != nil {
		return fmt.Errorf("failed to register profiler executor: %w", err)
	}
	log.Info("Registered ProfilerCollectionExecutor to task scheduler")

	// 8. Start cleanup job (run every 24 hours)
	cleanupJob := profiler.NewProfilerCleanupJob(
		lifecycleMgr,
		"0 2 * * *", // Placeholder schedule string
	)
	if err := cleanupJob.Start(ctx); err != nil {
		return fmt.Errorf("failed to start cleanup job: %w", err)
	}
	log.Info("Started profiler cleanup job")

	// Register stop callback
	go func() {
		<-ctx.Done()
		cleanupJob.Stop()
	}()

	// 9. Start storage metrics updater
	go startStorageMetricsUpdater(ctx, metadataMgr)

	log.Info("Profiler services initialized successfully")

	// Note: Profiler task creation is handled by detection/task_creator.go
	// When detection completes and identifies a PyTorch training workload,
	// a profiler_collection task will be created automatically

	return nil
}

// startStorageMetricsUpdater starts storage metrics updater
func startStorageMetricsUpdater(ctx context.Context, metadataMgr *profiler.MetadataManager) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updateStorageMetrics(ctx, metadataMgr)
		}
	}
}

// updateStorageMetrics updates storage metrics
func updateStorageMetrics(ctx context.Context, metadataMgr *profiler.MetadataManager) {
	// TODO: Implement GetStorageStats in MetadataManager
	// stats, err := metadataMgr.GetStorageStats(ctx)
	// if err != nil {
	// 	log.Errorf("Failed to get storage stats: %v", err)
	// 	return
	// }
	//
	// for storageType, stat := range stats {
	// 	profiler.UpdateStorageMetrics(storageType, stat.TotalSize, stat.FileCount)
	// }
	_ = metadataMgr // Suppress unused variable warning
}
