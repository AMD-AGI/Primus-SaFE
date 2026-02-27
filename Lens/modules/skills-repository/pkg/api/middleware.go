// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/skills-repository/pkg/safe"
	"github.com/gin-gonic/gin"
)

const (
	// Header keys (match proxy_handler.go and common/constant.go)
	HeaderUserID   = "userId"
	HeaderUsername = "userName"

	// Context keys
	ContextKeyUserID   = "userId"
	ContextKeyUsername = "userName"
	ContextKeyIsAdmin  = "isAdmin"
)

// UserInfo represents authenticated user information
type UserInfo struct {
	UserID   string
	Username string
	IsAdmin  bool
}

// GetUserInfo extracts user info from gin context
func GetUserInfo(c *gin.Context) *UserInfo {
	userID, _ := c.Get(ContextKeyUserID)
	username, _ := c.Get(ContextKeyUsername)
	isAdmin, _ := c.Get(ContextKeyIsAdmin)

	uid, _ := userID.(string)
	uname, _ := username.(string)
	admin, _ := isAdmin.(bool)

	return &UserInfo{
		UserID:   uid,
		Username: uname,
		IsAdmin:  admin,
	}
}

// AuthMiddleware extracts user information from request headers
// and stores it in the gin context for downstream handlers.
// If auth is required and headers are missing, returns 401 Unauthorized.
// If a SaFE UserClient is provided, it queries the SaFE API to resolve
// the user's roles and admin status.
func AuthMiddleware(required bool, safeClient *safe.UserClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetHeader(HeaderUserID)
		username := c.GetHeader(HeaderUsername)

		// If auth is required, check for user ID
		if required && userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authentication required: missing userId header",
			})
			return
		}

		// Store user info in context
		c.Set(ContextKeyUserID, userID)
		c.Set(ContextKeyUsername, username)

		// Query SaFE API for user roles and admin status
		isAdmin := false
		if safeClient != nil && userID != "" {
			if userInfo, err := safeClient.GetUserInfo(c.Request.Context(), userID); err == nil && userInfo != nil {
				isAdmin = userInfo.IsAdmin()
				// Use SaFE username if proxy didn't provide one
				if username == "" && userInfo.Name != "" {
					c.Set(ContextKeyUsername, userInfo.Name)
				}
			}
		}
		c.Set(ContextKeyIsAdmin, isAdmin)

		c.Next()
	}
}
