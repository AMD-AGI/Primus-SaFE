// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// StandaloneConfig holds installer configuration loaded from a YAML file (e.g. Helm chart values).
// Used when running "install dataplane --config <path>" without a control plane.
type StandaloneConfig struct {
	Namespace     string
	ClusterName   string
	StorageClass  string
	ImageRegistry string
	MergedValues  map[string]interface{}
}

// LoadFromFile loads configuration from a YAML file (values.yaml from the Helm chart ConfigMap).
// Expected structure: global.namespace, global.clusterName, global.storageClass, global.imageRegistry, etc.
func LoadFromFile(path string) (*StandaloneConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config YAML: %w", err)
	}

	out := &StandaloneConfig{
		Namespace:     "primus-lens",
		ClusterName:   "default",
		StorageClass:  "local-path",
		ImageRegistry: "docker.io",
		MergedValues:  raw,
	}

	if g, ok := raw["global"].(map[string]interface{}); ok {
		if v, ok := g["namespace"].(string); ok && v != "" {
			out.Namespace = v
		}
		if v, ok := g["clusterName"].(string); ok && v != "" {
			out.ClusterName = v
		}
		if v, ok := g["storageClass"].(string); ok && v != "" {
			out.StorageClass = v
		}
		if v, ok := g["imageRegistry"].(string); ok && v != "" {
			out.ImageRegistry = v
		}
	}

	return out, nil
}
