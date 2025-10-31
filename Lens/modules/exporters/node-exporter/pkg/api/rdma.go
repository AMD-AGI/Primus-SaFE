package api

import (
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/AMD-AGI/primus-lens/node-exporter/pkg/collector"
	"github.com/gin-gonic/gin"
	"net/http"
)

func getRdmaDevices(c *gin.Context) {
	c.JSON(http.StatusOK, rest.SuccessResp(c, collector.GetRdmaDevices()))
}
