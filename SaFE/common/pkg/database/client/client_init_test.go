/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package client

import (
	"os"
	"os/exec"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// childEnv marks the re-executed copy of this test that is expected to die.
const childEnv = "SAFE_DB_CLIENT_FATAL_CHILD"

// TestNewClientExitsWhenItCannotBeBuilt pins the one thing that cannot be
// asserted in-process, because the behaviour under test is the process ending.
//
// The alternative it replaced is why this is worth a subprocess. `once` is spent
// whether the body succeeded or not, so returning on failure left a nil
// singleton for the life of the process -- no retry, and callers that could not
// tell "not ready" from "never will be". One of them logs a warning and skips
// registering its controller, so the failure was both permanent and quiet.
//
// Run as a child rather than mocked: klog.Fatalf calls os.Exit, so a test that
// reached it in-process would take the test binary with it, and stubbing the exit
// would test the stub rather than what a pod does.
func TestNewClientExitsWhenItCannotBeBuilt(t *testing.T) {
	if os.Getenv(childEnv) == "1" {
		// No config, so checkParams finds no dbname, user, password or host.
		viper.Reset()
		NewClient()
		// Reached only if the failure path returned instead of exiting, which is
		// the regression this test exists for. The parent reads the zero status.
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestNewClientExitsWhenItCannotBeBuilt")
	cmd.Env = append(os.Environ(), childEnv+"=1")
	out, err := cmd.CombinedOutput()

	require.Error(t, err, "a db client that cannot be built must end the process, not return nil:\n%s", out)
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "expected an exit status, got %v", err)
	assert.NotEqual(t, 0, exitErr.ExitCode())
	assert.Contains(t, string(out), "failed to check db params",
		"the exit has to say which check failed, or a restarting pod explains nothing")
}
