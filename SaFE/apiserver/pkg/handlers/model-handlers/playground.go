/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	sqrl "github.com/Masterminds/squirrel"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/lib/pq"
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

	// Call inference service with streaming support
	if req.Stream {
		h.streamChat(c, baseUrl, req)
	} else {
		h.nonStreamChat(c, baseUrl, req)
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

// streamChat handles streaming chat using SSE.
func (h *Handler) streamChat(c *gin.Context, baseUrl string, req *ChatRequest) {
	// Set SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no") // Disable nginx buffering

	// Build request body
	requestBody := map[string]interface{}{
		"messages": req.Messages,
		"stream":   true,
	}
	if req.Temperature > 0 {
		requestBody["temperature"] = req.Temperature
	}
	if req.TopK > 0 {
		requestBody["top_k"] = req.TopK
	}
	if req.TopP > 0 {
		requestBody["top_p"] = req.TopP
	}
	if req.MaxTokens > 0 {
		requestBody["max_tokens"] = req.MaxTokens
	}
	if req.FrequencyPenalty != 0 {
		requestBody["frequency_penalty"] = req.FrequencyPenalty
	}
	if req.EnableThinking {
		requestBody["enable_thinking"] = req.EnableThinking
		if req.ThinkingBudget > 0 {
			requestBody["thinking_budget"] = req.ThinkingBudget
		}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		c.SSEvent("error", fmt.Sprintf("failed to marshal request: %v", err))
		return
	}

	// Call inference service
	url := fmt.Sprintf("%s/v1/chat/completions", baseUrl)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.SSEvent("error", fmt.Sprintf("failed to create request: %v", err))
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 300 * time.Second} // Longer timeout for streaming
	resp, err := client.Do(httpReq)
	if err != nil {
		c.SSEvent("error", fmt.Sprintf("failed to call inference service: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.SSEvent("error", fmt.Sprintf("inference service error (status %d): %s", resp.StatusCode, string(bodyBytes)))
		return
	}

	// Stream response chunks
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.SSEvent("error", "streaming not supported")
		return
	}

	// Read and forward SSE events
	reader := io.Reader(resp.Body)
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			// Write chunk directly to response
			c.Writer.Write(buf[:n])
			flusher.Flush()
		}
		if err != nil {
			if err != io.EOF {
				klog.ErrorS(err, "error reading stream")
			}
			break
		}
	}

	klog.Infof("streaming chat completed for inference: %s", req.InferenceId)
}

// nonStreamChat handles non-streaming chat.
func (h *Handler) nonStreamChat(c *gin.Context, baseUrl string, req *ChatRequest) {
	// Build request body
	requestBody := map[string]interface{}{
		"messages": req.Messages,
		"stream":   false,
	}
	if req.Temperature > 0 {
		requestBody["temperature"] = req.Temperature
	}
	if req.TopK > 0 {
		requestBody["top_k"] = req.TopK
	}
	if req.TopP > 0 {
		requestBody["top_p"] = req.TopP
	}
	if req.MaxTokens > 0 {
		requestBody["max_tokens"] = req.MaxTokens
	}
	if req.FrequencyPenalty != 0 {
		requestBody["frequency_penalty"] = req.FrequencyPenalty
	}
	if req.EnableThinking {
		requestBody["enable_thinking"] = req.EnableThinking
		if req.ThinkingBudget > 0 {
			requestBody["thinking_budget"] = req.ThinkingBudget
		}
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("failed to marshal request: %v", err)})
		return
	}

	// Call inference service
	url := fmt.Sprintf("%s/v1/chat/completions", baseUrl)
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("failed to create request: %v", err)})
		return
	}

	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("failed to call inference service: %v", err)})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		c.JSON(500, gin.H{"error": fmt.Sprintf("inference service error (status %d): %s", resp.StatusCode, string(bodyBytes))})
		return
	}

	// Parse and return response
	var apiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		c.JSON(500, gin.H{"error": fmt.Sprintf("failed to parse response: %v", err)})
		return
	}

	c.JSON(200, apiResp)
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
	return gin.H{"message": "session deleted successfully"}, nil
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
	var messages []MessageHistory
	messageCount := 0
	if dbSession.Messages != "" {
		if err := jsonutils.Unmarshal([]byte(dbSession.Messages), &messages); err == nil {
			messageCount = len(messages)
		}
	}

	return PlaygroundSessionInfo{
		Id:           dbSession.Id,
		ModelName:    dbSession.ModelName,
		DisplayName:  dbSession.DisplayName,
		SystemPrompt: dbSession.SystemPrompt,
		MessageCount: messageCount,
		CreatedAt:    getTime(dbSession.CreationTime),
		UpdatedAt:    getTime(dbSession.UpdateTime),
	}
}

// cvtDBSessionToDetail converts database session to SessionDetail.
func cvtDBSessionToDetail(dbSession *dbclient.PlaygroundSession) *PlaygroundSessionDetail {
	var messages []MessageHistory
	if dbSession.Messages != "" {
		if err := jsonutils.Unmarshal([]byte(dbSession.Messages), &messages); err != nil {
			klog.ErrorS(err, "failed to unmarshal messages", "id", dbSession.Id)
			messages = []MessageHistory{}
		}
	}

	return &PlaygroundSessionDetail{
		Id:           dbSession.Id,
		ModelName:    dbSession.ModelName,
		DisplayName:  dbSession.DisplayName,
		SystemPrompt: dbSession.SystemPrompt,
		Messages:     messages,
		CreatedAt:    getTime(dbSession.CreationTime),
		UpdatedAt:    getTime(dbSession.UpdateTime),
	}
}

// toNullTime converts time.Time to pq.NullTime.
func toNullTime(t time.Time) pq.NullTime {
	return pq.NullTime{
		Valid: true,
		Time:  t,
	}
}
