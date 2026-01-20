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

// InitCDRouters initializes routes for CD (Continuous Deployment) management.
// Write operations are audited with resourceType "deployment".
func InitCDRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath+"/cd", middleware.Authorize(), middleware.Preprocess())
	{
		// Deployment management with audit
		group.POST("/deployments", middleware.Audit("deployment"), h.CreateDeploymentRequest)
		group.POST("/deployments/:id/approve", middleware.Audit("deployment", "approve"), h.ApproveDeploymentRequest)
		group.POST("/deployments/:id/rollback", middleware.Audit("deployment", "rollback"), h.RollbackDeployment)

		// Read-only routes (no audit)
		group.GET("/deployments", h.ListDeploymentRequests)
		group.GET("/deployments/:id", h.GetDeploymentRequest)
		group.GET("/env-config", h.GetCurrentEnvConfig)
		group.GET("/components", h.GetDeployableComponents)
	}
}
