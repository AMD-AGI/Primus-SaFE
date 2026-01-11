/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package dataset_handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middle"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitDatasetRouters initializes and registers all dataset API routes with the Gin engine.
// It sets up authenticated routes for managing datasets including create, list, get, delete operations.
func InitDatasetRouters(e *gin.Engine, h *Handler) {
	// Dataset API requires authentication and preprocessing
	group := e.Group(common.PrimusRouterCustomRootPath, middle.Authorize(), middle.Preprocess())
	{
		// Dataset type and template endpoints (must be before :id routes)
		group.GET("datasets/types", h.ListDatasetTypes)             // List all dataset types
		group.GET("datasets/templates/:type", h.GetDatasetTemplate) // Get template for a dataset type

		// Dataset CRUD operations
		group.POST("datasets", h.CreateDataset)             // Create dataset with file upload and download to workspace
		group.GET("datasets", h.ListDatasets)               // List datasets with filtering
		group.GET("datasets/:id", h.GetDataset)             // Get dataset details
		group.DELETE("datasets/:id", h.DeleteDataset)       // Delete dataset
		group.GET("datasets/:id/files", h.ListDatasetFiles) // List files in dataset
	}
}
