/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the A2A Gateway configuration.
type Config struct {
	ServerPort       int    `yaml:"server_port"`
	MetricsPort      int    `yaml:"metrics_port"`
	DBSecretPath     string `yaml:"db_secret_path"`
	DBSSLMode        string `yaml:"db_ssl_mode"`
	CryptoSecretPath string `yaml:"crypto_secret_path"`
}

// Load reads configuration from a YAML file.
func Load(path string) (*Config, error) {
	cfg := &Config{
		ServerPort:       8089,
		MetricsPort:      9090,
		DBSecretPath:     "/etc/secrets/db",
		DBSSLMode:        "require",
		CryptoSecretPath: "/etc/secrets/crypto",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}
	return cfg, nil
}
