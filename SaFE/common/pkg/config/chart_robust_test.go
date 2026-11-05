/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package config

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	testifyassert "github.com/stretchr/testify/assert"
	testifyrequire "github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// robustTokenCallers are the components that call the Robust API through
// robustclient and therefore need its service token.
var robustTokenCallers = []string{"apiserver", "job-manager", "resource-manager"}

type renderedPodSecrets struct {
	mounts  map[string]string // volume name -> mount path, over all containers
	secrets map[string]struct {
		name     string
		optional bool
	}
}

// renderDeploymentSecrets returns the secret volumes and their mounts of each
// rendered Deployment, keyed by Deployment name.
func renderDeploymentSecrets(t *testing.T, values ...string) map[string]renderedPodSecrets {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	cmd := exec.Command("helm", append([]string{"template", "rel", chartPath}, values...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	testifyrequire.NoErrorf(t, err, "helm template failed:\n%s", stderr.String())

	result := map[string]renderedPodSecrets{}
	decoder := yaml.NewDecoder(bytes.NewReader(out))
	for {
		var doc struct {
			Kind     string `yaml:"kind"`
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct {
							VolumeMounts []struct {
								Name      string `yaml:"name"`
								MountPath string `yaml:"mountPath"`
							} `yaml:"volumeMounts"`
						} `yaml:"containers"`
						Volumes []struct {
							Name   string `yaml:"name"`
							Secret *struct {
								SecretName string `yaml:"secretName"`
								Optional   bool   `yaml:"optional"`
							} `yaml:"secret"`
						} `yaml:"volumes"`
					} `yaml:"spec"`
				} `yaml:"template"`
			} `yaml:"spec"`
		}
		if err := decoder.Decode(&doc); err != nil {
			testifyrequire.ErrorIsf(t, err, io.EOF, "rendered chart is not valid YAML: %v", err)
			break
		}
		if doc.Kind != "Deployment" {
			continue
		}
		pod := renderedPodSecrets{mounts: map[string]string{}, secrets: map[string]struct {
			name     string
			optional bool
		}{}}
		for _, c := range doc.Spec.Template.Spec.Containers {
			for _, m := range c.VolumeMounts {
				pod.mounts[m.Name] = m.MountPath
			}
		}
		for _, v := range doc.Spec.Template.Spec.Volumes {
			if v.Secret != nil {
				pod.secrets[v.Name] = struct {
					name     string
					optional bool
				}{v.Secret.SecretName, v.Secret.Optional}
			}
		}
		result[doc.Metadata.Name] = pod
	}
	return result
}

// TestChartGivesEveryRobustCallerTheServiceToken pins the whole path for each
// caller: the Secret is mounted (optional, so a missing Secret cannot stop the
// pod), the rendered config points robust.secret_path at the mount, and the
// getter reads the token from a file laid out the way the kubelet writes it.
func TestChartGivesEveryRobustCallerTheServiceToken(t *testing.T) {
	deployments := renderDeploymentSecrets(t)
	for _, component := range robustTokenCallers {
		t.Run(component, func(t *testing.T) {
			pod, ok := deployments["rel-"+component]
			testifyrequire.True(t, ok, "no rel-%s Deployment rendered", component)
			vol, ok := pod.secrets["robust-service-token"]
			testifyrequire.True(t, ok, "rel-%s has no robust-service-token secret volume", component)
			testifyassert.Equal(t, "robust-service-token", vol.name)
			testifyassert.True(t, vol.optional, "the Secret must be optional")
			mountPath := pod.mounts["robust-service-token"]
			testifyassert.Equal(t, "/etc/secrets/robust", mountPath)

			rendered := renderConfigMapData(t, "release-name-"+component, "config.yaml")
			loadRendered(t, rendered)
			testifyassert.Equal(t, mountPath, viper.GetString(robustSecretPath),
				"the config must read the token where the Deployment mounts it")

			// Re-point the configured directory at a temp copy of the mount.
			dir := t.TempDir()
			testifyrequire.NoError(t, os.WriteFile(filepath.Join(dir, RobustServiceTokenKey), []byte("tok\n"), 0o600))
			viper.Set(robustSecretPath, dir)
			testifyassert.Equal(t, "tok", GetRobustServiceToken())
		})
	}
}

func TestChartCanOmitTheRobustServiceToken(t *testing.T) {
	deployments := renderDeploymentSecrets(t, "--set", "robust.service_token_secret=")
	for _, component := range robustTokenCallers {
		pod := deployments["rel-"+component]
		_, ok := pod.secrets["robust-service-token"]
		testifyassert.False(t, ok, component)
		rendered := renderConfigMapData(t, "release-name-"+component, "config.yaml", "--set", "robust.service_token_secret=")
		testifyassert.False(t, strings.Contains(rendered, "robust:"), component)
	}
}

func TestChartRendersACustomRobustSecret(t *testing.T) {
	values := []string{"--set", "robust.service_token_secret=other", "--set", "robust.secret_path=/var/run/robust"}
	deployments := renderDeploymentSecrets(t, values...)
	for _, component := range robustTokenCallers {
		pod := deployments["rel-"+component]
		testifyassert.Equal(t, "other", pod.secrets["robust-service-token"].name, component)
		testifyassert.Equal(t, "/var/run/robust", pod.mounts["robust-service-token"], component)
		loadRendered(t, renderConfigMapData(t, "release-name-"+component, "config.yaml", values...))
		testifyassert.Equal(t, "/var/run/robust", viper.GetString(robustSecretPath), component)
	}
}

// TestRobustServiceTokenNeedsAConfiguredDirectory: with robust.secret_path
// unset the getter must not fall back to a file in the working directory.
func TestRobustServiceTokenNeedsAConfiguredDirectory(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	dir := t.TempDir()
	testifyrequire.NoError(t, os.WriteFile(filepath.Join(dir, RobustServiceTokenKey), []byte("stray"), 0o600))
	t.Chdir(dir)
	testifyassert.Equal(t, "", GetRobustServiceToken())
}
