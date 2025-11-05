package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ListJobExecutionHistories handles GET /api/job-execution-histories
// 查询参数:
//   - cluster: 指定集群名称 (可选，默认使用配置的默认集群或当前集群)
//   - job_name: 按任务名称过滤 (支持模糊匹配)
//   - job_type: 按任务类型过滤 (支持模糊匹配)
//   - status: 按状态过滤 (running/success/failed/cancelled/timeout)
//   - cluster_name: 按集群名称过滤
//   - hostname: 按主机名过滤
//   - start_time_from: 开始时间范围 (RFC3339格式)
//   - start_time_to: 结束时间范围 (RFC3339格式)
//   - min_duration: 最小执行时长(秒)
//   - max_duration: 最大执行时长(秒)
//   - page_num: 页码 (默认: 1)
//   - page_size: 每页大小 (默认: 20, 最大: 100)
//   - order_by: 排序字段 (默认: started_at DESC)
func ListJobExecutionHistories(c *gin.Context) {
	// Get cluster clients based on query parameter
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	log.Infof("ListJobExecutionHistories: clusterName: %s", clients.ClusterName)

	// Parse pagination parameters
	pageNum := 1
	if page := c.Query("page_num"); page != "" {
		if p, err := strconv.Atoi(page); err == nil && p > 0 {
			pageNum = p
		}
	}

	pageSize := 20
	if size := c.Query("page_size"); size != "" {
		if s, err := strconv.Atoi(size); err == nil && s > 0 {
			pageSize = s
			if pageSize > 100 {
				pageSize = 100 // Max page size limit
			}
		}
	}

	// Build filter
	filter := &database.JobExecutionHistoryFilter{
		Offset:  (pageNum - 1) * pageSize,
		Limit:   pageSize,
		OrderBy: c.DefaultQuery("order_by", "started_at DESC"),
	}

	// Apply filters
	if jobName := c.Query("job_name"); jobName != "" {
		filter.JobName = &jobName
	}
	if jobType := c.Query("job_type"); jobType != "" {
		filter.JobType = &jobType
	}
	if status := c.Query("status"); status != "" {
		filter.Status = &status
	}
	if clusterNameFilter := c.Query("cluster_name"); clusterNameFilter != "" {
		filter.ClusterName = &clusterNameFilter
	}
	if hostname := c.Query("hostname"); hostname != "" {
		filter.Hostname = &hostname
	}

	// Parse time range
	if startTimeFrom := c.Query("start_time_from"); startTimeFrom != "" {
		if t, err := time.Parse(time.RFC3339, startTimeFrom); err == nil {
			filter.StartTimeFrom = &t
		} else {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid start_time_from format, use RFC3339", nil))
			return
		}
	}
	if startTimeTo := c.Query("start_time_to"); startTimeTo != "" {
		if t, err := time.Parse(time.RFC3339, startTimeTo); err == nil {
			filter.StartTimeTo = &t
		} else {
			c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid start_time_to format, use RFC3339", nil))
			return
		}
	}

	// Parse duration range
	if minDuration := c.Query("min_duration"); minDuration != "" {
		if d, err := strconv.ParseFloat(minDuration, 64); err == nil {
			filter.MinDuration = &d
		}
	}
	if maxDuration := c.Query("max_duration"); maxDuration != "" {
		if d, err := strconv.ParseFloat(maxDuration, 64); err == nil {
			filter.MaxDuration = &d
		}
	}

	// Query database with cluster context
	facade := database.GetFacade().GetJobExecutionHistory().WithCluster(clients.ClusterName)
	histories, total, err := facade.ListJobExecutionHistories(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to list job execution histories: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"data":     histories,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}))
}

// GetJobExecutionHistory handles GET /api/job-execution-histories/:id
// 根据ID获取任务执行历史详情
// 查询参数:
//   - cluster: 指定集群名称 (可选，默认使用配置的默认集群或当前集群)
func GetJobExecutionHistory(c *gin.Context) {
	// Get cluster clients based on query parameter
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	log.Infof("GetJobExecutionHistory: clusterName: %s", clients.ClusterName)

	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "invalid history ID", nil))
		return
	}

	facade := database.GetFacade().GetJobExecutionHistory().WithCluster(clients.ClusterName)
	history, err := facade.GetJobExecutionHistoryByID(c.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get job execution history: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if history == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "history record not found", nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), history))
}

// GetRecentFailures handles GET /api/job-execution-histories/recent-failures
// 获取最近的失败记录
// 查询参数:
//   - cluster: 指定集群名称 (可选，默认使用配置的默认集群或当前集群)
//   - limit: 返回记录数 (默认: 10, 最大: 100)
func GetRecentFailures(c *gin.Context) {
	// Get cluster clients based on query parameter
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	log.Infof("GetRecentFailures: clusterName: %s", clients.ClusterName)

	limit := 10
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
			if limit > 100 {
				limit = 100
			}
		}
	}

	facade := database.GetFacade().GetJobExecutionHistory().WithCluster(clients.ClusterName)
	histories, err := facade.GetRecentFailures(c.Request.Context(), limit)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get recent failures: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), histories))
}

// GetJobStatistics handles GET /api/job-execution-histories/statistics/:job_name
// 获取指定任务的统计信息
// 查询参数:
//   - cluster: 指定集群名称 (可选，默认使用配置的默认集群或当前集群)
func GetJobStatistics(c *gin.Context) {
	// Get cluster clients based on query parameter
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	log.Infof("GetJobStatistics: clusterName: %s", clients.ClusterName)

	jobName := c.Param("job_name")
	if jobName == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "job_name is required", nil))
		return
	}

	facade := database.GetFacade().GetJobExecutionHistory().WithCluster(clients.ClusterName)
	stats, err := facade.GetJobStatistics(c.Request.Context(), jobName)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get job statistics: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), stats))
}
