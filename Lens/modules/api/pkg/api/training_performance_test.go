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
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
		// Note: Other test cases require cluster manager and database initialization
		// These should be tested as integration tests with proper setup
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
			checkResponse: func(t *testing.T, w *httptest.ResponseRecorder) {
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Contains(t, response, "error")
			},
		},
		{
			name:           "Invalid start time",
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
		// Note: Other test cases require cluster manager and database initialization
		// These should be tested as integration tests with proper setup
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
