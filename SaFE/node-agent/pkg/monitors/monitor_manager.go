/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package monitors

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"k8s.io/klog/v2"

	commonfaults "github.com/AMD-AIG-AIMA/SAFE/common/pkg/faults"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

// MonitorManager manages multiple monitors, handling their lifecycle and configuration updates.
type MonitorManager struct {
	// All monitors
	monitors sync.Map
	// Queue for monitor results
	queue *types.MonitorQueue
	// Current node being monitored
	node *node.Node
	// Used to control whether to exit the monitor
	tomb *channel.Tomb
	// Path to monitor configuration files
	configPath string
	// Path to monitor scripts
	scriptPath string
	// Flag indicating if manager has exited
	isExited bool
}

// NewMonitorManager creates a new MonitorManager instance.
func NewMonitorManager(queue *types.MonitorQueue, opts *types.Options, node *node.Node) *MonitorManager {
	mgr := &MonitorManager{
		queue:      queue,
		node:       node,
		tomb:       channel.NewTomb(),
		configPath: opts.ConfigMapPath,
		scriptPath: opts.ScriptPath,
		isExited:   true,
	}
	return mgr
}

// Start initializes and starts all monitors, begins watching for config changes.
func (mgr *MonitorManager) Start() error {
	if err := mgr.startMonitors(); err != nil {
		return err
	}
	go mgr.updateConfig()
	mgr.isExited = false
	return nil
}

// Stop terminates all monitors and stops the manager.
func (mgr *MonitorManager) Stop() {
	if !mgr.isExited && mgr.tomb != nil {
		mgr.tomb.Stop()
		mgr.stopMonitors()
	}
	mgr.isExited = true
	return
}

// startMonitors loads and starts all configured monitors.
func (mgr *MonitorManager) startMonitors() error {
	if err := mgr.loadMonitors(); err != nil {
		return err
	}
	count := 0
	mgr.monitors.Range(func(key, value interface{}) bool {
		if inst, ok := value.(*Monitor); ok {
			inst.Start()
			count++
		}
		return true
	})
	klog.Infof("start all monitors, total count: %d", count)
	return nil
}

// stopMonitors stops all currently running monitors.
func (mgr *MonitorManager) stopMonitors() {
	count := 0
	mgr.monitors.Range(func(key, value interface{}) bool {
		inst, ok := value.(*Monitor)
		if !ok {
			return true
		}
		inst.Stop()
		count++
		return true
	})
	klog.Infof("stop all monitors, total count: %d", count)
}

// updateConfig watches for configuration file changes and triggers reloads.
func (mgr *MonitorManager) updateConfig() {
	defer mgr.tomb.Done()

	retryCount := 0
	baseDelay := time.Second
	maxDelay := 30 * time.Second

	for {
		select {
		case <-mgr.tomb.Stopping():
			klog.Infof("stop to watch dir: %s", mgr.configPath)
			return
		default:
			if err := mgr.watchConfig(); err != nil {
				retryCount++
				delay := time.Duration(1<<uint(min(retryCount, 10))) * baseDelay
				if delay > maxDelay {
					delay = maxDelay
				}
				klog.ErrorS(err, "failed to watch config, retrying with backoff...", "delay", delay)
				time.Sleep(delay)
			} else {
				retryCount = 0
			}
		}
	}
}

// watchConfig sets up filesystem watcher to monitor config directory.
func (mgr *MonitorManager) watchConfig() error {
	watcher, err := utils.GetDirWatcher(mgr.configPath)
	if err != nil {
		klog.ErrorS(err, "failed to get watcher", "path", mgr.configPath)
		return err
	}
	defer func() {
		if err = watcher.Close(); err != nil {
			klog.ErrorS(err, "failed to close dir watcher")
		}
	}()

	timeout := time.After(10 * time.Minute)
	klog.Infof("start to watch dir(%s) to update config", mgr.configPath)
	for {
		select {
		case <-mgr.tomb.Stopping():
			return nil
		case <-timeout:
			return nil //restart after timeout
		case ev, ok := <-watcher.Events:
			if ok && (ev.Op == fsnotify.Create || ev.Op == fsnotify.Write || ev.Op == fsnotify.Remove) {
				if err = mgr.reloadMonitors(); err != nil {
					klog.ErrorS(err, "failed to reload monitors")
				}
			}
		case err, ok := <-watcher.Errors:
			if err != nil {
				return err
			} else if !ok {
				return fmt.Errorf("unknown error")
			}
			return nil
		}
	}
}

// loadMonitors loads all monitor configurations from config files.
func (mgr *MonitorManager) loadMonitors() error {
	allConfigs, err := mgr.getMonitorConfigs(mgr.configPath)
	if err != nil {
		return err
	}
	for i, config := range allConfigs {
		monitor := NewMonitor(allConfigs[i], mgr.queue, mgr.node, mgr.scriptPath)
		if monitor != nil {
			klog.Infof("load monitor. id: %s", config.Id)
			mgr.monitors.Store(config.Id, monitor)
		}
	}
	return nil
}

// reloadMonitors reloads monitor configurations when files change.
func (mgr *MonitorManager) reloadMonitors() error {
	newMonitorConfigs, err := mgr.getMonitorConfigs(mgr.configPath)
	if err != nil {
		return err
	}
	if !mgr.isMonitorsChanged(newMonitorConfigs) {
		return nil
	}
	mgr.removeNonExistMonitor(newMonitorConfigs)

	// Add a new monitor or process the existing monitor
	for _, newConf := range newMonitorConfigs {
		currentMonitor := mgr.getMonitor(newConf.Id)
		switch {
		case currentMonitor == nil:
			mgr.addMonitor(newConf)
		case currentMonitor.config.Cronjob != newConf.Cronjob:
			// If the key configuration of monitor is changed, restart it
			mgr.removeMonitor(newConf.Id)
			mgr.addMonitor(newConf)
		default:
			*currentMonitor.config = *newConf
			if currentMonitor.IsExited() {
				currentMonitor.Start()
			}
		}
	}
	return nil
}

// isMonitorsChanged checks if monitor configurations have changed.
func (mgr *MonitorManager) isMonitorsChanged(newConfigs []*MonitorConfig) bool {
	newConfigsMap := make(map[string]*MonitorConfig)
	for i := range newConfigs {
		newConfigsMap[newConfigs[i].Id] = newConfigs[i]
	}

	count := 0
	mgr.monitors.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	if count != len(newConfigsMap) {
		return true
	}

	isChanged := false
	mgr.monitors.Range(func(key, value interface{}) bool {
		id, ok := key.(string)
		if !ok {
			return true
		}
		monitor, ok := value.(*Monitor)
		if !ok {
			return true
		}
		newConfig, exists := newConfigsMap[id]
		if !exists {
			isChanged = true
			return false
		}
		if !reflect.DeepEqual(*monitor.config, *newConfig) {
			isChanged = true
			return false
		}
		return true
	})
	return isChanged
}

// removeNonExistMonitor removes monitors that no longer exist in configuration.
func (mgr *MonitorManager) removeNonExistMonitor(newConfigs []*MonitorConfig) {
	newConfigsSet := sets.NewSet()
	for _, c := range newConfigs {
		newConfigsSet.Insert(c.Id)
	}
	var toDelKeys []string
	mgr.monitors.Range(func(key, value interface{}) bool {
		id, ok := key.(string)
		if !ok {
			return true
		}
		if !newConfigsSet.Has(id) {
			toDelKeys = append(toDelKeys, id)
		}
		return true
	})
	for _, key := range toDelKeys {
		m := mgr.getMonitor(key)
		if m != nil {
			mgr.addDisableMessage(m.config.Id)
			m.Stop()
		}
		mgr.monitors.Delete(key)
	}
}

// addMonitor creates and starts a new monitor.
func (mgr *MonitorManager) addMonitor(conf *MonitorConfig) {
	monitor := NewMonitor(conf, mgr.queue, mgr.node, mgr.scriptPath)
	if monitor == nil {
		return
	}
	mgr.monitors.Store(conf.Id, monitor)
	monitor.Start()
}

// removeMonitor stops and removes a monitor by ID.
func (mgr *MonitorManager) removeMonitor(key string) {
	monitor := mgr.getMonitor(key)
	if monitor != nil {
		monitor.Stop()
	}
	mgr.monitors.Delete(key)
}

// getMonitor retrieves a monitor by ID.
func (mgr *MonitorManager) getMonitor(key string) *Monitor {
	val, ok := mgr.monitors.Load(key)
	if !ok {
		return nil
	}
	monitor, ok := val.(*Monitor)
	if !ok {
		return nil
	}
	return monitor
}

// getMonitorConfigs reads and validates all monitor configuration files.
func (mgr *MonitorManager) getMonitorConfigs(configPath string) ([]*MonitorConfig, error) {
	var results []*MonitorConfig
	files, err := os.ReadDir(configPath)
	if err != nil {
		klog.ErrorS(err, "failed to read directory", "path", configPath)
		return nil, err
	}
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".") {
			continue
		}
		path := filepath.Join(configPath, f.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			klog.ErrorS(err, "failed to read file", "path", path)
			continue
		}
		conf := &MonitorConfig{}
		if err = json.Unmarshal(data, conf); err != nil {
			klog.ErrorS(err, "failed to unmarshal json", "data", string(data))
			continue
		}
		if !conf.IsEnable() || !mgr.node.IsMatchGpuChip(conf.Chip) {
			key := commonfaults.GenerateTaintKey(conf.Id)
			if mgr.node.FindConditionByType(key) != nil {
				mgr.addDisableMessage(conf.Id)
			}
			continue
		}
		conf.SetDefaults()
		if err = conf.Validate(); err != nil {
			klog.ErrorS(err, "invalid config, skip it", "data", string(data))
			continue
		}
		results = append(results, conf)
	}
	return results, nil
}

// addDisableMessage adds a disable message to the monitor queue.
func (mgr *MonitorManager) addDisableMessage(id string) {
	item := &types.MonitorMessage{
		Id:         id,
		StatusCode: types.StatusDisable,
	}
	(*mgr.queue).Add(item)
}
