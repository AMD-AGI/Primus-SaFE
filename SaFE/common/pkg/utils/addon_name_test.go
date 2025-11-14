/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGenAddonName tests the addon name generation logic
// This is extracted from addon.go genAddonName function for testing
func TestGenAddonName(t *testing.T) {
	tests := []struct {
		name         string
		cluster      string
		namespace    string
		releaseName  string
		expectedName string
	}{
		{
			name:         "all fields provided",
			cluster:      "prod-cluster",
			namespace:    "monitoring",
			releaseName:  "prometheus",
			expectedName: "prod-cluster-monitoring-prometheus",
		},
		{
			name:         "empty namespace should use default",
			cluster:      "test-cluster",
			namespace:    "",
			releaseName:  "grafana",
			expectedName: "test-cluster-default-grafana",
		},
		{
			name:         "simple case",
			cluster:      "dev",
			namespace:    "kube-system",
			releaseName:  "metrics-server",
			expectedName: "dev-kube-system-metrics-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := genAddonName(tt.cluster, tt.namespace, tt.releaseName)
			assert.Equal(t, tt.expectedName, result)
		})
	}
}

// genAddonName generates a unique name for an addon
// Format: {cluster}-{namespace}-{releaseName}
// If namespace is empty, uses "default"
func genAddonName(cluster, namespace, releaseName string) string {
	if namespace == "" {
		return cluster + "-" + "default" + "-" + releaseName
	}
	return cluster + "-" + namespace + "-" + releaseName
}
