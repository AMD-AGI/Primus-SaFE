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

	// Process tree routes
	processTreeGroup := group.Group("/process-tree")
	{
		processTreeGroup.POST("/pod", GetPodProcessTree)
		processTreeGroup.POST("/python", FindPythonProcessesInPod)
		processTreeGroup.POST("/tensorboard", FindTensorboardFilesInPod)
		processTreeGroup.POST("/pytorch-profiler", FindPyTorchProfilerFilesInPod)
		processTreeGroup.POST("/env", GetProcessEnvironment)
		processTreeGroup.POST("/args", GetProcessArguments)
	}

	// Container filesystem routes
	containerFSGroup := group.Group("/container-fs")
	{
		// File operations
		containerFSGroup.POST("/read", ReadContainerFile)
		containerFSGroup.POST("/list", ListContainerDirectory)
		containerFSGroup.POST("/info", GetContainerFileInfo)

		// TensorBoard-specific operations
		containerFSGroup.POST("/tensorboard/logs", GetTensorBoardLogs)
		containerFSGroup.POST("/tensorboard/event", ReadTensorBoardEvent)
	}

	return nil
}
