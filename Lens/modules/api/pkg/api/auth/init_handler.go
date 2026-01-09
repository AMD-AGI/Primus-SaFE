// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"net/http"

	cpauth "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

var (
	initializer  *cpauth.Initializer
	safeDetector *cpauth.SafeDetector
)

// InitializeAuthHandlers initializes the auth handlers with dependencies
func InitializeAuthHandlers(init *cpauth.Initializer, detector *cpauth.SafeDetector) {
	initializer = init
	safeDetector = detector
}

// GetInitStatus returns the current initialization status
// GET /api/v1/init/status
// This endpoint does not require authentication
func GetInitStatus(c *gin.Context) {
	if initializer == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "initializer not configured"})
		return
	}

	status, err := initializer.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := &InitStatusResponse{
		Initialized:    status.SystemInitialized,
		AuthMode:       status.AuthMode,
		SafeDetected:   status.SafeDetected,
		SuggestedMode:  status.SuggestedMode,
		RootUserExists: status.RootUserExists,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}

// SetupInit performs the initial system setup
// POST /api/v1/init/setup
// This endpoint is only available when the system is not initialized
func SetupInit(c *gin.Context) {
	if initializer == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "initializer not configured"})
		return
	}

	// Check if already initialized
	status, err := initializer.GetStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if status.SystemInitialized {
		c.JSON(http.StatusBadRequest, gin.H{"error": "system is already initialized"})
		return
	}

	var req InitSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := initializer.Initialize(c.Request.Context(), &cpauth.InitializeOptions{
		AuthMode:     req.AuthMode,
		RootPassword: req.RootPassword,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := &InitSetupResponse{
		Initialized: result.Success,
		AuthMode:    result.AuthMode,
	}

	if result.Success {
		resp.RootUser = &RootUserInfo{
			Username:           cpauth.RootUsername,
			MustChangePassword: result.RootPasswordGenerated,
		}
		if result.RootPasswordGenerated {
			resp.RootUser.GeneratedPassword = result.RootPassword
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, resp))
}
