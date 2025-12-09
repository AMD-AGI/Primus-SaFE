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

// tensorflowMetadataFields defines metadata fields in tensorflow data source that are not actual metrics
var tensorflowMetadataFields = map[string]bool{
	"step":      true,
	"wall_time": true,
	"file":      true,
	"scalars":   true, // 原始 scalars 结构（用于调试）
	"texts":     true, // 原始 texts 结构（用于调试）
}

// commonMetadataFields defines common metadata fields across all data sources
// These are not actual metrics and should be filtered in GetAvailableMetrics and GetMetricsData
var commonMetadataFields = map[string]bool{
	"iteration":        true, // Returned by GetIterationTimes
	"target_iteration": true, // Returned by GetIterationTimes
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

// IterationInfo represents iteration information for deduplication
type IterationInfo struct {
	Timestamp       int64
	TargetIteration *float64
	DataSource      string
}

// isMetricField determines if a field is an actual metric based on data source type
func isMetricField(fieldName string, dataSource string) bool {
	switch dataSource {
	case "wandb":
		// wandb data source needs to filter out metadata fields
		return !wandbMetadataFields[fieldName]
	case "tensorflow":
		// tensorflow data source needs to filter out metadata fields and "vs samples" metrics
		if tensorflowMetadataFields[fieldName] {
			return false
		}
		// 暂时不支持 "vs samples" 和 "vs steps" 视图
		if strings.Contains(fieldName, " vs samples") || strings.Contains(fieldName, " vs steps") {
			return false
		}
		return true
	case "log":
		// All fields in log data source are metrics
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
			// Filter out common metadata fields (iteration-related)
			if commonMetadataFields[metricName] {
				continue
			}

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
			// Filter out common metadata fields (iteration-related)
			if commonMetadataFields[metricName] {
				continue
			}

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

	// 对 tensorflow 数据源进行去重处理
	// 移除时间相近但 step 明显不同的重复数据点（多X轴问题）
	if dataSource == "tensorflow" || (dataSource == "" && len(dataPoints) > 0 && dataPoints[0].DataSource == "tensorflow") {
		dataPoints = deduplicateTensorflowDataPoints(dataPoints)
	}

	response := MetricsDataResponse{
		WorkloadUID: workloadUID,
		DataSource:  dataSource,
		Data:        dataPoints,
		TotalCount:  len(dataPoints),
	}

	ctx.JSON(200, response)
}

// deduplicateTensorflowDataPoints removes duplicate data points from TensorFlow data source
// that have similar timestamps but significantly different iteration values (multi x-axis issue)
func deduplicateTensorflowDataPoints(dataPoints []MetricDataPoint) []MetricDataPoint {
	if len(dataPoints) == 0 {
		return dataPoints
	}

	// 按 metric_name 分组
	metricGroups := make(map[string][]MetricDataPoint)
	for _, dp := range dataPoints {
		metricGroups[dp.MetricName] = append(metricGroups[dp.MetricName], dp)
	}

	result := make([]MetricDataPoint, 0, len(dataPoints))

	// 对每个 metric 进行去重
	for metricName, points := range metricGroups {
		if len(points) == 0 {
			continue
		}

		// 按时间戳排序
		sortedPoints := make([]MetricDataPoint, len(points))
		copy(sortedPoints, points)
		// 简单冒泡排序（数据量通常不大）
		for i := 0; i < len(sortedPoints); i++ {
			for j := i + 1; j < len(sortedPoints); j++ {
				if sortedPoints[i].Timestamp > sortedPoints[j].Timestamp {
					sortedPoints[i], sortedPoints[j] = sortedPoints[j], sortedPoints[i]
				}
			}
		}

		// 去重：对于时间相近的数据点，只保留 iteration 较小的
		kept := make([]bool, len(sortedPoints))
		for i := 0; i < len(sortedPoints); i++ {
			kept[i] = true
		}

		const timeWindowMs = 10000           // 10秒时间窗口
		const iterationRatioThreshold = 10.0 // iteration 相差10倍以上认为是重复

		for i := 0; i < len(sortedPoints); i++ {
			if !kept[i] {
				continue
			}

			// 检查后续的数据点
			for j := i + 1; j < len(sortedPoints); j++ {
				if !kept[j] {
					continue
				}

				// 时间差异超过窗口，后续的点肯定也超过
				timeDiff := sortedPoints[j].Timestamp - sortedPoints[i].Timestamp
				if timeDiff > timeWindowMs {
					break
				}

				// 时间相近，检查 iteration
				iter1 := float64(sortedPoints[i].Iteration)
				iter2 := float64(sortedPoints[j].Iteration)

				if iter1 == 0 || iter2 == 0 {
					continue
				}

				ratio := iter2 / iter1
				if ratio < 1 {
					ratio = 1 / ratio
				}

				// 如果 iteration 相差很大（可能是 samples vs iteration），保留较小的
				if ratio >= iterationRatioThreshold {
					if sortedPoints[i].Iteration < sortedPoints[j].Iteration {
						kept[j] = false
					} else {
						kept[i] = false
						break // i 已被标记为不保留，跳出内层循环
					}
				}
			}
		}

		// 收集保留的数据点
		keptCount := 0
		for i, point := range sortedPoints {
			if kept[i] {
				result = append(result, point)
				keptCount++
			}
		}

		// 记录去重信息（用于调试）
		if keptCount < len(sortedPoints) {
			_ = metricName // 避免未使用变量警告
		}
	}

	return result
}

// hasTensorflowData checks if the iteration map contains tensorflow data
func hasTensorflowData(iterationMap map[int32]*IterationInfo) bool {
	for _, info := range iterationMap {
		if info.DataSource == "tensorflow" {
			return true
		}
	}
	return false
}

// filterAnomalousIterations removes anomalous iteration values from tensorflow data
// Anomalous iterations are typically samples (several times or tens of times larger than normal iterations)
func filterAnomalousIterations(iterationMap map[int32]*IterationInfo) map[int32]*IterationInfo {
	if len(iterationMap) == 0 {
		return iterationMap
	}

	// 收集所有 iteration 值并排序
	iterations := make([]int32, 0, len(iterationMap))
	for iter := range iterationMap {
		iterations = append(iterations, iter)
	}

	// 简单冒泡排序
	for i := 0; i < len(iterations); i++ {
		for j := i + 1; j < len(iterations); j++ {
			if iterations[i] > iterations[j] {
				iterations[i], iterations[j] = iterations[j], iterations[i]
			}
		}
	}

	if len(iterations) < 3 {
		// 数据点太少，不进行过滤
		return iterationMap
	}

	// 策略：计算相邻 iteration 之间的比率，识别突变点
	// 如果某个 iteration 相对于前面的值增长超过10倍，认为是异常值
	const anomalyRatioThreshold = 10.0

	// 找到第一个异常的 iteration（通常是从 iteration 突然变成 samples）
	anomalyStartIndex := -1
	for i := 1; i < len(iterations); i++ {
		if iterations[i-1] == 0 {
			continue
		}

		ratio := float64(iterations[i]) / float64(iterations[i-1])
		if ratio >= anomalyRatioThreshold {
			anomalyStartIndex = i
			break
		}
	}

	// 如果没有发现异常，返回原始数据
	if anomalyStartIndex == -1 {
		return iterationMap
	}

	// 过滤掉异常的 iteration
	filtered := make(map[int32]*IterationInfo)
	for i := 0; i < anomalyStartIndex; i++ {
		iter := iterations[i]
		filtered[iter] = iterationMap[iter]
	}

	// 如果过滤后数据太少，可能判断错误，返回原始数据
	if len(filtered) < len(iterationMap)/2 {
		// 过滤掉了超过一半的数据，可能判断有误
		// 尝试反向策略：保留较大的值
		// 但这种情况比较少见，为了安全起见，返回原始数据
		return iterationMap
	}

	return filtered
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

	// 对于 tensorflow 数据源，先过滤异常的 iteration 值
	// 这些异常值通常是 samples（正常 iteration 的几倍到几十倍）
	if dataSource == "tensorflow" || (dataSource == "" && hasTensorflowData(iterationMap)) {
		iterationMap = filterAnomalousIterations(iterationMap)
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

	// 对 tensorflow 数据源进行去重处理
	if dataSource == "tensorflow" || (dataSource == "" && len(dataPoints) > 0 && dataPoints[0].DataSource == "tensorflow") {
		dataPoints = deduplicateTensorflowDataPoints(dataPoints)
	}

	response := MetricsDataResponse{
		WorkloadUID: workloadUID,
		DataSource:  dataSource,
		Data:        dataPoints,
		TotalCount:  len(dataPoints),
	}

	ctx.JSON(200, response)
}
