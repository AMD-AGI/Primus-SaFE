// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package robust

import (
	"context"
	"sync"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/config"
	"k8s.io/klog/v2"
)

var (
	defaultTimeout time.Duration
	defaultBaseURL string
	clients        sync.Map // clusterName → *Client
)

// Init initializes the Robust data plane integration based on config.
// Per-cluster Robust API URLs are read from cluster_config.robust_api_url
// in the control plane DB. The config.DataPlane.Robust.DefaultBaseURL is
// used as fallback when a cluster has no robust_api_url set.
func Init(ctx context.Context, cfg *config.Config) {
	if cfg.GetDataPlaneMode() == "local" {
		klog.Info("[robust] data plane mode: local (no Robust integration)")
		return
	}

	defaultTimeout = cfg.DataPlane.Robust.Timeout
	if defaultTimeout == 0 {
		defaultTimeout = 10 * time.Second
	}
	defaultBaseURL = cfg.GetRobustDefaultBaseURL()

	klog.Infof("[robust] data plane enabled: mode=%s defaultBaseUrl=%s",
		cfg.GetDataPlaneMode(), defaultBaseURL)
}

// RegisterCluster registers a Robust API client for a specific cluster.
// Called by the control plane cluster manager when loading cluster configs.
func RegisterCluster(clusterName, robustAPIURL string) {
	if robustAPIURL == "" {
		robustAPIURL = defaultBaseURL
	}
	if robustAPIURL == "" {
		return
	}
	client := NewClient(robustAPIURL, defaultTimeout)
	clients.Store(clusterName, client)
	klog.V(2).Infof("[robust] registered cluster %s → %s", clusterName, robustAPIURL)
}

// UnregisterCluster removes a cluster's Robust client.
func UnregisterCluster(clusterName string) {
	clients.Delete(clusterName)
}

// GetClientForCluster returns the Robust client for a cluster, or nil if not registered.
func GetClientForCluster(clusterName string) *Client {
	if v, ok := clients.Load(clusterName); ok {
		return v.(*Client)
	}
	if defaultBaseURL != "" {
		client := NewClient(defaultBaseURL, defaultTimeout)
		clients.Store(clusterName, client)
		return client
	}
	return nil
}
