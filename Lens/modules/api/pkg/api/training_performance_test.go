package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestGetAvailableMetrics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.Default()
	router.GET("/workloads/:uid/metrics/available", GetAvailableMetrics)

	tests := []struct {
		name           string
		uid            string
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:           "Missing UID",
			uid:            "",
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				// Should return error
			},
		},
		{
			name:           "Valid UID",
			uid:            "test-workload-123",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response AvailableMetricsResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "test-workload-123", response.WorkloadUID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/workloads/"+tt.uid+"/metrics/available", nil)
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
			name:           "Missing UID",
			uid:            "",
			queryParams:    "",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:           "Valid UID without filters",
			uid:            "test-workload-123",
			queryParams:    "",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response MetricsDataResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "test-workload-123", response.WorkloadUID)
			},
		},
		{
			name:           "With data_source filter",
			uid:            "test-workload-123",
			queryParams:    "?data_source=wandb",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response MetricsDataResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "wandb", response.DataSource)
			},
		},
		{
			name:           "With metrics filter",
			uid:            "test-workload-123",
			queryParams:    "?metrics=loss,accuracy",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response MetricsDataResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				// Should only contain loss and accuracy metrics
			},
		},
		{
			name:           "With time range",
			uid:            "test-workload-123",
			queryParams:    "?start=1704067200000&end=1704153600000",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response MetricsDataResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				// All data points should be within time range
				for _, point := range response.Data {
					assert.GreaterOrEqual(t, point.Timestamp, int64(1704067200000))
					assert.LessOrEqual(t, point.Timestamp, int64(1704153600000))
				}
			},
		},
		{
			name:           "Invalid start time",
			uid:            "test-workload-123",
			queryParams:    "?start=invalid&end=1704153600000",
			expectedStatus: http.StatusBadRequest,
			checkResponse:  nil,
		},
		{
			name:           "Combined filters",
			uid:            "test-workload-123",
			queryParams:    "?data_source=wandb&metrics=loss,accuracy&start=1704067200000&end=1704153600000",
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response MetricsDataResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "wandb", response.DataSource)
				// All data should be from wandb source
				for _, point := range response.Data {
					assert.Equal(t, "wandb", point.DataSource)
					assert.Contains(t, []string{"loss", "accuracy"}, point.MetricName)
				}
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

// Example test data
func mockTrainingPerformanceData() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"metric_name": "train/loss",
			"value":       1.234,
			"timestamp":   int64(1704067200000),
			"iteration":   int32(1),
			"data_source": "wandb",
		},
		{
			"metric_name": "train/accuracy",
			"value":       0.567,
			"timestamp":   int64(1704067200000),
			"iteration":   int32(1),
			"data_source": "wandb",
		},
		{
			"metric_name": "train/loss",
			"value":       0.987,
			"timestamp":   int64(1704067260000),
			"iteration":   int32(2),
			"data_source": "log",
		},
	}
}

