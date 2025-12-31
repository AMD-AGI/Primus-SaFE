package airegistry

import (
	"context"
	"net/http"
	"time"
)

// HealthChecker checks the health of registered agents
type HealthChecker struct {
	registry           Registry
	client             *http.Client
	unhealthyThreshold int
}

// HealthCheckResult contains the result of a health check
type HealthCheckResult struct {
	AgentName string
	Healthy   bool
	Error     error
	Duration  time.Duration
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(registry Registry, timeout time.Duration, unhealthyThreshold int) *HealthChecker {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	if unhealthyThreshold == 0 {
		unhealthyThreshold = 3
	}

	return &HealthChecker{
		registry: registry,
		client: &http.Client{
			Timeout: timeout,
		},
		unhealthyThreshold: unhealthyThreshold,
	}
}

// CheckAll checks all registered agents and updates their status
func (h *HealthChecker) CheckAll(ctx context.Context) []HealthCheckResult {
	agents, err := h.registry.List(ctx)
	if err != nil {
		return nil
	}

	results := make([]HealthCheckResult, 0, len(agents))
	for _, agent := range agents {
		result := h.Check(ctx, agent)
		results = append(results, result)
	}

	return results
}

// Check checks a single agent's health
func (h *HealthChecker) Check(ctx context.Context, agent *AgentRegistration) HealthCheckResult {
	start := time.Now()

	healthPath := agent.HealthCheckPath
	if healthPath == "" {
		healthPath = "/health"
	}

	url := agent.Endpoint + healthPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return h.recordResult(ctx, agent, false, err, time.Since(start))
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return h.recordResult(ctx, agent, false, err, time.Since(start))
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 300
	return h.recordResult(ctx, agent, healthy, nil, time.Since(start))
}

// recordResult updates the agent status based on health check result
func (h *HealthChecker) recordResult(ctx context.Context, agent *AgentRegistration, healthy bool, err error, duration time.Duration) HealthCheckResult {
	result := HealthCheckResult{
		AgentName: agent.Name,
		Healthy:   healthy,
		Error:     err,
		Duration:  duration,
	}

	var newStatus AgentStatus
	var failureCount int

	if healthy {
		newStatus = AgentStatusHealthy
		failureCount = 0
	} else {
		failureCount = agent.FailureCount + 1
		if failureCount >= h.unhealthyThreshold {
			newStatus = AgentStatusUnhealthy
		} else {
			// Keep current status until threshold reached
			newStatus = agent.Status
			if newStatus == "" {
				newStatus = AgentStatusUnknown
			}
		}
	}

	// Update status in registry
	h.registry.UpdateStatus(ctx, agent.Name, newStatus, failureCount)

	return result
}

// IsHealthy returns true if the agent is healthy
func IsHealthy(status AgentStatus) bool {
	return status == AgentStatusHealthy
}

// ShouldRetry returns true if we should retry calling the agent
func ShouldRetry(status AgentStatus) bool {
	return status != AgentStatusUnhealthy
}
