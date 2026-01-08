package airegistry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRouter(t *testing.T) {
	registry := NewMemoryStore()
	router := NewRouter(registry)

	assert.NotNil(t, router)
	assert.NotNil(t, router.cache)
	assert.True(t, router.cache.enabled)
}

func TestRouter_Route_NoAgent(t *testing.T) {
	registry := NewMemoryStore()
	router := NewRouter(registry)

	_, err := router.Route(context.Background(), "unknown.topic")
	assert.Equal(t, ErrNoAgentForTopic, err)
}

func TestRouter_Route_Success(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	// Register an agent
	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	// Route to the topic
	result, err := router.Route(ctx, "test.topic")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", result.Name)
}

func TestRouter_Route_UsesCache(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	// Register an agent
	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	// First route - populates cache
	result1, err := router.Route(ctx, "test.topic")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", result1.Name)

	// Remove from registry
	err = registry.Unregister(ctx, "test-agent")
	require.NoError(t, err)

	// Second route - should still return from cache
	result2, err := router.Route(ctx, "test.topic")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", result2.Name)
}

func TestRouter_InvalidateCache(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	// Register and route
	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	_, err = router.Route(ctx, "test.topic")
	require.NoError(t, err)

	// Invalidate cache
	router.InvalidateCache()

	// Unregister
	err = registry.Unregister(ctx, "test-agent")
	require.NoError(t, err)

	// Now should fail
	_, err = router.Route(ctx, "test.topic")
	assert.Equal(t, ErrNoAgentForTopic, err)
}

func TestRouter_InvalidateCacheForTopic(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	// Register agents for multiple topics
	agent1 := &AgentRegistration{
		Name:     "agent-1",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1"},
		Status:   AgentStatusHealthy,
	}
	agent2 := &AgentRegistration{
		Name:     "agent-2",
		Endpoint: "http://localhost:8081",
		Topics:   []string{"topic2"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent1)
	require.NoError(t, err)
	err = registry.Register(ctx, agent2)
	require.NoError(t, err)

	// Route to both topics to populate cache
	_, err = router.Route(ctx, "topic1")
	require.NoError(t, err)
	_, err = router.Route(ctx, "topic2")
	require.NoError(t, err)

	// Invalidate only topic1
	router.InvalidateCacheForTopic("topic1")

	// topic2 should still be cached
	assert.NotNil(t, router.cache.get("topic2"))
	// topic1 should be gone
	assert.Nil(t, router.cache.get("topic1"))
}

func TestRouter_InvalidateCacheForAgent(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	// Register an agent handling multiple topics
	agent := &AgentRegistration{
		Name:     "multi-topic-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1", "topic2", "topic3"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	// Route to all topics
	_, _ = router.Route(ctx, "topic1")
	_, _ = router.Route(ctx, "topic2")
	_, _ = router.Route(ctx, "topic3")

	// Invalidate by agent name
	router.InvalidateCacheForAgent("multi-topic-agent")

	// All should be cleared
	assert.Nil(t, router.cache.get("topic1"))
	assert.Nil(t, router.cache.get("topic2"))
	assert.Nil(t, router.cache.get("topic3"))
}

func TestRouter_DisableCache(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	// Disable cache
	router.DisableCache()

	// Route
	_, err = router.Route(ctx, "test.topic")
	require.NoError(t, err)

	// Cache should be empty even after route
	assert.Nil(t, router.cache.get("test.topic"))
}

func TestRouter_EnableCache(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	router.DisableCache()
	router.EnableCache()

	// Route
	_, err = router.Route(ctx, "test.topic")
	require.NoError(t, err)

	// Cache should now have the entry
	assert.NotNil(t, router.cache.get("test.topic"))
}

func TestRouter_RouteAll(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	// Register multiple agents for same topic pattern
	agent1 := &AgentRegistration{
		Name:     "agent-1",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.*"},
		Status:   AgentStatusHealthy,
	}
	agent2 := &AgentRegistration{
		Name:     "agent-2",
		Endpoint: "http://localhost:8081",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent1)
	require.NoError(t, err)
	err = registry.Register(ctx, agent2)
	require.NoError(t, err)

	// Get all agents for topic
	agents, err := router.RouteAll(ctx, "test.topic")
	require.NoError(t, err)
	assert.Len(t, agents, 2)
}

func TestRouter_Route_UnhealthyCacheEntry(t *testing.T) {
	ctx := context.Background()
	registry := NewMemoryStore()
	router := NewRouter(registry)

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"test.topic"},
		Status:   AgentStatusHealthy,
	}
	err := registry.Register(ctx, agent)
	require.NoError(t, err)

	// Route to populate cache
	_, err = router.Route(ctx, "test.topic")
	require.NoError(t, err)

	// Manually set cached entry to unhealthy
	router.cache.mu.Lock()
	router.cache.routes["test.topic"].Status = AgentStatusUnhealthy
	router.cache.mu.Unlock()

	// Route should re-fetch from registry
	result, err := router.Route(ctx, "test.topic")
	require.NoError(t, err)
	assert.Equal(t, AgentStatusHealthy, result.Status)
}

func TestTopicMatcher_Match(t *testing.T) {
	matcher := &TopicMatcher{}

	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		{"alert.advisor.aggregate", "alert.advisor.aggregate", true},
		{"alert.advisor.*", "alert.advisor.aggregate", true},
		{"alert.advisor.*", "alert.advisor.generate", true},
		{"alert.*", "alert.advisor.aggregate", true},
		{"alert.*", "alert.handler.analyze", true},
		{"report.*", "alert.advisor.aggregate", false},
		{"alert.advisor.aggregate", "alert.advisor.generate", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			got := matcher.Match(tt.pattern, tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTopicMatcher_MatchAny(t *testing.T) {
	matcher := &TopicMatcher{}

	patterns := []string{"alert.*", "report.*"}

	assert.True(t, matcher.MatchAny(patterns, "alert.advisor.aggregate"))
	assert.True(t, matcher.MatchAny(patterns, "report.summary"))
	assert.False(t, matcher.MatchAny(patterns, "scan.identify"))
}

func TestExtractDomain(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"alert.advisor.aggregate", "alert"},
		{"report.summary", "report"},
		{"scan", "scan"},
		{"a.b.c.d.e", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := ExtractDomain(tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractAgent(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"alert.advisor.aggregate", "advisor"},
		{"report.generator.summary", "generator"},
		{"scan.identify", "identify"},
		{"single", ""},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := ExtractAgent(tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestExtractAction(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"alert.advisor.aggregate", "aggregate"},
		{"alert.advisor.generate-suggestions", "generate-suggestions"},
		{"report.summary", ""},
		{"single", ""},
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			got := ExtractAction(tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRouteCache(t *testing.T) {
	cache := &routeCache{
		routes:  make(map[string]*AgentRegistration),
		enabled: true,
	}

	// Test set and get
	agent := &AgentRegistration{Name: "agent-1"}
	cache.set("topic1", agent)
	assert.NotNil(t, cache.get("topic1"))
	assert.Equal(t, "agent-1", cache.get("topic1").Name)

	// Test remove
	cache.remove("topic1")
	assert.Nil(t, cache.get("topic1"))

	// Test clear
	cache.set("topic2", agent)
	cache.set("topic3", agent)
	cache.clear()
	assert.Nil(t, cache.get("topic2"))
	assert.Nil(t, cache.get("topic3"))
}

func TestRouteCache_Disabled(t *testing.T) {
	cache := &routeCache{
		routes:  make(map[string]*AgentRegistration),
		enabled: false,
	}

	agent := &AgentRegistration{Name: "agent-1"}
	cache.set("topic1", agent)
	
	// Should not store when disabled
	assert.Nil(t, cache.get("topic1"))
}

