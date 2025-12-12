package bootstrap

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controller"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/server"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/api"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector/report"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var schemes = &runtime.SchemeBuilder{
	corev1.AddToScheme,
}

func Bootstrap(ctx context.Context) error {
	// Setup graceful shutdown handler
	setupGracefulShutdown()

	return server.InitServerWithPreInitFunc(ctx, func(ctx context.Context, cfg *config.Config) error {
		err := controller.RegisterScheme(schemes)
		if err != nil {
			return err
		}
		if err := collector.Init(ctx, *cfg); err != nil {
			return err
		}

		// Initialize container filesystem readers
		api.InitContainerFS()

		router.RegisterGroup(api.RegisterRouter)
		collector.Start(ctx)
		return nil
	})
}

// setupGracefulShutdown sets up graceful shutdown handler for HTTP reporter
func setupGracefulShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Infof("Received signal %v, shutting down gracefully...", sig)

		// Shutdown HTTP reporter to flush any buffered events
		report.Shutdown()

		log.Info("Graceful shutdown completed")
		os.Exit(0)
	}()
}
