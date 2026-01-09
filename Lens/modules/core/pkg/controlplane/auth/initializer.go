// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"encoding/json"
	"os"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gorm.io/gorm"
)

// Initializer handles system initialization
type Initializer struct {
	facade       cpdb.FacadeInterface
	safeDetector *SafeDetector
}

// InitializationStatus represents the current initialization status
type InitializationStatus struct {
	SystemInitialized bool     `json:"systemInitialized"`
	AuthInitialized   bool     `json:"authInitialized"`
	AuthMode          AuthMode `json:"authMode"`
	SafeDetected      bool     `json:"safeDetected"`
	SuggestedMode     AuthMode `json:"suggestedMode"`
	RootUserExists    bool     `json:"rootUserExists"`
}

// InitializationResult represents the result of initialization
type InitializationResult struct {
	Success             bool     `json:"success"`
	AuthMode            AuthMode `json:"authMode"`
	RootPasswordGenerated bool   `json:"rootPasswordGenerated"`
	RootPassword        string   `json:"rootPassword,omitempty"` // Only returned if generated
	Message             string   `json:"message"`
}

// NewInitializer creates a new Initializer
func NewInitializer(safeDetector *SafeDetector) *Initializer {
	return &Initializer{
		facade:       cpdb.GetFacade(),
		safeDetector: safeDetector,
	}
}

// GetStatus returns the current initialization status
func (i *Initializer) GetStatus(ctx context.Context) (*InitializationStatus, error) {
	status := &InitializationStatus{}

	// Check system.initialized
	systemInit, err := i.getConfigBool(ctx, ConfigKeySystemInitialized)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	status.SystemInitialized = systemInit

	// Check auth.initialized
	authInit, err := i.getConfigBool(ctx, ConfigKeyAuthInitialized)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	status.AuthInitialized = authInit

	// Get current auth mode
	authMode, err := i.getConfigString(ctx, ConfigKeyAuthMode)
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	if authMode != "" {
		status.AuthMode = AuthMode(authMode)
	} else {
		status.AuthMode = AuthModeNone
	}

	// Check if root user exists
	rootUser, err := i.facade.GetUser().GetByUsername(ctx, RootUsername)
	status.RootUserExists = err == nil && rootUser != nil

	// Detect SaFE environment
	if i.safeDetector != nil {
		result, err := i.safeDetector.DetectSaFE(ctx)
		if err == nil {
			status.SafeDetected = result.ShouldEnableSafeMode
		}
	}

	// Suggest mode based on detection
	if status.SafeDetected {
		status.SuggestedMode = AuthModeSaFE
	} else {
		status.SuggestedMode = AuthModeNone
	}

	return status, nil
}

// Initialize performs the initial system setup
func (i *Initializer) Initialize(ctx context.Context, opts *InitializeOptions) (*InitializationResult, error) {
	result := &InitializationResult{}

	// Check if already initialized
	status, err := i.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	if status.SystemInitialized {
		result.Success = false
		result.Message = "System is already initialized"
		return result, nil
	}

	// Create root user
	rootPassword, generated, err := i.createRootUser(ctx, opts.RootPassword)
	if err != nil {
		return nil, err
	}

	result.RootPasswordGenerated = generated
	if generated {
		result.RootPassword = rootPassword
		log.Warnf("Root user created with generated password: %s", rootPassword)
		log.Warn("Please change the root password on first login!")
	}

	// Determine auth mode
	authMode := opts.AuthMode
	if authMode == "" {
		if status.SafeDetected {
			authMode = AuthModeSaFE
		} else {
			authMode = AuthModeNone
		}
	}

	// Set auth mode
	if err := i.setConfigString(ctx, ConfigKeyAuthMode, string(authMode), "auth"); err != nil {
		return nil, err
	}

	// Mark system as initialized
	if err := i.setConfigBool(ctx, ConfigKeySystemInitialized, true, "system"); err != nil {
		return nil, err
	}

	// Mark auth as initialized
	if err := i.setConfigBool(ctx, ConfigKeyAuthInitialized, true, "auth"); err != nil {
		return nil, err
	}

	// If SaFE detected and mode is safe, enable integration
	if authMode == AuthModeSaFE && status.SafeDetected {
		if err := i.setConfigBool(ctx, ConfigKeySafeIntegrationEnabled, true, "auth"); err != nil {
			return nil, err
		}
		if err := i.setConfigBool(ctx, ConfigKeySafeIntegrationAutoDetected, true, "auth"); err != nil {
			return nil, err
		}
	}

	result.Success = true
	result.AuthMode = authMode
	result.Message = "System initialized successfully"

	log.Infof("System initialized with auth mode: %s", authMode)

	return result, nil
}

// InitializeOptions contains options for initialization
type InitializeOptions struct {
	// AuthMode to set (if empty, will be auto-detected)
	AuthMode AuthMode `json:"authMode"`
	// RootPassword to set (if empty, will be generated)
	RootPassword string `json:"rootPassword"`
}

// createRootUser creates the root user if it doesn't exist
func (i *Initializer) createRootUser(ctx context.Context, password string) (string, bool, error) {
	// Check if root user already exists
	existingUser, err := i.facade.GetUser().GetByUsername(ctx, RootUsername)
	if err == nil && existingUser != nil {
		return "", false, nil // Root user already exists
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return "", false, err
	}

	// Get password from options, environment, or generate
	passwordGenerated := false
	if password == "" {
		password = os.Getenv(EnvRootPassword)
	}
	if password == "" {
		var err error
		password, err = GenerateRandomPassword()
		if err != nil {
			return "", false, err
		}
		passwordGenerated = true
	}

	// Hash password
	passwordHash, err := HashPassword(password)
	if err != nil {
		return "", false, err
	}

	// Create root user
	now := time.Now()
	rootUser := &model.LensUsers{
		ID:                 RootUserID,
		Username:           RootUsername,
		Email:              RootEmail,
		DisplayName:        RootDisplayName,
		AuthType:           string(AuthTypeLocal),
		Status:             string(UserStatusActive),
		IsAdmin:            true,
		IsRoot:             true,
		PasswordHash:       passwordHash,
		MustChangePassword: passwordGenerated, // Force password change if generated
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := i.facade.GetUser().Create(ctx, rootUser); err != nil {
		return "", false, err
	}

	log.Info("Root user created successfully")

	if passwordGenerated {
		return password, true, nil
	}
	return "", false, nil
}

// Helper methods for config operations

func (i *Initializer) getConfigBool(ctx context.Context, key string) (bool, error) {
	config, err := i.facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		return false, err
	}

	// Parse the value from ExtType
	if val, ok := config.Value["value"]; ok {
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			return v == "true", nil
		}
	}

	// Try direct bool value
	if val, ok := config.Value[""].(bool); ok {
		return val, nil
	}

	return false, nil
}

func (i *Initializer) setConfigBool(ctx context.Context, key string, value bool, category string) error {
	valueJSON, _ := json.Marshal(value)
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     model.ExtType{"value": value},
		Category:  category,
		UpdatedAt: time.Now(),
	}

	_ = valueJSON // unused but for clarity
	return i.facade.GetSystemConfig().Set(ctx, config)
}

func (i *Initializer) getConfigString(ctx context.Context, key string) (string, error) {
	config, err := i.facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		return "", err
	}

	// Parse the value from ExtType
	if val, ok := config.Value["value"]; ok {
		if str, ok := val.(string); ok {
			return str, nil
		}
	}

	return "", nil
}

func (i *Initializer) setConfigString(ctx context.Context, key string, value string, category string) error {
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     model.ExtType{"value": value},
		Category:  category,
		UpdatedAt: time.Now(),
	}

	return i.facade.GetSystemConfig().Set(ctx, config)
}

// EnsureInitialized ensures the system is initialized
// This should be called during application startup
func (i *Initializer) EnsureInitialized(ctx context.Context) error {
	status, err := i.GetStatus(ctx)
	if err != nil {
		return err
	}

	if status.SystemInitialized {
		log.Info("System already initialized")
		return nil
	}

	// Auto-initialize with defaults
	log.Info("System not initialized, performing auto-initialization...")

	result, err := i.Initialize(ctx, &InitializeOptions{})
	if err != nil {
		return err
	}

	if result.RootPasswordGenerated {
		log.Warnf("=======================================================")
		log.Warnf("ROOT USER CREATED WITH GENERATED PASSWORD")
		log.Warnf("Username: %s", RootUsername)
		log.Warnf("Password: %s", result.RootPassword)
		log.Warnf("Please change this password on first login!")
		log.Warnf("=======================================================")
	}

	return nil
}
