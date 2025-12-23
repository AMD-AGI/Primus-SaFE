package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/bootstrap"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Infof("Received signal %v, shutting down...", sig)
		cancel()
	}()

	if err := bootstrap.Bootstrap(ctx); err != nil {
		log.Fatalf("Failed to bootstrap inference metrics exporter: %v", err)
	}
}
