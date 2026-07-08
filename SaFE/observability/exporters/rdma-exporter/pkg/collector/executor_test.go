// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package collector

import (
	"os"
	"testing"
)

func TestNewCommandExecutor(t *testing.T) {
	os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")
	os.Unsetenv("RDMA_EXPORTER_NSENTER_TARGET")

	e := NewCommandExecutor()
	if e == nil {
		t.Fatal("NewCommandExecutor returned nil")
	}
	if e.useNsenter {
		t.Error("expected useNsenter to be false by default")
	}
	if e.nsenterTarget != 1 {
		t.Errorf("expected nsenterTarget 1, got %d", e.nsenterTarget)
	}
}

func TestNewCommandExecutorWithNsenterTrue(t *testing.T) {
	os.Setenv("RDMA_EXPORTER_USE_NSENTER", "true")
	defer os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if !e.useNsenter {
		t.Error("expected useNsenter to be true")
	}
}

func TestNewCommandExecutorWithNsenterYes(t *testing.T) {
	os.Setenv("RDMA_EXPORTER_USE_NSENTER", "yes")
	defer os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if !e.useNsenter {
		t.Error("expected useNsenter to be true for 'yes'")
	}
}

func TestNewCommandExecutorWithNsenterOne(t *testing.T) {
	os.Setenv("RDMA_EXPORTER_USE_NSENTER", "1")
	defer os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if !e.useNsenter {
		t.Error("expected useNsenter to be true for '1'")
	}
}

func TestNewCommandExecutorWithNsenterFalse(t *testing.T) {
	os.Setenv("RDMA_EXPORTER_USE_NSENTER", "false")
	defer os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if e.useNsenter {
		t.Error("expected useNsenter to be false")
	}
}

func TestNewCommandExecutorWithNsenterNo(t *testing.T) {
	os.Setenv("RDMA_EXPORTER_USE_NSENTER", "no")
	defer os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if e.useNsenter {
		t.Error("expected useNsenter to be false for 'no'")
	}
}

func TestNewCommandExecutorWithCustomTarget(t *testing.T) {
	os.Setenv("RDMA_EXPORTER_NSENTER_TARGET", "99")
	defer os.Unsetenv("RDMA_EXPORTER_NSENTER_TARGET")

	e := NewCommandExecutor()
	if e.nsenterTarget != 99 {
		t.Errorf("expected nsenterTarget 99, got %d", e.nsenterTarget)
	}
}

func TestNewCommandExecutorWithInvalidTarget(t *testing.T) {
	os.Setenv("RDMA_EXPORTER_NSENTER_TARGET", "not_a_number")
	defer os.Unsetenv("RDMA_EXPORTER_NSENTER_TARGET")

	e := NewCommandExecutor()
	// Should fall back to default
	if e.nsenterTarget != 1 {
		t.Errorf("expected nsenterTarget 1 for invalid input, got %d", e.nsenterTarget)
	}
}

func TestIsNsenterEnabled(t *testing.T) {
	os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if e.IsNsenterEnabled() {
		t.Error("expected IsNsenterEnabled to be false")
	}
}

func TestExecuteDirectCommand(t *testing.T) {
	os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()

	output, err := e.Execute("go", "version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output) == 0 {
		t.Error("expected non-empty output")
	}
}

func TestExecuteFailingCommand(t *testing.T) {
	os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()

	_, err := e.Execute("nonexistent_command_12345")
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestGetNodeName(t *testing.T) {
	os.Setenv("NODE_NAME", "rdma-test-node")
	defer os.Unsetenv("NODE_NAME")

	name := getNodeName()
	if name != "rdma-test-node" {
		t.Errorf("expected rdma-test-node, got %s", name)
	}
}

func TestGetNodeNameFallback(t *testing.T) {
	os.Unsetenv("NODE_NAME")

	name := getNodeName()
	if name == "" {
		t.Error("expected non-empty node name")
	}
}

func TestRunPreflightChecks(t *testing.T) {
	os.Unsetenv("RDMA_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()

	// Should not return error even if rdma tool is not available
	err := RunPreflightChecks(e)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

