// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the installer configuration
type Config struct {
	// Task identification
	TaskID      int32
	ClusterName string

	// Control Plane DB connection
	CPDBHost     string
	CPDBPort     int
	CPDBName     string
	CPDBUser     string
	CPDBPassword string
	CPDBSSLMode  string

	// Helm settings
	HelmTimeout string
	DryRun      bool
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	taskIDStr := os.Getenv("TASK_ID")
	if taskIDStr == "" {
		return nil, fmt.Errorf("TASK_ID environment variable is required")
	}
	taskID, err := strconv.ParseInt(taskIDStr, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid TASK_ID: %w", err)
	}

	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		return nil, fmt.Errorf("CLUSTER_NAME environment variable is required")
	}

	cpDBHost := os.Getenv("CP_DB_HOST")
	if cpDBHost == "" {
		cpDBHost = "primus-lens-control-plane-primary.primus-lens.svc.cluster.local"
	}

	cpDBPortStr := os.Getenv("CP_DB_PORT")
	cpDBPort := 5432
	if cpDBPortStr != "" {
		if p, err := strconv.Atoi(cpDBPortStr); err == nil {
			cpDBPort = p
		}
	}

	cpDBName := os.Getenv("CP_DB_NAME")
	if cpDBName == "" {
		cpDBName = "primus-lens-control-plane"
	}

	cpDBUser := os.Getenv("CP_DB_USER")
	if cpDBUser == "" {
		return nil, fmt.Errorf("CP_DB_USER environment variable is required")
	}

	cpDBPassword := os.Getenv("CP_DB_PASSWORD")
	if cpDBPassword == "" {
		return nil, fmt.Errorf("CP_DB_PASSWORD environment variable is required")
	}

	cpDBSSLMode := os.Getenv("CP_DB_SSL_MODE")
	if cpDBSSLMode == "" {
		cpDBSSLMode = "require"
	}

	helmTimeout := os.Getenv("HELM_TIMEOUT")
	if helmTimeout == "" {
		helmTimeout = "10m"
	}

	dryRun := os.Getenv("DRY_RUN") == "true"

	return &Config{
		TaskID:       int32(taskID),
		ClusterName:  clusterName,
		CPDBHost:     cpDBHost,
		CPDBPort:     cpDBPort,
		CPDBName:     cpDBName,
		CPDBUser:     cpDBUser,
		CPDBPassword: cpDBPassword,
		CPDBSSLMode:  cpDBSSLMode,
		HelmTimeout:  helmTimeout,
		DryRun:       dryRun,
	}, nil
}

// GetCPDBDSN returns the PostgreSQL connection string for control plane DB
func (c *Config) GetCPDBDSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.CPDBHost, c.CPDBPort, c.CPDBUser, c.CPDBPassword, c.CPDBName, c.CPDBSSLMode,
	)
}
