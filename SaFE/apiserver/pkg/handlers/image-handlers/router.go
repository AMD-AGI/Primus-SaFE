/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import (
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/gin-gonic/gin"
)

// InitImageRouter initializes and registers all image-related API routes.
func InitImageRouter(e *gin.Engine, h *ImageHandler) {
	group := e.Group("/api/v1/")
	harborGroup := group.Group("/harbor", middleware.Authorize())
	{
		harborGroup.GET("stats", func(c *gin.Context) {
			handle(c, h.GetHarborStats)
		})
	}
	imageGroup := e.Group("/api/v1/images", middleware.Authorize())
	{
		imageGroup.GET("", func(c *gin.Context) {
			handle(c, h.listImage)
		})
		imageGroup.GET("custom", func(c *gin.Context) {
			handle(c, h.listExportedImage)
		})
		imageGroup.GET("prewarm", func(c *gin.Context) {
			handle(c, h.listPrewarmImage)
		})
		imageGroup.GET(":id/importing-details", func(c *gin.Context) {
			handle(c, h.getImportingDetail)
		})
		imageGroup.GET(":id/importing-logs", func(c *gin.Context) {
			handle(c, h.getImportingLogs)
		})

		imageGroup.DELETE("custom/:jobId", middleware.Audit("custom-image"), func(c *gin.Context) {
			handle(c, h.deleteExportedImage)
		})
		imageGroup.DELETE(":id", middleware.Audit("image"), func(c *gin.Context) {
			handle(c, h.deleteImage)
		})
		imageGroup.PUT(":id/importing:retry", middleware.Audit("image", "retry"), func(c *gin.Context) {
			handle(c, h.retryDispatchImportImageJob)
		})
	}
	imageImportGroup := e.Group("/api/v1/images:import")
	{
		imageImportGroup.POST("", middleware.Authorize(), middleware.Audit("image", "import"), func(c *gin.Context) {
			handle(c, h.importImage)
		})
		imageImportGroup.PUT(":name/progress", func(c *gin.Context) {
			handle(c, h.updateImportProgress)
		})
	}

	imageRegistryGroup := group.Group("/image-registries", middleware.Authorize(), middleware.Audit("imageregistry"))
	{
		imageRegistryGroup.POST("", func(c *gin.Context) {
			handle(c, h.createImageRegistry)
		})
		imageRegistryGroup.PUT(":id", func(c *gin.Context) {
			handle(c, h.updateImageRegistry)
		})
		imageRegistryGroup.DELETE(":id", func(c *gin.Context) {
			handle(c, h.deleteImageRegistry)
		})
		imageRegistryGroup.GET("", func(c *gin.Context) {
			handle(c, h.listImageRegistry)
		})
	}
}
