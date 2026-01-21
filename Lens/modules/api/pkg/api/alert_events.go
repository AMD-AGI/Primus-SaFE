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

// Alert status constants
const (
	AlertStatusFiring   = "firing"
	AlertStatusResolved = "resolved"
	AlertStatusSilenced = "silenced"
)

// Alert severity constants
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

// AlertTrendPoint represents a single point in the trend data
type AlertTrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Critical  int       `json:"critical"`
	High      int       `json:"high"`
	Warning   int       `json:"warning"`
	Info      int       `json:"info"`
}

// ListAlertEvents handles GET /api/alerts - list alerts with filters
func ListAlertEvents(c *gin.Context) {
	clusterName := c.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	source := c.Query("source")
	alertName := c.Query("alert_name")
	severity := c.Query("severity")
	status := c.Query("status")
	workloadID := c.Query("workload_id")
	podName := c.Query("pod_name")
	nodeName := c.Query("node_name")

	pageNum, _ := strconv.Atoi(c.DefaultQuery("pageNum", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageNum < 1 {
		pageNum = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	filter := &database.AlertEventsFilter{
		Offset: (pageNum - 1) * pageSize,
		Limit:  pageSize,
	}

	if clusterName != "" && clusterName != "all" {
		filter.ClusterName = &clusterName
	}
	if source != "" {
		filter.Source = &source
	}
	if alertName != "" {
		filter.AlertName = &alertName
	}
	if severity != "" {
		filter.Severity = &severity
	}
	if status != "" {
		filter.Status = &status
	}
	if workloadID != "" {
		filter.WorkloadID = &workloadID
	}
	if podName != "" {
		filter.PodName = &podName
	}
	if nodeName != "" {
		filter.NodeName = &nodeName
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	alerts, total, err := facade.ListAlertEventss(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to list alert events: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"data":     alerts,
		"total":    total,
		"pageNum":  pageNum,
		"pageSize": pageSize,
	}))
}

// GetAlertEvent handles GET /api/alerts/:id - get a single alert
func GetAlertEvent(c *gin.Context) {
	alertID := c.Param("id")
	if alertID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "alert ID is required", nil))
		return
	}

	clusterName := c.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()
	alert, err := facade.GetAlertEventsByID(c.Request.Context(), alertID)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get alert event: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if alert == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "alert not found", nil))
		return
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), alert))
}

// GetAlertSummary handles GET /api/alerts/summary - get alert summary by severity with changes
func GetAlertSummary(c *gin.Context) {
	clusterName := c.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()

	// Get current counts by severity
	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	// Build filter for current firing alerts
	currentFilter := &database.AlertEventsFilter{
		Status: strPtr(AlertStatusFiring),
		Limit:  10000,
	}
	if clusterName != "" && clusterName != "all" {
		currentFilter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(c.Request.Context(), currentFilter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get current alerts: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Count by severity
	currentCounts := map[string]int{
		SeverityCritical: 0,
		SeverityHigh:     0,
		SeverityWarning:  0,
		SeverityInfo:     0,
	}
	for _, alert := range alerts {
		currentCounts[alert.Severity]++
	}

	// Get counts from 1 hour ago for comparison
	historicalFilter := &database.AlertEventsFilter{
		Status:       strPtr(AlertStatusFiring),
		StartsBefore: &oneHourAgo,
		Limit:        10000,
	}
	if clusterName != "" && clusterName != "all" {
		historicalFilter.ClusterName = &clusterName
	}

	historicalAlerts, _, err := facade.ListAlertEventss(c.Request.Context(), historicalFilter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Warningf("Failed to get historical alerts: %v", err)
		// Continue with zero changes
	}

	historicalCounts := map[string]int{
		SeverityCritical: 0,
		SeverityHigh:     0,
		SeverityWarning:  0,
		SeverityInfo:     0,
	}
	for _, alert := range historicalAlerts {
		historicalCounts[alert.Severity]++
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"critical": gin.H{
			"count":  currentCounts[SeverityCritical],
			"change": currentCounts[SeverityCritical] - historicalCounts[SeverityCritical],
		},
		"high": gin.H{
			"count":  currentCounts[SeverityHigh],
			"change": currentCounts[SeverityHigh] - historicalCounts[SeverityHigh],
		},
		"warning": gin.H{
			"count":  currentCounts[SeverityWarning],
			"change": currentCounts[SeverityWarning] - historicalCounts[SeverityWarning],
		},
		"info": gin.H{
			"count":  currentCounts[SeverityInfo],
			"change": currentCounts[SeverityInfo] - historicalCounts[SeverityInfo],
		},
	}))
}

// GetAlertTrend handles GET /api/alerts/trend - get alert trend data
func GetAlertTrend(c *gin.Context) {
	clusterName := c.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	groupBy := c.DefaultQuery("group_by", "hour")
	hoursStr := c.DefaultQuery("hours", "24")

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 24
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()

	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)

	// Build filter
	filter := &database.AlertEventsFilter{
		StartsAfter: &startTime,
		Limit:       10000,
	}
	if clusterName != "" && clusterName != "all" {
		filter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get alerts for trend: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Group by time interval
	var interval time.Duration
	if groupBy == "day" {
		interval = 24 * time.Hour
	} else {
		interval = time.Hour
	}

	// Create time buckets
	buckets := make(map[int64]*AlertTrendPoint)
	current := startTime.Truncate(interval)
	for current.Before(now) {
		buckets[current.Unix()] = &AlertTrendPoint{
			Timestamp: current,
			Critical:  0,
			High:      0,
			Warning:   0,
			Info:      0,
		}
		current = current.Add(interval)
	}

	// Fill buckets with alert counts
	for _, alert := range alerts {
		bucketTime := alert.StartsAt.Truncate(interval).Unix()
		if bucket, ok := buckets[bucketTime]; ok {
			switch alert.Severity {
			case SeverityCritical:
				bucket.Critical++
			case SeverityHigh:
				bucket.High++
			case SeverityWarning:
				bucket.Warning++
			case SeverityInfo:
				bucket.Info++
			}
		}
	}

	// Convert to sorted slice
	result := make([]*AlertTrendPoint, 0, len(buckets))
	current = startTime.Truncate(interval)
	for current.Before(now) {
		if bucket, ok := buckets[current.Unix()]; ok {
			result = append(result, bucket)
		}
		current = current.Add(interval)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), result))
}

// GetTopAlertSources handles GET /api/alerts/top-sources - get top alert sources
func GetTopAlertSources(c *gin.Context) {
	clusterName := c.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	limitStr := c.DefaultQuery("limit", "10")
	hoursStr := c.DefaultQuery("hours", "24")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 24
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()

	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Build filter
	filter := &database.AlertEventsFilter{
		StartsAfter: &startTime,
		Limit:       10000,
	}
	if clusterName != "" && clusterName != "all" {
		filter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get alerts for top sources: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Count by alert name
	counts := make(map[string]int)
	for _, alert := range alerts {
		counts[alert.AlertName]++
	}

	// Sort and get top N
	type sourceCount struct {
		AlertName string `json:"alert_name"`
		Count     int    `json:"count"`
	}

	sources := make([]sourceCount, 0, len(counts))
	for name, count := range counts {
		sources = append(sources, sourceCount{
			AlertName: name,
			Count:     count,
		})
	}

	// Sort by count descending
	for i := 0; i < len(sources); i++ {
		for j := i + 1; j < len(sources); j++ {
			if sources[j].Count > sources[i].Count {
				sources[i], sources[j] = sources[j], sources[i]
			}
		}
	}

	// Limit results
	if len(sources) > limit {
		sources = sources[:limit]
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), sources))
}

// GetAlertsByCluster handles GET /api/alerts/by-cluster - get alert counts by cluster
func GetAlertsByCluster(c *gin.Context) {
	hoursStr := c.DefaultQuery("hours", "24")

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 24
	}

	// Get all cluster names
	clusterManager := clientsets.GetClusterManager()
	clusterNames := clusterManager.GetClusterNames()

	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	type clusterCount struct {
		ClusterName string `json:"cluster_name"`
		Count       int    `json:"count"`
	}

	result := make([]clusterCount, 0, len(clusterNames))

	// Query each cluster
	for _, clusterName := range clusterNames {
		facade := database.GetFacadeForCluster(clusterName).GetAlert()

		filter := &database.AlertEventsFilter{
			StartsAfter: &startTime,
			Status:      strPtr(AlertStatusFiring),
			ClusterName: &clusterName,
			Limit:       10000,
		}

		alerts, _, err := facade.ListAlertEventss(c.Request.Context(), filter)
		if err != nil {
			log.GlobalLogger().WithContext(c).Warningf("Failed to get alerts for cluster %s: %v", clusterName, err)
			continue
		}

		result = append(result, clusterCount{
			ClusterName: clusterName,
			Count:       len(alerts),
		})
	}

	// Sort by count descending
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), result))
}

// GetAlertCorrelations handles GET /api/alerts/:id/correlations - get alert correlations
func GetAlertCorrelations(c *gin.Context) {
	alertID := c.Param("id")
	if alertID == "" {
		c.JSON(http.StatusBadRequest, rest.ErrorResp(c.Request.Context(), http.StatusBadRequest, "alert ID is required", nil))
		return
	}

	clusterName := c.Query("cluster")
	if clusterName == "" {
		clusterName = clientsets.GetClusterManager().GetCurrentClusterName()
	}

	facade := database.GetFacadeForCluster(clusterName).GetAlert()

	// Get the alert first
	alert, err := facade.GetAlertEventsByID(c.Request.Context(), alertID)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get alert: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if alert == nil {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "alert not found", nil))
		return
	}

	// Find related alerts (same workload, pod, or node within time window)
	timeWindow := 30 * time.Minute
	startTime := alert.StartsAt.Add(-timeWindow)
	endTime := alert.StartsAt.Add(timeWindow)

	filter := &database.AlertEventsFilter{
		StartsAfter:  &startTime,
		StartsBefore: &endTime,
		Limit:        100,
	}

	// Filter by same resource
	if alert.WorkloadID != "" {
		filter.WorkloadID = &alert.WorkloadID
	} else if alert.PodName != "" {
		filter.PodName = &alert.PodName
	} else if alert.NodeName != "" {
		filter.NodeName = &alert.NodeName
	}

	relatedAlerts, _, err := facade.ListAlertEventss(c.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(c).Errorf("Failed to get related alerts: %v", err)
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Remove current alert from results
	var correlations []*struct {
		AlertID         string  `json:"alert_id"`
		AlertName       string  `json:"alert_name"`
		Severity        string  `json:"severity"`
		CorrelationType string  `json:"correlation_type"`
		Score           float64 `json:"score"`
	}

	for _, related := range relatedAlerts {
		if related.ID == alertID {
			continue
		}

		correlationType := "time"
		score := 0.5

		if related.WorkloadID == alert.WorkloadID && alert.WorkloadID != "" {
			correlationType = "workload"
			score = 0.9
		} else if related.PodName == alert.PodName && alert.PodName != "" {
			correlationType = "pod"
			score = 0.85
		} else if related.NodeName == alert.NodeName && alert.NodeName != "" {
			correlationType = "node"
			score = 0.7
		}

		correlations = append(correlations, &struct {
			AlertID         string  `json:"alert_id"`
			AlertName       string  `json:"alert_name"`
			Severity        string  `json:"severity"`
			CorrelationType string  `json:"correlation_type"`
			Score           float64 `json:"score"`
		}{
			AlertID:         related.ID,
			AlertName:       related.AlertName,
			Severity:        related.Severity,
			CorrelationType: correlationType,
			Score:           score,
		})
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c.Request.Context(), gin.H{
		"correlations": correlations,
	}))
}

// Helper function to create string pointer
func strPtr(s string) *string {
	return &s
}
