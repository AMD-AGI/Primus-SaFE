/*
 * Copyright © AMD. 2025-2026. All rights reserved.
 */

package utils

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"gotest.tools/assert"
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
	statusCode, resp := ExecuteScript(args, 0)
	assert.Equal(t, statusCode, types.StatusError)
	assert.Equal(t, resp, "error")
}

func TestExecScriptWithParams(t *testing.T) {
	path := "./test.sh"
	err := os.WriteFile(path, []byte("#!/bin/bash\necho arg1=$1,arg2=$2\nexit 0"), 0777)
	assert.NilError(t, err)
	defer os.Remove(path)

	params := []string{"val1", "val2"}
	args := []string{path}
	args = append(args, params...)

	statusCode, resp := ExecuteScript(args, 0)
	assert.Equal(t, statusCode, types.StatusOk)
	assert.Equal(t, strings.TrimSpace(resp), "arg1=val1,arg2=val2")
}

func TestExecCommand(t *testing.T) {
	cmd := "echo hi\nexit 0"
	statusCode, resp := ExecuteCommand(cmd, 0)
	assert.Equal(t, statusCode, types.StatusOk)
	assert.Equal(t, strings.TrimSpace(resp), "hi")
}

func TestExecCommandWithTimeout(t *testing.T) {
	cmd := "sleep 1\necho hi\nexit 0"
	timeout := 300 * time.Millisecond
	statusCode, resp := ExecuteCommand(cmd, timeout)
	assert.Equal(t, statusCode, -1)
	assert.Equal(t, strings.TrimSpace(resp), "signal: killed")
}
