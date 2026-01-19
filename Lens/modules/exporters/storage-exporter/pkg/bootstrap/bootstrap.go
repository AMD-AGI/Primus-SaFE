// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"

	coreconfig "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/collector"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/exporter"
)

var (
	exp *exporter.Exporter
)

// Init initializes the storage exporter
func Init(ctx context.Context, conf *coreconfig.Config) error {
	// Load storage exporter specific config
	storageConf, err := config.LoadStorageExporterConfig()
	if err != nil {
		log.Warnf("Failed to load storage exporter config, using defaults: %v", err)
		storageConf = &config.StorageExporterConfig{}
	}

	// Log configured mounts
	log.Infof("Configured storage mounts: %d", len(storageConf.Storage.Mounts))
	for _, m := range storageConf.Storage.Mounts {
		log.Infof("  - %s: %s (%s/%s)", m.Name, m.MountPath, m.StorageType, m.FilesystemName)
	}

	// Initialize collector
	coll := collector.NewCollector(storageConf.Storage.Mounts)

	// Initialize exporter
	exp = exporter.NewExporter(coll, storageConf)

	// Register metrics handler
	exp.Register()

	// Start collection loop
	go exp.StartCollectionLoop(ctx, storageConf.Storage.GetScrapeInterval())

	// Set custom gatherer for metrics endpoint
	server.SetDefaultGather(exp)

	log.Info("Storage exporter initialized successfully")
	return nil
}
