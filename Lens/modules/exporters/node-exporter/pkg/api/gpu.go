// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector"
	"github.com/gin-gonic/gin"
	"net/http"
)

func GetDriverVersion(ctx *gin.Context) {
	version := collector.GetDriverVersion()
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, version))
}

func GetGpuDeviceInfos(ctx *gin.Context) {
	info := collector.GetGpuDeviceInfo()
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, info))
}

func GetCardMetrics(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, collector.GetCardMetrics()))
}

func getDriCardMapping(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, collector.GetDriCardInfoMapping()))
}
