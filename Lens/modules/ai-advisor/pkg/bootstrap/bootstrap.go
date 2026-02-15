// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package bootstrap

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/api/handlers"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/common"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/distill"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/loganalysis"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/pipeline"
	advisorTask "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aigateway"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	configHelper "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router/middleware"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/snapshot"
	coreTask "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

// Global handlers
var (
	detectionHandler      *handlers.DetectionHandler
	analysisHandler       *handlers.AnalysisHandler
	recommendationHandler *handlers.RecommendationHandler
	anomalyHandler        *handlers.AnomalyHandler
	diagnosticsHandler    *handlers.DiagnosticsHandler
	insightsHandler       *handlers.InsightsHandler
	wandbHandler          *handlers.WandBHandler
	coordinatorHandler    *handlers.CoordinatorHandler
)

// generateInstanceID generates instance ID
func generateInstanceID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	rand.Seed(time.Now().UnixNano())
	randomSuffix := rand.Intn(10000)

	return fmt.Sprintf("ai-advisor-%s-%d", hostname, randomSuffix)
}

func Bootstrap(ctx context.Context) error {
	// Initialize OpenTelemetry tracer
	err := trace.InitTracer("primus-lens-ai-advisor")
	if err != nil {
		log.Errorf("Failed to init OpenTelemetry tracer: %v", err)
		// Don't block startup, degrade to no tracing
	} else {
		log.Info("OpenTelemetry tracer initialized successfully for AI Advisor service")
	}

	// Register cleanup function
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()

	// Register Kubernetes scheme
	if err := controller.RegisterScheme(schemes); err != nil {
		log.Errorf("Failed to register Kubernetes scheme: %v", err)
		return fmt.Errorf("failed to register scheme: %w", err)
	}
	log.Info("Kubernetes scheme registered successfully")

	return server.InitServerWithPreInitFunc(ctx, func(ctx context.Context, cfg *config.Config) error {
		// Initialize dependencies
		metadataFacade := database.NewAiWorkloadMetadataFacade()
		systemConfigMgr := configHelper.GetDefaultConfigManager()

		// Generate instance ID for this AI Advisor instance
		instanceID := generateInstanceID()
		log.Infof("AI Advisor instance ID: %s", instanceID)

		// Initialize detection manager
		detectionMgr, err := detection.InitializeDetectionManager(metadataFacade, systemConfigMgr, instanceID)
		if err != nil {
			log.Errorf("Failed to initialize detection manager: %v", err)
			// Don't block startup, but warn
		} else {
			log.Info("Detection manager initialized successfully")

			// Register evidence bridge to store legacy detections as evidence
			detection.RegisterEvidenceBridge(detectionMgr)
		}

		// Initialize metadata collector
		// Create storage using the metadata facade (which has DB access internally)
		storage := metadata.NewFacadeStorage(metadataFacade)
		if err := metadata.InitCollector(ctx, storage); err != nil {
			log.Errorf("Failed to initialize metadata collector: %v", err)
			// Don't block startup, but warn
		} else {
			log.Info("Metadata collector initialized successfully")
		}

		// Initialize task scheduler with increased concurrency
		// Default is 20, but clusters with 100+ active workloads need more capacity
		// Long-running tasks (profiler_collection, tensorboard_stream) can run for hours/days
		// Quick tasks (detection_coordinator, *_probe) should not be blocked by long-running ones
		schedulerConfig := coreTask.DefaultSchedulerConfig()
		schedulerConfig.MaxConcurrentTasks = 100 // Increased from 20 to support parallel execution
		taskScheduler := coreTask.NewTaskScheduler(instanceID, schedulerConfig)

		// Register metadata collection executor
		metadataExecutor := advisorTask.NewMetadataCollectionExecutor(metadata.GetCollector())
		if err := taskScheduler.RegisterExecutor(metadataExecutor); err != nil {
			log.Errorf("Failed to register metadata collection executor: %v", err)
		} else {
			log.Info("Metadata collection executor registered")
		}

		// Register TensorBoard stream executor
		tensorboardStreamExecutor := advisorTask.NewTensorBoardStreamExecutor()
		if err := taskScheduler.RegisterExecutor(tensorboardStreamExecutor); err != nil {
			log.Errorf("Failed to register tensorboard stream executor: %v", err)
		} else {
			log.Info("TensorBoard stream executor registered")
		}

		// Register Active Detection executor (legacy, to be replaced by coordinator)
		activeDetectionExecutor := advisorTask.NewActiveDetectionExecutor(metadata.GetCollector())
		if err := taskScheduler.RegisterExecutor(activeDetectionExecutor); err != nil {
			log.Errorf("Failed to register active detection executor: %v", err)
		} else {
			log.Info("Active detection executor registered")
		}

		// Register Detection Coordinator executor
		detectionCoordinator := advisorTask.NewDetectionCoordinator(metadata.GetCollector(), instanceID)
		if err := taskScheduler.RegisterExecutor(detectionCoordinator); err != nil {
			log.Errorf("Failed to register detection coordinator: %v", err)
		} else {
			log.Info("Detection coordinator registered")
		}

		// Register detection sub-task executors
		processProbeExecutor := advisorTask.NewProcessProbeExecutor(metadata.GetCollector())
		if err := taskScheduler.RegisterExecutor(processProbeExecutor); err != nil {
			log.Errorf("Failed to register process probe executor: %v", err)
		} else {
			log.Info("Process probe executor registered")
		}

		imageProbeExecutor := advisorTask.NewImageProbeExecutor(metadata.GetCollector())
		if err := taskScheduler.RegisterExecutor(imageProbeExecutor); err != nil {
			log.Errorf("Failed to register image probe executor: %v", err)
		} else {
			log.Info("Image probe executor registered")
		}

		labelProbeExecutor := advisorTask.NewLabelProbeExecutor(metadata.GetCollector())
		if err := taskScheduler.RegisterExecutor(labelProbeExecutor); err != nil {
			log.Errorf("Failed to register label probe executor: %v", err)
		} else {
			log.Info("Label probe executor registered")
		}

		logDetectionExecutor := advisorTask.NewLogDetectionExecutor()
		if err := taskScheduler.RegisterExecutor(logDetectionExecutor); err != nil {
			log.Errorf("Failed to register log detection executor: %v", err)
		} else {
			log.Info("Log detection executor registered")
		}

		// Register Analysis Pipeline executor (intent-aware replacement for coordinator)
		conductorURL := os.Getenv("CONDUCTOR_URL")
		if conductorURL == "" {
			conductorURL = "http://primus-conductor:8080"
		}
		aiGatewayURL := os.Getenv("AI_GATEWAY_URL")
		if aiGatewayURL == "" {
			aiGatewayURL = "http://primus-lens-ai-gateway:8080/api/v1"
		}
		podProber := common.NewPodProber(metadata.GetCollector())

		// Initialize snapshot store for code snapshots (S3 / local / inline-DB)
		var snapshotStore snapshot.Store
		if cfg.SnapshotStore != nil && cfg.SnapshotStore.Enabled {
			snapCfg := cfg.SnapshotStore.ToSnapshotConfig()
			store, storeErr := snapshot.New(snapCfg)
			if storeErr != nil {
				log.Errorf("Failed to initialize snapshot store (%s): %v", snapCfg.Type, storeErr)
				log.Warn("Code snapshots will fall back to inline database storage")
			} else {
				snapshotStore = store
				log.Infof("Snapshot store initialized: type=%s", store.Type())
			}
		}

		analysisPipeline := pipeline.NewWorkloadAnalysisPipeline(conductorURL, aiGatewayURL, instanceID, podProber, snapshotStore)
		if err := taskScheduler.RegisterExecutor(analysisPipeline); err != nil {
			log.Errorf("Failed to register analysis pipeline executor: %v", err)
		} else {
			log.Info("Analysis pipeline executor registered")
		}

		// Register intent-analyzer agent with ai-gateway via HTTP API
		go registerIntentAnalyzerAgent(ctx, aiGatewayURL, instanceID)

		// Register log analysis executor (training metric gap detection)
		logAnalysisExecutor := loganalysis.NewLogAnalysisExecutor(aiGatewayURL)
		if err := taskScheduler.RegisterExecutor(logAnalysisExecutor); err != nil {
			log.Errorf("Failed to register log analysis executor: %v", err)
		} else {
			log.Info("Log analysis executor registered")
		}

		// Initialize profiler services (includes ProfilerCollectionExecutor registration)
		// Pass metadata collector for node-exporter client access
		if err := InitProfilerServices(ctx, taskScheduler, metadata.GetCollector()); err != nil {
			log.Errorf("Failed to initialize profiler services: %v", err)
			// Don't block startup, but warn
		} else {
			log.Info("Profiler services initialized successfully")
		}

		// Start task scheduler
		if err := taskScheduler.Start(); err != nil {
			log.Errorf("Failed to start task scheduler: %v", err)
		} else {
			log.Infof("Task scheduler started (instance: %s)", instanceID)
		}

		// Start periodic scan for undetected workloads
		taskCreator := detection.GetTaskCreator()
		if taskCreator != nil {
			go startPeriodicWorkloadScan(ctx, taskCreator)
			log.Info("Periodic workload scan started (interval: 1 minute)")

			// Start periodic scan for workloads needing intent analysis
			go startPeriodicIntentScan(ctx, taskCreator)
			log.Info("Periodic intent analysis scan started (interval: 2 minutes)")
		}

		// Start daily flywheel jobs (backtesting + promotion + distillation via ai-gateway)
		flywheelGWClient := aigateway.NewClient(aiGatewayURL)
		go startDailyFlywheelJobs(ctx, flywheelGWClient)
		log.Info("Daily flywheel jobs started (backtest + promote + distill via ai-gateway, interval: 24h)")

		// Register cleanup for task scheduler
		go func() {
			<-ctx.Done()
			log.Info("Shutting down task scheduler...")
			if err := taskScheduler.Stop(); err != nil {
				log.Errorf("Error stopping task scheduler: %v", err)
			}
		}()

		// Initialize handlers
		detectionHandler = handlers.NewDetectionHandler(detectionMgr)
		analysisHandler = handlers.NewAnalysisHandler(metadataFacade)
		recommendationHandler = handlers.NewRecommendationHandler(metadataFacade)
		anomalyHandler = handlers.NewAnomalyHandler(metadataFacade)
		diagnosticsHandler = handlers.NewDiagnosticsHandler(metadataFacade)
		insightsHandler = handlers.NewInsightsHandler(metadataFacade)
		wandbHandler = handlers.NewWandBHandler(detection.GetWandBDetector())
		coordinatorHandler = handlers.NewCoordinatorHandler()

		// Register routes
		router.RegisterGroup(initRouter)

		log.Info("AI Advisor initialized successfully")
		return nil
	})
}

// startPeriodicWorkloadScan starts a goroutine that periodically scans for
// undetected workloads and creates detection coordinator tasks for them
func startPeriodicWorkloadScan(ctx context.Context, taskCreator *detection.TaskCreator) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Run initial scan after a short delay to let the system stabilize
	time.Sleep(10 * time.Second)
	if err := taskCreator.ScanForUndetectedWorkloads(ctx); err != nil {
		log.Errorf("Initial workload scan failed: %v", err)
	} else {
		log.Debug("Initial workload scan completed")
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping periodic workload scan")
			return
		case <-ticker.C:
			if err := taskCreator.ScanForUndetectedWorkloads(ctx); err != nil {
				log.Errorf("Periodic workload scan failed: %v", err)
			} else {
				log.Debug("Periodic workload scan completed")
			}
		}
	}
}

// startPeriodicIntentScan starts a goroutine that periodically scans for
// workloads that have been detected but need intent analysis
func startPeriodicIntentScan(ctx context.Context, taskCreator *detection.TaskCreator) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	// Initial delay to let detection coordinator populate workload_detection first
	time.Sleep(30 * time.Second)
	if err := taskCreator.ScanForWorkloadsNeedingIntent(ctx); err != nil {
		log.Errorf("Initial intent scan failed: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping periodic intent scan")
			return
		case <-ticker.C:
			if err := taskCreator.ScanForWorkloadsNeedingIntent(ctx); err != nil {
				log.Errorf("Periodic intent scan failed: %v", err)
			}
		}
	}
}

func initRouter(group *gin.RouterGroup) error {
	// Framework Detection APIs
	detectionGroup := group.Group("/detection")
	{
		// Report detection from any source
		detectionGroup.POST("", detectionHandler.ReportDetection)

		// Query detection results
		detectionGroup.GET("/workloads/:uid", middleware.WithTracingRate(0.001), detectionHandler.GetDetection)
		detectionGroup.POST("/batch", detectionHandler.BatchGetDetection)

		// Statistics
		detectionGroup.GET("/stats", detectionHandler.GetStats)

		// Manual annotation
		detectionGroup.PUT("/workloads/:uid", detectionHandler.UpdateDetection)

		// Detection Coordinator APIs
		// Log detection report from telemetry-processor
		detectionGroup.POST("/log-report", coordinatorHandler.HandleLogReport)

		// Detection coverage APIs
		detectionGroup.GET("/coverage/:uid", coordinatorHandler.GetCoverageStatus)
		detectionGroup.POST("/coverage/:uid/initialize", coordinatorHandler.InitializeCoverage)
		detectionGroup.GET("/coverage/:uid/log-window", coordinatorHandler.GetUncoveredLogWindow)
	}

	// Performance Analysis APIs
	analysisGroup := group.Group("/analysis")
	{
		analysisGroup.POST("/performance", analysisHandler.AnalyzePerformance)
		analysisGroup.GET("/workloads/:uid/performance", analysisHandler.GetPerformanceReport)
		analysisGroup.GET("/workloads/:uid/trends", analysisHandler.GetTrends)
	}

	// Anomaly Detection APIs
	anomalyGroup := group.Group("/anomalies")
	{
		anomalyGroup.POST("/detect", anomalyHandler.DetectAnomalies)
		anomalyGroup.GET("/workloads/:uid", anomalyHandler.GetAnomalies)
		anomalyGroup.GET("/workloads/:uid/latest", anomalyHandler.GetLatestAnomalies)
	}

	// Recommendation APIs
	recommendationGroup := group.Group("/recommendations")
	{
		recommendationGroup.GET("/workloads/:uid", recommendationHandler.GetRecommendations)
		recommendationGroup.POST("/evaluate", recommendationHandler.EvaluateRecommendations)
		recommendationGroup.POST("/workloads/:uid/generate", recommendationHandler.GenerateRecommendations)
	}

	// Diagnostics APIs
	diagnosticsGroup := group.Group("/diagnostics")
	{
		diagnosticsGroup.POST("/analyze", diagnosticsHandler.AnalyzeWorkload)
		diagnosticsGroup.GET("/workloads/:uid", diagnosticsHandler.GetDiagnosticReport)
		diagnosticsGroup.GET("/workloads/:uid/root-causes", diagnosticsHandler.GetRootCauses)
	}

	// Model Insights APIs
	insightsGroup := group.Group("/insights")
	{
		insightsGroup.POST("/model", insightsHandler.AnalyzeModel)
		insightsGroup.GET("/workloads/:uid", insightsHandler.GetModelInsights)
		insightsGroup.POST("/estimate-memory", insightsHandler.EstimateMemory)
		insightsGroup.POST("/estimate-compute", insightsHandler.EstimateCompute)
	}

	// WandB APIs
	wandbGroup := group.Group("/wandb")
	{
		wandbGroup.POST("/detection", wandbHandler.ReceiveDetection)
	}

	// Workload Metadata APIs
	metadataGroup := group.Group("/metadata")
	{
		// Collect metadata for a workload
		metadataGroup.POST("/collect", handlers.CollectWorkloadMetadata)

		// Query metadata
		metadataGroup.GET("/workloads/:uid", middleware.WithTracingRate(0.001), handlers.GetWorkloadMetadata)
		metadataGroup.POST("/query", handlers.QueryWorkloadMetadata)
		metadataGroup.GET("/recent", handlers.ListRecentMetadata)
		metadataGroup.GET("/frameworks/:framework", handlers.GetMetadataByFramework)

		// Statistics
		metadataGroup.GET("/stats", handlers.GetMetadataStatistics)

		// Management
		metadataGroup.DELETE("/workloads/:uid", handlers.DeleteWorkloadMetadata)
	}

	// Health check
	group.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "ai-advisor",
		})
	})

	log.Info("AI Advisor routes registered successfully")
	return nil
}

// registerIntentAnalyzerAgent registers the intent-analyzer agent via ai-gateway HTTP API.
// It retries periodically so that the advisor can start even if the gateway is not yet ready.
func registerIntentAnalyzerAgent(ctx context.Context, aiGatewayURL string, instanceID string) {
	// Wait for the server to be ready
	time.Sleep(5 * time.Second)

	if aiGatewayURL == "" {
		log.Warn("AI_GATEWAY_URL not set, skipping agent registration")
		return
	}

	gwClient := aigateway.NewClient(aiGatewayURL)

	endpoint := os.Getenv("AI_ADVISOR_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://primus-lens-ai-advisor:8080"
	}

	reg := &aigateway.AgentRegistration{
		Name:        "intent-analyzer",
		Version:     "1.0.0",
		Description: "Workload intent analyzer - determines what GPU workloads are doing",
		Endpoint:    endpoint,
		Topics: []string{
			"intent.analyze.workload",
			"intent.analyze.logs",
			"intent.analyze.code",
		},
		Tags: []string{"instance:" + instanceID},
	}

	// Retry registration up to 5 times with backoff
	for attempt := 1; attempt <= 5; attempt++ {
		if err := gwClient.RegisterAgent(ctx, reg); err != nil {
			log.Warnf("Failed to register intent-analyzer agent (attempt %d/5): %v", attempt, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(attempt*5) * time.Second):
				continue
			}
		}
		log.Info("Intent-analyzer agent registered with ai-gateway via HTTP API")
		return
	}
	log.Error("Exhausted retries for intent-analyzer agent registration")
}

// startDailyFlywheelJobs runs the flywheel cycle:
// 1. Backtest all candidate rules (proposed/testing status)
// 2. Promote validated rules, retire underperforming ones
// 3. Trigger distillation for new confirmed intents via ai-gateway
func startDailyFlywheelJobs(ctx context.Context, gwClient *aigateway.Client) {
	// Wait for initial data to accumulate
	time.Sleep(5 * time.Minute)

	// Run immediately on first start, then every 24 hours
	runFlywheelCycle(ctx, gwClient)

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("Stopping daily flywheel jobs")
			return
		case <-ticker.C:
			runFlywheelCycle(ctx, gwClient)
		}
	}
}

func runFlywheelCycle(ctx context.Context, gwClient *aigateway.Client) {
	log.Info("Flywheel: starting daily cycle")

	// Step 1: Backtest all candidate rules
	backtester := distill.NewBacktester()
	backtested, err := backtester.BacktestAll(ctx)
	if err != nil {
		log.Errorf("Flywheel: backtesting failed: %v", err)
	} else {
		log.Infof("Flywheel: backtested %d rules", backtested)
	}

	// Step 2: Promote validated rules, retire underperforming
	promoter := distill.NewPromoter()
	if err := promoter.RunPromotionCycle(ctx); err != nil {
		log.Errorf("Flywheel: promotion cycle failed: %v", err)
	}

	// Step 3: Trigger distillation via ai-gateway (Conductor bridge picks it up)
	if gwClient != nil {
		triggerDistillation(ctx, gwClient)
	}

	log.Info("Flywheel: daily cycle complete")
}

// triggerDistillation collects confirmed intents grouped by category
// and publishes distill tasks to ai-gateway
func triggerDistillation(ctx context.Context, gwClient *aigateway.Client) {
	facade := database.NewWorkloadDetectionFacade()

	categories := []string{
		"training", "fine_tuning", "inference", "evaluation",
		"data_processing", "benchmark",
	}

	for _, category := range categories {
		detections, _, err := facade.ListByCategory(ctx,
			category, 50, 0,
		)
		if err != nil {
			log.Errorf("Flywheel: failed to fetch detections for category %s: %v", category, err)
			continue
		}

		if len(detections) < 5 {
			continue
		}

		log.Infof("Flywheel: triggering distillation for category=%s with %d samples", category, len(detections))

		distill.TriggerConductorDistillation(ctx, gwClient, category, detections)
	}
}
