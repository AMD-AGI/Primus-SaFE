package config

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	SMTP     SMTPConfig      `yaml:"smtp"`
	Clusters []ClusterConfig `yaml:"clusters"`
	LogLevel string          `yaml:"log_level"`
	APIPort  int             `yaml:"api_port"`
}

type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	From     string `yaml:"from"`
	FromName string `yaml:"from_name"`
	// User and plain credential for SMTP AUTH; set via EMAIL_RELAY_SMTP_USER / EMAIL_RELAY_SMTP_CREDENTIAL.
	User       string `yaml:"-"`
	Credential string `yaml:"-"`
}

type ClusterConfig struct {
	Name              string        `yaml:"name"`
	BaseURL           string        `yaml:"base_url"`
	APIPath           string        `yaml:"api_path"`
	Auth              AuthConfig    `yaml:"-"`
	ReconnectInterval time.Duration `yaml:"reconnect_interval"`
}

// AuthConfig holds upstream HTTP credentials; values come from environment only.
type AuthConfig struct {
	Outbound string `yaml:"-"`
	Internal string `yaml:"-"`
}

// ApplyHeaders sets upstream HTTP auth headers.
func (a AuthConfig) ApplyHeaders(req *http.Request) {
	if a.Outbound != "" {
		req.Header.Set("Authorization", "Bearer "+a.Outbound)
	} else if a.Internal != "" {
		req.Header.Set("X-Internal-Token", a.Internal)
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := &Config{
		LogLevel: "info",
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	if cfg.SMTP.Host == "" {
		return nil, fmt.Errorf("smtp.host is required")
	}
	if cfg.SMTP.Port == 0 {
		cfg.SMTP.Port = 25
	}
	if cfg.SMTP.From == "" {
		return nil, fmt.Errorf("smtp.from is required")
	}
	if cfg.APIPort == 0 {
		cfg.APIPort = 8090
	}

	cfg.SMTP.User = os.Getenv("EMAIL_RELAY_SMTP_USER")
	cfg.SMTP.Credential = os.Getenv("EMAIL_RELAY_SMTP_CREDENTIAL")

	for i := range cfg.Clusters {
		if cfg.Clusters[i].Name == "" {
			return nil, fmt.Errorf("clusters[%d].name is required", i)
		}
		if cfg.Clusters[i].BaseURL == "" {
			return nil, fmt.Errorf("clusters[%d].base_url is required", i)
		}
		cfg.Clusters[i].BaseURL = strings.TrimRight(cfg.Clusters[i].BaseURL, "/")
		if cfg.Clusters[i].APIPath == "" {
			cfg.Clusters[i].APIPath = "/api/v1/email-relay"
		}
		cfg.Clusters[i].APIPath = strings.TrimRight(cfg.Clusters[i].APIPath, "/")
		if cfg.Clusters[i].ReconnectInterval == 0 {
			cfg.Clusters[i].ReconnectInterval = 5 * time.Second
		}
		prefix := fmt.Sprintf("EMAIL_RELAY_CLUSTER_%d_", i)
		cfg.Clusters[i].Auth.Outbound = os.Getenv(prefix + "OUTBOUND")
		cfg.Clusters[i].Auth.Internal = os.Getenv(prefix + "INTERNAL")
	}

	return cfg, nil
}
