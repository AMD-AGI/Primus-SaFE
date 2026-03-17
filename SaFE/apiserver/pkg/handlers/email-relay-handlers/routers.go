/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package emailrelayhandlers

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

// InitEmailRelayRouters registers email relay endpoints.
// All endpoints require authentication via either:
//   - SaFE API key (Authorization: Bearer <api-key>)
//   - SaFE user session token (Cookie)
//   - X-Internal-Token (service-to-service)
func InitEmailRelayRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath+"/email-relay", h.AuthorizeRelay())
	{
		group.GET("/stream", h.Stream)
		group.POST("/:id/ack", h.Ack)
		group.POST("/:id/fail", h.Fail)
		group.POST("/submit", middleware.Audit("email-relay", "submit"), h.Submit)
	}
}
