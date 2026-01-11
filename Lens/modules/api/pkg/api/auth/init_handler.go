// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"context"
	"net/http"
	"time"

	cpauth "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetInitStatus returns the current initialization status
// GET /api/v1/init/status
// This endpoint does not require authentication
func GetInitStatus(c *gin.Context) {
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	status := &InitStatusResponse{}

	// Check system.initialized
	systemInit, err := getConfigBool(ctx, facade, cpauth.ConfigKeySystemInitialized)
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	status.Initialized = systemInit

	// Get current auth mode
	authMode, _ := getConfigString(ctx, facade, cpauth.ConfigKeyAuthMode)
	if authMode != "" {
		status.AuthMode = cpauth.AuthMode(authMode)
	} else {
		status.AuthMode = cpauth.AuthModeNone
	}

	// Check if root user exists
	rootUser, err := facade.GetUser().GetByUsername(ctx, cpauth.RootUsername)
	status.RootUserExists = err == nil && rootUser != nil && rootUser.ID != ""

	// Check if safe adapter URL is configured (indicates SaFE is available)
	adapterURL, _ := getConfigString(ctx, facade, cpauth.ConfigKeySafeAdapterURL)
	status.SafeDetected = adapterURL != ""

	// Suggest mode based on detection
	if status.SafeDetected {
		status.SuggestedMode = cpauth.AuthModeSaFE
	} else {
		status.SuggestedMode = cpauth.AuthModeNone
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, status))
}

// SetupInit performs the initial system setup
// POST /api/v1/init/setup
// This endpoint is only available when the system is not initialized
func SetupInit(c *gin.Context) {
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Check if already initialized
	systemInit, err := getConfigBool(ctx, facade, cpauth.ConfigKeySystemInitialized)
	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if systemInit {
		c.JSON(http.StatusBadRequest, gin.H{"error": "system is already initialized"})
		return
	}

	var req InitSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create root user
	rootPassword, generated, err := createRootUser(ctx, facade, req.RootPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Determine auth mode
	authMode := req.AuthMode
	if authMode == "" {
		authMode = cpauth.AuthModeNone
	}

	// Handle SafeConfig if provided
	if req.SafeConfig != nil && req.SafeConfig.Enabled {
		// Set auth mode to safe
		authMode = cpauth.AuthModeSaFE

		// Save adapter URL if provided
		if req.SafeConfig.AdapterURL != "" {
			if err := setConfigString(ctx, facade, cpauth.ConfigKeySafeAdapterURL, req.SafeConfig.AdapterURL, "safe"); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save adapter URL"})
				return
			}
			log.Infof("SaFE adapter URL configured: %s", req.SafeConfig.AdapterURL)
		}

		// Save SSO URL if provided
		if req.SafeConfig.SSOURL != "" {
			if err := setConfigString(ctx, facade, cpauth.ConfigKeySafeSSOURL, req.SafeConfig.SSOURL, "safe"); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save SSO URL"})
				return
			}
			log.Infof("SaFE SSO URL configured: %s", req.SafeConfig.SSOURL)
		}

		// Enable safe integration
		if err := setConfigBool(ctx, facade, cpauth.ConfigKeySafeIntegrationEnabled, true, "auth"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enable safe integration"})
			return
		}
	}

	// Set auth mode
	if err := setConfigString(ctx, facade, cpauth.ConfigKeyAuthMode, string(authMode), "auth"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set auth mode"})
		return
	}

	// Mark auth as initialized
	if err := setConfigBool(ctx, facade, cpauth.ConfigKeyAuthInitialized, true, "auth"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark auth as initialized"})
		return
	}

	// Mark system as initialized
	if err := setConfigBool(ctx, facade, cpauth.ConfigKeySystemInitialized, true, "system"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark system as initialized"})
		return
	}

	log.Infof("System initialized with auth mode: %s", authMode)

	resp := &InitSetupResponse{
		Initialized: true,
		AuthMode:    authMode,
	}

	if generated {
		resp.RootUser = &RootUserInfo{
			Username:          cpauth.RootUsername,
			MustChangePassword: true,
			GeneratedPassword: rootPassword,
		}
		log.Warnf("Root user created with generated password")
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// createRootUser creates the root user if it doesn't exist
func createRootUser(ctx context.Context, facade cpdb.FacadeInterface, password string) (string, bool, error) {
	// Check if root user already exists
	existingUser, err := facade.GetUser().GetByUsername(ctx, cpauth.RootUsername)
	if err == nil && existingUser != nil && existingUser.ID != "" {
		log.Info("Root user already exists, skipping creation")
		return "", false, nil
	}

	// Generate password if not provided
	passwordGenerated := false
	if password == "" {
		var err error
		password, err = cpauth.GenerateRandomPassword()
		if err != nil {
			return "", false, err
		}
		passwordGenerated = true
	}

	// Hash password
	passwordHash, err := cpauth.HashPassword(password)
	if err != nil {
		return "", false, err
	}

	// Create root user
	now := time.Now()
	rootUser := &model.LensUsers{
		ID:                 cpauth.RootUserID,
		Username:           cpauth.RootUsername,
		Email:              cpauth.RootEmail,
		DisplayName:        cpauth.RootDisplayName,
		AuthType:           string(cpauth.AuthTypeLocal),
		Status:             string(cpauth.UserStatusActive),
		IsAdmin:            true,
		IsRoot:             true,
		PasswordHash:       passwordHash,
		MustChangePassword: passwordGenerated,
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	if err := facade.GetUser().Create(ctx, rootUser); err != nil {
		return "", false, err
	}

	log.Info("Root user created successfully")

	if passwordGenerated {
		return password, true, nil
	}
	return "", false, nil
}

// Helper functions for config operations

func getConfigBool(ctx context.Context, facade cpdb.FacadeInterface, key string) (bool, error) {
	config, err := facade.GetSystemConfig().Get(ctx, key)
	if err != nil {
		return false, err
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

func setConfigBool(ctx context.Context, facade cpdb.FacadeInterface, key string, value bool, category string) error {
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     model.ExtType{"value": value},
		Category:  category,
		UpdatedAt: time.Now(),
	}
	return facade.GetSystemConfig().Set(ctx, config)
}

func getConfigString(ctx context.Context, facade cpdb.FacadeInterface, key string) (string, error) {
	config, err := facade.GetSystemConfig().Get(ctx, key)
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

func setConfigString(ctx context.Context, facade cpdb.FacadeInterface, key, value, category string) error {
	config := &model.LensSystemConfigs{
		Key:       key,
		Value:     model.ExtType{"value": value},
		Category:  category,
		UpdatedAt: time.Now(),
	}
	return facade.GetSystemConfig().Set(ctx, config)
}
