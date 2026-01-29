// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"gopkg.in/yaml.v3"
)

// HelmClient wraps Helm CLI operations
type HelmClient struct {
	chartRepo       string
	timeout         time.Duration
	debug           bool
	useLocalCharts  bool
	localChartsPath string
}

// NewHelmClient creates a new HelmClient
func NewHelmClient() *HelmClient {
	chartRepo := os.Getenv("HELM_CHART_REPO")
	if chartRepo == "" {
		chartRepo = "oci://docker.io/primussafe"
	}

	timeoutStr := os.Getenv("HELM_TIMEOUT")
	timeout := 10 * time.Minute
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	// Check if local charts should be used (for offline installation)
	useLocalCharts := os.Getenv("HELM_USE_LOCAL_CHARTS") == "true"
	localChartsPath := os.Getenv("HELM_LOCAL_CHARTS_PATH")
	if localChartsPath == "" {
		localChartsPath = "/app/charts"
	}

	return &HelmClient{
		chartRepo:       chartRepo,
		timeout:         timeout,
		debug:           os.Getenv("HELM_DEBUG") == "true",
		useLocalCharts:  useLocalCharts,
		localChartsPath: localChartsPath,
	}
}

// Install installs a Helm chart
func (h *HelmClient) Install(ctx context.Context, kubeconfig []byte, namespace, releaseName, chartName string, values map[string]interface{}) error {
	return h.installOrUpgrade(ctx, kubeconfig, namespace, releaseName, chartName, values, false)
}

// Upgrade upgrades a Helm release
func (h *HelmClient) Upgrade(ctx context.Context, kubeconfig []byte, namespace, releaseName, chartName string, values map[string]interface{}) error {
	return h.installOrUpgrade(ctx, kubeconfig, namespace, releaseName, chartName, values, true)
}

// resolveChartPath resolves the chart path based on local or remote mode
func (h *HelmClient) resolveChartPath(chartName string) (string, error) {
	if !h.useLocalCharts {
		// Use remote OCI registry
		return fmt.Sprintf("%s/%s", h.chartRepo, chartName), nil
	}

	// Use local charts - find the .tgz file
	// Chart files are named like: primus-lens-operators-1.0.0.tgz or primus-lens-operators-latest.tgz
	entries, err := os.ReadDir(h.localChartsPath)
	if err != nil {
		return "", fmt.Errorf("failed to read local charts directory %s: %w", h.localChartsPath, err)
	}

	// Look for matching chart file
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Match chart name prefix (e.g., "primus-lens-operators-" for chart "primus-lens-operators")
		if strings.HasPrefix(name, chartName+"-") && strings.HasSuffix(name, ".tgz") {
			chartPath := fmt.Sprintf("%s/%s", h.localChartsPath, name)
			log.Infof("Using local chart: %s", chartPath)
			return chartPath, nil
		}
	}

	return "", fmt.Errorf("local chart not found for %s in %s", chartName, h.localChartsPath)
}

// installOrUpgrade performs helm install or upgrade
func (h *HelmClient) installOrUpgrade(ctx context.Context, kubeconfig []byte, namespace, releaseName, chartName string, values map[string]interface{}, upgrade bool) error {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig temp file: %w", err)
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	kubeconfigFile.Close()

	// Write values to temp file
	valuesFile, err := os.CreateTemp("", "values-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create values temp file: %w", err)
	}
	defer os.Remove(valuesFile.Name())

	valuesYAML, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("failed to marshal values: %w", err)
	}
	if _, err := valuesFile.Write(valuesYAML); err != nil {
		return fmt.Errorf("failed to write values: %w", err)
	}
	valuesFile.Close()

	// Build helm command - resolve chart path based on local or remote mode
	chartPath, err := h.resolveChartPath(chartName)
	if err != nil {
		return fmt.Errorf("failed to resolve chart path: %w", err)
	}

	var args []string
	if upgrade {
		args = []string{"upgrade", releaseName, chartPath}
	} else {
		args = []string{"install", releaseName, chartPath}
	}

	args = append(args,
		"--kubeconfig", kubeconfigFile.Name(),
		"--namespace", namespace,
		"--create-namespace",
		"--values", valuesFile.Name(),
		"--timeout", h.timeout.String(),
		"--wait",
	)

	if h.debug {
		args = append(args, "--debug")
	}

	log.Infof("Executing: helm %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Errorf("Helm command failed: %s", stderr.String())
		return fmt.Errorf("helm %s failed: %s", args[0], stderr.String())
	}

	log.Infof("Helm %s completed: %s", args[0], stdout.String())
	return nil
}

// Uninstall uninstalls a Helm release
func (h *HelmClient) Uninstall(ctx context.Context, kubeconfig []byte, namespace, releaseName string) error {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig temp file: %w", err)
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	kubeconfigFile.Close()

	args := []string{
		"uninstall", releaseName,
		"--kubeconfig", kubeconfigFile.Name(),
		"--namespace", namespace,
	}

	log.Infof("Executing: helm %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Errorf("Helm uninstall failed: %s", stderr.String())
		return fmt.Errorf("helm uninstall failed: %s", stderr.String())
	}

	log.Infof("Helm uninstall completed: %s", stdout.String())
	return nil
}

// ReleaseStatus checks if a release exists and its status
func (h *HelmClient) ReleaseStatus(ctx context.Context, kubeconfig []byte, namespace, releaseName string) (exists bool, healthy bool, err error) {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return false, false, fmt.Errorf("failed to create kubeconfig temp file: %w", err)
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return false, false, fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	kubeconfigFile.Close()

	args := []string{
		"status", releaseName,
		"--kubeconfig", kubeconfigFile.Name(),
		"--namespace", namespace,
		"-o", "json",
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Release not found
		if strings.Contains(stderr.String(), "not found") {
			return false, false, nil
		}
		return false, false, fmt.Errorf("helm status failed: %s", stderr.String())
	}

	// Check if status is "deployed"
	output := stdout.String()
	healthy = strings.Contains(output, `"status":"deployed"`)

	return true, healthy, nil
}

// WaitForPods waits for pods with given label selector to be ready
func (h *HelmClient) WaitForPods(ctx context.Context, kubeconfig []byte, namespace, labelSelector string, timeout time.Duration) error {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig temp file: %w", err)
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	kubeconfigFile.Close()

	args := []string{
		"wait", "pods",
		"--kubeconfig", kubeconfigFile.Name(),
		"--namespace", namespace,
		"-l", labelSelector,
		"--for=condition=Ready",
		"--timeout", timeout.String(),
	}

	log.Infof("Waiting for pods: kubectl %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Errorf("Wait for pods failed: %s", stderr.String())
		return fmt.Errorf("wait for pods failed: %s", stderr.String())
	}

	log.Infof("Pods ready: %s", stdout.String())
	return nil
}

// ApplyYAML applies a YAML manifest
func (h *HelmClient) ApplyYAML(ctx context.Context, kubeconfig []byte, namespace string, yamlContent []byte) error {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create kubeconfig temp file: %w", err)
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}
	kubeconfigFile.Close()

	// Write YAML to temp file
	yamlFile, err := os.CreateTemp("", "manifest-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create yaml temp file: %w", err)
	}
	defer os.Remove(yamlFile.Name())

	if _, err := yamlFile.Write(yamlContent); err != nil {
		return fmt.Errorf("failed to write yaml: %w", err)
	}
	yamlFile.Close()

	args := []string{
		"apply",
		"--kubeconfig", kubeconfigFile.Name(),
		"--namespace", namespace,
		"-f", yamlFile.Name(),
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		log.Errorf("kubectl apply failed: %s", stderr.String())
		return fmt.Errorf("kubectl apply failed: %s", stderr.String())
	}

	log.Infof("kubectl apply completed: %s", stdout.String())
	return nil
}

// GetSecretValue retrieves a value from a Kubernetes secret
func (h *HelmClient) GetSecretValue(ctx context.Context, kubeconfig []byte, namespace, secretName, key string) (string, error) {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return "", err
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return "", err
	}
	kubeconfigFile.Close()

	args := []string{
		"get", "secret", secretName,
		"--kubeconfig", kubeconfigFile.Name(),
		"--namespace", namespace,
		"-o", fmt.Sprintf("jsonpath={.data.%s}", key),
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("kubectl get secret failed: %s", stderr.String())
	}

	// Decode base64
	decoded := make([]byte, len(stdout.Bytes()))
	n, err := decodeBase64(stdout.Bytes(), decoded)
	if err != nil {
		return "", err
	}

	return string(decoded[:n]), nil
}

// decodeBase64 decodes base64 encoded bytes
func decodeBase64(src, dst []byte) (int, error) {
	cmd := exec.Command("base64", "-d")
	cmd.Stdin = bytes.NewReader(src)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return 0, err
	}

	copy(dst, stdout.Bytes())
	return stdout.Len(), nil
}
