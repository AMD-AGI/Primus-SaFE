// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package api

import (
	"net/http"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/gin-gonic/gin"
)

// AgentHandler handles agent registration API requests
type AgentHandler struct {
	registry airegistry.Registry
}

// NewAgentHandler creates a new AgentHandler
func NewAgentHandler(registry airegistry.Registry) *AgentHandler {
	return &AgentHandler{
		registry: registry,
	}
}

// RegisterRequest represents the agent registration request
type RegisterRequest struct {
	Name            string            `json:"name" binding:"required"`
	Endpoint        string            `json:"endpoint" binding:"required"`
	Topics          []string          `json:"topics" binding:"required"`
	HealthCheckPath string            `json:"health_check_path"`
	TimeoutSecs     int               `json:"timeout_secs"`
	Metadata        map[string]string `json:"metadata"`
}

// AgentResponse represents an agent in API responses
type AgentResponse struct {
	Name            string            `json:"name"`
	Endpoint        string            `json:"endpoint"`
	Topics          []string          `json:"topics"`
	Status          string            `json:"status"`
	HealthCheckPath string            `json:"health_check_path"`
	TimeoutSecs     int               `json:"timeout_secs"`
	LastHealthCheck *time.Time        `json:"last_health_check,omitempty"`
	FailureCount    int               `json:"failure_count"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	RegisteredAt    time.Time         `json:"registered_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// Register handles POST /api/v1/ai/agents/register
func (h *AgentHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Set defaults
	healthCheckPath := req.HealthCheckPath
	if healthCheckPath == "" {
		healthCheckPath = "/health"
	}

	timeoutSecs := req.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = 60
	}

	agent := &airegistry.AgentRegistration{
		Name:            req.Name,
		Endpoint:        req.Endpoint,
		Topics:          req.Topics,
		HealthCheckPath: healthCheckPath,
		Timeout:         time.Duration(timeoutSecs) * time.Second,
		Status:          airegistry.AgentStatusUnknown,
		Metadata:        req.Metadata,
	}

	if err := h.registry.Register(c.Request.Context(), agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Agent registered successfully",
		"name":    req.Name,
	})
}

// Unregister handles DELETE /api/v1/ai/agents/:name
func (h *AgentHandler) Unregister(c *gin.Context) {
	name := c.Param("name")

	if err := h.registry.Unregister(c.Request.Context(), name); err != nil {
		if err == airegistry.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Agent not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Agent unregistered successfully",
		"name":    name,
	})
}

// List handles GET /api/v1/ai/agents
func (h *AgentHandler) List(c *gin.Context) {
	agents, err := h.registry.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	response := make([]AgentResponse, len(agents))
	for i, agent := range agents {
		response[i] = toAgentResponse(agent)
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": response,
		"total":  len(response),
	})
}

// Get handles GET /api/v1/ai/agents/:name
func (h *AgentHandler) Get(c *gin.Context) {
	name := c.Param("name")

	agent, err := h.registry.Get(c.Request.Context(), name)
	if err != nil {
		if err == airegistry.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Agent not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, toAgentResponse(agent))
}

// GetHealth handles GET /api/v1/ai/agents/:name/health
func (h *AgentHandler) GetHealth(c *gin.Context) {
	name := c.Param("name")

	agent, err := h.registry.Get(c.Request.Context(), name)
	if err != nil {
		if err == airegistry.ErrAgentNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Agent not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"name":              agent.Name,
		"status":            agent.Status,
		"last_health_check": agent.LastHealthCheck,
		"failure_count":     agent.FailureCount,
	})
}

// toAgentResponse converts an AgentRegistration to AgentResponse
func toAgentResponse(agent *airegistry.AgentRegistration) AgentResponse {
	var lastHealthCheck *time.Time
	if !agent.LastHealthCheck.IsZero() {
		lastHealthCheck = &agent.LastHealthCheck
	}

	return AgentResponse{
		Name:            agent.Name,
		Endpoint:        agent.Endpoint,
		Topics:          agent.Topics,
		Status:          string(agent.Status),
		HealthCheckPath: agent.HealthCheckPath,
		TimeoutSecs:     int(agent.Timeout.Seconds()),
		LastHealthCheck: lastHealthCheck,
		FailureCount:    agent.FailureCount,
		Metadata:        agent.Metadata,
		RegisteredAt:    agent.RegisteredAt,
		UpdatedAt:       agent.UpdatedAt,
	}
}

