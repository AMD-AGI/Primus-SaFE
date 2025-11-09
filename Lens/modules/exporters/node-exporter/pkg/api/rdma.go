package api

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/node-exporter/pkg/collector"
	"github.com/gin-gonic/gin"
	"net/http"
)

func getRdmaDevices(c *gin.Context) {
	c.JSON(http.StatusOK, rest.SuccessResp(c, collector.GetRdmaDevices()))
}
