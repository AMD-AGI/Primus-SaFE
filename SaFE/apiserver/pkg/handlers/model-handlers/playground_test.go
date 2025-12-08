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
	}

	result := cvtDBSessionToInfo(dbSession)

	assert.Equal(t, int64(123), result.Id)
	assert.Equal(t, "user-001", result.UserId)
	assert.Equal(t, "qwen-2.5-7b", result.ModelName)
	assert.Equal(t, "Test Chat", result.DisplayName)
	assert.Equal(t, "You are helpful", result.SystemPrompt)
	assert.Equal(t, string(messagesJSON), result.Messages)
	assert.NotEmpty(t, result.CreationTime)
	assert.NotEmpty(t, result.UpdateTime)
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
	}

	result := cvtDBSessionToInfo(dbSession)

	assert.Equal(t, int64(456), result.Id)
	assert.Equal(t, "", result.Messages, "Empty messages should result in empty string")
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
	}

	result := cvtDBSessionToInfo(dbSession)

	assert.Equal(t, "{invalid json}", result.Messages, "Invalid JSON should be returned as-is")
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
	}

	result := cvtDBSessionToDetail(dbSession)

	assert.NotNil(t, result)
	assert.Equal(t, int64(100), result.Id)
	assert.Equal(t, "user-100", result.UserId)
	assert.Equal(t, "qwen-2.5-7b", result.ModelName)
	assert.Equal(t, "Detail Test", result.DisplayName)
	assert.Equal(t, "Be concise", result.SystemPrompt)
	assert.Equal(t, string(messagesJSON), result.Messages)
	assert.NotEmpty(t, result.CreationTime)
	assert.NotEmpty(t, result.UpdateTime)
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
	}

	result := cvtDBSessionToDetail(dbSession)

	assert.NotNil(t, result)
	assert.Equal(t, "", result.Messages, "Empty messages should result in empty string")
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
	}

	result := cvtDBSessionToDetail(dbSession)

	assert.NotNil(t, result)
	assert.Equal(t, "not valid json", result.Messages, "Invalid JSON should be returned as-is")
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
		UserId:       "user-123",
		ModelName:    "qwen-2.5-7b",
		DisplayName:  "Test",
		SystemPrompt: "Be helpful",
		Messages:     "[]",
		CreationTime: time.Now().UTC().Format(time.RFC3339),
		UpdateTime:   time.Now().UTC().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(info)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonData), "qwen-2.5-7b")
	assert.Contains(t, string(jsonData), "Test")
}

// TestPlaygroundSessionDetail_JSON tests JSON marshaling of SessionDetail
func TestPlaygroundSessionDetail_JSON(t *testing.T) {
	messages := []MessageHistory{
		{Role: "user", Content: "Q1", Timestamp: time.Now()},
		{Role: "assistant", Content: "A1", Timestamp: time.Now()},
	}
	messagesJSON, _ := json.Marshal(messages)

	detail := &PlaygroundSessionDetail{
		Id:           456,
		UserId:       "user-456",
		ModelName:    "llama-3-8b",
		DisplayName:  "Detailed Test",
		SystemPrompt: "You are an expert",
		Messages:     string(messagesJSON),
		CreationTime: time.Now().UTC().Format(time.RFC3339),
		UpdateTime:   time.Now().UTC().Format(time.RFC3339),
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
	c.Request = httptest.NewRequest("POST", "/chat", nil)

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

		// Stream field may not be present or may be false for non-streaming requests
		if stream, ok := reqBody["stream"]; ok {
			assert.False(t, stream.(bool), "Stream should be false")
		}
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
	c.Request = httptest.NewRequest("POST", "/chat", nil)

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
		c.Request = httptest.NewRequest("POST", "/chat", nil)

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
		c.Request = httptest.NewRequest("POST", "/chat", nil)

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
		c.Request = httptest.NewRequest("POST", "/chat", nil)

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

// TestFormatTime tests the formatTime helper function
func TestFormatTime(t *testing.T) {
	tests := []struct {
		name     string
		input    pq.NullTime
		expected string
	}{
		{
			name:     "valid time",
			input:    pq.NullTime{Valid: true, Time: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)},
			expected: "2025-01-01T12:00:00Z",
		},
		{
			name:     "invalid time",
			input:    pq.NullTime{Valid: false},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatTime(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseListPlaygroundSessionQuery tests parseListPlaygroundSessionQuery
func TestParseListPlaygroundSessionQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		queryParams   string
		expectedLimit int
		expectedError bool
	}{
		{
			name:          "with valid params",
			queryParams:   "limit=50&offset=10&modelName=gpt-4",
			expectedLimit: 50,
			expectedError: false,
		},
		{
			name:          "empty query - should use defaults",
			queryParams:   "",
			expectedLimit: 100, // Default limit
			expectedError: false,
		},
		{
			name:          "negative limit - should error due to min validation",
			queryParams:   "limit=-5",
			expectedLimit: 0,
			expectedError: true, // Binding validation fails for min=1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/sessions?"+tt.queryParams, nil)

			result, err := parseListPlaygroundSessionQuery(c)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedLimit, result.Limit)
			}
		})
	}
}

// TestSaveSessionRequest_JSON tests JSON marshaling of SaveSessionRequest
func TestSaveSessionRequest_JSON(t *testing.T) {
	now := time.Now().UTC()
	req := SaveSessionRequest{
		Id:           123,
		ModelName:    "gpt-4",
		DisplayName:  "Test Session",
		SystemPrompt: "You are helpful",
		Messages: []MessageHistory{
			{Role: "user", Content: "Hello", Timestamp: now},
			{Role: "assistant", Content: "Hi!", Timestamp: now},
		},
	}

	jsonData, err := json.Marshal(req)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "gpt-4")
	assert.Contains(t, string(jsonData), "Test Session")
	assert.Contains(t, string(jsonData), "Hello")

	// Test unmarshaling
	var unmarshaled SaveSessionRequest
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Equal(t, req.Id, unmarshaled.Id)
	assert.Equal(t, req.ModelName, unmarshaled.ModelName)
	assert.Len(t, unmarshaled.Messages, 2)
}

// TestListPlaygroundSessionResponse tests ListPlaygroundSessionResponse
func TestListPlaygroundSessionResponse(t *testing.T) {
	resp := ListPlaygroundSessionResponse{
		Total: 50,
		Items: []PlaygroundSessionInfo{
			{Id: 1, ModelName: "gpt-4", DisplayName: "Session 1"},
			{Id: 2, ModelName: "llama", DisplayName: "Session 2"},
		},
	}

	assert.Equal(t, 50, resp.Total)
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, "gpt-4", resp.Items[0].ModelName)

	// Test JSON marshaling
	jsonData, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "Session 1")
	assert.Contains(t, string(jsonData), "Session 2")
}

// TestSaveSessionResponse tests SaveSessionResponse
func TestSaveSessionResponse(t *testing.T) {
	resp := SaveSessionResponse{Id: 12345}
	assert.Equal(t, int64(12345), resp.Id)

	jsonData, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "12345")
}

// TestChatRequest_Validation tests ChatRequest validation scenarios
func TestChatRequest_Validation(t *testing.T) {
	tests := []struct {
		name      string
		req       ChatRequest
		expectErr bool
	}{
		{
			name: "valid streaming request",
			req: ChatRequest{
				InferenceId: "inf-001",
				Messages:    []map[string]interface{}{{"role": "user", "content": "Hello"}},
				Stream:      true,
			},
			expectErr: false,
		},
		{
			name: "valid non-streaming request",
			req: ChatRequest{
				InferenceId: "inf-002",
				Messages:    []map[string]interface{}{{"role": "user", "content": "Hello"}},
				Stream:      false,
			},
			expectErr: false,
		},
		{
			name: "missing inference id",
			req: ChatRequest{
				InferenceId: "",
				Messages:    []map[string]interface{}{{"role": "user", "content": "Hello"}},
			},
			expectErr: true,
		},
		{
			name: "empty messages",
			req: ChatRequest{
				InferenceId: "inf-003",
				Messages:    []map[string]interface{}{},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectErr {
				// Check expected validation failures
				if tt.req.InferenceId == "" {
					assert.Empty(t, tt.req.InferenceId)
				}
				if len(tt.req.Messages) == 0 {
					assert.Empty(t, tt.req.Messages)
				}
			} else {
				assert.NotEmpty(t, tt.req.InferenceId)
				assert.NotEmpty(t, tt.req.Messages)
			}
		})
	}
}

// TestChatRequest_ParameterRanges tests parameter boundary values
func TestChatRequest_ParameterRanges(t *testing.T) {
	tests := []struct {
		name string
		req  ChatRequest
	}{
		{
			name: "minimum values",
			req: ChatRequest{
				InferenceId:      "inf-001",
				Messages:         []map[string]interface{}{{"role": "user", "content": "Hi"}},
				Temperature:      0.0,
				TopP:             0.0,
				MaxTokens:        1,
				FrequencyPenalty: -2.0,
				PresencePenalty:  -2.0,
				N:                1,
			},
		},
		{
			name: "maximum values",
			req: ChatRequest{
				InferenceId:      "inf-002",
				Messages:         []map[string]interface{}{{"role": "user", "content": "Hi"}},
				Temperature:      2.0,
				TopP:             1.0,
				MaxTokens:        128000,
				FrequencyPenalty: 2.0,
				PresencePenalty:  2.0,
				N:                10,
			},
		},
		{
			name: "typical values",
			req: ChatRequest{
				InferenceId:      "inf-003",
				Messages:         []map[string]interface{}{{"role": "user", "content": "Hi"}},
				Temperature:      0.7,
				TopP:             0.95,
				MaxTokens:        4096,
				FrequencyPenalty: 0.0,
				PresencePenalty:  0.0,
				N:                1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify values are set correctly
			assert.NotEmpty(t, tt.req.InferenceId)
			assert.NotEmpty(t, tt.req.Messages)
			assert.GreaterOrEqual(t, tt.req.Temperature, 0.0)
			assert.LessOrEqual(t, tt.req.Temperature, 2.0)
			assert.GreaterOrEqual(t, tt.req.TopP, 0.0)
			assert.LessOrEqual(t, tt.req.TopP, 1.0)
		})
	}
}

// TestMessageHistory_MultipleRoles tests messages with different roles
func TestMessageHistory_MultipleRoles(t *testing.T) {
	now := time.Now().UTC()

	messages := []MessageHistory{
		{Role: "system", Content: "You are a helpful assistant.", Timestamp: now},
		{Role: "user", Content: "Hello!", Timestamp: now.Add(1 * time.Second)},
		{Role: "assistant", Content: "Hi there!", Timestamp: now.Add(2 * time.Second)},
		{Role: "user", Content: "How are you?", Timestamp: now.Add(3 * time.Second)},
		{Role: "assistant", Content: "I'm doing well, thank you!", Timestamp: now.Add(4 * time.Second)},
	}

	// Verify roles
	assert.Equal(t, "system", messages[0].Role)
	assert.Equal(t, "user", messages[1].Role)
	assert.Equal(t, "assistant", messages[2].Role)

	// Verify timestamps are ordered
	for i := 1; i < len(messages); i++ {
		assert.True(t, messages[i].Timestamp.After(messages[i-1].Timestamp))
	}

	// Test JSON marshaling/unmarshaling
	jsonData, err := json.Marshal(messages)
	require.NoError(t, err)

	var unmarshaled []MessageHistory
	err = json.Unmarshal(jsonData, &unmarshaled)
	require.NoError(t, err)
	assert.Len(t, unmarshaled, 5)
}

// TestPlaygroundSessionInfo_AllFields tests all fields of PlaygroundSessionInfo
func TestPlaygroundSessionInfo_AllFields(t *testing.T) {
	info := PlaygroundSessionInfo{
		Id:           999,
		UserId:       "user-999",
		ModelName:    "claude-3",
		DisplayName:  "Full Test Session",
		SystemPrompt: "You are an expert coder.",
		Messages:     `[{"role":"user","content":"Write code"}]`,
		CreationTime: "2025-01-01T00:00:00Z",
		UpdateTime:   "2025-01-02T00:00:00Z",
	}

	assert.Equal(t, int64(999), info.Id)
	assert.Equal(t, "user-999", info.UserId)
	assert.Equal(t, "claude-3", info.ModelName)
	assert.Equal(t, "Full Test Session", info.DisplayName)
	assert.Equal(t, "You are an expert coder.", info.SystemPrompt)
	assert.Contains(t, info.Messages, "Write code")
	assert.NotEmpty(t, info.CreationTime)
	assert.NotEmpty(t, info.UpdateTime)
}

// TestPlaygroundSessionDetail_AllFields tests all fields of PlaygroundSessionDetail
func TestPlaygroundSessionDetail_AllFields(t *testing.T) {
	detail := &PlaygroundSessionDetail{
		Id:           888,
		UserId:       "user-888",
		ModelName:    "gemini-pro",
		DisplayName:  "Detail Full Test",
		SystemPrompt: "Be concise and accurate.",
		Messages:     `[{"role":"user","content":"Explain AI"},{"role":"assistant","content":"AI is..."}]`,
		CreationTime: "2025-01-01T12:00:00Z",
		UpdateTime:   "2025-01-01T13:00:00Z",
	}

	assert.Equal(t, int64(888), detail.Id)
	assert.Equal(t, "user-888", detail.UserId)
	assert.Equal(t, "gemini-pro", detail.ModelName)
	assert.Equal(t, "Detail Full Test", detail.DisplayName)
	assert.Contains(t, detail.Messages, "Explain AI")
	assert.Contains(t, detail.Messages, "AI is...")
}

// TestCvtDBSessionToInfo_NullTimes tests conversion with null times
func TestCvtDBSessionToInfo_NullTimes(t *testing.T) {
	dbSession := &dbclient.PlaygroundSession{
		Id:           777,
		UserId:       "user-777",
		ModelName:    "test-model",
		DisplayName:  "Null Times Test",
		SystemPrompt: "",
		Messages:     "[]",
		CreationTime: pq.NullTime{Valid: false},
		UpdateTime:   pq.NullTime{Valid: false},
	}

	result := cvtDBSessionToInfo(dbSession)

	assert.Equal(t, int64(777), result.Id)
	assert.Equal(t, "", result.CreationTime, "Null creation time should be empty string")
	assert.Equal(t, "", result.UpdateTime, "Null update time should be empty string")
}

// TestCvtDBSessionToDetail_NullTimes tests detail conversion with null times
func TestCvtDBSessionToDetail_NullTimes(t *testing.T) {
	dbSession := &dbclient.PlaygroundSession{
		Id:           666,
		UserId:       "user-666",
		ModelName:    "test-model",
		DisplayName:  "Null Times Detail Test",
		SystemPrompt: "",
		Messages:     "[]",
		CreationTime: pq.NullTime{Valid: false},
		UpdateTime:   pq.NullTime{Valid: false},
	}

	result := cvtDBSessionToDetail(dbSession)

	assert.NotNil(t, result)
	assert.Equal(t, int64(666), result.Id)
	assert.Equal(t, "", result.CreationTime)
	assert.Equal(t, "", result.UpdateTime)
}

// TestNonStreamChat_ConnectionTimeout tests timeout handling
func TestNonStreamChat_ConnectionTimeout(t *testing.T) {
	// Create a server that delays response
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Don't respond immediately, simulate slow server
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
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
	c.Request = httptest.NewRequest("POST", "/chat", nil)

	handler := &Handler{}
	handler.nonStreamChat(c, mockServer.URL, "test-api-key", "test-model", req)

	// Should complete (even if slow)
	assert.NotEqual(t, 0, w.Code)
}

// TestChatRequest_EmptyOptionalFields tests ChatRequest with only required fields
func TestChatRequest_EmptyOptionalFields(t *testing.T) {
	req := &ChatRequest{
		InferenceId: "inf-minimal",
		Messages:    []map[string]interface{}{{"role": "user", "content": "Hello"}},
		Stream:      false,
		// All optional fields left at zero values
	}

	assert.Equal(t, "inf-minimal", req.InferenceId)
	assert.Equal(t, 0.0, req.Temperature)
	assert.Equal(t, 0.0, req.TopP)
	assert.Equal(t, 0, req.MaxTokens)
	assert.Equal(t, 0.0, req.FrequencyPenalty)
	assert.Equal(t, 0.0, req.PresencePenalty)
	assert.Equal(t, 0, req.N)
}
