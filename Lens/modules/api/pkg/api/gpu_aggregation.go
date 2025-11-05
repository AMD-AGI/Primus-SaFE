package api

import (
	"net/http"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ClusterHourlyStatsRequest 集群小时统计查询请求
type ClusterHourlyStatsRequest struct {
	StartTime string `form:"start_time" binding:"required"` // RFC3339 格式
	EndTime   string `form:"end_time" binding:"required"`   // RFC3339 格式
}

// NamespaceHourlyStatsRequest namespace小时统计查询请求
type NamespaceHourlyStatsRequest struct {
	Namespace string `form:"namespace"`                     // 可选,为空则查询所有namespace
	StartTime string `form:"start_time" binding:"required"` // RFC3339 格式
	EndTime   string `form:"end_time" binding:"required"`   // RFC3339 格式
}

// LabelHourlyStatsRequest label/annotation小时统计查询请求
type LabelHourlyStatsRequest struct {
	DimensionType  string `form:"dimension_type" binding:"required,oneof=label annotation"` // label 或 annotation
	DimensionKey   string `form:"dimension_key" binding:"required"`                         // label key
	DimensionValue string `form:"dimension_value"`                                          // 可选,为空则查询该key的所有value
	StartTime      string `form:"start_time" binding:"required"`                            // RFC3339 格式
	EndTime        string `form:"end_time" binding:"required"`                              // RFC3339 格式
}

// SnapshotsRequest 快照查询请求
type SnapshotsRequest struct {
	StartTime string `form:"start_time"` // RFC3339 格式,可选
	EndTime   string `form:"end_time"`   // RFC3339 格式,可选
}

// getClusterHourlyStats 查询集群级别小时统计
// @Summary 查询集群GPU小时统计
// @Tags GPU聚合
// @Accept json
// @Produce json
// @Param cluster query string false "集群名称"
// @Param start_time query string true "开始时间 (RFC3339格式)"
// @Param end_time query string true "结束时间 (RFC3339格式)"
// @Success 200 {object} rest.Response{data=[]dbmodel.ClusterGpuHourlyStats}
// @Router /gpu-aggregation/cluster/hourly-stats [get]
func getClusterHourlyStats(ctx *gin.Context) {
	var req ClusterHourlyStatsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// 解析时间
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// 获取集群客户端
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// 查询数据
	stats, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetClusterHourlyStats(ctx, startTime, endTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get cluster hourly stats", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, stats))
}

// getNamespaceHourlyStats 查询namespace级别小时统计
// @Summary 查询Namespace GPU小时统计
// @Tags GPU聚合
// @Accept json
// @Produce json
// @Param cluster query string false "集群名称"
// @Param namespace query string false "命名空间名称(可选,为空则查询所有)"
// @Param start_time query string true "开始时间 (RFC3339格式)"
// @Param end_time query string true "结束时间 (RFC3339格式)"
// @Success 200 {object} rest.Response{data=[]dbmodel.NamespaceGpuHourlyStats}
// @Router /gpu-aggregation/namespaces/hourly-stats [get]
func getNamespaceHourlyStats(ctx *gin.Context) {
	var req NamespaceHourlyStatsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// 解析时间
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// 获取集群客户端
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// 查询数据
	var stats []*dbmodel.NamespaceGpuHourlyStats
	facade := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation()

	if req.Namespace != "" {
		// 查询特定namespace
		stats, err = facade.GetNamespaceHourlyStats(ctx, req.Namespace, startTime, endTime)
	} else {
		// 查询所有namespace
		stats, err = facade.ListNamespaceHourlyStats(ctx, startTime, endTime)
	}

	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get namespace hourly stats", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, stats))
}

// getLabelHourlyStats 查询label/annotation级别小时统计
// @Summary 查询Label/Annotation GPU小时统计
// @Tags GPU聚合
// @Accept json
// @Produce json
// @Param cluster query string false "集群名称"
// @Param dimension_type query string true "维度类型 (label或annotation)"
// @Param dimension_key query string true "维度key"
// @Param dimension_value query string false "维度value(可选,为空则查询该key的所有value)"
// @Param start_time query string true "开始时间 (RFC3339格式)"
// @Param end_time query string true "结束时间 (RFC3339格式)"
// @Success 200 {object} rest.Response{data=[]dbmodel.LabelGpuHourlyStats}
// @Router /gpu-aggregation/labels/hourly-stats [get]
func getLabelHourlyStats(ctx *gin.Context) {
	var req LabelHourlyStatsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// 解析时间
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
		return
	}

	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
		return
	}

	// 获取集群客户端
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// 查询数据
	var stats []*dbmodel.LabelGpuHourlyStats
	facade := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation()

	if req.DimensionValue != "" {
		// 查询特定dimension value
		stats, err = facade.GetLabelHourlyStats(ctx, req.DimensionType,
			req.DimensionKey, req.DimensionValue, startTime, endTime)
	} else {
		// 查询该key的所有value
		stats, err = facade.ListLabelHourlyStatsByKey(ctx, req.DimensionType,
			req.DimensionKey, startTime, endTime)
	}

	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get label hourly stats", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, stats))
}

// getLatestSnapshot 获取最新的GPU分配快照
// @Summary 获取最新的GPU分配快照
// @Tags GPU聚合
// @Accept json
// @Produce json
// @Param cluster query string false "集群名称"
// @Success 200 {object} rest.Response{data=dbmodel.GpuAllocationSnapshots}
// @Router /gpu-aggregation/snapshots/latest [get]
func getLatestSnapshot(ctx *gin.Context) {
	// 获取集群客户端
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// 查询最新快照
	snapshot, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		GetLatestSnapshot(ctx)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to get latest snapshot", errors.CodeDatabaseError))
		return
	}

	if snapshot == nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted).WithMessage("No snapshot found"))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, snapshot))
}

// listSnapshots 查询历史快照列表
// @Summary 查询GPU分配快照历史
// @Tags GPU聚合
// @Accept json
// @Produce json
// @Param cluster query string false "集群名称"
// @Param start_time query string false "开始时间 (RFC3339格式,可选)"
// @Param end_time query string false "结束时间 (RFC3339格式,可选)"
// @Success 200 {object} rest.Response{data=[]dbmodel.GpuAllocationSnapshots}
// @Router /gpu-aggregation/snapshots [get]
func listSnapshots(ctx *gin.Context) {
	var req SnapshotsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		_ = ctx.Error(errors.WrapError(err, "Invalid request parameters", errors.RequestParameterInvalid))
		return
	}

	// 获取集群客户端
	cm := clientsets.GetClusterManager()
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// 默认查询最近24小时
	endTime := time.Now()
	startTime := endTime.Add(-24 * time.Hour)

	// 如果提供了时间参数,使用提供的时间
	if req.StartTime != "" {
		startTime, err = time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			_ = ctx.Error(errors.WrapError(err, "Invalid start_time format", errors.RequestParameterInvalid))
			return
		}
	}

	if req.EndTime != "" {
		endTime, err = time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			_ = ctx.Error(errors.WrapError(err, "Invalid end_time format", errors.RequestParameterInvalid))
			return
		}
	}

	// 查询快照列表
	snapshots, err := database.GetFacadeForCluster(clients.ClusterName).GetGpuAggregation().
		ListSnapshots(ctx, startTime, endTime)
	if err != nil {
		_ = ctx.Error(errors.WrapError(err, "Failed to list snapshots", errors.CodeDatabaseError))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, snapshots))
}
