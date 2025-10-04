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

func (mgr *MonitorManager) Start() error {
	if err := mgr.startMonitors(); err != nil {
		return err
	}
	go mgr.updateConfig()
	mgr.isExited = false
	return nil
}

func (mgr *MonitorManager) Stop() {
	if !mgr.isExited && mgr.tomb != nil {
		mgr.tomb.Stop()
		mgr.stopMonitors()
	}
	mgr.isExited = true
	return
}

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

func (mgr *MonitorManager) updateConfig() {
	defer mgr.tomb.Done()

	for {
		select {
		case <-mgr.tomb.Stopping():
			klog.Infof("stop to watch dir: %s", mgr.configPath)
			return
		default:
			if err := mgr.watchConfig(); err != nil {
				time.Sleep(time.Second)
			}
		}
	}
}

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

	klog.Infof("start to watch dir(%s) to update config", mgr.configPath)
	for {
		select {
		case <-mgr.tomb.Stopping():
			return nil
		case ev, ok := <-watcher.Events:
			if ok && (ev.Op == fsnotify.Create || ev.Op == fsnotify.Write || ev.Op == fsnotify.Remove) {
				if err = mgr.reloadMonitors(); err != nil {
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

func (mgr *MonitorManager) loadMonitors() error {
	allConfigs, err := mgr.getMonitorConfigs(mgr.configPath)
	if err != nil {
		return err
	}
	for i, conf := range allConfigs {
		monitor := NewMonitor(allConfigs[i], mgr.queue, mgr.node, mgr.scriptPath)
		if monitor != nil {
			klog.Infof("load monitor. id: %s", conf.Id)
			mgr.monitors.Store(conf.Id, monitor)
		}
	}
	return nil
}

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

func (mgr *MonitorManager) isMonitorsChanged(newConfigs []*MonitorConfig) bool {
	newConfigsMap := make(map[string]*MonitorConfig)
	for i := range newConfigs {
		newConfigsMap[newConfigs[i].Id] = newConfigs[i]
	}

	count := 0
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

func (mgr *MonitorManager) addMonitor(conf *MonitorConfig) {
	monitor := NewMonitor(conf, mgr.queue, mgr.node, mgr.scriptPath)
	if monitor == nil {
		return
	}
	mgr.monitors.Store(conf.Id, monitor)
	monitor.Start()
}

func (mgr *MonitorManager) removeMonitor(key string) {
	monitor := mgr.getMonitor(key)
	if monitor != nil {
		monitor.Stop()
	}
	mgr.monitors.Delete(key)
}

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

func (mgr *MonitorManager) addDisableMessage(id string) {
	item := &types.MonitorMessage{
		Id:         id,
		StatusCode: types.StatusDisable,
	}
	(*mgr.queue).Add(item)
}
