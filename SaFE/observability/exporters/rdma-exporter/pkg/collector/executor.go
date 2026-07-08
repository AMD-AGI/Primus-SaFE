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
	useNsenter := os.Getenv("RDMA_EXPORTER_USE_NSENTER")
	switch strings.ToLower(useNsenter) {
	case "true", "1", "yes":
		e.useNsenter = true
	case "false", "0", "no":
		e.useNsenter = false
	default:
		// RDMA exporter defaults to false (no nsenter)
		// because rdma command usually works inside container
		e.useNsenter = false
	}

	// Parse nsenter target
	if target := os.Getenv("RDMA_EXPORTER_NSENTER_TARGET"); target != "" {
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

// Execute runs command with or without nsenter
func (e *CommandExecutor) Execute(cmdName string, args ...string) ([]byte, error) {
	var cmd *exec.Cmd

	if e.useNsenter {
		// Build nsenter command (RDMA uses simpler nsenter flags)
		nsenterArgs := []string{
			"--target", strconv.Itoa(e.nsenterTarget),
			"--mount",
			"--net",
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

	// Check rdma tool
	_, err := executor.Execute("rdma", "dev")
	if err != nil {
		slog.Warn("rdma tool may not be available", "error", err)
	}

	return nil
}
