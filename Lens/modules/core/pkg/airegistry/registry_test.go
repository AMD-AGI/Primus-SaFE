package airegistry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentStatus_Constants(t *testing.T) {
	assert.Equal(t, AgentStatus("healthy"), AgentStatusHealthy)
	assert.Equal(t, AgentStatus("unhealthy"), AgentStatusUnhealthy)
	assert.Equal(t, AgentStatus("unknown"), AgentStatusUnknown)
}

func TestAgentRegistration_TableName(t *testing.T) {
	agent := AgentRegistration{}
	assert.Equal(t, "ai_agent_registrations", agent.TableName())
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.NotNil(t, cfg)
	assert.Equal(t, "memory", cfg.Mode)
	assert.Equal(t, 30*time.Second, cfg.HealthCheckInterval)
	assert.Equal(t, 3, cfg.UnhealthyThreshold)
}

func TestNewRegistry_Memory(t *testing.T) {
	cfg := &RegistryConfig{
		Mode: "memory",
	}

	registry, err := NewRegistry(cfg)
	require.NoError(t, err)
	assert.NotNil(t, registry)
	_, ok := registry.(*MemoryStore)
	assert.True(t, ok)
}

func TestNewRegistry_Config(t *testing.T) {
	cfg := &RegistryConfig{
		Mode: "config",
		StaticAgents: []StaticAgentConfig{
			{
				Name:     "test-agent",
				Endpoint: "http://localhost:8080",
				Topics:   []string{"test.topic"},
			},
		},
	}

	registry, err := NewRegistry(cfg)
	require.NoError(t, err)
	assert.NotNil(t, registry)
	_, ok := registry.(*ConfigStore)
	assert.True(t, ok)
}

func TestNewRegistry_DB(t *testing.T) {
	cfg := &RegistryConfig{
		Mode: "db",
	}

	registry, err := NewRegistry(cfg)
	assert.Error(t, err)
	assert.Nil(t, registry)
	assert.Contains(t, err.Error(), "db mode")
}

func TestNewRegistry_Hybrid(t *testing.T) {
	cfg := &RegistryConfig{
		Mode: "hybrid",
	}

	registry, err := NewRegistry(cfg)
	assert.Error(t, err)
	assert.Nil(t, registry)
	assert.Contains(t, err.Error(), "hybrid mode")
}

func TestNewRegistry_Default(t *testing.T) {
	cfg := &RegistryConfig{
		Mode: "unknown-mode",
	}

	registry, err := NewRegistry(cfg)
	require.NoError(t, err)
	assert.NotNil(t, registry)
	// Should default to memory store
	_, ok := registry.(*MemoryStore)
	assert.True(t, ok)
}

func TestNewRegistry_NilConfig(t *testing.T) {
	registry, err := NewRegistry(nil)
	require.NoError(t, err)
	assert.NotNil(t, registry)
}

func TestErrors(t *testing.T) {
	assert.NotNil(t, ErrAgentNotFound)
	assert.NotNil(t, ErrAgentAlreadyExists)
	assert.NotNil(t, ErrNoAgentForTopic)
	assert.NotNil(t, ErrAgentUnhealthy)

	assert.Contains(t, ErrAgentNotFound.Error(), "not found")
	assert.Contains(t, ErrAgentAlreadyExists.Error(), "already exists")
	assert.Contains(t, ErrNoAgentForTopic.Error(), "topic")
	assert.Contains(t, ErrAgentUnhealthy.Error(), "unhealthy")
}

func TestStaticAgentConfig(t *testing.T) {
	cfg := StaticAgentConfig{
		Name:            "test-agent",
		Endpoint:        "http://localhost:8080",
		Topics:          []string{"alert.*", "report.*"},
		Timeout:         60 * time.Second,
		HealthCheckPath: "/healthz",
	}

	assert.Equal(t, "test-agent", cfg.Name)
	assert.Equal(t, "http://localhost:8080", cfg.Endpoint)
	assert.Len(t, cfg.Topics, 2)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.Equal(t, "/healthz", cfg.HealthCheckPath)
}

func TestRegistryConfig(t *testing.T) {
	cfg := &RegistryConfig{
		Mode:                "config",
		DatabaseDSN:         "postgres://localhost/test",
		ConfigFile:          "/etc/agents.yaml",
		HealthCheckInterval: 60 * time.Second,
		UnhealthyThreshold:  5,
		StaticAgents: []StaticAgentConfig{
			{Name: "agent1"},
			{Name: "agent2"},
		},
	}

	assert.Equal(t, "config", cfg.Mode)
	assert.Equal(t, "postgres://localhost/test", cfg.DatabaseDSN)
	assert.Equal(t, "/etc/agents.yaml", cfg.ConfigFile)
	assert.Equal(t, 60*time.Second, cfg.HealthCheckInterval)
	assert.Equal(t, 5, cfg.UnhealthyThreshold)
	assert.Len(t, cfg.StaticAgents, 2)
}

func TestAgentRegistration_Fields(t *testing.T) {
	now := time.Now()
	agent := &AgentRegistration{
		Name:            "test-agent",
		Endpoint:        "http://localhost:8080",
		Topics:          []string{"topic1", "topic2"},
		TopicsJSON:      `["topic1","topic2"]`,
		HealthCheckPath: "/health",
		Timeout:         30 * time.Second,
		TimeoutSeconds:  30,
		Status:          AgentStatusHealthy,
		LastHealthCheck: now,
		FailureCount:    0,
		Metadata:        map[string]string{"version": "1.0"},
		MetadataJSON:    `{"version":"1.0"}`,
		RegisteredAt:    now,
		UpdatedAt:       now,
	}

	assert.Equal(t, "test-agent", agent.Name)
	assert.Equal(t, "http://localhost:8080", agent.Endpoint)
	assert.Len(t, agent.Topics, 2)
	assert.Equal(t, "/health", agent.HealthCheckPath)
	assert.Equal(t, 30*time.Second, agent.Timeout)
	assert.Equal(t, AgentStatusHealthy, agent.Status)
	assert.Equal(t, 0, agent.FailureCount)
	assert.Equal(t, "1.0", agent.Metadata["version"])
}
