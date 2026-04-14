/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"sync"

	"k8s.io/klog/v2"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

var (
	mu                  sync.RWMutex
	multiClusterClients = map[string]*SearchClient{}
	robustClientRef     *robustclient.Client
	defaultIndex        = "fluentbit-"
)

func GetOpensearchClient(clusterName string) *SearchClient {
	mu.RLock()
	if sc, ok := multiClusterClients[clusterName]; ok {
		mu.RUnlock()
		return sc
	}
	mu.RUnlock()
	return getOrCreateClient(clusterName)
}

func GetAnyOpensearchClient() *SearchClient {
	mu.RLock()
	for _, sc := range multiClusterClients {
		mu.RUnlock()
		return sc
	}
	mu.RUnlock()

	if robustClientRef == nil {
		return nil
	}
	names := robustClientRef.ClusterNames()
	if len(names) == 0 {
		return nil
	}
	return getOrCreateClient(names[0])
}

func getOrCreateClient(clusterName string) *SearchClient {
	if robustClientRef == nil {
		return nil
	}
	cc := robustClientRef.ForCluster(clusterName)
	if cc == nil {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()
	if sc, ok := multiClusterClients[clusterName]; ok {
		return sc
	}
	sc := NewClient(SearchClientConfig{DefaultIndex: defaultIndex}, cc)
	multiClusterClients[clusterName] = sc
	return sc
}

func InitRobustClient(rc *robustclient.Client) {
	mu.Lock()
	defer mu.Unlock()
	robustClientRef = rc
	multiClusterClients = make(map[string]*SearchClient)
	klog.V(2).Info("[opensearch] initialized with robust client proxy mode")
}

func StartDiscover(_ interface{}) error {
	return nil
}
