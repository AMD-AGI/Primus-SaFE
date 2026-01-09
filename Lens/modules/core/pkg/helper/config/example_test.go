// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config_test

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
)

// Example configuration structures
type DatabaseConfig struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type FeatureFlags struct {
	EnableNewUI       bool `json:"enable_new_ui"`
	EnableBetaFeature bool `json:"enable_beta_feature"`
	MaxUploadSize     int  `json:"max_upload_size"`
}

// ExampleManager_Get demonstrates how to get configuration and parse it to a struct
func ExampleManager_Get() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// Define struct to receive configuration
	var dbConfig DatabaseConfig

	// Get configuration
	err := manager.Get(ctx, "database.config", &dbConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Database Host: %s\n", dbConfig.Host)
	fmt.Printf("Database Port: %d\n", dbConfig.Port)
}

// ExampleManager_Set demonstrates how to set configuration
func ExampleManager_Set() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// Create configuration object
	dbConfig := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Username: "admin",
		Password: "secret",
		Database: "primus_lens",
	}

	// Set configuration
	err := manager.Set(ctx, "database.config", dbConfig,
		config.WithDescription("Database connection configuration"),
		config.WithCategory("database"),
		config.WithCreatedBy("admin"),
		config.WithRecordHistory(true),
	)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Configuration set successfully")
}

// ExampleManager_GetOrDefault demonstrates how to get configuration with default value
func ExampleManager_GetOrDefault() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	var features FeatureFlags

	// Default configuration
	defaultFeatures := FeatureFlags{
		EnableNewUI:       false,
		EnableBetaFeature: false,
		MaxUploadSize:     10485760, // 10MB
	}

	// Get configuration, use default value if not exists
	err := manager.GetOrDefault(ctx, "feature.flags", &features, defaultFeatures)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("New UI Enabled: %v\n", features.EnableNewUI)
	fmt.Printf("Max Upload Size: %d bytes\n", features.MaxUploadSize)
}

// ExampleManager_List demonstrates how to list configurations
func ExampleManager_List() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// List configurations of a specific category
	configs, err := manager.ListByCategory(ctx, "database")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	for _, cfg := range configs {
		fmt.Printf("Key: %s, Description: %s\n", cfg.Key, cfg.Description)
	}
}

// ExampleManager_BatchSet demonstrates batch setting configurations
func ExampleManager_BatchSet() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// Prepare batch configurations
	batchConfigs := []config.BatchConfig{
		{
			Key: "smtp.host",
			Value: map[string]interface{}{
				"host": "smtp.example.com",
				"port": 587,
			},
			Description: "SMTP server configuration",
			Category:    "email",
			CreatedBy:   "admin",
		},
		{
			Key: "smtp.auth",
			Value: map[string]interface{}{
				"username": "noreply@example.com",
				"password": "secret",
			},
			Description: "SMTP authentication",
			Category:    "email",
			IsEncrypted: true,
			CreatedBy:   "admin",
		},
	}

	// Batch set
	err := manager.BatchSet(ctx, batchConfigs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Batch configuration set successfully")
}

// ExampleCachedManager_GetCached demonstrates how to use cached manager
func ExampleCachedManager_GetCached() {
	ctx := context.Background()

	// Create cached manager with 5 minute cache TTL
	cachedManager := config.NewCachedManagerForCluster("default", 5*time.Minute)

	var dbConfig DatabaseConfig

	// First call reads from database
	err := cachedManager.GetCached(ctx, "database.config", &dbConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Second call reads from cache (faster)
	err = cachedManager.GetCached(ctx, "database.config", &dbConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Database configuration loaded: %s:%d\n", dbConfig.Host, dbConfig.Port)
}

// ExampleManager_GetHistory demonstrates how to get configuration history
func ExampleManager_GetHistory() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// Get latest 10 history records
	history, err := manager.GetHistory(ctx, "database.config", 10)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	for _, h := range history {
		fmt.Printf("Version %d changed by %s at %s\n",
			h.Version, h.ChangedBy, h.ChangedAt.Format(time.RFC3339))
	}
}

// ExampleManager_Rollback demonstrates how to rollback configuration
func ExampleManager_Rollback() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// Rollback to version 3
	err := manager.Rollback(ctx, "database.config", 3, "admin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Configuration rolled back successfully")
}

// ExampleCachedManager_Preload demonstrates how to preload configurations to cache
func ExampleCachedManager_Preload() {
	ctx := context.Background()
	cachedManager := config.NewCachedManagerForCluster("default", 5*time.Minute)

	// Preload multiple configurations to cache
	keys := []string{
		"database.config",
		"feature.flags",
		"smtp.host",
		"smtp.auth",
	}

	err := cachedManager.Preload(ctx, keys)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Get cache statistics
	stats := cachedManager.GetCacheStats()
	fmt.Printf("Cache stats: %+v\n", stats)
}
