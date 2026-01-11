// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package oidc

import (
	"github.com/gin-gonic/gin"
)

// RegisterRouter registers OIDC authentication routes
func RegisterRouter(group *gin.RouterGroup) {
	handlers := NewHandlers()

	// Auth routes (no authentication required)
	auth := group.Group("/auth")
	{
		// ============================================
		// Frontend Entry Point - Start Here
		// ============================================
		// Get auth status, providers, and config
		// This is the main entry point for frontend to determine auth flow
		auth.GET("/status", handlers.GetAuthStatus)

		// List available providers (legacy, use /status instead)
		auth.GET("/providers", handlers.ListProviders)

		// ============================================
		// OAuth/OIDC Flow Endpoints
		// ============================================
		// Initiate authorization flow
		auth.GET("/authorize", handlers.Authorize)
		// Handle authorization callback
		auth.GET("/callback", handlers.Callback)

		// ============================================
		// Form-based Login (for local/LDAP)
		// ============================================
		auth.POST("/login", handlers.Login)

		// ============================================
		// Token Validation & Session
		// ============================================
		// Validate SaFE token (for middleware)
		auth.GET("/validate", handlers.ValidateSafeToken)

		// Logout
		auth.POST("/logout", handlers.Logout)
	}
}
