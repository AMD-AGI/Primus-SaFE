// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

// This file provides convenient examples of common configuration usage

import (
	"context"
)

// Example configurations structures
// Example configuration structures

// DatabaseConfig database configuration
type DatabaseConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Database        string `json:"database"`
	MaxConnections  int    `json:"max_connections"`
	ConnMaxLifetime int    `json:"conn_max_lifetime"`
}

// SMTPConfig SMTP configuration
type SMTPConfig struct{
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	FromEmail  string `json:"from_email"`
	FromName   string `json:"from_name"`
	EnableTLS  bool   `json:"enable_tls"`
	EnableAuth bool   `json:"enable_auth"`
}

// FeatureFlags feature flags configuration
type FeatureFlags struct {
	EnableNewUI        bool   `json:"enable_new_ui"`
	EnableBetaFeature  bool   `json:"enable_beta_feature"`
	EnableDebugMode    bool   `json:"enable_debug_mode"`
	MaxUploadSize      int64  `json:"max_upload_size"`
	MaxConcurrentUsers int    `json:"max_concurrent_users"`
	APIRateLimit       int    `json:"api_rate_limit"`
	LogLevel           string `json:"log_level"`
}

// CacheConfig cache configuration
type CacheConfig struct {
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type"` // redis, memcached, memory
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Password    string `json:"password"`
	DB          int    `json:"db"`
	MaxRetries  int    `json:"max_retries"`
	PoolSize    int    `json:"pool_size"`
	IdleTimeout int    `json:"idle_timeout"`
}

// SecurityConfig security configuration
type SecurityConfig struct {
	JWTSecret              string   `json:"jwt_secret"`
	JWTExpirationHours     int      `json:"jwt_expiration_hours"`
	PasswordMinLength      int      `json:"password_min_length"`
	PasswordRequireSpecial bool     `json:"password_require_special"`
	PasswordRequireNumber  bool     `json:"password_require_number"`
	PasswordRequireUpper   bool     `json:"password_require_upper"`
	MaxLoginAttempts       int      `json:"max_login_attempts"`
	LockoutDurationMinutes int      `json:"lockout_duration_minutes"`
	AllowedOrigins         []string `json:"allowed_origins"`
	EnableCSRF             bool     `json:"enable_csrf"`
}

// Predefined configuration keys
// Predefined configuration key constants
const (
	KeyDatabaseConfig = "system.database.config"
	KeySMTPConfig     = "system.smtp.config"
	KeyFeatureFlags   = "system.feature.flags"
	KeyCacheConfig    = "system.cache.config"
	KeySecurityConfig = "system.security.config"
)

// Configuration categories
// Configuration category constants
const (
	CategorySystem   = "system"
	CategoryDatabase = "database"
	CategoryEmail    = "email"
	CategoryFeature  = "feature"
	CategoryCache    = "cache"
	CategorySecurity = "security"
	CategoryNetwork  = "network"
	CategoryStorage  = "storage"
)

// Utility functions for common operations
// Utility functions for common operations

// GetDatabaseConfig retrieves database configuration
func GetDatabaseConfig(ctx context.Context, manager *Manager) (*DatabaseConfig, error) {
	var config DatabaseConfig
	err := manager.Get(ctx, KeyDatabaseConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SetDatabaseConfig sets database configuration
func SetDatabaseConfig(ctx context.Context, manager *Manager, config *DatabaseConfig, updatedBy string) error {
	return manager.Set(ctx, KeyDatabaseConfig, config,
		WithDescription("Database connection configuration"),
		WithCategory(CategoryDatabase),
		WithUpdatedBy(updatedBy),
		WithRecordHistory(true),
	)
}

// GetFeatureFlags retrieves feature flags configuration
func GetFeatureFlags(ctx context.Context, manager *Manager) (*FeatureFlags, error) {
	var flags FeatureFlags
	err := manager.Get(ctx, KeyFeatureFlags, &flags)
	if err != nil {
		return nil, err
	}
	return &flags, nil
}

// SetFeatureFlags sets feature flags configuration
func SetFeatureFlags(ctx context.Context, manager *Manager, flags *FeatureFlags, updatedBy string) error {
	return manager.Set(ctx, KeyFeatureFlags, flags,
		WithDescription("System feature flags configuration"),
		WithCategory(CategoryFeature),
		WithUpdatedBy(updatedBy),
		WithRecordHistory(true),
	)
}

// GetSecurityConfig retrieves security configuration
func GetSecurityConfig(ctx context.Context, manager *Manager) (*SecurityConfig, error) {
	var config SecurityConfig
	err := manager.Get(ctx, KeySecurityConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SetSecurityConfig sets security configuration (encrypted storage)
func SetSecurityConfig(ctx context.Context, manager *Manager, config *SecurityConfig, updatedBy string) error {
	return manager.Set(ctx, KeySecurityConfig, config,
		WithDescription("System security configuration"),
		WithCategory(CategorySecurity),
		WithEncrypted(true), // Mark as encrypted
		WithUpdatedBy(updatedBy),
		WithRecordHistory(true),
	)
}

// InitDefaultConfigs initializes default configurations
func InitDefaultConfigs(ctx context.Context, manager *Manager) error {
	// Check if configuration exists, create default values if not

	// Default database configuration
	exists, err := manager.Exists(ctx, KeyDatabaseConfig)
	if err != nil {
		return err
	}
	if !exists {
		defaultDB := &DatabaseConfig{
			Host:            "localhost",
			Port:            5432,
			Username:        "postgres",
			Password:        "",
			Database:        "primus_lens",
			MaxConnections:  100,
			ConnMaxLifetime: 3600,
		}
		if err := SetDatabaseConfig(ctx, manager, defaultDB, "system"); err != nil {
			return err
		}
	}

	// Default feature flags
	exists, err = manager.Exists(ctx, KeyFeatureFlags)
	if err != nil {
		return err
	}
	if !exists {
		defaultFlags := &FeatureFlags{
			EnableNewUI:        false,
			EnableBetaFeature:  false,
			EnableDebugMode:    false,
			MaxUploadSize:      10 * 1024 * 1024, // 10MB
			MaxConcurrentUsers: 1000,
			APIRateLimit:       100,
			LogLevel:           "info",
		}
		if err := SetFeatureFlags(ctx, manager, defaultFlags, "system"); err != nil {
			return err
		}
	}

	// Default cache configuration
	exists, err = manager.Exists(ctx, KeyCacheConfig)
	if err != nil {
		return err
	}
	if !exists {
		defaultCache := &CacheConfig{
			Enabled:     true,
			Type:        "memory",
			Host:        "localhost",
			Port:        6379,
			Password:    "",
			DB:          0,
			MaxRetries:  3,
			PoolSize:    10,
			IdleTimeout: 300,
		}
		if err := manager.Set(ctx, KeyCacheConfig, defaultCache,
			WithDescription("Cache configuration"),
			WithCategory(CategoryCache),
			WithCreatedBy("system"),
		); err != nil {
			return err
		}
	}

	return nil
}
