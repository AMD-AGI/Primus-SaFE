package utils

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// Exec creates a new process with the specified arguments.
func Exec(ctx context.Context, name string, arg ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, arg...)
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

	output, err := cmd.CombinedOutput()
	value := strings.TrimSpace(string(output))
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
