/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package daemon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/exporters"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/monitors"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestDaemonInitConfig loads yaml config from the config map directory.
func TestDaemonInitConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, types.AppConfig)
	assert.NilError(t, os.WriteFile(cfg, []byte("log_level: info\n"), 0644))
	d := &Daemon{}
	assert.NilError(t, d.initConfig(dir))
}

// TestDaemonInitConfigMissingFile returns error when config file is absent.
func TestDaemonInitConfigMissingFile(t *testing.T) {
	d := &Daemon{}
	err := d.initConfig(t.TempDir())
	assert.Assert(t, err != nil)
}

// TestDaemonStartWithoutInit logs and returns when daemon is not initialized.
func TestDaemonStartWithoutInit(t *testing.T) {
	d := &Daemon{}
	d.Start()
}

// TestDaemonStopWithoutComponents shuts down safely with nil subsystems.
func TestDaemonStopWithoutComponents(t *testing.T) {
	d := &Daemon{}
	d.Stop()
}

// TestDaemonStopWithMonitors shuts down the monitor manager and work queue.
func TestDaemonStopWithMonitors(t *testing.T) {
	manager, _, queue := newDaemonTestComponents(t)
	d := &Daemon{
		monitors: manager,
		queue:    queue,
		isInited: true,
	}
	d.Stop()
}

// TestDaemonStartUninitializedNode returns early when node startup fails.
func TestDaemonStartUninitializedNode(t *testing.T) {
	d := &Daemon{
		isInited: true,
		ctx:      context.Background(),
	}
	d.Start()
}

// TestDaemonStartMonitorLoadFails returns early when monitor configs cannot be loaded.
func TestDaemonStartMonitorLoadFails(t *testing.T) {
	_, n, queue := newDaemonTestComponents(t)
	opts := &types.Options{
		NodeName:      n.GetK8sNode().Name,
		ConfigMapPath: filepath.Join(t.TempDir(), "missing-config-dir"),
		ScriptPath:    t.TempDir(),
	}
	manager := monitors.NewMonitorManager(&queue, opts, n)
	d := &Daemon{
		ctx:       context.Background(),
		node:      n,
		monitors:  manager,
		queue:     queue,
		isInited:  true,
	}
	d.Start()
}

// TestDaemonStartCancelledContext stops when the root context is cancelled.
func TestDaemonStartCancelledContext(t *testing.T) {
	manager, n, queue := newDaemonTestComponents(t)
	ctx, cancel := context.WithCancel(context.Background())
	exp := exporters.NewExporterManager(&queue, n)
	go func() {
		time.Sleep(50 * time.Millisecond)
		queue.ShutDown()
		cancel()
	}()
	d := &Daemon{
		ctx:       ctx,
		opts:      &types.Options{NodeName: n.GetK8sNode().Name},
		queue:     queue,
		monitors:  manager,
		node:      n,
		exporters: exp,
		isInited:  true,
	}
	d.Start()
}
