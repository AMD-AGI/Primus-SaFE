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
	corev1 "k8s.io/api/core/v1"

	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
)

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
