/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"bytes"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode"

	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"
	testifyrequire "github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// chartPath is the chart that renders the configuration consumed by SaFE services.
const chartPath = "../../../charts/primus-safe"

func renderConfigMapData(t *testing.T, name, key string, values ...string) string {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	if _, err := os.Stat(chartPath); err != nil {
		t.Skipf("chart not found at %s", chartPath)
	}

	args := append([]string{"template", chartPath}, values...)
	cmd := exec.Command("helm", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	// helm puts the reason on stderr, and nothing below can run without a render.
	testifyrequire.NoErrorf(t, err, "helm template failed:\n%s", stderr.String())

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
			// End of stream is the loop's exit; anything else means the chart
			// rendered something that is not YAML, which is worth saying out loud.
			testifyrequire.ErrorIsf(t, err, io.EOF, "rendered chart is not valid YAML: %v\n%s", err, out)
			break
		}
		if doc.Kind == "ConfigMap" && strings.Contains(doc.Metadata.Name, name) {
			if cfg, ok := doc.Data[key]; ok {
				return cfg
			}
		}
	}
	t.Fatalf("no %s key in rendered ConfigMap %s", key, name)
	return ""
}

func renderApiserverConfig(t *testing.T, values ...string) string {
	t.Helper()
	return renderConfigMapData(t, "apiserver", "config.yaml", values...)
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

// TestChartRendersReverseForwardDefaults pins what a stock install actually gets:
// remote forwarding on, bound to loopback inside the pod, and limited.
func TestChartRendersReverseForwardDefaults(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t))

	testifyassert.True(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, []string{"127.0.0.1"}, GetSSHReverseForwardBindAddresses())
	testifyassert.Equal(t, 1024, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 65535, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 8, GetSSHReverseForwardMaxPerSession())
}

// TestChartRendersReverseForwardOverrides pins that values written by an operator
// reach the getters, including turning the feature off - the case Helm's `default`
// used to swallow.
func TestChartRendersReverseForwardOverrides(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t,
		"--set", "ssh.reverse_forward.enable=false",
		"--set", "ssh.reverse_forward.port_min=20000",
		"--set", "ssh.reverse_forward.port_max=20010",
		"--set", "ssh.reverse_forward.max_forwards_per_session=3",
		"--set", "ssh.reverse_forward.bind_addresses={127.0.0.1,0.0.0.0}",
	))

	// Turning it off has to reach the getters too. Helm's `default` treats false as
	// absent, so writing enable: false once rendered back to on.
	testifyassert.False(t, IsSSHReverseForwardEnable())
	testifyassert.Equal(t, []string{"127.0.0.1", "0.0.0.0"}, GetSSHReverseForwardBindAddresses())
	testifyassert.Equal(t, 20000, GetSSHReverseForwardPortMin())
	testifyassert.Equal(t, 20010, GetSSHReverseForwardPortMax())
	testifyassert.Equal(t, 3, GetSSHReverseForwardMaxPerSession())
}

// TestChartRendersReverseForwardEnabledExplicitly covers the other direction: an
// operator who writes the default out in full gets what they wrote, rather than a
// render that only works while the value is absent.
func TestChartRendersReverseForwardEnabledExplicitly(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t, "--set", "ssh.reverse_forward.enable=true"))
	testifyassert.True(t, IsSSHReverseForwardEnable())
}

// TestChartRendersAnEmptyBindListAsEmpty pins the other half of the same trap: an
// operator who removes every bind address gets none, not the default put back.
func TestChartRendersAnEmptyBindListAsEmpty(t *testing.T) {
	loadRendered(t, renderApiserverConfig(t, "--set", "ssh.reverse_forward.bind_addresses={}"))
	testifyassert.Empty(t, GetSSHReverseForwardBindAddresses())
}

func TestChartRendersCICDProxyRelayAsNativeSidecar(t *testing.T) {
	type container struct {
		Name          string `yaml:"name"`
		RestartPolicy string `yaml:"restartPolicy"`
		Resources     struct {
			Limits   map[string]string `yaml:"limits"`
			Requests map[string]string `yaml:"requests"`
		} `yaml:"resources"`
	}
	var runner struct {
		Spec struct {
			Spec struct {
				Containers     []container `yaml:"containers"`
				InitContainers []container `yaml:"initContainers"`
			} `yaml:"spec"`
		} `yaml:"spec"`
	}
	rendered := renderConfigMapData(t, "github-runner-template", "template",
		"--show-only", "templates/configmap/github_runner_template.yaml",
		"--set", "cicd.proxy_relay_image=example/proxy-relay:latest")
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))

	for _, current := range runner.Spec.Spec.Containers {
		testifyassert.NotEqual(t, "proxy-relay", current.Name)
	}
	for _, current := range runner.Spec.Spec.InitContainers {
		if current.Name != "proxy-relay" {
			continue
		}
		testifyassert.Equal(t, "Always", current.RestartPolicy)
		testifyassert.Equal(t, map[string]string{"cpu": "500m", "memory": "256Mi"}, current.Resources.Limits)
		testifyassert.Empty(t, current.Resources.Requests)
		testifyassert.NotContains(t, current.Resources.Limits, "amd.com/gpu")
		return
	}
	t.Fatal("proxy-relay init container not found")
}

func TestChartRendersCICDProxyRelayCredentialEncoding(t *testing.T) {
	type container struct {
		Name string   `yaml:"name"`
		Args []string `yaml:"args"`
	}
	var runner struct {
		Spec struct {
			Spec struct {
				InitContainers []container `yaml:"initContainers"`
			} `yaml:"spec"`
		} `yaml:"spec"`
	}
	rendered := renderConfigMapData(t, "github-runner-template", "template",
		"--show-only", "templates/configmap/github_runner_template.yaml",
		"--set", "cicd.proxy_relay_image=example/proxy-relay:latest")
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))

	var script string
	for _, current := range runner.Spec.Spec.InitContainers {
		if current.Name == "proxy-relay" {
			testifyrequire.Len(t, current.Args, 1)
			script = current.Args[0]
			break
		}
	}
	testifyrequire.NotEmpty(t, script)
	testifyassert.Contains(t, script, "username=$(percent_encode </etc/secrets/proxy/username)")
	testifyassert.Contains(t, script, "password=$(percent_encode </etc/secrets/proxy/password)")
	testifyassert.Contains(t, script, `login=" login=${username}:${password}"`)

	start := strings.Index(script, "percent_encode() {")
	testifyrequire.NotEqual(t, -1, start)
	end := strings.Index(script[start:], "\n}")
	testifyrequire.NotEqual(t, -1, end)
	encoder := script[start : start+end+2]
	encode := func(t *testing.T, value string) string {
		t.Helper()
		cmd := exec.Command("/bin/sh", "-c", encoder+"\npercent_encode")
		cmd.Stdin = strings.NewReader(value)
		out, err := cmd.CombinedOutput()
		testifyrequire.NoErrorf(t, err, "relay credential encoder failed: %s", out)
		return string(out)
	}

	encodedUsername := encode(t, "proxy user%#")
	testifyassert.Regexp(t, `^(%[0-9a-f]{2})+$`, encodedUsername)
	for _, password := range []string{"p@ss w0rd", "p%40ss", "päss🔒"} {
		t.Run(password, func(t *testing.T) {
			encodedPassword := encode(t, password)
			testifyassert.Regexp(t, `^(%[0-9a-f]{2})+$`, encodedPassword)
			cachePeer := "cache_peer proxy.example parent 3128 0 no-query default login=" + encodedUsername + ":" + encodedPassword
			fields := strings.Fields(cachePeer)
			testifyrequire.Len(t, fields, 8)
			testifyassert.Equal(t, "login="+encodedUsername+":"+encodedPassword, fields[7])
			testifyassert.Equal(t, -1, strings.IndexFunc(fields[7], unicode.IsSpace))

			credentials := strings.SplitN(strings.TrimPrefix(fields[7], "login="), ":", 2)
			testifyrequire.Len(t, credentials, 2)
			username, err := url.PathUnescape(credentials[0])
			testifyrequire.NoError(t, err)
			decodedPassword, err := url.PathUnescape(credentials[1])
			testifyrequire.NoError(t, err)
			testifyassert.Equal(t, "proxy user%#", username)
			testifyassert.Equal(t, password, decodedPassword)
		})
	}
}
