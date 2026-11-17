/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"net"
	"sync"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// The in-cluster services that serve the Robust API, used when no
	// annotation is set on the Cluster CR and SaFE runs in the same K8s
	// cluster as Robust. The primus-robust-stack chart serves it from a
	// standalone robust-api and runs robust-analyzer as a worker, which
	// answers every API route with 404; a robust-analyzer installed alone
	// (role combined) serves the API itself.
	robustAPIHost            = "robust-api.primus-robust.svc"
	robustAPIEndpoint        = "http://" + robustAPIHost + ":8085"
	robustAnalyzerEndpoint   = "http://robust-analyzer.primus-robust.svc:8085"
	annotationRobustEndpoint = "primus-safe.amd.com/robust-api-endpoint"
)

// Discovery watches Cluster CRs and auto-registers robust-analyzer endpoints on the Client.
type Discovery struct {
	k8sClient client.Client
	rc        *Client
	interval  time.Duration
	stopOnce  sync.Once
	stopCh    chan struct{}
	// lookupHost resolves a service name, to tell which Robust services exist.
	lookupHost func(ctx context.Context, host string) ([]string, error)
}

// NewDiscovery creates a discovery that syncs Cluster CRs to robust client endpoints.
func NewDiscovery(k8sClient client.Client, rc *Client, interval time.Duration) *Discovery {
	return &Discovery{
		k8sClient:  k8sClient,
		rc:         rc,
		interval:   interval,
		stopCh:     make(chan struct{}),
		lookupHost: net.DefaultResolver.LookupHost,
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

	defaultEndpoint := d.defaultEndpoint(ctx)
	seen := make(map[string]bool, len(clusterList.Items))
	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		name := cluster.Name
		seen[name] = true

		if !cluster.IsReady() {
			continue
		}

		endpoint := resolveRobustEndpoint(cluster, defaultEndpoint)
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

// defaultEndpoint is the in-cluster Robust API: robust-api when that
// service exists, else robust-analyzer.
func (d *Discovery) defaultEndpoint(ctx context.Context) string {
	lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if addrs, err := d.lookupHost(lookupCtx, robustAPIHost); err == nil && len(addrs) > 0 {
		return robustAPIEndpoint
	}
	return robustAnalyzerEndpoint
}

// resolveRobustEndpoint determines the Robust API endpoint for a cluster.
// Priority:
//  1. Annotation primus-safe.amd.com/robust-api-endpoint on the Cluster CR.
//     Use this for cross-cluster setups where the Robust API is reachable
//     via NodePort, LoadBalancer, or Ingress on the data cluster.
//  2. The in-cluster service, fallback. This works when SaFE apiserver and
//     Robust are deployed in the same K8s cluster.
//
// Note: ControlPlaneStatus.Endpoints (K8s API server addresses) are NOT
// used here. The Robust API is a regular pod behind a ClusterIP service,
// not a hostNetwork process listening on the control-plane node IP.
func resolveRobustEndpoint(cluster *v1.Cluster, fallback string) string {
	if ep, ok := cluster.Annotations[annotationRobustEndpoint]; ok && ep != "" {
		return ep
	}
	return fallback
}
