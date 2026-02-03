// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/cluster"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/cpconfig"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/perfetto"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/pyspy"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/registry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/api/pkg/api/release"
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
		// Node analysis endpoints - complex, not migrated
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
	// Phase 3-6 Unified: Workload endpoints
	workloadGroup := group.Group("/workloads")
	{
		workloadGroup.GET("", getUnifiedHandler("/workloads"))
		// Phase 5 Unified: Workload statistics, hierarchy query, GPU history
		workloadGroup.GET("statistic", getUnifiedHandler("/workloads/statistic"))
		workloadGroup.GET("hierarchy", getUnifiedHandler("/workloads/hierarchy"))
		workloadGroup.GET("gpuUtilizationHistory", getUnifiedHandler("/workloads/gpuUtilizationHistory"))
		// Phase 6 Unified: Workload hierarchy by UID, metrics, training performance
		workloadGroup.GET(":uid/hierarchy", getUnifiedHandler("/workloads/:uid/hierarchy"))
		workloadGroup.GET(":uid", getUnifiedHandler("/workloads/:uid"))
		workloadGroup.GET(":uid/metrics", getUnifiedHandler("/workloads/:uid/metrics"))
		workloadGroup.GET(":uid/trainingPerformance", getUnifiedHandler("/workloads/:uid/trainingPerformance"))
		// Phase 6 Unified: Training performance new APIs
		workloadGroup.GET(":uid/metrics/sources", getUnifiedHandler("/workloads/:uid/metrics/sources"))
		workloadGroup.GET(":uid/metrics/available", getUnifiedHandler("/workloads/:uid/metrics/available"))
		workloadGroup.GET(":uid/metrics/data", getUnifiedHandler("/workloads/:uid/metrics/data"))
		workloadGroup.GET(":uid/metrics/iteration-times", getUnifiedHandler("/workloads/:uid/metrics/iteration-times"))
		// Process tree API for py-spy profiling
		workloadGroup.POST(":uid/process-tree", pyspy.GetProcessTree)
	}
	// Phase 6 Unified: Workload metadata
	group.GET("workloadMetadata", getUnifiedHandler("/workloadMetadata"))
	storageGroup := group.Group("/storage")
	{
		storageGroup.GET("stat", getUnifiedHandler("/storage/stat"))
	}

	// Phase 7 Unified: Alert Event routes - Alert events query and analysis
	alertsGroup := group.Group("/alerts")
	{
		// Summary and trend endpoints - must be defined before :id
		alertsGroup.GET("/summary", getUnifiedHandler("/alerts/summary"))
		alertsGroup.GET("/trend", getUnifiedHandler("/alerts/trend"))
		alertsGroup.GET("/top-sources", getUnifiedHandler("/alerts/top-sources"))
		alertsGroup.GET("/by-cluster", getUnifiedHandler("/alerts/by-cluster"))
		// List alerts
		alertsGroup.GET("", getUnifiedHandler("/alerts"))
		// Get alert by ID
		alertsGroup.GET("/:id", getUnifiedHandler("/alerts/:id"))
		// Get alert correlations
		alertsGroup.GET("/:id/correlations", getUnifiedHandler("/alerts/:id/correlations"))
	}

	// Phase 7 Unified: Metric Alert Rule management routes
	metricAlertRuleGroup := group.Group("/metric-alert-rules")
	{
		metricAlertRuleGroup.POST("", CreateMetricAlertRule)
		metricAlertRuleGroup.GET("", getUnifiedHandler("/metric-alert-rules"))
		metricAlertRuleGroup.GET(":id", getUnifiedHandler("/metric-alert-rules/:id"))
		metricAlertRuleGroup.PUT(":id", UpdateMetricAlertRule)
		metricAlertRuleGroup.DELETE(":id", DeleteMetricAlertRule)
		metricAlertRuleGroup.POST(":id/clone", CloneMetricAlertRule)
		metricAlertRuleGroup.POST("sync", SyncMetricAlertRulesToCluster)
		metricAlertRuleGroup.GET(":id/status", getUnifiedHandler("/metric-alert-rules/:id/status"))
	}

	// Phase 7 Unified: Log Alert Rule management routes
	logAlertRuleGroup := group.Group("/log-alert-rules")
	{
		logAlertRuleGroup.POST("", CreateLogAlertRule)
		logAlertRuleGroup.GET("", getUnifiedHandler("/log-alert-rules"))
		logAlertRuleGroup.GET("/multi-cluster", getUnifiedHandler("/log-alert-rules/multi-cluster"))
		logAlertRuleGroup.GET(":id", getUnifiedHandler("/log-alert-rules/:id"))
		logAlertRuleGroup.PUT(":id", UpdateLogAlertRule)
		logAlertRuleGroup.DELETE(":id", DeleteLogAlertRule)
		logAlertRuleGroup.POST("/batch-update", BatchUpdateLogAlertRules)
		logAlertRuleGroup.POST("/test", TestLogAlertRule)
		logAlertRuleGroup.GET(":id/statistics", getUnifiedHandler("/log-alert-rules/:id/statistics"))
		logAlertRuleGroup.GET(":id/versions", getUnifiedHandler("/log-alert-rules/:id/versions"))
		logAlertRuleGroup.POST(":id/rollback/:version", RollbackLogAlertRule)
		logAlertRuleGroup.POST(":id/clone", CloneLogAlertRule)
	}

	// Log Alert Rule Template routes - Phase 10.5: GET endpoints migrated
	logAlertRuleTemplateGroup := group.Group("/log-alert-rule-templates")
	{
		logAlertRuleTemplateGroup.POST("", CreateLogAlertRuleTemplate)
		logAlertRuleTemplateGroup.GET("", getUnifiedHandler("/log-alert-rule-templates"))
		logAlertRuleTemplateGroup.GET(":id", getUnifiedHandler("/log-alert-rule-templates/:id"))
		logAlertRuleTemplateGroup.DELETE(":id", DeleteLogAlertRuleTemplate)
		logAlertRuleTemplateGroup.POST(":id/instantiate", CreateRuleFromTemplate)
	}

	// Alert Rule Advice routes - Phase 10.5: GET endpoints migrated
	alertRuleAdviceGroup := group.Group("/alert-rule-advices")
	{
		alertRuleAdviceGroup.POST("", CreateAlertRuleAdvice)
		alertRuleAdviceGroup.POST("/batch", BatchCreateAlertRuleAdvices)
		alertRuleAdviceGroup.GET("", getUnifiedHandler("/alert-rule-advices"))
		alertRuleAdviceGroup.GET("/summary", getUnifiedHandler("/alert-rule-advices/summary"))
		alertRuleAdviceGroup.GET("/statistics", getUnifiedHandler("/alert-rule-advices/statistics"))
		alertRuleAdviceGroup.GET(":id", getUnifiedHandler("/alert-rule-advices/:id"))
		alertRuleAdviceGroup.PUT(":id", UpdateAlertRuleAdvice)
		alertRuleAdviceGroup.DELETE(":id", DeleteAlertRuleAdvice)
		alertRuleAdviceGroup.POST("/batch-delete", BatchDeleteAlertRuleAdvices)
		alertRuleAdviceGroup.POST(":id/status", UpdateAdviceStatus)
		alertRuleAdviceGroup.POST("/batch-status", BatchUpdateAdviceStatus)
		alertRuleAdviceGroup.POST(":id/apply", ApplyAlertRuleAdvice)
	}

	// Notification Channel routes - Phase 10.5: GET endpoints migrated
	notificationChannelGroup := group.Group("/notification-channels")
	{
		notificationChannelGroup.GET("/types", getUnifiedHandler("/notification-channels/types"))
		notificationChannelGroup.POST("", CreateNotificationChannel)
		notificationChannelGroup.GET("", getUnifiedHandler("/notification-channels"))
		notificationChannelGroup.GET(":id", getUnifiedHandler("/notification-channels/:id"))
		notificationChannelGroup.PUT(":id", UpdateNotificationChannel)
		notificationChannelGroup.DELETE(":id", DeleteNotificationChannel)
		notificationChannelGroup.POST(":id/test", TestNotificationChannel)
	}

	// Phase 8 Unified: GPU Aggregation routes - GPU aggregation data query
	gpuAggregationGroup := group.Group("/gpu-aggregation")
	{
		// Metadata queries
		gpuAggregationGroup.GET("/clusters", getUnifiedHandler("/gpu-aggregation/clusters"))
		gpuAggregationGroup.GET("/namespaces", getUnifiedHandler("/gpu-aggregation/namespaces"))
		gpuAggregationGroup.GET("/dimension-keys", getUnifiedHandler("/gpu-aggregation/dimension-keys"))
		gpuAggregationGroup.GET("/dimension-values", getUnifiedHandler("/gpu-aggregation/dimension-values"))

		// Cluster-level hourly stats
		gpuAggregationGroup.GET("/cluster/hourly-stats", getUnifiedHandler("/gpu-aggregation/cluster/hourly-stats"))

		// Namespace-level hourly stats
		gpuAggregationGroup.GET("/namespaces/hourly-stats", getUnifiedHandler("/gpu-aggregation/namespaces/hourly-stats"))

		// Label/Annotation-level hourly stats
		gpuAggregationGroup.GET("/labels/hourly-stats", getUnifiedHandler("/gpu-aggregation/labels/hourly-stats"))

		// Workload-level hourly stats
		gpuAggregationGroup.GET("/workloads/hourly-stats", getUnifiedHandler("/gpu-aggregation/workloads/hourly-stats"))

		// Snapshot queries
		gpuAggregationGroup.GET("/snapshots/latest", getUnifiedHandler("/gpu-aggregation/snapshots/latest"))
		gpuAggregationGroup.GET("/snapshots", getUnifiedHandler("/gpu-aggregation/snapshots"))
	}

	// Phase 8 Unified: Job Execution History routes - Job execution history query
	jobHistoryGroup := group.Group("/job-execution-histories")
	{
		// Get recent failure records - must be defined before :id
		jobHistoryGroup.GET("/recent-failures", getUnifiedHandler("/job-execution-histories/recent-failures"))
		// Get job statistics - must be defined before :id
		jobHistoryGroup.GET("/statistics/:job_name", getUnifiedHandler("/job-execution-histories/statistics/:job_name"))
		// List query - supports multiple filter conditions
		jobHistoryGroup.GET("", getUnifiedHandler("/job-execution-histories"))
		// Get details
		jobHistoryGroup.GET("/:id", getUnifiedHandler("/job-execution-histories/:id"))
	}

	// Phase 8 Unified (GET only): AI Workload Metadata routes - AI workload metadata management with conflict detection
	aiMetadataGroup := group.Group("/ai-workload-metadata")
	{
		// List all AI workload metadata
		aiMetadataGroup.GET("", getUnifiedHandler("/ai-workload-metadata"))
		// Get specific AI workload metadata by workload UID
		aiMetadataGroup.GET("/:workload_uid", getUnifiedHandler("/ai-workload-metadata/:workload_uid"))
		// Annotate workload framework (user annotation) - POST not migrated
		aiMetadataGroup.POST("/:workload_uid/annotate", AnnotateWorkloadFramework)
		// Update AI workload metadata (full update) - PUT not migrated
		aiMetadataGroup.PUT("/:workload_uid", UpdateAiWorkloadMetadata)
		// Delete AI workload metadata - DELETE not migrated
		aiMetadataGroup.DELETE("/:workload_uid", DeleteAiWorkloadMetadata)
		// Get detection conflict logs for a specific workload
		aiMetadataGroup.GET("/:workload_uid/conflicts", getUnifiedHandler("/ai-workload-metadata/:workload_uid/conflicts"))
	}

	// Phase 8 Unified: Detection Conflict routes - Detection conflict logs query
	conflictGroup := group.Group("/detection-conflicts")
	{
		// List all recent detection conflicts across workloads
		conflictGroup.GET("", getUnifiedHandler("/detection-conflicts"))
	}

	// Phase 8 Unified: Weekly Report routes - GPU usage weekly reports
	weeklyReportGroup := group.Group("/weekly-reports/gpu_utilization")
	{
		// Get latest report - must be defined before :id
		weeklyReportGroup.GET("/latest", getUnifiedHandler("/weekly-reports/gpu_utilization/latest"))
		// List reports with pagination
		weeklyReportGroup.GET("", getUnifiedHandler("/weekly-reports/gpu_utilization"))
		// Get report metadata
		weeklyReportGroup.GET("/:id", getUnifiedHandler("/weekly-reports/gpu_utilization/:id"))
		// Download report in different formats (HTML/PDF not migrated - binary content)
		weeklyReportGroup.GET("/:id/html", DownloadWeeklyReportHTML)
		weeklyReportGroup.GET("/:id/pdf", DownloadWeeklyReportPDF)
		weeklyReportGroup.GET("/:id/json", getUnifiedHandler("/weekly-reports/gpu_utilization/:id/json"))
	}

	// Phase 8 Unified (GET only): Detection Config routes - Framework log parsing configuration management
	detectionConfigGroup := group.Group("/detection-configs")
	{
		// Framework configuration management
		frameworkGroup := detectionConfigGroup.Group("/frameworks")
		{
			// List all enabled frameworks
			frameworkGroup.GET("", getUnifiedHandler("/detection-configs/frameworks"))
			// Get specific framework configuration
			frameworkGroup.GET("/:name", getUnifiedHandler("/detection-configs/frameworks/:name"))
			// Update framework configuration - PUT not migrated
			frameworkGroup.PUT("/:name", UpdateFrameworkConfig)
		}

		// Cache management
		cacheGroup := detectionConfigGroup.Group("/cache")
		{
			// Refresh configuration cache - POST not migrated
			cacheGroup.POST("/refresh", RefreshDetectionConfigCache)
			// Get cache TTL
			cacheGroup.GET("/ttl", getUnifiedHandler("/detection-configs/cache/ttl"))
			// Set cache TTL - PUT not migrated
			cacheGroup.PUT("/ttl", SetCacheTTL)
		}
	}

	// Phase 9 Unified: Profiler file routes
	profilerGroup := group.Group("/profiler")
	{
		filesGroup := profilerGroup.Group("/files")
		{
			// List profiler files for a workload - migrated
			filesGroup.GET("", getUnifiedHandler("/profiler/files"))
			// Get profiler file metadata - migrated
			filesGroup.GET("/:id", getUnifiedHandler("/profiler/files/:id"))
			// Download profiler file content (binary - NOT migrated)
			filesGroup.GET("/:id/content", tracelens.GetProfilerFileContent)
		}
	}

	// Phase 9+10 Unified: TraceLens Session routes - On-demand trace analysis
	tracelensGroup := group.Group("/tracelens")
	{
		// Get available resource profiles
		tracelensGroup.GET("/resource-profiles", getUnifiedHandler("/tracelens/resource-profiles"))

		// Session management
		sessionsGroup := tracelensGroup.Group("/sessions")
		{
			// Phase 10: Create a new analysis session - migrated
			sessionsGroup.POST("", getUnifiedHandler("/tracelens/sessions"))
			// List active sessions
			sessionsGroup.GET("", getUnifiedHandler("/tracelens/sessions"))
			// Get session statistics
			sessionsGroup.GET("/stats", getUnifiedHandler("/tracelens/sessions/stats"))
			// Get a specific session
			sessionsGroup.GET("/:session_id", getUnifiedHandler("/tracelens/sessions/:session_id"))
			// Phase 10: Extend session TTL - migrated
			sessionsGroup.PATCH("/:session_id", getUnifiedHandler("/tracelens/sessions/:session_id"))
			// Phase 10: Delete a session - migrated
			sessionsGroup.DELETE("/:session_id", getUnifiedHandler("/tracelens/sessions/:session_id"))

			// UI Proxy - Proxy HTTP/WebSocket requests to TraceLens pod - NOT migrated (proxy)
			sessionsGroup.Any("/:session_id/ui/*path", tracelens.ProxyUI)
		}
		// List sessions for a workload
		tracelensGroup.GET("/workloads/:workload_uid/sessions", getUnifiedHandler("/tracelens/workloads/:workload_uid/sessions"))
	}

	// Phase 9+10 Unified: Perfetto Viewer routes - Lightweight trace visualization
	perfettoGroup := group.Group("/perfetto")
	{
		// Session management
		perfettoSessionsGroup := perfettoGroup.Group("/sessions")
		{
			// Phase 10: Create a new Perfetto viewer session - migrated
			perfettoSessionsGroup.POST("", getUnifiedHandler("/perfetto/sessions"))
			// Get a specific session - migrated
			perfettoSessionsGroup.GET("/:session_id", getUnifiedHandler("/perfetto/sessions/:session_id"))
			// Phase 10: Extend session TTL - migrated
			perfettoSessionsGroup.PATCH("/:session_id", getUnifiedHandler("/perfetto/sessions/:session_id"))
			// Phase 10: Delete a session - migrated
			perfettoSessionsGroup.DELETE("/:session_id", getUnifiedHandler("/perfetto/sessions/:session_id"))

			// UI Proxy - Proxy HTTP/WebSocket requests to Perfetto pod - NOT migrated (proxy)
			perfettoSessionsGroup.Any("/:session_id/ui/*path", perfetto.ProxyUI)
		}
	}

	// Phase 9 Unified (GET only): Container Registry Configuration routes - Per-cluster image registry settings
	registryGroup := group.Group("/registry")
	{
		// Get current registry configuration
		registryGroup.GET("/config", getUnifiedHandler("/registry/config"))
		// Set registry configuration - PUT not migrated
		registryGroup.PUT("/config", registry.SetRegistryConfig)
		// Sync configuration from Harbor external URL - POST not migrated
		registryGroup.POST("/sync-from-harbor", registry.SyncFromHarbor)
		// Get image URL for a specific image
		registryGroup.GET("/image-url", getUnifiedHandler("/registry/image-url"))
	}

	// Phase 9 Unified (GET only): System Configuration routes - General system configuration management
	sysconfigGroup := group.Group("/system-config")
	{
		// List all configurations
		sysconfigGroup.GET("", getUnifiedHandler("/system-config"))
		// Get a specific configuration
		sysconfigGroup.GET("/:key", getUnifiedHandler("/system-config/:key"))
		// Set a configuration - PUT not migrated
		sysconfigGroup.PUT("/:key", sysconfig.SetConfig)
		// Delete a configuration - DELETE not migrated
		sysconfigGroup.DELETE("/:key", sysconfig.DeleteConfig)
		// Get configuration history
		sysconfigGroup.GET("/:key/history", getUnifiedHandler("/system-config/:key/history"))
	}

	// Real-time Status routes - Phase 3+10.5 Unified
	realtimeGroup := group.Group("/realtime")
	{
		realtimeGroup.GET("/status", getUnifiedHandler("/realtime/status"))
		realtimeGroup.GET("/running-tasks", getUnifiedHandler("/realtime/running-tasks"))
	}
  
	// Phase 8 Unified (GET only): Detection Status routes - Framework detection status and task progress
	detectionStatusGroup := group.Group("/detection-status")
	{
		// Summary - must be defined before :workload_uid
		detectionStatusGroup.GET("/summary", getUnifiedHandler("/detection-status/summary"))
		// Log report endpoint (for telemetry-processor) - POST not migrated
		detectionStatusGroup.POST("/log-report", ReportLogDetection)
		// List all detection statuses
		detectionStatusGroup.GET("", getUnifiedHandler("/detection-status"))
		// Get detection status for a specific workload
		detectionStatusGroup.GET("/:workload_uid", getUnifiedHandler("/detection-status/:workload_uid"))
		// Get coverage for a workload
		detectionStatusGroup.GET("/:workload_uid/coverage", getUnifiedHandler("/detection-status/:workload_uid/coverage"))
		// Initialize coverage for a workload - POST not migrated
		detectionStatusGroup.POST("/:workload_uid/coverage/initialize", InitializeDetectionCoverage)
		// Get uncovered log window
		detectionStatusGroup.GET("/:workload_uid/coverage/log-gap", getUnifiedHandler("/detection-status/:workload_uid/coverage/log-gap"))
		// Get detection tasks for a workload
		detectionStatusGroup.GET("/:workload_uid/tasks", getUnifiedHandler("/detection-status/:workload_uid/tasks"))
		// Get evidence for a workload
		detectionStatusGroup.GET("/:workload_uid/evidence", getUnifiedHandler("/detection-status/:workload_uid/evidence"))
		// Manually trigger detection - POST not migrated
		detectionStatusGroup.POST("/:workload_uid/trigger", TriggerDetection)
	}

	// Phase 9 Unified (GET only): Py-Spy Profiling routes - Python profiling via py-spy
	pyspyGroup := group.Group("/pyspy")
	{
		// Create a new py-spy sampling task - POST not migrated
		pyspyGroup.POST("/sample", pyspy.CreateTask)
		// List py-spy tasks with filters - POST not migrated
		pyspyGroup.POST("/tasks", pyspy.ListTasks)
		// Get a specific task by ID
		pyspyGroup.GET("/task/:id", getUnifiedHandler("/pyspy/task/:id"))
		// Cancel a task - POST not migrated
		pyspyGroup.POST("/task/:id/cancel", pyspy.CancelTask)
		// List files for a task
		pyspyGroup.GET("/file/:task_id", getUnifiedHandler("/pyspy/file/:task_id"))
		// Download a specific file (binary - not migrated)
		pyspyGroup.GET("/file/:task_id/:filename", pyspy.DownloadFile)
	}

	// Phase 9 Unified (GET only): GitHub Workflow Metrics routes - GitHub workflow metrics collection
	githubWorkflowMetricsGroup := group.Group("/github-workflow-metrics")
	{
		// Config management
		configsGroup := githubWorkflowMetricsGroup.Group("/configs")
		{
			// POST/PUT/PATCH/DELETE not migrated
			configsGroup.POST("", CreateGithubWorkflowConfig)
			configsGroup.GET("", getUnifiedHandler("/github-workflow-metrics/configs"))
			configsGroup.GET("/:id", getUnifiedHandler("/github-workflow-metrics/configs/:id"))
			configsGroup.PUT("/:id", UpdateGithubWorkflowConfig)
			configsGroup.PATCH("/:id", PatchGithubWorkflowConfig)
			configsGroup.DELETE("/:id", DeleteGithubWorkflowConfig)
			// Runs for a config
			configsGroup.GET("/:id/runs", getUnifiedHandler("/github-workflow-metrics/configs/:id/runs"))
			// Schemas for a config
			configsGroup.POST("/:id/schemas", CreateGithubWorkflowSchema)
			configsGroup.GET("/:id/schemas", getUnifiedHandler("/github-workflow-metrics/configs/:id/schemas"))
			// Get active schema for a config
			configsGroup.GET("/:id/schemas/active", getUnifiedHandler("/github-workflow-metrics/configs/:id/schemas/active"))
			// AI Schema generation - POST not migrated
			configsGroup.POST("/:id/schemas/regenerate", RegenerateGithubWorkflowSchema)
			configsGroup.POST("/:id/schemas/preview", PreviewSchemaExtraction)
			// Metrics for a config
			configsGroup.GET("/:id/metrics", getUnifiedHandler("/github-workflow-metrics/configs/:id/metrics"))
			// Advanced metrics query with JSONB dimension filtering - POST not migrated
			configsGroup.POST("/:id/metrics/query", QueryGithubWorkflowMetricsAdvanced)
			// Metrics aggregation by time interval - POST not migrated
			configsGroup.POST("/:id/metrics/aggregate", GetGithubWorkflowMetricsAggregation)
			// Metrics time-series trends - POST not migrated
			configsGroup.POST("/:id/metrics/trends", GetGithubWorkflowMetricsTrends)
			// Stats for a config
			configsGroup.GET("/:id/stats", getUnifiedHandler("/github-workflow-metrics/configs/:id/stats"))
			// Summary statistics for a config
			configsGroup.GET("/:id/summary", getUnifiedHandler("/github-workflow-metrics/configs/:id/summary"))
			// Available dimensions with values
			configsGroup.GET("/:id/dimensions", getUnifiedHandler("/github-workflow-metrics/configs/:id/dimensions"))
			// Single dimension values
			configsGroup.GET("/:id/dimensions/:dimension/values", getUnifiedHandler("/github-workflow-metrics/configs/:id/dimensions/:dimension/values"))
			// Available fields (dimension fields and metric fields)
			configsGroup.GET("/:id/fields", getUnifiedHandler("/github-workflow-metrics/configs/:id/fields"))
			// Export metrics as CSV - binary, not migrated
			configsGroup.GET("/:id/export", ExportGithubWorkflowMetrics)
			// Backfill APIs
			configsGroup.POST("/:id/backfill", TriggerBackfill)
			configsGroup.GET("/:id/backfill/status", getUnifiedHandler("/github-workflow-metrics/configs/:id/backfill/status"))
			configsGroup.POST("/:id/backfill/cancel", CancelBackfill)
			configsGroup.GET("/:id/backfill/tasks", getUnifiedHandler("/github-workflow-metrics/configs/:id/backfill/tasks"))
			// Batch retry failed runs - POST not migrated
			configsGroup.POST("/:id/runs/batch-retry", RetryFailedRuns)
			// List completed EphemeralRunners for a config
			configsGroup.GET("/:id/runners", getUnifiedHandler("/github-workflow-metrics/configs/:id/runners"))
			// Dashboard APIs
			configsGroup.GET("/:id/dashboard", getUnifiedHandler("/github-workflow-metrics/configs/:id/dashboard"))
			configsGroup.GET("/:id/dashboard/builds", getUnifiedHandler("/github-workflow-metrics/configs/:id/dashboard/builds"))
			configsGroup.POST("/:id/dashboard/refresh", RefreshDashboardSummary)
			// Commit analysis API
			configsGroup.GET("/:id/commits/stats", getUnifiedHandler("/github-workflow-metrics/configs/:id/commits/stats"))
			// Analytics and history (previously in V2 group)
			configsGroup.GET("/:id/analytics", getUnifiedHandler("/github-workflow-metrics/configs/:id/analytics"))
			configsGroup.GET("/:id/history", getUnifiedHandler("/github-workflow-metrics/configs/:id/history"))
		}
		// Run management (global)
		runsGroup := githubWorkflowMetricsGroup.Group("/runs")
		{
			// List all runs globally (must be defined before /:id)
			runsGroup.GET("", getUnifiedHandler("/github-workflow-metrics/runs"))
			runsGroup.GET("/:id", getUnifiedHandler("/github-workflow-metrics/runs/:id"))
			runsGroup.GET("/:id/metrics", getUnifiedHandler("/github-workflow-metrics/runs/:id/metrics"))
			// Run detail with commits and performance comparison
			runsGroup.GET("/:id/detail", getUnifiedHandler("/github-workflow-metrics/runs/:id/detail"))
			// Retry single run - POST not migrated
			runsGroup.POST("/:id/retry", RetryGithubWorkflowRun)
			// Commit and details from V2 group
			runsGroup.GET("/:id/commit", getUnifiedHandler("/github-workflow-metrics/runs/:id/commit"))
			runsGroup.GET("/:id/details", getUnifiedHandler("/github-workflow-metrics/runs/:id/details"))
			// Real-time workflow state sync APIs
			runsGroup.GET("/:id/state", GetRunLiveState)
			runsGroup.GET("/:id/live", HandleLiveWorkflowState)
			runsGroup.GET("/:id/jobs", GetRunJobs)
			runsGroup.POST("/:id/sync/start", StartRunSync)
			runsGroup.POST("/:id/sync/stop", StopRunSync)
			// Job and step logs APIs
			runsGroup.GET("/:id/jobs/:job_id/logs", GetJobLogs)
			runsGroup.GET("/:id/jobs/:job_id/steps/:step_number/logs", GetStepLogs)
			// Analysis tasks by run ID
			runsGroup.GET("/:id/analysis-tasks", GetAnalysisTasksByRunID)
		}
		// Schema details
		schemasGroup := githubWorkflowMetricsGroup.Group("/schemas")
		{
			schemasGroup.GET("/:id", getUnifiedHandler("/github-workflow-metrics/schemas/:id"))
			schemasGroup.POST("/:id/activate", SetGithubWorkflowSchemaActive)
		}
		// Analysis tasks - AI workflow analysis tasks
		analysisTasksGroup := githubWorkflowMetricsGroup.Group("/analysis-tasks")
		{
			// List all analysis tasks with optional filters
			analysisTasksGroup.GET("", ListAnalysisTasks)
			// Get a specific analysis task
			analysisTasksGroup.GET("/:task_id", GetAnalysisTaskByID)
			// Update an analysis task (status and ext fields)
			analysisTasksGroup.PUT("/:task_id", UpdateAnalysisTask)
			// Retry a failed analysis task
			analysisTasksGroup.POST("/:task_id/retry", RetryAnalysisTask)
		}
	}

	// Phase 9 Unified (GET only): GitHub Runners management - AutoScalingRunnerSet discovery and analytics
	githubRunnersGroup := group.Group("/github-runners")
	{
		// Runner Sets - discovered AutoScalingRunnerSets
		runnerSetsGroup := githubRunnersGroup.Group("/runner-sets")
		{
			// List all runner sets (with optional stats)
			runnerSetsGroup.GET("", getUnifiedHandler("/github-runners/runner-sets"))
			// Get runner set by namespace/name
			runnerSetsGroup.GET("/:namespace/:name", getUnifiedHandler("/github-runners/runner-sets/:namespace/:name"))
			// Get runner set by ID
			runnerSetsGroup.GET("/by-id/:id", getUnifiedHandler("/github-runners/runner-sets/by-id/:id"))
			// Get runs for a runner set
			runnerSetsGroup.GET("/by-id/:id/runs", getUnifiedHandler("/github-runners/runner-sets/by-id/:id/runs"))
			// Get config for a runner set (may return null)
			runnerSetsGroup.GET("/by-id/:id/config", getUnifiedHandler("/github-runners/runner-sets/by-id/:id/config"))
			// Get statistics for a runner set
			runnerSetsGroup.GET("/by-id/:id/stats", getUnifiedHandler("/github-runners/runner-sets/by-id/:id/stats"))
			// Create config for a runner set - POST not migrated
			runnerSetsGroup.POST("/by-id/:id/config", CreateConfigForRunnerSet)
		}

		// Repositories - aggregated view by repository
		repositoriesGroup := githubRunnersGroup.Group("/repositories")
		{
			// List all repositories with aggregated runner set statistics
			repositoriesGroup.GET("", getUnifiedHandler("/github-runners/repositories"))
			// Get repository details with aggregated statistics
			repositoriesGroup.GET("/:owner/:repo", getUnifiedHandler("/github-runners/repositories/:owner/:repo"))
			// List runner sets for a specific repository
			repositoriesGroup.GET("/:owner/:repo/runner-sets", getUnifiedHandler("/github-runners/repositories/:owner/:repo/runner-sets"))
			// Get metrics metadata for all configs in a repository
			repositoriesGroup.GET("/:owner/:repo/metrics/metadata", getUnifiedHandler("/github-runners/repositories/:owner/:repo/metrics/metadata"))
			// Query metrics trends across all configs in a repository
			repositoriesGroup.POST("/:owner/:repo/metrics/trends", getUnifiedHandler("/github-runners/repositories/:owner/:repo/metrics/trends"))
		}
	}

	// Note: V2 group endpoints (analytics, history, commit, details) are now merged 
	// into the main github-workflow-metrics group above using unified handlers

	// Skills Repository routes - Proxy to skills-repository service
	skillsGroup := group.Group("/skills")
	{
		// List all skills - supports pagination and filtering
		skillsGroup.GET("", getUnifiedHandler("/skills"))
		// Semantic search - must be defined before /:name
		skillsGroup.POST("/search", getUnifiedHandler("/skills/search"))
		// Import skills from GitHub
		skillsGroup.POST("/import/github", getUnifiedHandler("/skills/import/github"))
		// Get skill by name
		skillsGroup.GET("/:name", getUnifiedHandler("/skills/:name"))
		// Get skill content (SKILL.md)
		skillsGroup.GET("/:name/content", getUnifiedHandler("/skills/:name/content"))
		// Create a new skill
		skillsGroup.POST("", getUnifiedHandler("/skills"))
		// Update a skill
		skillsGroup.PUT("/:name", getUnifiedHandler("/skills/:name"))
		// Delete a skill
		skillsGroup.DELETE("/:name", getUnifiedHandler("/skills/:name"))
	}

	// Release Management routes (Control Plane only)
	releaseGroup := group.Group("/releases")
	{
		// Release Versions
		versionsGroup := releaseGroup.Group("/versions")
		{
			versionsGroup.GET("", release.ListReleaseVersions)
			versionsGroup.POST("", release.CreateReleaseVersion)
			versionsGroup.GET("/:id", release.GetReleaseVersion)
			versionsGroup.PUT("/:id", release.UpdateReleaseVersion)
			versionsGroup.DELETE("/:id", release.DeleteReleaseVersion)
			versionsGroup.PATCH("/:id/status", release.UpdateReleaseVersionStatus)
		}

		// Cluster Release Configs
		clustersGroup := releaseGroup.Group("/clusters")
		{
			clustersGroup.GET("", release.ListClusterReleaseConfigs)
			clustersGroup.GET("/default", release.GetDefaultCluster)
			clustersGroup.PUT("/default", release.SetDefaultCluster)
			clustersGroup.DELETE("/default", release.ClearDefaultCluster)
			clustersGroup.GET("/:cluster_name", release.GetClusterReleaseConfig)
			clustersGroup.PUT("/:cluster_name/version", release.UpdateClusterVersion)
			clustersGroup.PUT("/:cluster_name/values", release.UpdateClusterValuesOverride)
			clustersGroup.GET("/:cluster_name/history", release.ListReleaseHistory)
			clustersGroup.POST("/:cluster_name/deploy", release.TriggerDeploy)
			clustersGroup.POST("/:cluster_name/rollback", release.TriggerRollback)
		}

		// Release History
		historyGroup := releaseGroup.Group("/history")
		{
			historyGroup.GET("/:id", release.GetReleaseHistoryByID)
		}
	}

	// Cluster Management routes (Control Plane only)
	managementGroup := group.Group("/management")
	{
		clusterMgmtGroup := managementGroup.Group("/clusters")
		{
			clusterMgmtGroup.GET("", cluster.ListClusters)
			clusterMgmtGroup.POST("", cluster.CreateCluster)
			clusterMgmtGroup.GET("/:cluster_name", cluster.GetCluster)
			clusterMgmtGroup.PUT("/:cluster_name", cluster.UpdateCluster)
			clusterMgmtGroup.DELETE("/:cluster_name", cluster.DeleteCluster)
			clusterMgmtGroup.PUT("/:cluster_name/default", cluster.SetDefaultCluster)
			clusterMgmtGroup.POST("/:cluster_name/test", cluster.TestClusterConnection)

			// Infrastructure initialization
			clusterMgmtGroup.POST("/:cluster_name/initialize", cluster.InitializeInfrastructure)
			clusterMgmtGroup.GET("/:cluster_name/infrastructure/status", cluster.GetInfrastructureStatus)

			// Task management
			clusterMgmtGroup.GET("/:cluster_name/tasks", cluster.ListTasks)
			clusterMgmtGroup.GET("/:cluster_name/tasks/active", cluster.GetActiveTask)
			clusterMgmtGroup.GET("/:cluster_name/tasks/:task_id", cluster.GetTask)
			clusterMgmtGroup.GET("/:cluster_name/tasks/:task_id/logs", cluster.GetTaskLogs)
		}

		// Control plane configuration routes
		configGroup := managementGroup.Group("/config")
		{
			configGroup.GET("", cpconfig.ListConfigs)
			configGroup.GET("/installer", cpconfig.GetInstallerConfig)
			configGroup.GET("/:key", cpconfig.GetConfig)
			configGroup.PUT("/:key", cpconfig.SetConfig)
			configGroup.DELETE("/:key", cpconfig.DeleteConfig)
		}
	}

	return nil
}
