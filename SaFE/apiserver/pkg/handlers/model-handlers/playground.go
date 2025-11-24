/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
	openai "github.com/sashabaranov/go-openai"
	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// Chat handles direct chat with an inference model without saving session.
// Supports streaming via SSE (Server-Sent Events).
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

	// Get inference from database
	dbInference, err := h.dbClient.GetInference(c.Request.Context(), req.InferenceId)
	if err != nil {
		c.JSON(404, gin.H{"error": fmt.Sprintf("inference not found: %v", err)})
		return
	}

	// Check inference is running
	phase := getString(dbInference.Phase)
	if phase != "Running" {
		c.JSON(400, gin.H{"error": fmt.Sprintf("inference is not running, current phase: %s", phase)})
		return
	}

	// Parse instance to get base URL
	var instanceData map[string]interface{}
	if dbInference.Instance.Valid {
		if err := jsonutils.Unmarshal([]byte(dbInference.Instance.String), &instanceData); err != nil {
			c.JSON(500, gin.H{"error": "failed to parse instance data"})
			return
		}
	}

	baseUrl, ok := instanceData["baseUrl"].(string)
	if !ok || baseUrl == "" {
		c.JSON(400, gin.H{"error": "inference service base URL not available"})
		return
	}

	// Get API key from instance (optional)
	apiKey, _ := instanceData["apiKey"].(string)

	// Get model name: prioritize instance.model, fallback to inference.model_name
	modelName := dbInference.ModelName
	if instanceModel, ok := instanceData["model"].(string); ok && instanceModel != "" {
		modelName = instanceModel
	}

	if modelName == "" {
		c.JSON(400, gin.H{"error": "model name not specified in instance or inference"})
		return
	}

	// Call inference service with streaming support
	if req.Stream {
		h.streamChat(c, baseUrl, apiKey, modelName, req)
	} else {
		h.nonStreamChat(c, baseUrl, apiKey, modelName, req)
	}
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

	// Create OpenAI client config
	config := openai.DefaultConfig(apiKey)
	if baseUrl != "" {
		config.BaseURL = baseUrl + "/v1"
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

	klog.Infof("streaming chat completed for inference: %s", req.InferenceId)
}

// nonStreamChat handles non-streaming chat with OpenAI SDK.
func (h *Handler) nonStreamChat(c *gin.Context, baseUrl string, apiKey string, modelName string, req *ChatRequest) {
	// Create OpenAI client config
	config := openai.DefaultConfig(apiKey)
	if baseUrl != "" {
		config.BaseURL = baseUrl + "/v1"
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

	// Return response in OpenAI format
	c.JSON(200, resp)
	klog.Infof("non-streaming chat completed for inference: %s", req.InferenceId)
}

// saveSession implements the session save logic - saves or updates a session.
func (h *Handler) saveSession(c *gin.Context) (interface{}, error) {
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
