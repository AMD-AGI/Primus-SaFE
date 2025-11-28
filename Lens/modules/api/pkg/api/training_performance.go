package api

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
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
		_ = ctx.Error(errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workload_uid is required"))
		return
	}

	// 获取所有训练性能数据
	performances, err := database.GetFacade().GetTraining().
		ListTrainingPerformanceByWorkloadUID(ctx, workloadUID)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithMessage(err.Error()))
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
		_ = ctx.Error(errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workload_uid is required"))
		return
	}

	// 获取数据源参数
	dataSource := ctx.Query("data_source")

	// 获取训练性能数据
	var performances []*model.TrainingPerformance
	var err error

	if dataSource != "" {
		// 按数据源过滤
		performances, err = database.GetFacade().GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(ctx, workloadUID, dataSource)
	} else {
		// 获取所有数据源
		performances, err = database.GetFacade().GetTraining().
			ListTrainingPerformanceByWorkloadUID(ctx, workloadUID)
	}

	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithMessage(err.Error()))
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
//   - metrics: 指标名称列表，逗号分隔 (可选，不指定则返回所有指标)
//   - start: 开始时间戳（毫秒）(可选)
//   - end: 结束时间戳（毫秒）(可选)
func GetMetricsData(ctx *gin.Context) {
	workloadUID := ctx.Param("uid")
	if workloadUID == "" {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.RequestParameterInvalid).
			WithMessage("workload_uid is required"))
		return
	}

	// 解析查询参数
	dataSource := ctx.Query("data_source")
	metricsStr := ctx.Query("metrics")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")

	// 解析指标列表
	var requestedMetrics []string
	if metricsStr != "" {
		requestedMetrics = strings.Split(metricsStr, ",")
		// 去除空格
		for i := range requestedMetrics {
			requestedMetrics[i] = strings.TrimSpace(requestedMetrics[i])
		}
	}

	// 解析时间范围
	var startTime, endTime time.Time
	var hasTimeRange bool

	if startStr != "" && endStr != "" {
		startMs, err := strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			_ = ctx.Error(errors.NewError().
				WithCode(errors.RequestParameterInvalid).
				WithMessage("invalid start time format"))
			return
		}

		endMs, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			_ = ctx.Error(errors.NewError().
				WithCode(errors.RequestParameterInvalid).
				WithMessage("invalid end time format"))
			return
		}

		startTime = time.UnixMilli(startMs)
		endTime = time.UnixMilli(endMs)
		hasTimeRange = true
	}

	// 查询数据
	var performances []*model.TrainingPerformance
	var err error

	if hasTimeRange {
		performances, err = database.GetFacade().GetTraining().
			ListTrainingPerformanceByWorkloadUIDDataSourceAndTimeRange(
				ctx, workloadUID, dataSource, startTime, endTime,
			)
	} else {
		performances, err = database.GetFacade().GetTraining().
			ListTrainingPerformanceByWorkloadUIDAndDataSource(
				ctx, workloadUID, dataSource,
			)
	}

	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithMessage(err.Error()))
		return
	}

	// 构建数据点列表
	dataPoints := make([]MetricDataPoint, 0)
	metricsSet := make(map[string]bool)
	if len(requestedMetrics) > 0 {
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

			// 如果指定了指标列表，只返回请求的指标
			if len(metricsSet) > 0 && !metricsSet[metricName] {
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
