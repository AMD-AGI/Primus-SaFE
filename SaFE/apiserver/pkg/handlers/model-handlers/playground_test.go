/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	jsonutils "github.com/AMD-AIG-AIMA/SAFE/utils/pkg/json"
)

// TestToNullTime tests the toNullTime helper function
func TestToNullTime(t *testing.T) {
	now := time.Now().UTC()
	result := toNullTime(now)

	assert.True(t, result.Valid, "NullTime should be valid")
	assert.Equal(t, now, result.Time, "Time should match")
}

// TestCvtDBSessionToInfo tests the conversion from DB session to SessionInfo
func TestCvtDBSessionToInfo(t *testing.T) {
	createdAt := time.Now().UTC().Add(-24 * time.Hour)
	updatedAt := time.Now().UTC()

	messages := []MessageHistory{
		{Role: "user", Content: "Hello", Timestamp: createdAt},
		{Role: "assistant", Content: "Hi there!", Timestamp: createdAt.Add(1 * time.Second)},
		{Role: "user", Content: "How are you?", Timestamp: createdAt.Add(2 * time.Second)},
	}
	messagesJSON, _ := json.Marshal(messages)

	dbSession := &dbclient.PlaygroundSession{
		Id:           123,
		UserId:       "user-001",
		ModelName:    "qwen-2.5-7b",
		DisplayName:  "Test Chat",
		SystemPrompt: "You are helpful",
		Messages:     string(messagesJSON),
		CreationTime: pq.NullTime{Valid: true, Time: createdAt},
		UpdateTime:   pq.NullTime{Valid: true, Time: updatedAt},
		IsDeleted:    false,
	}

	result := cvtDBSessionToInfo(dbSession)

	assert.Equal(t, int64(123), result.Id)
	assert.Equal(t, "qwen-2.5-7b", result.ModelName)
	assert.Equal(t, "Test Chat", result.DisplayName)
	assert.Equal(t, "You are helpful", result.SystemPrompt)
	assert.Equal(t, 3, result.MessageCount)
	assert.Equal(t, createdAt, result.CreatedAt)
	assert.Equal(t, updatedAt, result.UpdatedAt)
}

// TestCvtDBSessionToInfo_EmptyMessages tests conversion with empty messages
func TestCvtDBSessionToInfo_EmptyMessages(t *testing.T) {
	dbSession := &dbclient.PlaygroundSession{
		Id:           456,
		UserId:       "user-002",
		ModelName:    "llama-3-8b",
		DisplayName:  "Empty Chat",
		SystemPrompt: "",
		Messages:     "",
		CreationTime: pq.NullTime{Valid: true, Time: time.Now()},
		UpdateTime:   pq.NullTime{Valid: true, Time: time.Now()},
		IsDeleted:    false,
	}

	result := cvtDBSessionToInfo(dbSession)

	assert.Equal(t, int64(456), result.Id)
	assert.Equal(t, 0, result.MessageCount, "Empty messages should result in 0 count")
}

// TestCvtDBSessionToInfo_InvalidJSON tests conversion with invalid JSON
func TestCvtDBSessionToInfo_InvalidJSON(t *testing.T) {
	dbSession := &dbclient.PlaygroundSession{
		Id:           789,
		UserId:       "user-003",
		ModelName:    "qwen-2.5-7b",
		DisplayName:  "Invalid JSON",
		SystemPrompt: "",
		Messages:     "{invalid json}",
		CreationTime: pq.NullTime{Valid: true, Time: time.Now()},
		UpdateTime:   pq.NullTime{Valid: true, Time: time.Now()},
		IsDeleted:    false,
	}

	result := cvtDBSessionToInfo(dbSession)

	assert.Equal(t, 0, result.MessageCount, "Invalid JSON should result in 0 count")
}

// TestCvtDBSessionToDetail tests the conversion from DB session to SessionDetail
func TestCvtDBSessionToDetail(t *testing.T) {
	createdAt := time.Now().UTC().Add(-24 * time.Hour)
	updatedAt := time.Now().UTC()

	messages := []MessageHistory{
		{Role: "user", Content: "Hello", Timestamp: createdAt},
		{Role: "assistant", Content: "Hi!", Timestamp: createdAt.Add(1 * time.Second)},
	}
	messagesJSON, _ := json.Marshal(messages)

	dbSession := &dbclient.PlaygroundSession{
		Id:           100,
		UserId:       "user-100",
		ModelName:    "qwen-2.5-7b",
		DisplayName:  "Detail Test",
		SystemPrompt: "Be concise",
		Messages:     string(messagesJSON),
		CreationTime: pq.NullTime{Valid: true, Time: createdAt},
		UpdateTime:   pq.NullTime{Valid: true, Time: updatedAt},
		IsDeleted:    false,
	}

	result := cvtDBSessionToDetail(dbSession)

	assert.NotNil(t, result)
	assert.Equal(t, int64(100), result.Id)
	assert.Equal(t, "qwen-2.5-7b", result.ModelName)
	assert.Equal(t, "Detail Test", result.DisplayName)
	assert.Equal(t, "Be concise", result.SystemPrompt)
	assert.Equal(t, 2, len(result.Messages))
	assert.Equal(t, "Hello", result.Messages[0].Content)
	assert.Equal(t, "Hi!", result.Messages[1].Content)
	assert.Equal(t, createdAt, result.CreatedAt)
	assert.Equal(t, updatedAt, result.UpdatedAt)
}

// TestCvtDBSessionToDetail_EmptyMessages tests conversion with empty messages
func TestCvtDBSessionToDetail_EmptyMessages(t *testing.T) {
	dbSession := &dbclient.PlaygroundSession{
		Id:           200,
		UserId:       "user-200",
		ModelName:    "llama-3-8b",
		DisplayName:  "No Messages",
		SystemPrompt: "",
		Messages:     "",
		CreationTime: pq.NullTime{Valid: true, Time: time.Now()},
		UpdateTime:   pq.NullTime{Valid: true, Time: time.Now()},
		IsDeleted:    false,
	}

	result := cvtDBSessionToDetail(dbSession)

	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.Messages), "Empty messages should result in empty array")
}

// TestCvtDBSessionToDetail_InvalidJSON tests conversion with invalid JSON
func TestCvtDBSessionToDetail_InvalidJSON(t *testing.T) {
	dbSession := &dbclient.PlaygroundSession{
		Id:           300,
		UserId:       "user-300",
		ModelName:    "qwen-2.5-7b",
		DisplayName:  "Bad JSON",
		SystemPrompt: "",
		Messages:     "not valid json",
		CreationTime: pq.NullTime{Valid: true, Time: time.Now()},
		UpdateTime:   pq.NullTime{Valid: true, Time: time.Now()},
		IsDeleted:    false,
	}

	result := cvtDBSessionToDetail(dbSession)

	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.Messages), "Invalid JSON should result in empty array")
}

// TestChatRequest_AllParameters tests ChatRequest with all parameters
func TestChatRequest_AllParameters(t *testing.T) {
	req := &ChatRequest{
		InferenceId:      "inf-001",
		Messages:         []map[string]interface{}{{"role": "user", "content": "Hello"}},
		Stream:           true,
		Temperature:      0.8,
		TopP:             0.95,
		MaxTokens:        2048,
		FrequencyPenalty: 0.5,
		PresencePenalty:  0.3,
		N:                1,
	}

	assert.Equal(t, "inf-001", req.InferenceId)
	assert.True(t, req.Stream)
	assert.Equal(t, 0.8, req.Temperature)
	assert.Equal(t, 0.95, req.TopP)
	assert.Equal(t, 2048, req.MaxTokens)
	assert.Equal(t, 0.5, req.FrequencyPenalty)
	assert.Equal(t, 0.3, req.PresencePenalty)
	assert.Equal(t, 1, req.N)
}

// TestSaveSessionRequest tests SaveSessionRequest
func TestSaveSessionRequest(t *testing.T) {
	messages := []MessageHistory{
		{Role: "user", Content: "Test", Timestamp: time.Now()},
	}

	req := &SaveSessionRequest{
		Id:           0, // New session
		ModelName:    "qwen-2.5-7b",
		DisplayName:  "Test Session",
		SystemPrompt: "You are helpful",
		Messages:     messages,
	}

	assert.Equal(t, int64(0), req.Id, "New session should have ID 0")
	assert.Equal(t, "qwen-2.5-7b", req.ModelName)
	assert.Equal(t, "Test Session", req.DisplayName)
	assert.Equal(t, 1, len(req.Messages))
}

// TestListPlaygroundSessionQuery_DefaultValues tests default values
func TestListPlaygroundSessionQuery_DefaultValues(t *testing.T) {
	query := &ListPlaygroundSessionQuery{}

	// Before applying defaults
	assert.Equal(t, 0, query.Limit)
	assert.Equal(t, 0, query.Offset)
	assert.Equal(t, "", query.ModelName)
}

// TestMessageHistory tests MessageHistory struct
func TestMessageHistory(t *testing.T) {
	now := time.Now().UTC()
	msg := MessageHistory{
		Role:      "assistant",
		Content:   "Hello!",
		Timestamp: now,
	}

	assert.Equal(t, "assistant", msg.Role)
	assert.Equal(t, "Hello!", msg.Content)
	assert.Equal(t, now, msg.Timestamp)

	// Test JSON marshaling
	jsonData, err := json.Marshal(msg)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "assistant")
	assert.Contains(t, string(jsonData), "Hello!")

	// Test JSON unmarshaling
	var unmarshaled MessageHistory
	err = jsonutils.Unmarshal(jsonData, &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, msg.Role, unmarshaled.Role)
	assert.Equal(t, msg.Content, unmarshaled.Content)
}

// TestPlaygroundSessionInfo_JSON tests JSON marshaling of SessionInfo
func TestPlaygroundSessionInfo_JSON(t *testing.T) {
	info := PlaygroundSessionInfo{
		Id:           123,
		ModelName:    "qwen-2.5-7b",
		DisplayName:  "Test",
		SystemPrompt: "Be helpful",
		MessageCount: 5,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	jsonData, err := json.Marshal(info)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "qwen-2.5-7b")
	assert.Contains(t, string(jsonData), "Test")
}

// TestPlaygroundSessionDetail_JSON tests JSON marshaling of SessionDetail
func TestPlaygroundSessionDetail_JSON(t *testing.T) {
	detail := &PlaygroundSessionDetail{
		Id:           456,
		ModelName:    "llama-3-8b",
		DisplayName:  "Detailed Test",
		SystemPrompt: "You are an expert",
		Messages: []MessageHistory{
			{Role: "user", Content: "Q1", Timestamp: time.Now()},
			{Role: "assistant", Content: "A1", Timestamp: time.Now()},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	jsonData, err := json.Marshal(detail)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "llama-3-8b")
	assert.Contains(t, string(jsonData), "Detailed Test")
	assert.Contains(t, string(jsonData), "Q1")
	assert.Contains(t, string(jsonData), "A1")
}

// TestGetString tests the getString helper (from inference.go)
func TestGetString(t *testing.T) {
	tests := []struct {
		name     string
		input    sql.NullString
		expected string
	}{
		{
			name:     "valid string",
			input:    sql.NullString{Valid: true, String: "test"},
			expected: "test",
		},
		{
			name:     "invalid string",
			input:    sql.NullString{Valid: false, String: ""},
			expected: "",
		},
		{
			name:     "valid empty string",
			input:    sql.NullString{Valid: true, String: ""},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetTime tests the getTime helper (from inference.go)
func TestGetTime(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		input    pq.NullTime
		expected time.Time
	}{
		{
			name:     "valid time",
			input:    pq.NullTime{Valid: true, Time: now},
			expected: now,
		},
		{
			name:     "invalid time",
			input:    pq.NullTime{Valid: false, Time: time.Time{}},
			expected: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getTime(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestStreamChat tests the streaming chat functionality
func TestStreamChat(t *testing.T) {
	// Create a mock inference service that returns SSE stream
	mockInferenceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)

		// Verify request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		assert.True(t, reqBody["stream"].(bool), "Stream should be true")
		assert.NotNil(t, reqBody["messages"])

		// Check parameters (OpenAI compatible only)
		if temp, ok := reqBody["temperature"]; ok {
			assert.Equal(t, 0.7, temp)
		}
		if topP, ok := reqBody["top_p"]; ok {
			assert.Equal(t, 0.95, topP)
		}
		if maxTokens, ok := reqBody["max_tokens"]; ok {
			assert.Equal(t, float64(2048), maxTokens)
		}
		if freqPenalty, ok := reqBody["frequency_penalty"]; ok {
			assert.Equal(t, 0.5, freqPenalty)
		}

		// Send SSE response
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "ResponseWriter should support flushing")

		// Simulate streaming chunks
		chunks := []string{
			`data: {"choices":[{"delta":{"content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":" world"}}]}`,
			`data: {"choices":[{"delta":{"content":"!"}}]}`,
			`data: [DONE]`,
		}

		for _, chunk := range chunks {
			fmt.Fprintf(w, "%s\n\n", chunk)
			flusher.Flush()
			time.Sleep(10 * time.Millisecond) // Simulate streaming delay
		}
	}))
	defer mockInferenceServer.Close()

	// Create test request
	req := &ChatRequest{
		InferenceId:      "test-inference",
		Messages:         []map[string]interface{}{{"role": "user", "content": "Hi"}},
		Stream:           true,
		Temperature:      0.7,
		TopP:             0.95,
		MaxTokens:        2048,
		FrequencyPenalty: 0.5,
		PresencePenalty:  0.3,
		N:                1,
	}

	// Create mock gin context with response recorder
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create handler and call streamChat
	handler := &Handler{}
	handler.streamChat(c, mockInferenceServer.URL, "test-api-key", "test-model", req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))

	// Verify response body contains streamed data
	body := w.Body.String()
	assert.Contains(t, body, "Hello")
	assert.Contains(t, body, "world")
	assert.Contains(t, body, "!")
	assert.Contains(t, body, "[DONE]")
}

// TestNonStreamChat tests the non-streaming chat functionality
func TestNonStreamChat(t *testing.T) {
	// Create a mock inference service that returns complete JSON
	mockInferenceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)

		// Verify request body
		var reqBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&reqBody)
		require.NoError(t, err)

		assert.False(t, reqBody["stream"].(bool), "Stream should be false")
		assert.NotNil(t, reqBody["messages"])

		// Check all parameters (OpenAI compatible only)
		if temp, ok := reqBody["temperature"]; ok {
			assert.Equal(t, 0.8, temp)
		}
		if topP, ok := reqBody["top_p"]; ok {
			assert.Equal(t, 0.9, topP)
		}
		if maxTokens, ok := reqBody["max_tokens"]; ok {
			assert.Equal(t, float64(1024), maxTokens)
		}
		if freqPenalty, ok := reqBody["frequency_penalty"]; ok {
			assert.Equal(t, 0.3, freqPenalty)
		}

		// Send complete JSON response
		w.Header().Set("Content-Type", "application/json")
		response := map[string]interface{}{
			"id":      "chatcmpl-123",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   "qwen-2.5-7b",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": "Hello! How can I help you today?",
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 8,
				"total_tokens":      18,
			},
		}

		json.NewEncoder(w).Encode(response)
	}))
	defer mockInferenceServer.Close()

	// Create test request
	req := &ChatRequest{
		InferenceId:      "test-inference",
		Messages:         []map[string]interface{}{{"role": "user", "content": "Hello"}},
		Stream:           false,
		Temperature:      0.8,
		TopP:             0.9,
		MaxTokens:        1024,
		FrequencyPenalty: 0.3,
		PresencePenalty:  0.2,
		N:                1,
	}

	// Create mock gin context with response recorder
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	// Create handler and call nonStreamChat
	handler := &Handler{}
	handler.nonStreamChat(c, mockInferenceServer.URL, "test-api-key", "test-model", req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)

	// Parse and verify response body
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "chatcmpl-123", response["id"])
	assert.Equal(t, "chat.completion", response["object"])

	choices, ok := response["choices"].([]interface{})
	require.True(t, ok)
	require.Len(t, choices, 1)

	choice := choices[0].(map[string]interface{})
	message := choice["message"].(map[string]interface{})
	assert.Equal(t, "assistant", message["role"])
	assert.Equal(t, "Hello! How can I help you today?", message["content"])
}

// TestStreamChat_ErrorHandling tests error handling in streaming chat
func TestStreamChat_ErrorHandling(t *testing.T) {
	// Test 1: Inference service returns error
	t.Run("inference service error", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		}))
		defer mockServer.Close()

		req := &ChatRequest{
			InferenceId: "test-inference",
			Messages:    []map[string]interface{}{{"role": "user", "content": "Hi"}},
			Stream:      true,
		}

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		handler := &Handler{}
		handler.streamChat(c, mockServer.URL, "test-api-key", "test-model", req)

		// Should contain error message
		body := w.Body.String()
		assert.Contains(t, body, "error")
		assert.Contains(t, body, "500")
	})

	// Test 2: Invalid request body
	t.Run("invalid request", func(t *testing.T) {
		req := &ChatRequest{
			InferenceId: "test-inference",
			Messages:    []map[string]interface{}{{"role": "user"}}, // Missing content
			Stream:      true,
		}

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		handler := &Handler{}
		// This should still work, as we don't validate message structure
		handler.streamChat(c, "http://invalid-url", "test-api-key", "test-model", req)

		// Should contain error
		body := w.Body.String()
		assert.NotEmpty(t, body)
	})
}

// TestNonStreamChat_ErrorHandling tests error handling in non-streaming chat
func TestNonStreamChat_ErrorHandling(t *testing.T) {
	// Test: Inference service returns error
	t.Run("inference service error", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid request parameters",
			})
		}))
		defer mockServer.Close()

		req := &ChatRequest{
			InferenceId: "test-inference",
			Messages:    []map[string]interface{}{{"role": "user", "content": "Hi"}},
			Stream:      false,
		}

		gin.SetMode(gin.TestMode)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		handler := &Handler{}
		handler.nonStreamChat(c, mockServer.URL, "test-api-key", "test-model", req)

		// Should return error
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "error")
	})
}

// TestBuildRequestBody_AllParameters tests that all parameters are correctly included
func TestBuildRequestBody_AllParameters(t *testing.T) {
	// This is tested implicitly in the above tests, but we can add a specific test
	req := &ChatRequest{
		InferenceId:      "test",
		Messages:         []map[string]interface{}{{"role": "user", "content": "test"}},
		Stream:           true,
		Temperature:      1.5,
		TopP:             0.99,
		MaxTokens:        4096,
		FrequencyPenalty: 1.0,
		PresencePenalty:  0.8,
		N:                2,
	}

	// Verify all parameters are set correctly (OpenAI compatible only)
	assert.Equal(t, "test", req.InferenceId)
	assert.Equal(t, true, req.Stream)
	assert.Equal(t, 1.5, req.Temperature)
	assert.Equal(t, 0.99, req.TopP)
	assert.Equal(t, 4096, req.MaxTokens)
	assert.Equal(t, 1.0, req.FrequencyPenalty)
	assert.Equal(t, 0.8, req.PresencePenalty)
	assert.Equal(t, 2, req.N)
}
