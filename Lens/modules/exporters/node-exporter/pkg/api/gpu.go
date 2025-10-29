package api

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/collector"
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
