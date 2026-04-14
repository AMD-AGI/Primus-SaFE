/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	robustAPIPort            = 8085
	robustAPIServiceTemplate = "http://robust-api.primus-robust.svc:8085"
	annotationRobustEndpoint = "primus-safe.amd.com/robust-api-endpoint"
)

// Discovery watches Cluster CRs and auto-registers robust-api endpoints on the Client.
type Discovery struct {
	k8sClient client.Client
	rc        *Client
	interval  time.Duration
	stopOnce  sync.Once
	stopCh    chan struct{}
}

// NewDiscovery creates a discovery that syncs Cluster CRs to robust client endpoints.
func NewDiscovery(k8sClient client.Client, rc *Client, interval time.Duration) *Discovery {
	return &Discovery{
		k8sClient: k8sClient,
		rc:        rc,
		interval:  interval,
		stopCh:    make(chan struct{}),
	}
}

// Start begins the periodic sync loop.
func (d *Discovery) Start(ctx context.Context) {
	go func() {
		d.syncOnce(ctx)
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-d.stopCh:
				return
			case <-ticker.C:
				d.syncOnce(ctx)
			}
		}
	}()
	klog.Infof("[robustclient] discovery started (interval=%s)", d.interval)
}

// Stop halts the sync loop.
func (d *Discovery) Stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
}

func (d *Discovery) syncOnce(ctx context.Context) {
	clusterList := &v1.ClusterList{}
	if err := d.k8sClient.List(ctx, clusterList); err != nil {
		klog.Warningf("[robustclient] list clusters failed: %v", err)
		return
	}

	seen := make(map[string]bool, len(clusterList.Items))
	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		name := cluster.Name
		seen[name] = true

		if !cluster.IsReady() {
			continue
		}

		endpoint := resolveRobustEndpoint(cluster)
		if endpoint == "" {
			continue
		}

		existing := d.rc.ForCluster(name)
		if existing != nil && existing.BaseURL() == endpoint {
			continue
		}

		d.rc.RegisterCluster(name, endpoint)
		klog.Infof("[robustclient] discovered cluster %s -> %s", name, endpoint)
	}

	for _, name := range d.rc.ClusterNames() {
		if !seen[name] {
			d.rc.RemoveCluster(name)
			klog.Infof("[robustclient] removed stale cluster %s", name)
		}
	}
}

// resolveRobustEndpoint determines the robust-api endpoint for a cluster.
// Priority: annotation > status endpoints (port substitution) > default service template.
func resolveRobustEndpoint(cluster *v1.Cluster) string {
	if ep, ok := cluster.Annotations[annotationRobustEndpoint]; ok && ep != "" {
		return ep
	}

	if eps := cluster.Status.ControlPlaneStatus.Endpoints; len(eps) > 0 {
		return fmt.Sprintf("http://%s:%d", extractHost(eps[0]), robustAPIPort)
	}

	return ""
}

func extractHost(endpoint string) string {
	if len(endpoint) > 8 && endpoint[:8] == "https://" {
		endpoint = endpoint[8:]
	} else if len(endpoint) > 7 && endpoint[:7] == "http://" {
		endpoint = endpoint[7:]
	}
	for i, ch := range endpoint {
		if ch == ':' || ch == '/' {
			return endpoint[:i]
		}
	}
	return endpoint
}
