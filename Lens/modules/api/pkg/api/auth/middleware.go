// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/session"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// SessionAuthMiddleware creates a middleware that validates session tokens
// This middleware is specifically for the new auth system and does NOT affect existing APIs
func SessionAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get token from cookie first
		token, err := c.Cookie(SessionCookieName)
		if err != nil || token == "" {
			// Try Authorization header as fallback
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Authentication required",
				"code":    "AUTH_REQUIRED",
			})
			return
		}

		// Validate session
		sessionMgr := session.GetManager()
		sessionInfo, err := sessionMgr.Validate(c.Request.Context(), token)
		if err != nil {
			log.Debugf("Session validation failed: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid or expired session",
				"code":    "SESSION_INVALID",
			})
			return
		}

		// Store session info in context for handlers
		c.Set("session", sessionInfo)
		c.Set("user_id", sessionInfo.UserID)
		c.Set("username", sessionInfo.Username)
		c.Set("is_admin", sessionInfo.IsAdmin)

		c.Next()
	}
}

// AdminAuthMiddleware creates a middleware that requires admin privileges
// Must be used after SessionAuthMiddleware
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if session exists (set by SessionAuthMiddleware)
		sessionInfo, exists := c.Get("session")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Authentication required",
				"code":    "AUTH_REQUIRED",
			})
			return
		}

		info := sessionInfo.(*session.SessionInfo)
		if !info.IsAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Admin privileges required",
				"code":    "ADMIN_REQUIRED",
			})
			return
		}

		c.Next()
	}
}

// OptionalAuthMiddleware creates a middleware that validates session if present
// Does not reject requests without session - useful for public endpoints that can show more info to authenticated users
func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try to get token from cookie first
		token, err := c.Cookie(SessionCookieName)
		if err != nil || token == "" {
			// Try Authorization header as fallback
			authHeader := c.GetHeader("Authorization")
			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if token == "" {
			// No token, continue without session
			c.Next()
			return
		}

		// Validate session
		sessionMgr := session.GetManager()
		sessionInfo, err := sessionMgr.Validate(c.Request.Context(), token)
		if err != nil {
			// Invalid token, continue without session
			log.Debugf("Optional auth: session validation failed: %v", err)
			c.Next()
			return
		}

		// Store session info in context for handlers
		c.Set("session", sessionInfo)
		c.Set("user_id", sessionInfo.UserID)
		c.Set("username", sessionInfo.Username)
		c.Set("is_admin", sessionInfo.IsAdmin)

		c.Next()
	}
}
