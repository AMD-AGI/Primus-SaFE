/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package failover

import (
	"encoding/json"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	commonutils "github.com/AMD-AIG-AIMA/SAFE/common/pkg/utils"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/sets"
)

// FailoverConfig represents the configuration for failover monitoring
type FailoverConfig struct {
	Id string `json:"id"`
}

// Release cleans up resources associated with the FailoverConfig
// it implements the interface of commonutils.Object
func (conf *FailoverConfig) Release() error {
	return nil
}

// addFailoverConfig processes a ConfigMap and updates the failover manager with configurations
// It parses JSON configurations from the ConfigMap data, normalizes IDs to lowercase,
// adds new configurations to the manager, and removes obsolete ones
// Parameters:
//   - cm: The ConfigMap containing failover configurations
//   - failoverManager: The ObjectManager to store configurations
func addFailoverConfig(cm *corev1.ConfigMap, failoverManager *commonutils.ObjectManager) {
	currentIds := sets.NewSet()
	for _, val := range cm.Data {
		conf := &FailoverConfig{}
		if err := json.Unmarshal([]byte(val), conf); err != nil {
			klog.ErrorS(err, "failed to unmarshal json", "data", val)
			continue
		}
		conf.Id = strings.ToLower(conf.Id)
		currentIds.Insert(conf.Id)
		if !isMonitorIdExists(failoverManager, conf.Id) {
			failoverManager.AddOrReplace(conf.Id, conf)
		}
	}
	ids, _ := failoverManager.GetAll()
	for _, id := range ids {
		if !currentIds.Has(id) {
			failoverManager.Delete(id)
		}
	}
}

// isMonitorIdExists checks if a monitor ID exists in the failover manager
// Parameters:
//   - failoverManager: The ObjectManager to search in
//   - id: The identifier to look for
//
// Returns:
//   - bool: True if the ID exists and is a valid FailoverConfig, false otherwise
func isMonitorIdExists(failoverManager *commonutils.ObjectManager, id string) bool {
	obj, ok := failoverManager.Get(id)
	if !ok {
		return false
	}
	conf, ok := obj.(*FailoverConfig)
	if ok && conf != nil {
		return true
	}
	return false
}
