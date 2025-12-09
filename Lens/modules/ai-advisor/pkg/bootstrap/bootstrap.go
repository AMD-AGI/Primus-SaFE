package bootstrap

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/api/handlers"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/metadata"
	advisorTask "github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	configHelper "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router/middleware"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
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
)

// generateInstanceID 生成实例 ID
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

		// Initialize TensorBoard reader
		handlers.InitTensorBoardReader()

		// Note: TensorBoard stream HTTP APIs are disabled.
		// Stream functionality is available through task executor only.
		// handlers.InitStreamReader()

		// Initialize task scheduler
		taskScheduler := coreTask.NewTaskScheduler(instanceID, coreTask.DefaultSchedulerConfig())

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

		// Start task scheduler
		if err := taskScheduler.Start(); err != nil {
			log.Errorf("Failed to start task scheduler: %v", err)
		} else {
			log.Infof("Task scheduler started (instance: %s)", instanceID)
		}

		// Register cleanup for task scheduler
		go func() {
			<-ctx.Done()
			log.Info("Shutting down task scheduler...")
			if err := taskScheduler.Stop(); err != nil {
				log.Errorf("Error stopping task scheduler: %v", err)
			}
		}()

		// Initialize task monitor handler (for API)
		handlers.InitTaskMonitor()

		// Initialize handlers
		detectionHandler = handlers.NewDetectionHandler(detectionMgr)
		analysisHandler = handlers.NewAnalysisHandler(metadataFacade)
		recommendationHandler = handlers.NewRecommendationHandler(metadataFacade)
		anomalyHandler = handlers.NewAnomalyHandler(metadataFacade)
		diagnosticsHandler = handlers.NewDiagnosticsHandler(metadataFacade)
		insightsHandler = handlers.NewInsightsHandler(metadataFacade)
		wandbHandler = handlers.NewWandBHandler(detection.GetWandBDetector())

		// Register routes
		router.RegisterGroup(initRouter)

		log.Info("AI Advisor initialized successfully")
		return nil
	})
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

	// Task Monitor APIs (替代原 WorkloadMonitor APIs)
	taskGroup := group.Group("/tasks")
	{
		// 任务统计
		taskGroup.GET("/stats", handlers.GetTaskStatistics)

		// 任务列表
		taskGroup.GET("", handlers.ListAllTasks) // 支持 ?status=xxx&task_type=xxx

		// 特定任务详情
		taskGroup.GET("/:workload_uid/:task_type", handlers.GetTask)

		// 某个 workload 的所有任务
		taskGroup.GET("/workload/:workload_uid", handlers.ListWorkloadTasks)

		// 活跃的流式任务
		taskGroup.GET("/streams/active", handlers.GetActiveStreams)
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

	// TensorBoard Log Access APIs (Non-intrusive)
	tensorboardGroup := group.Group("/tensorboard")
	{
		// Get TensorBoard log files information
		tensorboardGroup.POST("/logs", handlers.GetTensorBoardLogs)

		// Read specific event file
		tensorboardGroup.POST("/event", handlers.ReadTensorBoardEvent)

		// List all event files
		tensorboardGroup.POST("/files", handlers.ListTensorBoardEventFiles)

		// Generic container file operations (with security restrictions)
		tensorboardGroup.POST("/file/read", handlers.ReadContainerFile)
		tensorboardGroup.POST("/file/info", handlers.GetContainerFileInfo)

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
