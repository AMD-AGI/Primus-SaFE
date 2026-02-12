// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/conf"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/bootstrap"
)

func main() {
	// Initialize logger
	logConf := conf.DefaultConfig()
	logConf.Level = conf.InfoLevel
	log.InitGlobalLogger(logConf)

	log.Info("Starting Skills Repository Service...")

	// Create and begin server
	server, err := bootstrap.NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	// Start server in background
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down Skills Repository Service...")
	if err := server.Stop(); err != nil {
		log.Errorf("Error during shutdown: %v", err)
	}
	log.Info("Skills Repository Service stopped")
}
