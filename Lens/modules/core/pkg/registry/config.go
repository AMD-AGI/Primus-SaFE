// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/config"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// ConfigKey is the system_config key for container registry settings
const ConfigKey = "container_registry"

// Category for system_config
const ConfigCategory = "infrastructure"

// DefaultRegistry is the default registry when no configuration is set
const DefaultRegistry = "docker.io"

// DefaultNamespace is the default image namespace
const DefaultNamespace = "primussafe"

// DefaultTag is the default image tag when no version is configured
const DefaultTag = "latest"

// Known image names
const (
	ImageTraceLens      = "tracelens"
	ImagePerfettoViewer = "perfetto-viewer"
)

// Config represents the container registry configuration
type Config struct {
	// Registry is the container registry hostname (e.g., "harbor.example.com", "docker.io")
	Registry string `json:"registry"`

	// Namespace is the image namespace/project (e.g., "primussafe")
	Namespace string `json:"namespace"`

	// HarborExternalURL is the external URL of Harbor (if using Harbor)
	// This is used to construct full image URLs
	HarborExternalURL string `json:"harbor_external_url,omitempty"`

	// ImageVersions contains specific version tags for images
	// Key: image name, Value: tag (e.g., {"tracelens": "202501051200", "perfetto-viewer": "latest"})
	// If not set for an image, defaults to "latest"
	ImageVersions map[string]string `json:"image_versions,omitempty"`
}

// GetImageURL returns the full image URL for a given image name
// Format: {registry}/{namespace}/{imageName}:{tag}
// Priority: ImageVersions[imageName] > defaultTag > "latest"
func (c *Config) GetImageURL(imageName, defaultTag string) string {
	registry := c.Registry
	if registry == "" {
		registry = DefaultRegistry
	}

	namespace := c.Namespace
	if namespace == "" {
		namespace = DefaultNamespace
	}

	// Priority: config version > default tag > "latest"
	tag := DefaultTag
	if defaultTag != "" {
		tag = defaultTag
	}
	if c.ImageVersions != nil {
		if configVersion, ok := c.ImageVersions[imageName]; ok && configVersion != "" {
			tag = configVersion
		}
	}

	// docker.io images don't need the hostname prefix for pulling
	// but we include it for consistency
	return fmt.Sprintf("%s/%s/%s:%s", registry, namespace, imageName, tag)
}

// GetDefaultImageURL returns the default image URL using only system_config
// This is the recommended way to get image URLs - no hardcoded values
func (c *Config) GetDefaultImageURL(imageName string) string {
	return c.GetImageURL(imageName, DefaultTag)
}

// GetConfig retrieves the container registry configuration for a cluster
// Returns default config if not found
func GetConfig(ctx context.Context, clusterName string) (*Config, error) {
	mgr := config.NewManagerForCluster(clusterName)

	var cfg Config
	err := mgr.Get(ctx, ConfigKey, &cfg)
	if err != nil {
		// If config not found, return default
		if strings.Contains(err.Error(), "not found") {
			log.Debugf("Container registry config not found for cluster %s, using defaults", clusterName)
			return &Config{
				Registry:  DefaultRegistry,
				Namespace: DefaultNamespace,
			}, nil
		}
		return nil, fmt.Errorf("failed to get registry config: %w", err)
	}

	return &cfg, nil
}

// SetConfig sets the container registry configuration for a cluster
func SetConfig(ctx context.Context, clusterName string, cfg *Config, updatedBy string) error {
	mgr := config.NewManagerForCluster(clusterName)

	return mgr.Set(ctx, ConfigKey, cfg,
		config.WithDescription("Container registry configuration for pulling pod images"),
		config.WithCategory(ConfigCategory),
		config.WithUpdatedBy(updatedBy),
		config.WithRecordHistory(true),
		config.WithChangeReason("Registry configuration update"),
	)
}

// GetImageURLForCluster is a convenience function to get image URL for a cluster
// The defaultTag parameter is used as fallback if no version is configured in system_config
func GetImageURLForCluster(ctx context.Context, clusterName, imageName, defaultTag string) string {
	cfg, err := GetConfig(ctx, clusterName)
	if err != nil {
		log.Warnf("Failed to get registry config for cluster %s: %v, using default", clusterName, err)
		cfg = &Config{
			Registry:  DefaultRegistry,
			Namespace: DefaultNamespace,
		}
	}

	return cfg.GetImageURL(imageName, defaultTag)
}

// GetDefaultImageURLForCluster returns the image URL using only system_config settings
// This is the recommended way - all configuration comes from system_config:
// - registry: from config or default "docker.io"
// - namespace: from config or default "primussafe"
// - version: from config.ImageVersions[imageName] or default "latest"
func GetDefaultImageURLForCluster(ctx context.Context, clusterName, imageName string) string {
	cfg, err := GetConfig(ctx, clusterName)
	if err != nil {
		log.Warnf("Failed to get registry config for cluster %s: %v, using default", clusterName, err)
		cfg = &Config{
			Registry:  DefaultRegistry,
			Namespace: DefaultNamespace,
		}
	}

	return cfg.GetDefaultImageURL(imageName)
}

// SyncFromHarborSecret syncs the registry configuration from Harbor's harbor-core secret
// This is a manual sync operation that reads the Harbor external URL from the secret
// and updates the system_config
//
// Parameters:
// - ctx: context
// - clusterName: target cluster name for storing config
// - harborExternalURL: the Harbor external URL (e.g., "https://harbor.example.com")
// - updatedBy: user performing the sync
func SyncFromHarborSecret(ctx context.Context, clusterName string, harborExternalURL string, updatedBy string) error {
	// Parse the URL to extract the hostname
	hostname := harborExternalURL

	// Remove protocol prefix if present
	hostname = strings.TrimPrefix(hostname, "https://")
	hostname = strings.TrimPrefix(hostname, "http://")

	// Remove trailing slash if present
	hostname = strings.TrimSuffix(hostname, "/")

	cfg := &Config{
		Registry:          hostname,
		Namespace:         DefaultNamespace,
		HarborExternalURL: harborExternalURL,
	}

	return SetConfig(ctx, clusterName, cfg, updatedBy)
}

