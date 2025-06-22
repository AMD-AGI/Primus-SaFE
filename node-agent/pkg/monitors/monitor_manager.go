/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
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

	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/node"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/types"
	"github.com/AMD-AIG-AIMA/SAFE/node-agent/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/channel"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

type MonitorManager struct {
	monitors   sync.Map
	queue      *types.MonitorQueue
	node       *node.Node
	tomb       *channel.Tomb
	configPath string
	scriptPath string
	isExited   bool
}

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

func (mm *MonitorManager) Start() error {
	if err := mm.startMonitors(); err != nil {
		return err
	}
	go mm.updateConfig()
	mm.isExited = false
	return nil
}

func (mm *MonitorManager) Stop() {
	if !mm.isExited && mm.tomb != nil {
		mm.tomb.Stop()
		mm.stopMonitors()
	}
	mm.isExited = true
	return
}

func (mm *MonitorManager) startMonitors() error {
	if err := mm.loadMonitors(); err != nil {
		return err
	}
	count := 0
	mm.monitors.Range(func(key, value interface{}) bool {
		if inst, ok := value.(*Monitor); ok {
			inst.Start()
			count++
		}
		return true
	})
	klog.Infof("start all monitors, total count: %d", count)
	return nil
}

func (mm *MonitorManager) stopMonitors() {
	count := 0
	mm.monitors.Range(func(key, value interface{}) bool {
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

func (mm *MonitorManager) updateConfig() {
	defer mm.tomb.Done()

	for {
		select {
		case <-mm.tomb.Stopping():
			klog.Infof("stop to watch dir: %s", mm.configPath)
			return
		default:
			if err := mm.watchConfig(); err != nil {
				time.Sleep(time.Second)
			}
		}
	}
}

func (mm *MonitorManager) watchConfig() error {
	watcher, err := utils.GetDirWatcher(mm.configPath)
	if err != nil {
		klog.ErrorS(err, "failed to get watcher", "path", mm.configPath)
		return err
	}
	defer func() {
		if err = watcher.Close(); err != nil {
			klog.ErrorS(err, "failed to close dir watcher")
		}
	}()

	klog.Infof("start to watch dir(%s) to update config", mm.configPath)
	for {
		select {
		case <-mm.tomb.Stopping():
			return nil
		case ev, ok := <-watcher.Events:
			if ok && (ev.Op == fsnotify.Create || ev.Op == fsnotify.Write || ev.Op == fsnotify.Remove) {
				if err = mm.reloadMonitors(); err != nil {
					klog.ErrorS(err, "failed to reload monitors")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return fmt.Errorf("unknown error")
			} else {
				return err
			}
		}
	}
}

func (mm *MonitorManager) loadMonitors() error {
	allConfigs, err := mm.getMonitorConfigs(mm.configPath)
	if err != nil {
		return err
	}
	for i, conf := range allConfigs {
		monitor := NewMonitor(allConfigs[i], mm.queue, mm.node, mm.scriptPath)
		if monitor != nil {
			klog.Infof("load monitor. id: %s", conf.Id)
			mm.monitors.Store(conf.Id, monitor)
		}
	}
	return nil
}

func (mm *MonitorManager) reloadMonitors() error {
	newMonitorConfigs, err := mm.getMonitorConfigs(mm.configPath)
	if err != nil {
		return err
	}
	if !mm.isMonitorsChanged(newMonitorConfigs) {
		return nil
	}
	mm.removeNonExistMonitor(newMonitorConfigs)

	// Add a new monitor or process the existing monitor
	for _, newConf := range newMonitorConfigs {
		currentMonitor := mm.getMonitor(newConf.Id)
		switch {
		case currentMonitor == nil:
			mm.addMonitor(newConf)
		case currentMonitor.config.Cronjob != newConf.Cronjob ||
			!reflect.DeepEqual(currentMonitor.config.Arguments, newConf.Arguments):
			// If the key configuration of monitor is changed, restart it
			mm.removeMonitor(newConf.Id)
			mm.addMonitor(newConf)
		default:
			*currentMonitor.config = *newConf
			if currentMonitor.IsExited() {
				currentMonitor.Start()
			}
		}
	}
	return nil
}

func (mm *MonitorManager) isMonitorsChanged(newConfigs []*MonitorConfig) bool {
	newConfigsMap := make(map[string]*MonitorConfig)
	for i := range newConfigs {
		newConfigsMap[newConfigs[i].Id] = newConfigs[i]
	}

	count := 0
	isChanged := false
	mm.monitors.Range(func(key, value interface{}) bool {
		id, ok := key.(string)
		if !ok {
			return true
		}
		monitor, ok := value.(*Monitor)
		if !ok {
			return true
		}
		newConfig, ok := newConfigsMap[id]
		if !ok {
			isChanged = true
			return false
		}
		if !reflect.DeepEqual(*monitor.config, *newConfig) {
			isChanged = true
			return false
		}
		count++
		return true
	})
	if isChanged || count != len(newConfigsMap) {
		return true
	}
	return false
}

func (mm *MonitorManager) removeNonExistMonitor(newConfigs []*MonitorConfig) {
	newConfigsSet := sets.NewSet()
	for _, c := range newConfigs {
		newConfigsSet.Insert(c.Id)
	}
	var toDelKeys []string
	mm.monitors.Range(func(key, value interface{}) bool {
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
		m := mm.getMonitor(key)
		if m != nil {
			mm.addDisableMessage(m.config.Id)
			m.Stop()
		}
		mm.monitors.Delete(key)
	}
}

func (mm *MonitorManager) addMonitor(conf *MonitorConfig) {
	monitor := NewMonitor(conf, mm.queue, mm.node, mm.scriptPath)
	if monitor == nil {
		return
	}
	mm.monitors.Store(conf.Id, monitor)
	monitor.Start()
}

func (mm *MonitorManager) removeMonitor(key string) {
	monitor := mm.getMonitor(key)
	if monitor != nil {
		monitor.Stop()
	}
	mm.monitors.Delete(key)
}

func (mm *MonitorManager) getMonitor(key string) *Monitor {
	val, ok := mm.monitors.Load(key)
	if !ok {
		return nil
	}
	monitor, ok := val.(*Monitor)
	if !ok {
		return nil
	}
	return monitor
}

func (mm *MonitorManager) getMonitorConfigs(configPath string) ([]*MonitorConfig, error) {
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
		if !conf.IsEnable() || !mm.node.IsMatchChip(conf.Chip) {
			if mm.node.FindConditionByType(conf.Id) != nil {
				mm.addDisableMessage(conf.Id)
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

func (mm *MonitorManager) addDisableMessage(id string) {
	item := &types.MonitorMessage{
		Id:         id,
		StatusCode: types.StatusDisable,
	}
	(*mm.queue).Add(item)
}
