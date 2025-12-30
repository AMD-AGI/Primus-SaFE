package api

import (
	"net/http"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/airegistry"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/aitaskqueue"
	"github.com/gin-gonic/gin"
)

// StatsHandler handles statistics API requests
type StatsHandler struct {
	registry  airegistry.Registry
	taskQueue *aitaskqueue.PGStore
}

// NewStatsHandler creates a new StatsHandler
func NewStatsHandler(registry airegistry.Registry, taskQueue *aitaskqueue.PGStore) *StatsHandler {
	return &StatsHandler{
		registry:  registry,
		taskQueue: taskQueue,
	}
}

// StatsResponse represents the statistics response
type StatsResponse struct {
	Agents AgentStats `json:"agents"`
	Tasks  TaskStats  `json:"tasks"`
}

// AgentStats contains agent statistics
type AgentStats struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
	Unknown   int `json:"unknown"`
}

// TaskStats contains task statistics
type TaskStats struct {
	Pending    int64 `json:"pending"`
	Processing int64 `json:"processing"`
	Completed  int64 `json:"completed"`
	Failed     int64 `json:"failed"`
	Cancelled  int64 `json:"cancelled"`
	Total      int64 `json:"total"`
}

// GetStats handles GET /api/v1/ai/stats
func (h *StatsHandler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()

	// Get agent stats
	agentStats := AgentStats{}
	agents, err := h.registry.List(ctx)
	if err == nil {
		agentStats.Total = len(agents)
		for _, agent := range agents {
			switch agent.Status {
			case airegistry.AgentStatusHealthy:
				agentStats.Healthy++
			case airegistry.AgentStatusUnhealthy:
				agentStats.Unhealthy++
			default:
				agentStats.Unknown++
			}
		}
	}

	// Get task stats
	taskStats := TaskStats{}
	queueStats, err := aitaskqueue.GetStats(ctx, h.taskQueue)
	if err == nil {
		taskStats.Pending = queueStats.PendingCount
		taskStats.Processing = queueStats.ProcessingCount
		taskStats.Completed = queueStats.CompletedCount
		taskStats.Failed = queueStats.FailedCount
		taskStats.Cancelled = queueStats.CancelledCount
		taskStats.Total = queueStats.TotalCount
	}

	c.JSON(http.StatusOK, StatsResponse{
		Agents: agentStats,
		Tasks:  taskStats,
	})
}

