// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"net/http"
	"time"

	cpauth "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
	cpdb "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetAuthMode returns the current authentication mode
// GET /api/v1/admin/auth/mode
func GetAuthMode(c *gin.Context) {
	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Get current auth mode
	authModeConfig, err := facade.GetSystemConfig().Get(ctx, cpauth.ConfigKeyAuthMode)
	var authMode cpauth.AuthMode = cpauth.AuthModeNone
	if err == nil && authModeConfig != nil {
		if val, ok := authModeConfig.Value["value"]; ok {
			if str, ok := val.(string); ok {
				authMode = cpauth.AuthMode(str)
			}
		}
	}

	// Check if initialized
	initConfig, _ := facade.GetSystemConfig().Get(ctx, cpauth.ConfigKeyAuthInitialized)
	initialized := false
	if initConfig != nil {
		if val, ok := initConfig.Value["value"]; ok {
			if b, ok := val.(bool); ok {
				initialized = b
			}
		}
	}

	resp := &AuthModeResponse{
		Mode:           authMode,
		Initialized:    initialized,
		AvailableModes: []cpauth.AuthMode{
			cpauth.AuthModeNone,
			cpauth.AuthModeLDAP,
			cpauth.AuthModeSSO,
			cpauth.AuthModeSaFE,
		},
	}

	// Get SaFE integration info
	safeEnabled, _ := facade.GetSystemConfig().Get(ctx, cpauth.ConfigKeySafeIntegrationEnabled)
	safeAutoDetected, _ := facade.GetSystemConfig().Get(ctx, cpauth.ConfigKeySafeIntegrationAutoDetected)

	if safeEnabled != nil || safeAutoDetected != nil {
		resp.SafeIntegration = &SafeIntegrationInfo{}
		if safeEnabled != nil {
			if val, ok := safeEnabled.Value["value"]; ok {
				if b, ok := val.(bool); ok {
					resp.SafeIntegration.Enabled = b
				}
			}
		}
		if safeAutoDetected != nil {
			if val, ok := safeAutoDetected.Value["value"]; ok {
				if b, ok := val.(bool); ok {
					resp.SafeIntegration.AutoDetected = b
				}
			}
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// SetAuthMode sets the authentication mode
// PUT /api/v1/admin/auth/mode
func SetAuthMode(c *gin.Context) {
	var req SetAuthModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Validate auth mode
	validModes := map[cpauth.AuthMode]bool{
		cpauth.AuthModeNone: true,
		cpauth.AuthModeLDAP: true,
		cpauth.AuthModeSSO:  true,
		cpauth.AuthModeSaFE: true,
	}
	if !validModes[req.Mode] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth mode"})
		return
	}

	// Update auth mode
	now := time.Now()
	config := &model.LensSystemConfigs{
		Key:       cpauth.ConfigKeyAuthMode,
		Value:     model.ExtType{"value": string(req.Mode)},
		Category:  "auth",
		UpdatedAt: now,
	}

	if err := facade.GetSystemConfig().Set(ctx, config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	message := "Authentication mode changed"
	switch req.Mode {
	case cpauth.AuthModeLDAP:
		message = "Authentication mode changed to LDAP. Please configure LDAP provider."
	case cpauth.AuthModeSSO:
		message = "Authentication mode changed to SSO. Please configure SSO provider."
	case cpauth.AuthModeSaFE:
		message = "Authentication mode changed to SaFE integration."
	case cpauth.AuthModeNone:
		message = "Authentication disabled. All requests will be allowed."
	}

	resp := &SetAuthModeResponse{
		Mode:    req.Mode,
		Message: message,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// ChangeRootPassword changes the root user's password
// POST /api/v1/admin/root/change-password
func ChangeRootPassword(c *gin.Context) {
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()
	facade := cpdb.GetFacade()

	// Get root user
	rootUser, err := facade.GetUser().GetRootUser(ctx)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "root user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Verify current password
	if !cpauth.VerifyPassword(req.CurrentPassword, rootUser.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	// Hash new password
	newHash, err := cpauth.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	// Update password
	rootUser.PasswordHash = newHash
	rootUser.MustChangePassword = false
	rootUser.UpdatedAt = time.Now()

	if err := facade.GetUser().Update(ctx, rootUser); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{"message": "password changed successfully"}))
}
