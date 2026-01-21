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

		// Dataset routes (only registered if S3 is enabled)
		if h.IsDatasetEnabled() {
			group.GET("datasets/types", h.ListDatasetTypes)         // List all dataset types with schemas
			group.POST("datasets", h.CreateDataset)                 // Create dataset with file upload and download to workspace
			group.GET("datasets", h.ListDatasets)                   // List datasets with filtering
			group.GET("datasets/:id", h.GetDataset)                 // Get dataset details (includes file list)
			group.DELETE("datasets/:id", h.DeleteDataset)           // Delete dataset
			group.GET("datasets/:id/files/*path", h.GetDatasetFile) // Get or preview a specific file (use ?preview=true for content)
		}

		// Evaluation routes
		// Note: Benchmark datasets are now in the dataset table, use GET /datasets?datasetType=evaluation
		group.GET("evaluations/available-services", h.ListAvailableEvalServices) // List models/services for evaluation
		group.POST("evaluations/tasks", h.CreateEvaluationTask)               // Create evaluation task
		group.GET("evaluations/tasks", h.ListEvaluationTasks)                 // List evaluation tasks
		group.GET("evaluations/tasks/:id", h.GetEvaluationTask)               // Get task details
		group.DELETE("evaluations/tasks/:id", h.DeleteEvaluationTask)         // Delete/cancel task
		group.GET("evaluations/tasks/:id/report", h.GetEvaluationReport)      // Get evaluation report
	}
}
