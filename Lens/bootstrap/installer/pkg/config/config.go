package config

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"
)

// Config represents the installer configuration
type Config struct {
	Global     GlobalConfig     `json:"global"`
	Profile    string           `json:"profile"`
	Database   DatabaseConfig   `json:"database"`
	OpenSearch OpenSearchConfig `json:"opensearch"`
	VM         VMConfig         `json:"victoriametrics"`
	Logging    LoggingConfig    `json:"logging"`
	Grafana    GrafanaConfig    `json:"grafana"`
	Monitoring MonitoringConfig `json:"monitoring"`
	Apps       AppsConfig       `json:"apps"`
}

// GlobalConfig represents global settings
type GlobalConfig struct {
	ClusterName   string `json:"clusterName"`
	Namespace     string `json:"namespace"`
	StorageClass  string `json:"storageClass"`
	AccessMode    string `json:"accessMode"`
	ImageRegistry string `json:"imageRegistry"`
	AccessType    string `json:"accessType"`
	Domain        string `json:"domain"`
}

// DatabaseConfig represents PostgreSQL configuration
type DatabaseConfig struct {
	Enabled    bool   `json:"enabled"`
	InitScript string `json:"initScript"`
}

// OpenSearchConfig represents OpenSearch configuration
type OpenSearchConfig struct {
	Enabled     bool   `json:"enabled"`
	ClusterName string `json:"clusterName"`
}

// VMConfig represents VictoriaMetrics configuration
type VMConfig struct {
	Enabled bool `json:"enabled"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Enabled bool `json:"enabled"`
}

// GrafanaConfig represents Grafana configuration
type GrafanaConfig struct {
	Enabled       bool   `json:"enabled"`
	AdminPassword string `json:"adminPassword"`
}

// MonitoringConfig represents monitoring configuration
type MonitoringConfig struct {
	KubeStateMetrics struct {
		Enabled bool `json:"enabled"`
	} `json:"kubeStateMetrics"`
}

// AppsConfig represents application configuration
type AppsConfig struct {
	Enabled bool `json:"enabled"`
}

// LoadFromFile loads configuration from a YAML file
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return cfg, nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Global: GlobalConfig{
			ClusterName:   "my-cluster",
			Namespace:     "primus-lens",
			StorageClass:  "local-path",
			AccessMode:    "ReadWriteOnce",
			ImageRegistry: "docker.io",
			AccessType:    "ssh-tunnel",
			Domain:        "lens-primus.ai",
		},
		Profile: "minimal",
		Database: DatabaseConfig{
			Enabled:    true,
			InitScript: "setup_primus_lens.sql",
		},
		OpenSearch: OpenSearchConfig{
			Enabled:     true,
			ClusterName: "primus-lens-logs",
		},
		VM: VMConfig{
			Enabled: true,
		},
		Logging: LoggingConfig{
			Enabled: true,
		},
		Grafana: GrafanaConfig{
			Enabled:       true,
			AdminPassword: "admin",
		},
		Monitoring: MonitoringConfig{
			KubeStateMetrics: struct {
				Enabled bool `json:"enabled"`
			}{Enabled: true},
		},
		Apps: AppsConfig{
			Enabled: true,
		},
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Global.ClusterName == "" {
		return fmt.Errorf("global.clusterName is required")
	}
	if c.Global.Namespace == "" {
		return fmt.Errorf("global.namespace is required")
	}
	return nil
}
