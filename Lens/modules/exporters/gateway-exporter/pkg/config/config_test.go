package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayConfig_GetScrapeInterval(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "valid duration 30s",
			input:    "30s",
			expected: 30 * time.Second,
		},
		{
			name:     "valid duration 1m",
			input:    "1m",
			expected: 1 * time.Minute,
		},
		{
			name:     "valid duration 5m30s",
			input:    "5m30s",
			expected: 5*time.Minute + 30*time.Second,
		},
		{
			name:     "invalid duration returns default",
			input:    "invalid",
			expected: 30 * time.Second,
		},
		{
			name:     "empty duration returns default",
			input:    "",
			expected: 30 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := GatewayConfig{ScrapeInterval: tt.input}
			result := g.GetScrapeInterval()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGatewayConfig_GetCacheTTL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{
			name:     "valid duration 60s",
			input:    "60s",
			expected: 60 * time.Second,
		},
		{
			name:     "valid duration 2m",
			input:    "2m",
			expected: 2 * time.Minute,
		},
		{
			name:     "valid duration 1h",
			input:    "1h",
			expected: 1 * time.Hour,
		},
		{
			name:     "invalid duration returns default",
			input:    "invalid",
			expected: 60 * time.Second,
		},
		{
			name:     "empty duration returns default",
			input:    "",
			expected: 60 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := GatewayConfig{CacheTTL: tt.input}
			result := g.GetCacheTTL()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestLoadGatewayExporterConfig(t *testing.T) {
	t.Run("loads config from file", func(t *testing.T) {
		// Create temp config file
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")
		configContent := `
httpPort: 9999
loadK8SClient: true
gateway:
  scrapeInterval: "45s"
  cacheTTL: "120s"
enrichment:
  watchNamespaces:
    - default
    - production
  workloadLabels:
    - app
    - version
metrics:
  staticLabels:
    cluster: test-cluster
`
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		// Set env var
		os.Setenv("CONFIG_PATH", configPath)
		defer os.Unsetenv("CONFIG_PATH")

		// Load config
		config, err := LoadGatewayExporterConfig()
		require.NoError(t, err)

		assert.Equal(t, 9999, config.HttpPort)
		assert.True(t, config.LoadK8SClient)
		assert.Equal(t, "45s", config.Gateway.ScrapeInterval)
		assert.Equal(t, "120s", config.Gateway.CacheTTL)
		assert.Contains(t, config.Enrichment.WatchNamespaces, "default")
		assert.Contains(t, config.Enrichment.WatchNamespaces, "production")
		assert.Contains(t, config.Enrichment.WorkloadLabels, "app")
		assert.Equal(t, "test-cluster", config.Metrics.StaticLabels["cluster"])
	})

	t.Run("sets default port when not specified", func(t *testing.T) {
		// Create temp config file without httpPort
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")
		configContent := `
gateway:
  scrapeInterval: "30s"
`
		err := os.WriteFile(configPath, []byte(configContent), 0644)
		require.NoError(t, err)

		os.Setenv("CONFIG_PATH", configPath)
		defer os.Unsetenv("CONFIG_PATH")

		config, err := LoadGatewayExporterConfig()
		require.NoError(t, err)

		assert.Equal(t, 8991, config.HttpPort)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		os.Setenv("CONFIG_PATH", "/non/existent/path/config.yaml")
		defer os.Unsetenv("CONFIG_PATH")

		_, err := LoadGatewayExporterConfig()
		assert.Error(t, err)
	})

	t.Run("returns error for invalid yaml", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "config.yaml")
		// Invalid YAML content
		err := os.WriteFile(configPath, []byte(`{{{invalid yaml`), 0644)
		require.NoError(t, err)

		os.Setenv("CONFIG_PATH", configPath)
		defer os.Unsetenv("CONFIG_PATH")

		_, err = LoadGatewayExporterConfig()
		assert.Error(t, err)
	})
}

func TestGatewayExporterConfig_Defaults(t *testing.T) {
	t.Run("empty config has zero values", func(t *testing.T) {
		config := GatewayExporterConfig{}
		assert.Equal(t, 0, config.HttpPort)
		assert.False(t, config.LoadK8SClient)
		assert.Empty(t, config.Gateway.Collectors)
		assert.Empty(t, config.Enrichment.WatchNamespaces)
	})
}

func TestEnrichmentConfig(t *testing.T) {
	t.Run("can hold multiple namespaces", func(t *testing.T) {
		config := EnrichmentConfig{
			WatchNamespaces: []string{"ns1", "ns2", "ns3"},
			WorkloadLabels:  []string{"app", "version", "tier"},
		}
		assert.Len(t, config.WatchNamespaces, 3)
		assert.Len(t, config.WorkloadLabels, 3)
	})
}

func TestMetricsConfig(t *testing.T) {
	t.Run("can hold static labels", func(t *testing.T) {
		config := MetricsConfig{
			StaticLabels: map[string]string{
				"env":     "production",
				"cluster": "main",
				"region":  "us-west-2",
			},
		}
		assert.Len(t, config.StaticLabels, 3)
		assert.Equal(t, "production", config.StaticLabels["env"])
	})
}
