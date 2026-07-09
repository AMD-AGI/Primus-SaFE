/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"context"
	"sync"
	"time"

	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/observability"
)

var (
	mu                  sync.RWMutex
	multiClusterClients = map[string]*SearchClient{}
	logsRegistry        *observability.LogsRegistry
	defaultIndex        = "node-"
)

// GetOpensearchClient returns the SearchClient for a cluster, rebuilding it if
// the discovered OpenSearch endpoint changed. Returns nil when no endpoint is
// known for the cluster (e.g. logs disabled or cluster not ready).
func GetOpensearchClient(clusterName string) *SearchClient {
	mu.RLock()
	sc, ok := multiClusterClients[clusterName]
	reg := logsRegistry
	mu.RUnlock()
	if ok {
		// No active registry -> test-injected client; return as-is.
		if reg == nil {
			return sc
		}
		// Cached client is still valid only if it points at the current
		// LogsClient (the registry swaps the object on endpoint change).
		if lc := reg.ForCluster(clusterName); lc != nil && sc.logsClient == lc {
			return sc
		}
	}
	return getOrCreateClient(clusterName)
}

// GetAnyOpensearchClient returns any available SearchClient (first cached, else
// the first discovered cluster). Used by callers that don't target a specific
// cluster.
func GetAnyOpensearchClient() *SearchClient {
	mu.RLock()
	for _, sc := range multiClusterClients {
		mu.RUnlock()
		return sc
	}
	reg := logsRegistry
	mu.RUnlock()

	if reg == nil {
		return nil
	}
	names := reg.ClusterNames()
	if len(names) == 0 {
		return nil
	}
	return getOrCreateClient(names[0])
}

func getOrCreateClient(clusterName string) *SearchClient {
	if logsRegistry == nil {
		return nil
	}
	lc := logsRegistry.ForCluster(clusterName)
	if lc == nil {
		return nil
	}

	mu.Lock()
	defer mu.Unlock()
	if sc, ok := multiClusterClients[clusterName]; ok && sc.logsClient == lc {
		return sc
	}
	sc := NewClient(SearchClientConfig{DefaultIndex: defaultIndex}, lc)
	multiClusterClients[clusterName] = sc
	return sc
}

// InitDirect wires the SaFE-native direct OpenSearch path: it builds a
// per-cluster LogsRegistry from config and starts a LogsDiscovery that keeps it
// populated from Cluster CR annotations (+ a default endpoint). Callers pass a
// controller-runtime client (mgr.GetClient()); safe to call from both the
// apiserver and resource-manager. When logs are not configured (no endpoint),
// discovery finds nothing and GetOpensearchClient returns nil.
func InitDirect(ctx context.Context, k8sClient client.Client) {
	reg := observability.NewLogsRegistry(observability.LogsClientConfig{
		Username:           commonconfig.GetOpenSearchUser(),
		Password:           commonconfig.GetOpenSearchPasswd(),
		InsecureSkipVerify: commonconfig.GetObservabilityLogsInsecureSkipVerify(),
	})
	disc := observability.NewLogsDiscovery(k8sClient, reg, observability.LogsDiscoveryConfig{
		Interval:        30 * time.Second,
		AnnotationKey:   commonconfig.GetObservabilityLogsEndpointAnnotation(),
		DefaultEndpoint: commonconfig.GetObservabilityLogsEndpoint(),
	})
	disc.Start(ctx)

	mu.Lock()
	defer mu.Unlock()
	logsRegistry = reg
	multiClusterClients = make(map[string]*SearchClient)
	if prefix := commonconfig.GetObservabilityLogsIndexPrefix(); prefix != "" {
		defaultIndex = prefix
	}
	klog.Infof("[opensearch] initialized SaFE-native direct mode (index prefix: %s, default endpoint: %q)",
		defaultIndex, commonconfig.GetObservabilityLogsEndpoint())
}

// SetLogsRegistryForTest installs a registry directly and resets the client
// cache. Tests only.
func SetLogsRegistryForTest(reg *observability.LogsRegistry) {
	mu.Lock()
	defer mu.Unlock()
	logsRegistry = reg
	multiClusterClients = make(map[string]*SearchClient)
}

func StartDiscover(_ interface{}) error {
	return nil
}
