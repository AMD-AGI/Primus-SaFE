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

func TestChartRendersCICDProxyRelayUpstreamDomains(t *testing.T) {
	// The relay exists to tunnel what GitHub's IP allow list gates. Defaulting the list
	// to empty would instead force every destination upstream with no direct fallback,
	// so package mirrors and internal registries would fail as opaque tunnel errors.
	domains := func(t *testing.T, values ...string) string {
		t.Helper()
		rendered := renderConfigMapData(t, "github-runner-template", "template",
			append([]string{"--show-only", "templates/configmap/github_runner_template.yaml",
				"--set", "cicd.proxy_relay_image=example/proxy-relay:latest"}, values...)...)
		var runner struct {
			Spec struct {
				Spec struct {
					InitContainers []struct {
						Name string `yaml:"name"`
						Env  []struct {
							Name  string `yaml:"name"`
							Value string `yaml:"value"`
						} `yaml:"env"`
					} `yaml:"initContainers"`
				} `yaml:"spec"`
			} `yaml:"spec"`
		}
		testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))
		for _, current := range runner.Spec.Spec.InitContainers {
			if current.Name != "proxy-relay" {
				continue
			}
			for _, env := range current.Env {
				if env.Name == "PROXY_UPSTREAM_DOMAINS" {
					return env.Value
				}
			}
			t.Fatal("PROXY_UPSTREAM_DOMAINS not found on the relay")
		}
		t.Fatal("proxy-relay init container not found")
		return ""
	}

	testifyassert.Equal(t, ".github.com .githubusercontent.com .ghcr.io", domains(t))
	testifyassert.Equal(t, ".corp.example",
		domains(t, "--set", "cicd.proxy_relay_upstream_domains=.corp.example"))
	// An explicit empty value stays the all-upstream escape hatch.
	testifyassert.Equal(t, "", domains(t, "--set", "cicd.proxy_relay_upstream_domains="))
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
	testifyassert.Contains(t, script, "pw=$(percent_encode </etc/secrets/proxy/password)")
	testifyassert.Contains(t, script, `login=" login=${username}:${pw}"`)

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

func TestChartRendersHostedRunnerRuntime(t *testing.T) {
	type container struct {
		Name            string   `yaml:"name"`
		Image           string   `yaml:"image"`
		RestartPolicy   string   `yaml:"restartPolicy"`
		Args            []string `yaml:"args"`
		SecurityContext struct {
			Privileged bool `yaml:"privileged"`
			RunAsUser  int  `yaml:"runAsUser"`
		} `yaml:"securityContext"`
	}
	var runner struct {
		Spec struct {
			Template struct {
				Spec struct {
					DNSPolicy                     string      `yaml:"dnsPolicy"`
					TerminationGracePeriodSeconds int         `yaml:"terminationGracePeriodSeconds"`
					Containers                    []container `yaml:"containers"`
					InitContainers                []container `yaml:"initContainers"`
				} `yaml:"spec"`
			} `yaml:"template"`
		} `yaml:"spec"`
	}
	rendered := renderConfigMapData(t, "github-hosted-runner-template", "template",
		"--show-only", "templates/configmap/github_hosted_runner_template.yaml")
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &runner))

	testifyassert.Equal(t, "ClusterFirst", runner.Spec.Template.Spec.DNSPolicy)
	testifyassert.Equal(t, 180, runner.Spec.Template.Spec.TerminationGracePeriodSeconds)
	testifyrequire.Len(t, runner.Spec.Template.Spec.Containers, 1)
	testifyassert.Equal(t, "ghcr.io/actions/actions-runner:2.328.0",
		runner.Spec.Template.Spec.Containers[0].Image)
	testifyassert.False(t, runner.Spec.Template.Spec.Containers[0].SecurityContext.Privileged)
	testifyassert.Equal(t, 1001, runner.Spec.Template.Spec.Containers[0].SecurityContext.RunAsUser)

	names := map[string]container{}
	for _, current := range runner.Spec.Template.Spec.InitContainers {
		names[current.Name] = current
	}
	testifyassert.Equal(t, "ghcr.io/actions/actions-runner:2.328.0", names["init-dind-externals"].Image)
	testifyassert.Equal(t, "docker:28.3.3-dind", names["dind"].Image)
	testifyassert.Equal(t, "Always", names["dind"].RestartPolicy)
	testifyassert.True(t, names["dind"].SecurityContext.Privileged)
	testifyassert.Equal(t, []string{
		"dockerd",
		"--host=unix:///var/run/docker.sock",
		"--group=123",
	}, names["dind"].Args)
}

// sandboxPod is the part of the rendered sandbox template the extra CA touches.
type sandboxPod struct {
	Spec struct {
		PodTemplate struct {
			Spec struct {
				Containers []struct {
					Name    string   `yaml:"name"`
					Command []string `yaml:"command"`
					Args    []string `yaml:"args"`
					Env     []struct {
						Name      string                 `yaml:"name"`
						Value     string                 `yaml:"value"`
						ValueFrom map[string]interface{} `yaml:"valueFrom"`
					} `yaml:"env"`
					EnvFrom      []map[string]interface{} `yaml:"envFrom"`
					VolumeMounts []struct {
						Name      string `yaml:"name"`
						MountPath string `yaml:"mountPath"`
						ReadOnly  bool   `yaml:"readOnly"`
					} `yaml:"volumeMounts"`
				} `yaml:"containers"`
				InitContainers []struct {
					Name  string `yaml:"name"`
					Image string `yaml:"image"`
				} `yaml:"initContainers"`
				Volumes []struct {
					Name   string `yaml:"name"`
					Secret *struct {
						SecretName string `yaml:"secretName"`
					} `yaml:"secret"`
				} `yaml:"volumes"`
			} `yaml:"spec"`
		} `yaml:"podTemplate"`
	} `yaml:"spec"`
}

func renderSandboxPod(t *testing.T, values ...string) sandboxPod {
	t.Helper()
	args := append([]string{"--show-only", "templates/configmap/sandbox_template.yaml"}, values...)
	rendered := renderConfigMapData(t, "sandbox-template", "template", args...)
	var pod sandboxPod
	testifyrequire.NoError(t, yaml.Unmarshal([]byte(rendered), &pod))
	testifyrequire.Len(t, pod.Spec.PodTemplate.Spec.Containers, 1)
	return pod
}

var sandboxCAVariables = []string{"SSL_CERT_FILE", "REQUESTS_CA_BUNDLE", "CURL_CA_BUNDLE", "NODE_EXTRA_CA_CERTS"}

func TestChartRendersTheSandboxWithoutAnExtraCAByDefault(t *testing.T) {
	// The injector's setup script refuses to start a sandbox whose CA variables point
	// at the merged bundle when no certificate was found. Pointing them there with no
	// CA configured would stop every sandbox on a deployment that needs none.
	pod := renderSandboxPod(t)
	main := pod.Spec.PodTemplate.Spec.Containers[0]
	for _, current := range main.Env {
		testifyassert.NotContains(t, sandboxCAVariables, current.Name)
	}
	for _, current := range main.VolumeMounts {
		testifyassert.NotEqual(t, "extra-ca", current.Name)
	}
	for _, current := range pod.Spec.PodTemplate.Spec.Volumes {
		testifyassert.NotEqual(t, "extra-ca", current.Name)
	}
}

func TestChartMountsTheSandboxExtraCASecret(t *testing.T) {
	pod := renderSandboxPod(t, "--set", "sandbox.extra_ca_secret=sandbox-extra-ca")
	main := pod.Spec.PodTemplate.Spec.Containers[0]

	env := map[string]string{}
	for _, current := range main.Env {
		env[current.Name] = current.Value
	}
	for _, name := range sandboxCAVariables {
		testifyassert.Equal(t, "/shared/bin/ca-bundle.pem", env[name], name)
	}

	mounted := false
	for _, current := range main.VolumeMounts {
		if current.Name == "extra-ca" {
			mounted = true
			testifyassert.Equal(t, "/etc/claw/extra-ca", current.MountPath)
			testifyassert.True(t, current.ReadOnly)
		}
	}
	testifyassert.True(t, mounted, "extra-ca is not mounted in the main container")

	for _, current := range pod.Spec.PodTemplate.Spec.Volumes {
		if current.Name == "extra-ca" {
			testifyrequire.NotNil(t, current.Secret)
			testifyassert.Equal(t, "sandbox-extra-ca", current.Secret.SecretName)
			return
		}
	}
	t.Fatal("extra-ca volume not found")
}

func TestChartRendersNoSandboxEnvSecretByDefault(t *testing.T) {
	pod := renderSandboxPod(t)
	main := pod.Spec.PodTemplate.Spec.Containers[0]
	testifyassert.Empty(t, main.EnvFrom)
	for _, current := range main.Env {
		if current.Name == "NODE_NAME" {
			fieldRef, _ := current.ValueFrom["fieldRef"].(map[string]interface{})
			testifyassert.Equal(t, "spec.nodeName", fieldRef["fieldPath"])
			continue
		}
		testifyassert.Nil(t, current.ValueFrom, "%s reads from a reference", current.Name)
	}
}

func TestChartInjectsTheSandboxEnvSecret(t *testing.T) {
	plain := renderSandboxPod(t, "--set", "sandbox.extra_ca_secret=sandbox-extra-ca")
	pod := renderSandboxPod(t, "--set", "sandbox.env_secret=sandbox-env",
		"--set", "sandbox.extra_ca_secret=sandbox-extra-ca")
	main := pod.Spec.PodTemplate.Spec.Containers[0]
	// The whole Secret, by reference, optional; SecretEnvSource has no namespace, so it is the
	// Secret in the Pod's own (the workspace's) namespace.
	testifyassert.Equal(t, []map[string]interface{}{
		{"secretRef": map[string]interface{}{"name": "sandbox-env", "optional": true}},
	}, main.EnvFrom)
	// The explicit env is unchanged, and explicit env wins over envFrom in Kubernetes, so the
	// Secret cannot replace the CA variables.
	testifyassert.Equal(t, plain.Spec.PodTemplate.Spec.Containers[0].Env, main.Env)
}

// Only sandbox-env, the name the apiserver and resource-manager reserve whether or not the
// feature is on, is accepted: any other name would be guarded only once the apiserver had
// restarted with it, after the template already read it. A value helm parses as a number or a
// boolean (0, false) is refused too, not read as unset.
func TestChartRefusesAnySandboxEnvSecretNameButTheReservedOne(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	for _, name := range []string{"other", "sandbox-env2", "0", "false", "Not_A_Name", strings.Repeat("a", 254),
		"a\n- name: x"} {
		cmd := exec.Command("helm", "template", chartPath, "--show-only",
			"templates/configmap/sandbox_template.yaml", "--set", "sandbox.env_secret="+name)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		testifyrequire.Error(t, err, name)
		testifyassert.Contains(t, stderr.String(), "set it to sandbox-env or leave it empty", name)
		testifyassert.NotContains(t, string(out), "envFrom")
	}
}

// The apiserver needs no configuration for the guard, so the chart gives it none and turning the
// feature on does not restart it.
func TestChartLeavesTheApiserverAloneForTheSandboxEnvSecret(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	render := func(values ...string) string {
		args := append([]string{"template", chartPath, "--show-only", "templates/apiserver/deployment.yaml",
			"--show-only", "templates/apiserver/config.yaml"}, values...)
		out, err := exec.Command("helm", args...).Output()
		testifyrequire.NoError(t, err)
		return string(out)
	}
	testifyassert.Equal(t, render(), render("--set", "sandbox.env_secret=sandbox-env"))
}

// An image may set HSA_ENABLE_IPC_MODE_LEGACY=1 (ROCm vLLM images do). In an unprivileged Pod
// without host IPC the legacy amdkfd IPC import is refused, so every GPU-to-GPU IPC handle fails
// to open. The sandbox pins the default IPC path in the Pod spec, which overrides the image ENV
// for every process in the container, PID 1 included.
func TestChartPinsTheSandboxToTheNonLegacyHIPIPCPath(t *testing.T) {
	for _, values := range [][]string{nil, {"--set", "sandbox.extra_ca_secret=sandbox-extra-ca",
		"--set", "sandbox.env_secret=sandbox-env"}} {
		pod := renderSandboxPod(t, values...)
		main := pod.Spec.PodTemplate.Spec.Containers[0]
		found := 0
		for _, current := range main.Env {
			if current.Name == "HSA_ENABLE_IPC_MODE_LEGACY" {
				found++
				testifyassert.Equal(t, "0", current.Value, "values %v", values)
				testifyassert.Nil(t, current.ValueFrom, "values %v", values)
			}
		}
		testifyassert.Equal(t, 1, found, "HSA_ENABLE_IPC_MODE_LEGACY entries with values %v", values)
	}
}

// sandboxPathSetupScript is the PATH setup the startup command opens with, byte for byte. It is
// what pathSetupScript() in the PrimusClaw workload manager (sandbox/pkg/workloadmanager) returns
// for the sandboxes it builds itself, so a SaFE sandbox and a PrimusClaw one start with the same
// PATH from the same image. Change both together.
//
// It keeps the PATH the container starts with (the image's) first and in its order, drops entries
// that are empty, relative or contain a ".." segment, and appends each runtime directory it lacks
// once: the injected tools in /shared/bin, the workspace user's `pip install --user` scripts, and
// the system directories the startup command runs its own commands from.
const sandboxPathSetupScript = `__sandbox_path_rest="${PATH-}:"'/shared/bin:/home/sandbox/.local/bin:/usr/local/bin:/usr/bin:/bin:/sbin:'; __sandbox_path=; while [ -n "$__sandbox_path_rest" ]; do __sandbox_path_dir="${__sandbox_path_rest%%:*}"; __sandbox_path_rest="${__sandbox_path_rest#*:}"; case "$__sandbox_path_dir" in */../*|*/..) ;; /*) case ":$__sandbox_path:" in *":$__sandbox_path_dir:"*) ;; *) __sandbox_path="${__sandbox_path:+$__sandbox_path:}$__sandbox_path_dir" ;; esac ;; esac; done; PATH="$__sandbox_path"; export PATH; unset __sandbox_path_rest __sandbox_path_dir __sandbox_path; `

// sandboxStartupTail is what the startup command runs once PATH is set up.
const sandboxStartupTail = "mkdir -p /app /app/work && /shared/bin/setup-amd-ca.sh && if [ -n \"${HYPERLOOM_SIGNING_KEY_PEM-}\" ]; then keyfile=\"${HYPERLOOM_SIGNING_KEY_FILE:-/dev/shm/hyperloom-signing-key.pem}\"; case \"$keyfile\" in /app|/app/*) echo \"signing key path is under /app\" >&2; exit 1;; esac; mkdir -p \"$(dirname \"$keyfile\")\"; umask 077; printf '%s\\n' \"$HYPERLOOM_SIGNING_KEY_PEM\" > \"$keyfile\"; chmod 0400 \"$keyfile\"; unset HYPERLOOM_SIGNING_KEY_PEM; export HYPERLOOM_SIGNING_KEY_FILE=\"$keyfile\"; fi && if [ -n \"${SANDBOX_STARTUP-}\" ]; then eval \"$SANDBOX_STARTUP\"; fi && exec /shared/bin/envd --port=8080 --workspace=/app"

// sandboxRuntimePath is PATH when the container starts with none.
const sandboxRuntimePath = "/shared/bin:/home/sandbox/.local/bin:/usr/local/bin:/usr/bin:/bin:/sbin"

// A PATH env var on the container replaces the image's ENV PATH, so the template must set none,
// whatever else is turned on.
func TestChartSetsNoPathOnTheSandbox(t *testing.T) {
	for _, values := range [][]string{
		nil,
		{"--set", "sandbox.extra_ca_secret=sandbox-extra-ca", "--set", "sandbox.env_secret=sandbox-env"},
	} {
		pod := renderSandboxPod(t, values...)
		for _, current := range pod.Spec.PodTemplate.Spec.Containers[0].Env {
			testifyassert.NotEqual(t, "PATH", current.Name, "values %v", values)
		}
	}
}

// The rendered startup command is /bin/sh -c with exactly the PATH setup followed by the unchanged
// startup tail, whatever else is turned on: the chart's YAML quoting must not alter one byte of it.
func TestChartRendersTheSandboxPathSetupExactly(t *testing.T) {
	for _, values := range [][]string{
		nil,
		{"--set", "sandbox.extra_ca_secret=sandbox-extra-ca", "--set", "sandbox.env_secret=sandbox-env"},
	} {
		main := renderSandboxPod(t, values...).Spec.PodTemplate.Spec.Containers[0]
		testifyrequire.Equal(t, []string{"/bin/sh", "-c"}, main.Command, "values %v", values)
		testifyrequire.Equal(t, []string{sandboxPathSetupScript + sandboxStartupTail}, main.Args, "values %v", values)
		foundNode := false
		foundKey := false
		for _, current := range main.Env {
			if current.Name == "NODE_NAME" {
				foundNode = true
				fieldRef, _ := current.ValueFrom["fieldRef"].(map[string]interface{})
				testifyrequire.NotNil(t, fieldRef, "values %v", values)
				testifyassert.Equal(t, "spec.nodeName", fieldRef["fieldPath"], "values %v", values)
			}
			if current.Name == "HYPERLOOM_SIGNING_KEY_FILE" {
				foundKey = true
				testifyassert.Equal(t, "/run/signing-key/signing-key.pem", current.Value, "values %v", values)
				testifyassert.Nil(t, current.ValueFrom, "values %v", values)
			}
		}
		testifyrequire.True(t, foundNode, "values %v", values)
		testifyrequire.True(t, foundKey, "values %v", values)
	}
}

// The PATH setup, as rendered, runs under every POSIX shell on the host, since /bin/sh in a
// sandbox image may be any of them.
func TestChartSandboxStartupKeepsTheImagePath(t *testing.T) {
	args := renderSandboxPod(t).Spec.PodTemplate.Spec.Containers[0].Args
	testifyrequire.Len(t, args, 1)
	setup, found := strings.CutSuffix(args[0], sandboxStartupTail)
	testifyrequire.True(t, found, "startup command does not end in the startup tail: %s", args[0])
	var shells []string
	for _, name := range []string{"sh", "dash", "bash", "busybox"} {
		if shell, err := exec.LookPath(name); err == nil {
			shells = append(shells, shell)
		}
	}
	testifyrequire.NotEmpty(t, shells, "no POSIX shell on the host")
	cases := []struct {
		name, start, want string
	}{
		{"image path first, missing entries appended",
			"/opt/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"/opt/venv/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/shared/bin:/home/sandbox/.local/bin"},
		{"nothing duplicated when every entry is present", sandboxRuntimePath, sandboxRuntimePath},
		{"a directory only named as a prefix still counts as missing", "/shared/bin2:/bin/x",
			"/shared/bin2:/bin/x:" + sandboxRuntimePath},
		{"an empty path becomes exactly the runtime entries", "", sandboxRuntimePath},
		{"a duplicate in the image path is kept once", "/opt/a:/opt/b:/opt/a", "/opt/a:/opt/b:" + sandboxRuntimePath},
		{"relative and empty entries are dropped", "bin:/opt/a::.:~/bin:/opt/b:",
			"/opt/a:/opt/b:" + sandboxRuntimePath},
		{"entries with a .. segment are dropped", "..:/opt/../etc:/opt/a:/opt/b/..:/opt/..x:/..",
			"/opt/a:/opt/..x:" + sandboxRuntimePath},
		{"a directory with a space or a glob is kept literally", "/opt/my tools/bin:/opt/*:/opt/[a]",
			"/opt/my tools/bin:/opt/*:/opt/[a]:" + sandboxRuntimePath},
	}
	for _, shell := range shells {
		for _, current := range cases {
			command := []string{shell}
			switch filepath.Base(shell) {
			case "busybox":
				command = append(command, "sh")
			case "bash":
				// No startup files: a host's bashrc may change PATH before the script runs.
				command = append(command, "--norc", "--noprofile", "--posix")
			}
			// The setup unsets its own variables; exit 7 marks one it left behind.
			command = append(command, "-c", setup+`for v in __sandbox_path_rest __sandbox_path_dir __sandbox_path; do eval "[ -z \"\${$v+x}\" ]" || exit 7; done; printf '%s' "$PATH"`)
			cmd := exec.Command(command[0], command[1:]...)
			cmd.Env = []string{"PATH=" + current.start}
			out, err := cmd.Output()
			testifyrequire.NoError(t, err, "%s: %s", shell, current.name)
			testifyassert.Equal(t, current.want, string(out), "%s: %s", shell, current.name)
		}
	}
}

func TestChartPullsTheEnvdInjectorThroughTheSiteProxy(t *testing.T) {
	pod := renderSandboxPod(t, "--set", "global.sub_domain=site", "--set", "global.domain=example.com")
	testifyrequire.Len(t, pod.Spec.PodTemplate.Spec.InitContainers, 1)
	testifyassert.True(t, strings.HasPrefix(pod.Spec.PodTemplate.Spec.InitContainers[0].Image,
		"harbor.site.example.com/proxy/primussafe/agent-sandbox-envd-injector@sha256:"))
}

// TestChartRendersModelMaxConcurrentDownloads pins what an operator's value means: unset
// is 4, and 0 (which Helm's `default` used to replace with 4) as well as a negative
// value reach the getter as "no limit".
func TestChartRendersModelMaxConcurrentDownloads(t *testing.T) {
	render := func(values ...string) {
		loadRendered(t, renderConfigMapData(t, "resource-manager", "config.yaml", values...))
	}
	render()
	testifyassert.Equal(t, 4, GetModelMaxConcurrentDownloads())
	render("--set", "model.max_concurrent_downloads=2")
	testifyassert.Equal(t, 2, GetModelMaxConcurrentDownloads())
	render("--set", "model.max_concurrent_downloads=0")
	testifyassert.Equal(t, 0, GetModelMaxConcurrentDownloads())
	render("--set", "model.max_concurrent_downloads=-1")
	testifyassert.Equal(t, -1, GetModelMaxConcurrentDownloads())
}
