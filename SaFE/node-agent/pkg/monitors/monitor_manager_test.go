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
	"time"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
)

var (
	TestConfigPath = "./config"
)

func addFakeConfig(t *testing.T, config *MonitorConfig) {
	fullPath, err := filepath.Abs(TestConfigPath)
	err = os.Mkdir(fullPath, 0777)
	if !os.IsExist(err) {
		assert.NilError(t, err)
	}
	fullPath = filepath.Join(fullPath, config.Script)
	config.IsDebug = true
	data, err := json.Marshal(config)
	assert.NilError(t, err)
	err = utils.WriteFile(fullPath, string(data), 0777)
	assert.NilError(t, err)
}

func addFakeConfigs(t *testing.T, ids []string, scripts []string) {
	if len(ids) != len(scripts) {
		return
	}
	for i, s := range scripts {
		addFakeConfig(t, newMonitorConfig(ids[i], s))
	}
}

func deleteFakeConfigs(t *testing.T) {
	err := os.RemoveAll(TestConfigPath)
	assert.NilError(t, err)
}

func newMonitorManager(t *testing.T) *MonitorManager {
	var queue types.MonitorQueue
	queue = workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedControllerRateLimiter[*types.MonitorMessage](),
		workqueue.TypedRateLimitingQueueConfig[*types.MonitorMessage]{Name: "monitors"})
	n := newNode(t)
	fullConfigPath, err := filepath.Abs(TestConfigPath)
	assert.NilError(t, err)
	opt := &types.Options{
		ConfigMapPath: fullConfigPath,
		ScriptPath:    ".",
	}
	mgr := NewMonitorManager(&queue, opt, n)
	return mgr
}

func TestMain(m *testing.M) {
	scripts := []string{"test1.sh", "test2.sh"}
	for _, script := range scripts {
		fullPath := filepath.Join(".", script)
		os.WriteFile(fullPath, []byte("echo hi; exit 0"), 0777)
	}
	exitCode := m.Run()
	for _, script := range scripts {
		fullPath := filepath.Join(".", script)
		os.Remove(fullPath)
	}
	os.Exit(exitCode)
}

func TestStartManager(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.0", "safe.1"}, []string{"test1.sh", "test2.sh"})
	defer func() {
		deleteFakeConfigs(t)
	}()

	err := manager.Start()
	assert.NilError(t, err)
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, getMonitorsCount(manager), 2)
	monitor1 := manager.getMonitor("safe.0")
	assert.Equal(t, monitor1 != nil, true)
	assert.Equal(t, monitor1.config.Script, "test1.sh")
	assert.Equal(t, monitor1.IsExited(), false)
	monitor2 := manager.getMonitor("safe.1")
	assert.Equal(t, monitor2 != nil, true)
	assert.Equal(t, monitor2.config.Script, "test2.sh")
	assert.Equal(t, monitor2.IsExited(), false)

	manager.Stop()
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, monitor1.IsExited(), true)
	assert.Equal(t, monitor2.IsExited(), true)
}

func TestMonitorAdded(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.0"}, []string{"test1.sh"})
	defer func() {
		deleteFakeConfigs(t)
	}()

	err := manager.Start()
	assert.NilError(t, err)
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, getMonitorsCount(manager), 1)
	monitor := manager.getMonitor("safe.0")
	assert.Equal(t, monitor != nil, true)
	assert.Equal(t, monitor.config.Script, "test1.sh")
	assert.Equal(t, monitor.IsExited(), false)

	addFakeConfig(t, newMonitorConfig("safe.1", "test2.sh"))
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, getMonitorsCount(manager), 2)
	monitor2 := manager.getMonitor("safe.1")
	assert.Equal(t, monitor2 != nil, true)
	assert.Equal(t, monitor2.config.Script, "test2.sh")
	assert.Equal(t, monitor2.IsExited(), false)
	manager.Stop()

	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, monitor.IsExited(), true)
	assert.Equal(t, monitor2.IsExited(), true)
}

func TestMonitorRemoved(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.0", "safe.1"}, []string{"test1.sh", "test2.sh"})
	defer func() {
		deleteFakeConfigs(t)
	}()

	err := manager.Start()
	assert.NilError(t, err)
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, getMonitorsCount(manager), 2)
	monitor := manager.getMonitor("safe.0")
	assert.Equal(t, monitor != nil, true)
	assert.Equal(t, monitor.config.Script, "test1.sh")
	assert.Equal(t, monitor.IsExited(), false)
	monitor2 := manager.getMonitor("safe.1")
	assert.Equal(t, monitor2 != nil, true)
	assert.Equal(t, monitor2.config.Script, "test2.sh")
	assert.Equal(t, monitor2.IsExited(), false)
	time.Sleep(time.Millisecond * 200)

	path := filepath.Join(manager.configPath, "test2.sh")
	os.Remove(path)
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, getMonitorsCount(manager), 1)
	monitor = manager.getMonitor("safe.0")
	assert.Equal(t, monitor != nil, true)
	assert.Equal(t, monitor.config.Script, "test1.sh")
	monitor2 = manager.getMonitor("safe.1")
	assert.Equal(t, monitor2 == nil, true)

	manager.Stop()
}

func TestMonitorRestart(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.0"}, []string{"test1.sh"})
	defer func() {
		deleteFakeConfigs(t)
	}()

	err := manager.Start()
	assert.NilError(t, err)
	time.Sleep(time.Millisecond * 200)
	defer func() {
		manager.Stop()
	}()

	assert.Equal(t, getMonitorsCount(manager), 1)
	monitor := manager.getMonitor("safe.0")
	assert.Equal(t, monitor != nil, true)
	assert.Equal(t, monitor.IsExited(), false)

	assert.Equal(t, getMonitorsCount(manager), 1)
	config := newMonitorConfig("safe.0", "test1.sh")
	config.Disabled()
	addFakeConfig(t, config)
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, monitor.IsExited(), true)

	config2 := newMonitorConfig("safe.0", "test1.sh")
	addFakeConfig(t, config2)
	time.Sleep(time.Millisecond * 200)
	assert.Equal(t, getMonitorsCount(manager), 1)
	monitor = manager.getMonitor("safe.0")
	assert.Equal(t, monitor.IsExited(), false)
}

func TestMonitorChipChanged(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.0"}, []string{"test1.sh"})
	defer func() {
		deleteFakeConfigs(t)
	}()

	err := manager.Start()
	assert.NilError(t, err)
	time.Sleep(time.Millisecond * 200)
	defer func() {
		manager.Stop()
	}()

	time.Sleep(time.Millisecond * 100)
	assert.Equal(t, getMonitorsCount(manager), 1)
	monitor := manager.getMonitor("safe.0")
	assert.Equal(t, monitor != nil, true)
	assert.Equal(t, monitor.IsExited(), false)
	assert.Equal(t, monitor.config.Chip, "")

	config := newMonitorConfig("safe.0", "test1.sh")
	config.Chip = string(v1.AmdGpuChip)
	addFakeConfig(t, config)
	time.Sleep(time.Millisecond * 200)

	assert.Equal(t, getMonitorsCount(manager), 1)
	monitor2 := manager.getMonitor("safe.0")
	assert.Equal(t, monitor, monitor2)
	assert.Equal(t, monitor2.IsExited(), false)
	assert.Equal(t, monitor2.config.Chip, string(v1.AmdGpuChip))
}

func getMonitorsCount(manager *MonitorManager) int {
	count := 0
	manager.monitors.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// --- merged from monitor_manager_extra_test.go ---

// TestRemoveMonitor stops and deletes a monitor entry.
func TestRemoveMonitor(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.rm"}, []string{"test1.sh"})
	defer deleteFakeConfigs(t)

	assert.NilError(t, manager.loadMonitors())
	monitor := manager.getMonitor("safe.rm")
	assert.Assert(t, monitor != nil)
	manager.removeMonitor("safe.rm")
	assert.Assert(t, manager.getMonitor("safe.rm") == nil)
}

// TestGetMonitorConfigsInvalidDir returns error when config path is missing.
func TestGetMonitorConfigsInvalidDir(t *testing.T) {
	manager := newMonitorManager(t)
	_, err := manager.getMonitorConfigs(filepath.Join(t.TempDir(), "no-such-dir"))
	assert.Assert(t, err != nil)
}

// TestGetMonitorConfigsSkipsInvalidJSON ignores malformed config files.
func TestGetMonitorConfigsSkipsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{"), 0644))
	manager := newMonitorManager(t)
	manager.configPath = dir
	configs, err := manager.getMonitorConfigs(dir)
	assert.NilError(t, err)
	assert.Equal(t, len(configs), 0)
}

// TestIsMonitorsChangedDetectsCountMismatch reports true when monitor count differs.
func TestIsMonitorsChangedDetectsCountMismatch(t *testing.T) {
	manager := newMonitorManager(t)
	addFakeConfigs(t, []string{"safe.a"}, []string{"test1.sh"})
	defer deleteFakeConfigs(t)
	assert.NilError(t, manager.loadMonitors())

	changed := manager.isMonitorsChanged([]*MonitorConfig{
		newMonitorConfig("safe.a", "test1.sh"),
		newMonitorConfig("safe.b", "test2.sh"),
	})
	assert.Equal(t, changed, true)
}

// TestAddDisableMessage enqueues a disable status message.
func TestAddDisableMessage(t *testing.T) {
	manager := newMonitorManager(t)
	manager.addDisableMessage("safe.disable")
	msg, ok := (*manager.queue).Get()
	assert.Equal(t, ok, false)
	assert.Equal(t, msg.StatusCode, types.StatusDisable)
	(*manager.queue).Done(msg)
}

// TestGetMonitorConfigsAddsDisableMessage enqueues disable when a disabled monitor still has a condition.
func TestGetMonitorConfigsAddsDisableMessage(t *testing.T) {
	manager := newMonitorManager(t)
	dir := t.TempDir()
	conf := newMonitorConfig("safe.off", "test1.sh")
	conf.Disabled()
	data, err := json.Marshal(conf)
	assert.NilError(t, err)
	assert.NilError(t, os.WriteFile(filepath.Join(dir, "off.json"), data, 0644))

	key := commonfaults.GenerateTaintKey("safe.off")
	assert.NilError(t, manager.node.UpdateConditions([]corev1.NodeCondition{{
		Type:   corev1.NodeConditionType(key),
		Status: corev1.ConditionTrue,
	}}))

	manager.configPath = dir
	configs, err := manager.getMonitorConfigs(dir)
	assert.NilError(t, err)
	assert.Equal(t, len(configs), 0)

	msg, shutdown := (*manager.queue).Get()
	assert.Equal(t, shutdown, false)
	assert.Equal(t, msg.StatusCode, types.StatusDisable)
	(*manager.queue).Done(msg)
}

// --- merged from monitor_manager_reload_test.go ---

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
