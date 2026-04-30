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
	dbClient           dbclient.Interface
	k8sClient          ctrlclient.Client
	clawClient         *ClawClient
	clawAgentID        string
	defaultWS          string
	maxConcurrent      int
	proxyImageRegistry string
	hubs               *hubRegistry
	// wsLocks serialises the concurrency-check + DB-insert pair per workspace
	// so two simultaneous requests can't both pass the maxConcurrent gate.
	wsLocks *workspaceLockMap
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
		dbClient:           dbClient,
		k8sClient:          k8sClient,
		clawClient:         NewClawClient(baseURL, apiKey),
		clawAgentID:        commonconfig.GetModelOptimizationClawAgentID(),
		defaultWS:          commonconfig.GetModelOptimizationDefaultWorkspace(),
		maxConcurrent:      commonconfig.GetModelOptimizationMaxConcurrent(),
		proxyImageRegistry: commonconfig.GetGlobalImageRegistry(),
		hubs:               newHubRegistry(),
		wsLocks:            newWorkspaceLockMap(),
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
	resp, err := h.submitTask(c.Request.Context(), &req, userID, userName, "", clawBearerForGin(c))
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
	if err := validateCreateTaskRequest(req); err != nil {
		return nil, err
	}

	workspace := req.Workspace
	if workspace == "" {
		workspace = h.defaultWS
	}

	// Serialize count+insert per workspace to prevent TOCTOU on the
	// concurrency gate — two simultaneous requests cannot both sneak through.
	unlock := h.wsLocks.lock(workspace)
	defer unlock()

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

	resolved, err := ResolveModelForOptimization(ctx, h.dbClient, req.ModelID, workspace)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}

	promptCfg := h.promptConfigFromRequest(req, resolved, workspace)
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

	sessionName := fmt.Sprintf("safe-opt-%s-%s", resolved.DisplayName, taskID[len(taskID)-8:])
	sessionID, err := withClawRetry(ctx, clawBearer, "create session", func(rctx context.Context) (string, error) {
		return h.clawClient.CreateSession(rctx, &SessionRequest{
			Name:    sessionName,
			AgentID: h.clawAgentID,
		})
	})
	if err != nil {
		klog.ErrorS(err, "create claw session", "task_id", taskID)
		_ = h.dbClient.UpdateOptimizationTaskStatus(ctx, taskID,
			dbclient.OptimizationTaskStatusFailed, 0, "failed to create Claw session: "+err.Error())
		return nil, commonerrors.NewInternalError("failed to create Claw session")
	}
	_ = h.dbClient.UpdateOptimizationTaskClawSession(ctx, taskID, sessionID)

	_, sendErr := withClawRetry(ctx, clawBearer, "send message", func(rctx context.Context) (struct{}, error) {
		return struct{}{}, h.clawClient.SendMessage(rctx, sessionID, &MessageRequest{
			Content:     prompt,
			MessageType: "text",
			TaskMode:    "agent",
			WorkspaceID: workspace,
			Tools:       []int{16, 18},
		})
	})
	if sendErr != nil {
		klog.ErrorS(sendErr, "send hyperloom prompt", "task_id", taskID, "session_id", sessionID)
		cleanupCtx, cleanupCancel := context.WithTimeout(WithClawBearer(context.Background(), clawBearer), 10*time.Second)
		defer cleanupCancel()
		_ = h.clawClient.DeleteSession(cleanupCtx, sessionID)
		_ = h.dbClient.UpdateOptimizationTaskStatus(ctx, taskID,
			dbclient.OptimizationTaskStatusFailed, 0, "failed to send prompt: "+sendErr.Error())
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
		cleanupCtx, cleanupCancel := context.WithTimeout(WithClawBearer(context.Background(), clawBearerForGin(c)), 10*time.Second)
		defer cleanupCancel()
		_ = h.clawClient.DeleteSession(cleanupCtx, task.ClawSessionID)
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
	for _, dbev := range events {
		if dbev.Seq > lastSeq {
			lastSeq = dbev.Seq
		}
		if err := writeFrame(dbEventToAPI(dbev)); err != nil {
			return
		}
	}

	// If the task is already finished and no new events are coming, emit a
	// terminal done frame and return.
	if task.Status == dbclient.OptimizationTaskStatusSucceeded ||
		task.Status == dbclient.OptimizationTaskStatusFailed ||
		task.Status == dbclient.OptimizationTaskStatusInterrupted {
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
	// Cap stream lifetime at 4 hours so a hung Claw session never leaks a
	// goroutine forever. Hyperloom typically finishes in under 90 minutes.
	ctx, cancel := context.WithTimeout(WithClawBearer(context.Background(), clawBearer), 4*time.Hour)
	defer cancel()
	go func() {
		<-hub.Done()
		cancel()
	}()

	// Retry loop: on transient drops (EOF, network reset) resume from the
	// last Claw event id using ?after=<id>. Give up after maxStreamRetries.
	const maxStreamRetries = 10
	lastClawEventID := ""
	retryDelay := 3 * time.Second

	for attempt := 0; attempt <= maxStreamRetries; attempt++ {
		if attempt > 0 {
			klog.InfoS("claw stream: reconnecting",
				"task_id", taskID, "attempt", attempt, "after", lastClawEventID)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
			if retryDelay < 30*time.Second {
				retryDelay *= 2
			}
		}

		err := h.clawClient.Stream(ctx, sessionID, lastClawEventID, func(raw ClawSSEEvent) error {
			if raw.ID != "" {
				lastClawEventID = raw.ID
			}
			parsed := parser.Parse(raw)
			for _, p := range parsed {
				ev := h.buildEvent(taskID, hub, p.Type, p.Payload)
				h.persistAndBroadcast(taskID, hub, ev)
				h.maybeUpdateTaskStatus(taskID, p)
			}
			return nil
		})

		if err == nil || errors.Is(err, context.Canceled) || ctx.Err() != nil {
			return
		}

		klog.ErrorS(err, "claw stream dropped, will retry",
			"task_id", taskID, "session_id", sessionID, "attempt", attempt)
		if attempt == maxStreamRetries {
			streamErr = err
		}
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

const maxEventPayloadBytes = 2048

func (h *Handler) persistAndBroadcast(taskID string, hub *taskHub, ev Event) {
	// Broadcast to live SSE subscribers first; persistence is best-effort.
	hub.broadcast(ev)

	// Skip high-volume read events (tool:read carries full skill file content
	// that is static and can be very large; storing it fills the DB quickly).
	if ev.Type == EventTypeLog {
		var lp LogEventPayload
		if err := json.Unmarshal(ev.Payload, &lp); err == nil && lp.Source == "tool:read" {
			return
		}
	}

	payload := string(ev.Payload)
	if len(payload) > maxEventPayloadBytes {
		payload = payload[:maxEventPayloadBytes]
	}

	seq := parseSeqFromEventID(ev.ID)
	dbev := &dbclient.OptimizationEvent{
		EventID:   ev.ID,
		TaskID:    taskID,
		Type:      string(ev.Type),
		Payload:   payload,
		Seq:       seq,
		Timestamp: ev.Timestamp,
	}
	if err := h.dbClient.AppendOptimizationEvent(context.Background(), dbev); err != nil {
		klog.ErrorS(err, "persist optimization event failed",
			"task_id", taskID, "seq", seq)
	}
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
				status = dbclient.OptimizationTaskStatusFailed
				msg = "claw stream error: " + streamErr.Error()
			} else {
				msg = "completed"
			}
			_ = h.dbClient.UpdateOptimizationTaskStatus(
				context.Background(), taskID, status, task.CurrentPhase, msg,
			)
		}
	}
	done := makeDoneEvent(taskID, hub.nextSeq(), status, msg)
	h.persistAndBroadcast(taskID, hub, done)
	h.hubs.remove(taskID)
}

// ── Helpers ─────────────────────────────────────────────────────────────

// clawBearerForGin resolves the Bearer token for outbound PrimusClaw calls. Order matches
// Primus-Claw + SaFE /auth/verify semantics: explicit user API key, then per-user platform
// key (same as Hyperloom cookie flows), then optional file-based service key.
func clawBearerForGin(c *gin.Context) string {
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

func (h *Handler) promptConfigFromRequest(req *CreateTaskRequest, m *ResolvedModel, workspace string) PromptConfig {
	return PromptConfig{
		ProxyImageRegistry: h.proxyImageRegistry,
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
