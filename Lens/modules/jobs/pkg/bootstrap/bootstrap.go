package bootstrap

import (
	"context"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aiclient"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitopics"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/jobs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func Init(ctx context.Context, cfg *config.Config) error {
	if cfg.Jobs == nil {
		return errors.New("jobs config is required")
	}

	// Initialize OpenTelemetry tracer
	err := trace.InitTracer("primus-lens-jobs")
	if err != nil {
		log.Errorf("Failed to init OpenTelemetry tracer: %v", err)
		// Don't block startup, degrade to no tracing
	} else {
		log.Info("OpenTelemetry tracer initialized successfully for jobs service")
	}

	// Register cleanup function
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()

	err = controller.RegisterScheme(schemes)
	if err != nil {
		return err
	}

	// Initialize AI client if configured
	if err := initAIClient(cfg.Jobs); err != nil {
		log.Warnf("Failed to initialize AI client: %v, AI extraction will be disabled", err)
		// Don't fail startup, just disable AI features
	}

	// Start jobs with configuration
	err = jobs.Start(ctx, cfg.Jobs)
	if err != nil {
		return err
	}

	return nil
}

// initAIClient initializes the global AI client for jobs that need AI extraction
func initAIClient(jobsCfg *config.JobsConfig) error {
	// Check if AI agent is configured
	if jobsCfg == nil || jobsCfg.AIAgent == nil || jobsCfg.AIAgent.Endpoint == "" {
		log.Info("AI agent not configured, skipping AI client initialization")
		return nil
	}

	agentCfg := jobsCfg.AIAgent

	// Create static agent configuration for the configured AI agent
	staticAgents := []airegistry.StaticAgentConfig{
		{
			Name:            agentCfg.Name,
			Endpoint:        agentCfg.Endpoint,
			Topics:          []string{aitopics.TopicGithubMetricsExtract},
			Timeout:         agentCfg.Timeout,
			HealthCheckPath: "/health",
		},
	}

	// Create registry with static config
	registry, err := airegistry.NewRegistry(&airegistry.RegistryConfig{
		Mode:                "config",
		StaticAgents:        staticAgents,
		HealthCheckInterval: 30 * time.Second,
		UnhealthyThreshold:  3,
	})
	if err != nil {
		return err
	}

	// Create AI client with default config
	clientCfg := aiclient.DefaultClientConfig()
	if agentCfg.Timeout > 0 {
		clientCfg.DefaultTimeout = agentCfg.Timeout
	}
	if agentCfg.Retry > 0 {
		clientCfg.RetryCount = agentCfg.Retry
	}

	client := aiclient.New(clientCfg, registry, nil)
	aiclient.SetGlobalClient(client)

	log.Infof("AI client initialized with agent: %s at %s", agentCfg.Name, agentCfg.Endpoint)
	return nil
}
