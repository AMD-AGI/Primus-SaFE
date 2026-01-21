// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"os"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	coreconfig "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/storage-exporter/pkg/exporter"
)

var (
	ctrl *controller.Controller
	exp  *exporter.Exporter
)

// Init initializes the storage exporter
func Init(ctx context.Context, conf *coreconfig.Config) error {
	// Load storage exporter specific config
	storageConf, err := config.LoadStorageExporterConfig()
	if err != nil {
		log.Warnf("Failed to load storage exporter config, using defaults: %v", err)
		storageConf = &config.StorageExporterConfig{
			Storage: config.StorageConfig{
				ScrapeInterval: "60s",
			},
		}
	}

	// Get namespace from env or config
	namespace := os.Getenv("POD_NAMESPACE")
	if namespace == "" {
		namespace = storageConf.Controller.Namespace
	}
	if namespace == "" {
		namespace = "primus-lens"
	}

	log.Infof("Storage exporter namespace: %s", namespace)

	// Get Kubernetes client
	k8sClient := clientsets.GetClusterManager().GetCurrentClusterClients().K8SClientSet.Clientsets

	// Initialize controller
	ctrl = controller.NewController(k8sClient, namespace, storageConf)

	// Initialize exporter
	exp = exporter.NewExporter(ctrl, storageConf)

	// Register metrics handler
	exp.Register()

	// Start controller
	if err := ctrl.Start(ctx); err != nil {
		return err
	}

	// Start metrics update loop
	go exp.StartMetricsUpdateLoop(ctx, storageConf.Storage.GetScrapeInterval())

	// Set custom gatherer for metrics endpoint
	server.SetDefaultGather(exp)

	log.Info("Storage exporter initialized successfully")
	return nil
}
