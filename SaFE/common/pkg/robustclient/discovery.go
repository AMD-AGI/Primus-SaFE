/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
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
	// <namespace>/<name> of a Secret in the management cluster whose
	// service-token key the cluster's Robust API accepts instead of the
	// global token.
	annotationRobustTokenSecret = "primus-safe.amd.com/robust-service-token-secret"
	// <namespace>/<name> of a Secret in the management cluster whose ca.crt
	// key holds the PEM certificates the cluster's Robust API is signed by,
	// trusted on top of the system pool.
	annotationRobustCASecret = "primus-safe.amd.com/robust-api-ca-secret"
	robustCASecretKey        = "ca.crt"
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
	// managementCluster names the Cluster CR SaFE runs in, "" when unknown.
	managementCluster func() string
	// noEndpointLogged holds the clusters already reported as having no
	// Robust endpoint, so the report is made once.
	noEndpointLogged map[string]bool
	// caErrLogged holds, per cluster, the CA failure last reported, so each
	// failure is reported once.
	caErrLogged map[string]string
	// unresolvedLogged is set once the lack of a management cluster among
	// several Cluster CRs has been reported.
	unresolvedLogged bool
}

// NewDiscovery creates a discovery that syncs Cluster CRs to robust client endpoints.
func NewDiscovery(k8sClient client.Client, rc *Client, interval time.Duration) *Discovery {
	return &Discovery{
		k8sClient:  k8sClient,
		rc:         rc,
		interval:   interval,
		stopCh:     make(chan struct{}),
		lookupHost: net.DefaultResolver.LookupHost,

		managementCluster: commonconfig.GetRobustManagementCluster,
		noEndpointLogged:  make(map[string]bool),
		caErrLogged:       make(map[string]string),
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

	local, all := fallbackCluster(d.managementCluster(), clusterList.Items)
	if all && !d.unresolvedLogged {
		d.unresolvedLogged = true
		klog.Warningf("[robustclient] %d Cluster CRs and none resolves as the management cluster: set "+
			"robust.management_cluster (.env robust_management_cluster) or label exactly one Cluster %s; until then "+
			"every cluster without the %s annotation uses the in-cluster Robust services",
			len(clusterList.Items), v1.ClusterControlPlaneLabel, annotationRobustEndpoint)
	} else if !all {
		d.unresolvedLogged = false
	}
	defaultEndpoint := ""
	if local != "" || all {
		defaultEndpoint = d.defaultEndpoint(ctx)
	}
	seen := make(map[string]bool, len(clusterList.Items))
	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		name := cluster.Name
		seen[name] = true

		if !cluster.IsReady() {
			continue
		}

		fallback := ""
		if name == local || all {
			fallback = defaultEndpoint
		}
		endpoint := resolveRobustEndpoint(cluster, fallback)
		if endpoint == "" {
			if d.rc.ForCluster(name) != nil {
				d.rc.RemoveCluster(name)
			}
			if !d.noEndpointLogged[name] {
				d.noEndpointLogged[name] = true
				klog.Warningf("[robustclient] cluster %s has no %s annotation and is not the management cluster; "+
					"it gets no Robust client", name, annotationRobustEndpoint)
			}
			continue
		}
		delete(d.noEndpointLogged, name)

		tokenSecret := cluster.Annotations[annotationRobustTokenSecret]
		caSecret := cluster.Annotations[annotationRobustCASecret]
		caPEM := d.clusterCA(ctx, name, caSecret)
		existing := d.rc.ForCluster(name)
		if existing != nil && existing.BaseURL() == endpoint && existing.TokenSecret() == tokenSecret &&
			existing.CASecret() == caSecret && existing.CAHash() == CAHash(caPEM) {
			continue
		}

		var token func() string
		if tokenSecret != "" {
			token = d.secretToken(name, tokenSecret)
		}
		if err := d.rc.RegisterClusterWithCA(name, endpoint, tokenSecret, token, caSecret, caPEM); err != nil {
			klog.Errorf("[robustclient] cluster %s: CA from %s: %v; trusting the system pool only", name, caSecret, err)
			_ = d.rc.RegisterClusterWithCA(name, endpoint, tokenSecret, token, caSecret, nil)
		}
		klog.Infof("[robustclient] discovered cluster %s -> %s (token secret %q, CA secret %q)",
			name, endpoint, tokenSecret, caSecret)
	}

	for _, name := range d.rc.ClusterNames() {
		if !seen[name] {
			d.rc.RemoveCluster(name)
			klog.Infof("[robustclient] removed stale cluster %s", name)
		}
	}
	for name := range d.noEndpointLogged {
		if !seen[name] {
			delete(d.noEndpointLogged, name)
		}
	}
	for name := range d.caErrLogged {
		if !seen[name] {
			delete(d.caErrLogged, name)
		}
	}
}

// clusterCA returns the PEM certificates under key ca.crt of the Secret ref
// (<namespace>/<name>), read on every sync so a content change is noticed. It
// returns nil, leaving the cluster on the system pool, when ref is empty or the
// certificates cannot be used; such a failure is reported once per cluster.
func (d *Discovery) clusterCA(ctx context.Context, cluster, ref string) []byte {
	if ref == "" {
		delete(d.caErrLogged, cluster)
		return nil
	}
	caPEM, err := d.readCA(ctx, ref)
	if err != nil {
		if msg := ref + ": " + err.Error(); d.caErrLogged[cluster] != msg {
			d.caErrLogged[cluster] = msg
			klog.Errorf("[robustclient] cluster %s: %s %q: %v; trusting the system pool only",
				cluster, annotationRobustCASecret, ref, err)
		}
		return nil
	}
	delete(d.caErrLogged, cluster)
	return caPEM
}

func (d *Discovery) readCA(ctx context.Context, ref string) ([]byte, error) {
	namespace, name, ok := strings.Cut(ref, "/")
	if !ok || namespace == "" || name == "" || strings.Contains(name, "/") {
		return nil, errors.New("must be <namespace>/<name>")
	}
	getCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	secret := &corev1.Secret{}
	if err := d.k8sClient.Get(getCtx, client.ObjectKey{Namespace: namespace, Name: name}, secret); err != nil {
		return nil, fmt.Errorf("read secret: %w", err)
	}
	caPEM := secret.Data[robustCASecretKey]
	if _, err := certPoolWithPEM(caPEM); err != nil {
		return nil, fmt.Errorf("key %s: %w", robustCASecretKey, err)
	}
	return caPEM, nil
}

// fallbackCluster returns the only cluster allowed to use the in-cluster Robust
// services: the configured management cluster or, when none is configured, the
// single Cluster CR, else the only one labeled as the control plane cluster.
// all is true when several Cluster CRs leave it unknown; every cluster may then
// fall back, as before the management cluster existed.
func fallbackCluster(configured string, clusters []v1.Cluster) (local string, all bool) {
	if configured != "" {
		return configured, false
	}
	switch len(clusters) {
	case 0:
		return "", false
	case 1:
		return clusters[0].Name, false
	}
	for i := range clusters {
		if !v1.HasLabel(&clusters[i], v1.ClusterControlPlaneLabel) {
			continue
		}
		if local != "" {
			return "", true
		}
		local = clusters[i].Name
	}
	return local, local == ""
}

// secretToken returns a token source reading key service-token of the Secret
// ref (<namespace>/<name>) on every call, so a rotation takes effect without a
// re-registration. When the Secret cannot be read it keeps the last token read.
func (d *Discovery) secretToken(cluster, ref string) func() string {
	namespace, name, ok := strings.Cut(ref, "/")
	if !ok || namespace == "" || name == "" || strings.Contains(name, "/") {
		klog.Errorf("[robustclient] cluster %s: %s must be <namespace>/<name>, got %q; no token is sent",
			cluster, annotationRobustTokenSecret, ref)
		return func() string { return "" }
	}
	var mu sync.Mutex
	last := ""
	return func() string {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		secret := &corev1.Secret{}
		err := d.k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			klog.Warningf("[robustclient] cluster %s: read token secret %s: %v", cluster, ref, err)
			return last
		}
		last = strings.TrimSpace(string(secret.Data[commonconfig.RobustServiceTokenKey]))
		return last
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
