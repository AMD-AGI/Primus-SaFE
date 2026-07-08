// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.

package collector

import (
	"os"
	"testing"
)

func TestNewCommandExecutor(t *testing.T) {
	// Clear any env vars that might affect the test
	os.Unsetenv("GPU_EXPORTER_USE_NSENTER")
	os.Unsetenv("GPU_EXPORTER_NSENTER_TARGET")

	e := NewCommandExecutor()
	if e == nil {
		t.Fatal("NewCommandExecutor returned nil")
	}
	// Default should not use nsenter
	if e.useNsenter {
		t.Error("expected useNsenter to be false by default")
	}
	if e.nsenterTarget != 1 {
		t.Errorf("expected nsenterTarget 1, got %d", e.nsenterTarget)
	}
}

func TestNewCommandExecutorWithNsenterTrue(t *testing.T) {
	os.Setenv("GPU_EXPORTER_USE_NSENTER", "true")
	defer os.Unsetenv("GPU_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if !e.useNsenter {
		t.Error("expected useNsenter to be true")
	}
}

func TestNewCommandExecutorWithNsenterFalse(t *testing.T) {
	os.Setenv("GPU_EXPORTER_USE_NSENTER", "false")
	defer os.Unsetenv("GPU_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if e.useNsenter {
		t.Error("expected useNsenter to be false")
	}
}

func TestNewCommandExecutorWithCustomTarget(t *testing.T) {
	os.Setenv("GPU_EXPORTER_NSENTER_TARGET", "42")
	defer os.Unsetenv("GPU_EXPORTER_NSENTER_TARGET")

	e := NewCommandExecutor()
	if e.nsenterTarget != 42 {
		t.Errorf("expected nsenterTarget 42, got %d", e.nsenterTarget)
	}
}

func TestIsNsenterEnabled(t *testing.T) {
	os.Unsetenv("GPU_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()
	if e.IsNsenterEnabled() {
		t.Error("expected IsNsenterEnabled to be false")
	}
}

func TestExecuteDirectCommand(t *testing.T) {
	os.Unsetenv("GPU_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()

	// Test executing a simple command that exists everywhere (go is guaranteed to be available in test)
	output, err := e.Execute("go", "version")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(output) == 0 {
		t.Error("expected non-empty output")
	}
}

func TestExecuteFailingCommand(t *testing.T) {
	os.Unsetenv("GPU_EXPORTER_USE_NSENTER")

	e := NewCommandExecutor()

	_, err := e.Execute("nonexistent_command_12345")
	if err == nil {
		t.Error("expected error for nonexistent command")
	}
}

func TestGetNodeName(t *testing.T) {
	// Test with NODE_NAME env var
	os.Setenv("NODE_NAME", "test-node-123")
	defer os.Unsetenv("NODE_NAME")

	name := getNodeName()
	if name != "test-node-123" {
		t.Errorf("expected test-node-123, got %s", name)
	}
}

func TestGetNodeNameFallback(t *testing.T) {
	os.Unsetenv("NODE_NAME")

	name := getNodeName()
	if name == "" {
		t.Error("expected non-empty node name")
	}
}

