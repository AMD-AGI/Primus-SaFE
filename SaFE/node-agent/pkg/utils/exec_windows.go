//go:build windows

/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"context"
	"os/exec"
	"time"
)

// Exec creates a new process with the specified arguments.
// Windows has no process groups like Unix Setpgid; cancel kills the direct child only.
func Exec(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return cmd.Process.Kill()
	}
	cmd.WaitDelay = 5 * time.Second
	return cmd
}
