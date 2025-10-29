/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
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
	"k8s.io/client-go/util/workqueue"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
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
