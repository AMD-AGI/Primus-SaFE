package airegistry

import (
	"context"
	"errors"
	"time"
)

// Common errors
var (
	ErrAgentNotFound      = errors.New("agent not found")
	ErrAgentAlreadyExists = errors.New("agent already exists")
	ErrNoAgentForTopic    = errors.New("no agent registered for topic")
	ErrAgentUnhealthy     = errors.New("agent is unhealthy")
)

// AgentStatus represents the health status of an agent
type AgentStatus string

const (
	AgentStatusHealthy   AgentStatus = "healthy"
	AgentStatusUnhealthy AgentStatus = "unhealthy"
	AgentStatusUnknown   AgentStatus = "unknown"
)

// AgentRegistration represents a registered AI agent
type AgentRegistration struct {
	Name            string            `json:"name" gorm:"primaryKey"`
	Endpoint        string            `json:"endpoint"`
	Topics          []string          `json:"topics" gorm:"-"`        // Stored separately or as JSON
	TopicsJSON      string            `json:"-" gorm:"column:topics"` // JSON serialized topics
	HealthCheckPath string            `json:"health_check_path"`
	Timeout         time.Duration     `json:"timeout" gorm:"-"`
	TimeoutSeconds  int               `json:"-" gorm:"column:timeout_secs"` // For DB storage
	Status          AgentStatus       `json:"status"`
	LastHealthCheck time.Time         `json:"last_health_check"`
	FailureCount    int               `json:"failure_count"`
	Metadata        map[string]string `json:"metadata" gorm:"-"`
	MetadataJSON    string            `json:"-" gorm:"column:metadata"`
	RegisteredAt    time.Time         `json:"registered_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// TableName returns the table name for GORM
func (AgentRegistration) TableName() string {
	return "ai_agent_registrations"
}

// Registry defines the interface for agent registration storage
type Registry interface {
	// Register registers or updates an agent
	Register(ctx context.Context, agent *AgentRegistration) error

	// Unregister removes an agent from the registry
	Unregister(ctx context.Context, name string) error

	// Get retrieves an agent by name
	Get(ctx context.Context, name string) (*AgentRegistration, error)

	// List returns all registered agents
	List(ctx context.Context) ([]*AgentRegistration, error)

	// ListByTopic returns agents that handle a specific topic
	ListByTopic(ctx context.Context, topic string) ([]*AgentRegistration, error)

	// UpdateStatus updates the health status of an agent
	UpdateStatus(ctx context.Context, name string, status AgentStatus, failureCount int) error

	// GetHealthyAgentForTopic returns a healthy agent that can handle the topic
	GetHealthyAgentForTopic(ctx context.Context, topic string) (*AgentRegistration, error)
}

// RegistryConfig contains configuration for the registry
type RegistryConfig struct {
	// Mode: "memory", "db", "config", "hybrid"
	Mode string `json:"mode" yaml:"mode"`

	// Database connection string (for db mode)
	DatabaseDSN string `json:"database_dsn" yaml:"database_dsn"`

	// Config file path (for config mode)
	ConfigFile string `json:"config_file" yaml:"config_file"`

	// Static agents (for config/hybrid mode)
	StaticAgents []StaticAgentConfig `json:"agents" yaml:"agents"`

	// Health check settings
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
	UnhealthyThreshold  int           `json:"unhealthy_threshold" yaml:"unhealthy_threshold"`
}

// StaticAgentConfig represents agent configuration from config file
type StaticAgentConfig struct {
	Name            string        `json:"name" yaml:"name"`
	Endpoint        string        `json:"endpoint" yaml:"endpoint"`
	Topics          []string      `json:"topics" yaml:"topics"`
	Timeout         time.Duration `json:"timeout" yaml:"timeout"`
	HealthCheckPath string        `json:"health_check_path" yaml:"health_check_path"`
}

// DefaultConfig returns default registry configuration
func DefaultConfig() *RegistryConfig {
	return &RegistryConfig{
		Mode:                "memory",
		HealthCheckInterval: 30 * time.Second,
		UnhealthyThreshold:  3,
	}
}

// NewRegistry creates a new registry based on the configuration
func NewRegistry(cfg *RegistryConfig) (Registry, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	switch cfg.Mode {
	case "memory":
		return NewMemoryStore(), nil
	case "config":
		return NewConfigStore(cfg.StaticAgents), nil
	case "db":
		// DB store requires external initialization
		return nil, errors.New("db mode requires explicit store creation with DB connection")
	case "hybrid":
		// Hybrid combines config + db
		return nil, errors.New("hybrid mode requires explicit store creation")
	default:
		return NewMemoryStore(), nil
	}
}
