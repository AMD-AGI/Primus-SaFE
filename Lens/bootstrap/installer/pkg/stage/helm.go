// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package stage

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/types"
)

// HelmStage represents a stage that installs a Helm chart
type HelmStage struct {
	name        string
	releaseName string
	chartPath   string
	valuesFile  string
	timeout     time.Duration
	wait        bool
	setValues   map[string]string
	namespace   string
	updateDeps  bool
}

// HelmStageOption is a functional option for HelmStage
type HelmStageOption func(*HelmStage)

// WithValuesFile sets the values file
func WithValuesFile(path string) HelmStageOption {
	return func(s *HelmStage) {
		s.valuesFile = path
	}
}

// WithTimeout sets the timeout
func WithTimeout(d time.Duration) HelmStageOption {
	return func(s *HelmStage) {
		s.timeout = d
	}
}

// WithWait enables waiting for resources to be ready
func WithWait(wait bool) HelmStageOption {
	return func(s *HelmStage) {
		s.wait = wait
	}
}

// WithSetValue sets a value override
func WithSetValue(key, value string) HelmStageOption {
	return func(s *HelmStage) {
		if s.setValues == nil {
			s.setValues = make(map[string]string)
		}
		s.setValues[key] = value
	}
}

// WithNamespace sets the namespace
func WithNamespace(ns string) HelmStageOption {
	return func(s *HelmStage) {
		s.namespace = ns
	}
}

// WithUpdateDeps enables dependency update before installation
func WithUpdateDeps(update bool) HelmStageOption {
	return func(s *HelmStage) {
		s.updateDeps = update
	}
}

// NewHelmStage creates a new Helm stage
func NewHelmStage(name, releaseName, chartPath string, opts ...HelmStageOption) *HelmStage {
	s := &HelmStage{
		name:        name,
		releaseName: releaseName,
		chartPath:   chartPath,
		timeout:     10 * time.Minute,
		wait:        false,
		setValues:   make(map[string]string),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Name returns the stage name
func (s *HelmStage) Name() string {
	return s.name
}

// updateDependencies runs helm dependency update for the chart
func (s *HelmStage) updateDependencies(ctx context.Context, opts types.RunOptions) error {
	args := []string{
		"dependency", "update", s.chartPath,
	}

	if opts.DryRun {
		fmt.Printf("  [DRY-RUN] helm %s\n", strings.Join(args, " "))
		return nil
	}

	if opts.Verbose {
		fmt.Printf("  Updating dependencies: helm %s\n", strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm dependency update failed: %w\nOutput: %s", err, string(output))
	}

	if opts.Verbose {
		fmt.Printf("  Dependencies updated: %s\n", string(output))
	}

	return nil
}

// ensureNamespaceWithHelmLabels creates the namespace with proper Helm labels if it doesn't exist
func (s *HelmStage) ensureNamespaceWithHelmLabels(ctx context.Context, namespace string, opts types.RunOptions) error {
	// Check if namespace exists
	kubectlArgs := []string{"get", "namespace", namespace}
	if opts.Kubeconfig != "" {
		kubectlArgs = append(kubectlArgs, "--kubeconfig", opts.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, "kubectl", kubectlArgs...)
	if err := cmd.Run(); err != nil {
		// Namespace doesn't exist, create it with Helm labels
		manifest := fmt.Sprintf(`apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    app.kubernetes.io/managed-by: Helm
  annotations:
    meta.helm.sh/release-name: %s
    meta.helm.sh/release-namespace: %s
`, namespace, s.releaseName, namespace)

		if opts.DryRun {
			if opts.Verbose {
				fmt.Printf("  [DRY-RUN] Would create namespace %s with Helm labels\n", namespace)
			}
			return nil
		}

		if opts.Verbose {
			fmt.Printf("  Creating namespace %s with Helm labels...\n", namespace)
		}

		applyArgs := []string{"apply", "-f", "-"}
		if opts.Kubeconfig != "" {
			applyArgs = append(applyArgs, "--kubeconfig", opts.Kubeconfig)
		}

		applyCmd := exec.CommandContext(ctx, "kubectl", applyArgs...)
		applyCmd.Stdin = strings.NewReader(manifest)
		output, err := applyCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to create namespace: %w\nOutput: %s", err, string(output))
		}
	}
	return nil
}

// Run executes the Helm installation
func (s *HelmStage) Run(ctx context.Context, opts types.RunOptions) error {
	// Update dependencies if enabled
	if s.updateDeps {
		if err := s.updateDependencies(ctx, opts); err != nil {
			return fmt.Errorf("failed to update dependencies: %w", err)
		}
	}

	namespace := s.namespace
	if namespace == "" {
		namespace = opts.Namespace
	}

	// Ensure namespace exists with proper Helm labels (workaround for helm --create-namespace bug)
	if err := s.ensureNamespaceWithHelmLabels(ctx, namespace, opts); err != nil {
		return err
	}

	args := []string{
		"upgrade", "--install", s.releaseName, s.chartPath,
		"--namespace", namespace,
		"--create-namespace",
		"--timeout", s.timeout.String(),
	}

	if s.valuesFile != "" {
		args = append(args, "-f", s.valuesFile)
	}

	for key, value := range s.setValues {
		args = append(args, "--set", fmt.Sprintf("%s=%s", key, value))
	}

	if s.wait {
		args = append(args, "--wait")
	}

	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}

	if opts.DryRun {
		args = append(args, "--dry-run")
		fmt.Printf("  [DRY-RUN] helm %s\n", strings.Join(args, " "))
		return nil
	}

	if opts.Verbose {
		fmt.Printf("  Executing: helm %s\n", strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("helm install failed: %w\nOutput: %s", err, string(output))
	}

	if opts.Verbose {
		fmt.Printf("  Output: %s\n", string(output))
	}

	return nil
}

// Verify verifies that the Helm release is deployed
func (s *HelmStage) Verify(ctx context.Context, opts types.RunOptions) (*types.StageStatus, error) {
	namespace := s.namespace
	if namespace == "" {
		namespace = opts.Namespace
	}

	args := []string{
		"status", s.releaseName,
		"--namespace", namespace,
		"-o", "json",
	}

	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()

	status := &types.StageStatus{
		Name: s.name,
	}

	if err != nil {
		// Release not found
		if strings.Contains(string(output), "not found") {
			status.State = types.StatePending
			status.Message = "Release not installed"
			return status, nil
		}
		status.State = types.StateUnknown
		status.Message = fmt.Sprintf("Failed to get status: %v", err)
		return status, nil
	}

	// Check if deployed
	if strings.Contains(string(output), `"status":"deployed"`) {
		status.State = types.StateReady
		status.Message = "Release deployed"
	} else if strings.Contains(string(output), `"status":"pending"`) {
		status.State = types.StateInProgress
		status.Message = "Release pending"
	} else {
		status.State = types.StateFailed
		status.Message = "Release in failed state"
	}

	return status, nil
}

// Rollback uninstalls the Helm release
func (s *HelmStage) Rollback(ctx context.Context, opts types.RunOptions) error {
	namespace := s.namespace
	if namespace == "" {
		namespace = opts.Namespace
	}

	args := []string{
		"uninstall", s.releaseName,
		"--namespace", namespace,
	}

	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}

	if opts.DryRun {
		fmt.Printf("  [DRY-RUN] helm %s\n", strings.Join(args, " "))
		return nil
	}

	if opts.Verbose {
		fmt.Printf("  Executing: helm %s\n", strings.Join(args, " "))
	}

	cmd := exec.CommandContext(ctx, "helm", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore "not found" errors
		if strings.Contains(string(output), "not found") {
			return nil
		}
		return fmt.Errorf("helm uninstall failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}
