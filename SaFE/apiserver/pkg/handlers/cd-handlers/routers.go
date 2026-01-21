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

// InitCDRouters initializes routes for both Safe and Lens CD
// Use query param ?type=safe or ?type=lens to filter (default: safe)
func InitCDRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath+"/cd", middleware.Authorize(), middleware.Preprocess())
	{
		// Unified endpoints - use type field in body or query param to distinguish
		group.POST("/deployments", h.CreateDeploymentRequest)       // body.type = "safe" or "lens"
		group.GET("/deployments", h.ListDeploymentRequests)         // ?type=safe or ?type=lens
		group.GET("/deployments/:id", h.GetDeploymentRequest)
		group.POST("/deployments/:id/approve", h.ApproveDeploymentRequest)
		group.POST("/deployments/:id/rollback", h.RollbackDeployment)

		// Get latest deployment configuration
		// Query params: ?type=safe or ?type=lens (default: safe)
		group.GET("/env-config", h.GetCurrentEnvConfig)

		// Get deployable components list
		group.GET("/components", h.GetDeployableComponents)
	}
}
