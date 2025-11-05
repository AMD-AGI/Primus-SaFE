package config_test

import (
	"context"
	"fmt"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/helper/config"
)

// 示例配置结构
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

// ExampleManager_Get 演示如何获取配置并解析到结构体
func ExampleManager_Get() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// 定义接收配置的结构体
	var dbConfig DatabaseConfig

	// 获取配置
	err := manager.Get(ctx, "database.config", &dbConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Database Host: %s\n", dbConfig.Host)
	fmt.Printf("Database Port: %d\n", dbConfig.Port)
}

// ExampleManager_Set 演示如何设置配置
func ExampleManager_Set() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// 创建配置对象
	dbConfig := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Username: "admin",
		Password: "secret",
		Database: "primus_lens",
	}

	// 设置配置
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

// ExampleManager_GetOrDefault 演示如何获取配置并提供默认值
func ExampleManager_GetOrDefault() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	var features FeatureFlags

	// 默认配置
	defaultFeatures := FeatureFlags{
		EnableNewUI:       false,
		EnableBetaFeature: false,
		MaxUploadSize:     10485760, // 10MB
	}

	// 获取配置，如果不存在则使用默认值
	err := manager.GetOrDefault(ctx, "feature.flags", &features, defaultFeatures)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("New UI Enabled: %v\n", features.EnableNewUI)
	fmt.Printf("Max Upload Size: %d bytes\n", features.MaxUploadSize)
}

// ExampleManager_List 演示如何列出配置
func ExampleManager_List() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// 列出特定类别的配置
	configs, err := manager.ListByCategory(ctx, "database")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	for _, cfg := range configs {
		fmt.Printf("Key: %s, Description: %s\n", cfg.Key, cfg.Description)
	}
}

// ExampleManager_BatchSet 演示批量设置配置
func ExampleManager_BatchSet() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// 准备批量配置
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

	// 批量设置
	err := manager.BatchSet(ctx, batchConfigs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Batch configuration set successfully")
}

// ExampleCachedManager_GetCached 演示如何使用缓存管理器
func ExampleCachedManager_GetCached() {
	ctx := context.Background()

	// 创建带缓存的管理器，缓存有效期 5 分钟
	cachedManager := config.NewCachedManagerForCluster("default", 5*time.Minute)

	var dbConfig DatabaseConfig

	// 第一次调用会从数据库读取
	err := cachedManager.GetCached(ctx, "database.config", &dbConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// 第二次调用会从缓存读取（更快）
	err = cachedManager.GetCached(ctx, "database.config", &dbConfig)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Database configuration loaded: %s:%d\n", dbConfig.Host, dbConfig.Port)
}

// ExampleManager_GetHistory 演示如何获取配置历史
func ExampleManager_GetHistory() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// 获取最近 10 条历史记录
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

// ExampleManager_Rollback 演示如何回滚配置
func ExampleManager_Rollback() {
	ctx := context.Background()
	manager := config.NewManagerForCluster("default")

	// 回滚到版本 3
	err := manager.Rollback(ctx, "database.config", 3, "admin")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Configuration rolled back successfully")
}

// ExampleCachedManager_Preload 演示如何预加载配置到缓存
func ExampleCachedManager_Preload() {
	ctx := context.Background()
	cachedManager := config.NewCachedManagerForCluster("default", 5*time.Minute)

	// 预加载多个配置到缓存
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

	// 获取缓存统计
	stats := cachedManager.GetCacheStats()
	fmt.Printf("Cache stats: %+v\n", stats)
}
