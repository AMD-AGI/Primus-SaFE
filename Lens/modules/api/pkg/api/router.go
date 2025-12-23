package api

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/tracelens"
	"github.com/gin-gonic/gin"
)

func RegisterRouter(group *gin.RouterGroup) error {
	nodeGroup := group.Group("/nodes")
	{
		nodeGroup.GET("gpuAllocation", getClusterGpuAllocationInfo)
		nodeGroup.GET("gpuUtilization", getClusterGPUUtilization)
		nodeGroup.GET("gpuUtilizationHistory", getGpuUsageHistory)
		nodeGroup.GET("", getGPUNodeList)
		// Node Fragmentation Analysis API
		nodeGroup.GET("fragmentation-analysis", getFragmentationAnalysis)
		nodeGroup.GET("load-balance-analysis", getLoadBalanceAnalysis)
		nodeGroup.GET(":name", getNodeInfoByName)
		nodeGroup.GET(":name/fragmentation", getNodeFragmentation)
		nodeGroup.GET(":name/gpuDevices", getGpuDevice)
		nodeGroup.GET(":name/gpuMetrics", getNodeGpuMetrics)
		nodeGroup.GET(":name/utilization", getNodeUtilization)
		nodeGroup.GET(":name/utilizationHistory", getNodeUtilizationHistory)
		nodeGroup.GET(":name/workloads", getNodeWorkload)
		nodeGroup.GET(":name/workloadsHistory", getNodeWorkloadHistory)
	}

	// Pod routes - Pod REST API
	podGroup := group.Group("/pods")
	{
		// Query pod statistics with filtering and pagination
		podGroup.GET("/stats", getPodStats)
		// Get detailed information for a single pod
		podGroup.GET("/:pod_uid", getPodDetail)
		// Get GPU usage history for a pod
		podGroup.GET("/:pod_uid/gpu-history", getPodGPUHistory)
		// Get events related to a pod
		podGroup.GET("/:pod_uid/events", getPodEvents)
		// Compare multiple pods side-by-side
		podGroup.GET("/comparison", comparePods)
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
		workloadGroup.GET("statistic", getWorkloadsStatistic)
		workloadGroup.GET("hierarchy", getWorkloadHierarchyByKindName)
		workloadGroup.GET("gpuUtilizationHistory", getWorkloadGpuUtilizationHistoryByKindName)
		workloadGroup.GET(":uid/hierarchy", getWorkloadHierarchy)
		workloadGroup.GET(":uid", getWorkloadInfo)
		workloadGroup.GET(":uid/metrics", getWorkloadMetrics)
		workloadGroup.GET(":uid/trainingPerformance", GetWorkloadTrainingPerformance)
		// Training performance new APIs
		workloadGroup.GET(":uid/metrics/sources", GetDataSources)
		workloadGroup.GET(":uid/metrics/available", GetAvailableMetrics)
		workloadGroup.GET(":uid/metrics/data", GetMetricsData)
		workloadGroup.GET(":uid/metrics/iteration-times", GetIterationTimes)
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
		// Metadata queries
		gpuAggregationGroup.GET("/clusters", getClusters)
		gpuAggregationGroup.GET("/namespaces", getNamespaces)
		gpuAggregationGroup.GET("/dimension-keys", getDimensionKeys)
		gpuAggregationGroup.GET("/dimension-values", getDimensionValues)

		// Cluster-level hourly stats
		gpuAggregationGroup.GET("/cluster/hourly-stats", getClusterHourlyStats)

		// Namespace-level hourly stats
		gpuAggregationGroup.GET("/namespaces/hourly-stats", getNamespaceHourlyStats)

		// Label/Annotation-level hourly stats
		gpuAggregationGroup.GET("/labels/hourly-stats", getLabelHourlyStats)

		// Workload-level hourly stats
		gpuAggregationGroup.GET("/workloads/hourly-stats", getWorkloadHourlyStats)

		// Snapshot queries
		gpuAggregationGroup.GET("/snapshots/latest", getLatestSnapshot)
		gpuAggregationGroup.GET("/snapshots", listSnapshots)
	}

	// Job Execution History routes - Job execution history query
	jobHistoryGroup := group.Group("/job-execution-histories")
	{
		// Get recent failure records - must be defined before :id
		jobHistoryGroup.GET("/recent-failures", GetRecentFailures)
		// Get job statistics - must be defined before :id
		jobHistoryGroup.GET("/statistics/:job_name", GetJobStatistics)
		// List query - supports multiple filter conditions
		jobHistoryGroup.GET("", ListJobExecutionHistories)
		// Get details
		jobHistoryGroup.GET("/:id", GetJobExecutionHistory)
	}

	// AI Workload Metadata routes - AI workload metadata management with conflict detection
	aiMetadataGroup := group.Group("/ai-workload-metadata")
	{
		// List all AI workload metadata
		aiMetadataGroup.GET("", ListAiWorkloadMetadata)
		// Get specific AI workload metadata by workload UID
		aiMetadataGroup.GET("/:workload_uid", GetAiWorkloadMetadata)
		// Annotate workload framework (user annotation)
		aiMetadataGroup.POST("/:workload_uid/annotate", AnnotateWorkloadFramework)
		// Update AI workload metadata (full update)
		aiMetadataGroup.PUT("/:workload_uid", UpdateAiWorkloadMetadata)
		// Delete AI workload metadata
		aiMetadataGroup.DELETE("/:workload_uid", DeleteAiWorkloadMetadata)
		// Get detection conflict logs for a specific workload
		aiMetadataGroup.GET("/:workload_uid/conflicts", GetDetectionConflictLogs)
	}

	// Detection Conflict routes - Detection conflict logs query
	conflictGroup := group.Group("/detection-conflicts")
	{
		// List all recent detection conflicts across workloads
		conflictGroup.GET("", ListAllDetectionConflicts)
	}

	// Weekly Report routes - GPU usage weekly reports
	weeklyReportGroup := group.Group("/weekly-reports/gpu_utilization")
	{
		// Get latest report - must be defined before :id
		weeklyReportGroup.GET("/latest", GetLatestWeeklyReport)
		// List reports with pagination
		weeklyReportGroup.GET("", ListWeeklyReports)
		// Get report metadata
		weeklyReportGroup.GET("/:id", GetWeeklyReport)
		// Download report in different formats
		weeklyReportGroup.GET("/:id/html", DownloadWeeklyReportHTML)
		weeklyReportGroup.GET("/:id/pdf", DownloadWeeklyReportPDF)
		weeklyReportGroup.GET("/:id/json", DownloadWeeklyReportJSON)
	}

	// Detection Config routes - Framework log parsing configuration management
	detectionConfigGroup := group.Group("/detection-configs")
	{
		// Framework configuration management
		frameworkGroup := detectionConfigGroup.Group("/frameworks")
		{
			// List all enabled frameworks
			frameworkGroup.GET("", ListFrameworks)
			// Get specific framework configuration
			frameworkGroup.GET("/:name", GetFrameworkConfig)
			// Update framework configuration
			frameworkGroup.PUT("/:name", UpdateFrameworkConfig)
		}

		// Cache management
		cacheGroup := detectionConfigGroup.Group("/cache")
		{
			// Refresh configuration cache
			cacheGroup.POST("/refresh", RefreshDetectionConfigCache)
			// Get cache TTL
			cacheGroup.GET("/ttl", GetCacheTTL)
			// Set cache TTL
			cacheGroup.PUT("/ttl", SetCacheTTL)
		}
	}

	// Profiler file routes - Download profiler file content
	profilerGroup := group.Group("/profiler")
	{
		filesGroup := profilerGroup.Group("/files")
		{
			// List profiler files for a workload
			filesGroup.GET("", tracelens.ListProfilerFiles)
			// Get profiler file metadata
			filesGroup.GET("/:id", tracelens.GetProfilerFileInfo)
			// Download profiler file content
			filesGroup.GET("/:id/content", tracelens.GetProfilerFileContent)
		}
	}

	// TraceLens Session routes - On-demand trace analysis
	tracelensGroup := group.Group("/tracelens")
	{
		// Session management
		sessionsGroup := tracelensGroup.Group("/sessions")
		{
			// Create a new analysis session
			sessionsGroup.POST("", tracelens.CreateSession)
			// List active sessions
			sessionsGroup.GET("", tracelens.ListActiveSessions)
			// Get session statistics
			sessionsGroup.GET("/stats", tracelens.GetSessionStats)
			// Get a specific session
			sessionsGroup.GET("/:session_id", tracelens.GetSession)
			// Extend session TTL
			sessionsGroup.POST("/:session_id/extend", tracelens.ExtendSession)
			// Delete a session
			sessionsGroup.DELETE("/:session_id", tracelens.DeleteSession)

			// Phase 4: UI Proxy - Proxy HTTP/WebSocket requests to TraceLens pod
			// Catch-all proxy for all UI paths (includes health check at /ui/health)
			sessionsGroup.Any("/:session_id/ui/*path", tracelens.ProxyUI)
		}
		// List sessions for a workload
		tracelensGroup.GET("/workloads/:workload_uid/sessions", tracelens.ListWorkloadSessions)
	}

	// Real-time Status routes - Real-time cluster status monitoring
	realtimeGroup := group.Group("/realtime")
	{
		// Get optimized real-time cluster status
		realtimeGroup.GET("/status", getRealtimeStatus)
		// Get currently running GPU tasks
		realtimeGroup.GET("/running-tasks", getRunningTasks)
	}

	// Detection Status routes - Framework detection status and task progress
	detectionStatusGroup := group.Group("/detection-status")
	{
		// Summary - must be defined before :workload_uid
		detectionStatusGroup.GET("/summary", GetDetectionSummary)
		// Log report endpoint (for telemetry-processor)
		detectionStatusGroup.POST("/log-report", ReportLogDetection)
		// List all detection statuses
		detectionStatusGroup.GET("", ListDetectionStatuses)
		// Get detection status for a specific workload
		detectionStatusGroup.GET("/:workload_uid", GetDetectionStatus)
		// Get coverage for a workload
		detectionStatusGroup.GET("/:workload_uid/coverage", GetDetectionCoverage)
		// Initialize coverage for a workload
		detectionStatusGroup.POST("/:workload_uid/coverage/initialize", InitializeDetectionCoverage)
		// Get uncovered log window
		detectionStatusGroup.GET("/:workload_uid/coverage/log-gap", GetUncoveredLogWindow)
		// Get detection tasks for a workload
		detectionStatusGroup.GET("/:workload_uid/tasks", GetDetectionTasks)
		// Get evidence for a workload
		detectionStatusGroup.GET("/:workload_uid/evidence", GetDetectionEvidence)
		// Manually trigger detection
		detectionStatusGroup.POST("/:workload_uid/trigger", TriggerDetection)
	}

	return nil
}
