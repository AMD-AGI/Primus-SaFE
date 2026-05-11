/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package optimization

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/handlers/authority"
	apiutils "github.com/AMD-AIG-AIMA/SAFE/apiserver/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// Handler is the HTTP entry point for Model Optimization. It orchestrates
// DB persistence, Claw session management, and the per-task event hub.
//
// A single Handler instance is shared across all HTTP requests for the life
// of the apiserver process. All its fields are immutable after construction;
// per-task state lives inside taskHub entries on the hubRegistry.
type Handler struct {
	dbClient        dbclient.Interface
	k8sClient       ctrlclient.Client
	clawClient      *ClawClient
	clawAgentID     string
	defaultWS       string
	maxConcurrent   int
	hubs            *hubRegistry
	hyperloomPlugin int // Claw plugin ID for GPU resource resolution (0 = disabled)
}

// NewHandler instantiates the handler. Returns nil and a log warning when
// prerequisite configuration is missing, so the caller can skip route
// registration without failing apiserver startup.
func NewHandler(k8sClient ctrlclient.Client, dbClient dbclient.Interface) (*Handler, error) {
	if dbClient == nil {
		return nil, errors.New("model optimization: database client is required")
	}
	if k8sClient == nil {
		return nil, errors.New("model optimization: k8s client is required")
	}
	baseURL := commonconfig.GetModelOptimizationClawBaseURL()
	if baseURL == "" {
		klog.Warning("model optimization: claw_base_url unset, global.domain/sub_domain could not derive https://<host>/claw-api/v1; create/stream will fail until configured")
	}
	apiKey := commonconfig.GetModelOptimizationClawAPIKey()
	return &Handler{
		dbClient:        dbClient,
		k8sClient:       k8sClient,
		clawClient:      NewClawClient(baseURL, apiKey),
		clawAgentID:     commonconfig.GetModelOptimizationClawAgentID(),
		defaultWS:       commonconfig.GetModelOptimizationDefaultWorkspace(),
		maxConcurrent:   commonconfig.GetModelOptimizationMaxConcurrent(),
		hubs:            newHubRegistry(),
		hyperloomPlugin: commonconfig.GetModelOptimizationClawPluginID(),
	}, nil
}

// ── CreateTask ──────────────────────────────────────────────────────────

// CreateTask handles POST /v1/optimization/tasks. It validates the target
// Model, persists the task, creates a Claw session, and starts an async
// consumer goroutine that fans SSE events out through the task hub.
func (h *Handler) CreateTask(c *gin.Context) {
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid request body: "+err.Error()))
		return
	}
	userID := c.GetString(common.UserId)
	userName := c.GetString(common.UserName)
	resp, err := h.submitTask(c.Request.Context(), &req, userID, userName, "", h.clawBearerForGin(c))
	if err != nil {
		apiutils.AbortWithApiError(c, err)
		return
	}
	c.JSON(http.StatusCreated, resp)
}

func (h *Handler) submitTask(
	ctx context.Context,
	req *CreateTaskRequest,
	userID, userName string,
	fixedTaskID string,
	clawBearer string,
) (*CreateTaskResponse, error) {
	if h.clawClient == nil || h.clawClient.baseURL == "" {
		return nil, commonerrors.NewInternalError("Claw base URL not configured; Model Optimization disabled")
	}
	if strings.TrimSpace(clawBearer) == "" {
		return nil, commonerrors.NewUnauthorized(
			"PrimusClaw authentication required: log in (platform key), send Authorization: Bearer ak-..., or configure model_optimization secret claw_api_key",
		)
	}

	workspace := req.Workspace
	if workspace == "" {
		workspace = h.defaultWS
	}

	if h.maxConcurrent > 0 {
		running, err := h.dbClient.CountRunningOptimizationTasks(ctx, workspace)
		if err != nil {
			klog.ErrorS(err, "count running optimization tasks", "workspace", workspace)
			return nil, commonerrors.NewInternalError("failed to enforce concurrency limit")
		}
		if int(running) >= h.maxConcurrent {
			return nil, commonerrors.NewBadRequest(
				fmt.Sprintf("workspace %q already has %d running optimization tasks (limit=%d)",
					workspace, running, h.maxConcurrent),
			)
		}
	}

	resolved, err := ResolveModelForOptimization(ctx, h.dbClient, h.k8sClient, req.ModelID, workspace)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	promptCfg := NormalizePromptConfig(promptConfigFromRequest(req, resolved, workspace))
	prompt := BuildHyperloomPrompt(promptCfg)

	taskID := fixedTaskID
	if taskID == "" {
		taskID = "opt-" + uuid.NewString()
	}
	task := &dbclient.OptimizationTask{
		ID:             taskID,
		DisplayName:    firstNonEmpty(req.DisplayName, resolved.DisplayName, req.ModelID),
		Workspace:      workspace,
		UserID:         userID,
		UserName:       userName,
		ModelID:        resolved.ID,
		ModelPath:      resolved.LocalPath,
		Mode:           promptCfg.Mode,
		Framework:      promptCfg.Framework,
		Precision:      promptCfg.Precision,
		TP:             promptCfg.TP,
		EP:             promptCfg.EP,
		GPUType:        promptCfg.GPUType,
		ISL:            promptCfg.ISL,
		OSL:            promptCfg.OSL,
		Concurrency:    promptCfg.Concurrency,
		KernelBackends: mustJSON(promptCfg.KernelBackends),
		GeakStepLimit:  promptCfg.GeakStepLimit,
		Image:          promptCfg.Image,
		ResultsPath:    promptCfg.ResultsPath,
		Prompt:         prompt,
		Status:         dbclient.OptimizationTaskStatusPending,
		CurrentPhase:   0,
		Message:        "",
		ClawSessionID:  "",
		FinalMetrics:   "",
		ReportPath:     "",
		StartedAt:      pq.NullTime{},
		FinishedAt:     pq.NullTime{},
	}
	if err := h.dbClient.UpsertOptimizationTask(ctx, task); err != nil {
		klog.ErrorS(err, "create optimization task: db insert")
		return nil, commonerrors.NewInternalError("failed to persist task")
	}

	createCtx, cancel := context.WithTimeout(WithClawBearer(context.Background(), clawBearer), 30*time.Second)
	defer cancel()
	sessionName := fmt.Sprintf("safe-opt-%s-%s", resolved.DisplayName, taskID[len(taskID)-8:])
	sessionID, err := h.clawClient.CreateSession(createCtx, &SessionRequest{
		Name:    sessionName,
		AgentID: h.clawAgentID,
	})
	if err != nil {
		klog.ErrorS(err, "create claw session", "task_id", taskID)
		_ = h.dbClient.UpdateOptimizationTaskStatus(ctx, taskID,
			dbclient.OptimizationTaskStatusFailed, 0, "failed to create Claw session: "+err.Error())
		return nil, commonerrors.NewInternalError("failed to create Claw session")
	}
	_ = h.dbClient.UpdateOptimizationTaskClawSession(ctx, taskID, sessionID)

	sendCtx, sendCancel := context.WithTimeout(WithClawBearer(context.Background(), clawBearer), 30*time.Second)
	defer sendCancel()
	gpuCount := promptCfg.TP * promptCfg.EP
	if gpuCount <= 0 {
		gpuCount = 1
	}
	msgReq := &MessageRequest{
		Content:     prompt,
		MessageType: "text",
		TaskMode:    "agent",
		WorkspaceID: workspace,
		// Image is read by sessions.ts as body.image and takes priority over the
		// plugin's default image, so the GPU sandbox uses the framework-correct
		// container (vLLM or SGLang) rather than the plugin's fixed sglang image.
		Image: promptCfg.Image,
	}
	// Attach the Hyperloom plugin so Claw resolves resource_gpu from the plugin
	// definition — required for GPU sandbox creation. The plugin provides the
	// base resource spec; the resource_gpu.resources override below adjusts
	// GPU/CPU/memory to match the actual TP×EP parallelism requested.
	if h.hyperloomPlugin > 0 {
		msgReq.PluginID = h.hyperloomPlugin
	}
	// Override resource with TP-based counts. Sessions.ts reads body.resource
	// (line 407) and uses it as finalResources, overriding the plugin's fixed
	// default of 8 GPUs with the actual TP×EP count.
	msgReq.Resource = map[string]string{
		"gpu":         fmt.Sprintf("%d", gpuCount), // Brain reads "gpu" key in normalizeWorkloadResourcesEntry
		"amd.com/gpu": fmt.Sprintf("%d", gpuCount), // SaFE workload API k8s resource key
		"cpu":         fmt.Sprintf("%d", promptCfg.RayCpu),
		"memory":      fmt.Sprintf("%dGi", promptCfg.RayMemoryGi),
	}
	if err := h.clawClient.SendMessage(sendCtx, sessionID, msgReq); err != nil {
		klog.ErrorS(err, "send hyperloom prompt", "task_id", taskID, "session_id", sessionID)
		_ = h.clawClient.DeleteSession(WithClawBearer(context.Background(), clawBearer), sessionID)
		_ = h.dbClient.UpdateOptimizationTaskStatus(ctx, taskID,
			dbclient.OptimizationTaskStatusFailed, 0, "failed to send prompt: "+err.Error())
		return nil, commonerrors.NewInternalError("failed to submit task to Claw")
	}

	_ = h.dbClient.UpdateOptimizationTaskStatus(ctx, taskID,
		dbclient.OptimizationTaskStatusRunning, 0, "")
	hub, _ := h.hubs.getOrCreate(taskID, 0)
	go h.consumeClawStream(taskID, sessionID, hub, clawBearer)

	return &CreateTaskResponse{
		ID:            taskID,
		ClawSessionID: sessionID,
	}, nil
}

// ── ListTasks / GetTask / DeleteTask ────────────────────────────────────

// ListTasks handles GET /v1/optimization/tasks.
func (h *Handler) ListTasks(c *gin.Context) {
	var q ListTasksQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		apiutils.AbortWithApiError(c, commonerrors.NewBadRequest("invalid query: "+err.Error()))
		return
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	filter := dbclient.OptimizationTaskFilter{
		Workspace: q.Workspace,
		Status:    q.Status,
		ModelID:   q.ModelID,
		UserID:    q.UserID,
		Search:    q.Search,
		Limit:     q.Limit,
		Offset:    q.Offset,
	}
	tasks, total, err := h.dbClient.ListOptimizationTasks(c.Request.Context(), filter)
	if err != nil {
		klog.ErrorS(err, "list optimization tasks")
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to list tasks"))
		return
	}
	items := make([]TaskInfo, 0, len(tasks))
	for _, t := range tasks {
		items = append(items, taskInfoFromDB(t, false))
	}
	c.JSON(http.StatusOK, ListTasksResponse{
		Total: int(total),
		Items: items,
	})
}

// GetTask handles GET /v1/optimization/tasks/:id.
func (h *Handler) GetTask(c *gin.Context) {
	id := c.Param("id")
	task, err := h.dbClient.GetOptimizationTask(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("task not found"))
			return
		}
		klog.ErrorS(err, "get optimization task", "id", id)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to load task"))
		return
	}
	if task == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("task not found"))
		return
	}
	c.JSON(http.StatusOK, taskInfoFromDB(task, true))
}

// DeleteTask handles DELETE /v1/optimization/tasks/:id. The underlying Claw
// session is closed best-effort; DB soft-delete is authoritative.
func (h *Handler) DeleteTask(c *gin.Context) {
	id := c.Param("id")
	task, err := h.dbClient.GetOptimizationTask(c.Request.Context(), id)
	if err != nil || task == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("task not found"))
		return
	}
	if task.ClawSessionID != "" {
		_ = h.clawClient.DeleteSession(WithClawBearer(context.Background(), h.clawBearerForGin(c)), task.ClawSessionID)
	}
	h.hubs.remove(id)
	if err := h.dbClient.DeleteOptimizationTask(c.Request.Context(), id); err != nil {
		klog.ErrorS(err, "delete optimization task", "id", id)
		apiutils.AbortWithApiError(c, commonerrors.NewInternalError("failed to delete task"))
		return
	}
	c.Status(http.StatusNoContent)
}

// ── Events ──────────────────────────────────────────────────────────────

// StreamEvents handles GET /v1/optimization/tasks/:id/events (SSE). It:
//  1. replays any persisted events after ?after_event_id= (by seq);
//  2. attaches the caller as a hub subscriber for future events;
//  3. writes SSE frames until the task completes or the client disconnects.
func (h *Handler) StreamEvents(c *gin.Context) {
	id := c.Param("id")
	task, err := h.dbClient.GetOptimizationTask(c.Request.Context(), id)
	if err != nil || task == nil {
		apiutils.AbortWithApiError(c, commonerrors.NewNotFoundWithMessage("task not found"))
		return
	}

	afterSeq := parseAfterSeq(c.Query("after_event_id"))

	// Prepare SSE headers before any write.
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		klog.Warning("stream events: response writer is not a Flusher")
		return
	}

	ctx := c.Request.Context()
	writeFrame := func(ev Event) error {
		return writeSSEEvent(c.Writer, flusher, ev)
	}

	// 1. Replay.
	events, err := h.dbClient.ListOptimizationEvents(ctx, id, afterSeq, 10000)
	if err != nil {
		klog.ErrorS(err, "replay optimization events", "task_id", id)
	}
	lastSeq := afterSeq
	isTerminal := task.Status == dbclient.OptimizationTaskStatusSucceeded ||
		task.Status == dbclient.OptimizationTaskStatusFailed ||
		task.Status == dbclient.OptimizationTaskStatusInterrupted
	for _, dbev := range events {
		if dbev.Seq > lastSeq {
			lastSeq = dbev.Seq
		}
		// For terminal tasks, skip persisted done events — we emit a fresh done
		// at the very end so backfill benchmark/kernel events (which have higher
		// synthetic seq numbers) are always delivered before the terminal marker.
		if isTerminal && EventType(dbev.Type) == EventTypeDone {
			continue
		}
		if err := writeFrame(dbEventToAPI(dbev)); err != nil {
			return
		}
	}

	// If the task is already finished and no new events are coming, emit a
	// terminal done frame and return.
	if isTerminal {
		// For succeeded tasks: if the replay produced no benchmark events, the
		// pipeline ran in orchestrator mode (sub-jobs). Try to backfill from
		// Claw session artifacts. This fires asynchronously; on the next page
		// load the events are already in DB and will be replayed before done.
		if task.Status == dbclient.OptimizationTaskStatusSucceeded && !hasBenchmarkEvents(events) {
			go h.fetchAndInjectMetrics(context.Background(), task)
		}
		_ = writeFrame(makeDoneEvent(id, lastSeq+1, task.Status, task.Message))
		return
	}

	// 2. Subscribe to live stream.
	hub, _ := h.hubs.getOrCreate(id, lastSeq)
	subID := uuid.NewString()
	ch, cancel := hub.subscribe(subID, lastSeq)
	defer cancel()

	// 3. Forward live events with a heartbeat.
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-hub.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := writeFrame(ev); err != nil {
				return
			}
		case <-heartbeat.C:
			if _, err := c.Writer.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// ── Background consumer ─────────────────────────────────────────────────

// consumeClawStream is the per-task goroutine that pulls the upstream Claw
// SSE stream, parses it, persists events, and broadcasts them to live HTTP
// subscribers. It runs until the stream ends or returns an error; either
// way the task is transitioned to a terminal status.
func (h *Handler) consumeClawStream(taskID, sessionID string, hub *taskHub, clawBearer string) {
	var streamErr error
	defer func() {
		if r := recover(); r != nil {
			klog.ErrorS(fmt.Errorf("%v", r), "consume claw stream panicked", "task_id", taskID)
			streamErr = fmt.Errorf("panic: %v", r)
		}
		h.finalizeTask(taskID, hub, streamErr)
	}()

	parser := NewSSEParser()
	ctx, cancel := context.WithCancel(WithClawBearer(context.Background(), clawBearer))
	defer cancel()
	// The stream loop runs for as long as Claw keeps the session alive, which
	// for Hyperloom is typically 30-60 minutes. We rely on ctx to cancel on
	// shutdown and on Claw to close the body when the agent finishes.
	go func() {
		<-hub.Done()
		cancel()
	}()

	// Claw does not close the SSE stream when the agent goes idle — the
	// goroutine would block on scanner.Scan() forever. Poll GetSession every
	// 60 s and cancel the stream context once the session reaches a terminal
	// state so finalizeTask runs promptly.
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pollCtx, pollCancel := context.WithTimeout(
					WithClawBearer(context.Background(), clawBearer),
					10*time.Second,
				)
				ss, err := h.clawClient.GetSession(pollCtx, sessionID)
				pollCancel()
				if err != nil {
					klog.V(4).InfoS("consumeClawStream: poll session failed",
						"task_id", taskID, "error", err)
					continue
				}
				if ss.IsTerminal() {
					klog.InfoS("consumeClawStream: session terminal, cancelling stream",
						"task_id", taskID, "session_id", sessionID,
						"status", ss.Status, "agent_status", ss.AgentStatus)
					cancel()
					return
				}
			}
		}
	}()

	onEvent := func(raw ClawSSEEvent) error {
		parsed := parser.Parse(raw)
		for _, p := range parsed {
			ev := h.buildEvent(taskID, hub, p.Type, p.Payload)
			h.persistAndBroadcast(taskID, hub, ev)
			h.maybeUpdateTaskStatus(taskID, p)
		}
		return nil
	}

	for {
		err := h.clawClient.Stream(ctx, sessionID, "", onEvent)
		if err == nil || errors.Is(err, context.Canceled) {
			// Normal exit: stream EOF or context cancelled (agent idle / hub closed).
			break
		}
		// Stream dropped unexpectedly (network blip, LB idle timeout, etc.).
		// Check whether the session is still active before retrying.
		klog.ErrorS(err, "claw stream dropped, checking session before retry",
			"task_id", taskID, "session_id", sessionID)
		checkCtx, checkCancel := context.WithTimeout(
			WithClawBearer(context.Background(), clawBearer),
			10*time.Second,
		)
		ss, getErr := h.clawClient.GetSession(checkCtx, sessionID)
		checkCancel()
		if getErr != nil || ss.IsTerminal() {
			streamErr = err
			break
		}
		// Session still running — wait briefly then reconnect.
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		klog.InfoS("claw stream reconnecting", "task_id", taskID, "session_id", sessionID)
	}
}

func (h *Handler) buildEvent(taskID string, hub *taskHub, evType EventType, payload interface{}) Event {
	seq := hub.nextSeq()
	return Event{
		ID:        fmt.Sprintf("%s-%d", taskID, seq),
		TaskID:    taskID,
		Type:      evType,
		Timestamp: nowMillis(),
		Payload:   marshalPayload(payload),
	}
}

func (h *Handler) persistAndBroadcast(taskID string, hub *taskHub, ev Event) {
	// Persist in a best-effort manner; the live channel still gets the event.
	seq := parseSeqFromEventID(ev.ID)
	dbev := &dbclient.OptimizationEvent{
		EventID:   ev.ID,
		TaskID:    taskID,
		Type:      string(ev.Type),
		Payload:   string(ev.Payload),
		Seq:       seq,
		Timestamp: ev.Timestamp,
	}
	if err := h.dbClient.AppendOptimizationEvent(context.Background(), dbev); err != nil {
		klog.V(4).InfoS("persist optimization event failed",
			"task_id", taskID, "seq", seq, "error", err)
	}
	hub.broadcast(ev)
}

// maybeUpdateTaskStatus promotes the task's DB status when we see either a
// phase transition (updates current_phase) or a terminal status marker.
func (h *Handler) maybeUpdateTaskStatus(taskID string, p ParsedEvent) {
	if p.Type != EventTypePhase {
		return
	}
	phase, ok := p.Payload.(PhaseEventPayload)
	if !ok {
		return
	}
	_ = h.dbClient.UpdateOptimizationTaskStatus(
		context.Background(), taskID,
		dbclient.OptimizationTaskStatusRunning,
		phase.Phase,
		"phase: "+phase.PhaseName,
	)
}

// finalizeTask runs when the Claw stream ends; it marks the task succeeded
// by default (the skill will emit a failed-terminal phase event if needed),
// flushes a done frame, and tears down the hub.
func (h *Handler) finalizeTask(taskID string, hub *taskHub, streamErr error) {
	task, err := h.dbClient.GetOptimizationTask(context.Background(), taskID)
	status := dbclient.OptimizationTaskStatusSucceeded
	msg := ""
	if err == nil && task != nil {
		// Respect any terminal status the phase parser might have set.
		switch task.Status {
		case dbclient.OptimizationTaskStatusFailed,
			dbclient.OptimizationTaskStatusInterrupted,
			dbclient.OptimizationTaskStatusSucceeded:
			status = task.Status
			msg = task.Message
		default:
			if streamErr != nil {
				// The SSE connection dropped (e.g. network blip, LB timeout).
				// The Claw session may still be running — query it before
				// deciding the final status so we don't mark a live task Failed.
				status, msg = h.resolveStatusFromClaw(task.ClawSessionID, streamErr)
			} else {
				// Agent went idle normally. Verify the skill ran to completion by
				// checking for the optimization report — its absence means the skill
				// exited early (e.g. sandbox_create_failed) even though Claw reports idle.
				if task.ClawSessionID != "" && !h.hasOptimizationReport(task.ClawSessionID) {
					status = dbclient.OptimizationTaskStatusFailed
					msg = "optimization report not found; skill may have exited early"
				} else {
					msg = "completed"
				}
			}
			_ = h.dbClient.UpdateOptimizationTaskStatus(
				context.Background(), taskID, status, task.CurrentPhase, msg,
			)
		}
	}
	// If the session is still running (stream dropped transiently), do not emit
	// a terminal done frame and do not tear down the hub. The Detail page will
	// see Running status on next poll and keep showing a live task.
	if status == dbclient.OptimizationTaskStatusRunning {
		h.hubs.remove(taskID)
		return
	}

	done := makeDoneEvent(taskID, hub.nextSeq(), status, msg)
	h.persistAndBroadcast(taskID, hub, done)
	h.hubs.remove(taskID)

	// Best-effort: pull optimization report from Claw session artifacts and
	// inject benchmark/kernel events so the Detail page can display them even
	// when the pipeline ran sub-jobs.
	if task != nil && status == dbclient.OptimizationTaskStatusSucceeded {
		go h.fetchAndInjectMetrics(context.Background(), task)
	}
}

// resolveStatusFromClaw queries the Claw session to determine the true task
// status when the SSE stream dropped unexpectedly. This prevents marking a
// still-running Hyperloom session as Failed due to a transient connection drop.
func (h *Handler) resolveStatusFromClaw(sessionID string, streamErr error) (dbclient.OptimizationTaskStatus, string) {
	fallbackStatus := dbclient.OptimizationTaskStatusFailed
	fallbackMsg := "claw stream error: " + streamErr.Error()

	if sessionID == "" {
		return fallbackStatus, fallbackMsg
	}

	ctx, cancel := context.WithTimeout(
		WithClawBearer(context.Background(), h.clawClient.apiKey),
		10*time.Second,
	)
	defer cancel()

	ss, err := h.clawClient.GetSession(ctx, sessionID)
	if err != nil {
		klog.V(4).InfoS("resolveStatusFromClaw: get session failed, keeping stream error",
			"session_id", sessionID, "error", err)
		return fallbackStatus, fallbackMsg
	}

	klog.InfoS("resolveStatusFromClaw: claw session status",
		"session_id", sessionID,
		"status", ss.Status, "agent_status", ss.AgentStatus, "stream_err", streamErr)

	if !ss.IsTerminal() {
		// Session is still running — do not mark as Failed. Return Running so
		// the caller skips writing a terminal status; the Detail page will show
		// the task as still in progress.
		klog.InfoS("resolveStatusFromClaw: session still active, not marking failed",
			"session_id", sessionID, "agent_status", ss.AgentStatus)
		return dbclient.OptimizationTaskStatusRunning, ""
	}
	if ss.Status == "failed" || ss.AgentStatus == "failed" {
		return dbclient.OptimizationTaskStatusFailed, "claw session failed"
	}
	return dbclient.OptimizationTaskStatusSucceeded, "completed"
}

// hasOptimizationReport checks whether the Claw session contains an
// optimization_report.md artifact. A missing report means the skill exited
// before Phase 10 (Save Results) and the task should be marked Failed.
// Returns true on any listing error so transient failures don't flip a
// genuinely-succeeded task to Failed.
func (h *Handler) hasOptimizationReport(sessionID string) bool {
	ctx, cancel := context.WithTimeout(
		WithClawBearer(context.Background(), h.clawClient.apiKey),
		15*time.Second,
	)
	defer cancel()
	files, err := h.clawClient.ListSessionFiles(ctx, sessionID)
	if err != nil {
		klog.V(4).InfoS("hasOptimizationReport: list files failed, assuming present",
			"session_id", sessionID, "error", err)
		return true
	}
	for _, f := range files {
		if strings.Contains(f.Path, "optimization_report.md") {
			return true
		}
	}
	return false
}

// ── Helpers ─────────────────────────────────────────────────────────────

// clawBearerForTask resolves the Bearer token for a task owner using their
// platform key. Used by recoverRunningTasks so it can access Claw sessions
// that were created with a user-specific token rather than the service key.
func (h *Handler) clawBearerForTask(ctx context.Context, userID, userName string) string {
	if userID != "" {
		if tok := authority.ApiKeyTokenInstance(); tok != nil {
			pk, err := tok.GetOrCreatePlatformKey(ctx, userID, userName)
			if err != nil {
				klog.ErrorS(err, "clawBearerForTask: GetOrCreatePlatformKey failed", "userId", userID)
			} else if strings.TrimSpace(pk) != "" {
				return pk
			}
		}
	}
	return commonconfig.GetModelOptimizationClawAPIKey()
}

// clawBearerForGin resolves the Bearer token for outbound PrimusClaw calls.
//
// Priority:
//  1. Explicit user API key from the request Authorization header.
//  2. Per-user platform key (ak-xxx) via GetOrCreatePlatformKey.
//  3. File-based service key from model_optimization config.
func (h *Handler) clawBearerForGin(c *gin.Context) string {
	if c != nil {
		if k := authority.ExtractApiKeyFromRequest(c.GetHeader("Authorization")); k != "" {
			return k
		}
		userID := c.GetString(common.UserId)
		userName := c.GetString(common.UserName)
		if userID != "" {
			if tok := authority.ApiKeyTokenInstance(); tok != nil {
				pk, err := tok.GetOrCreatePlatformKey(c.Request.Context(), userID, userName)
				if err != nil {
					klog.ErrorS(err, "model optimization: GetOrCreatePlatformKey for PrimusClaw",
						"userId", userID)
				} else if strings.TrimSpace(pk) != "" {
					return pk
				}
			}
		}
	}
	return commonconfig.GetModelOptimizationClawAPIKey()
}

// Start runs background recovery after the handler is registered. It should
// be called once in a goroutine immediately after InitRoutes.
// It reconnects in-progress Claw sessions that survived an apiserver restart
// (tasks still Running in DB but no live goroutine).
func (h *Handler) Start(ctx context.Context) {
	// Give the HTTP server a moment to finish binding before we start making
	// outbound Claw calls (avoids log noise during the startup race).
	time.Sleep(5 * time.Second)

	h.recoverRunningTasks(ctx)
}

// recoverRunningTasks reconnects Claw SSE streams for tasks that were Running
// when the apiserver last exited. For each such task it queries the Claw
// session: still active → restart consumeClawStream; terminal → finalize.
func (h *Handler) recoverRunningTasks(ctx context.Context) {
	tasks, _, err := h.dbClient.ListOptimizationTasks(ctx, dbclient.OptimizationTaskFilter{
		Status: string(dbclient.OptimizationTaskStatusRunning),
		Limit:  1000,
	})
	if err != nil {
		klog.ErrorS(err, "recoverRunningTasks: list running tasks failed")
		return
	}
	if len(tasks) == 0 {
		return
	}
	klog.InfoS("recoverRunningTasks: found running tasks", "count", len(tasks))

	for _, task := range tasks {
		if task.ClawSessionID == "" {
			// No Claw session — mark failed so it doesn't block the concurrency limit.
			_ = h.dbClient.UpdateOptimizationTaskStatus(ctx, task.ID,
				dbclient.OptimizationTaskStatusFailed, task.CurrentPhase, "no claw session after restart")
			continue
		}

		// Resolve a fresh bearer per task using the task owner's platform key so
		// recoverRunningTasks can access sessions created with user-specific tokens.
		clawBearer := h.clawBearerForTask(ctx, task.UserID, task.UserName)
		clawCtx := WithClawBearer(ctx, clawBearer)

		ss, err := h.clawClient.GetSession(clawCtx, task.ClawSessionID)
		if err != nil {
			klog.ErrorS(err, "recoverRunningTasks: get session failed, skipping",
				"task_id", task.ID, "session_id", task.ClawSessionID)
			continue
		}

		if ss.IsTerminal() {
			status, msg := h.resolveStatusFromClaw(task.ClawSessionID, fmt.Errorf("apiserver restarted"))
			_ = h.dbClient.UpdateOptimizationTaskStatus(ctx, task.ID, status, task.CurrentPhase, msg)
			klog.InfoS("recoverRunningTasks: finalized terminal task",
				"task_id", task.ID, "status", status)
			if status == dbclient.OptimizationTaskStatusSucceeded {
				go h.fetchAndInjectMetrics(ctx, task)
			}
		} else {
			// Session still active — reconnect the stream.
			hub, _ := h.hubs.getOrCreate(task.ID, 0)
			go h.consumeClawStream(task.ID, task.ClawSessionID, hub, clawBearer)
			klog.InfoS("recoverRunningTasks: reconnected stream",
				"task_id", task.ID, "session_id", task.ClawSessionID)
		}
	}
}

// getLiteLLMKey returns the user's sk-xxx LiteLLM virtual key for use as
// ANTHROPIC_API_KEY inside the GPU sandbox. Falls back to empty string on any error.
func (h *Handler) getLiteLLMKey(ctx context.Context, userID, userName string) string {
	email := h.getUserEmail(ctx, userID)
	if email == "" {
		return ""
	}
	tok := authority.ApiKeyTokenInstance()
	if tok == nil {
		return ""
	}
	sk, err := tok.GetVirtualKeyByEmail(ctx, email)
	if err != nil {
		klog.V(4).InfoS("getLiteLLMKey: GetVirtualKeyByEmail failed", "userId", userID, "error", err)
		return ""
	}
	return strings.TrimSpace(sk)
}

// getUserEmail looks up the User CR by userId and returns the email annotation.
func (h *Handler) getUserEmail(ctx context.Context, userID string) string {
	if h.k8sClient == nil || userID == "" {
		return ""
	}
	user := &v1.User{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: userID}, user); err != nil {
		return ""
	}
	return v1.GetUserEmail(user)
}

func promptConfigFromRequest(req *CreateTaskRequest, m *ResolvedModel, workspace string) PromptConfig {
	return PromptConfig{
		DisplayName:    firstNonEmpty(req.DisplayName, m.DisplayName),
		ModelName:      m.ModelName,
		ModelPath:      m.LocalPath,
		Mode:           req.Mode,
		Framework:      req.Framework,
		Precision:      req.Precision,
		TP:             req.TP,
		EP:             req.EP,
		GPUType:        req.GPUType,
		ISL:            req.ISL,
		OSL:            req.OSL,
		Concurrency:    req.Concurrency,
		KernelBackends: req.KernelBackends,
		GeakStepLimit:  req.GeakStepLimit,
		Image:          req.Image,
		InferenceXPath: req.InferenceXPath,
		Workspace:      workspace,
		ResultsPath:    req.ResultsPath,
		RayReplica:     req.RayReplica,
		RayGpu:         req.RayGpu,
		RayCpu:         req.RayCpu,
		RayMemoryGi:    req.RayMemory,
		TargetGpu:      req.TargetGpu,
		BaselineCSV:    req.BaselineCSV,
		BaselineCount:  req.BaselineCount,
	}
}

func taskInfoFromDB(t *dbclient.OptimizationTask, includePrompt bool) TaskInfo {
	var kernelBackends []string
	if t.KernelBackends != "" {
		_ = json.Unmarshal([]byte(t.KernelBackends), &kernelBackends)
	}
	info := TaskInfo{
		ID:             t.ID,
		DisplayName:    t.DisplayName,
		ModelID:        t.ModelID,
		ModelPath:      t.ModelPath,
		Workspace:      t.Workspace,
		UserID:         t.UserID,
		UserName:       t.UserName,
		Mode:           t.Mode,
		Framework:      t.Framework,
		Precision:      t.Precision,
		TP:             t.TP,
		EP:             t.EP,
		GPUType:        t.GPUType,
		ISL:            t.ISL,
		OSL:            t.OSL,
		Concurrency:    t.Concurrency,
		KernelBackends: kernelBackends,
		GeakStepLimit:  t.GeakStepLimit,
		Image:          t.Image,
		ResultsPath:    t.ResultsPath,
		ClawSessionID:  t.ClawSessionID,
		Status:         OptimizationTaskStatus(t.Status),
		CurrentPhase:   t.CurrentPhase,
		Message:        t.Message,
		CreatedAt:      nullTimeToISO(t.CreatedAt),
		UpdatedAt:      nullTimeToISO(t.UpdatedAt),
		StartedAt:      nullTimeToISO(t.StartedAt),
		FinishedAt:     nullTimeToISO(t.FinishedAt),
	}
	if includePrompt {
		info.Prompt = t.Prompt
	}
	return info
}

// nullTimeToISO formats a pq.NullTime as an RFC3339 UTC string, or "" if not set.
func nullTimeToISO(t pq.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

func mustJSON(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func dbEventToAPI(dbev *dbclient.OptimizationEvent) Event {
	return Event{
		ID:        dbev.EventID,
		TaskID:    dbev.TaskID,
		Type:      EventType(dbev.Type),
		Timestamp: dbev.Timestamp,
		Payload:   json.RawMessage(dbev.Payload),
	}
}

func parseAfterSeq(afterEventID string) int64 {
	// We encode event ids as "<taskID>-<seq>", so split on the last dash.
	if afterEventID == "" {
		return 0
	}
	parts := strings.Split(afterEventID, "-")
	if len(parts) == 0 {
		return 0
	}
	var n int64
	for _, r := range parts[len(parts)-1] {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	return n
}

func parseSeqFromEventID(eventID string) int64 {
	return parseAfterSeq(eventID)
}

func makeDoneEvent(taskID string, seq int64, status dbclient.OptimizationTaskStatus, message string) Event {
	payload := DoneEventPayload{
		Status:  OptimizationTaskStatus(status),
		Message: message,
	}
	return Event{
		ID:        fmt.Sprintf("%s-%d", taskID, seq),
		TaskID:    taskID,
		Type:      EventTypeDone,
		Timestamp: nowMillis(),
		Payload:   marshalPayload(payload),
	}
}

// writeSSEEvent serializes an Event into a valid SSE frame and flushes it.
func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, ev Event) error {
	var b strings.Builder
	b.WriteString("id: ")
	b.WriteString(ev.ID)
	b.WriteByte('\n')
	b.WriteString("event: ")
	b.WriteString(string(ev.Type))
	b.WriteByte('\n')
	// Encode the full event envelope as JSON so the client gets a
	// self-describing payload.
	data, _ := json.Marshal(ev)
	b.WriteString("data: ")
	b.Write(data)
	b.WriteString("\n\n")
	if _, err := w.Write([]byte(b.String())); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
