// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/perfetto"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/pyspy"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/registry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/sysconfig"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/tracelens"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/mcp/unified"
	"github.com/gin-gonic/gin"
)

// getUnifiedHandler returns the unified handler for a given path, or nil if not found.
func getUnifiedHandler(path string) gin.HandlerFunc {
	ep := unified.GetRegistry().GetEndpointByPath(path)
	if ep != nil {
		return ep.GetGinHandler()
	}
	return nil
}

func RegisterRouter(group *gin.RouterGroup) error {
	nodeGroup := group.Group("/nodes")
	{
		// Phase 2-4 Unified: Node endpoints
		nodeGroup.GET("gpuAllocation", getUnifiedHandler("/nodes/gpuAllocation"))
		nodeGroup.GET("gpuUtilization", getUnifiedHandler("/nodes/gpuUtilization"))
		nodeGroup.GET("gpuUtilizationHistory", getUnifiedHandler("/nodes/gpuUtilizationHistory"))
		nodeGroup.GET("", getUnifiedHandler("/nodes"))
		// Not yet migrated
		nodeGroup.GET("fragmentation-analysis", getFragmentationAnalysis)
		nodeGroup.GET("load-balance-analysis", getLoadBalanceAnalysis)
		nodeGroup.GET(":name", getUnifiedHandler("/nodes/:name"))
		nodeGroup.GET(":name/fragmentation", getNodeFragmentation)
		nodeGroup.GET(":name/gpuDevices", getUnifiedHandler("/nodes/:name/gpuDevices"))
		// Phase 4 Unified: Node metrics and workloads
		nodeGroup.GET(":name/gpuMetrics", getUnifiedHandler("/nodes/:name/gpuMetrics"))
		nodeGroup.GET(":name/utilization", getUnifiedHandler("/nodes/:name/utilization"))
		nodeGroup.GET(":name/utilizationHistory", getUnifiedHandler("/nodes/:name/utilizationHistory"))
		nodeGroup.GET(":name/workloads", getUnifiedHandler("/nodes/:name/workloads"))
		nodeGroup.GET(":name/workloadsHistory", getUnifiedHandler("/nodes/:name/workloadsHistory"))
	}

	// Pod routes - Phase 3-5 Unified
	podGroup := group.Group("/pods")
	{
		podGroup.GET("/stats", getUnifiedHandler("/pods/stats"))
		// Phase 5 Unified: Pod comparison (must be before /:pod_uid to avoid conflict)
		podGroup.GET("/comparison", getUnifiedHandler("/pods/comparison"))
		podGroup.GET("/:pod_uid", getUnifiedHandler("/pods/:pod_uid"))
		// Phase 5 Unified: Pod GPU history and events
		podGroup.GET("/:pod_uid/gpu-history", getUnifiedHandler("/pods/:pod_uid/gpu-history"))
		podGroup.GET("/:pod_uid/events", getUnifiedHandler("/pods/:pod_uid/events"))
	}
	// Phase 2 Unified: Cluster endpoints
	clusterGroup := group.Group("/clusters")
	{
		clusterGroup.GET("overview", getUnifiedHandler("/clusters/overview"))
		clusterGroup.GET("consumers", getUnifiedHandler("/clusters/consumers"))
		clusterGroup.GET("gpuHeatmap", getUnifiedHandler("/clusters/gpuHeatmap"))
	}
	// Phase 3-5 Unified: Workload endpoints
	workloadGroup := group.Group("/workloads")
	{
		workloadGroup.GET("", getUnifiedHandler("/workloads"))
		// Phase 5 Unified: Workload statistics, hierarchy query, GPU history
		workloadGroup.GET("statistic", getUnifiedHandler("/workloads/statistic"))
		workloadGroup.GET("hierarchy", getUnifiedHandler("/workloads/hierarchy"))
		workloadGroup.GET("gpuUtilizationHistory", getUnifiedHandler("/workloads/gpuUtilizationHistory"))
		// Not yet migrated
		workloadGroup.GET(":uid/hierarchy", getWorkloadHierarchy)
		workloadGroup.GET(":uid", getUnifiedHandler("/workloads/:uid"))
		workloadGroup.GET(":uid/metrics", getWorkloadMetrics)
		workloadGroup.GET(":uid/trainingPerformance", GetWorkloadTrainingPerformance)
		// Training performance new APIs
		workloadGroup.GET(":uid/metrics/sources", GetDataSources)
		workloadGroup.GET(":uid/metrics/available", GetAvailableMetrics)
		workloadGroup.GET(":uid/metrics/data", GetMetricsData)
		workloadGroup.GET(":uid/metrics/iteration-times", GetIterationTimes)
		// Process tree API for py-spy profiling
		workloadGroup.POST(":uid/process-tree", pyspy.GetProcessTree)
	}
	group.GET("workloadMetadata", getWorkloadsMetadata)
	storageGroup := group.Group("/storage")
	{
		storageGroup.GET("stat", getStorageStat)
	}

	// Alert Event routes - Alert events query and analysis
	alertsGroup := group.Group("/alerts")
	{
		// Summary and trend endpoints - must be defined before :id
		alertsGroup.GET("/summary", GetAlertSummary)
		alertsGroup.GET("/trend", GetAlertTrend)
		alertsGroup.GET("/top-sources", GetTopAlertSources)
		alertsGroup.GET("/by-cluster", GetAlertsByCluster)
		// List alerts
		alertsGroup.GET("", ListAlertEvents)
		// Get alert by ID
		alertsGroup.GET("/:id", GetAlertEvent)
		// Get alert correlations
		alertsGroup.GET("/:id/correlations", GetAlertCorrelations)
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

	// Notification Channel routes - Reusable notification channel configurations
	notificationChannelGroup := group.Group("/notification-channels")
	{
		notificationChannelGroup.GET("/types", GetChannelTypes)
		notificationChannelGroup.POST("", CreateNotificationChannel)
		notificationChannelGroup.GET("", ListNotificationChannels)
		notificationChannelGroup.GET(":id", GetNotificationChannel)
		notificationChannelGroup.PUT(":id", UpdateNotificationChannel)
		notificationChannelGroup.DELETE(":id", DeleteNotificationChannel)
		notificationChannelGroup.POST(":id/test", TestNotificationChannel)
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
		// Get available resource profiles
		tracelensGroup.GET("/resource-profiles", tracelens.GetResourceProfiles)

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
			sessionsGroup.PATCH("/:session_id", tracelens.ExtendSession)
			// Delete a session
			sessionsGroup.DELETE("/:session_id", tracelens.DeleteSession)

			// Phase 4: UI Proxy - Proxy HTTP/WebSocket requests to TraceLens pod
			// Catch-all proxy for all UI paths (includes health check at /ui/health)
			sessionsGroup.Any("/:session_id/ui/*path", tracelens.ProxyUI)
		}
		// List sessions for a workload
		tracelensGroup.GET("/workloads/:workload_uid/sessions", tracelens.ListWorkloadSessions)
	}

	// Perfetto Viewer routes - Lightweight trace visualization
	perfettoGroup := group.Group("/perfetto")
	{
		// Session management
		perfettoSessionsGroup := perfettoGroup.Group("/sessions")
		{
			// Create a new Perfetto viewer session
			perfettoSessionsGroup.POST("", perfetto.CreateSession)
			// Get a specific session
			perfettoSessionsGroup.GET("/:session_id", perfetto.GetSession)
			// Extend session TTL
			perfettoSessionsGroup.PATCH("/:session_id", perfetto.ExtendSession)
			// Delete a session
			perfettoSessionsGroup.DELETE("/:session_id", perfetto.DeleteSession)

			// UI Proxy - Proxy HTTP/WebSocket requests to Perfetto pod
			perfettoSessionsGroup.Any("/:session_id/ui/*path", perfetto.ProxyUI)
		}
	}

	// Container Registry Configuration routes - Per-cluster image registry settings
	registryGroup := group.Group("/registry")
	{
		// Get current registry configuration
		registryGroup.GET("/config", registry.GetRegistryConfig)
		// Set registry configuration
		registryGroup.PUT("/config", registry.SetRegistryConfig)
		// Sync configuration from Harbor external URL
		registryGroup.POST("/sync-from-harbor", registry.SyncFromHarbor)
		// Get image URL for a specific image
		registryGroup.GET("/image-url", registry.GetImageURL)
	}

	// System Configuration routes - General system configuration management
	sysconfigGroup := group.Group("/system-config")
	{
		// List all configurations
		sysconfigGroup.GET("", sysconfig.ListConfigs)
		// Get a specific configuration
		sysconfigGroup.GET("/:key", sysconfig.GetConfig)
		// Set a configuration
		sysconfigGroup.PUT("/:key", sysconfig.SetConfig)
		// Delete a configuration
		sysconfigGroup.DELETE("/:key", sysconfig.DeleteConfig)
		// Get configuration history
		sysconfigGroup.GET("/:key/history", sysconfig.GetConfigHistory)
	}

	// Real-time Status routes - Phase 3 Unified
	realtimeGroup := group.Group("/realtime")
	{
		realtimeGroup.GET("/status", getUnifiedHandler("/realtime/status"))
		// Not yet migrated
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

	// Py-Spy Profiling routes - Python profiling via py-spy
	pyspyGroup := group.Group("/pyspy")
	{
		// Create a new py-spy sampling task
		pyspyGroup.POST("/sample", pyspy.CreateTask)
		// List py-spy tasks with filters
		pyspyGroup.POST("/tasks", pyspy.ListTasks)
		// Get a specific task by ID
		pyspyGroup.GET("/task/:id", pyspy.GetTask)
		// Cancel a task
		pyspyGroup.POST("/task/:id/cancel", pyspy.CancelTask)
		// List files for a task
		pyspyGroup.GET("/file/:task_id", pyspy.GetTaskFiles)
		// Download a specific file (proxy to node-exporter)
		pyspyGroup.GET("/file/:task_id/:filename", pyspy.DownloadFile)
	}

	// GitHub Workflow Metrics routes - GitHub workflow metrics collection
	githubWorkflowMetricsGroup := group.Group("/github-workflow-metrics")
	{
		// Config management
		configsGroup := githubWorkflowMetricsGroup.Group("/configs")
		{
			configsGroup.POST("", CreateGithubWorkflowConfig)
			configsGroup.GET("", ListGithubWorkflowConfigs)
			configsGroup.GET("/:id", GetGithubWorkflowConfig)
			configsGroup.PUT("/:id", UpdateGithubWorkflowConfig)
			configsGroup.PATCH("/:id", PatchGithubWorkflowConfig)
			configsGroup.DELETE("/:id", DeleteGithubWorkflowConfig)
			// Runs for a config
			configsGroup.GET("/:id/runs", ListGithubWorkflowRuns)
			// Schemas for a config
			configsGroup.POST("/:id/schemas", CreateGithubWorkflowSchema)
			configsGroup.GET("/:id/schemas", ListGithubWorkflowSchemas)
			// Get active schema for a config
			configsGroup.GET("/:id/schemas/active", GetActiveGithubWorkflowSchema)
			// AI Schema generation
			configsGroup.POST("/:id/schemas/regenerate", RegenerateGithubWorkflowSchema)
			configsGroup.POST("/:id/schemas/preview", PreviewSchemaExtraction)
			// Metrics for a config
			configsGroup.GET("/:id/metrics", ListGithubWorkflowMetrics)
			// Advanced metrics query with JSONB dimension filtering
			configsGroup.POST("/:id/metrics/query", QueryGithubWorkflowMetricsAdvanced)
			// Metrics aggregation by time interval
			configsGroup.POST("/:id/metrics/aggregate", GetGithubWorkflowMetricsAggregation)
			// Metrics time-series trends
			configsGroup.POST("/:id/metrics/trends", GetGithubWorkflowMetricsTrends)
			// Stats for a config
			configsGroup.GET("/:id/stats", GetGithubWorkflowMetricsStats)
			// Summary statistics for a config
			configsGroup.GET("/:id/summary", GetGithubWorkflowMetricsSummary)
			// Available dimensions with values
			configsGroup.GET("/:id/dimensions", GetGithubWorkflowMetricsDimensions)
			// Single dimension values
			configsGroup.GET("/:id/dimensions/:dimension/values", GetSingleDimensionValues)
			// Available fields (dimension fields and metric fields)
			configsGroup.GET("/:id/fields", GetGithubWorkflowMetricsFields)
			// Export metrics as CSV
			configsGroup.GET("/:id/export", ExportGithubWorkflowMetrics)
			// Backfill APIs
			configsGroup.POST("/:id/backfill", TriggerBackfill)
			configsGroup.GET("/:id/backfill/status", GetBackfillStatus)
			configsGroup.POST("/:id/backfill/cancel", CancelBackfill)
			configsGroup.GET("/:id/backfill/tasks", ListBackfillTasks)
			// Batch retry failed runs
			configsGroup.POST("/:id/runs/batch-retry", RetryFailedRuns)
			// List completed EphemeralRunners for a config
			configsGroup.GET("/:id/runners", ListEphemeralRunners)
		// Dashboard APIs
		configsGroup.GET("/:id/dashboard", GetDashboardSummary)
		configsGroup.GET("/:id/dashboard/builds", GetDashboardRecentBuilds)
		configsGroup.POST("/:id/dashboard/refresh", RefreshDashboardSummary)
		// Note: Insights are now available via Chat Agent, not as a fixed API
		// Commit analysis API
			configsGroup.GET("/:id/commits/stats", GetCommitStats)
		}
		// Run management (global)
		runsGroup := githubWorkflowMetricsGroup.Group("/runs")
		{
			// List all runs globally (must be defined before /:id)
			runsGroup.GET("", ListAllGithubWorkflowRuns)
			runsGroup.GET("/:id", GetGithubWorkflowRun)
			runsGroup.GET("/:id/metrics", GetGithubWorkflowMetricsByRun)
			// Run detail with commits and performance comparison
			runsGroup.GET("/:id/detail", GetRunDetail)
			// Retry single run
			runsGroup.POST("/:id/retry", RetryGithubWorkflowRun)
		}
		// Schema details
		schemasGroup := githubWorkflowMetricsGroup.Group("/schemas")
		{
			schemasGroup.GET("/:id", GetGithubWorkflowSchema)
			schemasGroup.POST("/:id/activate", SetGithubWorkflowSchemaActive)
		}
	}

	// GitHub Runners management - AutoScalingRunnerSet discovery and analytics
	githubRunnersGroup := group.Group("/github-runners")
	{
		// Runner Sets - discovered AutoScalingRunnerSets
		runnerSetsGroup := githubRunnersGroup.Group("/runner-sets")
		{
			// List all runner sets (with optional stats)
			runnerSetsGroup.GET("", ListGithubRunnerSets)
			// Get runner set by namespace/name
			runnerSetsGroup.GET("/:namespace/:name", GetGithubRunnerSet)
			// Get runner set by ID
			runnerSetsGroup.GET("/by-id/:id", GetGithubRunnerSetByID)
			// Get runs for a runner set
			runnerSetsGroup.GET("/by-id/:id/runs", ListRunsByRunnerSet)
			// Get config for a runner set (may return null)
			runnerSetsGroup.GET("/by-id/:id/config", GetConfigByRunnerSet)
			// Get statistics for a runner set
			runnerSetsGroup.GET("/by-id/:id/stats", GetStatsByRunnerSet)
			// Create config for a runner set
			runnerSetsGroup.POST("/by-id/:id/config", CreateConfigForRunnerSet)
		}
	}

	// Add commit and workflow run details endpoints to existing runs group
	// These are added to the github-workflow-metrics group
	githubWorkflowMetricsGroupV2 := group.Group("/github-workflow-metrics")
	{
		// Config-level analytics and history
		configsGroupV2 := githubWorkflowMetricsGroupV2.Group("/configs")
		{
			// Workflow analytics (execution counts, average times, etc.)
			configsGroupV2.GET("/:id/analytics", GetGithubWorkflowAnalytics)
			// Detailed execution history with commit and run details
			configsGroupV2.GET("/:id/history", GetGithubWorkflowRunHistory)
		}
		// Run-level commit and details
		runsGroupV2 := githubWorkflowMetricsGroupV2.Group("/runs")
		{
			// Get commit details for a run
			runsGroupV2.GET("/:id/commit", GetGithubWorkflowRunCommit)
			// Get workflow run details from GitHub
			runsGroupV2.GET("/:id/details", GetGithubWorkflowRunDetailsAPI)
		}
	}

	return nil
}
