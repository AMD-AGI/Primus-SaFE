/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	commonclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/k8sclient"
)

// GetEndpoint retrieve the endpoint address of the given cluster.
// It first tries the ClusterIP of the Kubernetes Service associated with the cluster.
// If the Service is not found or has no ports, it falls back to status endpoints.
// Returns an error if the cluster is nil, not ready, or no valid endpoint can be found.
func GetEndpoint(ctx context.Context, cli client.Client, cluster *v1.Cluster) (string, error) {
	if cluster == nil || !cluster.IsReady() {
		return "", fmt.Errorf("cluster is not ready")
	}
	service := &corev1.Service{}
	err := cli.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, service)
	if err == nil {
		if len(service.Spec.Ports) == 0 {
			return "", fmt.Errorf("service ports are empty")
		}
		return fmt.Sprintf("%s:%d", service.Spec.ClusterIP, service.Spec.Ports[0].Port), nil
	}
	if !apierrors.IsNotFound(err) {
		return "", fmt.Errorf("get service %s: %w", cluster.Name, err)
	}
	if len(cluster.Status.ControlPlaneStatus.Endpoints) == 0 {
		return "", fmt.Errorf("either the Service address or the Endpoint is empty")
	}
	return cluster.Status.ControlPlaneStatus.Endpoints[0], nil
}

// GetFallbackEndpoints returns direct apiserver endpoints from cluster status.
func GetFallbackEndpoints(cluster *v1.Cluster) []string {
	if cluster == nil {
		return nil
	}
	return append([]string(nil), cluster.Status.ControlPlaneStatus.Endpoints...)
}

// UsesServiceEndpoint reports whether the cluster has a Service fronting its apiserver.
func UsesServiceEndpoint(ctx context.Context, cli client.Client, cluster *v1.Cluster) bool {
	if cluster == nil {
		return false
	}
	service := &corev1.Service{}
	err := cli.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, service)
	return err == nil && len(service.Spec.Ports) > 0
}

// GetControlPlaneBackendIPs returns sorted apiserver backend IPs from admin-plane Endpoints.
func GetControlPlaneBackendIPs(ctx context.Context, cli client.Client, cluster *v1.Cluster) ([]string, error) {
	if cluster == nil {
		return nil, fmt.Errorf("cluster is nil")
	}
	endpoints := &corev1.Endpoints{}
	err := cli.Get(ctx, client.ObjectKey{Name: cluster.Name, Namespace: common.PrimusSafeNamespace}, endpoints)
	if err != nil {
		return nil, err
	}
	ips := make([]string, 0)
	for _, subset := range endpoints.Subsets {
		for _, addr := range subset.Addresses {
			if addr.IP != "" {
				ips = append(ips, addr.IP)
			}
		}
	}
	sort.Strings(ips)
	return ips, nil
}

// BackendIPsFingerprint builds a stable fingerprint for control-plane backend IPs.
func BackendIPsFingerprint(ips []string) string {
	if len(ips) == 0 {
		return ""
	}
	sorted := append([]string(nil), ips...)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}

// StatusEndpointsFingerprint builds a fingerprint for apiserver endpoints recorded on the cluster.
func StatusEndpointsFingerprint(cluster *v1.Cluster) string {
	endpoints := GetFallbackEndpoints(cluster)
	if len(endpoints) == 0 {
		return ""
	}
	normalized := make([]string, 0, len(endpoints))
	for _, ep := range endpoints {
		ep = commonclient.NormalizeEndpointHost(ep)
		if ep != "" {
			normalized = append(normalized, ep)
		}
	}
	sort.Strings(normalized)
	return strings.Join(normalized, ",")
}

// NewClientFactoryForCluster builds a data-plane client factory for the cluster.
// Service mode relies on client-go dial defaults; direct endpoints use ServerVersion probe failover.
func NewClientFactoryForCluster(ctx context.Context, adminClient client.Client, cluster *v1.Cluster,
	informerType commonclient.InformerType,
) (*commonclient.ClientFactory, error) {
	endpoint, err := GetEndpoint(ctx, adminClient, cluster)
	if err != nil {
		return nil, err
	}
	cps := &cluster.Status.ControlPlaneStatus
	if UsesServiceEndpoint(ctx, adminClient, cluster) {
		factory, err := commonclient.NewClientFactory(ctx, cluster.Name, endpoint,
			cps.CertData, cps.KeyData, cps.CAData, informerType)
		if err != nil {
			return nil, err
		}
		if ips, ipErr := GetControlPlaneBackendIPs(ctx, adminClient, cluster); ipErr == nil {
			factory.SetBackendFingerprint(BackendIPsFingerprint(ips))
		}
		return factory, nil
	}
	fallbacks := GetFallbackEndpoints(cluster)
	primary := endpoint
	var rest []string
	if len(fallbacks) > 0 {
		primary = fallbacks[0]
		rest = fallbacks[1:]
	}
	factory, err := commonclient.NewClientFactoryWithFallbacks(ctx, cluster.Name, primary, rest,
		cps.CertData, cps.KeyData, cps.CAData, informerType)
	if err != nil {
		return nil, err
	}
	factory.SetBackendFingerprint(StatusEndpointsFingerprint(cluster))
	return factory, nil
}

// ClientFactoryNeedsRefresh reports whether an existing factory should be replaced.
// A rebuild is only reported when it can actually succeed, so a full control-plane outage
// keeps the current factory instead of rebuilding it on every reconcile.
func ClientFactoryNeedsRefresh(ctx context.Context, adminClient client.Client, cluster *v1.Cluster,
	factory *commonclient.ClientFactory) bool {
	if factory == nil || cluster == nil {
		return true
	}
	if !clientFactoryOutdated(ctx, adminClient, cluster, factory) {
		return false
	}
	return rebuildTargetReachable(ctx, adminClient, cluster, factory)
}

// clientFactoryOutdated reports whether the factory no longer matches the desired apiserver target.
func clientFactoryOutdated(ctx context.Context, adminClient client.Client, cluster *v1.Cluster,
	factory *commonclient.ClientFactory) bool {
	if !factory.IsValid() {
		return true
	}
	selected := commonclient.NormalizeEndpointHost(factory.Endpoint())
	if UsesServiceEndpoint(ctx, adminClient, cluster) {
		endpoint, err := GetEndpoint(ctx, adminClient, cluster)
		if err != nil {
			return factory.BackendFingerprint() != ""
		}
		if selected != commonclient.NormalizeEndpointHost(endpoint) {
			return true
		}
		ips, err := GetControlPlaneBackendIPs(ctx, adminClient, cluster)
		if err != nil {
			return factory.BackendFingerprint() != ""
		}
		return factory.BackendFingerprint() != BackendIPsFingerprint(ips)
	}
	if selected == "" {
		return true
	}
	if StatusEndpointsFingerprint(cluster) != factory.BackendFingerprint() {
		return true
	}
	for _, ep := range GetFallbackEndpoints(cluster) {
		if selected == commonclient.NormalizeEndpointHost(ep) {
			return directModeFactoryNeedsRefresh(factory)
		}
	}
	return len(GetFallbackEndpoints(cluster)) > 0
}

// rebuildTargetReachable reports whether rebuilding the factory can produce a working client.
// Service mode always dials the same ClusterIP, so an unreachable target means every backend is
// down and a rebuild would only churn clients and informers until a control plane recovers.
func rebuildTargetReachable(ctx context.Context, adminClient client.Client, cluster *v1.Cluster,
	factory *commonclient.ClientFactory) bool {
	if !UsesServiceEndpoint(ctx, adminClient, cluster) {
		// Direct mode probes every candidate endpoint while building the factory.
		return true
	}
	endpoint, err := GetEndpoint(ctx, adminClient, cluster)
	if err != nil {
		return false
	}
	if commonclient.NormalizeEndpointHost(factory.Endpoint()) != commonclient.NormalizeEndpointHost(endpoint) {
		// The Service address moved, so the current client says nothing about the new target.
		return true
	}
	restCfg := factory.RestConfig()
	if restCfg == nil {
		return true
	}
	return commonclient.ProbeRESTConfig(restCfg) == nil
}

// directModeFactoryNeedsRefresh reports whether a direct-mode factory should be rebuilt.
func directModeFactoryNeedsRefresh(factory *commonclient.ClientFactory) bool {
	restCfg := factory.RestConfig()
	if restCfg == nil {
		return false
	}
	return commonclient.ProbeRESTConfig(restCfg) != nil
}
