/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package main

import (
	"flag"
	"os"

	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/server"
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "/etc/a2a-gateway/config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		klog.Fatalf("failed to load config: %v", err)
	}

	srv, err := server.New(cfg)
	if err != nil {
		klog.Fatalf("failed to create server: %v", err)
	}

	klog.InfoS("A2A Gateway starting", "port", cfg.ServerPort, "metricsPort", cfg.MetricsPort)
	if err := srv.Run(); err != nil {
		klog.Fatalf("server error: %v", err)
	}
}
