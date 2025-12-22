package config

import (
	"os"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/collector"
	"gopkg.in/yaml.v2"
)

// GatewayExporterConfig is the main configuration for gateway-exporter
type GatewayExporterConfig struct {
	HttpPort          int               `yaml:"httpPort" json:"httpPort"`
	LoadK8SClient     bool              `yaml:"loadK8SClient" json:"loadK8SClient"`
	Gateway           GatewayConfig     `yaml:"gateway" json:"gateway"`
	Enrichment        EnrichmentConfig  `yaml:"enrichment" json:"enrichment"`
	Metrics           MetricsConfig     `yaml:"metrics" json:"metrics"`
}

// GatewayConfig contains gateway scraping configuration
type GatewayConfig struct {
	ScrapeInterval string                       `yaml:"scrapeInterval" json:"scrapeInterval"`
	CacheTTL       string                       `yaml:"cacheTTL" json:"cacheTTL"`
	Collectors     []collector.CollectorConfig  `yaml:"collectors" json:"collectors"`
}

// GetScrapeInterval returns the scrape interval as duration
func (g GatewayConfig) GetScrapeInterval() time.Duration {
	d, err := time.ParseDuration(g.ScrapeInterval)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// GetCacheTTL returns the cache TTL as duration
func (g GatewayConfig) GetCacheTTL() time.Duration {
	d, err := time.ParseDuration(g.CacheTTL)
	if err != nil {
		return 60 * time.Second
	}
	return d
}

// EnrichmentConfig contains workload enrichment configuration
type EnrichmentConfig struct {
	WatchNamespaces []string `yaml:"watchNamespaces" json:"watchNamespaces"`
	WorkloadLabels  []string `yaml:"workloadLabels" json:"workloadLabels"`
}

// MetricsConfig contains prometheus metrics configuration
type MetricsConfig struct {
	StaticLabels map[string]string `yaml:"staticLabels" json:"staticLabels"`
}

// LoadGatewayExporterConfig loads configuration from file
func LoadGatewayExporterConfig() (*GatewayExporterConfig, error) {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	configFile, err := os.Open(configPath)
	if err != nil {
		return nil, err
	}
	defer configFile.Close()

	var config GatewayExporterConfig
	decoder := yaml.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	// Set defaults
	if config.HttpPort == 0 {
		config.HttpPort = 8991
	}

	return &config, nil
}

