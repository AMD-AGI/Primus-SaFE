package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, "my-cluster", cfg.Global.ClusterName)
	assert.Equal(t, "primus-lens", cfg.Global.Namespace)
	assert.Equal(t, "local-path", cfg.Global.StorageClass)
	assert.Equal(t, "minimal", cfg.Profile)
	assert.True(t, cfg.Database.Enabled)
	assert.True(t, cfg.OpenSearch.Enabled)
	assert.True(t, cfg.VM.Enabled)
	assert.True(t, cfg.Apps.Enabled)
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "missing cluster name",
			config: &Config{
				Global: GlobalConfig{
					ClusterName: "",
					Namespace:   "primus-lens",
				},
			},
			wantErr: true,
		},
		{
			name: "missing namespace",
			config: &Config{
				Global: GlobalConfig{
					ClusterName: "my-cluster",
					Namespace:   "",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoadFromFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-values.yaml")

	configContent := `
global:
  clusterName: "test-cluster"
  namespace: "test-namespace"
  storageClass: "standard"
profile: "normal"
database:
  enabled: true
opensearch:
  enabled: false
`
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)

	cfg, err := LoadFromFile(configPath)
	require.NoError(t, err)

	assert.Equal(t, "test-cluster", cfg.Global.ClusterName)
	assert.Equal(t, "test-namespace", cfg.Global.Namespace)
	assert.Equal(t, "standard", cfg.Global.StorageClass)
	assert.Equal(t, "normal", cfg.Profile)
	assert.True(t, cfg.Database.Enabled)
	assert.False(t, cfg.OpenSearch.Enabled)
}

func TestLoadFromFileNotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/values.yaml")
	assert.Error(t, err)
}

func TestLoadFromFileInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")

	err := os.WriteFile(configPath, []byte("invalid: yaml: content: ["), 0644)
	require.NoError(t, err)

	_, err = LoadFromFile(configPath)
	assert.Error(t, err)
}
