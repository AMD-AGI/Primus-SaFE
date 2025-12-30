package bootstrap

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/background"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/gin-gonic/gin"
)

// Global components
var (
	registry  airegistry.Registry
	taskQueue *aitaskqueue.PGStore
	bgManager *background.Manager
)

// Bootstrap starts the AI Gateway service using the core server framework
func Bootstrap(ctx context.Context) error {
	return server.InitServerWithPreInitFunc(ctx, func(ctx context.Context, cfg *config.Config) error {
		log.Info("Initializing AI Gateway components...")

		// Get registry mode from config (default to db)
		registryMode := "db"
		if cfg.AIGateway != nil && cfg.AIGateway.RegistryMode != "" {
			registryMode = cfg.AIGateway.RegistryMode
		}

		// Initialize registry based on mode
		var err error
		registry, err = initRegistry(registryMode)
		if err != nil {
			log.Errorf("Failed to initialize registry: %v", err)
			return err
		}

		// Initialize task queue with default config
		queueConfig := &aitaskqueue.QueueConfig{
			DefaultTimeout:    5 * 60, // 5 minutes in seconds
			DefaultMaxRetries: 3,
			RetentionDays:     7,
		}
		taskQueue = aitaskqueue.NewPGStore("", queueConfig)

		// Create and start background jobs
		bgManager = background.NewManager(registry, taskQueue, nil)
		bgManager.Start(ctx)

		// Register cleanup for background manager
		go func() {
			<-ctx.Done()
			log.Info("Shutting down AI Gateway background jobs...")
			bgManager.Stop()
		}()

		// Register routes
		router.RegisterGroup(initRouter)

		log.Info("AI Gateway initialized successfully")
		return nil
	})
}

// initRegistry initializes the agent registry based on mode
func initRegistry(mode string) (airegistry.Registry, error) {
	switch mode {
	case "memory":
		log.Info("Using in-memory registry")
		return airegistry.NewMemoryStore(), nil
	case "db":
		log.Info("Using database registry")
		return airegistry.NewDBStore(""), nil
	default:
		log.Warnf("Unknown registry mode: %s, using memory", mode)
		return airegistry.NewMemoryStore(), nil
	}
}

// initRouter registers all AI Gateway routes
func initRouter(group *gin.RouterGroup) error {
	// Agent registration endpoints
	agentsGroup := group.Group("/ai/agents")
	{
		agentHandler := api.NewAgentHandler(registry)
		agentsGroup.POST("/register", agentHandler.Register)
		agentsGroup.DELETE("/:name", agentHandler.Unregister)
		agentsGroup.GET("", agentHandler.List)
		agentsGroup.GET("/:name", agentHandler.Get)
		agentsGroup.GET("/:name/health", agentHandler.GetHealth)
	}

	// Task endpoints
	tasksGroup := group.Group("/ai/tasks")
	{
		taskHandler := api.NewTaskHandler(taskQueue)
		tasksGroup.GET("/:id", taskHandler.GetTask)
		tasksGroup.GET("/:id/status", taskHandler.GetTaskStatus)
		tasksGroup.POST("/:id/cancel", taskHandler.CancelTask)
		tasksGroup.GET("", taskHandler.ListTasks)
	}

	// Stats endpoint
	statsGroup := group.Group("/ai/stats")
	{
		statsHandler := api.NewStatsHandler(registry, taskQueue)
		statsGroup.GET("", statsHandler.GetStats)
	}

	log.Info("AI Gateway routes registered successfully")
	return nil
}
