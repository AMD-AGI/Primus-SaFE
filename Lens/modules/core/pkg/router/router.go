package router

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router/middleware"
	"github.com/gin-gonic/gin"
)

var (
	groupRegisters []GroupRegister
)

func RegisterGroup(group GroupRegister) {
	groupRegisters = append(groupRegisters, group)
}

func InitRouter(engine *gin.Engine, cfg *config.Config) error {
	g := engine.Group("/v1")
	g.Use(middleware.HandleMetrics())
	// 根据配置决定是否启用日志中间件
	if cfg.Middleware.IsLoggingEnabled() {
		log.Info("HTTP request logging middleware enabled")
		g.Use(middleware.HandleLogging())
	} else {
		log.Info("HTTP request logging middleware disabled")
	}

	// 错误处理中间件始终启用
	g.Use(middleware.HandleErrors())

	// 根据配置决定是否启用追踪中间件
	if cfg.Middleware.IsTracingEnabled() {
		log.Info("Distributed tracing middleware enabled")
		g.Use(middleware.HandleTracing())
	} else {
		log.Info("Distributed tracing middleware disabled")
	}

	// CORS中间件始终启用
	g.Use(middleware.CorsMiddleware())

	for _, group := range groupRegisters {
		err := group(g)
		if err != nil {
			return err
		}
	}
	return nil
}

type RouterRegister func(engine *gin.Engine) error

type GroupRegister func(group *gin.RouterGroup) error
