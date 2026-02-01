// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config represents the skills repository configuration
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Database  DatabaseConfig  `yaml:"database"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Discovery DiscoveryConfig `yaml:"discovery"`
}

// ServerConfig represents server configuration
type ServerConfig struct {
	Port int `yaml:"port"`
}

// DatabaseConfig represents database configuration
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"db_name"`
	SSLMode  string `yaml:"ssl_mode"`
}

// EmbeddingConfig represents embedding service configuration
type EmbeddingConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	Dimension int    `yaml:"dimension"`
	APIKey    string `yaml:"api_key"`
	BaseURL   string `yaml:"base_url"`
}

// DiscoveryConfig represents skills discovery configuration
type DiscoveryConfig struct {
	Sources      []SourceConfig `yaml:"sources"`
	SyncInterval string         `yaml:"sync_interval"`
	WatchEnabled bool           `yaml:"watch_enabled"`
}

// SourceConfig represents a skill source configuration
type SourceConfig struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"` // git, local
	URL      string `yaml:"url"`
	Branch   string `yaml:"branch"`
	Priority int    `yaml:"priority"`
	Watch    bool   `yaml:"watch"`
}

// Load loads configuration from environment variables and config file
func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: getEnvInt("SERVER_PORT", 8080),
		},
		Database: DatabaseConfig{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "primus-lens-control-plane"),
			Password: getEnv("DB_PASSWORD", ""),
			DBName:   getEnv("DB_NAME", "primus-lens-control-plane"),
			SSLMode:  getEnv("DB_SSL_MODE", "require"),
		},
		Embedding: EmbeddingConfig{
			Provider:  getEnv("EMBEDDING_PROVIDER", "openai"),
			Model:     getEnv("EMBEDDING_MODEL", "BAAI/bge-m3"),
			Dimension: getEnvInt("EMBEDDING_DIMENSION", 1024),
			APIKey:    getEnv("OPENAI_API_KEY", ""),
			BaseURL:   getEnv("OPENAI_BASE_URL", ""),
		},
		Discovery: DiscoveryConfig{
			SyncInterval: getEnv("DISCOVERY_SYNC_INTERVAL", "5m"),
			WatchEnabled: getEnvBool("DISCOVERY_WATCH_ENABLED", true),
		},
	}

	// Try to load from config file
	configPath := getEnv("CONFIG_PATH", "/etc/skills-repository/config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, err
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}
