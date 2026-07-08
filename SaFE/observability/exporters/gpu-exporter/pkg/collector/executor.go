// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package collector

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// CommandExecutor handles command execution with optional nsenter support
type CommandExecutor struct {
	useNsenter    bool
	nsenterTarget int
}

// NewCommandExecutor creates a new CommandExecutor with configuration from environment
func NewCommandExecutor() *CommandExecutor {
	e := &CommandExecutor{
		nsenterTarget: 1,
	}

	// Parse configuration
	// Default to direct execution since ROCm tools are included in the image.
	// Set GPU_EXPORTER_USE_NSENTER=true to use host tools via nsenter instead.
	useNsenter := os.Getenv("GPU_EXPORTER_USE_NSENTER")
	switch strings.ToLower(useNsenter) {
	case "true", "1", "yes":
		e.useNsenter = true
	case "auto":
		e.useNsenter = e.detectContainerEnvironment()
	default: // "false", "0", "no", or empty
		e.useNsenter = false
	}

	// Parse nsenter target
	if target := os.Getenv("GPU_EXPORTER_NSENTER_TARGET"); target != "" {
		if t, err := strconv.Atoi(target); err == nil {
			e.nsenterTarget = t
		}
	}

	slog.Info("CommandExecutor initialized",
		"use_nsenter", e.useNsenter,
		"nsenter_target", e.nsenterTarget,
	)

	return e
}

// detectContainerEnvironment checks if running inside a container
func (e *CommandExecutor) detectContainerEnvironment() bool {
	// Method 1: Check /.dockerenv (Docker)
	if _, err := os.Stat("/.dockerenv"); err == nil {
		slog.Debug("Detected Docker environment via /.dockerenv")
		return true
	}

	// Method 2: Check /run/.containerenv (Podman)
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		slog.Debug("Detected Podman environment via /run/.containerenv")
		return true
	}

	// Method 3: Check cgroup for container indicators
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") ||
			strings.Contains(content, "kubepods") ||
			strings.Contains(content, "containerd") {
			slog.Debug("Detected container environment via cgroup")
			return true
		}
	}

	// Method 4: Check for Kubernetes service account
	if _, err := os.Stat("/var/run/secrets/kubernetes.io"); err == nil {
		slog.Debug("Detected Kubernetes environment")
		return true
	}

	slog.Debug("No container environment detected, running in host mode")
	return false
}

// Execute runs command with or without nsenter
func (e *CommandExecutor) Execute(cmdName string, args ...string) ([]byte, error) {
	var cmd *exec.Cmd

	if e.useNsenter {
		// Build nsenter command
		nsenterArgs := []string{
			"--target", strconv.Itoa(e.nsenterTarget),
			"--mount",
			"--uts",
			"--ipc",
			"--net",
			"--pid",
			"--",
			cmdName,
		}
		nsenterArgs = append(nsenterArgs, args...)
		cmd = exec.Command("nsenter", nsenterArgs...)
		slog.Debug("Executing with nsenter", "command", cmdName, "args", args)
	} else {
		// Direct execution
		cmd = exec.Command(cmdName, args...)
		slog.Debug("Executing directly", "command", cmdName, "args", args)
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("command failed: %w, stderr: %s", err, errBuf.String())
	}

	return outBuf.Bytes(), nil
}

// IsNsenterEnabled returns whether nsenter is being used
func (e *CommandExecutor) IsNsenterEnabled() bool {
	return e.useNsenter
}

// RunPreflightChecks validates tool availability
func RunPreflightChecks(executor *CommandExecutor) error {
	// Check nsenter if needed
	if executor.IsNsenterEnabled() {
		if _, err := exec.LookPath("nsenter"); err != nil {
			return fmt.Errorf("nsenter not found but USE_NSENTER is enabled: %w", err)
		}
		slog.Info("nsenter is available")
	}

	// Check GPU tooling - amd-smi is the sole source for metrics and inventory.
	_, err := executor.Execute("amd-smi", "version")
	if err != nil {
		// Try alternative command
		_, err = executor.Execute("amd-smi", "list")
		if err != nil {
			slog.Warn("amd-smi may not be available", "error", err)
		}
	}

	return nil
}
