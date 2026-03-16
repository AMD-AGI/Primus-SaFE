/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package emailrelayhandlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitEmailRelayRouters registers email relay endpoints.
// These endpoints use X-Internal-Token authentication (not user session auth).
func InitEmailRelayRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath + "/email-relay")
	{
		group.GET("/stream", h.Stream)
		group.POST("/:id/ack", h.Ack)
		group.POST("/:id/fail", h.Fail)
		group.POST("/submit", h.Submit)
	}
}
