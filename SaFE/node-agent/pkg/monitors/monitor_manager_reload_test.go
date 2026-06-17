/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gotest.tools/assert"
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

// TestNewMonitorManager initializes manager paths from options.
func TestNewMonitorManager(t *testing.T) {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "new-mgr"})
	dir := t.TempDir()
	opts := &types.Options{ConfigMapPath: dir, ScriptPath: dir}
	mgr := NewMonitorManager(&queue, opts, newNode(t))
	assert.Assert(t, mgr != nil)
	assert.Equal(t, mgr.configPath, dir)
	assert.Equal(t, mgr.isExited, true)
}

// TestRemoveNonExistMonitor stops monitors removed from configuration.
func TestRemoveNonExistMonitor(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.keep", "safe.drop"}, []string{"test1.sh", "test2.sh"})
	defer deleteFakeConfigs(t)
	assert.NilError(t, manager.loadMonitors())
	assert.Equal(t, getMonitorsCount(manager), 2)

	manager.removeNonExistMonitor([]*MonitorConfig{
		newMonitorConfig("safe.keep", "test1.sh"),
	})
	assert.Equal(t, getMonitorsCount(manager), 1)
	assert.Assert(t, manager.getMonitor("safe.drop") == nil)
}

// TestReloadMonitorsCronChange restarts a monitor when its schedule changes.
func TestReloadMonitorsCronChange(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.cron"}, []string{"test1.sh"})
	defer deleteFakeConfigs(t)
	assert.NilError(t, manager.loadMonitors())

	updated := newMonitorConfig("safe.cron", "test1.sh")
	updated.Cronjob = "@every 2s"
	addFakeConfig(t, updated)
	assert.NilError(t, manager.reloadMonitors())

	monitor := manager.getMonitor("safe.cron")
	assert.Assert(t, monitor != nil)
	assert.Equal(t, monitor.config.Cronjob, "@every 2s")
}

// TestAddMonitor skips creation when the script file is missing.
func TestAddMonitor(t *testing.T) {
	manager := newMonitorManager(t)
	manager.addMonitor(newMonitorConfig("safe.missing", "no-script.sh"))
	assert.Equal(t, getMonitorsCount(manager), 0)
}

// TestMonitorManagerStopIdempotent ignores repeated stop calls.
func TestMonitorManagerStopIdempotent(t *testing.T) {
	manager := newMonitorManager(t)
	manager.isExited = true
	manager.Stop()
	manager.Stop()
}

// TestGetMonitorReturnsNilForMissingKey returns nil when the monitor id is unknown.
func TestGetMonitorReturnsNilForMissingKey(t *testing.T) {
	manager := newMonitorManager(t)
	assert.Assert(t, manager.getMonitor("safe.none") == nil)
}

// TestIsMonitorsChangedDetectsConfigDiff reports true when config content changes.
func TestIsMonitorsChangedDetectsConfigDiff(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.diff"}, []string{"test1.sh"})
	defer deleteFakeConfigs(t)
	assert.NilError(t, manager.loadMonitors())

	changed := manager.isMonitorsChanged([]*MonitorConfig{
		func() *MonitorConfig {
			c := newMonitorConfig("safe.diff", "test1.sh")
			c.TimeoutSecond = 99
			return c
		}(),
	})
	assert.Equal(t, changed, true)
}

// TestReloadMonitorsNoChange skips work when configuration is unchanged.
func TestReloadMonitorsNoChange(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.same"}, []string{"test1.sh"})
	defer deleteFakeConfigs(t)
	assert.NilError(t, manager.loadMonitors())
	before := getMonitorsCount(manager)
	assert.NilError(t, manager.reloadMonitors())
	assert.Equal(t, getMonitorsCount(manager), before)
}

// TestReloadMonitorsRestartExitedMonitor starts a monitor that was previously stopped.
func TestReloadMonitorsRestartExitedMonitor(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.restart"}, []string{"test1.sh"})
	defer deleteFakeConfigs(t)
	assert.NilError(t, manager.loadMonitors())
	monitor := manager.getMonitor("safe.restart")
	assert.Assert(t, monitor != nil)
	monitor.Stop()
	assert.Equal(t, monitor.IsExited(), true)

	updated := newMonitorConfig("safe.restart", "test1.sh")
	updated.TimeoutSecond = 120
	addFakeConfig(t, updated)
	assert.NilError(t, manager.reloadMonitors())
	assert.Equal(t, monitor.IsExited(), false)
}

// TestGetMonitorConfigsSkipsWrongChip ignores configs for mismatched GPU chips.
func TestGetMonitorConfigsSkipsWrongChip(t *testing.T) {
	dir := t.TempDir()
	conf := newMonitorConfig("safe.nv", "test1.sh")
	conf.Chip = string(v1.NvidiaGpuChip)
	data, err := json.Marshal(conf)
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "nv.json"), data, 0644))

	manager := newMonitorManager(t)
	manager.configPath = dir
	configs, err := manager.getMonitorConfigs(dir)
	assert.NilError(t, err)
	assert.Equal(t, len(configs), 0)
}
