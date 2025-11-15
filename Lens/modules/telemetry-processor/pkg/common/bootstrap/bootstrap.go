package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/alerts"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/containers"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/logs"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/metrics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/telemetry-processor/pkg/module/pods"
	"github.com/gin-gonic/gin"
)

func Bootstrap(ctx context.Context) error {
	// Initialize OpenTelemetry tracer
	err := trace.InitTracer("primus-lens-telemetry-processor")
	if err != nil {
		log.Errorf("Failed to init OpenTelemetry tracer: %v", err)
		// Don't block startup, degrade to no tracing
	} else {
		log.Info("OpenTelemetry tracer initialized successfully for telemetry-processor service")
	}

	// Register cleanup function
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()

	return server.InitServerWithPreInitFunc(ctx, func(ctx context.Context, cfg *config.Config) error {
		router.RegisterGroup(initRouter)
		pods.StartRefreshCaches(ctx)
		return nil
	})
}

func initRouter(group *gin.RouterGroup) error {
	// Metrics and logs endpoints
	group.Any("prometheus", metrics.InsertHandler)
	group.GET("pods/cache", metrics.GetPodCache)
	group.GET("pods/workload/cache", metrics.GetPodWorkloadCache)
	group.POST("logs", logs.ReceiveHttpLogs)
	
	// Metrics debug endpoints
	group.POST("metrics/debug/config", metrics.SetDebugConfigHandler)
	group.GET("metrics/debug/config", metrics.GetDebugConfigHandler)
	group.GET("metrics/debug/records", metrics.GetDebugRecordsHandler)
	group.DELETE("metrics/debug/records", metrics.ClearDebugRecordsHandler)
	group.POST("metrics/debug/disable", metrics.DisableDebugHandler)

	// Container event endpoints
	group.POST("container-events", containers.ReceiveContainerEvent)
	group.POST("container-events/batch", containers.ReceiveBatchContainerEvents)

	// Alert reception endpoints
	group.POST("alerts/metric", alerts.ReceiveMetricAlert)
	group.POST("alerts/log", alerts.ReceiveLogAlert)
	group.POST("alerts/trace", alerts.ReceiveTraceAlert)
	group.POST("alerts/webhook", alerts.ReceiveGenericWebhook)

	// Alert query endpoints
	group.GET("alerts", alerts.ListAlerts)
	group.GET("alerts/:id", alerts.GetAlert)
	group.GET("alerts/:id/correlations", alerts.GetAlertCorrelationsAPI)
	group.GET("alerts/statistics", alerts.GetAlertStatistics)

	// Alert rule management endpoints
	group.POST("alert-rules", alerts.CreateAlertRule)
	group.GET("alert-rules", alerts.ListAlertRules)
	group.GET("alert-rules/:id", alerts.GetAlertRule)
	group.PUT("alert-rules/:id", alerts.UpdateAlertRule)
	group.DELETE("alert-rules/:id", alerts.DeleteAlertRule)

	// Silence management endpoints
	group.POST("silences", alerts.CreateSilence)
	group.GET("silences", alerts.ListSilences)
	group.DELETE("silences/:id", alerts.DeleteSilence)

	return nil
}
