// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// minimalKubeconfig is a valid kubeconfig that parses but does not connect to a real cluster.
const minimalKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://127.0.0.1:6443
  name: test
contexts:
- context:
    cluster: test
    user: test
  name: test
current-context: test
users:
- name: test
  user:
    token: fake-token-for-test
`

// mockStageV2 is a StageV2 that runs successfully or fails on demand.
type mockStageV2 struct {
	name          string
	shouldRun     bool
	executeErr    error
	waitForReadyErr error
	required      bool
}

func (m *mockStageV2) Name() string { return m.name }

func (m *mockStageV2) CheckPrerequisites(ctx context.Context, client *ClusterClient, config *InstallConfig) ([]string, error) {
	return nil, nil
}

func (m *mockStageV2) ShouldRun(ctx context.Context, client *ClusterClient, config *InstallConfig) (bool, string, error) {
	return m.shouldRun, "mock reason", nil
}

func (m *mockStageV2) Execute(ctx context.Context, client *ClusterClient, config *InstallConfig) error {
	return m.executeErr
}

func (m *mockStageV2) WaitForReady(ctx context.Context, client *ClusterClient, config *InstallConfig, timeout time.Duration) error {
	return m.waitForReadyErr
}

func (m *mockStageV2) Rollback(ctx context.Context, client *ClusterClient, config *InstallConfig) error {
	return nil
}

func (m *mockStageV2) IsRequired() bool {
	if m.required {
		return true
	}
	return false
}

// mockStageReporter records OnStageStart and OnStageEnd calls.
type mockStageReporter struct {
	started []string
	ended   []string
}

func (m *mockStageReporter) OnStageStart(ctx context.Context, stageName string) {
	m.started = append(m.started, stageName)
}

func (m *mockStageReporter) OnStageEnd(ctx context.Context, stageName string, result StageResult) {
	m.ended = append(m.ended, stageName)
}

func TestExecutor_ExecuteStages_CallsReporter(t *testing.T) {
	config := &InstallConfig{
		Namespace:    "test-ns",
		ClusterName:  "test-cluster",
		StorageMode:  StorageModeLensManaged,
	}
	exec, err := NewExecutor([]byte(minimalKubeconfig), config)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	reporter := &mockStageReporter{}
	exec.SetReporter(reporter)

	stages := []StageV2{
		&mockStageV2{name: "stage-a", shouldRun: true, required: true},
		&mockStageV2{name: "stage-b", shouldRun: true, required: true},
	}

	ctx := context.Background()
	results, err := exec.ExecuteStages(ctx, stages)
	if err != nil {
		t.Fatalf("ExecuteStages: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("len(results) = %d, want 2", len(results))
	}
	if len(reporter.started) != 2 {
		t.Errorf("OnStageStart called %d times, want 2: %v", len(reporter.started), reporter.started)
	}
	if reporter.started[0] != "stage-a" || reporter.started[1] != "stage-b" {
		t.Errorf("OnStageStart order = %v", reporter.started)
	}
	if len(reporter.ended) != 0 {
		t.Errorf("OnStageEnd should not be called on success, got %v", reporter.ended)
	}
	if results[0].Status != StageStatusCompleted || results[1].Status != StageStatusCompleted {
		t.Errorf("results status = %q, %q", results[0].Status, results[1].Status)
	}
}

func TestExecutor_ExecuteStages_SkipsWhenShouldRunFalse(t *testing.T) {
	config := &InstallConfig{Namespace: "test-ns", ClusterName: "test", StorageMode: StorageModeLensManaged}
	exec, err := NewExecutor([]byte(minimalKubeconfig), config)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	reporter := &mockStageReporter{}
	exec.SetReporter(reporter)

	stages := []StageV2{
		&mockStageV2{name: "skip-me", shouldRun: false},
	}

	ctx := context.Background()
	results, err := exec.ExecuteStages(ctx, stages)
	if err != nil {
		t.Fatalf("ExecuteStages: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Status != StageStatusSkipped {
		t.Errorf("result.Status = %q, want skipped", results[0].Status)
	}
	// OnStageStart is only called when stage will execute (after ShouldRun passes)
	if len(reporter.started) != 0 {
		t.Errorf("OnStageStart should not be called for skipped stage, got %v", reporter.started)
	}
}

func TestExecutor_ExecuteStages_CallsOnStageEndOnFailure(t *testing.T) {
	config := &InstallConfig{Namespace: "test-ns", ClusterName: "test", StorageMode: StorageModeLensManaged}
	exec, err := NewExecutor([]byte(minimalKubeconfig), config)
	if err != nil {
		t.Fatalf("NewExecutor: %v", err)
	}

	reporter := &mockStageReporter{}
	exec.SetReporter(reporter)

	failErr := fmt.Errorf("injected execute failure")
	stages := []StageV2{
		&mockStageV2{name: "will-fail", shouldRun: true, executeErr: failErr, required: true},
		&mockStageV2{name: "never-runs", shouldRun: true, required: true},
	}

	ctx := context.Background()
	results, err := exec.ExecuteStages(ctx, stages)
	if err == nil {
		t.Fatal("ExecuteStages expected error")
	}

	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1 (stop on first failure)", len(results))
	}
	if results[0].Status != StageStatusFailed {
		t.Errorf("result.Status = %q, want failed", results[0].Status)
	}
	if len(reporter.started) != 1 {
		t.Errorf("OnStageStart calls = %d, want 1", len(reporter.started))
	}
	if len(reporter.ended) != 1 || reporter.ended[0] != "will-fail" {
		t.Errorf("OnStageEnd should be called once for will-fail, got %v", reporter.ended)
	}
}

