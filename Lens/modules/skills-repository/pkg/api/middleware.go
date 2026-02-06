// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	// Header keys (match proxy_handler.go and common/constant.go)
	HeaderUserID   = "userId"
	HeaderUsername = "userName"

	// Context keys
	ContextKeyUserID   = "userId"
	ContextKeyUsername = "userName"
)

// UserInfo represents authenticated user information
type UserInfo struct {
	UserID   string
	Username string
}

// GetUserInfo extracts user info from gin context
func GetUserInfo(c *gin.Context) *UserInfo {
	userID, _ := c.Get(ContextKeyUserID)
	username, _ := c.Get(ContextKeyUsername)

	uid, _ := userID.(string)
	uname, _ := username.(string)

	return &UserInfo{
		UserID:   uid,
		Username: uname,
	}
}

// AuthMiddleware extracts user information from request headers
// and stores it in the gin context for downstream handlers.
// If auth is required and headers are missing, returns 401 Unauthorized.
func AuthMiddleware(required bool) gin.HandlerFunc {
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

		c.Next()
	}
}
