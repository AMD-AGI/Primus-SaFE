/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middle"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitInferenceRouters initializes and registers all inference and playground API routes with the Gin engine.
// It sets up authenticated routes requiring authorization and preprocessing.
func InitInferenceRouters(e *gin.Engine, h *Handler) {
	// Inference and Playground API requires authentication and preprocessing.
	group := e.Group(common.PrimusRouterCustomRootPath, middle.Authorize(), middle.Preprocess())
	{
		// Playground routes
		group.POST("playground/chat", h.Chat)                              // Real-time chat with inference (streaming support)
		group.POST("playground/sessions", h.SaveSession)                   // Save or update session
		group.GET("playground/sessions", h.ListPlaygroundSession)          // List sessions
		group.GET("playground/sessions/:id", h.GetPlaygroundSession)       // Get session detail
		group.DELETE("playground/sessions/:id", h.DeletePlaygroundSession) // Delete session

		// Playground Model Management
		group.POST("playground/models", h.CreateModel)
		group.GET("playground/models", h.ListModels)
		group.GET("playground/models/:id", h.GetModel)
		group.DELETE("playground/models/:id", h.DeleteModel)
		group.POST("playground/models/:id/toggle", h.ToggleModel)
	}
}
