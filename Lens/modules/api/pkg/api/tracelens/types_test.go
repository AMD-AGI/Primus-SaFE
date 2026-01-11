// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package tracelens

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateSessionRequestJSON(t *testing.T) {
	jsonData := `{
		"workload_uid": "workload-123",
		"profiler_file_id": 456,
		"ttl_minutes": 120,
		"resource_profile": "large"
	}`

	var req CreateSessionRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	assert.Equal(t, "workload-123", req.WorkloadUID)
	assert.Equal(t, int32(456), req.ProfilerFileID)
	assert.Equal(t, 120, req.TTLMinutes)
	assert.Equal(t, "large", req.ResourceProfile)
}

func TestCreateSessionRequestDefaults(t *testing.T) {
	jsonData := `{
		"workload_uid": "workload-123",
		"profiler_file_id": 456
	}`

	var req CreateSessionRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	assert.Equal(t, "workload-123", req.WorkloadUID)
	assert.Equal(t, int32(456), req.ProfilerFileID)
	assert.Equal(t, 0, req.TTLMinutes) // default value
	assert.Equal(t, "", req.ResourceProfile)
}

func TestSessionResponseJSON(t *testing.T) {
	now := time.Now()
	readyAt := now.Add(-5 * time.Minute)
	expiresAt := now.Add(55 * time.Minute)

	resp := SessionResponse{
		SessionID:       "tls-abc123",
		WorkloadUID:     "workload-123",
		ProfilerFileID:  456,
		Status:          "ready",
		StatusMessage:   "Session is ready",
		UIPath:          "/v1/tracelens/sessions/tls-abc123/ui",
		PodName:         "tracelens-abc123",
		PodIP:           "10.0.0.100",
		ResourceProfile: "medium",
		CreatedAt:       now,
		ReadyAt:         &readyAt,
		ExpiresAt:       expiresAt,
		EstimatedReady:  30,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded SessionResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "tls-abc123", decoded.SessionID)
	assert.Equal(t, "workload-123", decoded.WorkloadUID)
	assert.Equal(t, int32(456), decoded.ProfilerFileID)
	assert.Equal(t, "ready", decoded.Status)
	assert.Equal(t, "Session is ready", decoded.StatusMessage)
	assert.Equal(t, "/v1/tracelens/sessions/tls-abc123/ui", decoded.UIPath)
	assert.Equal(t, "tracelens-abc123", decoded.PodName)
	assert.Equal(t, "10.0.0.100", decoded.PodIP)
	assert.Equal(t, "medium", decoded.ResourceProfile)
	assert.NotNil(t, decoded.ReadyAt)
	assert.Equal(t, 30, decoded.EstimatedReady)
}

func TestSessionResponseOptionalFields(t *testing.T) {
	now := time.Now()

	resp := SessionResponse{
		SessionID:      "tls-abc123",
		WorkloadUID:    "workload-123",
		ProfilerFileID: 456,
		Status:         "pending",
		CreatedAt:      now,
		ExpiresAt:      now.Add(1 * time.Hour),
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify omitempty fields are not present
	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotContains(t, decoded, "status_message")
	assert.NotContains(t, decoded, "ui_path")
	assert.NotContains(t, decoded, "pod_name")
	assert.NotContains(t, decoded, "pod_ip")
	assert.NotContains(t, decoded, "ready_at")
	assert.NotContains(t, decoded, "last_accessed_at")
}

func TestExtendSessionRequestJSON(t *testing.T) {
	jsonData := `{"extend_minutes": 60}`

	var req ExtendSessionRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	assert.Equal(t, 60, req.ExtendMinutes)
}

func TestListSessionsResponseJSON(t *testing.T) {
	now := time.Now()

	resp := ListSessionsResponse{
		Sessions: []SessionResponse{
			{
				SessionID:      "tls-1",
				WorkloadUID:    "workload-1",
				ProfilerFileID: 1,
				Status:         "ready",
				CreatedAt:      now,
				ExpiresAt:      now.Add(1 * time.Hour),
			},
			{
				SessionID:      "tls-2",
				WorkloadUID:    "workload-2",
				ProfilerFileID: 2,
				Status:         "pending",
				CreatedAt:      now,
				ExpiresAt:      now.Add(1 * time.Hour),
			},
		},
		Total: 2,
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded ListSessionsResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Len(t, decoded.Sessions, 2)
	assert.Equal(t, 2, decoded.Total)
	assert.Equal(t, "tls-1", decoded.Sessions[0].SessionID)
	assert.Equal(t, "tls-2", decoded.Sessions[1].SessionID)
}

func TestSessionStatusResponseJSON(t *testing.T) {
	resp := SessionStatusResponse{
		SessionID: "tls-abc123",
		Status:    "ready",
		Message:   "Pod is running",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded SessionStatusResponse
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "tls-abc123", decoded.SessionID)
	assert.Equal(t, "ready", decoded.Status)
	assert.Equal(t, "Pod is running", decoded.Message)
}

func TestSessionStatusResponseOptionalMessage(t *testing.T) {
	resp := SessionStatusResponse{
		SessionID: "tls-abc123",
		Status:    "pending",
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotContains(t, decoded, "message")
}

func TestPodStatusInfoStruct(t *testing.T) {
	info := PodStatusInfo{
		Exists: true,
		Phase:  "Running",
		Ready:  true,
		PodIP:  "10.0.0.100",
	}

	assert.True(t, info.Exists)
	assert.Equal(t, "Running", info.Phase)
	assert.True(t, info.Ready)
	assert.Equal(t, "10.0.0.100", info.PodIP)
}

func TestPodStatusInfoJSON(t *testing.T) {
	info := PodStatusInfo{
		Exists: true,
		Phase:  "Running",
		Ready:  true,
		PodIP:  "10.0.0.100",
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	var decoded PodStatusInfo
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, info.Exists, decoded.Exists)
	assert.Equal(t, info.Phase, decoded.Phase)
	assert.Equal(t, info.Ready, decoded.Ready)
	assert.Equal(t, info.PodIP, decoded.PodIP)
}

func TestPodStatusInfoOptionalPodIP(t *testing.T) {
	info := PodStatusInfo{
		Exists: false,
		Phase:  "NotFound",
		Ready:  false,
	}

	data, err := json.Marshal(info)
	require.NoError(t, err)

	var decoded map[string]interface{}
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.NotContains(t, decoded, "pod_ip")
}

func TestProfilerFileContentChunkStruct(t *testing.T) {
	content := []byte("test content data")
	chunk := ProfilerFileContentChunk{
		Content:         content,
		ContentEncoding: "gzip",
		ChunkIndex:      0,
		TotalChunks:     3,
	}

	assert.Equal(t, content, chunk.Content)
	assert.Equal(t, "gzip", chunk.ContentEncoding)
	assert.Equal(t, 0, chunk.ChunkIndex)
	assert.Equal(t, 3, chunk.TotalChunks)
}

