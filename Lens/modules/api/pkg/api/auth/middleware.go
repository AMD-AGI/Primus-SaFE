// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/router/middleware"
	"github.com/gin-gonic/gin"
)

// SessionAuthMiddleware returns a middleware that validates session authentication
// It uses the user info set by the global auth middleware (HandleAuth)
func SessionAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if user info was set by the global auth middleware
		userID := middleware.GetUserID(c)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    http.StatusUnauthorized,
				"message": "authentication required",
			})
			return
		}

		c.Next()
	}
}

// AdminAuthMiddleware returns a middleware that checks admin privileges
// Must be used after SessionAuthMiddleware
func AdminAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user type from context (set by HandleAuth middleware)
		userType := middleware.GetUserType(c)

		// Check if user is admin or root
		if userType != "admin" && userType != "root" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code":    http.StatusForbidden,
				"message": "admin privileges required",
			})
			return
		}

		c.Next()
	}
}
