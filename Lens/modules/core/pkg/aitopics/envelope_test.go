// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package aitopics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseStatus_Constants(t *testing.T) {
	assert.Equal(t, ResponseStatus("success"), StatusSuccess)
	assert.Equal(t, ResponseStatus("error"), StatusError)
	assert.Equal(t, ResponseStatus("partial"), StatusPartial)
}

func TestErrorCodes(t *testing.T) {
	assert.Equal(t, 0, CodeSuccess)
	assert.Equal(t, 1001, CodeInvalidRequest)
	assert.Equal(t, 1002, CodeTopicNotSupported)
	assert.Equal(t, 1003, CodePayloadInvalid)
	assert.Equal(t, 1004, CodeUnauthorized)
	assert.Equal(t, 2001, CodeInternalError)
	assert.Equal(t, 2002, CodeLLMError)
	assert.Equal(t, 2003, CodeToolCallFailed)
	assert.Equal(t, 2004, CodeTimeout)
	assert.Equal(t, 2005, CodeAgentUnavailable)
}

func TestNewRequest(t *testing.T) {
	ctx := RequestContext{
		ClusterID:    "cluster-1",
		TenantID:     "tenant-1",
		UserID:       "user-1",
		TraceID:      "trace-1",
		ToolEndpoint: "http://localhost:8080/tools",
		Locale:       "en-US",
	}

	payload := map[string]interface{}{
		"key": "value",
	}

	req, err := NewRequest(TopicAlertAdvisorAggregateWorkloads, ctx, payload)
	require.NoError(t, err)
	assert.NotEmpty(t, req.RequestID)
	assert.Equal(t, TopicAlertAdvisorAggregateWorkloads, req.Topic)
	assert.Equal(t, CurrentVersion, req.Version)
	assert.False(t, req.Timestamp.IsZero())
	assert.Equal(t, "cluster-1", req.Context.ClusterID)
	assert.NotEmpty(t, req.Payload)
}

func TestNewRequest_InvalidPayload(t *testing.T) {
	// Channel cannot be serialized to JSON
	invalidPayload := make(chan int)

	_, err := NewRequest(TopicAlertAdvisorAggregateWorkloads, RequestContext{}, invalidPayload)
	assert.Error(t, err)
}

func TestNewSuccessResponse(t *testing.T) {
	payload := map[string]interface{}{
		"result": "success",
	}

	resp, err := NewSuccessResponse("req-123", payload)
	require.NoError(t, err)
	assert.Equal(t, "req-123", resp.RequestID)
	assert.Equal(t, StatusSuccess, resp.Status)
	assert.Equal(t, CodeSuccess, resp.Code)
	assert.Equal(t, "success", resp.Message)
	assert.False(t, resp.Timestamp.IsZero())
	assert.NotEmpty(t, resp.Payload)
}

func TestNewSuccessResponse_InvalidPayload(t *testing.T) {
	invalidPayload := make(chan int)

	_, err := NewSuccessResponse("req-123", invalidPayload)
	assert.Error(t, err)
}

func TestNewErrorResponse(t *testing.T) {
	resp := NewErrorResponse("req-123", CodeInternalError, "something went wrong")

	assert.Equal(t, "req-123", resp.RequestID)
	assert.Equal(t, StatusError, resp.Status)
	assert.Equal(t, CodeInternalError, resp.Code)
	assert.Equal(t, "something went wrong", resp.Message)
	assert.False(t, resp.Timestamp.IsZero())
	assert.Nil(t, resp.Payload)
}

func TestRequest_UnmarshalPayload(t *testing.T) {
	type TestPayload struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	originalPayload := TestPayload{Name: "test", Value: 42}
	payloadBytes, _ := json.Marshal(originalPayload)

	req := &Request{
		Payload: payloadBytes,
	}

	var result TestPayload
	err := req.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Value)
}

func TestRequest_UnmarshalPayload_Invalid(t *testing.T) {
	req := &Request{
		Payload: json.RawMessage("invalid json"),
	}

	var result map[string]interface{}
	err := req.UnmarshalPayload(&result)
	assert.Error(t, err)
}

func TestResponse_UnmarshalPayload(t *testing.T) {
	type TestPayload struct {
		Result string `json:"result"`
	}

	originalPayload := TestPayload{Result: "success"}
	payloadBytes, _ := json.Marshal(originalPayload)

	resp := &Response{
		Payload: payloadBytes,
	}

	var result TestPayload
	err := resp.UnmarshalPayload(&result)
	require.NoError(t, err)
	assert.Equal(t, "success", result.Result)
}

func TestResponse_UnmarshalPayload_Invalid(t *testing.T) {
	resp := &Response{
		Payload: json.RawMessage("invalid json"),
	}

	var result map[string]interface{}
	err := resp.UnmarshalPayload(&result)
	assert.Error(t, err)
}

func TestResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		name   string
		status ResponseStatus
		code   int
		want   bool
	}{
		{"success with code 0", StatusSuccess, CodeSuccess, true},
		{"success with error code", StatusSuccess, CodeInternalError, false},
		{"error status", StatusError, CodeSuccess, false},
		{"error with error code", StatusError, CodeInternalError, false},
		{"partial status", StatusPartial, CodeSuccess, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &Response{
				Status: tt.status,
				Code:   tt.code,
			}
			assert.Equal(t, tt.want, resp.IsSuccess())
		})
	}
}

func TestGenerateRequestID(t *testing.T) {
	id1 := generateRequestID()
	id2 := generateRequestID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Should be unique
	assert.Len(t, id1, 36)       // UUID format
}

func TestRequestContext(t *testing.T) {
	ctx := RequestContext{
		ClusterID:    "cluster-123",
		TenantID:     "tenant-456",
		UserID:       "user-789",
		TraceID:      "trace-abc",
		ToolEndpoint: "http://localhost:9090/tools",
		Locale:       "zh-CN",
	}

	assert.Equal(t, "cluster-123", ctx.ClusterID)
	assert.Equal(t, "tenant-456", ctx.TenantID)
	assert.Equal(t, "user-789", ctx.UserID)
	assert.Equal(t, "trace-abc", ctx.TraceID)
	assert.Equal(t, "http://localhost:9090/tools", ctx.ToolEndpoint)
	assert.Equal(t, "zh-CN", ctx.Locale)
}

func TestRequest_Fields(t *testing.T) {
	now := time.Now()
	req := &Request{
		RequestID: "req-123",
		Topic:     TopicAlertAdvisorAggregateWorkloads,
		Version:   CurrentVersion,
		Timestamp: now,
		Context: RequestContext{
			ClusterID: "cluster-1",
		},
		Payload: json.RawMessage(`{"key":"value"}`),
	}

	assert.Equal(t, "req-123", req.RequestID)
	assert.Equal(t, TopicAlertAdvisorAggregateWorkloads, req.Topic)
	assert.Equal(t, CurrentVersion, req.Version)
	assert.Equal(t, now, req.Timestamp)
	assert.Equal(t, "cluster-1", req.Context.ClusterID)
	assert.NotEmpty(t, req.Payload)
}

func TestResponse_Fields(t *testing.T) {
	now := time.Now()
	resp := &Response{
		RequestID: "req-123",
		Status:    StatusSuccess,
		Code:      CodeSuccess,
		Message:   "Operation completed",
		Timestamp: now,
		Payload:   json.RawMessage(`{"result":"ok"}`),
	}

	assert.Equal(t, "req-123", resp.RequestID)
	assert.Equal(t, StatusSuccess, resp.Status)
	assert.Equal(t, CodeSuccess, resp.Code)
	assert.Equal(t, "Operation completed", resp.Message)
	assert.Equal(t, now, resp.Timestamp)
	assert.NotEmpty(t, resp.Payload)
}

func TestRequest_JSON_Serialization(t *testing.T) {
	ctx := RequestContext{
		ClusterID: "cluster-1",
		TenantID:  "tenant-1",
	}
	req, _ := NewRequest(TopicAlertAdvisorAggregateWorkloads, ctx, map[string]string{"key": "value"})

	// Serialize
	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Deserialize
	var decoded Request
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req.RequestID, decoded.RequestID)
	assert.Equal(t, req.Topic, decoded.Topic)
	assert.Equal(t, req.Version, decoded.Version)
	assert.Equal(t, req.Context.ClusterID, decoded.Context.ClusterID)
}

func TestResponse_JSON_Serialization(t *testing.T) {
	resp, _ := NewSuccessResponse("req-123", map[string]string{"result": "ok"})

	// Serialize
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Deserialize
	var decoded Response
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, resp.RequestID, decoded.RequestID)
	assert.Equal(t, resp.Status, decoded.Status)
	assert.Equal(t, resp.Code, decoded.Code)
	assert.Equal(t, resp.Message, decoded.Message)
}

