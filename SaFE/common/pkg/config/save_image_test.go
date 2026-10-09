/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSaveImageCluster(t *testing.T) {
	viper.Reset()
	defer viper.Reset()
	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`save_image:
  clusters:
  - cluster: edge
    target_registry: central.example.com
    target_project: custom
    staging_registry: edge.example.com
    staging_project: save-staging
    staging_ca_secret: harbor/edge-ca
    replication_timeout_second: 600
    host_aliases:
    - ip: 10.0.0.5
      hostnames: [edge.example.com]
  - cluster: Central
    target_registry: central.example.com
`), 0o600))
	require.NoError(t, LoadConfig(file))

	c, ok, err := GetSaveImageCluster("edge")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, SaveImageCluster{
		Cluster: "edge", TargetRegistry: "central.example.com", TargetProject: "custom",
		StagingRegistry: "edge.example.com", StagingProject: "save-staging", StagingCASecret: "harbor/edge-ca",
		ReplicationTimeoutSecond: 600,
		HostAliases:              []HostAlias{{IP: "10.0.0.5", Hostnames: []string{"edge.example.com"}}},
	}, c)

	// Cluster IDs are matched exactly; they are values, not keys viper folds to lower case.
	_, ok, err = GetSaveImageCluster("Central")
	require.NoError(t, err)
	assert.True(t, ok)
	_, ok, err = GetSaveImageCluster("central")
	require.NoError(t, err)
	assert.False(t, ok)

	viper.Set(saveImageClusters, "not a list")
	_, _, err = GetSaveImageCluster("edge")
	assert.Error(t, err, "settings that cannot be read are not an absence")
}

// The chart renders the operator's entries into both services that read them: the
// resource manager (where to stage and publish) and the job manager (host aliases).
func TestChartRendersSaveImageClusters(t *testing.T) {
	values := filepath.Join(t.TempDir(), "values.yaml")
	require.NoError(t, os.WriteFile(values, []byte(`save_image:
  clusters:
  - cluster: edge
    target_registry: central.example.com
    staging_registry: edge.example.com
    replication_timeout_second: 900
    host_aliases:
    - ip: 10.0.0.5
      hostnames: [edge.example.com]
`), 0o600))
	for _, component := range []string{"resource-manager", "job-manager"} {
		loadRendered(t, renderConfigMapData(t, component, "config.yaml", "-f", values))
		c, ok, err := GetSaveImageCluster("edge")
		require.NoError(t, err, component)
		require.True(t, ok, component)
		assert.Equal(t, "edge.example.com", c.StagingRegistry, component)
		assert.Equal(t, 900, c.ReplicationTimeoutSecond, component)
		assert.Equal(t, []HostAlias{{IP: "10.0.0.5", Hostnames: []string{"edge.example.com"}}}, c.HostAliases, component)
	}
	loadRendered(t, renderConfigMapData(t, "job-manager", "config.yaml"))
	_, ok, err := GetSaveImageCluster("edge")
	require.NoError(t, err, "a stock install renders an empty list")
	assert.False(t, ok)
}
