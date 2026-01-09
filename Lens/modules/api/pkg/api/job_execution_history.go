// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// ListJobExecutionHistories handles GET /api/job-execution-histories
// Query parameters:
//   - cluster: Cluster name (optional, defaults to default cluster or current cluster)
//   - job_name: Filter by job name (supports fuzzy matching)
//   - job_type: Filter by job type (supports fuzzy matching)
//   - status: Filter by status (running/success/failed/cancelled/timeout)
//   - cluster_name: Filter by cluster name
//   - hostname: Filter by hostname
//   - start_time_from: Start time range (RFC3339 format)
//   - start_time_to: End time range (RFC3339 format)
//   - min_duration: Minimum execution duration (seconds)
//   - max_duration: Maximum execution duration (seconds)
//   - page_num: Page number (default: 1)
//   - page_size: Page size (default: 20, max: 100)
//   - order_by: Sort field (default: started_at DESC)
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
	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
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
// Get job execution history details by ID
// Query parameters:
//   - cluster: Cluster name (optional, defaults to default cluster or current cluster)
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

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
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
// Get recent failure records
// Query parameters:
//   - cluster: Cluster name (optional, defaults to default cluster or current cluster)
//   - limit: Number of records to return (default: 10, max: 100)
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

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
	histories, err := facade.GetRecentFailures(c.Request.Context(), limit)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get recent failures: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), histories))
}

// GetJobStatistics handles GET /api/job-execution-histories/statistics/:job_name
// Get statistics for a specific job
// Query parameters:
//   - cluster: Cluster name (optional, defaults to default cluster or current cluster)
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

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
	stats, err := facade.GetJobStatistics(c.Request.Context(), jobName)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get job statistics: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), stats))
}

// GetDistinctJobTypes handles GET /api/job-execution-histories/distinct/job-types
// Get all distinct job types
// Query parameters:
//   - cluster: Cluster name (optional, defaults to default cluster or current cluster)
func GetDistinctJobTypes(c *gin.Context) {
	// Get cluster clients based on query parameter
	cm := clientsets.GetClusterManager()
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	log.Infof("GetDistinctJobTypes: clusterName: %s", clients.ClusterName)

	facade := database.GetFacadeForCluster(clients.ClusterName).GetJobExecutionHistory()
	jobTypes, err := facade.GetDistinctJobTypes(c.Request.Context())
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get distinct job types: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"job_types": jobTypes,
		"count":     len(jobTypes),
	}))
}
