package bootstrap

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/router"
	"github.com/AMD-AGI/primus-lens/core/pkg/server"
	"github.com/AMD-AGI/primus-lens/telemetry-processor/pkg/module/alerts"
	"github.com/AMD-AGI/primus-lens/telemetry-processor/pkg/module/logs"
	"github.com/AMD-AGI/primus-lens/telemetry-processor/pkg/module/metrics"
	"github.com/AMD-AGI/primus-lens/telemetry-processor/pkg/module/pods"
	"github.com/gin-gonic/gin"
)

func Bootstrap(ctx context.Context) error {
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
