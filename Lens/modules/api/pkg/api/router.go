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

	// Metric Alert Rule management routes
	metricAlertRuleGroup := group.Group("/metric-alert-rules")
	{
		metricAlertRuleGroup.POST("", CreateMetricAlertRule)
		metricAlertRuleGroup.GET("", ListMetricAlertRules)
		metricAlertRuleGroup.GET(":id", GetMetricAlertRule)
		metricAlertRuleGroup.PUT(":id", UpdateMetricAlertRule)
		metricAlertRuleGroup.DELETE(":id", DeleteMetricAlertRule)
		metricAlertRuleGroup.POST(":id/clone", CloneMetricAlertRule)
		metricAlertRuleGroup.POST("sync", SyncMetricAlertRulesToCluster)
		metricAlertRuleGroup.GET(":id/status", GetVMRuleStatus)
	}

	// Log Alert Rule management routes
	logAlertRuleGroup := group.Group("/log-alert-rules")
	{
		logAlertRuleGroup.POST("", CreateLogAlertRule)
		logAlertRuleGroup.GET("", ListLogAlertRules)
		logAlertRuleGroup.GET("/multi-cluster", ListLogAlertRulesMultiCluster)
		logAlertRuleGroup.GET(":id", GetLogAlertRule)
		logAlertRuleGroup.PUT(":id", UpdateLogAlertRule)
		logAlertRuleGroup.DELETE(":id", DeleteLogAlertRule)
		logAlertRuleGroup.POST("/batch-update", BatchUpdateLogAlertRules)
		logAlertRuleGroup.POST("/test", TestLogAlertRule)
		logAlertRuleGroup.GET(":id/statistics", GetLogAlertRuleStatistics)
		logAlertRuleGroup.GET(":id/versions", GetLogAlertRuleVersions)
		logAlertRuleGroup.POST(":id/rollback/:version", RollbackLogAlertRule)
		logAlertRuleGroup.POST(":id/clone", CloneLogAlertRule)
	}

	// Log Alert Rule Template routes
	logAlertRuleTemplateGroup := group.Group("/log-alert-rule-templates")
	{
		logAlertRuleTemplateGroup.POST("", CreateLogAlertRuleTemplate)
		logAlertRuleTemplateGroup.GET("", ListLogAlertRuleTemplates)
		logAlertRuleTemplateGroup.GET(":id", GetLogAlertRuleTemplate)
		logAlertRuleTemplateGroup.DELETE(":id", DeleteLogAlertRuleTemplate)
		logAlertRuleTemplateGroup.POST(":id/instantiate", CreateRuleFromTemplate)
	}

	// Alert Rule Advice routes
	alertRuleAdviceGroup := group.Group("/alert-rule-advices")
	{
		alertRuleAdviceGroup.POST("", CreateAlertRuleAdvice)
		alertRuleAdviceGroup.POST("/batch", BatchCreateAlertRuleAdvices)
		alertRuleAdviceGroup.GET("", ListAlertRuleAdvices)
		alertRuleAdviceGroup.GET("/summary", GetAdviceSummary)
		alertRuleAdviceGroup.GET("/statistics", GetAdviceStatistics)
		alertRuleAdviceGroup.GET(":id", GetAlertRuleAdvice)
		alertRuleAdviceGroup.PUT(":id", UpdateAlertRuleAdvice)
		alertRuleAdviceGroup.DELETE(":id", DeleteAlertRuleAdvice)
		alertRuleAdviceGroup.POST("/batch-delete", BatchDeleteAlertRuleAdvices)
		alertRuleAdviceGroup.POST(":id/status", UpdateAdviceStatus)
		alertRuleAdviceGroup.POST("/batch-status", BatchUpdateAdviceStatus)
		alertRuleAdviceGroup.POST(":id/apply", ApplyAlertRuleAdvice)
	}

	// GPU Aggregation routes - GPU聚合数据查询
	gpuAggregationGroup := group.Group("/gpu-aggregation")
	{
		// 集群级别小时统计
		gpuAggregationGroup.GET("/cluster/hourly-stats", getClusterHourlyStats)
		
		// Namespace级别小时统计
		gpuAggregationGroup.GET("/namespaces/hourly-stats", getNamespaceHourlyStats)
		
		// Label/Annotation级别小时统计
		gpuAggregationGroup.GET("/labels/hourly-stats", getLabelHourlyStats)
		
		// 快照查询
		gpuAggregationGroup.GET("/snapshots/latest", getLatestSnapshot)
		gpuAggregationGroup.GET("/snapshots", listSnapshots)
	}

	return nil
}
