package config

import (
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	// Default configuration values
	DefaultTaskPollInterval     = 5 * time.Second
	DefaultLockDuration         = 60 * time.Second
	DefaultLockRenewInterval    = 30 * time.Second
	DefaultScrapeInterval       = 15 * time.Second
	DefaultScrapeTimeout        = 10 * time.Second
	DefaultConfigReloadInterval = 60 * time.Second
	DefaultMaxConcurrentScrapes = 50
)

// ExporterConfig holds the configuration for the inference metrics exporter
type ExporterConfig struct {
	// InstanceID uniquely identifies this exporter instance
	InstanceID string

	// Task polling configuration
	TaskPollInterval time.Duration

	// Distributed lock configuration
	LockDuration      time.Duration
	LockRenewInterval time.Duration

	// Scraping configuration
	DefaultScrapeInterval time.Duration
	DefaultScrapeTimeout  time.Duration
	MaxConcurrentScrapes  int

	// Config reload interval
	ConfigReloadInterval time.Duration
}

// LoadExporterConfig loads configuration from environment variables
func LoadExporterConfig() *ExporterConfig {
	cfg := &ExporterConfig{
		InstanceID:            getInstanceID(),
		TaskPollInterval:      getDurationEnv("TASK_POLL_INTERVAL", DefaultTaskPollInterval),
		LockDuration:          getDurationEnv("LOCK_DURATION", DefaultLockDuration),
		LockRenewInterval:     getDurationEnv("LOCK_RENEW_INTERVAL", DefaultLockRenewInterval),
		DefaultScrapeInterval: getDurationEnv("DEFAULT_SCRAPE_INTERVAL", DefaultScrapeInterval),
		DefaultScrapeTimeout:  getDurationEnv("DEFAULT_SCRAPE_TIMEOUT", DefaultScrapeTimeout),
		MaxConcurrentScrapes:  getIntEnv("MAX_CONCURRENT_SCRAPES", DefaultMaxConcurrentScrapes),
		ConfigReloadInterval:  getDurationEnv("CONFIG_RELOAD_INTERVAL", DefaultConfigReloadInterval),
	}
	return cfg
}

// getInstanceID generates or retrieves the instance ID
func getInstanceID() string {
	// Try to get from environment (e.g., pod name in Kubernetes)
	if podName := os.Getenv("POD_NAME"); podName != "" {
		return podName
	}
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		return hostname
	}
	// Generate a unique ID
	return "exporter-" + uuid.New().String()[:8]
}

// getDurationEnv gets a duration from environment variable
func getDurationEnv(key string, defaultVal time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

// getIntEnv gets an integer from environment variable
func getIntEnv(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}
