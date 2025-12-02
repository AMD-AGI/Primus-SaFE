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

// wandb数据源中的元数据字段，这些不是实际的指标
var wandbMetadataFields = map[string]bool{
	"step":       true,
	"run_id":     true,
	"source":     true,
	"history":    true,
	"created_at": true,
	"updated_at": true,
}

// MetricInfo 指标信息
type MetricInfo struct {
	Name       string   `json:"name"`        // 指标名称
	DataSource []string `json:"data_source"` // 数据来源列表
	Count      int      `json:"count"`       // 该指标的数据点数量
}

// AvailableMetricsResponse 可用指标响应
type AvailableMetricsResponse struct {
	WorkloadUID string       `json:"workload_uid"`
	Metrics     []MetricInfo `json:"metrics"`
	TotalCount  int          `json:"total_count"` // 总指标数量
}

// MetricDataPoint 指标数据点
type MetricDataPoint struct {
	MetricName string  `json:"metric_name"` // 指标名称
	Value      float64 `json:"value"`       // 指标值
	Timestamp  int64   `json:"timestamp"`   // 时间戳（毫秒）
	Iteration  int32   `json:"iteration"`   // 训练步数/迭代次数
	DataSource string  `json:"data_source"` // 数据来源
}

// MetricsDataResponse 指标数据响应
type MetricsDataResponse struct {
	WorkloadUID string            `json:"workload_uid"`
	DataSource  string            `json:"data_source,omitempty"`
	Data        []MetricDataPoint `json:"data"`
	TotalCount  int               `json:"total_count"`
}

// DataSourceInfo 数据源信息
type DataSourceInfo struct {
	Name  string `json:"name"`  // 数据源名称
	Count int    `json:"count"` // 该数据源的数据点数量
}

// DataSourcesResponse 数据源列表响应
type DataSourcesResponse struct {
	WorkloadUID string           `json:"workload_uid"`
	DataSources []DataSourceInfo `json:"data_sources"`
	TotalCount  int              `json:"total_count"`
}

// isMetricField 判断字段是否为实际指标（根据数据源类型）
func isMetricField(fieldName string, dataSource string) bool {
	switch dataSource {
	case "wandb":
		// wandb数据源需要过滤元数据字段
		return !wandbMetadataFields[fieldName]
	case "log", "tensorflow":
		// log和tensorflow数据源的所有字段都是指标
		return true
	default:
		// 默认都视为指标
		return true
	}
}

// GetDataSources 获取指定 workload 的所有数据源
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

	// 获取所有训练性能数据
	performances, err := database.GetFacadeForCluster(clients.ClusterName).GetTraining().
		ListTrainingPerformanceByWorkloadUID(ctx, workloadUID)
	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// 统计数据源
	sourceMap := make(map[string]int) // data_source -> count
	for _, p := range performances {
		sourceMap[p.DataSource]++
	}

	// 构建响应
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

// GetAvailableMetrics 获取指定 workload 的所有可用指标
// GET /workloads/:uid/metrics/available
// Query Parameters:
//   - data_source: 数据来源 (可选，如 "log", "wandb", "tensorflow")
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

	// 获取数据源参数
	dataSource := ctx.Query("data_source")

	// 获取训练性能数据
	var performances []*model.TrainingPerformance

	if dataSource != "" {
		// 按数据源过滤
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, workloadUID, dataSource)
	} else {
		// 获取所有数据源
		performances, err = database.GetFacadeForCluster(clients.ClusterName).GetTraining().
			ListTrainingPerformanceByWorkloadUID(ctx, workloadUID)
	}

	if err != nil {
		ctx.AbortWithStatusJSON(500, gin.H{"error": err.Error()})
		return
	}

	// 统计所有可用指标
	metricMap := make(map[string]map[string]int) // metric_name -> {data_source -> count}
	for _, p := range performances {
		for metricName := range p.Performance {
			// 根据数据源类型过滤字段
			if !isMetricField(metricName, p.DataSource) {
				continue
			}

			if _, exists := metricMap[metricName]; !exists {
				metricMap[metricName] = make(map[string]int)
			}
			metricMap[metricName][p.DataSource]++
		}
	}

	// 构建响应
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

// GetMetricsData 获取指定指标的数据
// GET /workloads/:uid/metrics/data
// Query Parameters:
//   - data_source: 数据来源 (可选，如 "log", "wandb", "tensorflow")
//   - metrics: 指标名称列表，逗号分隔 (可选，支持 "all" 返回所有指标，或指定具体指标名，不指定则返回所有)
//   - start: 开始时间戳（毫秒）(可选)
//   - end: 结束时间戳（毫秒）(可选)
func GetMetricsData(ctx *gin.Context) {
	workloadUID := ctx.Param("uid")
	if workloadUID == "" {
		ctx.AbortWithStatusJSON(400, gin.H{"error": "workload_uid is required"})
		return
	}

	// 解析查询参数
	dataSource := ctx.Query("data_source")
	metricsStr := ctx.Query("metrics")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")

	// 解析指标列表
	var requestedMetrics []string
	var returnAllMetrics bool = true // 默认返回所有指标

	if metricsStr != "" {
		// 去除首尾空格
		metricsStr = strings.TrimSpace(metricsStr)

		// 如果明确指定 "all"，返回所有指标
		if strings.ToLower(metricsStr) == "all" {
			returnAllMetrics = true
		} else {
			// 支持 Grafana 格式：{metric1,metric2} 或普通格式：metric1,metric2
			// 去除花括号（如果存在）
			if strings.HasPrefix(metricsStr, "{") && strings.HasSuffix(metricsStr, "}") {
				metricsStr = metricsStr[1 : len(metricsStr)-1]
			}

			// 指定了具体的指标名称
			if metricsStr != "" {
				requestedMetrics = strings.Split(metricsStr, ",")
				// 去除空格
				for i := range requestedMetrics {
					requestedMetrics[i] = strings.TrimSpace(requestedMetrics[i])
				}
				returnAllMetrics = false
			}
		}
	}

	// 解析时间范围
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

	// 查询数据
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

	// 构建数据点列表
	dataPoints := make([]MetricDataPoint, 0)
	metricsSet := make(map[string]bool)

	// 如果不是返回所有指标，构建指标集合用于过滤
	if !returnAllMetrics && len(requestedMetrics) > 0 {
		for _, m := range requestedMetrics {
			metricsSet[m] = true
		}
	}

	for _, p := range performances {
		for metricName, value := range p.Performance {
			// 根据数据源类型过滤元数据字段
			if !isMetricField(metricName, p.DataSource) {
				continue
			}

			// 如果不是返回所有指标且指定了指标列表，只返回请求的指标
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

// GetIterationTimes 获取每个 iteration 的时间信息
// GET /workloads/:uid/metrics/iteration-times
// Query Parameters:
//   - data_source: 数据来源 (可选，如 "log", "wandb", "tensorflow")
//   - start: 开始时间戳（毫秒）(可选)
//   - end: 结束时间戳（毫秒）(可选)
//
// 返回格式与 GetMetricsData 相同，包含两个指标：
//   - metric_name: "iteration" - 当前迭代次数
//   - metric_name: "target_iteration" - 目标迭代次数
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

	// 解析查询参数
	dataSource := ctx.Query("data_source")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")

	// 解析时间范围
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

	// 查询数据
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

	// 构建指标数据点列表
	// 使用 map 去重，因为同一个 iteration 可能有多个指标记录
	type IterationInfo struct {
		Timestamp       int64
		TargetIteration *float64
		DataSource      string
	}
	iterationMap := make(map[int32]*IterationInfo)

	for _, p := range performances {
		timestamp := p.CreatedAt.UnixMilli()

		// 提取 TargetIteration（如果存在）
		var targetIteration *float64
		if targetIterValue, exists := p.Performance["TargetIteration"]; exists {
			targetIterFloat := convertToFloat(targetIterValue)
			if !math.IsNaN(targetIterFloat) {
				targetIteration = &targetIterFloat
			}
		}

		// 如果这个 iteration 还没记录，或者当前记录的时间更早，则更新
		if existing, exists := iterationMap[p.Iteration]; !exists || timestamp < existing.Timestamp {
			iterationMap[p.Iteration] = &IterationInfo{
				Timestamp:       timestamp,
				TargetIteration: targetIteration,
				DataSource:      p.DataSource,
			}
		}
	}

	// 转换为 MetricDataPoint 数组
	dataPoints := make([]MetricDataPoint, 0, len(iterationMap)*2)

	for iteration, info := range iterationMap {
		// 添加 iteration 数据点
		dataPoints = append(dataPoints, MetricDataPoint{
			MetricName: "iteration",
			Value:      float64(iteration),
			Timestamp:  info.Timestamp,
			Iteration:  iteration,
			DataSource: info.DataSource,
		})

		// 如果有 target_iteration，添加对应的数据点
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
