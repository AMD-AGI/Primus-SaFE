/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"sync"

	"k8s.io/klog/v2"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

var (
	mu                  sync.RWMutex
	multiClusterClients = map[string]*SearchClient{}
	robustClientRef     *robustclient.Client
	defaultIndex        = "fluentbit-"
)

func GetOpensearchClient(clusterName string) *SearchClient {
	return getOrCreateClient(clusterName)
}

func GetAnyOpensearchClient() *SearchClient {
	if robustClientRef == nil {
		return nil
	}
	names := robustClientRef.ClusterNames()
	if len(names) == 0 {
		return nil
	}
	return getOrCreateClient(names[0])
}

// getOrCreateClient returns a SearchClient for the cluster, rebuilding it when
// the underlying robust endpoint has changed.
//
// B2: robustclient.RegisterCluster replaces the *ClusterClient object whenever
// a cluster's endpoint changes (it never mutates in place). A SearchClient
// caches a pointer to the ClusterClient it was built with, so a cached entry
// whose clusterClient pointer no longer matches the current one is stale and
// must be rebuilt — otherwise the apiserver keeps hitting the old endpoint
// until it is restarted.
func getOrCreateClient(clusterName string) *SearchClient {
	if robustClientRef == nil {
		return nil
	}
	cc := robustClientRef.ForCluster(clusterName)
	if cc == nil {
		return nil
	}

	// Fast path: cached client that still points at the current ClusterClient.
	mu.RLock()
	if sc, ok := multiClusterClients[clusterName]; ok && sc.clusterClient == cc {
		mu.RUnlock()
		return sc
	}
	mu.RUnlock()

	mu.Lock()
	defer mu.Unlock()
	if sc, ok := multiClusterClients[clusterName]; ok && sc.clusterClient == cc {
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
	if prefix := commonconfig.GetOpenSearchIndexPrefix(); prefix != "" {
		defaultIndex = prefix
	}
	klog.Infof("[opensearch] initialized with robust client proxy mode (index prefix: %s)", defaultIndex)
}

func StartDiscover(_ interface{}) error {
	return nil
}
