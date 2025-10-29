package bootstrap

import (
	"context"

	"github.com/AMD-AGI/primus-lens/core/pkg/config"
	"github.com/AMD-AGI/primus-lens/core/pkg/router"
	"github.com/AMD-AGI/primus-lens/core/pkg/server"
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
	group.Any("prometheus", metrics.InsertHandler)
	group.GET("pods/cache", metrics.GetPodCache)
	group.GET("pods/workload/cache", metrics.GetPodWorkloadCache)
	group.POST("logs", logs.ReceiveHttpLogs)
	return nil
}
