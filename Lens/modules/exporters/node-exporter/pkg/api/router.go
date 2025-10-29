package api

import (
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func RegisterRouter(group *gin.RouterGroup) error {
	gpuGroup := group.Group("/gpus")
	{
		gpuGroup.GET("", GetGpuDeviceInfos)
	}
	rdmaGroup := group.Group("/rdma")
	{
		rdmaGroup.GET("", getRdmaDevices)
	}
	podGroup := group.Group("/pods")
	{
		podGroup.GET("", getPodReferenceInfo)
	}
	group.GET("gpuDriverVersion", GetDriverVersion)
	group.GET("cardMetrics", GetCardMetrics)
	group.GET("driMapping", getDriCardMapping)
	group.GET("metrics", func(c *gin.Context) {
		extraRegistry := prometheus.NewRegistry()

		gatherers := prometheus.Gatherers{
			prometheus.DefaultGatherer,
			extraRegistry,
		}

		h := promhttp.HandlerFor(
			gatherers,
			promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			},
		)

		h.ServeHTTP(c.Writer, c.Request)
	})

	return nil
}
