package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Setup
// ============================================================================

func init() {
	gin.SetMode(gin.TestMode)
}

// ============================================================================
// NewProfilerHandler Tests
// ============================================================================

func TestNewProfilerHandler(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)

	require.NotNil(t, handler)
	assert.Nil(t, handler.lifecycleMgr)
	assert.Nil(t, handler.metadataMgr)
}

// ============================================================================
// parseIntQuery Tests
// ============================================================================

func TestParseIntQuery(t *testing.T) {
	tests := []struct {
		name       string
		queryKey   string
		queryValue string
		defaultVal int
		expected   int
	}{
		{
			name:       "valid integer",
			queryKey:   "limit",
			queryValue: "100",
			defaultVal: 50,
			expected:   100,
		},
		{
			name:       "empty value uses default",
			queryKey:   "limit",
			queryValue: "",
			defaultVal: 50,
			expected:   50,
		},
		{
			name:       "invalid value uses default",
			queryKey:   "limit",
			queryValue: "not_a_number",
			defaultVal: 50,
			expected:   50,
		},
		{
			name:       "zero value",
			queryKey:   "offset",
			queryValue: "0",
			defaultVal: 10,
			expected:   0,
		},
		{
			name:       "negative value",
			queryKey:   "offset",
			queryValue: "-5",
			defaultVal: 0,
			expected:   -5,
		},
		{
			name:       "large value",
			queryKey:   "limit",
			queryValue: "999999",
			defaultVal: 100,
			expected:   999999,
		},
		{
			name:       "float value (truncated)",
			queryKey:   "limit",
			queryValue: "3.14",
			defaultVal: 10,
			expected:   10, // Invalid, uses default
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			if tt.queryValue != "" {
				c.Request = httptest.NewRequest("GET", "/?"+tt.queryKey+"="+tt.queryValue, nil)
			} else {
				c.Request = httptest.NewRequest("GET", "/", nil)
			}

			result := parseIntQuery(c, tt.queryKey, tt.defaultVal)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ============================================================================
// parseDuration Tests
// ============================================================================

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "hours",
			input:    "24h",
			expected: 24 * time.Hour,
			wantErr:  false,
		},
		{
			name:     "minutes",
			input:    "30m",
			expected: 30 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "seconds",
			input:    "60s",
			expected: 60 * time.Second,
			wantErr:  false,
		},
		{
			name:     "complex duration",
			input:    "1h30m",
			expected: 90 * time.Minute,
			wantErr:  false,
		},
		{
			name:     "milliseconds",
			input:    "500ms",
			expected: 500 * time.Millisecond,
			wantErr:  false,
		},
		{
			name:     "invalid format",
			input:    "invalid",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "empty string",
			input:    "",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "number only",
			input:    "100",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "negative duration",
			input:    "-1h",
			expected: -1 * time.Hour,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// ============================================================================
// RegisterRoutes Tests
// ============================================================================

func TestProfilerHandler_RegisterRoutes(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	group := router.Group("/api/v1")

	handler.RegisterRoutes(group)

	// Test that routes are registered
	routes := router.Routes()
	expectedRoutes := map[string]string{
		"/api/v1/profiler/cleanup":              "POST",
		"/api/v1/profiler/workloads/:uid/files": "GET",
		"/api/v1/profiler/stats":                "GET",
		"/api/v1/profiler/files/:id":            "GET",
		"/api/v1/profiler/files/:id/download":   "GET",
	}

	routeMap := make(map[string]string)
	for _, r := range routes {
		routeMap[r.Path] = r.Method
	}

	for path, method := range expectedRoutes {
		assert.Equal(t, method, routeMap[path], "Route %s should be %s", path, method)
	}
}

// ============================================================================
// TriggerCleanup Tests
// ============================================================================

func TestProfilerHandler_TriggerCleanup_NilLifecycleMgr(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.POST("/cleanup", handler.TriggerCleanup)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/cleanup", nil)
	router.ServeHTTP(w, req)

	// The implementation handles nil gracefully and returns success
	assert.Equal(t, http.StatusOK, w.Code)
}

// ============================================================================
// QueryFiles Tests
// ============================================================================

func TestProfilerHandler_QueryFiles(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.GET("/workloads/:uid/files", handler.QueryFiles)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/workloads/test-uid/files", nil)
	router.ServeHTTP(w, req)

	// Should return OK with stub response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "test-uid")
	assert.Contains(t, w.Body.String(), "not yet implemented")
}

func TestProfilerHandler_QueryFiles_WithParams(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.GET("/workloads/:uid/files", handler.QueryFiles)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/workloads/test-uid/files?file_type=chrome_trace&limit=10&offset=0", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ============================================================================
// GetStorageStats Tests
// ============================================================================

func TestProfilerHandler_GetStorageStats(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.GET("/stats", handler.GetStorageStats)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/stats", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "not yet implemented")
}

// ============================================================================
// GetFile Tests
// ============================================================================

func TestProfilerHandler_GetFile(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.GET("/files/:id", handler.GetFile)

	tests := []struct {
		name       string
		fileID     string
		statusCode int
	}{
		{
			name:       "valid file ID",
			fileID:     "123",
			statusCode: http.StatusOK,
		},
		{
			name:       "invalid file ID",
			fileID:     "invalid",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "negative file ID",
			fileID:     "-1",
			statusCode: http.StatusOK, // ParseInt handles negative numbers
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/files/"+tt.fileID, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
		})
	}
}

// ============================================================================
// GetDownloadURL Tests
// ============================================================================

func TestProfilerHandler_GetDownloadURL(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.GET("/files/:id/download", handler.GetDownloadURL)

	tests := []struct {
		name       string
		fileID     string
		expiresIn  string
		statusCode int
	}{
		{
			name:       "valid request",
			fileID:     "123",
			expiresIn:  "",
			statusCode: http.StatusOK,
		},
		{
			name:       "with expires_in",
			fileID:     "123",
			expiresIn:  "24h",
			statusCode: http.StatusOK,
		},
		{
			name:       "invalid file ID",
			fileID:     "invalid",
			expiresIn:  "",
			statusCode: http.StatusBadRequest,
		},
		{
			name:       "invalid expires_in",
			fileID:     "123",
			expiresIn:  "invalid",
			statusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			url := "/files/" + tt.fileID + "/download"
			if tt.expiresIn != "" {
				url += "?expires_in=" + tt.expiresIn
			}
			req := httptest.NewRequest("GET", url, nil)
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.statusCode, w.Code)
		})
	}
}

func TestProfilerHandler_GetDownloadURL_DefaultExpires(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.GET("/files/:id/download", handler.GetDownloadURL)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/files/123/download", nil)
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// Should include default expires_in value
	assert.Contains(t, w.Body.String(), "24h")
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestProfilerHandler_Integration(t *testing.T) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	group := router.Group("/api/v1")
	handler.RegisterRoutes(group)

	// Test all endpoints
	endpoints := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/profiler/cleanup"},
		{"GET", "/api/v1/profiler/workloads/test-uid/files"},
		{"GET", "/api/v1/profiler/stats"},
		{"GET", "/api/v1/profiler/files/123"},
		{"GET", "/api/v1/profiler/files/123/download"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(ep.method, ep.path, nil)
			router.ServeHTTP(w, req)

			// All endpoints should return either 200 or 500 (for nil managers)
			assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusInternalServerError,
				"Expected 200 or 500, got %d for %s %s", w.Code, ep.method, ep.path)
		})
	}
}

// ============================================================================
// Benchmark Tests
// ============================================================================

func BenchmarkParseIntQuery(b *testing.B) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?limit=100", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = parseIntQuery(c, "limit", 50)
	}
}

func BenchmarkParseDuration(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = parseDuration("24h")
	}
}

func BenchmarkProfilerHandler_QueryFiles(b *testing.B) {
	handler := NewProfilerHandler(nil, nil)
	router := gin.New()
	router.GET("/workloads/:uid/files", handler.QueryFiles)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/workloads/test-uid/files", nil)
		router.ServeHTTP(w, req)
	}
}
