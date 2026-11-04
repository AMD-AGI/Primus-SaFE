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
	group.POST("/experiments", middleware.Audit("post-training"), h.SubmitExperiment)
	group.GET("/experiments", h.ListExperiments)
	group.GET("/experiments/:id", h.GetExperiment)
	group.POST("/experiments/:id/stop", middleware.Audit("post-training"), h.StopExperiment)
	group.DELETE("/experiments/:id", middleware.Audit("post-training"), h.DeleteExperiment)
	group.GET("/templates", h.ListTemplates)
	group.GET("/templates/:id", h.GetTemplate)
	group.POST("/templates", middleware.Audit("post-training"), h.CreateTemplate)
	group.PUT("/templates/:id", middleware.Audit("post-training"), h.UpdateTemplate)
	group.DELETE("/templates/:id", middleware.Audit("post-training"), h.DeleteTemplate)
	group.GET("/runs/:id/samples", h.GetSamples)
	group.GET("/runs/:id/metrics", h.GetMetrics)
	group.GET("/runs/:id/metrics/export", h.ExportMetrics)
	group.GET("/runs/:id/import", h.GetRunImport)
	group.GET("/imports/config", h.GetImportConfig)
	group.POST("/imports/tokens", middleware.Audit("post-training"), h.IssueImportToken)
	group.GET("/imports/tokens/:id", h.GetImportToken)
	// The command upload authenticates with its one-time import token, not a session.
	engine.POST(common.PrimusRouterCustomRootPath+"/post-training/imports", h.UploadImport)
}
