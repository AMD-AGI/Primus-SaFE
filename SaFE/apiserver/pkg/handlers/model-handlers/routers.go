/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitInferenceRouters initializes and registers all model, playground, and dataset API routes with the Gin engine.
// It sets up authenticated routes requiring authorization and preprocessing.
// Write operations are audited with appropriate resourceTypes.
func InitInferenceRouters(e *gin.Engine, h *Handler) {
	// Model and Playground API requires authentication and preprocessing.
	group := e.Group(common.PrimusRouterCustomRootPath, middleware.Authorize(), middleware.Preprocess())
	{
		// Playground chat (high frequency, no audit)
		group.POST("playground/chat", h.Chat)
		group.GET("playground/services", h.ListPlaygroundServices)

		// Playground sessions with audit
		group.POST("playground/sessions", middleware.Audit("session"), h.SaveSession)
		group.DELETE("playground/sessions/:id", middleware.Audit("session"), h.DeletePlaygroundSession)
		group.GET("playground/sessions", h.ListPlaygroundSession)
		group.GET("playground/sessions/:id", h.GetPlaygroundSession)

		// Model management with audit
		group.POST("playground/models", middleware.Audit("model"), h.CreateModel)
		group.PATCH("playground/models/:id", middleware.Audit("model"), h.PatchModel)
		group.DELETE("playground/models/:id", middleware.Audit("model"), h.DeleteModel)
		group.POST("playground/models/:id/retry", middleware.Audit("model", "retry"), h.RetryModel)
		group.GET("playground/models", h.ListModels)
		group.GET("playground/models/:id", h.GetModel)
		group.GET("playground/models/:id/workloads", h.GetModelWorkloads)
		group.GET("playground/models/:id/workload-config", h.GetWorkloadConfig)
		group.GET("playground/models/:id/chat-url", h.GetChatURL)

		// Dataset routes (only registered if S3 is enabled)
		if h.IsDatasetEnabled() {
			group.POST("datasets", middleware.Audit("dataset"), h.CreateDataset)
			group.DELETE("datasets/:id", middleware.Audit("dataset"), h.DeleteDataset)
			group.GET("datasets/types", h.ListDatasetTypes)
			group.GET("datasets", h.ListDatasets)
			group.GET("datasets/:id", h.GetDataset)
			group.GET("datasets/:id/files/*path", h.GetDatasetFile)
		}
	}
}
