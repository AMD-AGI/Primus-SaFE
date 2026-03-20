/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package a2ahandlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
)

// InitA2ARouters registers A2A API routes on the Gin engine.
func InitA2ARouters(engine *gin.Engine, handler *Handler) {
	a2a := engine.Group("/api/v1/a2a")
	a2a.Use(func(c *gin.Context) {
		err := authority.ParseToken(c)
		if err != nil {
			apiutils.AbortWithApiError(c, err)
			return
		}
		c.Next()
	})

	a2a.POST("/services", handler.CreateService)
	a2a.GET("/services", handler.ListServices)
	a2a.GET("/services/:serviceName", handler.GetService)
	a2a.PATCH("/services/:serviceName", handler.UpdateService)
	a2a.DELETE("/services/:serviceName", handler.DeleteService)

	a2a.GET("/call-logs", handler.ListCallLogs)
	a2a.GET("/topology", handler.GetTopology)
}
