/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package image_handlers

import "github.com/gin-gonic/gin"

// InitImageRouter initializes and registers all image-related API routes.
func InitImageRouter(e *gin.Engine, h *ImageHandler) {
	group := e.Group("/api/v1/")
	harborGroup := group.Group("/harbor")
	{
		harborGroup.GET("stats", func(c *gin.Context) {
			handle(c, h.GetHarborStats)
		})
	}
	imageGroup := e.Group("/api/v1/images")
	{
		imageGroup.GET("", func(c *gin.Context) {
			handle(c, h.listImage)
		})
		imageGroup.GET("custom", func(c *gin.Context) {
			handle(c, h.listExportedImage)
		})
		imageGroup.DELETE(":id", func(c *gin.Context) {
			handle(c, h.deleteImage)
		})
		imageGroup.GET(":id/importing-details", func(c *gin.Context) {
			handle(c, h.getImportingDetail)
		})
		imageGroup.PUT(":id/importing:retry", func(c *gin.Context) {
			handle(c, h.retryDispatchImportImageJob)
		})
	}
	imageImportGroup := e.Group("/api/v1/images:import")
	{
		imageImportGroup.POST("", func(c *gin.Context) {
			handle(c, h.importImage)
		})
		imageImportGroup.PUT(":name/progress", func(c *gin.Context) {
			handle(c, h.updateImportProgress)
		})
	}

	imageRegistryGroup := group.Group("/image-registries")
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
