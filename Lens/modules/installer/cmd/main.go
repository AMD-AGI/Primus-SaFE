// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/installer/pkg/installer"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func main() {
	log.Info("=== Primus-Lens Dataplane Installer ===")

	// Load configuration from environment
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Infof("Task ID: %d, Cluster: %s", cfg.TaskID, cfg.ClusterName)

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Warnf("Received signal %v, initiating graceful shutdown...", sig)
		cancel()
	}()

	// Connect to Control Plane database
	log.Info("Connecting to Control Plane database...")
	db, err := connectDB(cfg.GetCPDBDSN())
	if err != nil {
		log.Fatalf("Failed to connect to Control Plane DB: %v", err)
	}
	log.Info("Connected to Control Plane database")

	// Create facade
	facade := cpdb.NewControlPlaneFacade(db)
	taskFacade := facade.GetDataplaneInstallTask()
	clusterFacade := facade.GetClusterConfig()

	// Get task from database
	log.Infof("Loading task %d from database...", cfg.TaskID)
	task, err := taskFacade.GetByID(ctx, cfg.TaskID)
	if err != nil {
		log.Fatalf("Failed to get task: %v", err)
	}

	// Verify task is for the expected cluster
	if task.ClusterName != cfg.ClusterName {
		log.Fatalf("Task cluster mismatch: expected %s, got %s", cfg.ClusterName, task.ClusterName)
	}

	// Check task status - allow resuming running tasks
	if task.Status != model.TaskStatusPending && task.Status != model.TaskStatusRunning {
		log.Fatalf("Task is not in pending/running status: %s", task.Status)
	}

	// Get cluster config
	log.Infof("Loading cluster config for %s...", task.ClusterName)
	clusterConfig, err := clusterFacade.GetByName(ctx, task.ClusterName)
	if err != nil {
		log.Fatalf("Failed to get cluster config: %v", err)
	}

	// Mark task as running
	log.Info("Marking task as running...")
	now := time.Now()
	if task.Status == model.TaskStatusPending {
		if err := taskFacade.MarkRunning(ctx, task.ID); err != nil {
			log.Fatalf("Failed to mark task as running: %v", err)
		}
		task.StartedAt = &now
	}

	// Update cluster status to deploying
	if err := clusterFacade.UpdateDataplaneStatus(ctx, task.ClusterName, model.DataplaneStatusDeploying, "Installation started"); err != nil {
		log.Warnf("Failed to update cluster status: %v", err)
	}

	// Create installer and execute
	log.Info("Starting dataplane installation...")
	dpInstaller := installer.NewDataplaneInstaller(facade)

	installErr := dpInstaller.Execute(ctx, task, clusterConfig)

	if installErr != nil {
		log.Errorf("Installation failed: %v", installErr)

		// Mark task as failed
		if err := taskFacade.MarkFailed(ctx, task.ID, installErr.Error()); err != nil {
			log.Errorf("Failed to mark task as failed: %v", err)
		}

		// Update cluster status based on install scope
		updateClusterStatusOnFailure(ctx, clusterFacade, task, installErr.Error())

		os.Exit(1)
	}

	// Mark task as completed
	log.Info("Installation completed successfully!")
	if err := taskFacade.MarkCompleted(ctx, task.ID); err != nil {
		log.Errorf("Failed to mark task as completed: %v", err)
	}

	// Update cluster status based on install scope
	updateClusterStatusOnSuccess(ctx, clusterFacade, task)

	log.Info("=== Dataplane Installer Completed ===")
}

// connectDB connects to PostgreSQL database
func connectDB(dsn string) (*gorm.DB, error) {
	gormConfig := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetMaxOpenConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	return db, nil
}

// updateClusterStatusOnSuccess updates cluster status when installation succeeds
func updateClusterStatusOnSuccess(ctx context.Context, clusterFacade cpdb.ClusterConfigFacadeInterface, task *model.DataplaneInstallTask) {
	switch task.InstallScope {
	case model.InstallScopeInfrastructure:
		// Infrastructure only - update infrastructure status
		if err := clusterFacade.UpdateInfrastructureStatus(ctx, task.ClusterName, model.InfrastructureStatusReady, ""); err != nil {
			log.Errorf("Failed to update infrastructure status for cluster %s: %v", task.ClusterName, err)
		} else {
			log.Infof("Updated infrastructure status to 'ready' for cluster %s", task.ClusterName)
		}
	case model.InstallScopeApps:
		// Apps only - update dataplane status
		if err := clusterFacade.UpdateDataplaneStatus(ctx, task.ClusterName, model.DataplaneStatusDeployed, ""); err != nil {
			log.Errorf("Failed to update dataplane status for cluster %s: %v", task.ClusterName, err)
		} else {
			log.Infof("Updated dataplane status to 'deployed' for cluster %s", task.ClusterName)
		}
	case model.InstallScopeFull, "":
		// Full installation - update both statuses
		if err := clusterFacade.UpdateInfrastructureStatus(ctx, task.ClusterName, model.InfrastructureStatusReady, ""); err != nil {
			log.Errorf("Failed to update infrastructure status for cluster %s: %v", task.ClusterName, err)
		}
		if err := clusterFacade.UpdateDataplaneStatus(ctx, task.ClusterName, model.DataplaneStatusDeployed, ""); err != nil {
			log.Errorf("Failed to update dataplane status for cluster %s: %v", task.ClusterName, err)
		}
		log.Infof("Updated infrastructure and dataplane status for cluster %s", task.ClusterName)
	}
}

// updateClusterStatusOnFailure updates cluster status when installation fails
func updateClusterStatusOnFailure(ctx context.Context, clusterFacade cpdb.ClusterConfigFacadeInterface, task *model.DataplaneInstallTask, failureReason string) {
	switch task.InstallScope {
	case model.InstallScopeInfrastructure:
		// Infrastructure only - update infrastructure status
		if err := clusterFacade.UpdateInfrastructureStatus(ctx, task.ClusterName, model.InfrastructureStatusFailed, failureReason); err != nil {
			log.Errorf("Failed to update infrastructure status for cluster %s: %v", task.ClusterName, err)
		} else {
			log.Infof("Updated infrastructure status to 'failed' for cluster %s", task.ClusterName)
		}
	case model.InstallScopeApps:
		// Apps only - update dataplane status
		if err := clusterFacade.UpdateDataplaneStatus(ctx, task.ClusterName, model.DataplaneStatusFailed, failureReason); err != nil {
			log.Errorf("Failed to update dataplane status for cluster %s: %v", task.ClusterName, err)
		} else {
			log.Infof("Updated dataplane status to 'failed' for cluster %s", task.ClusterName)
		}
	case model.InstallScopeFull, "":
		// Full installation - determine which status to update based on current stage
		if isInfrastructureStage(task.CurrentStage) {
			if err := clusterFacade.UpdateInfrastructureStatus(ctx, task.ClusterName, model.InfrastructureStatusFailed, failureReason); err != nil {
				log.Errorf("Failed to update infrastructure status for cluster %s: %v", task.ClusterName, err)
			} else {
				log.Infof("Updated infrastructure status to 'failed' for cluster %s (failed at stage %s)", task.ClusterName, task.CurrentStage)
			}
		} else {
			// Infrastructure was ready, apps failed
			if err := clusterFacade.UpdateInfrastructureStatus(ctx, task.ClusterName, model.InfrastructureStatusReady, ""); err != nil {
				log.Warnf("Failed to update infrastructure status to ready for cluster %s: %v", task.ClusterName, err)
			}
			if err := clusterFacade.UpdateDataplaneStatus(ctx, task.ClusterName, model.DataplaneStatusFailed, failureReason); err != nil {
				log.Errorf("Failed to update dataplane status for cluster %s: %v", task.ClusterName, err)
			} else {
				log.Infof("Updated dataplane status to 'failed' for cluster %s (failed at stage %s)", task.ClusterName, task.CurrentStage)
			}
		}
	}
}

// isInfrastructureStage checks if the given stage is an infrastructure stage
func isInfrastructureStage(stage string) bool {
	infrastructureStages := map[string]bool{
		"pending":             true,
		"operators":           true,
		"wait_operators":      true,
		"infrastructure":      true,
		"wait_infrastructure": true,
		"init":                true,
		"database_migration":  true,
		"storage_secret":      true,
	}
	return infrastructureStages[stage]
}
