/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"crypto/tls"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
	openai "github.com/sashabaranov/go-openai"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
)

// Chat handles direct chat with a model or workload.
// Supports streaming via SSE (Server-Sent Events).
// For remote_api models, uses the model's configured API endpoint.
// For workloads, uses the workload's ingress URL and requires modelName in request.
func (h *Handler) Chat(c *gin.Context) {
	req := &ChatRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("invalid request body: %v", err)})
		return
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		c.JSON(401, gin.H{"error": "user not authenticated"})
		return
	}

	// Validate that either modelId or workloadId is provided
	if req.ModelId == "" && req.WorkloadId == "" {
		c.JSON(400, gin.H{"error": "either modelId or workloadId must be provided"})
		return
	}

	var baseUrl, modelName, apiKey string
	ctx := c.Request.Context()

	if req.ModelId != "" {
		// Chat with remote_api model
		k8sModel := &v1.Model{}
		if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: req.ModelId}, k8sModel); err != nil {
			c.JSON(404, gin.H{"error": fmt.Sprintf("model not found: %v", err)})
			return
		}

		// Verify it's a remote_api model
		if k8sModel.Spec.Source.AccessMode != v1.AccessModeRemoteAPI {
			c.JSON(400, gin.H{"error": "model is not a remote_api type, use workloadId for local model inference"})
			return
		}

		// Verify model is ready
		if k8sModel.Status.Phase != v1.ModelPhaseReady {
			c.JSON(400, gin.H{"error": fmt.Sprintf("model is not ready, current phase: %s", k8sModel.Status.Phase)})
			return
		}

		baseUrl = k8sModel.Spec.Source.URL
		modelName = k8sModel.GetModelName()

		// Get API key from Secret (if apiKey reference exists for remote_api mode)
		if k8sModel.Spec.Source.ApiKey != nil && k8sModel.Spec.Source.ApiKey.Name != "" {
			apiKey = h.getApiKeyFromSecret(ctx, k8sModel.Spec.Source.ApiKey.Name)
		}
	} else {
		// Chat with workload (local inference service)
		k8sWorkload := &v1.Workload{}
		// Workloads are namespaced, we need to find it
		workloadList := &v1.WorkloadList{}
		if err := h.k8sClient.List(ctx, workloadList); err != nil {
			c.JSON(500, gin.H{"error": fmt.Sprintf("failed to list workloads: %v", err)})
			return
		}

		var found bool
		for _, w := range workloadList.Items {
			if w.Name == req.WorkloadId {
				k8sWorkload = &w
				found = true
				break
			}
		}

		if !found {
			c.JSON(404, gin.H{"error": fmt.Sprintf("workload not found: %s", req.WorkloadId)})
			return
		}

		// Verify workload is running
		if k8sWorkload.Status.Phase != v1.WorkloadRunning {
			c.JSON(400, gin.H{"error": fmt.Sprintf("workload is not running, current phase: %s", k8sWorkload.Status.Phase)})
			return
		}

		// For workloads, baseUrl must be provided by user
		if req.WorkloadBaseUrl == "" {
			c.JSON(400, gin.H{"error": "workloadBaseUrl is required when chatting with a workload"})
			return
		}
		baseUrl = req.WorkloadBaseUrl

		// For workloads, modelName must be provided by user
		if req.WorkloadModelName == "" {
			c.JSON(400, gin.H{"error": "workloadModelName is required when chatting with a workload"})
			return
		}
		modelName = req.WorkloadModelName

		// Optional API key from request
		apiKey = req.WorkloadApiKey
	}

	if baseUrl == "" {
		c.JSON(400, gin.H{"error": "service base URL not available"})
		return
	}

	if modelName == "" {
		c.JSON(400, gin.H{"error": "model name not specified"})
		return
	}

	// Call inference service with streaming support
	if req.Stream {
		h.streamChat(c, baseUrl, apiKey, modelName, req)
	} else {
		h.nonStreamChat(c, baseUrl, apiKey, modelName, req)
	}
}

// ListPlaygroundServices lists all available services for playground chat.
// This includes remote_api models and running inference workloads.
func (h *Handler) ListPlaygroundServices(c *gin.Context) {
	handle(c, h.listPlaygroundServices)
}

// listPlaygroundServices implements the playground services listing logic.
func (h *Handler) listPlaygroundServices(c *gin.Context) (interface{}, error) {
	// Parse query parameters
	query := &ListPlaygroundServicesQuery{}
	if err := c.ShouldBindQuery(query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}

	ctx := c.Request.Context()
	var items []PlaygroundServiceItem

	// 1. List remote_api models that are ready (not filtered by workspace)
	modelList := &v1.ModelList{}
	if err := h.k8sClient.List(ctx, modelList); err != nil {
		return nil, commonerrors.NewInternalError("failed to list models: " + err.Error())
	}

	for _, m := range modelList.Items {
		if m.Spec.Source.AccessMode == v1.AccessModeRemoteAPI && m.Status.Phase == v1.ModelPhaseReady {
			items = append(items, PlaygroundServiceItem{
				Type:        "remote_api",
				ID:          m.Name,
				DisplayName: m.Spec.DisplayName,
				ModelName:   m.GetModelName(),
				Phase:       string(m.Status.Phase),
			})
		}
	}

	// 2. List all running inference workloads (Deployment/StatefulSet types)
	// Note: source-model label is optional - used for filtering on Model Square page
	workloadList := &v1.WorkloadList{}
	if err := h.k8sClient.List(ctx, workloadList); err != nil {
		return nil, commonerrors.NewInternalError("failed to list workloads: " + err.Error())
	}

	for _, w := range workloadList.Items {
		// Only include workloads that are running
		if w.Status.Phase != v1.WorkloadRunning {
			continue
		}

		// Filter to only include inference-type workloads (Deployment/StatefulSet)
		// Exclude training jobs (PyTorchJob), CI/CD jobs (AutoscalingRunnerSet), etc.
		kind := w.Spec.Kind
		if kind != "" && kind != common.DeploymentKind && kind != common.StatefulSetKind {
			continue
		}

		// Filter by workspace if specified
		if query.Workspace != "" && w.Spec.Workspace != query.Workspace {
			continue
		}

		// Get source model ID from label (optional)
		sourceModelID := ""
		sourceModelName := ""
		if w.Labels != nil {
			sourceModelID = w.Labels[v1.SourceModelLabel]
			if sourceModelID != "" {
				// Try to get the source model's display name
				sourceModel := &v1.Model{}
				if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: sourceModelID}, sourceModel); err == nil {
					sourceModelName = sourceModel.Spec.DisplayName
				}
			}
		}

		items = append(items, PlaygroundServiceItem{
			Type:            "workload",
			ID:              w.Name,
			DisplayName:     w.Name, // Workload doesn't have DisplayName, use Name
			Phase:           string(w.Status.Phase),
			Workspace:       w.Namespace,
			SourceModelID:   sourceModelID,
			SourceModelName: sourceModelName,
		})
	}

	return &ListPlaygroundServicesResponse{
		Total: len(items),
		Items: items,
	}, nil
}

// GetChatURL gets the chat URL and configuration for a model or workload.
func (h *Handler) GetChatURL(c *gin.Context) {
	handle(c, h.getChatURL)
}

// getChatURL implements the chat URL retrieval logic.
func (h *Handler) getChatURL(c *gin.Context) (interface{}, error) {
	modelId := c.Param("id")
	if modelId == "" {
		return nil, commonerrors.NewBadRequest("model id is required")
	}

	ctx := c.Request.Context()

	// Get the model
	k8sModel := &v1.Model{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{Name: modelId}, k8sModel); err != nil {
		return nil, commonerrors.NewNotFound("model", modelId)
	}

	if k8sModel.Spec.Source.AccessMode != v1.AccessModeRemoteAPI {
		return nil, commonerrors.NewBadRequest("getChatURL is only available for remote_api models")
	}

	hasApiKey := k8sModel.Spec.Source.ApiKey != nil && k8sModel.Spec.Source.ApiKey.Name != ""

	return &ChatURLResponse{
		URL:       k8sModel.Spec.Source.URL,
		ModelName: k8sModel.GetModelName(),
		HasApiKey: hasApiKey,
	}, nil
}

// getTokenFromSecret retrieves token from a Kubernetes Secret (for HuggingFace tokens)
func (h *Handler) getTokenFromSecret(ctx context.Context, secretName string) string {
	secret := &corev1.Secret{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{
		Name:      secretName,
		Namespace: common.PrimusSafeNamespace,
	}, secret); err != nil {
		klog.ErrorS(err, "failed to get token secret", "secret", secretName)
		return ""
	}
	if key, exists := secret.Data["token"]; exists {
		return string(key)
	}
	return ""
}

// getApiKeyFromSecret retrieves API key from a Kubernetes Secret (for remote API access)
func (h *Handler) getApiKeyFromSecret(ctx context.Context, secretName string) string {
	secret := &corev1.Secret{}
	if err := h.k8sClient.Get(ctx, ctrlclient.ObjectKey{
		Name:      secretName,
		Namespace: common.PrimusSafeNamespace,
	}, secret); err != nil {
		klog.ErrorS(err, "failed to get apiKey secret", "secret", secretName)
		return ""
	}
	if key, exists := secret.Data["apiKey"]; exists {
		return string(key)
	}
	return ""
}

// SaveSession handles saving a chat session (create or update).
func (h *Handler) SaveSession(c *gin.Context) {
	handle(c, h.saveSession)
}

// ListPlaygroundSession handles listing playground sessions with filtering and pagination.
func (h *Handler) ListPlaygroundSession(c *gin.Context) {
	handle(c, h.listPlaygroundSession)
}

// GetPlaygroundSession retrieves detailed information about a specific playground session.
func (h *Handler) GetPlaygroundSession(c *gin.Context) {
	handle(c, h.getPlaygroundSession)
}

// DeletePlaygroundSession handles deletion of a single playground session.
func (h *Handler) DeletePlaygroundSession(c *gin.Context) {
	handle(c, h.deletePlaygroundSession)
}

// deletePlaygroundSession implements the session deletion logic.
func (h *Handler) deletePlaygroundSession(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("session management requires database")
	}

	sessionIdStr := c.Param("id")
	if sessionIdStr == "" {
		return nil, commonerrors.NewBadRequest("session id is required")
	}

	sessionId, err := strconv.ParseInt(sessionIdStr, 10, 64)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid session id")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Get session
	dbSession, err := h.dbClient.GetPlaygroundSession(c.Request.Context(), sessionId)
	if err != nil {
		return nil, err
	}

	// Check ownership
	if dbSession.UserId != userId {
		return nil, commonerrors.NewForbidden("not authorized to delete this session")
	}

	// Mark as deleted
	if err := h.dbClient.SetPlaygroundSessionDeleted(c.Request.Context(), sessionId); err != nil {
		return nil, err
	}

	klog.Infof("deleted playground session: id=%d, user: %s", sessionId, userId)
	return nil, nil
}

// streamChat handles streaming chat using SSE with OpenAI SDK.
func (h *Handler) streamChat(c *gin.Context, baseUrl string, apiKey string, modelName string, req *ChatRequest) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// Create OpenAI client config with custom HTTP client that skips TLS verification
	config := openai.DefaultConfig(apiKey)
	if baseUrl != "" {
		config.BaseURL = baseUrl + "/v1"
	}
	// Configure HTTP client to skip TLS certificate verification
	config.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 300 * time.Second,
	}
	client := openai.NewClientWithConfig(config)

	// Convert messages to OpenAI format
	messages := make([]openai.ChatCompletionMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: content,
		})
	}

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   true,
	}
	if req.Temperature > 0 {
		chatReq.Temperature = float32(req.Temperature)
	}
	if req.TopP > 0 {
		chatReq.TopP = float32(req.TopP)
	}
	if req.MaxTokens > 0 {
		chatReq.MaxCompletionTokens = req.MaxTokens
	}
	if req.FrequencyPenalty != 0 {
		chatReq.FrequencyPenalty = float32(req.FrequencyPenalty)
	}
	if req.PresencePenalty != 0 {
		chatReq.PresencePenalty = float32(req.PresencePenalty)
	}
	if req.N > 0 {
		chatReq.N = req.N
	}

	// Create stream
	ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
	defer cancel()

	stream, err := client.CreateChatCompletionStream(ctx, chatReq)
	if err != nil {
		c.SSEvent("error", fmt.Sprintf("failed to create stream: %v", err))
		return
	}
	defer stream.Close()

	// Get flusher for SSE
	flusher, _ := c.Writer.(interface{ Flush() })

	// Determine source ID for logging
	sourceID := req.ModelId
	if sourceID == "" {
		sourceID = req.WorkloadId
	}

	// Stream response
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			// Stream completed successfully
			c.SSEvent("message", "[DONE]")
			if flusher != nil {
				flusher.Flush()
			}
			break
		}
		if err != nil {
			klog.ErrorS(err, "error reading stream")
			c.SSEvent("error", fmt.Sprintf("stream error: %v", err))
			return
		}

		// Forward the response chunk in OpenAI SSE format
		if len(response.Choices) > 0 {
			c.SSEvent("message", response)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}

	klog.Infof("streaming chat completed for: %s", sourceID)
}

// nonStreamChat handles non-streaming chat with OpenAI SDK.
func (h *Handler) nonStreamChat(c *gin.Context, baseUrl string, apiKey string, modelName string, req *ChatRequest) {
	// Create OpenAI client config with custom HTTP client that skips TLS verification
	config := openai.DefaultConfig(apiKey)
	if baseUrl != "" {
		config.BaseURL = baseUrl + "/v1"
	}
	// Configure HTTP client to skip TLS certificate verification
	config.HTTPClient = &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		Timeout: 300 * time.Second,
	}
	client := openai.NewClientWithConfig(config)

	// Convert messages to OpenAI format
	messages := make([]openai.ChatCompletionMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		messages = append(messages, openai.ChatCompletionMessage{
			Role:    role,
			Content: content,
		})
	}

	// Build request
	chatReq := openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   false,
	}
	if req.Temperature > 0 {
		chatReq.Temperature = float32(req.Temperature)
	}
	if req.TopP > 0 {
		chatReq.TopP = float32(req.TopP)
	}
	if req.MaxTokens > 0 {
		chatReq.MaxCompletionTokens = req.MaxTokens
	}
	if req.FrequencyPenalty != 0 {
		chatReq.FrequencyPenalty = float32(req.FrequencyPenalty)
	}
	if req.PresencePenalty != 0 {
		chatReq.PresencePenalty = float32(req.PresencePenalty)
	}
	if req.N > 0 {
		chatReq.N = req.N
	}

	// Call API
	ctx, cancel := context.WithTimeout(c.Request.Context(), 300*time.Second)
	defer cancel()

	resp, err := client.CreateChatCompletion(ctx, chatReq)
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("failed to call inference service: %v", err)})
		return
	}

	// Determine source ID for logging
	sourceID := req.ModelId
	if sourceID == "" {
		sourceID = req.WorkloadId
	}

	// Return response in OpenAI format
	c.JSON(200, resp)
	klog.Infof("non-streaming chat completed for: %s", sourceID)
}

// saveSession implements the session save logic - saves or updates a session.
func (h *Handler) saveSession(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("session management requires database")
	}

	req := &SaveSessionRequest{}
	if err := c.ShouldBindJSON(req); err != nil {
		return nil, commonerrors.NewBadRequest(fmt.Sprintf("invalid request body: %v", err))
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Validate model_name is not empty
	if req.ModelName == "" {
		return nil, commonerrors.NewBadRequest("modelName is required")
	}

	// Marshal messages
	messagesJSON, err := json.Marshal(req.Messages)
	if err != nil {
		return nil, commonerrors.NewInternalError(fmt.Sprintf("failed to marshal messages: %v", err))
	}

	ctx := c.Request.Context()
	now := time.Now().UTC()

	if req.Id == 0 {
		// Create new session
		session := &dbclient.PlaygroundSession{
			UserId:       userId,
			ModelName:    req.ModelName,
			DisplayName:  req.DisplayName,
			SystemPrompt: req.SystemPrompt,
			Messages:     string(messagesJSON),
			CreationTime: toNullTime(now),
			UpdateTime:   toNullTime(now),
			IsDeleted:    false,
		}

		if err := h.dbClient.InsertPlaygroundSession(ctx, session); err != nil {
			return nil, err
		}

		klog.Infof("created playground session: id=%d, user: %s, model: %s", session.Id, userId, req.ModelName)
		return &SaveSessionResponse{Id: session.Id}, nil
	} else {
		// Update existing session
		existingSession, err := h.dbClient.GetPlaygroundSession(ctx, req.Id)
		if err != nil {
			return nil, err
		}

		// Verify ownership
		if existingSession.UserId != userId {
			return nil, commonerrors.NewForbidden("not authorized to update this session")
		}

		existingSession.ModelName = req.ModelName
		existingSession.DisplayName = req.DisplayName
		existingSession.SystemPrompt = req.SystemPrompt
		existingSession.Messages = string(messagesJSON)
		existingSession.UpdateTime = toNullTime(now)

		if err := h.dbClient.UpdatePlaygroundSession(ctx, existingSession); err != nil {
			return nil, err
		}

		klog.Infof("updated playground session: id=%d, user: %s, model: %s", req.Id, userId, req.ModelName)
		return &SaveSessionResponse{Id: req.Id}, nil
	}
}

// listPlaygroundSession implements the session listing logic.
func (h *Handler) listPlaygroundSession(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("session management requires database")
	}

	query, err := parseListPlaygroundSessionQuery(c)
	if err != nil {
		return nil, err
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Build database query
	dbTags := dbclient.GetPlaygroundSessionFieldTags()
	dbSql := sqrl.And{
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "IsDeleted"): false},
		sqrl.Eq{dbclient.GetFieldTag(dbTags, "UserId"): userId},
	}

	if query.ModelName != "" {
		dbSql = append(dbSql, sqrl.Eq{dbclient.GetFieldTag(dbTags, "ModelName"): query.ModelName})
	}

	orderBy := []string{fmt.Sprintf("%s DESC", dbclient.GetFieldTag(dbTags, "UpdateTime"))}

	ctx := c.Request.Context()
	sessions, err := h.dbClient.SelectPlaygroundSessions(ctx, dbSql, orderBy, query.Limit, query.Offset)
	if err != nil {
		return nil, err
	}

	count, err := h.dbClient.CountPlaygroundSessions(ctx, dbSql)
	if err != nil {
		return nil, err
	}

	items := make([]PlaygroundSessionInfo, 0, len(sessions))
	for _, session := range sessions {
		items = append(items, cvtDBSessionToInfo(session))
	}

	return &ListPlaygroundSessionResponse{
		Total: count,
		Items: items,
	}, nil
}

// getPlaygroundSession implements the session retrieval logic.
func (h *Handler) getPlaygroundSession(c *gin.Context) (interface{}, error) {
	if h.dbClient == nil {
		return nil, commonerrors.NewInternalError("session management requires database")
	}

	sessionIdStr := c.Param("id")
	if sessionIdStr == "" {
		return nil, commonerrors.NewBadRequest("session id is required")
	}

	sessionId, err := strconv.ParseInt(sessionIdStr, 10, 64)
	if err != nil {
		return nil, commonerrors.NewBadRequest("invalid session id")
	}

	userId := c.GetString(common.UserId)
	if userId == "" {
		return nil, commonerrors.NewUnauthorized("user not authenticated")
	}

	// Get from database
	dbSession, err := h.dbClient.GetPlaygroundSession(c.Request.Context(), sessionId)
	if err != nil {
		return nil, err
	}

	// Check ownership
	if dbSession.UserId != userId {
		return nil, commonerrors.NewForbidden("not authorized to access this session")
	}

	return cvtDBSessionToDetail(dbSession), nil
}

// parseListPlaygroundSessionQuery parses query parameters for listing sessions.
func parseListPlaygroundSessionQuery(c *gin.Context) (*ListPlaygroundSessionQuery, error) {
	query := &ListPlaygroundSessionQuery{}
	if err := c.ShouldBindWith(&query, binding.Query); err != nil {
		return nil, commonerrors.NewBadRequest("invalid query: " + err.Error())
	}

	// Set default values
	if query.Limit <= 0 {
		query.Limit = 100
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	return query, nil
}

// cvtDBSessionToInfo converts database session to SessionInfo.
func cvtDBSessionToInfo(dbSession *dbclient.PlaygroundSession) PlaygroundSessionInfo {
	return PlaygroundSessionInfo{
		Id:           dbSession.Id,
		UserId:       dbSession.UserId,
		ModelName:    dbSession.ModelName,
		DisplayName:  dbSession.DisplayName,
		SystemPrompt: dbSession.SystemPrompt,
		Messages:     dbSession.Messages,
		CreationTime: formatTime(dbSession.CreationTime),
		UpdateTime:   formatTime(dbSession.UpdateTime),
	}
}

// cvtDBSessionToDetail converts database session to SessionDetail.
func cvtDBSessionToDetail(dbSession *dbclient.PlaygroundSession) *PlaygroundSessionDetail {
	return &PlaygroundSessionDetail{
		Id:           dbSession.Id,
		UserId:       dbSession.UserId,
		ModelName:    dbSession.ModelName,
		DisplayName:  dbSession.DisplayName,
		SystemPrompt: dbSession.SystemPrompt,
		Messages:     dbSession.Messages,
		CreationTime: formatTime(dbSession.CreationTime),
		UpdateTime:   formatTime(dbSession.UpdateTime),
	}
}

// toNullTime converts time.Time to pq.NullTime.
func toNullTime(t time.Time) pq.NullTime {
	return pq.NullTime{
		Valid: true,
		Time:  t,
	}
}

// formatTime formats pq.NullTime to RFC3339 string.
func formatTime(nt pq.NullTime) string {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return ""
}

// getString safely extracts string from sql.NullString.
func getString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// getTime safely extracts time from pq.NullTime.
func getTime(nt pq.NullTime) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return time.Time{}
}
