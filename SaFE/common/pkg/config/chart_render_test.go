/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// chartPath is the chart that renders the config file the apiserver reads.
const chartPath = "../../../charts/primus-safe"

// renderApiserverConfig renders the chart and returns the apiserver's config.yaml.
// It skips the test where helm is unavailable.
func renderApiserverConfig(t *testing.T, values ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	if _, err := os.Stat(chartPath); err != nil {
		t.Skipf("chart not found at %s", chartPath)
	}

	args := append([]string{"template", chartPath}, values...)
	out, err := exec.Command("helm", args...).Output()
	testifyassert.NoError(t, err)

	decoder := yaml.NewDecoder(strings.NewReader(string(out)))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Data map[string]string `yaml:"data"`
		}
		if err := decoder.Decode(&doc); err != nil {
			break
		}
		if doc.Kind == "ConfigMap" && strings.Contains(doc.Metadata.Name, "apiserver") {
			if cfg, ok := doc.Data["config.yaml"]; ok {
				return cfg
			}
		}
	}
	t.Fatal("no apiserver config.yaml in the rendered chart")
	return ""
}

// loadRendered writes the rendered config where LoadConfig can read it, so the
// getters are driven by the same file the apiserver reads in the cluster.
func loadRendered(t *testing.T, rendered string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	path := filepath.Join(t.TempDir(), "config.yaml")
	testifyassert.NoError(t, os.WriteFile(path, []byte(rendered), 0o600))
	testifyassert.NoError(t, LoadConfig(path))
}

// TestChartRendersReverseForwardDefaults pins that a default install renders a
// config the getters read as "remote forwarding is off".
func TestChartRendersReverseForwardDefaults(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t))

	testifyassert.False(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, []string{"127.0.0.1"}, GetSSHReverseForwardBindAddresses())
	testifyassert.Equal(t, 10000, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 19999, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 8, GetSSHReverseForwardMaxPerSession())
}

// TestChartRendersReverseForwardOverrides pins that turning the feature on in values
// reaches the getters - the wiring an operator depends on when enabling it.
func TestChartRendersReverseForwardOverrides(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t,
		"--set", "ssh.reverse_forward.enable=true",
		"--set", "ssh.reverse_forward.port_min=20000",
		"--set", "ssh.reverse_forward.port_max=20010",
		"--set", "ssh.reverse_forward.max_forwards_per_session=3",
		"--set", "ssh.reverse_forward.bind_addresses={127.0.0.1,0.0.0.0}",
	))

	testifyassert.True(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, []string{"127.0.0.1", "0.0.0.0"}, GetSSHReverseForwardBindAddresses())
	testifyassert.Equal(t, 20000, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 20010, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 3, GetSSHReverseForwardMaxPerSession())
}
