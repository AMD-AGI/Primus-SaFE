/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// Exec creates a new process with the specified arguments.
// Setpgid ensures all child processes (including nsenter descendants) belong to
// the same process group so they can be killed together on timeout.
func Exec(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// execute a shell command
func ExecuteCommand(cmd string, timeout time.Duration) (int, string) {
	args := []string{"-c", cmd}
	return ExecuteScript(args, timeout)
}

// Execute a script, where args contains the script path and its input arguments.
func ExecuteScript(args []string, timeout time.Duration) (int, string) {
	var ctx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), timeout)
	} else {
		ctx, cancel = context.WithCancel(context.Background())
	}
	defer cancel()
	cmd := Exec(ctx, "/bin/bash", args...)

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	value := strings.TrimSpace(buf.String())
	statusCode := types.StatusUnknown
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			statusCode = exitError.ExitCode()
		}
		if value == "" {
			value = err.Error()
		}
	} else {
		statusCode = types.StatusOk
	}
	return statusCode, value
}
