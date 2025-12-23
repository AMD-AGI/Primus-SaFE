package collector

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/gateway-exporter/pkg/model"
)

// GatewayType represents the type of gateway
type GatewayType string

const (
	GatewayTypeHigress      GatewayType = "higress"
	GatewayTypeNginxIngress GatewayType = "nginx-ingress"
	GatewayTypeIstio        GatewayType = "istio"
)

// Collector is the interface that all gateway collectors must implement
type Collector interface {
	// Type returns the type of gateway this collector handles
	Type() GatewayType

	// Name returns a unique name for this collector instance
	Name() string

	// Collect fetches metrics from the gateway and returns raw traffic metrics
	Collect(ctx context.Context) ([]model.RawTrafficMetric, error)

	// Discover discovers gateway endpoints to scrape
	Discover(ctx context.Context) ([]GatewayEndpoint, error)

	// HealthCheck checks if the collector is healthy
	HealthCheck(ctx context.Context) error
}

// GatewayEndpoint represents a gateway instance to scrape
type GatewayEndpoint struct {
	Address     string            // e.g., "10.32.80.103:15020"
	MetricsPath string            // e.g., "/stats/prometheus"
	Labels      map[string]string // additional labels
}

// CollectorConfig contains configuration for a collector
type CollectorConfig struct {
	Type           GatewayType       `yaml:"type" json:"type"`
	Enabled        bool              `yaml:"enabled" json:"enabled"`
	Namespace      string            `yaml:"namespace" json:"namespace"`
	LabelSelector  map[string]string `yaml:"labelSelector" json:"labelSelector"`
	MetricsPort    int               `yaml:"metricsPort" json:"metricsPort"`
	MetricsPath    string            `yaml:"metricsPath" json:"metricsPath"`
	ScrapeInterval string            `yaml:"scrapeInterval" json:"scrapeInterval"`
	LabelMappings  map[string]string `yaml:"labelMappings" json:"labelMappings"`
}

