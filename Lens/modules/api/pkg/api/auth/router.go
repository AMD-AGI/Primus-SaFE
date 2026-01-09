// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"github.com/gin-gonic/gin"
)

// RegisterPublicAuthRouter registers public authentication routes
// These routes do not require authentication (login, init status)
func RegisterPublicAuthRouter(group *gin.RouterGroup) {
	authGroup := group.Group("/auth")
	{
		// Login/Logout - no auth required
		authGroup.POST("/login", Login)
		authGroup.POST("/logout", Logout)

		// Session refresh - validates existing session
		authGroup.POST("/refresh", RefreshSession)
	}

	// System initialization routes - no auth required for first-time setup
	initGroup := group.Group("/init")
	{
		initGroup.GET("/status", GetInitStatus)
		initGroup.POST("/setup", SetupInit)
	}
}

// RegisterProtectedAuthRouter registers protected authentication routes
// These routes require valid session authentication
func RegisterProtectedAuthRouter(group *gin.RouterGroup) {
	// Apply session auth middleware to this group
	authGroup := group.Group("/auth")
	authGroup.Use(SessionAuthMiddleware())
	{
		// Get current user info
		authGroup.GET("/me", GetCurrentUser)
	}
}

// RegisterAdminAuthRouter registers admin authentication management routes
// These routes require admin/root authentication
func RegisterAdminAuthRouter(group *gin.RouterGroup) {
	// Apply both session auth and admin check middleware
	adminGroup := group.Group("")
	adminGroup.Use(SessionAuthMiddleware(), AdminAuthMiddleware())
	{
		// Auth mode management
		authGroup := adminGroup.Group("/auth")
		{
			authGroup.GET("/mode", GetAuthMode)
			authGroup.PUT("/mode", SetAuthMode)

			// Auth provider management
			providerGroup := authGroup.Group("/providers")
			{
				providerGroup.GET("", ListAuthProviders)
				providerGroup.POST("", CreateAuthProvider)
				providerGroup.GET("/:id", GetAuthProvider)
				providerGroup.PUT("/:id", UpdateAuthProvider)
				providerGroup.DELETE("/:id", DeleteAuthProvider)
				providerGroup.POST("/:id/test", TestAuthProvider)
			}
		}

		// Root user management
		rootGroup := adminGroup.Group("/root")
		{
			rootGroup.POST("/change-password", ChangeRootPassword)
		}

		// System config management
		configGroup := adminGroup.Group("/configs")
		{
			configGroup.GET("", ListSystemConfigs)
			configGroup.GET("/:key", GetSystemConfig)
			configGroup.PUT("/:key", UpdateSystemConfig)
			configGroup.DELETE("/:key", DeleteSystemConfig)
		}
	}
}

// RegisterInitRouter registers initialization routes (legacy, for backward compatibility)
// Deprecated: Use RegisterPublicAuthRouter instead
func RegisterInitRouter(group *gin.RouterGroup) {
	initGroup := group.Group("/init")
	{
		initGroup.GET("/status", GetInitStatus)
		initGroup.POST("/setup", SetupInit)
	}
}
