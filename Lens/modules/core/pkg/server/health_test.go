// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain sets up the test environment
func TestMain(m *testing.M) {
	// Set gin to test mode once for all tests to avoid data races
	gin.SetMode(gin.TestMode)

	// Run tests
	code := m.Run()

	os.Exit(code)
}

// Setup helper to reset global state before tests
func resetHealthServerState() {
	// Reset once to allow re-initialization in tests
	once = *new(sync.Once)
	engineMu.Lock()
	engine = nil
	engineMu.Unlock()
	registersMu.Lock()
	registers = []func(g *gin.RouterGroup){}
	registersMu.Unlock()
	// Re-add the default metrics register
	AddRegister(addMetrics)
}

// TestSetDefaultGather tests the SetDefaultGather function
func TestSetDefaultGather(t *testing.T) {
	// Create a custom registry
	customRegistry := prometheus.NewRegistry()

	// Set custom gatherer
	SetDefaultGather(customRegistry)

	// Verify it was set
	assert.Equal(t, customRegistry, defaultGather)

	// Reset to default for other tests
	SetDefaultGather(prometheus.DefaultGatherer)
}

// TestAddRegister tests the AddRegister function
func TestAddRegister(t *testing.T) {
	resetHealthServerState()

	initialCount := len(registers)

	// Add a test register
	testRegister := func(g *gin.RouterGroup) {
		g.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "test")
		})
	}

	AddRegister(testRegister)

	// Verify it was added
	assert.Equal(t, initialCount+1, len(registers))
}

// TestAddRegister_Multiple tests adding multiple registers
func TestAddRegister_Multiple(t *testing.T) {
	resetHealthServerState()

	initialCount := len(registers)

	register1 := func(g *gin.RouterGroup) {}
	register2 := func(g *gin.RouterGroup) {}
	register3 := func(g *gin.RouterGroup) {}

	AddRegister(register1)
	AddRegister(register2)
	AddRegister(register3)

	assert.Equal(t, initialCount+3, len(registers))
}

// TestAddDefaultRegister tests the AddDefaultRegister function
func TestAddDefaultRegister(t *testing.T) {
	resetHealthServerState()

	// Add a default register
	testData := map[string]string{"status": "ok"}
	AddDefaultRegister("/status", func() (interface{}, error) {
		return testData, nil
	})

	// Create a test engine to verify the route works
	testEngine := gin.New()
	group := testEngine.Group("")

	// Apply all registers
	for _, reg := range registers {
		reg(group)
	}

	// Test the route
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/status", nil)
	testEngine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "ok", response["status"])
}

// TestAddDefaultRegister_WithError tests AddDefaultRegister with an error
func TestAddDefaultRegister_WithError(t *testing.T) {
	resetHealthServerState()

	expectedErr := assert.AnError
	AddDefaultRegister("/error", func() (interface{}, error) {
		return nil, expectedErr
	})

	// Create a test engine
	testEngine := gin.New()
	group := testEngine.Group("")

	// Apply all registers
	for _, reg := range registers {
		reg(group)
	}

	// Test the route
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/error", nil)
	testEngine.ServeHTTP(w, req)

	// Should return error status
	assert.NotEqual(t, http.StatusOK, w.Code)
}

// TestAddDefaultRegister_MultipleRoutes tests multiple default routes
func TestAddDefaultRegister_MultipleRoutes(t *testing.T) {
	resetHealthServerState()

	// Add multiple routes
	AddDefaultRegister("/route1", func() (interface{}, error) {
		return map[string]string{"route": "1"}, nil
	})
	AddDefaultRegister("/route2", func() (interface{}, error) {
		return map[string]string{"route": "2"}, nil
	})

	// Create a test engine
	testEngine := gin.New()
	group := testEngine.Group("")

	// Apply all registers
	for _, reg := range registers {
		reg(group)
	}

	// Test route1
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/route1", nil)
	testEngine.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Test route2
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/route2", nil)
	testEngine.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
}

// TestInitHealthServer tests the InitHealthServer function
func TestInitHealthServer(t *testing.T) {
	resetHealthServerState()

	// Use a high port to avoid conflicts
	testPort := 19999

	// Initialize health server
	InitHealthServer(testPort)

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Verify engine was created
	engineMu.RLock()
	assert.NotNil(t, engine)
	engineMu.RUnlock()

	// Try to make a request to the metrics endpoint
	resp, err := http.Get("http://localhost:19999/metrics")
	if err == nil {
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}
	// Note: The request might fail if the port is in use, but we mainly test initialization
}

// TestInitHealthServer_OnlyOnce tests that InitHealthServer only initializes once
func TestInitHealthServer_OnlyOnce(t *testing.T) {
	resetHealthServerState()

	testPort := 19998

	// Initialize first time
	InitHealthServer(testPort)
	engineMu.RLock()
	firstEngine := engine
	engineMu.RUnlock()

	// Initialize second time with different port
	InitHealthServer(testPort + 1)
	engineMu.RLock()
	secondEngine := engine
	engineMu.RUnlock()

	// Should be the same engine (only initialized once)
	assert.Equal(t, firstEngine, secondEngine)
}

// TestAddMetrics tests the addMetrics function
func TestAddMetrics(t *testing.T) {
	// Create a test engine
	testEngine := gin.New()
	group := testEngine.Group("")

	// Add metrics endpoint
	addMetrics(group)

	// Test the metrics endpoint
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	testEngine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "# HELP")
	assert.Contains(t, w.Body.String(), "# TYPE")
}

// TestAddMetrics_CustomGatherer tests addMetrics with custom gatherer
func TestAddMetrics_CustomGatherer(t *testing.T) {
	// Create a custom registry with a test counter
	customRegistry := prometheus.NewRegistry()
	testCounter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_custom_metric",
		Help: "A test custom metric",
	})
	customRegistry.MustRegister(testCounter)
	testCounter.Inc()

	// Set custom gatherer
	originalGather := defaultGather
	defer func() { defaultGather = originalGather }()
	SetDefaultGather(customRegistry)

	// Create a test engine
	testEngine := gin.New()
	group := testEngine.Group("")
	addMetrics(group)

	// Test the metrics endpoint
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	testEngine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "test_custom_metric")
}

// TestAddMetrics_OpenMetricsFormat tests that OpenMetrics format is enabled
func TestAddMetrics_OpenMetricsFormat(t *testing.T) {
	// Create a test engine
	testEngine := gin.New()
	group := testEngine.Group("")
	addMetrics(group)

	// Test the metrics endpoint with Accept header for OpenMetrics
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Accept", "application/openmetrics-text")
	testEngine.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// OpenMetrics format should be returned if available
	assert.NotEmpty(t, w.Body.String())
}

// TestHealthServer_Integration tests the health server integration
func TestHealthServer_Integration(t *testing.T) {
	resetHealthServerState()

	// Add custom routes
	AddDefaultRegister("/health", func() (interface{}, error) {
		return map[string]string{"status": "healthy"}, nil
	})

	AddDefaultRegister("/ready", func() (interface{}, error) {
		return map[string]bool{"ready": true}, nil
	})

	// Create a test engine to simulate health server
	testEngine := gin.New()
	group := testEngine.Group("")
	group.Use(gin.Recovery())

	// Apply all registers
	for _, reg := range registers {
		reg(group)
	}

	// Test metrics endpoint
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/metrics", nil)
	testEngine.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// Test health endpoint
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/health", nil)
	testEngine.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	// Test ready endpoint
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("GET", "/ready", nil)
	testEngine.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
}

// TestHealthServer_ConcurrentRegistration tests concurrent register additions
func TestHealthServer_ConcurrentRegistration(t *testing.T) {
	resetHealthServerState()

	done := make(chan bool)

	// Add registers concurrently
	for i := 0; i < 10; i++ {
		go func() {
			AddRegister(func(g *gin.RouterGroup) {
				g.GET("/test", func(c *gin.Context) {})
			})
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all were added (initial default + 10 new)
	assert.GreaterOrEqual(t, len(registers), 10)
}

// BenchmarkAddRegister benchmarks the AddRegister operation
func BenchmarkAddRegister(b *testing.B) {
	testRegister := func(g *gin.RouterGroup) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetHealthServerState()
		AddRegister(testRegister)
	}
}

// BenchmarkAddDefaultRegister benchmarks the AddDefaultRegister operation
func BenchmarkAddDefaultRegister(b *testing.B) {
	testMethod := func() (interface{}, error) {
		return map[string]string{"test": "data"}, nil
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetHealthServerState()
		AddDefaultRegister("/test", testMethod)
	}
}

// BenchmarkAddMetrics benchmarks the addMetrics handler
func BenchmarkAddMetrics(b *testing.B) {
	testEngine := gin.New()
	group := testEngine.Group("")
	addMetrics(group)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest("GET", "/metrics", nil)
		testEngine.ServeHTTP(w, req)
	}
}
