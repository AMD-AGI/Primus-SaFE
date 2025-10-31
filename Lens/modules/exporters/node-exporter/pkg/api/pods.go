package api

import (
	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/collector"
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
