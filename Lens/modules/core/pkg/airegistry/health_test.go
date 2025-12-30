package airegistry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHealthChecker(t *testing.T) {
	registry := NewMemoryStore()

	t.Run("with defaults", func(t *testing.T) {
		hc := NewHealthChecker(registry, 0, 0)
		assert.NotNil(t, hc)
		assert.NotNil(t, hc.client)
		assert.Equal(t, 3, hc.unhealthyThreshold)
	})

	t.Run("with custom values", func(t *testing.T) {
		hc := NewHealthChecker(registry, 10*time.Second, 5)
		assert.NotNil(t, hc)
		assert.Equal(t, 5, hc.unhealthyThreshold)
	})
}

func TestHealthChecker_Check_Healthy(t *testing.T) {
	// Create a test server that returns 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	registry := NewMemoryStore()
	ctx := context.Background()

	// Register agent with server endpoint
	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: server.URL,
		Topics:   []string{"test.topic"},
		Status:   AgentStatusUnknown,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	hc := NewHealthChecker(registry, 5*time.Second, 3)
	result := hc.Check(ctx, agent)

	assert.True(t, result.Healthy)
	assert.Nil(t, result.Error)
	assert.Equal(t, "test-agent", result.AgentName)
	assert.True(t, result.Duration > 0)

	// Verify status was updated
	updatedAgent, _ := registry.Get(ctx, "test-agent")
	assert.Equal(t, AgentStatusHealthy, updatedAgent.Status)
	assert.Equal(t, 0, updatedAgent.FailureCount)
}

func TestHealthChecker_Check_Unhealthy(t *testing.T) {
	// Create a test server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	registry := NewMemoryStore()
	ctx := context.Background()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: server.URL,
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	hc := NewHealthChecker(registry, 5*time.Second, 3)
	result := hc.Check(ctx, agent)

	assert.False(t, result.Healthy)
	assert.Equal(t, "test-agent", result.AgentName)
}

func TestHealthChecker_Check_ConnectionError(t *testing.T) {
	registry := NewMemoryStore()
	ctx := context.Background()

	// Use an endpoint that doesn't exist
	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:99999",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	hc := NewHealthChecker(registry, 1*time.Second, 3)
	result := hc.Check(ctx, agent)

	assert.False(t, result.Healthy)
	assert.NotNil(t, result.Error)
}

func TestHealthChecker_Check_CustomHealthPath(t *testing.T) {
	// Create a test server that returns 200 on custom path
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	registry := NewMemoryStore()
	ctx := context.Background()

	agent := &AgentRegistration{
		Name:            "test-agent",
		Endpoint:        server.URL,
		HealthCheckPath: "/healthz",
		Topics:          []string{"test.topic"},
		Status:          AgentStatusUnknown,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	hc := NewHealthChecker(registry, 5*time.Second, 3)
	result := hc.Check(ctx, agent)

	assert.True(t, result.Healthy)
}

func TestHealthChecker_Check_ThresholdTracking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	registry := NewMemoryStore()
	ctx := context.Background()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: server.URL,
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	hc := NewHealthChecker(registry, 5*time.Second, 3)

	// First failure - should not mark unhealthy yet
	_ = hc.Check(ctx, agent)
	updatedAgent, _ := registry.Get(ctx, "test-agent")
	assert.Equal(t, 1, updatedAgent.FailureCount)
	assert.Equal(t, AgentStatusHealthy, updatedAgent.Status) // Still healthy

	// Second failure
	agent.FailureCount = 1
	_ = hc.Check(ctx, agent)
	updatedAgent, _ = registry.Get(ctx, "test-agent")
	assert.Equal(t, 2, updatedAgent.FailureCount)

	// Third failure - should mark unhealthy
	agent.FailureCount = 2
	_ = hc.Check(ctx, agent)
	updatedAgent, _ = registry.Get(ctx, "test-agent")
	assert.Equal(t, 3, updatedAgent.FailureCount)
	assert.Equal(t, AgentStatusUnhealthy, updatedAgent.Status)
}

func TestHealthChecker_CheckAll(t *testing.T) {
	// Create test servers
	healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer healthyServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer unhealthyServer.Close()

	registry := NewMemoryStore()
	ctx := context.Background()

	// Register both agents
	err := registry.Register(ctx, &AgentRegistration{
		Name:     "healthy-agent",
		Endpoint: healthyServer.URL,
		Topics:   []string{"topic1"},
	})
	require.NoError(t, err)

	err = registry.Register(ctx, &AgentRegistration{
		Name:     "unhealthy-agent",
		Endpoint: unhealthyServer.URL,
		Topics:   []string{"topic2"},
	})
	require.NoError(t, err)

	hc := NewHealthChecker(registry, 5*time.Second, 3)
	results := hc.CheckAll(ctx)

	assert.Len(t, results, 2)

	healthyCount := 0
	unhealthyCount := 0
	for _, r := range results {
		if r.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	assert.Equal(t, 1, healthyCount)
	assert.Equal(t, 1, unhealthyCount)
}

func TestHealthChecker_CheckAll_EmptyRegistry(t *testing.T) {
	registry := NewMemoryStore()
	hc := NewHealthChecker(registry, 5*time.Second, 3)

	results := hc.CheckAll(context.Background())
	assert.Empty(t, results)
}

func TestIsHealthy(t *testing.T) {
	assert.True(t, IsHealthy(AgentStatusHealthy))
	assert.False(t, IsHealthy(AgentStatusUnhealthy))
	assert.False(t, IsHealthy(AgentStatusUnknown))
	assert.False(t, IsHealthy(AgentStatus("")))
}

func TestShouldRetry(t *testing.T) {
	assert.True(t, ShouldRetry(AgentStatusHealthy))
	assert.True(t, ShouldRetry(AgentStatusUnknown))
	assert.False(t, ShouldRetry(AgentStatusUnhealthy))
}

func TestHealthCheckResult(t *testing.T) {
	result := HealthCheckResult{
		AgentName: "test-agent",
		Healthy:   true,
		Error:     nil,
		Duration:  100 * time.Millisecond,
	}

	assert.Equal(t, "test-agent", result.AgentName)
	assert.True(t, result.Healthy)
	assert.Nil(t, result.Error)
	assert.Equal(t, 100*time.Millisecond, result.Duration)
}

func TestHealthChecker_Check_StatusCodes(t *testing.T) {
	tests := []struct {
		statusCode int
		healthy    bool
	}{
		{200, true},
		{201, true},
		{204, true},
		{299, true},
		{300, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{500, false},
		{502, false},
		{503, false},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			registry := NewMemoryStore()
			ctx := context.Background()

			agent := &AgentRegistration{
				Name:     "test-agent",
				Endpoint: server.URL,
				Topics:   []string{"test.topic"},
			}
			_ = registry.Register(ctx, agent)

			hc := NewHealthChecker(registry, 5*time.Second, 3)
			result := hc.Check(ctx, agent)

			assert.Equal(t, tt.healthy, result.Healthy)
		})
	}
}

func TestHealthChecker_recordResult_UnknownStatus(t *testing.T) {
	registry := NewMemoryStore()
	ctx := context.Background()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   "", // Empty status
	}
	_ = registry.Register(ctx, agent)

	hc := NewHealthChecker(registry, 5*time.Second, 3)
	
	// Simulate a failure with empty status
	result := hc.recordResult(ctx, agent, false, nil, 100*time.Millisecond)

	assert.False(t, result.Healthy)
	
	updatedAgent, _ := registry.Get(ctx, "test-agent")
	assert.Equal(t, AgentStatusUnknown, updatedAgent.Status)
}

