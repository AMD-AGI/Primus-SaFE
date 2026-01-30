// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package installer

import (
	"context"
	"time"
)

// StageV2 is the new stage interface with improved lifecycle methods.
// It provides explicit prerequisite checking, idempotency via ShouldRun,
// and separate wait logic for better observability and reliability.
type StageV2 interface {
	// Name returns the unique identifier for this stage
	Name() string

	// CheckPrerequisites verifies all dependencies are met before execution.
	// Returns a slice of missing prerequisites (empty if all met).
	// This should check things like:
	// - Required operators are deployed and ready
	// - Required secrets exist
	// - StorageClass exists
	CheckPrerequisites(ctx context.Context, client *ClusterClient, config *InstallConfig) ([]string, error)

	// ShouldRun determines if this stage needs to execute.
	// Returns: (shouldRun, reason, error)
	// This enables idempotency by checking if the stage's work is already done.
	// Examples:
	// - Operator stage: return false if ClusterRole already exists
	// - Infrastructure stage: return false if CR exists and is healthy
	ShouldRun(ctx context.Context, client *ClusterClient, config *InstallConfig) (bool, string, error)

	// Execute performs the main installation action.
	// This should be idempotent - safe to call multiple times.
	// Should NOT include wait logic - that belongs in WaitForReady.
	Execute(ctx context.Context, client *ClusterClient, config *InstallConfig) error

	// WaitForReady waits until the stage's resources are fully ready.
	// This is separate from Execute to allow for:
	// - Clear timeout handling
	// - Progress logging during wait
	// - Retry logic for transient "not found" errors
	WaitForReady(ctx context.Context, client *ClusterClient, config *InstallConfig, timeout time.Duration) error

	// Rollback attempts to undo the stage's changes (best effort).
	// Used when installation fails and cleanup is needed.
	Rollback(ctx context.Context, client *ClusterClient, config *InstallConfig) error

	// IsRequired returns true if this stage must succeed for installation to continue.
	// Optional stages (like OpenSearch) can return false.
	IsRequired() bool
}

// StageResult captures the outcome of a stage execution
type StageResult struct {
	Stage    string        `json:"stage"`
	Status   string        `json:"status"` // "skipped", "completed", "failed"
	Reason   string        `json:"reason,omitempty"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"-"`
}

// StageStatus constants
const (
	StageStatusSkipped   = "skipped"
	StageStatusCompleted = "completed"
	StageStatusFailed    = "failed"
)

// DefaultStageTimeouts provides default timeouts for each stage type
var DefaultStageTimeouts = map[string]time.Duration{
	"operator-pgo":         5 * time.Minute,
	"operator-vm":          5 * time.Minute,
	"operator-opensearch":  5 * time.Minute,
	"operator-grafana":     5 * time.Minute,
	"operator-fluent":      5 * time.Minute,
	"operator-ksm":         5 * time.Minute,
	"infra-postgres":       10 * time.Minute,
	"infra-victoriametrics": 5 * time.Minute,
	"infra-opensearch":     10 * time.Minute,
	"database-init":        5 * time.Minute,
	"database-migration":   5 * time.Minute,
	"storage-secret":       1 * time.Minute,
	"applications":         10 * time.Minute,
}

// GetStageTimeout returns the timeout for a stage, or default if not found
func GetStageTimeout(stageName string) time.Duration {
	if timeout, ok := DefaultStageTimeouts[stageName]; ok {
		return timeout
	}
	return 5 * time.Minute
}

// BaseStage provides default implementations for optional StageV2 methods
type BaseStage struct{}

// Rollback is a no-op by default
func (b *BaseStage) Rollback(ctx context.Context, client *ClusterClient, config *InstallConfig) error {
	return nil
}

// IsRequired returns true by default
func (b *BaseStage) IsRequired() bool {
	return true
}

// StageAdapter wraps old Stage interface to work with new executor
type StageAdapter struct {
	oldStage   Stage
	helmClient *HelmClient
}

// NewStageAdapter creates an adapter from old Stage to StageV2
func NewStageAdapter(oldStage Stage, helmClient *HelmClient) *StageAdapter {
	return &StageAdapter{
		oldStage:   oldStage,
		helmClient: helmClient,
	}
}

func (a *StageAdapter) Name() string {
	return a.oldStage.Name()
}

func (a *StageAdapter) CheckPrerequisites(ctx context.Context, client *ClusterClient, config *InstallConfig) ([]string, error) {
	// Old stages don't have prerequisite checking
	return nil, nil
}

func (a *StageAdapter) ShouldRun(ctx context.Context, client *ClusterClient, config *InstallConfig) (bool, string, error) {
	// Old stages always run (they handle idempotency internally)
	return true, "", nil
}

func (a *StageAdapter) Execute(ctx context.Context, client *ClusterClient, config *InstallConfig) error {
	return a.oldStage.Execute(ctx, a.helmClient, config)
}

func (a *StageAdapter) WaitForReady(ctx context.Context, client *ClusterClient, config *InstallConfig, timeout time.Duration) error {
	// Old stages handle waiting in Execute
	return nil
}

func (a *StageAdapter) Rollback(ctx context.Context, client *ClusterClient, config *InstallConfig) error {
	return nil
}

func (a *StageAdapter) IsRequired() bool {
	return true
}
