// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"gorm.io/gorm"

	log "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// System config keys (matching Lens API constants)
	ConfigKeyAuthMode               = "auth.mode"
	ConfigKeyAuthInitialized        = "auth.initialized"
	ConfigKeySafeIntegrationEnabled = "safe.integration.enabled"
	ConfigKeySafeAdapterURL         = "safe.adapter_url"
	ConfigKeySafeSSOURL             = "safe.sso_url"

	// AuthModeSaFE is the safe authentication mode
	AuthModeSaFE = "safe"

	// Environment variables (only used as fallback)
	EnvAutoRegister = "AUTO_REGISTER_SAFE" // Set to "false" to disable auto-registration
)

// LensSystemConfig represents the lens_system_configs table
type LensSystemConfig struct {
	Key         string                 `gorm:"column:key;primaryKey"`
	Value       map[string]interface{} `gorm:"column:value;serializer:json;not null"`
	Description string                 `gorm:"column:description"`
	Category    string                 `gorm:"column:category;not null"`
	IsSecret    bool                   `gorm:"column:is_secret;not null"`
	CreatedAt   time.Time              `gorm:"column:created_at;not null;default:now()"`
	UpdatedAt   time.Time              `gorm:"column:updated_at;not null;default:now()"`
}

func (LensSystemConfig) TableName() string {
	return "lens_system_configs"
}

// AutoRegisterService handles automatic registration of primus-safe-adapter with Lens
// It reads configuration from Lens DB and enables safe mode if properly configured
type AutoRegisterService struct {
	lensDB *gorm.DB
}

// NewAutoRegisterService creates a new AutoRegisterService
func NewAutoRegisterService(lensDB *gorm.DB) *AutoRegisterService {
	return &AutoRegisterService{
		lensDB: lensDB,
	}
}

// Register performs the auto-registration of safe adapter with Lens
// This reads configuration from DB and enables safe mode if adapter_url and sso_url are configured
// Configuration should be set via:
//  1. POST /api/v1/init/setup with safeConfig
//  2. PUT /api/v1/admin/configs/:key API
func (s *AutoRegisterService) Register(ctx context.Context) error {
	// Check if auto-registration is enabled
	if !s.isAutoRegisterEnabled() {
		log.Info("Auto-registration disabled via environment variable, skipping")
		return nil
	}

	if s.lensDB == nil {
		log.Warn("Lens DB not available, skipping auto-registration")
		return nil
	}

	log.Info("Checking safe mode configuration from database...")

	// 1. Check if adapter URL is configured in DB
	adapterURL, err := s.getConfigString(ctx, ConfigKeySafeAdapterURL)
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Warnf("Failed to read adapter URL from DB: %v", err)
	}

	if adapterURL == "" {
		log.Info("safe.adapter_url not configured in database, safe mode activation skipped")
		log.Info("To enable safe mode, configure safe.adapter_url via init API or admin config API")
		return nil
	}

	log.Infof("Found adapter URL in database: %s", adapterURL)

	// 2. Check if SSO URL is configured in DB
	ssoURL, err := s.getConfigString(ctx, ConfigKeySafeSSOURL)
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Warnf("Failed to read SSO URL from DB: %v", err)
	}

	if ssoURL != "" {
		log.Infof("Found SSO URL in database: %s", ssoURL)
	} else {
		log.Info("safe.sso_url not configured in database (optional)")
	}

	// 3. Check current auth mode
	currentMode, err := s.getConfigString(ctx, ConfigKeyAuthMode)
	if err != nil && err != gorm.ErrRecordNotFound {
		log.Warnf("Failed to get current auth mode: %v", err)
	}

	// 4. If adapter URL is configured, enable safe mode
	if currentMode == "" || currentMode == "none" {
		if err := s.setConfig(ctx, ConfigKeyAuthMode, AuthModeSaFE, "auth", "Authentication mode"); err != nil {
			return fmt.Errorf("failed to set auth mode: %w", err)
		}
		log.Info("Set auth mode to 'safe'")
	} else if currentMode == AuthModeSaFE {
		log.Info("Auth mode already set to 'safe'")
	} else {
		log.Infof("Auth mode is '%s', not changing to safe mode", currentMode)
		return nil
	}

	// 5. Enable safe integration
	if err := s.setConfigBool(ctx, ConfigKeySafeIntegrationEnabled, true, "auth", "SaFE integration enabled"); err != nil {
		return fmt.Errorf("failed to enable safe integration: %w", err)
	}

	// 6. Mark auth as initialized
	if err := s.setConfigBool(ctx, ConfigKeyAuthInitialized, true, "auth", "Auth initialized"); err != nil {
		return fmt.Errorf("failed to mark auth as initialized: %w", err)
	}

	log.Info("Safe mode activation completed successfully")
	return nil
}

// isAutoRegisterEnabled checks if auto-registration is enabled
func (s *AutoRegisterService) isAutoRegisterEnabled() bool {
	// Default to true if not explicitly disabled
	val := os.Getenv(EnvAutoRegister)
	if val == "false" || val == "0" || val == "no" {
		return false
	}
	return true // Enable by default
}

// setConfig sets a string config value
func (s *AutoRegisterService) setConfig(ctx context.Context, key, value, category, description string) error {
	now := time.Now()
	config := &LensSystemConfig{
		Key:         key,
		Value:       map[string]interface{}{"value": value},
		Category:    category,
		Description: description,
		UpdatedAt:   now,
	}

	return s.lensDB.WithContext(ctx).
		Where("key = ?", key).
		Assign(map[string]interface{}{
			"value":       config.Value,
			"category":    category,
			"description": description,
			"updated_at":  now,
		}).
		FirstOrCreate(config).Error
}

// setConfigBool sets a boolean config value
func (s *AutoRegisterService) setConfigBool(ctx context.Context, key string, value bool, category, description string) error {
	now := time.Now()
	config := &LensSystemConfig{
		Key:         key,
		Value:       map[string]interface{}{"value": value},
		Category:    category,
		Description: description,
		UpdatedAt:   now,
	}

	return s.lensDB.WithContext(ctx).
		Where("key = ?", key).
		Assign(map[string]interface{}{
			"value":       config.Value,
			"category":    category,
			"description": description,
			"updated_at":  now,
		}).
		FirstOrCreate(config).Error
}

// getConfigString gets a string config value
func (s *AutoRegisterService) getConfigString(ctx context.Context, key string) (string, error) {
	var config LensSystemConfig
	err := s.lensDB.WithContext(ctx).Where("key = ?", key).First(&config).Error
	if err != nil {
		return "", err
	}

	if val, ok := config.Value["value"]; ok {
		if str, ok := val.(string); ok {
			return str, nil
		}
	}
	return "", nil
}
