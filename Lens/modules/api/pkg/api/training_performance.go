package api

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/gin-gonic/gin"
)

// wandbMetadataFields defines metadata fields in wandb data source that are not actual metrics
var wandbMetadataFields = map[string]bool{
	"step":       true,
	"run_id":     true,
	"source":     true,
	"history":    true,
	"created_at": true,
	"updated_at": true,
}

// MetricInfo represents metric information
type MetricInfo struct {
	Name       string   `json:"name"`        // Metric name
	DataSource []string `json:"data_source"` // List of data sources
	Count      int      `json:"count"`       // Number of data points for this metric
}

// AvailableMetricsResponse represents available metrics response
type AvailableMetricsResponse struct {
	WorkloadUID string       `json:"workload_uid"`
	Metrics     []MetricInfo `json:"metrics"`
	TotalCount  int          `json:"total_count"` // Total number of metrics
}

// MetricDataPoint represents a single metric data point
type MetricDataPoint struct {
	MetricName string  `json:"metric_name"` // Metric name
	Value      float64 `json:"value"`       // Metric value
	Timestamp  int64   `json:"timestamp"`   // Timestamp in milliseconds
	Iteration  int32   `json:"iteration"`   // Training step/iteration number
	DataSource string  `json:"data_source"` // Data source
}

// MetricsDataResponse represents metrics data response
type MetricsDataResponse struct {
	WorkloadUID string            `json:"workload_uid"`
	DataSource  string            `json:"data_source,omitempty"`
	Data        []MetricDataPoint `json:"data"`
	TotalCount  int               `json:"total_count"`
}

// DataSourceInfo represents data source information
type DataSourceInfo struct {
	Name  string `json:"name"`  // Data source name
	Count int    `json:"count"` // Number of data points for this data source
}

// DataSourcesResponse represents data sources list response
type DataSourcesResponse struct {
	WorkloadUID string           `json:"workload_uid"`
	DataSources []DataSourceInfo `json:"data_sources"`
	TotalCount  int              `json:"total_count"`
}

// isMetricField determines if a field is an actual metric based on data source type
func isMetricField(fieldName string, dataSource string) bool {
	switch dataSource {
	case "wandb":
		// wandb data source needs to filter out metadata fields
		return !wandbMetadataFields[fieldName]
	case "log", "tensorflow":
		// All fields in log and tensorflow data sources are metrics
		return true
	default:
		// Default: treat all fields as metrics
		return true
	}
}

// GetDataSources retrieves all data sources for a specified workload
// GET /workloads/:uid/metrics/sources
func GetDataSources(ctx *gin.Context) {
	workloadUID := ctx.Param("uid")
	if workloadUID == "" {
		ctx.AbortWithStatusJSON(400, gin.H{"error": "workload_uid is required"})
		return
	}

	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Get all training performance data
	performances, err := database.GetFacadeForCluster(clients.ClusterName).GetTraining().
		ListTrainingPerformanceByWorkloadUID(ctx, workloadUID)
	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Count data sources
	sourceMap := make(map[string]int) // data_source -> count
	for _, p := range performances {
		sourceMap[p.DataSource]++
	}

	// Build response
	dataSources := make([]DataSourceInfo, 0, len(sourceMap))
	for source, count := range sourceMap {
		dataSources = append(dataSources, DataSourceInfo{
			Name:  source,
			Count: count,
		})
	}

	response := DataSourcesResponse{
		WorkloadUID: workloadUID,
		DataSources: dataSources,
		TotalCount:  len(dataSources),
	}

	ctx.JSON(200, response)
}

// GetAvailableMetrics retrieves all available metrics for a specified workload
// GET /workloads/:uid/metrics/available
// Query Parameters:
//   - data_source: Data source (optional, e.g., "log", "wandb", "tensorflow")
func GetAvailableMetrics(ctx *gin.Context) {
	workloadUID := ctx.Param("uid")
	if workloadUID == "" {
		ctx.AbortWithStatusJSON(400, gin.H{"error": "workload_uid is required"})
		return
	}

	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Get data source parameter
	dataSource := ctx.Query("data_source")

	// Get training performance data
	var performances []*model.TrainingPerformance

	if dataSource != "" {
		// Filter by data source
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, workloadUID, dataSource)
	} else {
		// Get all data sources
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUID(ctx, workloadUID)
	}

	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Count all available metrics
	metricMap := make(map[string]map[string]int) // metric_name -> {data_source -> count}
	for _, p := range performances {
		for metricName := range p.Performance {
			// Filter fields based on data source type
			if !isMetricField(metricName, p.DataSource) {
				continue
			}

			if _, exists := metricMap[metricName]; !exists {
				metricMap[metricName] = make(map[string]int)
			}
			metricMap[metricName][p.DataSource]++
		}
	}

	// Build response
	metrics := make([]MetricInfo, 0, len(metricMap))
	for metricName, sources := range metricMap {
		sourceList := make([]string, 0, len(sources))
		totalCount := 0
		for source, count := range sources {
			sourceList = append(sourceList, source)
			totalCount += count
		}
		metrics = append(metrics, MetricInfo{
			Name:       metricName,
			DataSource: sourceList,
			Count:      totalCount,
		})
	}

	response := AvailableMetricsResponse{
		WorkloadUID: workloadUID,
		Metrics:     metrics,
		TotalCount:  len(metrics),
	}

	ctx.JSON(200, response)
}

// GetMetricsData retrieves data for specified metrics
// GET /workloads/:uid/metrics/data
// Query Parameters:
//   - data_source: Data source (optional, e.g., "log", "wandb", "tensorflow")
//   - metrics: Comma-separated list of metric names (optional, supports "all" to return all metrics, or specific metric names; returns all if not specified)
//   - start: Start timestamp in milliseconds (optional)
//   - end: End timestamp in milliseconds (optional)
func GetMetricsData(ctx *gin.Context) {
	workloadUID := ctx.Param("uid")
	if workloadUID == "" {
		ctx.AbortWithStatusJSON(400, gin.H{"error": "workload_uid is required"})
		return
	}

	// Parse query parameters
	dataSource := ctx.Query("data_source")
	metricsStr := ctx.Query("metrics")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")

	// Parse metrics list
	var requestedMetrics []string
	var returnAllMetrics bool = true // Default: return all metrics

	if metricsStr != "" {
		// Trim leading and trailing spaces
		metricsStr = strings.TrimSpace(metricsStr)

		// If explicitly specified "all", return all metrics
		if strings.ToLower(metricsStr) == "all" {
			returnAllMetrics = true
		} else {
			// Support Grafana format: {metric1,metric2} or plain format: metric1,metric2
			// Remove curly braces if present
			if strings.HasPrefix(metricsStr, "{") && strings.HasSuffix(metricsStr, "}") {
				metricsStr = metricsStr[1 : len(metricsStr)-1]
			}

			// Specific metric names specified
			if metricsStr != "" {
				requestedMetrics = strings.Split(metricsStr, ",")
				// Trim spaces
				for i := range requestedMetrics {
					requestedMetrics[i] = strings.TrimSpace(requestedMetrics[i])
				}
				returnAllMetrics = false
			}
		}
	}

	// Parse time range
	var startTime, endTime time.Time
	var hasTimeRange bool

	if startStr != "" && endStr != "" {
		startMs, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			ctx.AbortWithStatusJSON(400, gin.H{"error": "invalid start time format"})
			return
		}

		endMs, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			ctx.AbortWithStatusJSON(400, gin.H{"error": "invalid end time format"})
			return
		}

		startTime = time.UnixMilli(startMs)
		endTime = time.UnixMilli(endMs)
		hasTimeRange = true
	}

	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Query data
	var performances []*model.TrainingPerformance

	if hasTimeRange {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(
				ctx, workloadUID, dataSource, startTime, endTime,
			)
	} else {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(
				ctx, workloadUID, dataSource,
			)
	}

	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Build data points list
	dataPoints := make([]MetricDataPoint, 0)
	metricsSet := make(map[string]bool)

	// If not returning all metrics, build metrics set for filtering
	if !returnAllMetrics && len(requestedMetrics) > 0 {
		for _, m := range requestedMetrics {
			metricsSet[m] = true
		}
	}

	for _, p := range performances {
		for metricName, value := range p.Performance {
			// Filter metadata fields based on data source type
			if !isMetricField(metricName, p.DataSource) {
				continue
			}

			// If not returning all metrics and metrics list is specified, only return requested metrics
			if !returnAllMetrics && len(metricsSet) > 0 && !metricsSet[metricName] {
				continue
			}

			valueFloat := convertToFloat(value)
			if math.IsNaN(valueFloat) {
				continue
			}

			dataPoints = append(dataPoints, MetricDataPoint{
				MetricName: metricName,
				Value:      valueFloat,
				Timestamp:  p.CreatedAt.UnixMilli(),
				Iteration:  p.Iteration,
				DataSource: p.DataSource,
			})
		}
	}

	response := MetricsDataResponse{
		WorkloadUID: workloadUID,
		DataSource:  dataSource,
		Data:        dataPoints,
		TotalCount:  len(dataPoints),
	}

	ctx.JSON(200, response)
}

// GetIterationTimes retrieves time information for each iteration
// GET /workloads/:uid/metrics/iteration-times
// Query Parameters:
//   - data_source: Data source (optional, e.g., "log", "wandb", "tensorflow")
//   - start: Start timestamp in milliseconds (optional)
//   - end: End timestamp in milliseconds (optional)
//
// Returns the same format as GetMetricsData, containing two metrics:
//   - metric_name: "iteration" - Current iteration number
//   - metric_name: "target_iteration" - Target iteration number
func GetIterationTimes(ctx *gin.Context) {
	workloadUID := ctx.Param("uid")
	if workloadUID == "" {
		ctx.AbortWithStatusJSON(400, gin.H{"error": "workload_uid is required"})
		return
	}

	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Parse query parameters
	dataSource := ctx.Query("data_source")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")

	// Parse time range
	var startTime, endTime time.Time
	var hasTimeRange bool

	if startStr != "" && endStr != "" {
		startMs, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			ctx.AbortWithStatusJSON(400, gin.H{"error": "invalid start time format"})
			return
		}

		endMs, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			ctx.AbortWithStatusJSON(400, gin.H{"error": "invalid end time format"})
			return
		}

		startTime = time.UnixMilli(startMs)
		endTime = time.UnixMilli(endMs)
		hasTimeRange = true
	}

	// Query data
	var performances []*model.TrainingPerformance

	if hasTimeRange {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(
				ctx, workloadUID, dataSource, startTime, endTime,
			)
	} else {
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(
				ctx, workloadUID, dataSource,
			)
	}

	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// Build metric data points list
	// Use map for deduplication since the same iteration may have multiple metric records
	type IterationInfo struct {
		Timestamp       int64
		TargetIteration *float64
		DataSource      string
	}
	iterationMap := make(map[int32]*IterationInfo)

	for _, p := range performances {
		timestamp := p.CreatedAt.UnixMilli()

		// Extract TargetIteration if it exists
		var targetIteration *float64
		if targetIterValue, exists := p.Performance["target_iteration"]; exists {
			targetIterFloat := convertToFloat(targetIterValue)
			if !math.IsNaN(targetIterFloat) {
				targetIteration = &targetIterFloat
			}
		}

		// If this iteration hasn't been recorded, or current record's timestamp is earlier, update it
		if existing, exists := iterationMap[p.Iteration]; !exists || timestamp < existing.Timestamp {
			iterationMap[p.Iteration] = &IterationInfo{
				Timestamp:       timestamp,
				TargetIteration: targetIteration,
				DataSource:      p.DataSource,
			}
		}
	}

	// Convert to MetricDataPoint array
	dataPoints := make([]MetricDataPoint, 0, len(iterationMap)*2)

	for iteration, info := range iterationMap {
		// Add iteration data point
		dataPoints = append(dataPoints, MetricDataPoint{
			MetricName: "iteration",
			Value:      float64(iteration),
			Timestamp:  info.Timestamp,
			Iteration:  iteration,
			DataSource: info.DataSource,
		})

		// If target_iteration exists, add corresponding data point
		if info.TargetIteration != nil {
			dataPoints = append(dataPoints, MetricDataPoint{
				MetricName: "target_iteration",
				Value:      *info.TargetIteration,
				Timestamp:  info.Timestamp,
				Iteration:  iteration,
				DataSource: info.DataSource,
			})
		}
	}

	response := MetricsDataResponse{
		WorkloadUID: workloadUID,
		DataSource:  dataSource,
		Data:        dataPoints,
		TotalCount:  len(dataPoints),
	}

	ctx.JSON(200, response)
}
