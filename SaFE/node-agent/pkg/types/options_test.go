/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package types

import (
	"flag"
	"os"
	"testing"

	"gotest.tools/assert"
)

// TestOptionsInitNilReceiver reports error when options pointer is nil.
func TestOptionsInitNilReceiver(t *testing.T) {
	var opt *Options
	err := opt.Init()
	assert.Error(t, err, "the options is not initialized")
}

// TestOptionsInitMissingNodeName fails when node_name flag is absent.
func TestOptionsInitMissingNodeName(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	opt := &Options{}
	err := opt.Init()
	assert.ErrorContains(t, err, "node_name")
}

// TestOptionsInitMissingConfigMapPath fails when configmap_path flag is absent.
func TestOptionsInitMissingConfigMapPath(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	opt := &Options{}
	os.Args = []string{"test", "-node_name=node-1"}
	err := opt.Init()
	assert.ErrorContains(t, err, "configmap_path")
}

// TestOptionsInitSuccess parses required flags.
func TestOptionsInitSuccess(t *testing.T) {
	flag.CommandLine = flag.NewFlagSet("test", flag.ContinueOnError)
	opt := &Options{}
	os.Args = []string{"test", "-node_name=node-1", "-configmap_path=/etc/config"}
	err := opt.Init()
	assert.NilError(t, err)
	assert.Equal(t, opt.NodeName, "node-1")
	assert.Equal(t, opt.ConfigMapPath, "/etc/config")
}
