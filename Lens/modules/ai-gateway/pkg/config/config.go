package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config contains all configuration for the AI Gateway
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Registry   RegistryConfig   `yaml:"registry"`
	Background BackgroundConfig `yaml:"background"`
	Database   DatabaseConfig   `yaml:"database"`
}

// ServerConfig contains HTTP server configuration
type ServerConfig struct {
	Port         int    `yaml:"port"`
	Host         string `yaml:"host"`
	ReadTimeout  int    `yaml:"read_timeout"`
	WriteTimeout int    `yaml:"write_timeout"`
}

// RegistryConfig contains agent registry configuration
type RegistryConfig struct {
	Mode                string        `yaml:"mode"` // memory, db, config
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	UnhealthyThreshold  int           `yaml:"unhealthy_threshold"`
	StaticAgents        []StaticAgent `yaml:"agents"`
}

// StaticAgent represents a statically configured agent
type StaticAgent struct {
	Name            string        `yaml:"name"`
	Endpoint        string        `yaml:"endpoint"`
	Topics          []string      `yaml:"topics"`
	Timeout         time.Duration `yaml:"timeout"`
	HealthCheckPath string        `yaml:"health_check_path"`
}

// BackgroundConfig contains background job configuration
type BackgroundConfig struct {
	HealthCheck HealthCheckJobConfig `yaml:"health_check"`
	Timeout     TimeoutJobConfig     `yaml:"timeout"`
	Cleanup     CleanupJobConfig     `yaml:"cleanup"`
}

// HealthCheckJobConfig contains health check job configuration
type HealthCheckJobConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
}

// TimeoutJobConfig contains timeout handling job configuration
type TimeoutJobConfig struct {
	Enabled  bool          `yaml:"enabled"`
	Interval time.Duration `yaml:"interval"`
}

// CleanupJobConfig contains cleanup job configuration
type CleanupJobConfig struct {
	Enabled         bool          `yaml:"enabled"`
	Interval        time.Duration `yaml:"interval"`
	RetentionPeriod time.Duration `yaml:"retention_period"`
}

// DatabaseConfig contains database connection configuration
type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Database string `yaml:"database"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	SSLMode  string `yaml:"ssl_mode"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         8003,
			Host:         "0.0.0.0",
			ReadTimeout:  30,
			WriteTimeout: 60,
		},
		Registry: RegistryConfig{
			Mode:                "db",
			HealthCheckInterval: 30 * time.Second,
			UnhealthyThreshold:  3,
		},
		Background: BackgroundConfig{
			HealthCheck: HealthCheckJobConfig{
				Enabled:  true,
				Interval: 30 * time.Second,
			},
			Timeout: TimeoutJobConfig{
				Enabled:  true,
				Interval: 1 * time.Minute,
			},
			Cleanup: CleanupJobConfig{
				Enabled:         true,
				Interval:        1 * time.Hour,
				RetentionPeriod: 7 * 24 * time.Hour,
			},
		},
	}
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	cfg := DefaultConfig()

	if port := os.Getenv("AI_GATEWAY_PORT"); port != "" {
		// Parse and set port
	}

	if host := os.Getenv("AI_GATEWAY_HOST"); host != "" {
		cfg.Server.Host = host
	}

	if mode := os.Getenv("AI_GATEWAY_REGISTRY_MODE"); mode != "" {
		cfg.Registry.Mode = mode
	}

	// Database from environment
	if host := os.Getenv("DB_HOST"); host != "" {
		cfg.Database.Host = host
	}
	if user := os.Getenv("DB_USER"); user != "" {
		cfg.Database.Username = user
	}
	if pass := os.Getenv("DB_PASSWORD"); pass != "" {
		cfg.Database.Password = pass
	}
	if db := os.Getenv("DB_NAME"); db != "" {
		cfg.Database.Database = db
	}

	return cfg
}

