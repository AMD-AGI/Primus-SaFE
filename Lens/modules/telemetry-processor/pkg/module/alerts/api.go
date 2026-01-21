// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ListAlerts handles GET /v1/alerts - list alerts with filters
func ListAlerts(ctx *gin.Context) {
	var req AlertQueryRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse query parameters: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Set default limit if not specified
	if req.Limit == 0 {
		req.Limit = 50
	}

	// Build filter
	filter := &database.AlertEventsFilter{
		Offset: req.Offset,
		Limit:  req.Limit,
	}

	if req.Source != "" {
		filter.Source = &req.Source
	}
	if req.AlertName != "" {
		filter.AlertName = &req.AlertName
	}
	if req.Severity != "" {
		filter.Severity = &req.Severity
	}
	if req.Status != "" {
		filter.Status = &req.Status
	}
	if req.WorkloadID != "" {
		filter.WorkloadID = &req.WorkloadID
	}
	if req.PodName != "" {
		filter.PodName = &req.PodName
	}
	if req.NodeName != "" {
		filter.NodeName = &req.NodeName
	}
	if req.ClusterName != "" {
		filter.ClusterName = &req.ClusterName
	}
	if req.StartsAfter != nil {
		filter.StartsAfter = req.StartsAfter
	}
	if req.StartsBefore != nil {
		filter.StartsBefore = req.StartsBefore
	}

	// Query database
	facade := database.GetFacade().GetAlert()
	alerts, total, err := facade.ListAlertEventss(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list alerts: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Convert to unified format
	result := make([]*UnifiedAlert, 0, len(alerts))
	for _, alert := range alerts {
		result = append(result, convertAlertEventToUnified(alert))
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"alerts": result,
		"total":  total,
		"offset": req.Offset,
		"limit":  req.Limit,
	}))
}

// GetAlert handles GET /v1/alerts/:id - get a single alert
func GetAlert(ctx *gin.Context) {
	alertID := ctx.Param("id")
	if alertID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "alert ID is required", nil))
		return
	}

	facade := database.GetFacade().GetAlert()
	alert, err := facade.GetAlertEventsByID(ctx.Request.Context(), alertID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if alert == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "alert not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), convertAlertEventToUnified(alert)))
}

// GetAlertCorrelationsAPI handles GET /v1/alerts/:id/correlations - get alert correlations
func GetAlertCorrelationsAPI(ctx *gin.Context) {
	alertID := ctx.Param("id")
	if alertID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "alert ID is required", nil))
		return
	}

	correlations, err := GetAlertCorrelations(ctx.Request.Context(), alertID)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert correlations: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"correlations": correlations,
	}))
}

// GetAlertStatistics handles GET /v1/alerts/statistics - get alert statistics
func GetAlertStatistics(ctx *gin.Context) {
	var req AlertStatisticsRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse query parameters: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Set default limit if not specified
	if req.Limit == 0 {
		req.Limit = 100
	}

	// Build filter
	filter := &database.AlertStatisticsFilter{
		DateFrom: req.DateFrom,
		DateTo:   req.DateTo,
		Offset:   req.Offset,
		Limit:    req.Limit,
	}

	if req.AlertName != "" {
		filter.AlertName = &req.AlertName
	}
	if req.Source != "" {
		filter.Source = &req.Source
	}
	if req.WorkloadID != "" {
		filter.WorkloadID = &req.WorkloadID
	}
	if req.ClusterName != "" {
		filter.ClusterName = &req.ClusterName
	}

	// Query database
	facade := database.GetFacade().GetAlert()
	stats, err := facade.ListAlertStatisticss(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert statistics: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"statistics": stats,
		"total":      len(stats),
	}))
}

// CreateAlertRule handles POST /v1/alert-rules - create a new alert rule
func CreateAlertRule(ctx *gin.Context) {
	var req AlertRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate request
	if req.Name == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "rule name is required", nil))
		return
	}
	if req.Source == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "rule source is required", nil))
		return
	}
	if req.RuleType == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "rule type is required", nil))
		return
	}

	// Create rule model
	ruleConfigExt := dbmodel.ExtType(req.RuleConfig)

	labelsExt := dbmodel.ExtType{}
	if req.Labels != nil {
		for k, v := range req.Labels {
			labelsExt[k] = v
		}
	}

	annotationsExt := dbmodel.ExtType{}
	if req.Annotations != nil {
		for k, v := range req.Annotations {
			annotationsExt[k] = v
		}
	}

	routeConfigExt := dbmodel.ExtType{}
	if req.RouteConfig != nil {
		routeConfigBytes, err := json.Marshal(req.RouteConfig)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
			return
		}
		if err := json.Unmarshal(routeConfigBytes, &routeConfigExt); err != nil {
			ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
			return
		}
	}

	rule := &dbmodel.AlertRules{
		Name:        req.Name,
		Source:      req.Source,
		Enabled:     req.Enabled,
		RuleType:    req.RuleType,
		RuleConfig:  ruleConfigExt,
		Severity:    req.Severity,
		Labels:      labelsExt,
		Annotations: annotationsExt,
		RouteConfig: routeConfigExt,
	}

	// Save to database
	facade := database.GetFacade().GetAlert()
	if err := facade.CreateAlertRules(ctx.Request.Context(), rule); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rule_id": rule.ID,
	}))
}

// ListAlertRules handles GET /v1/alert-rules - list alert rules
func ListAlertRules(ctx *gin.Context) {
	source := ctx.Query("source")
	enabledStr := ctx.Query("enabled")

	var enabled *bool
	if enabledStr != "" {
		e, err := strconv.ParseBool(enabledStr)
		if err == nil {
			enabled = &e
		}
	}

	facade := database.GetFacade().GetAlert()
	rules, err := facade.ListAlertRuless(ctx.Request.Context(), source, enabled)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list alert rules: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rules": rules,
		"total": len(rules),
	}))
}

// GetAlertRule handles GET /v1/alert-rules/:id - get a single alert rule
func GetAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	facade := database.GetFacade().GetAlert()
	rule, err := facade.GetAlertRulesByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if rule == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), rule))
}

// UpdateAlertRule handles PUT /v1/alert-rules/:id - update an alert rule
func UpdateAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	var req AlertRuleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Get existing rule
	facade := database.GetFacade().GetAlert()
	rule, err := facade.GetAlertRulesByID(ctx.Request.Context(), id)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	if rule == nil {
		ctx.JSON(http.StatusNotFound, rest.ErrorResp(ctx.Request.Context(), http.StatusNotFound, "rule not found", nil))
		return
	}

	// Update fields
	if req.Name != "" {
		rule.Name = req.Name
	}
	if req.Source != "" {
		rule.Source = req.Source
	}
	rule.Enabled = req.Enabled
	if req.RuleType != "" {
		rule.RuleType = req.RuleType
	}
	if req.RuleConfig != nil {
		rule.RuleConfig = dbmodel.ExtType(req.RuleConfig)
	}
	if req.Severity != "" {
		rule.Severity = req.Severity
	}
	if req.Labels != nil {
		rule.Labels = dbmodel.ExtType{}
		for k, v := range req.Labels {
			rule.Labels[k] = v
		}
	}
	if req.Annotations != nil {
		rule.Annotations = dbmodel.ExtType{}
		for k, v := range req.Annotations {
			rule.Annotations[k] = v
		}
	}
	if req.RouteConfig != nil {
		routeConfigBytes, err := json.Marshal(req.RouteConfig)
		if err == nil {
			rule.RouteConfig = dbmodel.ExtType{}
			json.Unmarshal(routeConfigBytes, &rule.RouteConfig)
		}
	}

	// Save to database
	if err := facade.UpdateAlertRules(ctx.Request.Context(), rule); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to update alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"rule_id": rule.ID,
	}))
}

// DeleteAlertRule handles DELETE /v1/alert-rules/:id - delete an alert rule
func DeleteAlertRule(ctx *gin.Context) {
	idStr := ctx.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "invalid rule ID", nil))
		return
	}

	facade := database.GetFacade().GetAlert()
	if err := facade.DeleteAlertRules(ctx.Request.Context(), id); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete alert rule: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "rule deleted successfully",
	}))
}

// CreateSilence handles POST /v1/silences - create a silence (legacy API, converts to label-based silence)
func CreateSilence(ctx *gin.Context) {
	var req SilenceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse request body: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	// Validate request
	if len(req.Matchers) == 0 {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "matchers are required", nil))
		return
	}
	if req.StartsAt.IsZero() || req.EndsAt.IsZero() {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "starts_at and ends_at are required", nil))
		return
	}
	if req.EndsAt.Before(req.StartsAt) {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "ends_at must be after starts_at", nil))
		return
	}

	// Convert legacy matchers to label matchers
	labelMatchers := make([]model.LabelMatcher, 0, len(req.Matchers))
	for _, m := range req.Matchers {
		labelMatchers = append(labelMatchers, model.LabelMatcher{
			Name:     m.Name,
			Operator: "=",
			Value:    m.Value,
		})
	}

	labelMatchersExt := dbmodel.ExtType{}
	labelMatchersBytes, err := json.Marshal(labelMatchers)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}
	if err := json.Unmarshal(labelMatchersBytes, &labelMatchersExt); err != nil {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}

	silence := &dbmodel.AlertSilences{
		ID:            uuid.New().String(),
		Name:          "legacy-silence-" + uuid.New().String()[:8],
		Description:   req.Comment,
		Enabled:       true,
		SilenceType:   "label",
		LabelMatchers: labelMatchersExt,
		StartsAt:      req.StartsAt,
		EndsAt:        req.EndsAt,
		Reason:        req.Comment,
		CreatedBy:     req.CreatedBy,
	}

	// Save to database
	facade := database.GetFacade().GetAlert()
	if err := facade.CreateAlertSilences(ctx.Request.Context(), silence); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to create silence: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusCreated, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"silence_id": silence.ID,
	}))
}

// ListSilences handles GET /v1/silences - list active silences
func ListSilences(ctx *gin.Context) {
	clusterName := ctx.Query("cluster")

	facade := database.GetFacade().GetAlert()
	silences, err := facade.ListActiveSilences(ctx.Request.Context(), time.Now(), clusterName)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to list silences: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"silences": silences,
		"total":    len(silences),
	}))
}

// DeleteSilence handles DELETE /v1/silences/:id - delete a silence
func DeleteSilence(ctx *gin.Context) {
	silenceID := ctx.Param("id")
	if silenceID == "" {
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, "silence ID is required", nil))
		return
	}

	facade := database.GetFacade().GetAlert()
	if err := facade.DeleteAlertSilences(ctx.Request.Context(), silenceID); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to delete silence: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"message": "silence deleted successfully",
	}))
}

// GetAlertSummary handles GET /v1/alerts/summary - get alert summary by severity with changes
func GetAlertSummary(ctx *gin.Context) {
	clusterName := ctx.Query("cluster")

	facade := database.GetFacade().GetAlert()

	// Get current counts by severity
	now := time.Now()
	oneHourAgo := now.Add(-time.Hour)

	// Build filter for current alerts
	currentFilter := &database.AlertEventsFilter{
		Status: strPtr(StatusFiring),
		Limit:  10000, // High limit to get all
	}
	if clusterName != "" {
		currentFilter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(ctx.Request.Context(), currentFilter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get current alerts: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
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
		Status:       strPtr(StatusFiring),
		StartsBefore: &oneHourAgo,
		Limit:        10000,
	}
	if clusterName != "" {
		historicalFilter.ClusterName = &clusterName
	}

	historicalAlerts, _, err := facade.ListAlertEventss(ctx.Request.Context(), historicalFilter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Warningf("Failed to get historical alerts: %v", err)
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

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
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

// GetAlertTrend handles GET /v1/alerts/trend - get alert trend data
func GetAlertTrend(ctx *gin.Context) {
	clusterName := ctx.Query("cluster")
	groupBy := ctx.DefaultQuery("group_by", "hour")
	hoursStr := ctx.DefaultQuery("hours", "24")

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 24
	}

	facade := database.GetFacade().GetAlert()

	now := time.Now()
	startTime := now.Add(-time.Duration(hours) * time.Hour)

	// Build filter
	filter := &database.AlertEventsFilter{
		StartsAfter: &startTime,
		Limit:       10000,
	}
	if clusterName != "" {
		filter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alerts for trend: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
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
	buckets := make(map[int64]*TrendPoint)
	current := startTime.Truncate(interval)
	for current.Before(now) {
		buckets[current.Unix()] = &TrendPoint{
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
	result := make([]*TrendPoint, 0, len(buckets))
	current = startTime.Truncate(interval)
	for current.Before(now) {
		if bucket, ok := buckets[current.Unix()]; ok {
			result = append(result, bucket)
		}
		current = current.Add(interval)
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), result))
}

// GetTopAlertSources handles GET /v1/alerts/top-sources - get top alert sources
func GetTopAlertSources(ctx *gin.Context) {
	clusterName := ctx.Query("cluster")
	limitStr := ctx.DefaultQuery("limit", "10")
	hoursStr := ctx.DefaultQuery("hours", "24")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 24
	}

	facade := database.GetFacade().GetAlert()

	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Build filter
	filter := &database.AlertEventsFilter{
		StartsAfter: &startTime,
		Limit:       10000,
	}
	if clusterName != "" {
		filter.ClusterName = &clusterName
	}

	alerts, _, err := facade.ListAlertEventss(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alerts for top sources: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
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

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), sources))
}

// GetAlertsByCluster handles GET /v1/alerts/by-cluster - get alert counts by cluster
func GetAlertsByCluster(ctx *gin.Context) {
	hoursStr := ctx.DefaultQuery("hours", "24")

	hours, err := strconv.Atoi(hoursStr)
	if err != nil || hours <= 0 {
		hours = 24
	}

	facade := database.GetFacade().GetAlert()

	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	// Build filter - get all clusters
	filter := &database.AlertEventsFilter{
		StartsAfter: &startTime,
		Status:      strPtr(StatusFiring),
		Limit:       10000,
	}

	alerts, _, err := facade.ListAlertEventss(ctx.Request.Context(), filter)
	if err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to get alerts by cluster: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}

	// Count by cluster
	counts := make(map[string]int)
	for _, alert := range alerts {
		cluster := alert.ClusterName
		if cluster == "" {
			cluster = "unknown"
		}
		counts[cluster]++
	}

	// Convert to response format
	type clusterCount struct {
		ClusterName string `json:"cluster_name"`
		Count       int    `json:"count"`
	}

	result := make([]clusterCount, 0, len(counts))
	for cluster, count := range counts {
		result = append(result, clusterCount{
			ClusterName: cluster,
			Count:       count,
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

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), result))
}

// TrendPoint represents a single point in the trend data
type TrendPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Critical  int       `json:"critical"`
	High      int       `json:"high"`
	Warning   int       `json:"warning"`
	Info      int       `json:"info"`
}

// Helper function to create string pointer
func strPtr(s string) *string {
	return &s
}
