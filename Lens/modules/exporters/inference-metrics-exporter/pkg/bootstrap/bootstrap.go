package bootstrap

import (
	"context"

	coreConfig "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/transformer"
	"github.com/gin-gonic/gin"
)

// Bootstrap initializes and starts the inference metrics exporter
func Bootstrap(ctx context.Context) error {
	// Initialize OpenTelemetry tracer
	if err := trace.InitTracer("inference-metrics-exporter"); err != nil {
		log.Errorf("Failed to init OpenTelemetry tracer: %v", err)
		// Don't block startup, degrade to no tracing
	} else {
		log.Info("OpenTelemetry tracer initialized successfully")
	}

	// Register cleanup function
	go func() {
		<-ctx.Done()
		if err := trace.CloseTracer(); err != nil {
			log.Errorf("Failed to close tracer: %v", err)
		}
	}()

	return server.InitServerWithPreInitFunc(ctx, func(ctx context.Context, cfg *coreConfig.Config) error {
		// Initialize default metric transformers
		transformer.InitDefaultTransformers()

		// Initialize exporter config
		exporterCfg := config.LoadExporterConfig()
		log.Infof("Loaded exporter config: instanceID=%s, pollInterval=%v, lockDuration=%v",
			exporterCfg.InstanceID, exporterCfg.TaskPollInterval, exporterCfg.LockDuration)

		// Initialize config watcher for hot-reload
		configWatcher := config.NewConfigWatcher(exporterCfg.ConfigReloadInterval)
		if err := configWatcher.Start(ctx); err != nil {
			log.Errorf("Failed to start config watcher: %v", err)
			// Continue anyway, use default configs
		}

		// Initialize metrics exporter (for /metrics endpoint)
		metricsExporter := exporter.NewMetricsExporter()

		// Initialize task receiver
		taskReceiver := task.NewTaskReceiver(exporterCfg, metricsExporter)

		// Start task receiver
		if err := taskReceiver.Start(ctx); err != nil {
			return err
		}

		// Register cleanup on context done
		go func() {
			<-ctx.Done()
			if err := configWatcher.Stop(); err != nil {
				log.Errorf("Failed to stop config watcher: %v", err)
			}
			if err := taskReceiver.Stop(); err != nil {
				log.Errorf("Failed to stop task receiver: %v", err)
			}
		}()

		// Register routes
		router.RegisterGroup(func(group *gin.RouterGroup) error {
			return api.RegisterRoutes(group, taskReceiver, metricsExporter, configWatcher)
		})

		return nil
	})
}

