/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"sync"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// defaultRobustEndpoint is the in-cluster service DNS of robust-analyzer
	// (which hosts the robust-api module). Used when no annotation is set on
	// the Cluster CR and SaFE apiserver is co-located with robust-analyzer in
	// the same K8s cluster.
	defaultRobustEndpoint    = "http://robust-analyzer.primus-robust.svc:8085"
	annotationRobustEndpoint = "primus-safe.amd.com/robust-api-endpoint"
)

// Discovery watches Cluster CRs and auto-registers robust-analyzer endpoints on the Client.
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

// resolveRobustEndpoint determines the robust-analyzer endpoint for a cluster.
// Priority:
//  1. Annotation primus-safe.amd.com/robust-api-endpoint on the Cluster CR.
//     Use this for cross-cluster setups where robust-analyzer is reachable
//     via NodePort, LoadBalancer, or Ingress on the data cluster.
//  2. In-cluster service DNS of robust-analyzer. This works when SaFE
//     apiserver and robust-analyzer are deployed in the same K8s cluster.
//
// Note: ControlPlaneStatus.Endpoints (K8s API server addresses) are NOT
// used here. robust-analyzer is a regular pod behind a ClusterIP service,
// not a hostNetwork process listening on the control-plane node IP.
// The annotation key keeps the legacy "robust-api" name for backward
// compatibility with already-deployed Cluster CRs.
func resolveRobustEndpoint(cluster *v1.Cluster) string {
	if ep, ok := cluster.Annotations[annotationRobustEndpoint]; ok && ep != "" {
		return ep
	}
	return defaultRobustEndpoint
}
