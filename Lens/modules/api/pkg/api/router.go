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
		nodeGroup.GET(":name/utilization", getNodeUtilization)
		nodeGroup.GET(":name/utilizationHistory", getNodeUtilizationHistory)
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

	// GPU Aggregation routes - GPU aggregation data query
	gpuAggregationGroup := group.Group("/gpu-aggregation")
	{
		// 元信息查询
		gpuAggregationGroup.GET("/clusters", getClusters)
		gpuAggregationGroup.GET("/namespaces", getNamespaces)
		gpuAggregationGroup.GET("/dimension-keys", getDimensionKeys)
		gpuAggregationGroup.GET("/dimension-values", getDimensionValues)

		// 集群级别小时统计
		gpuAggregationGroup.GET("/cluster/hourly-stats", getClusterHourlyStats)

		// Namespace级别小时统计
		gpuAggregationGroup.GET("/namespaces/hourly-stats", getNamespaceHourlyStats)

		// Label/Annotation级别小时统计
		gpuAggregationGroup.GET("/labels/hourly-stats", getLabelHourlyStats)

		// Workload级别小时统计
		gpuAggregationGroup.GET("/workloads/hourly-stats", getWorkloadHourlyStats)

		// 快照查询
		gpuAggregationGroup.GET("/snapshots/latest", getLatestSnapshot)
		gpuAggregationGroup.GET("/snapshots", listSnapshots)
	}

	// Job Execution History routes - 任务执行历史查询
	jobHistoryGroup := group.Group("/job-execution-histories")
	{
		// 获取最近的失败记录 - 必须在 :id 之前定义
		jobHistoryGroup.GET("/recent-failures", GetRecentFailures)
		// 获取任务统计信息 - 必须在 :id 之前定义
		jobHistoryGroup.GET("/statistics/:job_name", GetJobStatistics)
		// 列表查询 - 支持多种过滤条件
		jobHistoryGroup.GET("", ListJobExecutionHistories)
		// 获取详情
		jobHistoryGroup.GET("/:id", GetJobExecutionHistory)
	}

	return nil
}
