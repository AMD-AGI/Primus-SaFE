// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package airegistry

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
)

// DBStore implements Registry using database facade
type DBStore struct {
	facade      database.AIAgentRegistrationFacadeInterface
	clusterName string
}

// NewDBStore creates a new database-backed registry store
func NewDBStore(clusterName string) *DBStore {
	facade := database.NewAIAgentRegistrationFacade()
	if clusterName != "" {
		facade = facade.WithCluster(clusterName)
	}
	return &DBStore{
		facade:      facade,
		clusterName: clusterName,
	}
}

// NewDBStoreWithFacade creates a new database-backed registry store with a custom facade
func NewDBStoreWithFacade(facade database.AIAgentRegistrationFacadeInterface) *DBStore {
	return &DBStore{
		facade: facade,
	}
}

// Register registers or updates an agent
func (s *DBStore) Register(ctx context.Context, agent *AgentRegistration) error {
	// Convert to database model
	dbAgent := s.toDBModel(agent)
	return s.facade.Register(ctx, dbAgent)
}

// Unregister removes an agent from the registry
func (s *DBStore) Unregister(ctx context.Context, name string) error {
	err := s.facade.Unregister(ctx, name)
	if err == database.ErrAgentNotFound {
		return ErrAgentNotFound
	}
	return err
}

// Get retrieves an agent by name
func (s *DBStore) Get(ctx context.Context, name string) (*AgentRegistration, error) {
	dbAgent, err := s.facade.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if dbAgent == nil {
		return nil, ErrAgentNotFound
	}
	return s.fromDBModel(dbAgent), nil
}

// List returns all registered agents
func (s *DBStore) List(ctx context.Context) ([]*AgentRegistration, error) {
	dbAgents, err := s.facade.List(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*AgentRegistration, len(dbAgents))
	for i, dbAgent := range dbAgents {
		result[i] = s.fromDBModel(dbAgent)
	}
	return result, nil
}

// ListByTopic returns agents that handle a specific topic
func (s *DBStore) ListByTopic(ctx context.Context, topic string) ([]*AgentRegistration, error) {
	// Get all agents and filter in-memory for pattern matching
	allAgents, err := s.List(ctx)
	if err != nil {
		return nil, err
	}

	var result []*AgentRegistration
	for _, agent := range allAgents {
		if s.matchesTopic(agent, topic) {
			result = append(result, agent)
		}
	}
	return result, nil
}

// UpdateStatus updates the health status of an agent
func (s *DBStore) UpdateStatus(ctx context.Context, name string, status AgentStatus, failureCount int) error {
	err := s.facade.UpdateStatus(ctx, name, string(status), failureCount)
	if err == database.ErrAgentNotFound {
		return ErrAgentNotFound
	}
	return err
}

// GetHealthyAgentForTopic returns a healthy agent that can handle the topic
func (s *DBStore) GetHealthyAgentForTopic(ctx context.Context, topic string) (*AgentRegistration, error) {
	agents, err := s.ListByTopic(ctx, topic)
	if err != nil {
		return nil, err
	}

	if len(agents) == 0 {
		return nil, ErrNoAgentForTopic
	}

	// Prefer healthy agents
	for _, agent := range agents {
		if agent.Status == AgentStatusHealthy {
			return agent, nil
		}
	}

	// Check if all are unhealthy
	for _, agent := range agents {
		if agent.Status == AgentStatusUnhealthy {
			return nil, ErrAgentUnhealthy
		}
	}

	// Return first agent with unknown status
	return agents[0], nil
}

// toDBModel converts AgentRegistration to database model
func (s *DBStore) toDBModel(agent *AgentRegistration) *model.AIAgentRegistration {
	topicsJSON := ""
	if len(agent.Topics) > 0 {
		topicsBytes, _ := json.Marshal(agent.Topics)
		topicsJSON = string(topicsBytes)
	}

	metadataJSON := ""
	if len(agent.Metadata) > 0 {
		metaBytes, _ := json.Marshal(agent.Metadata)
		metadataJSON = string(metaBytes)
	}

	return &model.AIAgentRegistration{
		Name:            agent.Name,
		Endpoint:        agent.Endpoint,
		Topics:          agent.Topics,
		TopicsJSON:      topicsJSON,
		HealthCheckPath: agent.HealthCheckPath,
		TimeoutSecs:     int(agent.Timeout.Seconds()),
		Status:          string(agent.Status),
		LastHealthCheck: &agent.LastHealthCheck,
		FailureCount:    agent.FailureCount,
		Metadata:        agent.Metadata,
		MetadataJSON:    metadataJSON,
		RegisteredAt:    agent.RegisteredAt,
		UpdatedAt:       agent.UpdatedAt,
	}
}

// fromDBModel converts database model to AgentRegistration
func (s *DBStore) fromDBModel(dbAgent *model.AIAgentRegistration) *AgentRegistration {
	var lastHealthCheck time.Time
	if dbAgent.LastHealthCheck != nil {
		lastHealthCheck = *dbAgent.LastHealthCheck
	}

	return &AgentRegistration{
		Name:            dbAgent.Name,
		Endpoint:        dbAgent.Endpoint,
		Topics:          dbAgent.Topics,
		HealthCheckPath: dbAgent.HealthCheckPath,
		Timeout:         time.Duration(dbAgent.TimeoutSecs) * time.Second,
		Status:          AgentStatus(dbAgent.Status),
		LastHealthCheck: lastHealthCheck,
		FailureCount:    dbAgent.FailureCount,
		Metadata:        dbAgent.Metadata,
		RegisteredAt:    dbAgent.RegisteredAt,
		UpdatedAt:       dbAgent.UpdatedAt,
	}
}

// matchesTopic checks if an agent's topic patterns match the given topic
func (s *DBStore) matchesTopic(agent *AgentRegistration, topic string) bool {
	for _, pattern := range agent.Topics {
		if matchTopicPattern(pattern, topic) {
			return true
		}
	}
	return false
}
