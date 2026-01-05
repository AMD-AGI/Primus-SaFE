/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package utils

import (
	"os"
	"strings"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

func TestExecScript(t *testing.T) {
	path := "./test.sh"
	err := os.WriteFile(path, []byte("#!/bin/bash\nexit 0"), 0777)
	assert.NilError(t, err)
	defer os.Remove(path)

	args := []string{path}
	statusCode, _ := ExecuteScript(args, 0)
	assert.Equal(t, statusCode, types.StatusOk)
}

func TestExecScriptFailed(t *testing.T) {
	path := "./test.sh"
	err := os.WriteFile(path, []byte("#!/bin/bash\necho error\nexit 1"), 0777)
	assert.NilError(t, err)
	defer os.Remove(path)

	args := []string{path}
	statusCode, output := ExecuteScript(args, 0)
	assert.Equal(t, statusCode, types.StatusError)
	assert.Equal(t, output, "error")
}

func TestExecScriptWithParams(t *testing.T) {
	path := "./test.sh"
	err := os.WriteFile(path, []byte("#!/bin/bash\necho arg1=$1,arg2=$2\nexit 0"), 0777)
	assert.NilError(t, err)
	defer os.Remove(path)

	params := []string{"val1", "val2"}
	args := []string{path}
	args = append(args, params...)

	statusCode, output := ExecuteScript(args, 0)
	assert.Equal(t, statusCode, types.StatusOk)
	assert.Equal(t, strings.TrimSpace(output), "arg1=val1,arg2=val2")
}

func TestExecCommand(t *testing.T) {
	cmd := "echo hi\nexit $?"
	statusCode, output := ExecuteCommand(cmd, 0)
	assert.Equal(t, statusCode, types.StatusOk)
	assert.Equal(t, strings.TrimSpace(output), "hi")
}

func TestExecCommandWithTimeout(t *testing.T) {
	cmd := "sleep 1\necho hi\nexit 0"
	timeout := 300 * time.Millisecond
	statusCode, output := ExecuteCommand(cmd, timeout)
	assert.Equal(t, statusCode, -1)
	assert.Equal(t, strings.TrimSpace(output), "signal: killed")
}
