package stage

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/types"
)

// WaitCondition defines a condition to wait for
type WaitCondition struct {
	// Kind is the resource kind (e.g., "Pod", "Deployment", "PostgresCluster")
	Kind string

	// Name is the resource name
	Name string

	// Namespace is the resource namespace (optional, uses default if empty)
	Namespace string

	// LabelSelector is used instead of name for selecting resources
	LabelSelector string

	// Condition is the condition to wait for (e.g., "Ready", "Available")
	Condition string

	// JSONPath is used for custom condition checks
	JSONPath string

	// ExpectedValue is the expected value for JSONPath checks
	ExpectedValue string

	// Timeout is the timeout for this specific condition
	Timeout time.Duration
}

// WaitStage represents a stage that waits for resources to be ready
type WaitStage struct {
	name       string
	conditions []WaitCondition
	timeout    time.Duration
	interval   time.Duration
}

// WaitStageOption is a functional option for WaitStage
type WaitStageOption func(*WaitStage)

// WithWaitTimeout sets the overall timeout
func WithWaitTimeout(d time.Duration) WaitStageOption {
	return func(s *WaitStage) {
		s.timeout = d
	}
}

// WithPollInterval sets the polling interval
func WithPollInterval(d time.Duration) WaitStageOption {
	return func(s *WaitStage) {
		s.interval = d
	}
}

// NewWaitStage creates a new wait stage
func NewWaitStage(name string, conditions []WaitCondition, opts ...WaitStageOption) *WaitStage {
	s := &WaitStage{
		name:       name,
		conditions: conditions,
		timeout:    10 * time.Minute,
		interval:   5 * time.Second,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// Name returns the stage name
func (s *WaitStage) Name() string {
	return s.name
}

// Run waits for all conditions to be met
func (s *WaitStage) Run(ctx context.Context, opts types.RunOptions) error {
	if opts.DryRun {
		fmt.Printf("  [DRY-RUN] Would wait for %d conditions\n", len(s.conditions))
		for _, cond := range s.conditions {
			fmt.Printf("    - %s/%s: %s\n", cond.Kind, cond.Name, cond.Condition)
		}
		return nil
	}

	for _, cond := range s.conditions {
		if err := s.waitForCondition(ctx, opts, cond); err != nil {
			return fmt.Errorf("condition %s/%s failed: %w", cond.Kind, cond.Name, err)
		}
	}

	return nil
}

func (s *WaitStage) waitForCondition(ctx context.Context, opts types.RunOptions, cond WaitCondition) error {
	timeout := cond.Timeout
	if timeout == 0 {
		timeout = s.timeout
	}

	namespace := cond.Namespace
	if namespace == "" {
		namespace = opts.Namespace
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	condName := cond.Name
	if cond.LabelSelector != "" {
		condName = cond.LabelSelector
	}
	fmt.Printf("  Waiting for %s/%s to be %s...\n", cond.Kind, condName, cond.Condition)

	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %s/%s", cond.Kind, condName)
		case <-ticker.C:
			attempt++
			ready, err := s.checkCondition(ctx, opts, cond, namespace)
			if err != nil {
				if opts.Verbose {
					fmt.Printf("    [%d] Check failed: %v\n", attempt, err)
				}
				continue
			}
			if ready {
				fmt.Printf("  ✓ %s/%s is %s\n", cond.Kind, condName, cond.Condition)
				return nil
			}
			if opts.Verbose {
				fmt.Printf("    [%d] Not ready yet, retrying...\n", attempt)
			}
		}
	}
}

func (s *WaitStage) checkCondition(ctx context.Context, opts types.RunOptions, cond WaitCondition, namespace string) (bool, error) {
	// Handle CRD resources with JSONPath
	if cond.JSONPath != "" {
		return s.checkJSONPathCondition(ctx, opts, cond, namespace)
	}

	// Handle standard Kubernetes resources with kubectl wait
	return s.checkKubectlWait(ctx, opts, cond, namespace)
}

func (s *WaitStage) checkKubectlWait(ctx context.Context, opts types.RunOptions, cond WaitCondition, namespace string) (bool, error) {
	// First check for failed pods (CrashLoopBackOff, Error, ImagePullBackOff, etc.)
	if cond.LabelSelector != "" {
		if err := s.checkForFailedPods(ctx, opts, cond, namespace); err != nil {
			return false, err
		}
	}

	args := []string{
		"wait",
		"--for=condition=" + cond.Condition,
		"--timeout=5s",
		"-n", namespace,
	}

	if cond.LabelSelector != "" {
		args = append(args, cond.Kind, "-l", cond.LabelSelector)
	} else {
		args = append(args, fmt.Sprintf("%s/%s", strings.ToLower(cond.Kind), cond.Name))
	}

	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return false, nil // Not ready yet
	}

	return true, nil
}

// checkForFailedPods checks if any pods matching the selector are in a failed state
func (s *WaitStage) checkForFailedPods(ctx context.Context, opts types.RunOptions, cond WaitCondition, namespace string) error {
	// Get pods with the label selector
	args := []string{
		"get", "pods",
		"-l", cond.LabelSelector,
		"-n", namespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name}:{.status.phase}:{range .status.containerStatuses[*]}{.state.waiting.reason},{end}{\"\\n\"}{end}",
	}

	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil // Ignore errors, let the main wait handle it
	}

	// Check for failure states
	failedStates := []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError", "InvalidImageName"}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		for _, state := range failedStates {
			if strings.Contains(line, state) {
				// Extract pod name for better error message
				parts := strings.SplitN(line, ":", 2)
				podName := "unknown"
				if len(parts) > 0 {
					podName = parts[0]
				}
				return fmt.Errorf("pod %s is in failed state: %s", podName, state)
			}
		}
	}

	return nil
}

func (s *WaitStage) checkJSONPathCondition(ctx context.Context, opts types.RunOptions, cond WaitCondition, namespace string) (bool, error) {
	args := []string{
		"get",
		strings.ToLower(cond.Kind),
		cond.Name,
		"-n", namespace,
		"-o", fmt.Sprintf("jsonpath=%s", cond.JSONPath),
	}

	if opts.Kubeconfig != "" {
		args = append(args, "--kubeconfig", opts.Kubeconfig)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("kubectl get failed: %w", err)
	}

	value := strings.TrimSpace(string(output))
	return value == cond.ExpectedValue, nil
}

// Verify checks if all conditions are met
func (s *WaitStage) Verify(ctx context.Context, opts types.RunOptions) (*types.StageStatus, error) {
	status := &types.StageStatus{
		Name:      s.name,
		State:     types.StateReady,
		Resources: make([]types.ResourceStatus, 0, len(s.conditions)),
	}

	for _, cond := range s.conditions {
		namespace := cond.Namespace
		if namespace == "" {
			namespace = opts.Namespace
		}

		ready, err := s.checkCondition(ctx, opts, cond, namespace)

		resStatus := types.ResourceStatus{
			Kind: cond.Kind,
			Name: cond.Name,
		}

		if err != nil {
			resStatus.State = types.StateUnknown
			resStatus.Message = err.Error()
			status.State = types.StateFailed
		} else if ready {
			resStatus.State = types.StateReady
		} else {
			resStatus.State = types.StatePending
			if status.State != types.StateFailed {
				status.State = types.StatePending
			}
		}

		status.Resources = append(status.Resources, resStatus)
	}

	return status, nil
}

// Rollback is a no-op for wait stages
func (s *WaitStage) Rollback(ctx context.Context, opts types.RunOptions) error {
	// Nothing to rollback for a wait stage
	return nil
}
