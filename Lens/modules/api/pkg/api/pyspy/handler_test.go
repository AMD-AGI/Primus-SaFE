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

	// Test missing required fields
	reqBody := CreateTaskRequest{
		// Missing PodUID, NodeName, PID
	}
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

	// Should return 404 because route doesn't match
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestCancelTask_EmptyID(t *testing.T) {
	router := gin.New()
	router.POST("/api/v1/pyspy/task/:id/cancel", CancelTask)

	// When ID is empty but matches the route pattern, handler returns 400
	req, _ := http.NewRequest(http.MethodPost, "/api/v1/pyspy/task//cancel", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Route with empty :id matches and handler returns 400 for empty task id
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDownloadFile_RequiresTaskIDAndFilename(t *testing.T) {
	// Note: Full integration test requires database setup
	// This test just verifies the route matching behavior
	router := gin.New()
	
	// Test that the route with both params is matched correctly
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

func TestNodeExporterClient_New(t *testing.T) {
	client := NewNodeExporterClient()
	assert.NotNil(t, client)
	assert.NotNil(t, client.httpClient)
	assert.Equal(t, DefaultNodeExporterPort, client.port)
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

