package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// Server represents the AI Gateway HTTP server
type Server struct {
	config    *config.Config
	registry  airegistry.Registry
	taskQueue *aitaskqueue.PGStore
	router    *gin.Engine
}

// New creates a new Server instance
func New(cfg *config.Config, registry airegistry.Registry, taskQueue *aitaskqueue.PGStore) *Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(loggerMiddleware())

	srv := &Server{
		config:    cfg,
		registry:  registry,
		taskQueue: taskQueue,
		router:    router,
	}

	srv.setupRoutes()
	return srv
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Health check
	s.router.GET("/health", s.healthCheck)
	s.router.GET("/ready", s.readyCheck)

	// API v1
	v1 := s.router.Group("/api/v1")
	{
		// Agent registration endpoints
		agents := v1.Group("/ai/agents")
		{
			agentHandler := api.NewAgentHandler(s.registry)
			agents.POST("/register", agentHandler.Register)
			agents.DELETE("/:name", agentHandler.Unregister)
			agents.GET("", agentHandler.List)
			agents.GET("/:name", agentHandler.Get)
			agents.GET("/:name/health", agentHandler.GetHealth)
		}

		// Task endpoints
		tasks := v1.Group("/ai/tasks")
		{
			taskHandler := api.NewTaskHandler(s.taskQueue)
			tasks.GET("/:id", taskHandler.GetTask)
			tasks.GET("/:id/status", taskHandler.GetTaskStatus)
			tasks.POST("/:id/cancel", taskHandler.CancelTask)
			tasks.GET("", taskHandler.ListTasks)
		}

		// Metrics and stats
		stats := v1.Group("/ai/stats")
		{
			statsHandler := api.NewStatsHandler(s.registry, s.taskQueue)
			stats.GET("", statsHandler.GetStats)
		}
	}
}

// Run starts the HTTP server
func (s *Server) Run(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	log.Infof("AI Gateway listening on %s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      s.router,
		ReadTimeout:  time.Duration(s.config.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.Server.WriteTimeout) * time.Second,
	}

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		log.Info("Shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// healthCheck returns the health status
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "healthy",
		"time":   time.Now().UTC(),
	})
}

// readyCheck returns the readiness status
func (s *Server) readyCheck(c *gin.Context) {
	// Check if registry is available
	_, err := s.registry.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"status": "not ready",
			"error":  err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
		"time":   time.Now().UTC(),
	})
}

// loggerMiddleware returns a gin middleware for logging
func loggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		if path != "/health" && path != "/ready" {
			log.Infof("%s %s %d %v", c.Request.Method, path, status, latency)
		}
	}
}

