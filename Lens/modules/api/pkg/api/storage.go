package api

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"net/http"
)

func getStorageStat(ctx *gin.Context) {
	stats, err := storage.GetStorageStat(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, stats))
}
