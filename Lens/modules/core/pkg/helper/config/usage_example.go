package config

// 这个文件提供了一些常见配置使用的便捷示例

import (
	"context"
)

// Example configurations structures
// 示例配置结构体

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	Database        string `json:"database"`
	MaxConnections  int    `json:"max_connections"`
	ConnMaxLifetime int    `json:"conn_max_lifetime"`
}

// SMTPConfig SMTP 配置
type SMTPConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	FromEmail  string `json:"from_email"`
	FromName   string `json:"from_name"`
	EnableTLS  bool   `json:"enable_tls"`
	EnableAuth bool   `json:"enable_auth"`
}

// FeatureFlags 功能开关配置
type FeatureFlags struct {
	EnableNewUI        bool   `json:"enable_new_ui"`
	EnableBetaFeature  bool   `json:"enable_beta_feature"`
	EnableDebugMode    bool   `json:"enable_debug_mode"`
	MaxUploadSize      int64  `json:"max_upload_size"`
	MaxConcurrentUsers int    `json:"max_concurrent_users"`
	APIRateLimit       int    `json:"api_rate_limit"`
	LogLevel           string `json:"log_level"`
}

// CacheConfig 缓存配置
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

// SecurityConfig 安全配置
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
// 预定义配置键常量
const (
	KeyDatabaseConfig = "system.database.config"
	KeySMTPConfig     = "system.smtp.config"
	KeyFeatureFlags   = "system.feature.flags"
	KeyCacheConfig    = "system.cache.config"
	KeySecurityConfig = "system.security.config"
)

// Configuration categories
// 配置分类常量
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
// 常用操作的工具函数

// GetDatabaseConfig 获取数据库配置
func GetDatabaseConfig(ctx context.Context, manager *Manager) (*DatabaseConfig, error) {
	var config DatabaseConfig
	err := manager.Get(ctx, KeyDatabaseConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SetDatabaseConfig 设置数据库配置
func SetDatabaseConfig(ctx context.Context, manager *Manager, config *DatabaseConfig, updatedBy string) error {
	return manager.Set(ctx, KeyDatabaseConfig, config,
		WithDescription("数据库连接配置"),
		WithCategory(CategoryDatabase),
		WithUpdatedBy(updatedBy),
		WithRecordHistory(true),
	)
}

// GetFeatureFlags 获取功能开关配置
func GetFeatureFlags(ctx context.Context, manager *Manager) (*FeatureFlags, error) {
	var flags FeatureFlags
	err := manager.Get(ctx, KeyFeatureFlags, &flags)
	if err != nil {
		return nil, err
	}
	return &flags, nil
}

// SetFeatureFlags 设置功能开关配置
func SetFeatureFlags(ctx context.Context, manager *Manager, flags *FeatureFlags, updatedBy string) error {
	return manager.Set(ctx, KeyFeatureFlags, flags,
		WithDescription("系统功能开关配置"),
		WithCategory(CategoryFeature),
		WithUpdatedBy(updatedBy),
		WithRecordHistory(true),
	)
}

// GetSecurityConfig 获取安全配置
func GetSecurityConfig(ctx context.Context, manager *Manager) (*SecurityConfig, error) {
	var config SecurityConfig
	err := manager.Get(ctx, KeySecurityConfig, &config)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// SetSecurityConfig 设置安全配置（加密存储）
func SetSecurityConfig(ctx context.Context, manager *Manager, config *SecurityConfig, updatedBy string) error {
	return manager.Set(ctx, KeySecurityConfig, config,
		WithDescription("系统安全配置"),
		WithCategory(CategorySecurity),
		WithEncrypted(true), // 标记为加密
		WithUpdatedBy(updatedBy),
		WithRecordHistory(true),
	)
}

// InitDefaultConfigs 初始化默认配置
func InitDefaultConfigs(ctx context.Context, manager *Manager) error {
	// 检查配置是否已存在，不存在则创建默认值

	// 默认数据库配置
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

	// 默认功能开关
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

	// 默认缓存配置
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
			WithDescription("缓存配置"),
			WithCategory(CategoryCache),
			WithCreatedBy("system"),
		); err != nil {
			return err
		}
	}

	return nil
}
