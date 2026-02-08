// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/gin-gonic/gin"
)

// resolveClusterName resolves a cluster name through the cluster manager.
// If the provided name is not found, falls back to the default cluster.
func resolveClusterName(cluster string) string {
	cm := clientsets.GetClusterManager()
	if cm == nil {
		return ""
	}
	clients, err := cm.GetClusterClientsOrDefault(cluster)
	if err == nil {
		return clients.ClusterName
	}
	// Provided cluster not found, try default
	if cluster != "" {
		clients, err = cm.GetClusterClientsOrDefault("")
		if err == nil {
			return clients.ClusterName
		}
	}
	return ""
}

const (
	// SSE polling interval for reading from database
	SSEPollInterval = 2 * time.Second
	// SSE ping interval to keep connection alive
	SSEPingInterval = 30 * time.Second
	// Maximum SSE connection duration
	SSEMaxDuration = 2 * time.Hour
)

// LiveHandler handles workflow live streaming API
// It reads state from database (updated by SyncExecutor) and pushes to clients via SSE
type LiveHandler struct{}

// NewLiveHandler creates a new LiveHandler
func NewLiveHandler() *LiveHandler {
	return &LiveHandler{}
}

// RegisterRoutes registers the live streaming routes
func (h *LiveHandler) RegisterRoutes(r gin.IRoutes) {
	r.GET("/workflow/runs/:run_id/live", h.HandleLiveStream)
	r.GET("/workflow/runs/:run_id/state", h.GetCurrentState)
	r.POST("/workflow/runs/:run_id/sync/start", h.StartSync)
	r.POST("/workflow/runs/:run_id/sync/stop", h.StopSync)
}

// HandleLiveStream handles SSE streaming for real-time workflow updates
// It polls the database and pushes updates to connected clients
// @Summary Stream workflow run state updates
// @Description Streams real-time updates for a workflow run using Server-Sent Events (SSE)
// @Tags workflow
// @Produce text/event-stream
// @Param run_id path int true "Workflow Run ID"
// @Success 200 {object} WorkflowLiveState
// @Router /v1/github/workflow/runs/{run_id}/live [get]
func (h *LiveHandler) HandleLiveStream(c *gin.Context) {
	// Support both :id and :run_id parameter names for compatibility
	runIDStr := c.Param("id")
	if runIDStr == "" {
		runIDStr = c.Param("run_id")
	}
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run_id"})
		return
	}

	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Access-Control-Allow-Origin", "*")

	ctx := c.Request.Context()
	startTime := time.Now()

	// Ensure sync task is running
	if err := CreateSyncTask(ctx, runID); err != nil {
		log.Warnf("LiveHandler: failed to ensure sync task for run %d: %v", runID, err)
	}

	// Send initial state immediately
	state, err := h.buildStateFromDB(ctx, runID)
	if err != nil {
		log.Warnf("LiveHandler: failed to get initial state for run %d: %v", runID, err)
	}
	if state != nil {
		h.sendSSEEvent(c, "state", state)
	}

	// Track last state for change detection
	var lastStateHash string
	if state != nil {
		lastStateHash = h.computeStateHash(state)
	}

	// Set up tickers
	pollTicker := time.NewTicker(SSEPollInterval)
	defer pollTicker.Stop()

	pingTicker := time.NewTicker(SSEPingInterval)
	defer pingTicker.Stop()

	// Stream updates by polling database
	for {
		select {
		case <-pollTicker.C:
			// Check max duration
			if time.Since(startTime) > SSEMaxDuration {
				h.sendSSEEvent(c, "timeout", gin.H{"reason": "max_duration_exceeded"})
				return
			}

			// Fetch current state from database
			state, err := h.buildStateFromDB(ctx, runID)
			if err != nil {
				log.Warnf("LiveHandler: failed to fetch state for run %d: %v", runID, err)
				continue
			}

			if state == nil {
				continue
			}

			// Check if state changed
			currentHash := h.computeStateHash(state)
			if currentHash != lastStateHash {
				h.sendSSEEvent(c, "state", state)
				lastStateHash = currentHash
			}

			// Close stream when workflow completes
			if state.WorkflowStatus == "completed" {
				h.sendSSEEvent(c, "complete", gin.H{
					"run_id":     state.RunID,
					"conclusion": state.WorkflowConclusion,
				})
				return
			}

		case <-pingTicker.C:
			// Send ping to keep connection alive
			h.sendSSEEvent(c, "ping", gin.H{"timestamp": time.Now().Unix()})

		case <-ctx.Done():
			log.Debugf("LiveHandler: client disconnected for run %d", runID)
			return
		}
	}
}

// buildStateFromDB builds WorkflowLiveState from database (default cluster)
func (h *LiveHandler) buildStateFromDB(ctx context.Context, runID int64) (*WorkflowLiveState, error) {
	return h.buildStateFromDBForCluster(ctx, runID, "")
}

// buildStateFromDBForCluster builds WorkflowLiveState from database for a specific cluster
func (h *LiveHandler) buildStateFromDBForCluster(ctx context.Context, runID int64, clusterName string) (*WorkflowLiveState, error) {
	runFacade := database.GetFacadeForCluster(clusterName).GetGithubWorkflowRun()
	run, err := runFacade.GetByID(ctx, runID)
	if err != nil || run == nil {
		return nil, err
	}

	state := &WorkflowLiveState{
		RunID:              run.ID,
		GithubRunID:        run.GithubRunID,
		WorkflowName:       run.WorkflowName,
		HeadSHA:            run.HeadSha,
		HeadBranch:         run.HeadBranch,
		CollectionStatus:   run.Status,
		CurrentJobName:     run.CurrentJobName,
		CurrentStepName:    run.CurrentStepName,
		ProgressPercent:    int(run.ProgressPercent),
		UpdatedAt:          run.UpdatedAt,
	}

	if !run.LastSyncedAt.IsZero() {
		state.LastSyncedAt = run.LastSyncedAt
	}

	// Calculate elapsed time
	if !run.WorkloadStartedAt.IsZero() {
		state.StartedAt = &run.WorkloadStartedAt
		if run.WorkloadCompletedAt.IsZero() {
			state.ElapsedSeconds = int(time.Since(run.WorkloadStartedAt).Seconds())
		} else {
			state.ElapsedSeconds = int(run.WorkloadCompletedAt.Sub(run.WorkloadStartedAt).Seconds())
		}
	}

	// Determine workflow status from run status
	state.WorkflowStatus = h.inferWorkflowStatus(run)
	state.WorkflowConclusion = run.WorkflowConclusion

	// Load jobs from database.
	// For runner-based runs (github_job_id > 0), return only the single job
	// executed by this K8s runner.
	jobFacade := database.NewGithubWorkflowJobFacade().WithCluster(clusterName)
	var jobsWithSteps []*database.JobWithSteps
	if run.GithubJobID != 0 {
		single, findErr := jobFacade.FindByGithubJobIDWithSteps(ctx, run.GithubJobID)
		if findErr == nil && single != nil {
			jobsWithSteps = []*database.JobWithSteps{single}
		}
	} else {
		jobsWithSteps, _ = jobFacade.ListByRunIDWithSteps(ctx, runID)
	}
	if len(jobsWithSteps) > 0 {
		state.Jobs = make([]*JobLiveState, len(jobsWithSteps))
		for i, job := range jobsWithSteps {
			jobState := &JobLiveState{
				ID:              job.ID,
				GithubJobID:     job.GithubJobID,
				Name:            job.Name,
				Status:          job.Status,
				Conclusion:      job.Conclusion,
				StartedAt:       job.StartedAt,
				CompletedAt:     job.CompletedAt,
				DurationSeconds: job.DurationSeconds,
				RunnerName:      job.RunnerName,
			}

			if job.Steps != nil {
				jobState.Steps = make([]*StepLiveState, len(job.Steps))
				for j, step := range job.Steps {
					jobState.Steps[j] = &StepLiveState{
						Number:          step.StepNumber,
						Name:            step.Name,
						Status:          step.Status,
						Conclusion:      step.Conclusion,
						StartedAt:       step.StartedAt,
						CompletedAt:     step.CompletedAt,
						DurationSeconds: step.DurationSeconds,
					}

					// Track current step
					if step.Status == "in_progress" {
						jobState.CurrentStepNumber = step.StepNumber
						jobState.CurrentStepName = step.Name
					}
				}
			}

			state.Jobs[i] = jobState
		}
	}

	return state, nil
}

// inferWorkflowStatus infers GitHub workflow status from our run status
func (h *LiveHandler) inferWorkflowStatus(run *model.GithubWorkflowRuns) string {
	switch run.Status {
	case database.WorkflowRunStatusWorkloadPending:
		return "queued"
	case database.WorkflowRunStatusWorkloadRunning:
		return "in_progress"
	case database.WorkflowRunStatusCompleted, database.WorkflowRunStatusFailed:
		return "completed"
	default:
		if run.WorkloadCompletedAt.IsZero() {
			return "in_progress"
		}
		return "completed"
	}
}

// computeStateHash computes a simple hash for change detection
func (h *LiveHandler) computeStateHash(state *WorkflowLiveState) string {
	// Use key fields for hash to detect meaningful changes
	return fmt.Sprintf("%s-%s-%d-%s-%s",
		state.WorkflowStatus,
		state.WorkflowConclusion,
		state.ProgressPercent,
		state.CurrentJobName,
		state.CurrentStepName,
	)
}

// sendSSEEvent sends an SSE event to the client
func (h *LiveHandler) sendSSEEvent(c *gin.Context, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Errorf("LiveHandler: failed to marshal SSE data: %v", err)
		return
	}

	c.Writer.WriteString(fmt.Sprintf("event: %s\n", event))
	c.Writer.WriteString(fmt.Sprintf("data: %s\n\n", string(jsonData)))
	c.Writer.Flush()
}

// GetCurrentState returns the current workflow state (non-streaming)
// @Summary Get current workflow state
// @Description Returns the current state of a workflow run without streaming
// @Tags workflow
// @Produce json
// @Param run_id path int true "Workflow Run ID"
// @Success 200 {object} WorkflowLiveState
// @Router /v1/github/workflow/runs/{run_id}/state [get]
func (h *LiveHandler) GetCurrentState(c *gin.Context) {
	// Support both :id and :run_id parameter names for compatibility
	runIDStr := c.Param("id")
	if runIDStr == "" {
		runIDStr = c.Param("run_id")
	}
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run_id"})
		return
	}

	clusterName := resolveClusterName(c.Query("cluster"))
	state, err := h.buildStateFromDBForCluster(c.Request.Context(), runID, clusterName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if state == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "workflow run not found"})
		return
	}

	c.JSON(http.StatusOK, state)
}

// StartSync starts synchronization for a workflow run by creating a sync task
// @Summary Start sync for workflow run
// @Description Starts real-time synchronization for a workflow run
// @Tags workflow
// @Produce json
// @Param run_id path int true "Workflow Run ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/github/workflow/runs/{run_id}/sync/start [post]
func (h *LiveHandler) StartSync(c *gin.Context) {
	// Support both :id and :run_id parameter names for compatibility
	runIDStr := c.Param("id")
	if runIDStr == "" {
		runIDStr = c.Param("run_id")
	}
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run_id"})
		return
	}

	if err := CreateSyncTask(c.Request.Context(), runID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "syncing",
		"run_id":  runID,
		"message": "Sync task created/updated",
	})
}

// StopSync stops synchronization for a workflow run
// @Summary Stop sync for workflow run
// @Description Stops real-time synchronization for a workflow run
// @Tags workflow
// @Produce json
// @Param run_id path int true "Workflow Run ID"
// @Success 200 {object} map[string]interface{}
// @Router /v1/github/workflow/runs/{run_id}/sync/stop [post]
func (h *LiveHandler) StopSync(c *gin.Context) {
	// Support both :id and :run_id parameter names for compatibility
	runIDStr := c.Param("id")
	if runIDStr == "" {
		runIDStr = c.Param("run_id")
	}
	runID, err := strconv.ParseInt(runIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run_id"})
		return
	}

	// Cancel the sync task
	taskFacade := database.NewWorkloadTaskFacade()
	taskUID := generateSyncTaskUID(runID)

	syncTask, err := taskFacade.GetTask(c.Request.Context(), taskUID, TaskTypeGithubWorkflowSync)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if syncTask != nil {
		syncTask.Status = "cancelled"
		if err := taskFacade.UpsertTask(c.Request.Context(), syncTask); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "stopped",
		"run_id": runID,
	})
}
