/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestGetProxyServices(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		expectedCount int
		expectedFirst *ProxyService
	}{
		{
			name: "empty proxy services",
			configContent: `
proxy:
  services: []
`,
			expectedCount: 0,
			expectedFirst: nil,
		},
		{
			name: "single proxy service",
			configContent: `
proxy:
  services:
    - name: qa-agent
      prefix: /agent/qa
      target: http://qa-agent-service:8080
      enabled: true
`,
			expectedCount: 1,
			expectedFirst: &ProxyService{
				Name:    "qa-agent",
				Prefix:  "/agent/qa",
				Target:  "http://qa-agent-service:8080",
				Enabled: true,
			},
		},
		{
			name: "multiple proxy services",
			configContent: `
proxy:
  services:
    - name: qa-agent
      prefix: /agent/qa
      target: http://qa-agent-service:8080
      enabled: true
    - name: data-service
      prefix: /api/data
      target: http://data-service:9000
      enabled: false
`,
			expectedCount: 2,
			expectedFirst: &ProxyService{
				Name:    "qa-agent",
				Prefix:  "/agent/qa",
				Target:  "http://qa-agent-service:8080",
				Enabled: true,
			},
		},
		{
			name: "no proxy config",
			configContent: `
server:
  port: 8088
`,
			expectedCount: 0,
			expectedFirst: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
			assert.NoError(t, err)

			// Reset viper for each test
			viper.Reset()

			// Load the config
			err = LoadConfig(configPath)
			assert.NoError(t, err)

			// Get proxy services
			services := GetProxyServices()

			// Verify count
			assert.Equal(t, tt.expectedCount, len(services))

			// Verify first service if exists
			if tt.expectedFirst != nil && len(services) > 0 {
				assert.Equal(t, tt.expectedFirst.Name, services[0].Name)
				assert.Equal(t, tt.expectedFirst.Prefix, services[0].Prefix)
				assert.Equal(t, tt.expectedFirst.Target, services[0].Target)
				assert.Equal(t, tt.expectedFirst.Enabled, services[0].Enabled)
			}
		})
	}
}

func TestProxyServiceStruct(t *testing.T) {
	tests := []struct {
		name    string
		service ProxyService
		valid   bool
	}{
		{
			name: "valid service",
			service: ProxyService{
				Name:    "test-service",
				Prefix:  "/api/test",
				Target:  "http://test-service:8080",
				Enabled: true,
			},
			valid: true,
		},
		{
			name: "disabled service",
			service: ProxyService{
				Name:    "disabled-service",
				Prefix:  "/api/disabled",
				Target:  "http://disabled-service:8080",
				Enabled: false,
			},
			valid: true,
		},
		{
			name: "empty name",
			service: ProxyService{
				Name:    "",
				Prefix:  "/api/test",
				Target:  "http://test-service:8080",
				Enabled: true,
			},
			valid: false,
		},
		{
			name: "empty prefix",
			service: ProxyService{
				Name:    "test-service",
				Prefix:  "",
				Target:  "http://test-service:8080",
				Enabled: true,
			},
			valid: false,
		},
		{
			name: "empty target",
			service: ProxyService{
				Name:    "test-service",
				Prefix:  "/api/test",
				Target:  "",
				Enabled: true,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.service.Name != "" &&
				tt.service.Prefix != "" &&
				tt.service.Target != ""
			assert.Equal(t, tt.valid, isValid)
		})
	}
}

func TestGetProxyServicesWithInvalidYAML(t *testing.T) {
	tests := []struct {
		name          string
		configContent string
		shouldLoad    bool
	}{
		{
			name: "invalid yaml structure",
			configContent: `
proxy:
  services: "not-an-array"
`,
			shouldLoad: true, // This will load but return empty array
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			err := os.WriteFile(configPath, []byte(tt.configContent), 0644)
			assert.NoError(t, err)

			viper.Reset()

			// This might fail to load or return empty services
			err = LoadConfig(configPath)
			if tt.shouldLoad {
				assert.NoError(t, err)
			}

			services := GetProxyServices()

			// Should return empty array on error or invalid structure
			assert.NotNil(t, services)
		})
	}
}

func TestGetProxyServicesEnabledFiltering(t *testing.T) {
	configContent := `
proxy:
  services:
    - name: enabled-service
      prefix: /api/enabled
      target: http://enabled:8080
      enabled: true
    - name: disabled-service
      prefix: /api/disabled
      target: http://disabled:8080
      enabled: false
    - name: another-enabled
      prefix: /api/another
      target: http://another:8080
      enabled: true
`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	assert.NoError(t, err)

	viper.Reset()
	err = LoadConfig(configPath)
	assert.NoError(t, err)

	services := GetProxyServices()
	assert.Equal(t, 3, len(services))

	// Count enabled services
	enabledCount := 0
	for _, svc := range services {
		if svc.Enabled {
			enabledCount++
		}
	}
	assert.Equal(t, 2, enabledCount)
}

func TestProxyServiceDefaults(t *testing.T) {
	// Test that GetProxyServices returns empty array when no config is set
	viper.Reset()

	services := GetProxyServices()
	// GetProxyServices returns []ProxyService{} which is not nil
	assert.Equal(t, 0, len(services))
}
