/*
 * Copyright (c) 2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package failover

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"

	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

const (
	FailoverConfigmapName = "primus-safe-failover"
	GlobalRestart         = "global_restart"
)

type FailoverConfig struct {
	Key    string `json:"key"`
	Action string `json:"action"`
}

func (conf *FailoverConfig) Release() error {
	return nil
}

func addFailoverConfig(cm *corev1.ConfigMap, failoverManager *commonutils.ObjectManager) {
	currentSet := sets.NewSet()
	for _, val := range cm.Data {
		conf := &FailoverConfig{}
		if json.Unmarshal([]byte(val), conf) != nil {
			continue
		}
		conf.Key = strings.ToLower(conf.Key)
		currentSet.Insert(conf.Key)
		existConf := getFailoverConfig(failoverManager, conf.Key)
		if existConf == nil || existConf.Action != conf.Action {
			failoverManager.AddOrReplace(conf.Key, conf)
		}
	}
	keys, _ := failoverManager.GetAll()
	for _, key := range keys {
		if !currentSet.Has(key) {
			failoverManager.Delete(key)
		}
	}
}

func getFailoverConfig(failoverManager *commonutils.ObjectManager, key string) *FailoverConfig {
	obj, ok := failoverManager.Get(key)
	if !ok {
		return nil
	}
	conf, ok := obj.(*FailoverConfig)
	if ok && conf != nil {
		return conf
	}
	return nil
}
