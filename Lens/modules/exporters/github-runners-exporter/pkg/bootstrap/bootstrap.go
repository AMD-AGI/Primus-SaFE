// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/workflow"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/backfill"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/executor"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/reconciler"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var schemes = &runtime.SchemeBuilder{}

// schemeAdder adds the AutoScalingRunnerSet and EphemeralRunner types to the scheme
func schemeAdder(scheme *runtime.Scheme) error {
	// Register corev1 types (Pod, PodList, etc.) required by PVCReader to access node-exporter
	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}

	// Register the unstructured types for GitHub Actions Runner Controller CRDs
	scheme.AddKnownTypeWithName(
		types.AutoScalingRunnerSetGVK,
		&runtime.Unknown{},
	)
	scheme.AddKnownTypeWithName(
		types.EphemeralRunnerGVK,
		&runtime.Unknown{},
	)

	// Add GroupVersion to registry
	metaGV := schema.GroupVersion{Group: "actions.github.com", Version: "v1alpha1"}
	scheme.AddKnownTypes(metaGV)

	return nil
}

func init() {
	schemes.Register(schemeAdder)
}

var (
	// taskScheduler is the global task scheduler for this exporter
	taskScheduler *task.TaskScheduler
	// backfillRunner is the global backfill runner for this exporter
	backfillRunner *backfill.WorkflowBackfillRunner
)

// Init initializes the github-runners-exporter
func Init(ctx context.Context, cfg *config.Config) error {
	if err := RegisterController(ctx); err != nil {
		return err
	}

	// Initialize TaskScheduler for collection tasks
	if err := InitTaskScheduler(ctx); err != nil {
		return err
	}

	// Initialize and start backfill runner for historical data processing
	if err := InitBackfillRunner(ctx); err != nil {
		return err
	}

	log.Info("GitHub Runners Exporter initialized successfully")
	return nil
}

// InitTaskScheduler initializes the TaskScheduler for GitHub workflow collection
func InitTaskScheduler(ctx context.Context) error {
	// Create scheduler config with collection-specific settings
	schedulerConfig := &task.SchedulerConfig{
		ScanInterval:             5 * time.Second,  // Fast scan for real-time collection
		LockDuration:             5 * time.Minute,
		HeartbeatInterval:        30 * time.Second,
		MaxConcurrentTasks:       10, // Allow parallel collection
		StaleLockCleanupInterval: 1 * time.Minute,
		AutoStart:                true,
		// Consume collection and sync tasks - analysis tasks are handled by agent service
		ConsumeTaskTypes: []string{
			constant.TaskTypeGithubWorkflowCollection,
			workflow.TaskTypeGithubWorkflowSync,
		},
	}

	// Generate unique instance ID
	instanceID := "github-workflow-exporter"

	taskScheduler = task.NewTaskScheduler(instanceID, schedulerConfig)

	// Get K8S client for collector
	clusterManager := clientsets.GetClusterManager()
	currentCluster := clusterManager.GetCurrentClusterClients()
	var clientSets *clientsets.K8SClientSet
	if currentCluster != nil {
		clientSets = currentCluster.K8SClientSet
	}

	// Register CollectionExecutor
	collectionExecutor := executor.NewCollectionExecutor(clientSets)
	if err := taskScheduler.RegisterExecutor(collectionExecutor); err != nil {
		return err
	}
	log.Info("CollectionExecutor registered with TaskScheduler")

	// Register SyncExecutor for real-time workflow state synchronization
	syncExecutor := workflow.NewSyncExecutor()
	if err := taskScheduler.RegisterExecutor(syncExecutor); err != nil {
		return err
	}
	log.Info("SyncExecutor registered with TaskScheduler")

	// Register LogFetchExecutor for fetching and caching workflow logs
	logFetchExecutor := workflow.NewLogFetchExecutor()
	if err := taskScheduler.RegisterExecutor(logFetchExecutor); err != nil {
		return err
	}
	log.Info("LogFetchExecutor registered with TaskScheduler")

	// Start the scheduler
	if err := taskScheduler.Start(); err != nil {
		return err
	}
	log.Info("TaskScheduler started for GitHub workflow collection")

	return nil
}

// StopTaskScheduler stops the TaskScheduler gracefully
func StopTaskScheduler() error {
	if taskScheduler != nil {
		return taskScheduler.Stop()
	}
	return nil
}

// InitBackfillRunner initializes the backfill runner for historical data processing
func InitBackfillRunner(ctx context.Context) error {
	backfillRunner = backfill.NewWorkflowBackfillRunner()
	if err := backfillRunner.Start(ctx); err != nil {
		return err
	}
	log.Info("BackfillRunner started for GitHub workflow historical data processing")
	return nil
}

// StopBackfillRunner stops the BackfillRunner gracefully
func StopBackfillRunner() error {
	if backfillRunner != nil {
		return backfillRunner.Stop()
	}
	return nil
}

// RegisterController registers the reconcilers with the controller manager
func RegisterController(ctx context.Context) error {
	if err := controller.RegisterScheme(schemes); err != nil {
		return err
	}

	// Register AutoScalingRunnerSet reconciler
	arsReconciler := reconciler.NewAutoScalingRunnerSetReconciler()
	if err := arsReconciler.Init(ctx); err != nil {
		log.Errorf("Failed to initialize AutoScalingRunnerSetReconciler: %v", err)
		return err
	}
	controller.RegisterReconciler(arsReconciler)
	log.Info("AutoScalingRunnerSetReconciler registered")

	// Register EphemeralRunner reconciler
	erReconciler := reconciler.NewEphemeralRunnerReconciler()
	if err := erReconciler.Init(ctx); err != nil {
		log.Errorf("Failed to initialize EphemeralRunnerReconciler: %v", err)
		return err
	}
	controller.RegisterReconciler(erReconciler)
	log.Info("EphemeralRunnerReconciler registered")

	return nil
}

