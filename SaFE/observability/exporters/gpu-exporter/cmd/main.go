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
	"syscall"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/observability/exporters/gpu-exporter/pkg/collector"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	port := flag.String("port", getEnvOrDefault("GPU_EXPORTER_PORT", "9400"), "metrics port")
	interval := flag.Int("interval", getEnvOrDefaultInt("GPU_EXPORTER_INTERVAL", 5), "collection interval in seconds")
	logLevel := flag.String("log-level", getEnvOrDefault("GPU_EXPORTER_LOG_LEVEL", "info"), "log level (debug, info, warn, error)")
	flag.Parse()

	// Setup logging
	setupLogging(*logLevel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize collector
	c := collector.New(*interval)

	// Log startup info
	slog.Info("GPU Exporter starting",
		"port", *port,
		"interval", *interval,
		"use_nsenter", c.GetExecutor().IsNsenterEnabled(),
	)

	// Run preflight checks
	if err := collector.RunPreflightChecks(c.GetExecutor()); err != nil {
		slog.Warn("Preflight checks had warnings", "error", err)
		// Don't exit, as some checks might fail in development
	}

	// Start collector in background
	go c.Start(ctx)

	// HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
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

	slog.Info("GPU Exporter stopped")
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
		if _, err := os.Stdout.Write([]byte("")); err == nil {
			// Try to parse
			for _, c := range value {
				if c >= '0' && c <= '9' {
					result = result*10 + int(c-'0')
				} else {
					return defaultValue
				}
			}
			return result
		}
	}
	return defaultValue
}
