package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/api"
	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/config"
	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/history"
	"github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/relay"
	smtpsender "github.com/AMD-AIG-AIMA/SAFE/tools/email-relay-service/internal/smtp"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	initLogger(cfg.LogLevel)

	slog.Info("Email relay service starting",
		"smtp", fmt.Sprintf("%s:%d", cfg.SMTP.Host, cfg.SMTP.Port),
		"from", cfg.SMTP.From,
		"clusters", len(cfg.Clusters),
	)

	sender := smtpsender.NewSender(cfg.SMTP)
	store := history.NewStore(500)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("Received signal, shutting down", "signal", sig)
		cancel()
	}()

	// Start API server with dashboard UI
	apiServer := api.NewServer(sender, store)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.APIPort),
		Handler: apiServer.Handler(),
	}
	go func() {
		slog.Info("API server listening", "port", cfg.APIPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("API server error", "error", err)
		}
	}()
	go func() {
		<-ctx.Done()
		httpServer.Close()
	}()

	var wg sync.WaitGroup
	for _, clusterCfg := range cfg.Clusters {
		wg.Add(1)
		client := relay.NewClusterClient(clusterCfg, sender, store)
		go func() {
			defer wg.Done()
			client.Run(ctx)
		}()
		slog.Info("Started relay client", "cluster", clusterCfg.Name, "url", clusterCfg.BaseURL)
	}

	wg.Wait()
	slog.Info("Email relay service stopped")
}

func initLogger(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}
