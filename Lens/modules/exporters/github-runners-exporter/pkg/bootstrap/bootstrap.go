// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aiclient"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/workflow"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/backfill"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/executor"
	"github.com/AMD-AGI/Primus-SaFE/Lens/github-runners-exporter/pkg/processor"
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
	// stateProcessor processes runner state changes and creates tasks
	stateProcessor *processor.RunnerStateProcessor
	// staleRunCleaner periodically detects and cleans stale running records
	staleRunCleaner *processor.StaleRunCleaner
)

// defaultAgentEndpoint is the default in-cluster endpoint for the AI agent service.
// This is used when no explicit AI agent config is provided in the config file.
const defaultAgentEndpoint = "http://lens-agent-api-service:8001"

// Init initializes the github-runners-exporter
func Init(ctx context.Context, cfg *config.Config) error {
	if err := RegisterController(ctx); err != nil {
		return err
	}

	// Initialize AI client for schema analysis (optional, collection works with fallback)
	if err := initAIClient(cfg); err != nil {
		log.Warnf("Failed to initialize AI client: %v, schema analysis will use DB fallback", err)
	}

	// Initialize TaskScheduler for collection tasks
	if err := InitTaskScheduler(ctx); err != nil {
		return err
	}

	// Initialize RunnerStateProcessor (reads runner_states table, creates tasks)
	if err := InitStateProcessor(ctx); err != nil {
		return err
	}

	// Initialize and start backfill runner for historical data processing
	if err := InitBackfillRunner(ctx); err != nil {
		return err
	}

	// Initialize StaleRunCleaner (detects stale running/pending records and cleans them)
	if err := InitStaleRunCleaner(ctx); err != nil {
		return err
	}

	log.Info("GitHub Runners Exporter initialized successfully")
	return nil
}

// initAIClient initializes the global AI client for schema analysis.
// If the AI agent is not reachable, collection will fall back to using existing schemas from DB.
func initAIClient(cfg *config.Config) error {
	// Determine AI agent endpoint from config or use default
	agentEndpoint := defaultAgentEndpoint
	agentName := "lens-agent-api"
	agentTimeout := 120 * time.Second

	if cfg.Jobs != nil && cfg.Jobs.AIAgent != nil && cfg.Jobs.AIAgent.Endpoint != "" {
		agentEndpoint = cfg.Jobs.AIAgent.Endpoint
		if cfg.Jobs.AIAgent.Name != "" {
			agentName = cfg.Jobs.AIAgent.Name
		}
		if cfg.Jobs.AIAgent.Timeout > 0 {
			agentTimeout = cfg.Jobs.AIAgent.Timeout
		}
	}

	// Create static agent configuration
	staticAgents := []airegistry.StaticAgentConfig{
		{
			Name:            agentName,
			Endpoint:        agentEndpoint,
			Topics:          []string{aitopics.TopicGithubMetricsExtract, aitopics.TopicGithubSchemaAnalyze},
			Timeout:         agentTimeout,
			HealthCheckPath: "/health",
		},
	}

	// Create registry
	registry, err := airegistry.NewRegistry(&airegistry.RegistryConfig{
		Mode:                "config",
		StaticAgents:        staticAgents,
		HealthCheckInterval: 30 * time.Second,
		UnhealthyThreshold:  3,
	})
	if err != nil {
		return err
	}

	// Create and set global AI client
	clientCfg := aiclient.DefaultClientConfig()
	clientCfg.DefaultTimeout = agentTimeout
	client := aiclient.New(clientCfg, registry, nil)
	aiclient.SetGlobalClient(client)

	log.Infof("AI client initialized with agent: %s at %s", agentName, agentEndpoint)
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
		// Consume collection, sync, and graph-fetch tasks
		// Note: analysis/code-indexing tasks are handled by agent service
		ConsumeTaskTypes: []string{
			constant.TaskTypeGithubWorkflowCollection,
			workflow.TaskTypeGithubWorkflowSync, // Legacy sync (deprecated, kept for backward compatibility)
			constant.TaskTypeGithubInitialSync,    // Event-driven: on runner creation
			constant.TaskTypeGithubCompletionSync, // Event-driven: on runner completion
			constant.TaskTypeGithubPeriodicSync,   // Event-driven: every 5 minutes until workflow completes
			constant.TaskTypeGithubManualSync,     // Event-driven: user-triggered manual sync
			constant.TaskTypeGithubGraphFetch,      // Fetch workflow graph (jobs/steps) from GitHub API
			workflow.TaskTypeGithubWorkflowLogFetch, // Fetch and cache workflow logs
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

	// Register SyncExecutor for real-time workflow state synchronization (legacy, deprecated)
	// Kept for backward compatibility with existing tasks
	syncExecutor := workflow.NewSyncExecutor()
	if err := taskScheduler.RegisterExecutor(syncExecutor); err != nil {
		return err
	}
	log.Info("SyncExecutor registered with TaskScheduler (legacy)")

	// Register event-driven sync executors (new, replacing high-frequency polling)
	initialSyncExecutor := workflow.NewInitialSyncExecutor()
	if err := taskScheduler.RegisterExecutor(initialSyncExecutor); err != nil {
		return err
	}
	log.Info("InitialSyncExecutor registered with TaskScheduler")

	completionSyncExecutor := workflow.NewCompletionSyncExecutor()
	if err := taskScheduler.RegisterExecutor(completionSyncExecutor); err != nil {
		return err
	}
	log.Info("CompletionSyncExecutor registered with TaskScheduler")

	periodicSyncExecutor := workflow.NewPeriodicSyncExecutor()
	if err := taskScheduler.RegisterExecutor(periodicSyncExecutor); err != nil {
		return err
	}
	log.Info("PeriodicSyncExecutor registered with TaskScheduler")

	manualSyncExecutor := workflow.NewManualSyncExecutor()
	if err := taskScheduler.RegisterExecutor(manualSyncExecutor); err != nil {
		return err
	}
	log.Info("ManualSyncExecutor registered with TaskScheduler")

	// Register GraphFetchExecutor for fetching workflow graph (jobs/steps) from GitHub API
	graphFetchExecutor := executor.NewGraphFetchExecutor(clientSets)
	if err := taskScheduler.RegisterExecutor(graphFetchExecutor); err != nil {
		return err
	}
	log.Info("GraphFetchExecutor registered with TaskScheduler")

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

	// Recover orphaned periodic sync tasks (broken chains from previous restart)
	// and backfill completed summaries with missing job data
	go func() {
		workflow.RecoverOrphanedPeriodicSyncs(ctx)
		workflow.BackfillCompletedSummaries(ctx)
	}()

	return nil
}

// StopTaskScheduler stops the TaskScheduler gracefully
func StopTaskScheduler() error {
	if taskScheduler != nil {
		return taskScheduler.Stop()
	}
	return nil
}

// InitStateProcessor initializes the RunnerStateProcessor for processing K8s state changes
func InitStateProcessor(ctx context.Context) error {
	stateProcessor = processor.NewRunnerStateProcessor(&processor.ProcessorConfig{
		ScanInterval: 3 * time.Second,
		BatchSize:    100,
	})

	if err := stateProcessor.Start(ctx); err != nil {
		return err
	}
	log.Info("RunnerStateProcessor started for K8s state processing")
	return nil
}

// StopStateProcessor stops the RunnerStateProcessor gracefully
func StopStateProcessor() error {
	if stateProcessor != nil {
		return stateProcessor.Stop()
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

// InitStaleRunCleaner initializes the StaleRunCleaner for detecting stale running records
func InitStaleRunCleaner(ctx context.Context) error {
	staleRunCleaner = processor.NewStaleRunCleaner(&processor.StaleRunCleanerConfig{
		CheckInterval:  30 * time.Second,
		StaleThreshold: 10 * time.Minute,
	})

	if err := staleRunCleaner.Start(ctx); err != nil {
		return err
	}
	log.Info("StaleRunCleaner started for detecting stale running records")
	return nil
}

// StopStaleRunCleaner stops the StaleRunCleaner gracefully
func StopStaleRunCleaner() error {
	if staleRunCleaner != nil {
		return staleRunCleaner.Stop()
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

