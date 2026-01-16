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

// InitInferenceRouters initializes and registers all model and playground API routes with the Gin engine.
// It sets up authenticated routes requiring authorization and preprocessing.
func InitInferenceRouters(e *gin.Engine, h *Handler) {
	// Model and Playground API requires authentication and preprocessing.
	group := e.Group(common.PrimusRouterCustomRootPath, middleware.Authorize(), middleware.Preprocess())
	{
		// Playground routes
		group.POST("playground/chat", h.Chat)                              // Chat with model or workload
		group.GET("playground/services", h.ListPlaygroundServices)         // List available chat services
		group.POST("playground/sessions", h.SaveSession)                   // Save or update session
		group.GET("playground/sessions", h.ListPlaygroundSession)          // List sessions
		group.GET("playground/sessions/:id", h.GetPlaygroundSession)       // Get session detail
		group.DELETE("playground/sessions/:id", h.DeletePlaygroundSession) // Delete session

		// Model Management routes
		group.POST("playground/models", h.CreateModel)                          // Create a new model
		group.GET("playground/models", h.ListModels)                            // List all models
		group.GET("playground/models/:id", h.GetModel)                          // Get model by ID
		group.PATCH("playground/models/:id", h.PatchModel)                      // Partial update model fields
		group.DELETE("playground/models/:id", h.DeleteModel)                    // Delete a model
		group.POST("playground/models/:id/retry", h.RetryModel)                 // Retry failed model download
		group.GET("playground/models/:id/workloads", h.GetModelWorkloads)       // List workloads associated with model
		group.GET("playground/models/:id/workload-config", h.GetWorkloadConfig) // Get workload config for deployment
		group.GET("playground/models/:id/chat-url", h.GetChatURL)               // Get chat URL for remote_api model
	}
}
