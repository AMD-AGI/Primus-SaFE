package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/framework"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
)

// DetectionManagerInterface 定义检测管理器接口，便于测试
type DetectionManagerInterface interface {
	GetDetection(ctx context.Context, workloadUID string) (*model.FrameworkDetection, error)
	ReportDetection(ctx context.Context, workloadUID string, source string, frameworkName string, taskType string, confidence float64, evidence map[string]interface{}) error
	GetStatistics(ctx context.Context, startTime string, endTime string, namespace string) (*framework.DetectionStatistics, error)
}

// MockDetectionManager mock implementation
type MockDetectionManager struct {
	mock.Mock
}

func (m *MockDetectionManager) GetDetection(ctx context.Context, workloadUID string) (*model.FrameworkDetection, error) {
	args := m.Called(ctx, workloadUID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.FrameworkDetection), args.Error(1)
}

func (m *MockDetectionManager) ReportDetection(
	ctx context.Context,
	workloadUID string,
	source string,
	frameworkName string,
	taskType string,
	confidence float64,
	evidence map[string]interface{},
) error {
	args := m.Called(ctx, workloadUID, source, frameworkName, taskType, confidence, evidence)
	return args.Error(0)
}

func (m *MockDetectionManager) GetStatistics(
	ctx context.Context,
	startTime string,
	endTime string,
	namespace string,
) (*framework.DetectionStatistics, error) {
	args := m.Called(ctx, startTime, endTime, namespace)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*framework.DetectionStatistics), args.Error(1)
}

// TestableFrameworkDetectionHandler 包装 handler 以支持接口注入
type TestableFrameworkDetectionHandler struct {
	manager DetectionManagerInterface
}

func NewTestableHandler(manager DetectionManagerInterface) *TestableFrameworkDetectionHandler {
	return &TestableFrameworkDetectionHandler{manager: manager}
}

func (h *TestableFrameworkDetectionHandler) RegisterRoutes(router *gin.RouterGroup) {
	workloads := router.Group("/workloads")
	{
		workloads.GET("/:uid/framework-detection", h.GetDetection)
		workloads.POST("/:uid/framework-detection", h.UpdateDetection)
		workloads.POST("/framework-detection/batch", h.GetDetectionBatch)
	}
	detection := router.Group("/framework-detection")
	{
		detection.GET("/stats", h.GetStats)
	}
}

func (h *TestableFrameworkDetectionHandler) GetDetection(c *gin.Context) {
	workloadUID := c.Param("uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workload_uid is required"})
		return
	}

	detection, err := h.manager.GetDetection(c.Request.Context(), workloadUID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get detection"})
		return
	}

	if detection == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":        "workload not found",
			"workload_uid": workloadUID,
		})
		return
	}

	c.JSON(http.StatusOK, detection)
}

func (h *TestableFrameworkDetectionHandler) UpdateDetection(c *gin.Context) {
	workloadUID := c.Param("uid")
	if workloadUID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workload_uid is required"})
		return
	}

	var req UpdateDetectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request", "details": err.Error()})
		return
	}

	validFrameworks := []string{"primus", "deepspeed", "megatron", "pytorch", "tensorflow", "jax", "unknown"}
	valid := false
	for _, f := range validFrameworks {
		if f == req.Framework {
			valid = true
			break
		}
	}
	if !valid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid framework"})
		return
	}

	if req.Type == "" {
		req.Type = "training"
	}
	if req.Confidence == 0 {
		req.Confidence = 1.0
	}

	err := h.manager.ReportDetection(c.Request.Context(), workloadUID, req.Source, req.Framework, req.Type, req.Confidence, req.Evidence)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to report detection"})
		return
	}

	detection, _ := h.manager.GetDetection(c.Request.Context(), workloadUID)
	c.JSON(http.StatusOK, gin.H{"success": true, "detection": detection})
}

func (h *TestableFrameworkDetectionHandler) GetDetectionBatch(c *gin.Context) {
	var req BatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if len(req.WorkloadUIDs) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "batch size cannot exceed 100"})
		return
	}

	results := make([]map[string]interface{}, 0, len(req.WorkloadUIDs))
	for _, uid := range req.WorkloadUIDs {
		detection, err := h.manager.GetDetection(c.Request.Context(), uid)
		result := map[string]interface{}{"workload_uid": uid}
		if err != nil {
			result["error"] = err.Error()
		} else if detection == nil {
			result["error"] = "not found"
		} else {
			result["detection"] = detection
		}
		results = append(results, result)
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "total": len(results)})
}

func (h *TestableFrameworkDetectionHandler) GetStats(c *gin.Context) {
	startTime := c.Query("start_time")
	endTime := c.Query("end_time")
	namespace := c.Query("namespace")

	stats, err := h.manager.GetStatistics(c.Request.Context(), startTime, endTime, namespace)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get statistics"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// setupTestRouter 创建测试路由
func setupTestRouter(handler *TestableFrameworkDetectionHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	apiV1 := router.Group("/api/v1")
	handler.RegisterRoutes(apiV1)
	return router
}

// TestGetDetection_Success 测试获取检测结果成功
func TestGetDetection_Success(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	// Mock data
	expectedDetection := &model.FrameworkDetection{
		Framework:  "primus",
		Type:       "training",
		Confidence: 0.95,
		Status:     "verified",
		Sources: []model.DetectionSource{
			{
				Source:     "reuse",
				Framework:  "primus",
				Confidence: 0.85,
			},
		},
	}

	mockManager.On("GetDetection", mock.Anything, "workload-123").Return(expectedDetection, nil)

	// Make request
	req, _ := http.NewRequest("GET", "/api/v1/workloads/workload-123/framework-detection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response model.FrameworkDetection
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "primus", response.Framework)
	assert.Equal(t, 0.95, response.Confidence)

	mockManager.AssertExpectations(t)
}

// TestGetDetection_NotFound 测试获取检测结果 - 未找到
func TestGetDetection_NotFound(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	mockManager.On("GetDetection", mock.Anything, "workload-999").Return(nil, nil)

	req, _ := http.NewRequest("GET", "/api/v1/workloads/workload-999/framework-detection", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "workload not found", response["error"])
	assert.Equal(t, "workload-999", response["workload_uid"])

	mockManager.AssertExpectations(t)
}

// TestUpdateDetection_Success 测试更新检测结果成功
func TestUpdateDetection_Success(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	updateReq := UpdateDetectionRequest{
		Source:     "user",
		Framework:  "primus",
		Type:       "training",
		Confidence: 1.0,
		Evidence: map[string]interface{}{
			"method": "manual_annotation",
			"user":   "admin@example.com",
		},
	}

	mockManager.On("ReportDetection",
		mock.Anything,
		"workload-123",
		"user",
		"primus",
		"training",
		1.0,
		mock.AnythingOfType("map[string]interface {}"),
	).Return(nil)

	updatedDetection := &model.FrameworkDetection{
		Framework:  "primus",
		Confidence: 1.0,
		Status:     "verified",
	}
	mockManager.On("GetDetection", mock.Anything, "workload-123").Return(updatedDetection, nil)

	// Make request
	jsonData, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("POST", "/api/v1/workloads/workload-123/framework-detection", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.True(t, response["success"].(bool))
	assert.NotNil(t, response["detection"])

	mockManager.AssertExpectations(t)
}

// TestUpdateDetection_InvalidFramework 测试更新检测结果 - 无效框架
func TestUpdateDetection_InvalidFramework(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	updateReq := UpdateDetectionRequest{
		Source:     "user",
		Framework:  "invalid-framework",
		Confidence: 1.0,
	}

	jsonData, _ := json.Marshal(updateReq)
	req, _ := http.NewRequest("POST", "/api/v1/workloads/workload-123/framework-detection", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, "invalid framework", response["error"])
}

// TestGetDetectionBatch_Success 测试批量查询成功
func TestGetDetectionBatch_Success(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	batchReq := BatchRequest{
		WorkloadUIDs: []string{"workload-1", "workload-2", "workload-3"},
	}

	// Mock responses
	mockManager.On("GetDetection", mock.Anything, "workload-1").Return(&model.FrameworkDetection{
		Framework: "primus",
	}, nil)
	mockManager.On("GetDetection", mock.Anything, "workload-2").Return(&model.FrameworkDetection{
		Framework: "deepspeed",
	}, nil)
	mockManager.On("GetDetection", mock.Anything, "workload-3").Return(nil, nil) // Not found

	// Make request
	jsonData, _ := json.Marshal(batchReq)
	req, _ := http.NewRequest("POST", "/api/v1/workloads/framework-detection/batch", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	results := response["results"].([]interface{})
	assert.Equal(t, 3, len(results))
	assert.Equal(t, float64(3), response["total"])

	// Check first result (found)
	result1 := results[0].(map[string]interface{})
	assert.Equal(t, "workload-1", result1["workload_uid"])
	assert.NotNil(t, result1["detection"])

	// Check third result (not found)
	result3 := results[2].(map[string]interface{})
	assert.Equal(t, "workload-3", result3["workload_uid"])
	assert.Equal(t, "not found", result3["error"])

	mockManager.AssertExpectations(t)
}

// TestGetDetectionBatch_ExceedLimit 测试批量查询 - 超过限制
func TestGetDetectionBatch_ExceedLimit(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	// Create 101 workload UIDs
	workloadUIDs := make([]string, 101)
	for i := 0; i < 101; i++ {
		workloadUIDs[i] = "workload-" + string(rune(i))
	}

	batchReq := BatchRequest{
		WorkloadUIDs: workloadUIDs,
	}

	jsonData, _ := json.Marshal(batchReq)
	req, _ := http.NewRequest("POST", "/api/v1/workloads/framework-detection/batch", bytes.NewBuffer(jsonData))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// TestGetStats_Success 测试获取统计信息成功
func TestGetStats_Success(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	expectedStats := &framework.DetectionStatistics{
		TotalWorkloads: 1000,
		ByFramework: map[string]int64{
			"primus":    650,
			"deepspeed": 250,
			"megatron":  80,
			"unknown":   20,
		},
		ByStatus: map[string]int64{
			"verified":  800,
			"confirmed": 150,
			"suspected": 30,
			"unknown":   20,
		},
		BySource: map[string]int64{
			"reuse":     500,
			"component": 900,
			"log":       800,
			"wandb":     300,
			"user":      50,
		},
		AverageConfidence: 0.88,
		ConflictRate:      0.02,
		ReuseRate:         0.50,
	}

	mockManager.On("GetStatistics", mock.Anything, "", "", "").Return(expectedStats, nil)

	req, _ := http.NewRequest("GET", "/api/v1/framework-detection/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response framework.DetectionStatistics
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, int64(1000), response.TotalWorkloads)
	assert.Equal(t, int64(650), response.ByFramework["primus"])
	assert.Equal(t, 0.88, response.AverageConfidence)
	assert.Equal(t, 0.02, response.ConflictRate)
	assert.Equal(t, 0.50, response.ReuseRate)

	mockManager.AssertExpectations(t)
}

// TestGetStats_WithFilters 测试获取统计信息 - 带过滤条件
func TestGetStats_WithFilters(t *testing.T) {
	mockManager := new(MockDetectionManager)
	handler := NewTestableHandler(mockManager)
	router := setupTestRouter(handler)

	expectedStats := &framework.DetectionStatistics{
		TotalWorkloads: 500,
		ByFramework: map[string]int64{
			"primus": 500,
		},
		StartTime:         "2025-01-01T00:00:00Z",
		EndTime:           "2025-01-31T23:59:59Z",
		Namespace:         "production",
		AverageConfidence: 0.92,
	}

	mockManager.On("GetStatistics",
		mock.Anything,
		"2025-01-01T00:00:00Z",
		"2025-01-31T23:59:59Z",
		"production",
	).Return(expectedStats, nil)

	req, _ := http.NewRequest("GET", "/api/v1/framework-detection/stats?start_time=2025-01-01T00:00:00Z&end_time=2025-01-31T23:59:59Z&namespace=production", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response framework.DetectionStatistics
	err := json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, int64(500), response.TotalWorkloads)
	assert.Equal(t, "production", response.Namespace)

	mockManager.AssertExpectations(t)
}
