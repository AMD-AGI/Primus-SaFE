package airegistry

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryStore implements Registry using in-memory storage
type MemoryStore struct {
	mu     sync.RWMutex
	agents map[string]*AgentRegistration
}

// NewMemoryStore creates a new in-memory registry store
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		agents: make(map[string]*AgentRegistration),
	}
}

// Register registers or updates an agent
func (s *MemoryStore) Register(ctx context.Context, agent *AgentRegistration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if existing, ok := s.agents[agent.Name]; ok {
		// Update existing
		agent.RegisteredAt = existing.RegisteredAt
		agent.UpdatedAt = now
	} else {
		// New registration
		agent.RegisteredAt = now
		agent.UpdatedAt = now
	}

	if agent.Status == "" {
		agent.Status = AgentStatusUnknown
	}

	// Deep copy to avoid external modification
	copy := *agent
	copy.Topics = make([]string, len(agent.Topics))
	copy.Metadata = make(map[string]string)
	for i, t := range agent.Topics {
		copy.Topics[i] = t
	}
	for k, v := range agent.Metadata {
		copy.Metadata[k] = v
	}

	s.agents[agent.Name] = &copy
	return nil
}

// Unregister removes an agent from the registry
func (s *MemoryStore) Unregister(ctx context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.agents[name]; !ok {
		return ErrAgentNotFound
	}

	delete(s.agents, name)
	return nil
}

// Get retrieves an agent by name
func (s *MemoryStore) Get(ctx context.Context, name string) (*AgentRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agent, ok := s.agents[name]
	if !ok {
		return nil, ErrAgentNotFound
	}

	// Return a copy
	copy := *agent
	return &copy, nil
}

// List returns all registered agents
func (s *MemoryStore) List(ctx context.Context) ([]*AgentRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AgentRegistration, 0, len(s.agents))
	for _, agent := range s.agents {
		copy := *agent
		result = append(result, &copy)
	}
	return result, nil
}

// ListByTopic returns agents that handle a specific topic
func (s *MemoryStore) ListByTopic(ctx context.Context, topic string) ([]*AgentRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*AgentRegistration
	for _, agent := range s.agents {
		if s.matchesTopic(agent, topic) {
			copy := *agent
			result = append(result, &copy)
		}
	}
	return result, nil
}

// UpdateStatus updates the health status of an agent
func (s *MemoryStore) UpdateStatus(ctx context.Context, name string, status AgentStatus, failureCount int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	agent, ok := s.agents[name]
	if !ok {
		return ErrAgentNotFound
	}

	agent.Status = status
	agent.FailureCount = failureCount
	agent.LastHealthCheck = time.Now()
	agent.UpdatedAt = time.Now()
	return nil
}

// GetHealthyAgentForTopic returns a healthy agent that can handle the topic
func (s *MemoryStore) GetHealthyAgentForTopic(ctx context.Context, topic string) (*AgentRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, agent := range s.agents {
		if agent.Status == AgentStatusHealthy && s.matchesTopic(agent, topic) {
			copy := *agent
			return &copy, nil
		}
	}

	// If no healthy agent, check for any agent that matches
	for _, agent := range s.agents {
		if s.matchesTopic(agent, topic) {
			if agent.Status == AgentStatusUnhealthy {
				return nil, ErrAgentUnhealthy
			}
			// Unknown status - return it anyway
			copy := *agent
			return &copy, nil
		}
	}

	return nil, ErrNoAgentForTopic
}

// matchesTopic checks if an agent's topic patterns match the given topic
func (s *MemoryStore) matchesTopic(agent *AgentRegistration, topic string) bool {
	for _, pattern := range agent.Topics {
		if matchTopicPattern(pattern, topic) {
			return true
		}
	}
	return false
}

// matchTopicPattern matches a topic against a pattern (supports * wildcard)
// Examples:
//   - "alert.advisor.*" matches "alert.advisor.aggregate-workloads"
//   - "alert.*" matches "alert.advisor.aggregate-workloads" and "alert.handler.analyze"
//   - "alert.advisor.aggregate-workloads" matches exactly
func matchTopicPattern(pattern, topic string) bool {
	// Exact match
	if pattern == topic {
		return true
	}

	// Wildcard match
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		return strings.HasPrefix(topic, prefix+".")
	}

	// Deeper wildcard (e.g., "alert.*" should match "alert.advisor.something")
	if strings.Contains(pattern, "*") {
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(topic, parts[0]) && strings.HasSuffix(topic, parts[1])
		}
	}

	return false
}

