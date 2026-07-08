// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.
// Build trigger: dual registry support

package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/collector"
	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/network-exporter/pkg/reporter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	port := flag.String("port", getEnvOrDefault("NETWORK_EXPORTER_PORT", "9402"), "metrics port")
	interval := flag.Int("interval", getEnvOrDefaultInt("NETWORK_EXPORTER_INTERVAL", 5), "collection interval in seconds")
	logLevel := flag.String("log-level", getEnvOrDefault("NETWORK_EXPORTER_LOG_LEVEL", "info"), "log level (debug, info, warn, error)")
	flag.Parse()

	// Setup logging
	setupLogging(*logLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build reporter config from environment
	reporterCfg := buildReporterConfig()

	// Initialize handler
	handler, err := collector.NewHandler(*interval, reporterCfg)
	if err != nil {
		slog.Error("Failed to create handler", "error", err)
		os.Exit(1)
	}

	// Log startup info
	slog.Info("Network Exporter starting",
		"port", *port,
		"interval", *interval,
		"report_enabled", reporterCfg != nil && reporterCfg.Enabled,
	)

	// Initialize BPF programs
	if err := handler.Init(ctx); err != nil {
		slog.Error("Failed to initialize handler", "error", err)
		os.Exit(1)
	}
	defer handler.Close()

	// Start background collectors
	handler.Start(ctx)

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(handler, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readyHandler)

	server := &http.Server{
		Addr:         ":" + *port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		slog.Info("Received shutdown signal", "signal", sig)
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("Server shutdown error", "error", err)
		}
	}()

	slog.Info("Starting HTTP server", "addr", server.Addr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("Server error", "error", err)
		os.Exit(1)
	}

	slog.Info("Network Exporter stopped")
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

func setupLogging(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvOrDefaultInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		for _, c := range value {
			if c >= '0' && c <= '9' {
				result = result*10 + int(c-'0')
			} else {
				return defaultValue
			}
		}
		return result
	}
	return defaultValue
}

// buildReporterConfig constructs reporter config from environment variables:
//   - NETWORK_EXPORTER_REPORT_ENABLED: "true" to enable (default: disabled)
//   - FAULT_MANAGER_ENDPOINT: HTTP endpoint for fault-manager
//   - NETWORK_EXPORTER_REPORT_INTERVAL: report interval in seconds (default: 60)
func buildReporterConfig() *reporter.Config {
	enabled := strings.EqualFold(os.Getenv("NETWORK_EXPORTER_REPORT_ENABLED"), "true")
	endpoint := os.Getenv("FAULT_MANAGER_ENDPOINT")
	intervalSec := getEnvOrDefaultInt("NETWORK_EXPORTER_REPORT_INTERVAL", 60)

	return &reporter.Config{
		Enabled:  enabled,
		Endpoint: endpoint,
		Interval: time.Duration(intervalSec) * time.Second,
	}
}
