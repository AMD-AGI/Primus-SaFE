// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/AMD-AGI/Primus-SaFE/Lens/bootstrap/installer/pkg/config"
)

// mockStage is a mock implementation of Stage for testing
type mockStage struct {
	name         string
	runErr       error
	verifyState  State
	verifyErr    error
	rollbackErr  error
	runCalled    bool
	verifyCalled bool
	rollbackCalled bool
}

func (m *mockStage) Name() string {
	return m.name
}

func (m *mockStage) Run(ctx context.Context, opts RunOptions) error {
	m.runCalled = true
	return m.runErr
}

func (m *mockStage) Verify(ctx context.Context, opts RunOptions) (*StageStatus, error) {
	m.verifyCalled = true
	if m.verifyErr != nil {
		return nil, m.verifyErr
	}
	return &StageStatus{
		Name:  m.name,
		State: m.verifyState,
	}, nil
}

func (m *mockStage) Rollback(ctx context.Context, opts RunOptions) error {
	m.rollbackCalled = true
	return m.rollbackErr
}

func TestBaseWorkflowName(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	assert.Equal(t, "test-workflow", wf.Name())
}

func TestBaseWorkflowConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	assert.Equal(t, cfg, wf.Config())
}

func TestBaseWorkflowAddStage(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", verifyState: StateReady}
	stage2 := &mockStage{name: "stage2", verifyState: StateReady}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	stages := wf.Stages()
	assert.Len(t, stages, 2)
	assert.Equal(t, "stage1", stages[0].Name())
	assert.Equal(t, "stage2", stages[1].Name())
}

func TestBaseWorkflowInstallSuccess(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", verifyState: StateReady}
	stage2 := &mockStage{name: "stage2", verifyState: StateReady}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}

	err := wf.Install(ctx, opts)
	assert.NoError(t, err)
	assert.True(t, stage1.runCalled)
	assert.True(t, stage1.verifyCalled)
	assert.True(t, stage2.runCalled)
	assert.True(t, stage2.verifyCalled)
}

func TestBaseWorkflowInstallStageRunError(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", runErr: errors.New("run failed")}
	stage2 := &mockStage{name: "stage2", verifyState: StateReady}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}

	err := wf.Install(ctx, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage1 failed")
	assert.True(t, stage1.runCalled)
	assert.False(t, stage2.runCalled) // Should not be called
}

func TestBaseWorkflowInstallVerifyFailed(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", verifyState: StateFailed}

	wf.AddStage(stage1)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}

	err := wf.Install(ctx, opts)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stage1 failed")
}

func TestBaseWorkflowInstallContextCancelled(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", verifyState: StateReady}
	wf.AddStage(stage1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	opts := RunOptions{Namespace: "test"}

	err := wf.Install(ctx, opts)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)
}

func TestBaseWorkflowUninstallSuccess(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1"}
	stage2 := &mockStage{name: "stage2"}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}
	uninstallOpts := UninstallOptions{}

	err := wf.Uninstall(ctx, opts, uninstallOpts)
	assert.NoError(t, err)

	// Should be called in reverse order
	assert.True(t, stage2.rollbackCalled)
	assert.True(t, stage1.rollbackCalled)
}

func TestBaseWorkflowUninstallForce(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1"}
	stage2 := &mockStage{name: "stage2", rollbackErr: errors.New("rollback failed")}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}
	uninstallOpts := UninstallOptions{Force: true}

	err := wf.Uninstall(ctx, opts, uninstallOpts)
	assert.NoError(t, err) // Should succeed with force

	assert.True(t, stage2.rollbackCalled)
	assert.True(t, stage1.rollbackCalled)
}

func TestBaseWorkflowStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", verifyState: StateReady}
	stage2 := &mockStage{name: "stage2", verifyState: StateReady}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}

	status, err := wf.Status(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, "test-workflow", status.WorkflowName)
	assert.Equal(t, StateReady, status.OverallState)
	assert.Len(t, status.Stages, 2)
}

func TestBaseWorkflowStatusWithFailure(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", verifyState: StateReady}
	stage2 := &mockStage{name: "stage2", verifyState: StateFailed}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}

	status, err := wf.Status(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, StateFailed, status.OverallState)
}

func TestBaseWorkflowStatusWithPending(t *testing.T) {
	cfg := config.DefaultConfig()
	wf := NewBaseWorkflow("test-workflow", cfg)

	stage1 := &mockStage{name: "stage1", verifyState: StateReady}
	stage2 := &mockStage{name: "stage2", verifyState: StatePending}

	wf.AddStage(stage1)
	wf.AddStage(stage2)

	ctx := context.Background()
	opts := RunOptions{Namespace: "test"}

	status, err := wf.Status(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, StatePending, status.OverallState)
}

