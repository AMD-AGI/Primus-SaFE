// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector"
	"github.com/gin-gonic/gin"
	"net/http"
)

func getPodReferenceInfo(ctx *gin.Context) {
	result, err := collector.GetContainerInfo(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}
