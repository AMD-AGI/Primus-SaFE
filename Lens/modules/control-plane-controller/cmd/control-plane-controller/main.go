// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/control-plane-controller/pkg/jobs"
	"github.com/gin-gonic/gin"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Info("Received shutdown signal, stopping control-plane-controller...")
		cancel()
	}()

	if err := run(ctx); err != nil {
		log.Errorf("Control plane controller failed: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Initialize OpenTelemetry tracer
	if err := trace.InitTracer("control-plane-controller"); err != nil {
		log.Warnf("Failed to init OpenTelemetry tracer: %v", err)
	} else {
		log.Info("OpenTelemetry tracer initialized successfully")
		defer func() {
			if err := trace.CloseTracer(); err != nil {
				log.Errorf("Failed to close tracer: %v", err)
			}
		}()
	}

	// Determine if we need multi-cluster storage
	// Multi-cluster jobs (tracelens_cleanup, gpu_usage_weekly_report) need storage access
	// Pure CP jobs (dataplane_installer, multi_cluster_config_sync) do not
	//
	// If LoadStorageClient is explicitly configured, respect that setting.
	// Otherwise, default to false (pure CP mode) for safety.
	requireStorage := cfg.LoadStorageClient
	if requireStorage {
		log.Info("Storage client loading enabled - multi-cluster jobs will have full functionality")
	} else {
		log.Info("Storage client loading disabled - multi-cluster jobs will run with limited functionality")
	}

	// Initialize cluster manager with control plane declaration
	decl := clientsets.ComponentDeclaration{
		Type:           clientsets.ComponentTypeControlPlane,
		RequireK8S:     true,
		RequireStorage: requireStorage,
	}

	log.Infof("Initializing as ControlPlane component (storage: %v)", requireStorage)
	if err := clientsets.InitClusterManager(ctx, decl); err != nil {
		return fmt.Errorf("failed to initialize cluster manager: %w", err)
	}

	// Initialize control plane client (connects to CP database via pguser secret)
	log.Info("Initializing control plane database connection...")
	if err := clientsets.InitControlPlaneClient(ctx, cfg); err != nil {
		return fmt.Errorf("failed to initialize control plane client: %w", err)
	}
	log.Info("Control plane database initialized successfully")

	// Start jobs with configuration
	if err := jobs.Start(ctx, cfg.Jobs); err != nil {
		return fmt.Errorf("failed to start jobs: %w", err)
	}

	// Start health server
	go startHealthServer(cfg.HttpPort)

	// Wait for context cancellation
	<-ctx.Done()
	log.Info("Control plane controller stopped")
	return nil
}

func startHealthServer(port int) {
	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(gin.Recovery())

	engine.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "healthy",
			"component": "control-plane-controller",
		})
	})

	engine.GET("/ready", func(c *gin.Context) {
		// Check if control plane client is ready
		cpClient := clientsets.GetControlPlaneClientSet()
		if cpClient == nil || cpClient.Facade == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "not ready",
				"reason": "control plane database not initialized",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "ready",
		})
	})

	// Metrics endpoint placeholder
	engine.GET("/metrics", func(c *gin.Context) {
		// TODO: Integrate with Prometheus handler
		c.String(http.StatusOK, "# Metrics endpoint")
	})

	addr := fmt.Sprintf(":%d", port)
	log.Infof("Starting health server on %s", addr)
	if err := engine.Run(addr); err != nil {
		log.Errorf("Health server failed: %v", err)
	}
}
