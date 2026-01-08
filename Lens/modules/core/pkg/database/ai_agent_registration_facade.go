package database

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AIAgentRegistrationFacadeInterface defines the database operation interface for AI Agent registrations
type AIAgentRegistrationFacadeInterface interface {
	// Register registers or updates an agent
	Register(ctx context.Context, agent *model.AIAgentRegistration) error

	// Unregister removes an agent from the registry
	Unregister(ctx context.Context, name string) error

	// Get retrieves an agent by name
	Get(ctx context.Context, name string) (*model.AIAgentRegistration, error)

	// List returns all registered agents
	List(ctx context.Context) ([]*model.AIAgentRegistration, error)

	// ListByStatus returns agents with the specified status
	ListByStatus(ctx context.Context, status string) ([]*model.AIAgentRegistration, error)

	// ListByTopic returns agents that handle a specific topic (basic match)
	ListByTopic(ctx context.Context, topic string) ([]*model.AIAgentRegistration, error)

	// UpdateStatus updates the health status of an agent
	UpdateStatus(ctx context.Context, name string, status string, failureCount int) error

	// WithCluster method
	WithCluster(clusterName string) AIAgentRegistrationFacadeInterface
}

// AIAgentRegistrationFacade implements AIAgentRegistrationFacadeInterface
type AIAgentRegistrationFacade struct {
	BaseFacade
}

// NewAIAgentRegistrationFacade creates a new AIAgentRegistrationFacade instance
func NewAIAgentRegistrationFacade() AIAgentRegistrationFacadeInterface {
	return &AIAgentRegistrationFacade{}
}

func (f *AIAgentRegistrationFacade) WithCluster(clusterName string) AIAgentRegistrationFacadeInterface {
	return &AIAgentRegistrationFacade{
		BaseFacade: f.withCluster(clusterName),
	}
}

// Register registers or updates an agent
func (f *AIAgentRegistrationFacade) Register(ctx context.Context, agent *model.AIAgentRegistration) error {
	db := f.getDB().WithContext(ctx)

	now := time.Now()
	agent.UpdatedAt = now
	if agent.RegisteredAt.IsZero() {
		agent.RegisteredAt = now
	}

	// Serialize topics if needed
	if len(agent.Topics) > 0 && agent.TopicsJSON == "" {
		topicsBytes, err := json.Marshal(agent.Topics)
		if err != nil {
			return err
		}
		agent.TopicsJSON = string(topicsBytes)
	}

	// Serialize metadata if needed
	if len(agent.Metadata) > 0 && agent.MetadataJSON == "" {
		metaBytes, err := json.Marshal(agent.Metadata)
		if err != nil {
			return err
		}
		agent.MetadataJSON = string(metaBytes)
	}

	// Upsert
	result := db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"endpoint", "topics", "health_check_path", "timeout_secs",
			"status", "metadata", "updated_at",
		}),
	}).Create(agent)

	return result.Error
}

// Unregister removes an agent from the registry
func (f *AIAgentRegistrationFacade) Unregister(ctx context.Context, name string) error {
	db := f.getDB().WithContext(ctx)
	result := db.Delete(&model.AIAgentRegistration{}, "name = ?", name)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAgentNotFound
	}
	return nil
}

// Get retrieves an agent by name
func (f *AIAgentRegistrationFacade) Get(ctx context.Context, name string) (*model.AIAgentRegistration, error) {
	db := f.getDB().WithContext(ctx)
	var agent model.AIAgentRegistration
	err := db.Where("name = ?", name).First(&agent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	deserializeAgent(&agent)
	return &agent, nil
}

// List returns all registered agents
func (f *AIAgentRegistrationFacade) List(ctx context.Context) ([]*model.AIAgentRegistration, error) {
	db := f.getDB().WithContext(ctx)
	var agents []model.AIAgentRegistration
	err := db.Find(&agents).Error
	if err != nil {
		return nil, err
	}

	result := make([]*model.AIAgentRegistration, len(agents))
	for i := range agents {
		deserializeAgent(&agents[i])
		result[i] = &agents[i]
	}
	return result, nil
}

// ListByStatus returns agents with the specified status
func (f *AIAgentRegistrationFacade) ListByStatus(ctx context.Context, status string) ([]*model.AIAgentRegistration, error) {
	db := f.getDB().WithContext(ctx)
	var agents []model.AIAgentRegistration
	err := db.Where("status = ?", status).Find(&agents).Error
	if err != nil {
		return nil, err
	}

	result := make([]*model.AIAgentRegistration, len(agents))
	for i := range agents {
		deserializeAgent(&agents[i])
		result[i] = &agents[i]
	}
	return result, nil
}

// ListByTopic returns agents that handle a specific topic
// Note: This performs a basic JSON contains check. For pattern matching,
// use the airegistry package's router with in-memory filtering.
func (f *AIAgentRegistrationFacade) ListByTopic(ctx context.Context, topic string) ([]*model.AIAgentRegistration, error) {
	db := f.getDB().WithContext(ctx)
	var agents []model.AIAgentRegistration

	// Use PostgreSQL JSONB contains operator to check if topic is in the array
	// This checks for exact matches only
	err := db.Where("topics @> ?", `["`+topic+`"]`).Find(&agents).Error
	if err != nil {
		return nil, err
	}

	result := make([]*model.AIAgentRegistration, len(agents))
	for i := range agents {
		deserializeAgent(&agents[i])
		result[i] = &agents[i]
	}
	return result, nil
}

// UpdateStatus updates the health status of an agent
func (f *AIAgentRegistrationFacade) UpdateStatus(ctx context.Context, name string, status string, failureCount int) error {
	db := f.getDB().WithContext(ctx)
	result := db.Model(&model.AIAgentRegistration{}).
		Where("name = ?", name).
		Updates(map[string]interface{}{
			"status":            status,
			"failure_count":     failureCount,
			"last_health_check": time.Now(),
			"updated_at":        time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrAgentNotFound
	}
	return nil
}

// deserializeAgent deserializes JSON fields
func deserializeAgent(agent *model.AIAgentRegistration) {
	if agent.TopicsJSON != "" {
		json.Unmarshal([]byte(agent.TopicsJSON), &agent.Topics)
	}
	if agent.MetadataJSON != "" {
		json.Unmarshal([]byte(agent.MetadataJSON), &agent.Metadata)
	}
	agent.Timeout = time.Duration(agent.TimeoutSecs) * time.Second
}

// ErrAgentNotFound is returned when an agent is not found
var ErrAgentNotFound = errors.New("agent not found")

