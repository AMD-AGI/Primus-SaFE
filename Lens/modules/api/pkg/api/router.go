package api

import "github.com/gin-gonic/gin"

func RegisterRouter(group *gin.RouterGroup) error {
	nodeGroup := group.Group("/nodes")
	{
		nodeGroup.GET("gpuAllocation", getClusterGpuAllocationInfo)
		nodeGroup.GET("gpuUtilization", getClusterGPUUtilization)
		nodeGroup.GET("gpuUtilizationHistory", getGpuUsageHistory)
		nodeGroup.GET("", getGPUNodeList)
		nodeGroup.GET(":name", getNodeInfoByName)
		nodeGroup.GET(":name/gpuDevices", getGpuDevice)
		nodeGroup.GET(":name/gpuMetrics", getNodeGpuMetrics)
		nodeGroup.GET(":name/workloads", getNodeWorkload)
		nodeGroup.GET(":name/workloadsHistory", getNodeWorkloadHistory)
	}
	clusterGroup := group.Group("/clusters")
	{
		clusterGroup.GET("overview", getClusterOverview)
		clusterGroup.GET("consumers", getConsumerInfo)
		clusterGroup.GET("gpuHeatmap", getClusterGpuHeatmap)
	}
	workloadGroup := group.Group("/workloads")
	{
		workloadGroup.GET("", listWorkloads)
		workloadGroup.GET(":uid/hierarchy", getWorkloadHierarchy)
		workloadGroup.GET(":uid", getWorkloadInfo)
		workloadGroup.GET(":uid/metrics", getWorkloadMetrics)
		workloadGroup.GET(":uid/trainingPerformance", GetWorkloadTrainingPerformance)

	}
	group.GET("workloadMetadata", getWorkloadsMetadata)
	storageGroup := group.Group("/storage")
	{
		storageGroup.GET("stat", getStorageStat)
	}
	return nil
}
