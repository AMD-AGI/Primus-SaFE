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
	// Chart names can be:
	// - Simple: "primus-lens-operators" -> /app/charts/primus-lens-operators-1.0.0.tgz
	// - Nested: "operators/pgo" -> /app/charts/operators/primus-lens-pgo-1.0.0.tgz
	// - Nested: "infrastructure/postgres" -> /app/charts/infrastructure/primus-lens-postgres-1.0.0.tgz

	// Determine the search directory and base chart name
	searchDir := h.localChartsPath
	baseChartName := chartName

	// Handle nested paths (e.g., "operators/pgo", "infrastructure/postgres")
	if strings.Contains(chartName, "/") {
		parts := strings.SplitN(chartName, "/", 2)
		searchDir = fmt.Sprintf("%s/%s", h.localChartsPath, parts[0])
		baseChartName = parts[1]
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return "", fmt.Errorf("failed to read local charts directory %s: %w", searchDir, err)
	}

	// Look for matching chart file
	// Try multiple naming patterns:
	// 1. Exact match: pgo-1.0.0.tgz
	// 2. Prefixed: primus-lens-pgo-1.0.0.tgz
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Match patterns:
		// - baseChartName-version.tgz (e.g., pgo-1.0.0.tgz)
		// - primus-lens-baseChartName-version.tgz (e.g., primus-lens-pgo-1.0.0.tgz)
		if (strings.HasPrefix(name, baseChartName+"-") || strings.HasPrefix(name, "primus-lens-"+baseChartName+"-")) &&
			strings.HasSuffix(name, ".tgz") {
			chartPath := fmt.Sprintf("%s/%s", searchDir, name)
			log.Infof("Using local chart: %s", chartPath)
			return chartPath, nil
		}
	}

	return "", fmt.Errorf("local chart not found for %s in %s", chartName, searchDir)
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

// ClusterRoleExists checks if a ClusterRole exists in the cluster
func (h *HelmClient) ClusterRoleExists(ctx context.Context, kubeconfig []byte, name string) (bool, error) {
	// Write kubeconfig to temp file
	kubeconfigFile, err := os.CreateTemp("", "kubeconfig-*.yaml")
	if err != nil {
		return false, err
	}
	defer os.Remove(kubeconfigFile.Name())

	if _, err := kubeconfigFile.Write(kubeconfig); err != nil {
		return false, err
	}
	kubeconfigFile.Close()

	args := []string{
		"get", "clusterrole", name,
		"--kubeconfig", kubeconfigFile.Name(),
		"-o", "name",
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// If ClusterRole not found, return false without error
		if strings.Contains(stderr.String(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check clusterrole: %s", stderr.String())
	}

	return true, nil
}

// OperatorStatus tracks which operators exist and which need to be installed
type OperatorStatus struct {
	PGO              bool // PostgreSQL Operator
	OpenSearch       bool // OpenSearch Operator
	Grafana          bool // Grafana Operator
	VictoriaMetrics  bool // VictoriaMetrics Operator
	Fluent           bool // Fluent Operator
	KubeStateMetrics bool // Kube State Metrics
}

// AllExist returns true if all operators exist
func (s *OperatorStatus) AllExist() bool {
	return s.PGO && s.OpenSearch && s.Grafana && s.VictoriaMetrics && s.Fluent && s.KubeStateMetrics
}

// NoneExist returns true if no operators exist
func (s *OperatorStatus) NoneExist() bool {
	return !s.PGO && !s.OpenSearch && !s.Grafana && !s.VictoriaMetrics && !s.Fluent && !s.KubeStateMetrics
}

// DetectOperators checks which operators already exist in the cluster
func (h *HelmClient) DetectOperators(ctx context.Context, kubeconfig []byte) (*OperatorStatus, error) {
	status := &OperatorStatus{}

	// Check each operator by its ClusterRole or other unique resource
	checks := []struct {
		name     string
		resource string
		target   *bool
	}{
		{"PGO", "pgo", &status.PGO},
		{"OpenSearch", "opensearch-operator-manager-role", &status.OpenSearch},
		{"Grafana", "grafana-operator-manager-role", &status.Grafana},
		{"VictoriaMetrics", "vm-operator-victoria-metrics-operator", &status.VictoriaMetrics},
		{"Fluent", "fluent-operator", &status.Fluent},
		{"KubeStateMetrics", "kube-state-metrics", &status.KubeStateMetrics},
	}

	for _, check := range checks {
		exists, err := h.ClusterRoleExists(ctx, kubeconfig, check.resource)
		if err != nil {
			log.Warnf("Error checking %s ClusterRole %s: %v", check.name, check.resource, err)
			// Assume not exists on error
			*check.target = false
		} else {
			*check.target = exists
			if exists {
				log.Debugf("Operator %s already exists (found ClusterRole %s)", check.name, check.resource)
			}
		}
	}

	return status, nil
}

// OperatorsExist checks if key operator components already exist in the cluster
// This is used to detect if operators were installed by another release (e.g., control plane)
// Deprecated: Use DetectOperators for more granular control
func (h *HelmClient) OperatorsExist(ctx context.Context, kubeconfig []byte) (bool, string, error) {
	status, err := h.DetectOperators(ctx, kubeconfig)
	if err != nil {
		return false, "", err
	}

	if status.AllExist() {
		return true, "all", nil
	}

	return false, "", nil
}
