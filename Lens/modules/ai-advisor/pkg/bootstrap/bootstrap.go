package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/api/handlers"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-advisor/pkg/detection"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	configHelper "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router/middleware"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/gin-gonic/gin"
)

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

	return server.InitServerWithPreInitFunc(ctx, func(ctx context.Context, cfg *config.Config) error {
		// Initialize dependencies
		metadataFacade := database.NewAiWorkloadMetadataFacade()
		systemConfigMgr := configHelper.GetDefaultConfigManager()

		// Initialize detection manager
		detectionMgr, err := detection.InitializeDetectionManager(metadataFacade, systemConfigMgr)
		if err != nil {
			log.Errorf("Failed to initialize detection manager: %v", err)
			// Don't block startup, but warn
		} else {
			log.Info("Detection manager initialized successfully")
		}

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
