// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package alerts

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ReceiveMetricAlert handles metric alerts from VMAlert
func ReceiveMetricAlert(ctx *gin.Context) {
	var webhook VMAlertWebhook
	if err := ctx.ShouldBindJSON(&webhook); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse VMAlert webhook: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}
	
	log.GlobalLogger().WithContext(ctx).Infof("Received %d metric alerts from VMAlert", len(webhook.Alerts))
	
	// Convert VMAlert format to unified format
	alerts := convertVMAlertToUnified(&webhook)
	
	// Process each alert
	successCount := 0
	for _, alert := range alerts {
		if err := processAlert(ctx.Request.Context(), alert); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to process alert %s: %v", alert.ID, err)
		} else {
			successCount++
		}
	}
	
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"received": len(webhook.Alerts),
		"processed": successCount,
	}))
}

// ReceiveAlertManagerAlert handles alerts in AlertManager format (array of alerts)
// VMAlert sends alerts in this format when using the AlertManager-compatible endpoint
func ReceiveAlertManagerAlert(ctx *gin.Context) {
	var alertItems []VMAlertItem
	if err := ctx.ShouldBindJSON(&alertItems); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse AlertManager format alerts: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}
	
	log.GlobalLogger().WithContext(ctx).Infof("Received %d alerts in AlertManager format", len(alertItems))
	
	// Convert to unified format
	alerts := convertAlertManagerItemsToUnified(alertItems)
	
	// Process each alert
	successCount := 0
	for _, alert := range alerts {
		if err := processAlert(ctx.Request.Context(), alert); err != nil {
			log.GlobalLogger().WithContext(ctx).Errorf("Failed to process alert %s: %v", alert.ID, err)
		} else {
			successCount++
		}
	}
	
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"received": len(alertItems),
		"processed": successCount,
	}))
}

// ReceiveLogAlert handles log-based alerts
func ReceiveLogAlert(ctx *gin.Context) {
	var req LogAlertRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse log alert request: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}
	
	log.GlobalLogger().WithContext(ctx).Infof("Received log alert: %s", req.RuleName)
	
	// Convert log alert to unified format
	alert := convertLogAlertToUnified(&req)
	
	// Process alert
	if err := processAlert(ctx.Request.Context(), alert); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to process log alert: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"alert_id": alert.ID,
	}))
}

// ReceiveTraceAlert handles trace-based alerts
func ReceiveTraceAlert(ctx *gin.Context) {
	var req TraceAlertRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse trace alert request: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}
	
	log.GlobalLogger().WithContext(ctx).Infof("Received trace alert: %s", req.RuleName)
	
	// Convert trace alert to unified format
	alert := convertTraceAlertToUnified(&req)
	
	// Process alert
	if err := processAlert(ctx.Request.Context(), alert); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to process trace alert: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"alert_id": alert.ID,
	}))
}

// ReceiveGenericWebhook handles generic webhook alerts
func ReceiveGenericWebhook(ctx *gin.Context) {
	var data map[string]interface{}
	if err := ctx.ShouldBindJSON(&data); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to parse generic webhook: %v", err)
		ctx.JSON(http.StatusBadRequest, rest.ErrorResp(ctx.Request.Context(), http.StatusBadRequest, err.Error(), nil))
		return
	}
	
	log.GlobalLogger().WithContext(ctx).Infof("Received generic webhook alert")
	
	// Try to convert to unified format
	alert := convertGenericWebhookToUnified(data)
	
	// Process alert
	if err := processAlert(ctx.Request.Context(), alert); err != nil {
		log.GlobalLogger().WithContext(ctx).Errorf("Failed to process generic webhook alert: %v", err)
		ctx.JSON(http.StatusInternalServerError, rest.ErrorResp(ctx.Request.Context(), http.StatusInternalServerError, err.Error(), nil))
		return
	}
	
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx.Request.Context(), gin.H{
		"alert_id": alert.ID,
	}))
}

// convertVMAlertToUnified converts VMAlert webhook to unified alert format
func convertVMAlertToUnified(webhook *VMAlertWebhook) []*UnifiedAlert {
	alerts := make([]*UnifiedAlert, 0, len(webhook.Alerts))
	
	for _, item := range webhook.Alerts {
		alert := &UnifiedAlert{
			ID:          item.Fingerprint,
			Source:      SourceMetric,
			AlertName:   item.Labels["alertname"],
			Severity:    item.Labels["severity"],
			Status:      item.Status,
			Labels:      item.Labels,
			Annotations: item.Annotations,
			StartsAt:    parseTime(item.StartsAt),
		}
		
		// Handle resolved alerts
		if item.EndsAt != "" && item.EndsAt != "0001-01-01T00:00:00Z" {
			endsAt := parseTime(item.EndsAt)
			alert.EndsAt = &endsAt
		}
		
		// Extract context information from labels
		alert.WorkloadID = item.Labels["workload_id"]
		alert.PodName = item.Labels["pod"]
		alert.PodID = item.Labels["pod_id"]
		alert.NodeName = item.Labels["node"]
		alert.ClusterName = item.Labels["cluster"]
		
		// Merge common labels
		for k, v := range webhook.CommonLabels {
			if _, exists := alert.Labels[k]; !exists {
				alert.Labels[k] = v
			}
		}
		
		// Merge common annotations
		for k, v := range webhook.CommonAnnotations {
			if _, exists := alert.Annotations[k]; !exists {
				alert.Annotations[k] = v
			}
		}
		
		// Store raw data
		rawData, _ := json.Marshal(item)
		alert.RawData = rawData
		
		alerts = append(alerts, alert)
	}
	
	return alerts
}

// convertAlertManagerItemsToUnified converts AlertManager format alerts to unified format
func convertAlertManagerItemsToUnified(items []VMAlertItem) []*UnifiedAlert {
	alerts := make([]*UnifiedAlert, 0, len(items))
	
	for _, item := range items {
		// Ensure labels map is initialized
		labels := item.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		
		// Ensure annotations map is initialized
		annotations := item.Annotations
		if annotations == nil {
			annotations = make(map[string]string)
		}
		
		// Generate fingerprint if not provided
		fingerprint := item.Fingerprint
		if fingerprint == "" {
			// Generate fingerprint from alertname and sorted labels
			fingerprint = generateFingerprintFromLabels(labels)
		}
		
		// Determine status - default to firing if not specified
		status := item.Status
		if status == "" {
			status = StatusFiring
		}
		
		alert := &UnifiedAlert{
			ID:          fingerprint,
			Source:      SourceMetric,
			AlertName:   labels["alertname"],
			Severity:    labels["severity"],
			Status:      status,
			Labels:      labels,
			Annotations: annotations,
			StartsAt:    parseTime(item.StartsAt),
		}
		
		// Handle resolved alerts
		if item.EndsAt != "" && item.EndsAt != "0001-01-01T00:00:00Z" {
			endsAt := parseTime(item.EndsAt)
			alert.EndsAt = &endsAt
		}
		
		// Extract context information from labels
		alert.WorkloadID = labels["workload_id"]
		alert.PodName = labels["pod"]
		alert.PodID = labels["pod_id"]
		alert.NodeName = labels["node"]
		alert.ClusterName = labels["cluster"]
		
		// Store raw data
		rawData, _ := json.Marshal(item)
		alert.RawData = rawData
		
		alerts = append(alerts, alert)
	}
	
	return alerts
}

// convertLogAlertToUnified converts log alert to unified format
func convertLogAlertToUnified(req *LogAlertRequest) *UnifiedAlert {
	// Generate fingerprint based on rule name, workload, pod, and pattern
	fingerprint := generateFingerprint(req.RuleName, req.WorkloadID, req.PodName, req.Pattern)
	
	labels := req.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["alertname"] = req.RuleName
	labels["pattern"] = req.Pattern
	
	annotations := req.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["message"] = req.Message
	
	alert := &UnifiedAlert{
		ID:          fingerprint,
		Source:      SourceLog,
		AlertName:   req.RuleName,
		Severity:    req.Severity,
		Status:      StatusFiring,
		StartsAt:    req.LogTime,
		Labels:      labels,
		Annotations: annotations,
		WorkloadID:  req.WorkloadID,
		PodName:     req.PodName,
		PodID:       req.PodID,
		NodeName:    req.NodeName,
	}
	
	// Store raw data
	rawData, _ := json.Marshal(req)
	alert.RawData = rawData
	
	return alert
}

// convertTraceAlertToUnified converts trace alert to unified format
func convertTraceAlertToUnified(req *TraceAlertRequest) *UnifiedAlert {
	// Generate fingerprint based on rule name, trace ID, and service
	fingerprint := generateFingerprint(req.RuleName, req.TraceID, req.ServiceName, req.Operation)
	
	labels := req.Labels
	if labels == nil {
		labels = make(map[string]string)
	}
	labels["alertname"] = req.RuleName
	labels["trace_id"] = req.TraceID
	labels["span_id"] = req.SpanID
	labels["service_name"] = req.ServiceName
	labels["operation"] = req.Operation
	
	annotations := req.Annotations
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["message"] = req.Message
	annotations["duration"] = fmt.Sprintf("%.2fms", req.Duration)
	
	alert := &UnifiedAlert{
		ID:          fingerprint,
		Source:      SourceTrace,
		AlertName:   req.RuleName,
		Severity:    req.Severity,
		Status:      StatusFiring,
		StartsAt:    time.Now(),
		Labels:      labels,
		Annotations: annotations,
		WorkloadID:  req.WorkloadID,
		PodName:     req.PodName,
	}
	
	// Store raw data
	rawData, _ := json.Marshal(req)
	alert.RawData = rawData
	
	return alert
}

// convertGenericWebhookToUnified converts generic webhook to unified format
func convertGenericWebhookToUnified(data map[string]interface{}) *UnifiedAlert {
	alert := &UnifiedAlert{
		ID:          uuid.New().String(),
		Source:      "webhook",
		AlertName:   getStringValue(data, "alert_name", "generic_webhook_alert"),
		Severity:    getStringValue(data, "severity", SeverityInfo),
		Status:      getStringValue(data, "status", StatusFiring),
		StartsAt:    time.Now(),
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}
	
	// Extract labels if present
	if labels, ok := data["labels"].(map[string]interface{}); ok {
		for k, v := range labels {
			alert.Labels[k] = fmt.Sprintf("%v", v)
		}
	}
	
	// Extract annotations if present
	if annotations, ok := data["annotations"].(map[string]interface{}); ok {
		for k, v := range annotations {
			alert.Annotations[k] = fmt.Sprintf("%v", v)
		}
	}
	
	// Store raw data
	rawData, _ := json.Marshal(data)
	alert.RawData = rawData
	
	return alert
}

// parseTime parses time string in RFC3339 format
func parseTime(timeStr string) time.Time {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Now()
	}
	return t
}

// generateFingerprint generates a unique fingerprint for an alert
func generateFingerprint(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// generateFingerprintFromLabels generates a fingerprint from labels map
func generateFingerprintFromLabels(labels map[string]string) string {
	// Sort label keys for consistent fingerprint
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write([]byte(labels[k]))
		h.Write([]byte(","))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// getStringValue safely gets string value from map
func getStringValue(data map[string]interface{}, key, defaultValue string) string {
	if val, ok := data[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return defaultValue
}

