/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package daemon

import (
	"flag"
	"os"
	"testing"

	"gotest.tools/assert"
)

// TestNewDaemonFailsOnNodeInit stops when the in-cluster Kubernetes client cannot be created.
func TestNewDaemonFailsOnNodeInit(t *testing.T) {
	dir := t.TempDir()
	flag.CommandLine = flag.NewFlagSet("daemon-new-test", flag.ContinueOnError)
	os.Args = []string{
		"daemon-new-test",
		"-node_name=test-node",
		"-configmap_path=" + dir,
		"-script_path=" + dir,
		"-log_file_path=" + os.DevNull,
	}
	_, err := NewDaemon()
	assert.Assert(t, err != nil)
}
