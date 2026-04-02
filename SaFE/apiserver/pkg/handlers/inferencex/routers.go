/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package inferencex

import (
	"github.com/gin-gonic/gin"

	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/middleware"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func InitInferenceXRouters(e *gin.Engine, h *Handler) {
	group := e.Group(common.PrimusRouterCustomRootPath+"/inferencex", middleware.Authorize(), middleware.Preprocess())
	{
		group.GET("/benchmarks", h.GetBenchmarks)
		group.GET("/filters", h.GetFilters)
	}
}
