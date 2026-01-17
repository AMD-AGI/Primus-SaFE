/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cdhandlers

import (
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/gin-gonic/gin"
)

// InitCDRouters initializes routes
func InitCDRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath+"/cd", middleware.Authorize(), middleware.Preprocess())
	{
		group.POST("/deployments", h.CreateDeploymentRequest)
		group.GET("/deployments", h.ListDeploymentRequests)
		group.GET("/deployments/:id", h.GetDeploymentRequest)
		group.POST("/deployments/:id/approve", h.ApproveDeploymentRequest)
		group.POST("/deployments/:id/rollback", h.RollbackDeployment)

		// Get current environment configuration
		group.GET("/env-config", h.GetCurrentEnvConfig)

		// Get deployable components list
		group.GET("/components", h.GetDeployableComponents)
	}
}
