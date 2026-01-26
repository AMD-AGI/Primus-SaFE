// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/exporter"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/task"
	"github.com/AMD-AGI/Primus-SaFE/Lens/inference-metrics-exporter/pkg/transformer"
	"github.com/gin-gonic/gin"
)

// API holds dependencies for API handlers
type API struct {
	taskReceiver  *task.TaskReceiver
	exporter      *exporter.MetricsExporter
	configWatcher *config.ConfigWatcher
}

// RegisterRoutes registers all API routes
func RegisterRoutes(group *gin.RouterGroup, receiver *task.TaskReceiver, exp *exporter.MetricsExporter, watcher *config.ConfigWatcher) error {
	api := &API{
		taskReceiver:  receiver,
		exporter:      exp,
		configWatcher: watcher,
	}

	// Register /metrics endpoint at the group root for Prometheus scraping
	// This exposes all inference metrics collected from workloads
	group.GET("/metrics", func(c *gin.Context) {
		exp.Handler().ServeHTTP(c.Writer, c.Request)
	})

	// Inference exporter specific routes
	inference := group.Group("/inference-exporter")
	{
		// Targets management
		inference.GET("/targets", api.ListTargets)
		inference.GET("/targets/:uid", api.GetTarget)
		inference.GET("/targets/:uid/metrics", api.GetTargetMetrics)

		// Status and health
		inference.GET("/status", api.GetStatus)

		// Metrics debug endpoints
		inference.GET("/metrics/stats", api.GetMetricsStats)
		inference.GET("/metrics/debug", api.GetMetricsDebug)

		// Config management
		inference.POST("/config/reload", api.ReloadConfig)
		inference.GET("/config/frameworks", api.ListFrameworkConfigs)
		inference.GET("/config/frameworks/:name", api.GetFrameworkConfig)
	}

	return nil
}

// TargetResponse represents a scrape target in API responses
type TargetResponse struct {
	WorkloadUID         string            `json:"workload_uid"`
	Framework           string            `json:"framework"`
	Namespace           string            `json:"namespace"`
	PodName             string            `json:"pod_name"`
	PodIP               string            `json:"pod_ip"`
	MetricsURL          string            `json:"metrics_url"`
	Status              string            `json:"status"`
	Labels              map[string]string `json:"labels,omitempty"`
	LastScrape          *time.Time        `json:"last_scrape,omitempty"`
	ScrapeCount         int64             `json:"scrape_count"`
	ErrorCount          int64             `json:"error_count"`
	ConsecutiveErrors   int               `json:"consecutive_errors"`
}

// ListTargetsResponse is the response for listing targets
type ListTargetsResponse struct {
	Targets []TargetResponse `json:"targets"`
	Summary TargetsSummary   `json:"summary"`
}

// TargetsSummary contains summary statistics
type TargetsSummary struct {
	Total      int            `json:"total"`
	Healthy    int            `json:"healthy"`
	Unhealthy  int            `json:"unhealthy"`
	ByFramework map[string]int `json:"by_framework"`
}

// ListTargets returns all active scrape targets
// GET /v1/inference-exporter/targets
func (a *API) ListTargets(c *gin.Context) {
	tasks := a.taskReceiver.GetActiveTasks()

	targets := make([]TargetResponse, 0, len(tasks))
	summary := TargetsSummary{
		ByFramework: make(map[string]int),
	}

	for _, t := range tasks {
		target := TargetResponse{
			WorkloadUID:       t.WorkloadUID,
			Framework:         t.Ext.Framework,
			Namespace:         t.Ext.Namespace,
			PodName:           t.Ext.PodName,
			PodIP:             t.Ext.PodIP,
			MetricsURL:        t.GetMetricsURL(),
			Status:            t.Status,
			Labels:            t.Ext.Labels,
			LastScrape:        t.Ext.LastScrapeAt,
			ScrapeCount:       t.Ext.ScrapeCount,
			ErrorCount:        t.Ext.ErrorCount,
			ConsecutiveErrors: t.Ext.ConsecutiveErrs,
		}
		targets = append(targets, target)

		summary.Total++
		summary.ByFramework[t.Ext.Framework]++

		if t.Ext.ConsecutiveErrs > 0 {
			summary.Unhealthy++
		} else {
			summary.Healthy++
		}
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, ListTargetsResponse{
		Targets: targets,
		Summary: summary,
	}))
}

// GetTarget returns a specific scrape target
// GET /v1/inference-exporter/targets/:uid
func (a *API) GetTarget(c *gin.Context) {
	uid := c.Param("uid")

	t, found := a.taskReceiver.GetTask(uid)
	if !found {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "Target not found", nil))
		return
	}

	target := TargetResponse{
		WorkloadUID:       t.WorkloadUID,
		Framework:         t.Ext.Framework,
		Namespace:         t.Ext.Namespace,
		PodName:           t.Ext.PodName,
		PodIP:             t.Ext.PodIP,
		MetricsURL:        t.GetMetricsURL(),
		Status:            t.Status,
		Labels:            t.Ext.Labels,
		LastScrape:        t.Ext.LastScrapeAt,
		ScrapeCount:       t.Ext.ScrapeCount,
		ErrorCount:        t.Ext.ErrorCount,
		ConsecutiveErrors: t.Ext.ConsecutiveErrs,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, target))
}

// StatusResponse is the response for status endpoint
type StatusResponse struct {
	InstanceID       string         `json:"instance_id"`
	Status           string         `json:"status"`
	ActiveTargets    int            `json:"active_targets"`
	MaxTargets       int            `json:"max_targets"`
	Uptime           string         `json:"uptime"`
	TasksByFramework map[string]int `json:"tasks_by_framework"`
}

var startTime = time.Now()

// GetStatus returns the exporter status
// GET /v1/inference-exporter/status
func (a *API) GetStatus(c *gin.Context) {
	stats := a.taskReceiver.GetStats()

	status := "healthy"
	if stats.ActiveTasks >= stats.MaxTasks {
		status = "at_capacity"
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, StatusResponse{
		InstanceID:       stats.InstanceID,
		Status:           status,
		ActiveTargets:    stats.ActiveTasks,
		MaxTargets:       stats.MaxTasks,
		Uptime:           time.Since(startTime).String(),
		TasksByFramework: stats.TasksByFramework,
	}))
}

// ReloadConfigRequest is the request for config reload
type ReloadConfigRequest struct {
	Framework string `json:"framework,omitempty"` // Empty means all
}

// ReloadConfigResponse is the response for config reload
type ReloadConfigResponse struct {
	Message   string   `json:"message"`
	Reloaded  []string `json:"reloaded,omitempty"`
	Framework string   `json:"framework,omitempty"`
}

// ReloadConfig triggers a config reload
// POST /v1/inference-exporter/config/reload
func (a *API) ReloadConfig(c *gin.Context) {
	var req ReloadConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body
	}

	if a.configWatcher == nil {
		c.JSON(http.StatusServiceUnavailable, rest.ErrorResp(c.Request.Context(), http.StatusServiceUnavailable, "Config watcher not initialized", nil))
		return
	}

	if req.Framework != "" {
		// Reload specific framework
		if err := a.configWatcher.ReloadFramework(c.Request.Context(), req.Framework); err != nil {
			c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, "Failed to reload config", err))
			return
		}
		c.JSON(http.StatusOK, rest.SuccessResp(c, ReloadConfigResponse{
			Message:   "Config reloaded successfully",
			Framework: req.Framework,
			Reloaded:  []string{req.Framework},
		}))
	} else {
		// Reload all frameworks
		reloaded, err := a.configWatcher.ReloadAll(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, "Failed to reload configs", err))
			return
		}
		c.JSON(http.StatusOK, rest.SuccessResp(c, ReloadConfigResponse{
			Message:  "All configs reloaded",
			Reloaded: reloaded,
		}))
	}
}

// FrameworkConfigResponse represents a framework config in API response
type FrameworkConfigResponse struct {
	Framework   string                       `json:"framework"`
	ConfigHash  string                       `json:"config_hash,omitempty"`
	Mappings    int                          `json:"mappings_count"`
	MappingList []transformer.MetricMapping  `json:"mappings,omitempty"`
}

// ListFrameworkConfigs lists all loaded framework configurations
// GET /v1/inference-exporter/config/frameworks
func (a *API) ListFrameworkConfigs(c *gin.Context) {
	frameworks := transformer.DefaultRegistry.Frameworks()

	configs := make([]FrameworkConfigResponse, 0, len(frameworks))
	for _, f := range frameworks {
		t, ok := transformer.DefaultRegistry.Get(f)
		if !ok {
			continue
		}

		cfg := FrameworkConfigResponse{
			Framework: f,
		}

		// Get hash if watcher is available
		if a.configWatcher != nil {
			if hash, ok := a.configWatcher.GetConfigHash(f); ok {
				cfg.ConfigHash = hash
			}
		}

		// Get mapping count from transformer
		if bt, ok := t.(*transformer.BaseTransformer); ok {
			_ = bt // For future use if needed
		}

		configs = append(configs, cfg)
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
		"frameworks": configs,
		"total":      len(configs),
	}))
}

// GetFrameworkConfig returns config for a specific framework
// GET /v1/inference-exporter/config/frameworks/:name
func (a *API) GetFrameworkConfig(c *gin.Context) {
	name := c.Param("name")

	t, ok := transformer.DefaultRegistry.Get(name)
	if !ok {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "Framework not found", nil))
		return
	}

	cfg := FrameworkConfigResponse{
		Framework: name,
	}

	if a.configWatcher != nil {
		if hash, ok := a.configWatcher.GetConfigHash(name); ok {
			cfg.ConfigHash = hash
		}
	}

	_ = t // For future expansion

	c.JSON(http.StatusOK, rest.SuccessResp(c, cfg))
}

// GetTargetMetrics returns metrics for a specific target
// GET /v1/inference-exporter/targets/:uid/metrics
func (a *API) GetTargetMetrics(c *gin.Context) {
	uid := c.Param("uid")
	format := c.Query("format") // "json" or "prometheus" (default)

	target, found := a.taskReceiver.GetScrapeTarget(uid)
	if !found {
		c.JSON(http.StatusNotFound, rest.ErrorResp(c.Request.Context(), http.StatusNotFound, "Target not found", nil))
		return
	}

	rawMetrics := target.GetRawMetrics()
	transformedMetrics := target.GetTransformedMetrics()

	if format == "json" {
		c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
			"workload_uid":        uid,
			"raw_metric_count":    len(rawMetrics),
			"transformed_count":   len(transformedMetrics),
			"transformed_metrics": transformedMetrics,
		}))
		return
	}

	// Return Prometheus text format
	text, err := a.exporter.SerializeInferenceMetrics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, "Failed to serialize metrics", err))
		return
	}

	c.Data(http.StatusOK, "text/plain; charset=utf-8", text)
}

// MetricsStatsResponse contains metrics statistics
type MetricsStatsResponse struct {
	WorkloadCount  int            `json:"workload_count"`
	MetricFamilies int            `json:"metric_families"`
	TotalMetrics   int            `json:"total_metrics"`
	ByWorkload     map[string]int `json:"by_workload"`
}

// GetMetricsStats returns statistics about collected metrics
// GET /v1/inference-exporter/metrics/stats
func (a *API) GetMetricsStats(c *gin.Context) {
	stats := a.exporter.GetMetricsStats()
	c.JSON(http.StatusOK, rest.SuccessResp(c, MetricsStatsResponse{
		WorkloadCount:  stats.WorkloadCount,
		MetricFamilies: stats.MetricFamilies,
		TotalMetrics:   stats.TotalMetrics,
		ByWorkload:     stats.ByWorkload,
	}))
}

// GetMetricsDebug returns all metrics in debug format
// GET /v1/inference-exporter/metrics/debug
func (a *API) GetMetricsDebug(c *gin.Context) {
	format := c.Query("format") // "json" or "prometheus" (default)

	if format == "json" {
		allMetrics := a.exporter.GetInferenceCollector().GetAllMetrics()
		c.JSON(http.StatusOK, rest.SuccessResp(c, gin.H{
			"workload_count": len(allMetrics),
			"metrics":        allMetrics,
		}))
		return
	}

	// Return Prometheus text format
	text, err := a.exporter.SerializeInferenceMetrics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, rest.ErrorResp(c.Request.Context(), http.StatusInternalServerError, "Failed to serialize metrics", err))
		return
	}

	c.Data(http.StatusOK, "text/plain; charset=utf-8", text)
}

