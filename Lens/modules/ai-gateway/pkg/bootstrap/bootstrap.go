package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/background"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// Run starts the AI Gateway service
func Run(ctx context.Context) error {
	// Load configuration
	cfg := loadConfig()

	log.Info("Starting AI Gateway...")
	log.Infof("Registry mode: %s", cfg.Registry.Mode)

	// Initialize registry
	registry, err := initRegistry(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize registry: %w", err)
	}

	// Initialize task queue
	taskQueue := initTaskQueue(cfg)

	// Create and start background jobs
	bgManager := background.NewManager(registry, taskQueue, cfg)
	bgManager.Start(ctx)
	defer bgManager.Stop()

	// Create and start HTTP server
	srv := server.New(cfg, registry, taskQueue)
	return srv.Run(ctx)
}

// loadConfig loads configuration from file or environment
func loadConfig() *config.Config {
	configPath := os.Getenv("AI_GATEWAY_CONFIG")
	if configPath != "" {
		cfg, err := config.LoadFromFile(configPath)
		if err != nil {
			log.Warnf("Failed to load config from %s: %v, using defaults", configPath, err)
			return config.LoadFromEnv()
		}
		return cfg
	}
	return config.LoadFromEnv()
}

// initRegistry initializes the agent registry based on configuration
func initRegistry(cfg *config.Config) (airegistry.Registry, error) {
	switch cfg.Registry.Mode {
	case "memory":
		log.Info("Using in-memory registry")
		return airegistry.NewMemoryStore(), nil
	case "config":
		log.Info("Using config-based registry")
		agents := make([]airegistry.StaticAgentConfig, len(cfg.Registry.StaticAgents))
		for i, a := range cfg.Registry.StaticAgents {
			agents[i] = airegistry.StaticAgentConfig{
				Name:            a.Name,
				Endpoint:        a.Endpoint,
				Topics:          a.Topics,
				Timeout:         a.Timeout,
				HealthCheckPath: a.HealthCheckPath,
			}
		}
		return airegistry.NewConfigStore(agents), nil
	case "db":
		log.Info("Using database registry")
		return airegistry.NewDBStore(""), nil
	default:
		log.Warnf("Unknown registry mode: %s, using memory", cfg.Registry.Mode)
		return airegistry.NewMemoryStore(), nil
	}
}

// initTaskQueue initializes the task queue
func initTaskQueue(cfg *config.Config) *aitaskqueue.PGStore {
	queueConfig := &aitaskqueue.QueueConfig{
		DefaultTimeout:       5 * 60, // 5 minutes in seconds
		DefaultMaxRetries:    3,
		RetentionDays:        7,
		CleanupInterval:      cfg.Background.Cleanup.Interval,
		TimeoutCheckInterval: cfg.Background.Timeout.Interval,
	}
	return aitaskqueue.NewPGStore("", queueConfig)
}

// GetFacade returns the database facade for the current cluster
func GetFacade() database.FacadeInterface {
	return database.GetFacade()
}

