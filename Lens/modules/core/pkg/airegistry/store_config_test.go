package airegistry

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigStore(t *testing.T) {
	configs := []StaticAgentConfig{
		{
			Name:     "agent-1",
			Endpoint: "http://localhost:8080",
			Topics:   []string{"alert.*"},
		},
		{
			Name:     "agent-2",
			Endpoint: "http://localhost:8081",
			Topics:   []string{"report.*"},
		},
	}

	store := NewConfigStore(configs)
	assert.NotNil(t, store)
	assert.Len(t, store.agents, 2)
}

func TestNewConfigStore_DefaultValues(t *testing.T) {
	configs := []StaticAgentConfig{
		{
			Name:     "agent-1",
			Endpoint: "http://localhost:8080",
			Topics:   []string{"alert.*"},
			// No HealthCheckPath or Timeout specified
		},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	agent, err := store.Get(ctx, "agent-1")
	require.NoError(t, err)
	assert.Equal(t, "/health", agent.HealthCheckPath)
	assert.Equal(t, 60*time.Second, agent.Timeout)
	assert.Equal(t, AgentStatusUnknown, agent.Status)
}

func TestConfigStore_Register_ExistingAgent(t *testing.T) {
	configs := []StaticAgentConfig{
		{
			Name:     "agent-1",
			Endpoint: "http://localhost:8080",
			Topics:   []string{"alert.*"},
		},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	// Try to register existing agent with status update
	err := store.Register(ctx, &AgentRegistration{
		Name:         "agent-1",
		Status:       AgentStatusHealthy,
		FailureCount: 0,
	})
	require.NoError(t, err)

	// Verify status was updated
	agent, _ := store.Get(ctx, "agent-1")
	assert.Equal(t, AgentStatusHealthy, agent.Status)
}

func TestConfigStore_Register_NewAgent(t *testing.T) {
	configs := []StaticAgentConfig{
		{
			Name:     "agent-1",
			Endpoint: "http://localhost:8080",
			Topics:   []string{"alert.*"},
		},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	// Try to register new agent (should fail)
	err := store.Register(ctx, &AgentRegistration{
		Name:     "agent-2",
		Endpoint: "http://localhost:8081",
		Topics:   []string{"report.*"},
	})
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestConfigStore_Unregister(t *testing.T) {
	configs := []StaticAgentConfig{
		{
			Name:     "agent-1",
			Endpoint: "http://localhost:8080",
			Topics:   []string{"alert.*"},
		},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	// Config store is read-only, unregister should fail
	err := store.Unregister(ctx, "agent-1")
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestConfigStore_Get(t *testing.T) {
	configs := []StaticAgentConfig{
		{
			Name:     "agent-1",
			Endpoint: "http://localhost:8080",
			Topics:   []string{"alert.*"},
		},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	agent, err := store.Get(ctx, "agent-1")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", agent.Name)
	assert.Equal(t, "http://localhost:8080", agent.Endpoint)
}

func TestConfigStore_Get_NotFound(t *testing.T) {
	store := NewConfigStore(nil)
	ctx := context.Background()

	_, err := store.Get(ctx, "nonexistent")
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestConfigStore_Get_ReturnsCopy(t *testing.T) {
	configs := []StaticAgentConfig{
		{
			Name:     "agent-1",
			Endpoint: "http://localhost:8080",
			Topics:   []string{"alert.*"},
		},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	result1, _ := store.Get(ctx, "agent-1")
	result1.Endpoint = "http://modified:8080"

	result2, _ := store.Get(ctx, "agent-1")
	assert.Equal(t, "http://localhost:8080", result2.Endpoint)
}

func TestConfigStore_List(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"topic1"}},
		{Name: "agent-2", Endpoint: "http://localhost:8081", Topics: []string{"topic2"}},
		{Name: "agent-3", Endpoint: "http://localhost:8082", Topics: []string{"topic3"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestConfigStore_ListByTopic(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
		{Name: "agent-2", Endpoint: "http://localhost:8081", Topics: []string{"report.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	list, err := store.ListByTopic(ctx, "alert.advisor.aggregate")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "agent-1", list[0].Name)
}

func TestConfigStore_UpdateStatus(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	err := store.UpdateStatus(ctx, "agent-1", AgentStatusHealthy, 0)
	require.NoError(t, err)

	agent, _ := store.Get(ctx, "agent-1")
	assert.Equal(t, AgentStatusHealthy, agent.Status)
	assert.False(t, agent.LastHealthCheck.IsZero())
}

func TestConfigStore_UpdateStatus_NotFound(t *testing.T) {
	store := NewConfigStore(nil)
	ctx := context.Background()

	err := store.UpdateStatus(ctx, "nonexistent", AgentStatusHealthy, 0)
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestConfigStore_GetHealthyAgentForTopic(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	// Update status to healthy
	_ = store.UpdateStatus(ctx, "agent-1", AgentStatusHealthy, 0)

	result, err := store.GetHealthyAgentForTopic(ctx, "alert.advisor.aggregate")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", result.Name)
}

func TestConfigStore_GetHealthyAgentForTopic_NoAgent(t *testing.T) {
	store := NewConfigStore(nil)
	ctx := context.Background()

	_, err := store.GetHealthyAgentForTopic(ctx, "unknown.topic")
	assert.Equal(t, ErrNoAgentForTopic, err)
}

func TestConfigStore_GetHealthyAgentForTopic_Unhealthy(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	_ = store.UpdateStatus(ctx, "agent-1", AgentStatusUnhealthy, 3)

	_, err := store.GetHealthyAgentForTopic(ctx, "alert.advisor.aggregate")
	assert.Equal(t, ErrAgentUnhealthy, err)
}

func TestConfigStore_GetHealthyAgentForTopic_UnknownStatus(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	// Status is initially unknown
	result, err := store.GetHealthyAgentForTopic(ctx, "alert.advisor.aggregate")
	require.NoError(t, err)
	assert.Equal(t, "agent-1", result.Name)
}

func TestConfigStore_Reload(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	// Update status
	_ = store.UpdateStatus(ctx, "agent-1", AgentStatusHealthy, 0)

	// Reload with new config
	newConfigs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:9090", Topics: []string{"alert.*", "report.*"}},
		{Name: "agent-2", Endpoint: "http://localhost:8081", Topics: []string{"scan.*"}},
	}

	store.Reload(newConfigs)

	// Verify new config
	list, _ := store.List(ctx)
	assert.Len(t, list, 2)

	// Verify agent-1 preserved status
	agent1, _ := store.Get(ctx, "agent-1")
	assert.Equal(t, "http://localhost:9090", agent1.Endpoint)
	assert.Equal(t, AgentStatusHealthy, agent1.Status) // Status preserved

	// Verify agent-2 has default status
	agent2, _ := store.Get(ctx, "agent-2")
	assert.Equal(t, AgentStatusUnknown, agent2.Status)
}

func TestConfigStore_Reload_PreservesStatus(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()

	// Set failure count
	_ = store.UpdateStatus(ctx, "agent-1", AgentStatusUnhealthy, 5)
	agent, _ := store.Get(ctx, "agent-1")
	lastCheck := agent.LastHealthCheck

	// Reload same config
	store.Reload(configs)

	// Verify status preserved
	agent, _ = store.Get(ctx, "agent-1")
	assert.Equal(t, AgentStatusUnhealthy, agent.Status)
	assert.Equal(t, 5, agent.FailureCount)
	assert.Equal(t, lastCheck, agent.LastHealthCheck)
}

func TestConfigStore_Concurrency(t *testing.T) {
	configs := []StaticAgentConfig{
		{Name: "agent-1", Endpoint: "http://localhost:8080", Topics: []string{"alert.*"}},
		{Name: "agent-2", Endpoint: "http://localhost:8081", Topics: []string{"report.*"}},
	}

	store := NewConfigStore(configs)
	ctx := context.Background()
	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = store.List(ctx)
			_, _ = store.Get(ctx, "agent-1")
			_, _ = store.ListByTopic(ctx, "alert.advisor.aggregate")
			_, _ = store.GetHealthyAgentForTopic(ctx, "alert.advisor.aggregate")
		}()
	}

	// Concurrent status updates
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agentName := "agent-1"
			if idx%2 == 0 {
				agentName = "agent-2"
			}
			_ = store.UpdateStatus(ctx, agentName, AgentStatusHealthy, 0)
		}(i)
	}

	wg.Wait()
}

func TestConfigStore_ImplementsRegistry(t *testing.T) {
	var _ Registry = (*ConfigStore)(nil)
}
