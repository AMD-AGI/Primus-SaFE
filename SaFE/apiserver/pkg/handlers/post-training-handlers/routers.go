/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package posttraininghandlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitRoutes registers the Post Training routes under /api/v1/post-training.
func InitRoutes(engine *gin.Engine, h *Handler) {
	if h == nil {
		return
	}
	group := engine.Group(common.PrimusRouterCustomRootPath+"/post-training",
		middleware.Authorize(), middleware.Preprocess())
	group.GET("/models", h.ListModels)
	group.GET("/config", h.GetConfig)
	group.POST("/recipes/preview", h.PreviewRecipe)
	group.POST("/runs", middleware.Audit("post-training"), h.SubmitRun)
	group.GET("/runs", h.ListRuns)
	group.GET("/runs/:id", h.GetRun)
	group.POST("/runs/:id/stop", middleware.Audit("post-training"), h.StopRun)
	group.POST("/runs/:id/evaluate", middleware.Audit("post-training"), h.EvaluateRun)
	group.POST("/runs/:id/checkpoints/cleanup", middleware.Audit("post-training"), h.CleanupCheckpoints)
	group.POST("/runs/:id/exports", middleware.Audit("post-training"), h.ExportCheckpoint)
	group.DELETE("/runs/:id", middleware.Audit("post-training"), h.DeleteRun)
	group.GET("/runs/:id/metrics", h.GetMetrics)
	group.GET("/runs/:id/metrics/export", h.ExportMetrics)
}
