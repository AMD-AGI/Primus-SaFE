package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/AMD-AGI/Primus-SaFE/Lens/ai-gateway/pkg/bootstrap"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Info("Shutdown signal received, stopping...")
		cancel()
	}()

	if err := bootstrap.Run(ctx); err != nil {
		log.Errorf("Failed to start ai-gateway: %v", err)
		os.Exit(1)
	}
}

