// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/controlplane/auth/session"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	// SafeTokenCookieName is the cookie name used by SaFE for session token
	SafeTokenCookieName = "token"

	// sessionValidationRetryDelay is the delay before retrying session validation
	// This handles the case where user just logged in via SaFE but token hasn't synced yet
	sessionValidationRetryDelay = 2 * time.Second
)

// getTokenFromRequest extracts session token from request
// Priority: lens_session cookie -> SaFE token cookie -> Authorization header
func getTokenFromRequest(c *gin.Context) string {
	// 1. Try Lens session cookie first
	if token, err := c.Cookie(SessionCookieName); err == nil && token != "" {
		return token
	}

	// 2. Try SaFE token cookie (for SSO users authenticated via SaFE)
	if token, err := c.Cookie(SafeTokenCookieName); err == nil && token != "" {
		return token
	}

	// 3. Try Authorization header as fallback
	if authHeader := c.GetHeader("Authorization"); authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimPrefix(authHeader, "Bearer ")
	}

	return ""
}

// SessionAuthMiddleware creates a middleware that validates session tokens
// This middleware is specifically for the new auth system and does NOT affect existing APIs
// Supports both Lens native sessions and SaFE synced sessions (via primus-safe-adapter)
// Includes retry logic to handle race condition when token hasn't synced yet
func SessionAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := getTokenFromRequest(c)

		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Authentication required",
				"code":    "AUTH_REQUIRED",
			})
			return
		}

		// Validate session with retry logic
		sessionMgr := session.GetManager()
		sessionInfo, err := sessionMgr.Validate(c.Request.Context(), token)
		if err != nil {
			// First attempt failed, wait and retry once
			// This handles the case where user just logged in via SaFE
			// but the token hasn't been synced to Lens yet
			log.Debugf("Session validation failed (attempt 1): %v, retrying in %v", err, sessionValidationRetryDelay)

			select {
			case <-time.After(sessionValidationRetryDelay):
				// Retry validation after delay
				sessionInfo, err = sessionMgr.Validate(c.Request.Context(), token)
				if err != nil {
					log.Debugf("Session validation failed (attempt 2): %v", err)
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"success": false,
						"message": "Invalid or expired session",
						"code":    "SESSION_INVALID",
					})
					return
				}
				log.Debugf("Session validation succeeded on retry for user: %s", sessionInfo.UserID)
			case <-c.Request.Context().Done():
				// Request context cancelled
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"success": false,
					"message": "Request cancelled",
					"code":    "REQUEST_CANCELLED",
				})
				return
			}
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
// Supports both Lens native sessions and SaFE synced sessions (via primus-safe-adapter)
func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := getTokenFromRequest(c)

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
