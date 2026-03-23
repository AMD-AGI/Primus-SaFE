/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package proxy

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/auth"
	"github.com/AMD-AIG-AIMA/SAFE/a2a-gateway/pkg/metrics"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// Handler handles A2A invocation proxying.
type Handler struct {
	dbClient   dbclient.Interface
	httpClient *http.Client
}

// NewHandler creates a new proxy handler.
func NewHandler(dbClient dbclient.Interface) *Handler {
	return &Handler{
		dbClient:   dbClient,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// Invoke handles POST /a2a/invoke/:target and POST /a2a/invoke/:target/:skill
func (h *Handler) Invoke(c *gin.Context) {
	target := c.Param("target")
	skill := c.Param("skill")
	callerUserID := auth.GetUserID(c)
	start := time.Now()

	svc, err := h.dbClient.GetA2AService(c.Request.Context(), target)
	if err != nil || svc == nil {
		metrics.RequestsTotal.WithLabelValues(callerUserID, target, skill, "not_found").Inc()
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("agent %q not found", target)})
		return
	}

	if svc.Status != "active" {
		metrics.RequestsTotal.WithLabelValues(callerUserID, target, skill, "inactive").Inc()
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("agent %q is not active", target)})
		return
	}

	targetURL := svc.Endpoint + svc.A2APathPrefix + "/invoke"
	if skill != "" {
		targetURL += "/" + skill
	}

	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, c.Request.Body)
	if err != nil {
		metrics.RequestsTotal.WithLabelValues(callerUserID, target, skill, "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create proxy request"})
		return
	}
	for k, v := range c.Request.Header {
		proxyReq.Header[k] = v
	}

	resp, err := h.httpClient.Do(proxyReq)
	if err != nil {
		metrics.RequestsTotal.WithLabelValues(callerUserID, target, skill, "error").Inc()
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("failed to reach agent: %v", err)})
		h.logCall(c, callerUserID, target, skill, "error", time.Since(start), 0, 0, err.Error())
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	latency := time.Since(start)

	status := "success"
	if resp.StatusCode >= 400 {
		status = "error"
	}

	metrics.RequestsTotal.WithLabelValues(callerUserID, target, skill, status).Inc()
	metrics.RequestDuration.WithLabelValues(callerUserID, target, skill).Observe(latency.Seconds())

	h.logCall(c, callerUserID, target, skill, status, latency, int64(c.Request.ContentLength), int64(len(body)), "")

	for k, v := range resp.Header {
		for _, vv := range v {
			c.Header(k, vv)
		}
	}
	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

// ListAgents handles GET /a2a/agents
func (h *Handler) ListAgents(c *gin.Context) {
	services, err := h.dbClient.ListActiveA2AServices(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type agentInfo struct {
		ServiceName string `json:"serviceName"`
		DisplayName string `json:"displayName"`
		Endpoint    string `json:"endpoint"`
		Health      string `json:"health"`
	}

	agents := make([]agentInfo, 0, len(services))
	for _, svc := range services {
		agents = append(agents, agentInfo{
			ServiceName: svc.ServiceName,
			DisplayName: svc.DisplayName,
			Endpoint:    svc.Endpoint,
			Health:      svc.A2AHealth,
		})
	}
	c.JSON(http.StatusOK, gin.H{"agents": agents})
}

func (h *Handler) logCall(c *gin.Context, callerUserID, target, skill, status string, latency time.Duration, reqSize, respSize int64, errMsg string) {
	traceID := c.GetHeader("X-Trace-Id")
	if traceID == "" {
		traceID = fmt.Sprintf("gw-%d", time.Now().UnixNano())
	}

	callerAgent := c.GetHeader("X-Caller-Agent")
	if callerAgent == "" {
		callerAgent = "gateway"
	}

	log := &dbclient.A2ACallLog{
		TraceId:           traceID,
		CallerServiceName: callerAgent,
		CallerUserId:      callerUserID,
		TargetServiceName: target,
		SkillId:           skill,
		Status:            status,
		LatencyMs:         float64(latency.Milliseconds()),
		RequestSizeBytes:  reqSize,
		ResponseSizeBytes: respSize,
		CreatedAt:         pq.NullTime{Time: time.Now().UTC(), Valid: true},
	}
	if errMsg != "" {
		log.ErrorMessage = sql.NullString{String: errMsg, Valid: true}
	}

	if err := h.dbClient.InsertA2ACallLog(c.Request.Context(), log); err != nil {
		klog.ErrorS(err, "failed to insert a2a call log")
	}
}
