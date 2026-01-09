// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package airegistry

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMemoryStore(t *testing.T) {
	store := NewMemoryStore()
	assert.NotNil(t, store)
	assert.NotNil(t, store.agents)
}

func TestMemoryStore_Register(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1", "topic2"},
		Metadata: map[string]string{"version": "1.0"},
	}

	err := store.Register(ctx, agent)
	require.NoError(t, err)

	// Verify registered
	result, err := store.Get(ctx, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", result.Name)
	assert.Equal(t, "http://localhost:8080", result.Endpoint)
	assert.Len(t, result.Topics, 2)
	assert.Equal(t, AgentStatusUnknown, result.Status)
	assert.False(t, result.RegisteredAt.IsZero())
	assert.False(t, result.UpdatedAt.IsZero())
}

func TestMemoryStore_Register_Update(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// First registration
	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1"},
	}
	err := store.Register(ctx, agent)
	require.NoError(t, err)

	result1, _ := store.Get(ctx, "test-agent")
	firstRegisteredAt := result1.RegisteredAt

	time.Sleep(10 * time.Millisecond)

	// Update registration
	agent.Endpoint = "http://localhost:9090"
	agent.Topics = []string{"topic1", "topic2"}
	err = store.Register(ctx, agent)
	require.NoError(t, err)

	// Verify updated
	result2, _ := store.Get(ctx, "test-agent")
	assert.Equal(t, "http://localhost:9090", result2.Endpoint)
	assert.Len(t, result2.Topics, 2)
	// RegisteredAt should be preserved
	assert.Equal(t, firstRegisteredAt, result2.RegisteredAt)
	// UpdatedAt should be newer
	assert.True(t, result2.UpdatedAt.After(result2.RegisteredAt))
}

func TestMemoryStore_Register_DeepCopy(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1"},
		Metadata: map[string]string{"key": "value"},
	}
	err := store.Register(ctx, agent)
	require.NoError(t, err)

	// Modify original
	agent.Topics = append(agent.Topics, "topic2")
	agent.Metadata["key"] = "modified"

	// Stored should not be affected
	result, _ := store.Get(ctx, "test-agent")
	assert.Len(t, result.Topics, 1)
	assert.Equal(t, "value", result.Metadata["key"])
}

func TestMemoryStore_Unregister(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1"},
	}
	_ = store.Register(ctx, agent)

	// Unregister
	err := store.Unregister(ctx, "test-agent")
	require.NoError(t, err)

	// Verify gone
	_, err = store.Get(ctx, "test-agent")
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestMemoryStore_Unregister_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	err := store.Unregister(ctx, "nonexistent")
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestMemoryStore_Get(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1"},
	}
	_ = store.Register(ctx, agent)

	result, err := store.Get(ctx, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, "test-agent", result.Name)
}

func TestMemoryStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_, err := store.Get(ctx, "nonexistent")
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestMemoryStore_Get_ReturnsCopy(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	agent := &AgentRegistration{
		Name:     "test-agent",
		Endpoint: "http://localhost:8080",
		Topics:   []string{"topic1"},
	}
	_ = store.Register(ctx, agent)

	result1, _ := store.Get(ctx, "test-agent")
	result1.Endpoint = "http://modified:8080"

	result2, _ := store.Get(ctx, "test-agent")
	assert.Equal(t, "http://localhost:8080", result2.Endpoint)
}

func TestMemoryStore_List(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// Empty list
	list, err := store.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)

	// Register agents
	for i := 0; i < 3; i++ {
		agent := &AgentRegistration{
			Name:     "agent-" + string(rune('0'+i)),
			Endpoint: "http://localhost:8080",
			Topics:   []string{"topic"},
		}
		_ = store.Register(ctx, agent)
	}

	list, err = store.List(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 3)
}

func TestMemoryStore_ListByTopic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// Register agents with different topics
	_ = store.Register(ctx, &AgentRegistration{
		Name:   "agent-1",
		Topics: []string{"alert.advisor.*"},
		Status: AgentStatusHealthy,
	})
	_ = store.Register(ctx, &AgentRegistration{
		Name:   "agent-2",
		Topics: []string{"alert.handler.*"},
		Status: AgentStatusHealthy,
	})
	_ = store.Register(ctx, &AgentRegistration{
		Name:   "agent-3",
		Topics: []string{"report.*"},
		Status: AgentStatusHealthy,
	})

	// List by topic
	list, err := store.ListByTopic(ctx, "alert.advisor.aggregate")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "agent-1", list[0].Name)

	// List by different topic
	list, err = store.ListByTopic(ctx, "report.summary")
	require.NoError(t, err)
	assert.Len(t, list, 1)
	assert.Equal(t, "agent-3", list[0].Name)

	// No matches
	list, err = store.ListByTopic(ctx, "scan.identify")
	require.NoError(t, err)
	assert.Empty(t, list)
}

func TestMemoryStore_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	agent := &AgentRegistration{
		Name:   "test-agent",
		Topics: []string{"topic"},
		Status: AgentStatusUnknown,
	}
	_ = store.Register(ctx, agent)

	// Update status
	err := store.UpdateStatus(ctx, "test-agent", AgentStatusHealthy, 0)
	require.NoError(t, err)

	result, _ := store.Get(ctx, "test-agent")
	assert.Equal(t, AgentStatusHealthy, result.Status)
	assert.Equal(t, 0, result.FailureCount)
	assert.False(t, result.LastHealthCheck.IsZero())
}

func TestMemoryStore_UpdateStatus_NotFound(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	err := store.UpdateStatus(ctx, "nonexistent", AgentStatusHealthy, 0)
	assert.Equal(t, ErrAgentNotFound, err)
}

func TestMemoryStore_GetHealthyAgentForTopic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	// Register healthy agent
	_ = store.Register(ctx, &AgentRegistration{
		Name:   "healthy-agent",
		Topics: []string{"test.topic"},
		Status: AgentStatusHealthy,
	})

	result, err := store.GetHealthyAgentForTopic(ctx, "test.topic")
	require.NoError(t, err)
	assert.Equal(t, "healthy-agent", result.Name)
}

func TestMemoryStore_GetHealthyAgentForTopic_NoAgent(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_, err := store.GetHealthyAgentForTopic(ctx, "test.topic")
	assert.Equal(t, ErrNoAgentForTopic, err)
}

func TestMemoryStore_GetHealthyAgentForTopic_Unhealthy(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_ = store.Register(ctx, &AgentRegistration{
		Name:   "unhealthy-agent",
		Topics: []string{"test.topic"},
		Status: AgentStatusUnhealthy,
	})

	_, err := store.GetHealthyAgentForTopic(ctx, "test.topic")
	assert.Equal(t, ErrAgentUnhealthy, err)
}

func TestMemoryStore_GetHealthyAgentForTopic_UnknownStatus(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_ = store.Register(ctx, &AgentRegistration{
		Name:   "unknown-agent",
		Topics: []string{"test.topic"},
		Status: AgentStatusUnknown,
	})

	// Should return agent with unknown status
	result, err := store.GetHealthyAgentForTopic(ctx, "test.topic")
	require.NoError(t, err)
	assert.Equal(t, "unknown-agent", result.Name)
}

func TestMemoryStore_GetHealthyAgentForTopic_PrefersHealthy(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	_ = store.Register(ctx, &AgentRegistration{
		Name:   "unknown-agent",
		Topics: []string{"test.topic"},
		Status: AgentStatusUnknown,
	})
	_ = store.Register(ctx, &AgentRegistration{
		Name:   "healthy-agent",
		Topics: []string{"test.topic"},
		Status: AgentStatusHealthy,
	})

	result, err := store.GetHealthyAgentForTopic(ctx, "test.topic")
	require.NoError(t, err)
	assert.Equal(t, "healthy-agent", result.Name)
}

func TestMatchTopicPattern(t *testing.T) {
	tests := []struct {
		pattern string
		topic   string
		want    bool
	}{
		// Exact match
		{"alert.advisor.aggregate", "alert.advisor.aggregate", true},
		{"alert.advisor.aggregate", "alert.advisor.generate", false},

		// Wildcard at end
		{"alert.advisor.*", "alert.advisor.aggregate", true},
		{"alert.advisor.*", "alert.advisor.generate", true},
		{"alert.advisor.*", "alert.handler.analyze", false},

		// Deeper wildcard
		{"alert.*", "alert.advisor.aggregate", true},
		{"alert.*", "alert.handler.analyze", true},
		{"report.*", "alert.advisor.aggregate", false},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.topic, func(t *testing.T) {
			got := matchTopicPattern(tt.pattern, tt.topic)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMemoryStore_Concurrency(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	var wg sync.WaitGroup

	// Concurrent registrations
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agent := &AgentRegistration{
				Name:   "agent-" + string(rune('0'+(idx%10))),
				Topics: []string{"topic"},
			}
			_ = store.Register(ctx, agent)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = store.List(ctx)
			_, _ = store.ListByTopic(ctx, "topic")
			_, _ = store.Get(ctx, "agent-"+string(rune('0'+(idx%10))))
		}(i)
	}

	// Concurrent status updates
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = store.UpdateStatus(ctx, "agent-"+string(rune('0'+(idx%10))), AgentStatusHealthy, 0)
		}(i)
	}

	wg.Wait()
}

func TestMemoryStore_ImplementsRegistry(t *testing.T) {
	var _ Registry = (*MemoryStore)(nil)
}

