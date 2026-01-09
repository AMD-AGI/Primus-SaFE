// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package auth

import (
	"github.com/gin-gonic/gin"
)

// RegisterInitRouter registers initialization routes
// These routes do not require authentication
func RegisterInitRouter(group *gin.RouterGroup) {
	initGroup := group.Group("/init")
	{
		initGroup.GET("/status", GetInitStatus)
		initGroup.POST("/setup", SetupInit)
	}
}

// RegisterAdminAuthRouter registers admin authentication management routes
// These routes require admin/root authentication
func RegisterAdminAuthRouter(group *gin.RouterGroup) {
	// Auth mode management
	authGroup := group.Group("/auth")
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
	rootGroup := group.Group("/root")
	{
		rootGroup.POST("/change-password", ChangeRootPassword)
	}

	// System config management
	configGroup := group.Group("/configs")
	{
		configGroup.GET("", ListSystemConfigs)
		configGroup.GET("/:key", GetSystemConfig)
		configGroup.PUT("/:key", UpdateSystemConfig)
		configGroup.DELETE("/:key", DeleteSystemConfig)
	}
}
