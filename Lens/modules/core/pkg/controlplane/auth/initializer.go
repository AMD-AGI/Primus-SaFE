// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"time"

	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Initializer handles system initialization
type Initializer struct {
	safeDetector *SafeDetector
	k8sClient    client.Client
	facade       cpdb.FacadeInterface
}

// NewInitializer creates a new Initializer without K8s client
func NewInitializer(safeDetector *SafeDetector) *Initializer {
	return &Initializer{
		safeDetector: safeDetector,
		facade:       cpdb.GetFacade(),
	}
}

// NewInitializerWithK8s creates a new Initializer with K8s client
func NewInitializerWithK8s(safeDetector *SafeDetector, k8sClient client.Client) *Initializer {
	return &Initializer{
		safeDetector: safeDetector,
		k8sClient:    k8sClient,
		facade:       cpdb.GetFacade(),
	}
}

// SafeSetupConfig contains SaFE specific setup configurations
type SafeSetupConfig struct {
	Enabled    bool   `json:"enabled"`
	AdapterURL string `json:"adapterUrl,omitempty"`
	SSOURL     string `json:"ssoUrl,omitempty"`
}

// InitializeOptions contains options for system initialization
type InitializeOptions struct {
	AuthMode     AuthMode         `json:"authMode"`
	RootPassword string           `json:"rootPassword"`
	SafeConfig   *SafeSetupConfig `json:"safeConfig,omitempty"`
}

// InitializeResult contains result of initialization
type InitializeResult struct {
	Success    bool   `json:"success"`
	AuthMode   string `json:"authMode"`
	RootUserID string `json:"rootUserId,omitempty"`
	Message    string `json:"message,omitempty"`
}

// EnsureInitialized ensures the system is properly initialized
func (i *Initializer) EnsureInitialized(ctx context.Context) error {
	// Check if already initialized
	initialized, err := i.IsInitialized(ctx)
	if err != nil {
		return err
	}

	if initialized {
		log.Info("System already initialized")
		return nil
	}

	// Auto-detect SaFE if available
	if i.safeDetector != nil {
		result, err := i.safeDetector.DetectSaFE(ctx)
		if err != nil {
			log.Warnf("Failed to detect SaFE: %v", err)
		} else if result.ShouldEnableSafeMode {
			log.Info("SaFE detected, auto-enabling safe mode")
			if err := i.setConfigBool(ctx, ConfigKeySafeIntegrationAutoDetected, true, "auth"); err != nil {
				log.Warnf("Failed to set safe integration auto-detected flag: %v", err)
			}
		}
	}

	return nil
}

// IsInitialized checks if the system is initialized
func (i *Initializer) IsInitialized(ctx context.Context) (bool, error) {
	config, err := i.facade.GetSystemConfig().Get(ctx, ConfigKeyAuthInitialized)
	if err != nil {
		// Not found means not initialized
		return false, nil
	}

	if val, ok := config.Value["value"]; ok {
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			return v == "true", nil
		}
	}

	return false, nil
}

// Initialize performs system initialization
func (i *Initializer) Initialize(ctx context.Context, opts *InitializeOptions) (*InitializeResult, error) {
	result := &InitializeResult{}

	// Set auth mode
	if err := i.setConfigString(ctx, ConfigKeyAuthMode, string(opts.AuthMode), "auth"); err != nil {
		return nil, err
	}
	result.AuthMode = string(opts.AuthMode)

	// Handle SaFE configuration
	if opts.SafeConfig != nil && opts.SafeConfig.Enabled {
		if err := i.setConfigBool(ctx, ConfigKeySafeIntegrationEnabled, true, "auth"); err != nil {
			return nil, err
		}
		if opts.SafeConfig.AdapterURL != "" {
			if err := i.setConfigString(ctx, ConfigKeySafeAdapterURL, opts.SafeConfig.AdapterURL, "safe"); err != nil {
				return nil, err
			}
		}
		if opts.SafeConfig.SSOURL != "" {
			if err := i.setConfigString(ctx, ConfigKeySafeSSOURL, opts.SafeConfig.SSOURL, "safe"); err != nil {
				return nil, err
			}
		}
	}

	// Create root user if password provided
	if opts.RootPassword != "" {
		rootUser, err := i.createRootUser(ctx, opts.RootPassword)
		if err != nil {
			log.Warnf("Failed to create root user: %v", err)
		} else if rootUser != nil {
			result.RootUserID = rootUser.ID
		}
	}

	// Mark as initialized
	if err := i.setConfigBool(ctx, ConfigKeyAuthInitialized, true, "auth"); err != nil {
		return nil, err
	}
	if err := i.setConfigBool(ctx, ConfigKeySystemInitialized, true, "system"); err != nil {
		return nil, err
	}

	result.Success = true
	result.Message = "System initialized successfully"
	return result, nil
}

// GetAuthMode returns current auth mode
func (i *Initializer) GetAuthMode(ctx context.Context) (AuthMode, error) {
	config, err := i.facade.GetSystemConfig().Get(ctx, ConfigKeyAuthMode)
	if err != nil {
		return AuthModeNone, nil
	}

	if val, ok := config.Value["value"]; ok {
		if str, ok := val.(string); ok {
			return AuthMode(str), nil
		}
	}

	return AuthModeNone, nil
}

// createRootUser creates the root user
func (i *Initializer) createRootUser(ctx context.Context, password string) (*model.LensUsers, error) {
	// Hash password
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	rootUser := &model.LensUsers{
		ID:           RootUserID,
		Username:     RootUsername,
		DisplayName:  RootDisplayName,
		Email:        RootEmail,
		PasswordHash: hashedPassword,
		AuthType:     string(AuthTypeLocal),
		Status:       string(UserStatusActive),
		IsAdmin:      true,
		IsRoot:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := i.facade.GetUser().Create(ctx, rootUser); err != nil {
		return nil, err
	}

	return rootUser, nil
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

func (i *Initializer) setConfigBool(ctx context.Context, key string, value bool, category string) error {
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     model.ExtType{"value": value},
		Category:  category,
		UpdatedAt: time.Now(),
	}
	return i.facade.GetSystemConfig().Set(ctx, config)
}
