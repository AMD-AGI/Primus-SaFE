package airegistry

import (
	"context"
	"sync"
	"time"
)

// ConfigStore implements Registry using static configuration
// This store is read-only for agent registration (agents defined in config)
// but allows status updates
type ConfigStore struct {
	mu     sync.RWMutex
	agents map[string]*AgentRegistration
}

// NewConfigStore creates a new config-based registry store
func NewConfigStore(configs []StaticAgentConfig) *ConfigStore {
	store := &ConfigStore{
		agents: make(map[string]*AgentRegistration),
	}

	now := time.Now()
	for _, cfg := range configs {
		healthPath := cfg.HealthCheckPath
		if healthPath == "" {
			healthPath = "/health"
		}

		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}

		store.agents[cfg.Name] = &AgentRegistration{
			Name:            cfg.Name,
			Endpoint:        cfg.Endpoint,
			Topics:          cfg.Topics,
			HealthCheckPath: healthPath,
			Timeout:         timeout,
			Status:          AgentStatusUnknown,
			RegisteredAt:    now,
			UpdatedAt:       now,
		}
	}

	return store
}

// Register is not allowed for config store (read-only)
func (s *ConfigStore) Register(ctx context.Context, agent *AgentRegistration) error {
	// Config store is read-only, but we allow re-registration to update status
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.agents[agent.Name]
	if !ok {
		// Only allow updating existing agents
		return ErrAgentNotFound
	}

	// Allow updating status
	existing.Status = agent.Status
	existing.FailureCount = agent.FailureCount
	existing.LastHealthCheck = agent.LastHealthCheck
	existing.UpdatedAt = time.Now()

	return nil
}

// Unregister is not allowed for config store
func (s *ConfigStore) Unregister(ctx context.Context, name string) error {
	// Config store is read-only
	return ErrAgentNotFound
}

// Get retrieves an agent by name
func (s *ConfigStore) Get(ctx context.Context, name string) (*AgentRegistration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agent, ok := s.agents[name]
	if !ok {
		return nil, ErrAgentNotFound
	}

	copy := *agent
	return &copy, nil
}

// List returns all registered agents
func (s *ConfigStore) List(ctx context.Context) ([]*AgentRegistration, error) {
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
func (s *ConfigStore) ListByTopic(ctx context.Context, topic string) ([]*AgentRegistration, error) {
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
func (s *ConfigStore) UpdateStatus(ctx context.Context, name string, status AgentStatus, failureCount int) error {
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
func (s *ConfigStore) GetHealthyAgentForTopic(ctx context.Context, topic string) (*AgentRegistration, error) {
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
			copy := *agent
			return &copy, nil
		}
	}

	return nil, ErrNoAgentForTopic
}

// matchesTopic checks if an agent's topic patterns match the given topic
func (s *ConfigStore) matchesTopic(agent *AgentRegistration, topic string) bool {
	for _, pattern := range agent.Topics {
		if matchTopicPattern(pattern, topic) {
			return true
		}
	}
	return false
}

// Reload reloads the configuration from the provided configs
func (s *ConfigStore) Reload(configs []StaticAgentConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Preserve status for existing agents
	statusMap := make(map[string]struct {
		status      AgentStatus
		failureCount int
		lastCheck   time.Time
	})
	for name, agent := range s.agents {
		statusMap[name] = struct {
			status      AgentStatus
			failureCount int
			lastCheck   time.Time
		}{
			status:       agent.Status,
			failureCount: agent.FailureCount,
			lastCheck:    agent.LastHealthCheck,
		}
	}

	// Rebuild agents map
	s.agents = make(map[string]*AgentRegistration)
	now := time.Now()

	for _, cfg := range configs {
		healthPath := cfg.HealthCheckPath
		if healthPath == "" {
			healthPath = "/health"
		}

		timeout := cfg.Timeout
		if timeout == 0 {
			timeout = 60 * time.Second
		}

		agent := &AgentRegistration{
			Name:            cfg.Name,
			Endpoint:        cfg.Endpoint,
			Topics:          cfg.Topics,
			HealthCheckPath: healthPath,
			Timeout:         timeout,
			Status:          AgentStatusUnknown,
			RegisteredAt:    now,
			UpdatedAt:       now,
		}

		// Restore previous status if available
		if prev, ok := statusMap[cfg.Name]; ok {
			agent.Status = prev.status
			agent.FailureCount = prev.failureCount
			agent.LastHealthCheck = prev.lastCheck
		}

		s.agents[cfg.Name] = agent
	}
}

