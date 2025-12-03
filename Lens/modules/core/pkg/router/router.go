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
	// Decide whether to enable logging middleware based on configuration
	if cfg.Middleware.IsLoggingEnabled() {
		log.Info("HTTP request logging middleware enabled")
		g.Use(middleware.HandleLogging())
	} else {
		log.Info("HTTP request logging middleware disabled")
	}

	// Error handling middleware is always enabled
	g.Use(middleware.HandleErrors())

	// Decide whether to enable tracing middleware based on configuration
	if cfg.Middleware.IsTracingEnabled() {
		log.Info("Distributed tracing middleware enabled")
		g.Use(middleware.HandleTracing())
	} else {
		log.Info("Distributed tracing middleware disabled")
	}

	// CORS middleware is always enabled
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
