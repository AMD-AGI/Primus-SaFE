// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package pyspy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestCreateTaskRequest_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		req      CreateTaskRequest
		expected CreateTaskRequest
	}{
		{
			name: "empty request should have defaults",
			req:  CreateTaskRequest{},
			expected: CreateTaskRequest{
				Duration: 30,
				Rate:     100,
				Format:   "flamegraph",
			},
		},
		{
			name: "custom values should be preserved",
			req: CreateTaskRequest{
				Duration: 60,
				Rate:     200,
				Format:   "speedscope",
			},
			expected: CreateTaskRequest{
				Duration: 60,
				Rate:     200,
				Format:   "speedscope",
			},
		},
		{
			name: "zero duration should use default",
			req: CreateTaskRequest{
				Duration: 0,
				Rate:     150,
				Format:   "raw",
			},
			expected: CreateTaskRequest{
				Duration: 30,
				Rate:     150,
				Format:   "raw",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.SetDefaults()
			assert.Equal(t, tt.expected.Duration, tt.req.Duration)
			assert.Equal(t, tt.expected.Rate, tt.req.Rate)
			assert.Equal(t, tt.expected.Format, tt.req.Format)
		})
	}
}

func TestListTasksRequest_SetDefaults(t *testing.T) {
	tests := []struct {
		name     string
		req      ListTasksRequest
		expected ListTasksRequest
	}{
		{
			name: "empty request should have default limit",
			req:  ListTasksRequest{},
			expected: ListTasksRequest{
				Limit: 50,
			},
		},
		{
			name: "over max limit should be capped",
			req: ListTasksRequest{
				Limit: 200,
			},
			expected: ListTasksRequest{
				Limit: 100,
			},
		},
		{
			name: "valid limit should be preserved",
			req: ListTasksRequest{
				Limit: 25,
			},
			expected: ListTasksRequest{
				Limit: 25,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.SetDefaults()
			assert.Equal(t, tt.expected.Limit, tt.req.Limit)
		})
	}
}

func TestGetFilename(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/var/lib/lens/pyspy/profiles/task-123/profile.svg", "profile.svg"},
		{"/profile.json", "profile.json"},
		{"profile.txt", "profile.txt"},
		{"", ""},
	}

	for _, tt := range tests {
		result := getFilename(tt.path)
		assert.Equal(t, tt.expected, result, "path: %s", tt.path)
	}
}

func TestTaskResponse_JSON(t *testing.T) {
	now := time.Now()
	resp := TaskResponse{
		TaskID:       "pyspy-test123",
		Status:       "completed",
		PodUID:       "pod-uid-123",
		PodName:      "test-pod",
		PodNamespace: "default",
		NodeName:     "node-01",
		PID:          12345,
		Duration:     30,
		Format:       "flamegraph",
		OutputFile:   "/var/lib/lens/pyspy/profiles/pyspy-test123/profile.svg",
		FileSize:     125000,
		CreatedAt:    now,
		FilePath:     "/api/v1/pyspy/file/pyspy-test123/profile.svg",
	}

	jsonBytes, err := json.Marshal(resp)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"task_id":"pyspy-test123"`)
	assert.Contains(t, string(jsonBytes), `"status":"completed"`)
	assert.Contains(t, string(jsonBytes), `"file_size":125000`)
}

func TestCreateTask_ValidationError(t *testing.T) {
	router := gin.New()
	router.POST("/api/v1/pyspy/sample", CreateTask)

	reqBody := CreateTaskRequest{}
	body, _ := json.Marshal(reqBody)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/pyspy/sample", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetTask_MissingID(t *testing.T) {
	router := gin.New()
	router.GET("/api/v1/pyspy/task/:id", GetTask)

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/pyspy/task/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCancelTask_EmptyID(t *testing.T) {
	router := gin.New()
	router.POST("/api/v1/pyspy/task/:id/cancel", CancelTask)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/pyspy/task//cancel", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDownloadFile_RequiresTaskIDAndFilename(t *testing.T) {
	router := gin.New()

	matched := false
	router.GET("/api/v1/pyspy/file/:task_id/:filename", func(c *gin.Context) {
		matched = true
		taskID := c.Param("task_id")
		filename := c.Param("filename")
		assert.Equal(t, "task-123", taskID)
		assert.Equal(t, "profile.svg", filename)
		c.Status(http.StatusOK)
	})

	req, _ := http.NewRequest(http.MethodGet, "/api/v1/pyspy/file/task-123/profile.svg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.True(t, matched, "route should be matched")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestListTasksResponse_JSON(t *testing.T) {
	resp := ListTasksResponse{
		Tasks:  []TaskResponse{},
		Total:  0,
		Limit:  50,
		Offset: 0,
	}

	jsonBytes, err := json.Marshal(resp)
	assert.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"tasks":[]`)
	assert.Contains(t, string(jsonBytes), `"total":0`)
	assert.Contains(t, string(jsonBytes), `"limit":50`)
}

func TestCreateTaskRequest_AllFields(t *testing.T) {
	req := CreateTaskRequest{
		Cluster:      "production",
		PodUID:       "pod-uid-12345",
		PodName:      "my-python-app-abc123",
		PodNamespace: "data-science",
		NodeName:     "worker-node-1",
		PID:          9999,
		Duration:     60,
		Rate:         200,
		Format:       "speedscope",
		Native:       true,
		SubProcesses: true,
	}

	assert.Equal(t, "production", req.Cluster)
	assert.Equal(t, "pod-uid-12345", req.PodUID)
	assert.Equal(t, "my-python-app-abc123", req.PodName)
	assert.Equal(t, "data-science", req.PodNamespace)
	assert.Equal(t, "worker-node-1", req.NodeName)
	assert.Equal(t, 9999, req.PID)
	assert.Equal(t, 60, req.Duration)
	assert.Equal(t, 200, req.Rate)
	assert.Equal(t, "speedscope", req.Format)
	assert.True(t, req.Native)
	assert.True(t, req.SubProcesses)
}

func TestCreateTaskRequest_SetDefaults_NegativeValues(t *testing.T) {
	tests := []struct {
		name     string
		req      CreateTaskRequest
		expected CreateTaskRequest
	}{
		{
			name: "negative duration should use default",
			req: CreateTaskRequest{
				Duration: -10,
				Rate:     100,
				Format:   "flamegraph",
			},
			expected: CreateTaskRequest{
				Duration: 30,
				Rate:     100,
				Format:   "flamegraph",
			},
		},
		{
			name: "negative rate should use default",
			req: CreateTaskRequest{
				Duration: 30,
				Rate:     -50,
				Format:   "flamegraph",
			},
			expected: CreateTaskRequest{
				Duration: 30,
				Rate:     100,
				Format:   "flamegraph",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.SetDefaults()
			assert.Equal(t, tt.expected.Duration, tt.req.Duration)
			assert.Equal(t, tt.expected.Rate, tt.req.Rate)
			assert.Equal(t, tt.expected.Format, tt.req.Format)
		})
	}
}

func TestListTasksRequest_AllFields(t *testing.T) {
	req := ListTasksRequest{
		Cluster:      "staging",
		PodUID:       "pod-123",
		PodNamespace: "default",
		NodeName:     "node-2",
		Status:       "completed",
		Limit:        25,
		Offset:       50,
	}

	assert.Equal(t, "staging", req.Cluster)
	assert.Equal(t, "pod-123", req.PodUID)
	assert.Equal(t, "default", req.PodNamespace)
	assert.Equal(t, "node-2", req.NodeName)
	assert.Equal(t, "completed", req.Status)
	assert.Equal(t, 25, req.Limit)
	assert.Equal(t, 50, req.Offset)
}

func TestListTasksRequest_SetDefaults_EdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		req           ListTasksRequest
		expectedLimit int
	}{
		{"zero limit", ListTasksRequest{Limit: 0}, 50},
		{"negative limit", ListTasksRequest{Limit: -10}, 50},
		{"exactly 100", ListTasksRequest{Limit: 100}, 100},
		{"one over max", ListTasksRequest{Limit: 101}, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.req.SetDefaults()
			assert.Equal(t, tt.expectedLimit, tt.req.Limit)
		})
	}
}

func TestTaskResponse_AllFields(t *testing.T) {
	now := time.Now()
	startedAt := now.Add(-30 * time.Second)
	completedAt := now

	resp := TaskResponse{
		TaskID:       "pyspy-task-abcd1234",
		Status:       "completed",
		PodUID:       "pod-uid-xyz",
		PodName:      "data-processor-pod",
		PodNamespace: "ml-workloads",
		NodeName:     "gpu-node-3",
		PID:          55555,
		Duration:     30,
		Format:       "flamegraph",
		OutputFile:   "/data/pyspy/profiles/pyspy-task-abcd1234/output.svg",
		FileSize:     256000,
		CreatedAt:    now.Add(-1 * time.Minute),
		StartedAt:    &startedAt,
		CompletedAt:  &completedAt,
		FilePath:     "/api/v1/pyspy/file/pyspy-task-abcd1234/output.svg",
	}

	assert.Equal(t, "pyspy-task-abcd1234", resp.TaskID)
	assert.Equal(t, "completed", resp.Status)
	assert.Equal(t, 55555, resp.PID)
	assert.Equal(t, int64(256000), resp.FileSize)
	assert.NotNil(t, resp.StartedAt)
	assert.NotNil(t, resp.CompletedAt)
}

func TestCancelTaskRequest_Fields(t *testing.T) {
	req := CancelTaskRequest{
		Reason: "User requested cancellation",
	}
	assert.Equal(t, "User requested cancellation", req.Reason)

	emptyReq := CancelTaskRequest{}
	assert.Equal(t, "", emptyReq.Reason)
}

func TestTaskStatusResponse_Fields(t *testing.T) {
	resp := TaskStatusResponse{
		TaskID:  "task-cancel-001",
		Status:  "cancelled",
		Message: "Task cancelled successfully",
	}

	assert.Equal(t, "task-cancel-001", resp.TaskID)
	assert.Equal(t, "cancelled", resp.Status)
	assert.Equal(t, "Task cancelled successfully", resp.Message)
}

func TestGetFilename_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"unix absolute path", "/var/lib/lens/profile.svg", "profile.svg"},
		{"relative path", "data/profile.svg", "profile.svg"},
		{"just filename", "profile.svg", "profile.svg"},
		{"empty string", "", ""},
		{"path ending with slash", "/data/", ""},
		{"dotfile", "/data/.hidden", ".hidden"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getFilename(tt.path)
			assert.Equal(t, tt.expected, result, "path: %s", tt.path)
		})
	}
}

func TestCreateTask_InvalidJSON(t *testing.T) {
	router := gin.New()
	router.POST("/api/v1/pyspy/sample", CreateTask)

	req, _ := http.NewRequest(http.MethodPost, "/api/v1/pyspy/sample", bytes.NewBufferString("{invalid}"))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func BenchmarkCreateTaskRequest_SetDefaults(b *testing.B) {
	for i := 0; i < b.N; i++ {
		req := CreateTaskRequest{}
		req.SetDefaults()
	}
}

func BenchmarkGetFilename(b *testing.B) {
	path := "/var/lib/lens/pyspy/profiles/task-123/profile.svg"
	for i := 0; i < b.N; i++ {
		_ = getFilename(path)
	}
}
