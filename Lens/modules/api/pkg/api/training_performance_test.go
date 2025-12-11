package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestIsMetricField tests the isMetricField function
func TestIsMetricField(t *testing.T) {
	tests := []struct {
		name       string
		fieldName  string
		dataSource string
		expected   bool
	}{
		// wandb tests
		{"wandb_actual_metric", "train/loss", "wandb", true},
		{"wandb_metadata_step", "step", "wandb", false},
		{"wandb_metadata_run_id", "run_id", "wandb", false},
		{"wandb_metadata_source", "source", "wandb", false},
		{"wandb_metadata_history", "history", "wandb", false},
		{"wandb_metadata_created_at", "created_at", "wandb", false},
		{"wandb_metadata_updated_at", "updated_at", "wandb", false},

		// tensorflow tests
		{"tensorflow_actual_metric", "loss", "tensorflow", true},
		{"tensorflow_metadata_step", "step", "tensorflow", false},
		{"tensorflow_metadata_wall_time", "wall_time", "tensorflow", false},
		{"tensorflow_metadata_file", "file", "tensorflow", false},
		{"tensorflow_metadata_scalars", "scalars", "tensorflow", false},
		{"tensorflow_metadata_texts", "texts", "tensorflow", false},
		{"tensorflow_vs_samples", "loss vs samples", "tensorflow", false},
		{"tensorflow_vs_steps", "loss vs steps", "tensorflow", false},

		// log tests
		{"log_all_metrics", "any_field", "log", true},
		{"log_loss", "train/loss", "log", true},

		// default/unknown source tests
		{"unknown_source_all_metrics", "any_field", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMetricField(tt.fieldName, tt.dataSource)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDeduplicateTensorflowDataPoints tests the deduplicateTensorflowDataPoints function
func TestDeduplicateTensorflowDataPoints(t *testing.T) {
	tests := []struct {
		name       string
		dataPoints []MetricDataPoint
		expected   int // expected number of points after deduplication
	}{
		{
			name:       "empty_data",
			dataPoints: []MetricDataPoint{},
			expected:   0,
		},
		{
			name: "no_duplicates",
			dataPoints: []MetricDataPoint{
				{MetricName: "loss", Value: 1.0, Timestamp: 1000, Iteration: 1, DataSource: "tensorflow"},
				{MetricName: "loss", Value: 2.0, Timestamp: 2000, Iteration: 2, DataSource: "tensorflow"},
			},
			expected: 2,
		},
		{
			name: "duplicate_with_different_iterations",
			dataPoints: []MetricDataPoint{
				{MetricName: "loss", Value: 1.0, Timestamp: 1000, Iteration: 1, DataSource: "tensorflow"},
				{MetricName: "loss", Value: 1.1, Timestamp: 1005, Iteration: 100, DataSource: "tensorflow"}, // should be removed
				{MetricName: "loss", Value: 2.0, Timestamp: 20000, Iteration: 2, DataSource: "tensorflow"},
			},
			expected: 2,
		},
		{
			name: "multiple_metrics",
			dataPoints: []MetricDataPoint{
				{MetricName: "loss", Value: 1.0, Timestamp: 1000, Iteration: 1, DataSource: "tensorflow"},
				{MetricName: "loss", Value: 1.1, Timestamp: 1005, Iteration: 100, DataSource: "tensorflow"},
				{MetricName: "accuracy", Value: 0.8, Timestamp: 1000, Iteration: 1, DataSource: "tensorflow"},
				{MetricName: "accuracy", Value: 0.81, Timestamp: 1005, Iteration: 100, DataSource: "tensorflow"},
			},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deduplicateTensorflowDataPoints(tt.dataPoints)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

// TestHasTensorflowData tests the hasTensorflowData function
func TestHasTensorflowData(t *testing.T) {
	tests := []struct {
		name         string
		iterationMap map[int32]*IterationInfo
		expected     bool
	}{
		{
			name:         "empty_map",
			iterationMap: map[int32]*IterationInfo{},
			expected:     false,
		},
		{
			name: "has_tensorflow_data",
			iterationMap: map[int32]*IterationInfo{
				1: {DataSource: "tensorflow", Timestamp: 1000},
			},
			expected: true,
		},
		{
			name: "no_tensorflow_data",
			iterationMap: map[int32]*IterationInfo{
				1: {DataSource: "wandb", Timestamp: 1000},
				2: {DataSource: "log", Timestamp: 2000},
			},
			expected: false,
		},
		{
			name: "mixed_data",
			iterationMap: map[int32]*IterationInfo{
				1: {DataSource: "wandb", Timestamp: 1000},
				2: {DataSource: "tensorflow", Timestamp: 2000},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasTensorflowData(tt.iterationMap)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFilterAnomalousIterations tests the filterAnomalousIterations function
func TestFilterAnomalousIterations(t *testing.T) {
	tests := []struct {
		name         string
		iterationMap map[int32]*IterationInfo
		expectedLen  int
	}{
		{
			name:         "empty_map",
			iterationMap: map[int32]*IterationInfo{},
			expectedLen:  0,
		},
		{
			name: "no_anomalies",
			iterationMap: map[int32]*IterationInfo{
				1: {DataSource: "tensorflow", Timestamp: 1000},
				2: {DataSource: "tensorflow", Timestamp: 2000},
				3: {DataSource: "tensorflow", Timestamp: 3000},
			},
			expectedLen: 3,
		},
		{
			name: "with_anomalies",
			iterationMap: map[int32]*IterationInfo{
				1:    {DataSource: "tensorflow", Timestamp: 1000},
				2:    {DataSource: "tensorflow", Timestamp: 2000},
				3:    {DataSource: "tensorflow", Timestamp: 3000},
				1000: {DataSource: "tensorflow", Timestamp: 4000}, // anomaly: 1000/3 > 10
			},
			expectedLen: 3, // should filter out the anomaly
		},
		{
			name: "too_few_data_points",
			iterationMap: map[int32]*IterationInfo{
				1: {DataSource: "tensorflow", Timestamp: 1000},
				2: {DataSource: "tensorflow", Timestamp: 2000},
			},
			expectedLen: 2, // no filtering when < 3 data points
		},
		{
			name: "filtered_too_much",
			iterationMap: map[int32]*IterationInfo{
				1:   {DataSource: "tensorflow", Timestamp: 1000},
				100: {DataSource: "tensorflow", Timestamp: 2000}, // anomaly
				200: {DataSource: "tensorflow", Timestamp: 3000},
				300: {DataSource: "tensorflow", Timestamp: 4000},
			},
			expectedLen: 4, // should return original when filtered > 50%
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterAnomalousIterations(tt.iterationMap)
			assert.Equal(t, tt.expectedLen, len(result))
		})
	}
}

func TestGetAvailableMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/workloads/:uid/metrics/available", GetAvailableMetrics)

	tests := []struct {
		name           string
		uid            string
		queryParams    string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "missing_uid",
			uid:            "",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			url := "/workloads/" + tt.uid + "/metrics/available" + tt.queryParams
			req, _ := http.NewRequest("GET", url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

func TestGetMetricsData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/workloads/:uid/metrics/data", GetMetricsData)

	tests := []struct {
		name           string
		uid            string
		queryParams    string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "missing_uid",
			uid:            "",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
		{
			name:           "invalid_start_time",
			uid:            "test-workload-123",
			queryParams:    "?start=invalid&end=1704153600000",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
		{
			name:           "invalid_end_time",
			uid:            "test-workload-123",
			queryParams:    "?start=1704067200000&end=invalid",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			url := "/workloads/" + tt.uid + "/metrics/data" + tt.queryParams
			req, _ := http.NewRequest("GET", url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

func TestGetDataSources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/workloads/:uid/metrics/sources", GetDataSources)

	tests := []struct {
		name           string
		uid            string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "missing_uid",
			uid:            "",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			url := "/workloads/" + tt.uid + "/metrics/sources"
			req, _ := http.NewRequest("GET", url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}

func TestGetIterationTimes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/workloads/:uid/metrics/iteration-times", GetIterationTimes)

	tests := []struct {
		name           string
		uid            string
		queryParams    string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "missing_uid",
			uid:            "",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
		// Note: Tests with valid UID require cluster manager and database initialization
		// These should be tested as integration tests with proper setup
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			url := "/workloads/" + tt.uid + "/metrics/iteration-times" + tt.queryParams
			req, _ := http.NewRequest("GET", url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.checkResponse != nil {
				tt.checkResponse(t, w)
			}
		})
	}
}
