/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package observability

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// This file is the logs counterpart to metrics.go / registry.go / discovery.go.
// It provides a direct OpenSearch client plus a per-cluster registry and a
// Cluster-CR discovery loop, so the management plane can query container/node
// logs straight from each cluster's OpenSearch instead of proxying through the
// data-plane primus-robust robust-analyzer.

const (
	// defaultLogsTimeout bounds a single OpenSearch request.
	defaultLogsTimeout = 120 * time.Second
	// defaultLogsEndpointAnnotation is the Cluster CR annotation carrying a
	// per-cluster OpenSearch endpoint override.
	defaultLogsEndpointAnnotation = "primus-safe.amd.com/logs-endpoint"
)

// ── LogsClient ──────────────────────────────────────────────────────────

// LogsClientConfig configures a per-endpoint OpenSearch client.
type LogsClientConfig struct {
	// BaseURL is the OpenSearch HTTP root, e.g.
	// "https://primus-safe-logs.primus-safe-observability.svc:9200".
	BaseURL string
	// Username / Password are the OpenSearch basic-auth credentials. Empty
	// username sends the request unauthenticated (e.g. a security-disabled
	// dev cluster).
	Username string
	Password string
	// Timeout bounds a single HTTP request. Zero uses defaultLogsTimeout.
	Timeout time.Duration
	// InsecureSkipVerify disables TLS verification, needed for the OpenSearch
	// operator's self-signed HTTP cert.
	InsecureSkipVerify bool
}

// LogsClient issues raw HTTP requests against a single OpenSearch endpoint
// (typically one data cluster's OpenSearch service).
type LogsClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// NewLogsClient builds a logs client for one OpenSearch endpoint.
func NewLogsClient(cfg LogsClientConfig) *LogsClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultLogsTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		}
	}
	return &LogsClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		username:   cfg.Username,
		password:   cfg.Password,
		httpClient: &http.Client{Timeout: timeout, Transport: transport},
	}
}

// BaseURL returns the configured OpenSearch root.
func (c *LogsClient) BaseURL() string { return c.baseURL }

// Request performs an HTTP request against the OpenSearch endpoint. path is
// appended to BaseURL verbatim (e.g. "/node-2026.07.09/_search?..."). It
// returns the raw response body, turning non-2xx responses into an error that
// includes a truncated body so callers don't parse an error page as JSON.
func (c *LogsClient) Request(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("opensearch client base URL not configured")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, fmt.Errorf("create opensearch request: %w", err)
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opensearch request to %s: %w", c.baseURL+path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read opensearch response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("opensearch returned HTTP %d: %s", resp.StatusCode, truncate(string(respBody), 200))
	}
	return respBody, nil
}

// ── LogsRegistry ────────────────────────────────────────────────────────

// LogsRegistry holds one LogsClient per data cluster. It mirrors the shape of
// MetricsRegistry (RegisterCluster / RemoveCluster / ForCluster / ClusterNames)
// so the two observability backends discover endpoints the same way.
type LogsRegistry struct {
	mu       sync.RWMutex
	clusters map[string]*LogsClient
	defaults LogsClientConfig
}

// NewLogsRegistry builds an empty registry. defaults carries per-client
// settings (credentials, timeout, TLS) applied to every endpoint registered
// later; only BaseURL varies per cluster.
func NewLogsRegistry(defaults LogsClientConfig) *LogsRegistry {
	return &LogsRegistry{
		clusters: make(map[string]*LogsClient),
		defaults: defaults,
	}
}

// RegisterCluster adds or updates the OpenSearch endpoint for a cluster.
func (r *LogsRegistry) RegisterCluster(clusterName, endpoint string) {
	cfg := r.defaults
	cfg.BaseURL = endpoint

	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusters[clusterName] = NewLogsClient(cfg)
	klog.V(2).Infof("[observability] registered logs endpoint %s -> %s", clusterName, endpoint)
}

// RemoveCluster drops a cluster's OpenSearch endpoint.
func (r *LogsRegistry) RemoveCluster(clusterName string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.clusters, clusterName)
}

// ForCluster returns the logs client for a cluster, or nil if unregistered.
func (r *LogsRegistry) ForCluster(clusterName string) *LogsClient {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.clusters[clusterName]
}

// ClusterNames lists all registered cluster names.
func (r *LogsRegistry) ClusterNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.clusters))
	for name := range r.clusters {
		names = append(names, name)
	}
	return names
}

// ── LogsDiscovery ───────────────────────────────────────────────────────

// LogsDiscoveryConfig configures endpoint resolution (mirrors
// MetricsDiscoveryConfig).
type LogsDiscoveryConfig struct {
	Interval        time.Duration
	AnnotationKey   string
	DefaultEndpoint string
}

// LogsDiscovery watches Cluster CRs and keeps a LogsRegistry populated with
// each ready cluster's OpenSearch endpoint. Resolution priority per cluster:
//  1. Cluster CR annotation (default key primus-safe.amd.com/logs-endpoint) —
//     for cross-cluster setups where OpenSearch is exposed via NodePort /
//     LoadBalancer / Ingress on the data cluster.
//  2. A shared default endpoint (the in-cluster OpenSearch Service DNS) applied
//     to every ready cluster (co-located management + data plane).
type LogsDiscovery struct {
	k8sClient       client.Client
	registry        *LogsRegistry
	interval        time.Duration
	annotationKey   string
	defaultEndpoint string
	stopOnce        sync.Once
	stopCh          chan struct{}
}

// NewLogsDiscovery creates a discovery loop bound to a registry.
func NewLogsDiscovery(k8sClient client.Client, registry *LogsRegistry, cfg LogsDiscoveryConfig) *LogsDiscovery {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	annotationKey := cfg.AnnotationKey
	if annotationKey == "" {
		annotationKey = defaultLogsEndpointAnnotation
	}
	return &LogsDiscovery{
		k8sClient:       k8sClient,
		registry:        registry,
		interval:        interval,
		annotationKey:   annotationKey,
		defaultEndpoint: cfg.DefaultEndpoint,
		stopCh:          make(chan struct{}),
	}
}

// Start runs the reconcile loop in a background goroutine.
func (d *LogsDiscovery) Start(ctx context.Context) {
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
	klog.Infof("[observability] logs discovery started (interval=%s)", d.interval)
}

// Stop halts the reconcile loop.
func (d *LogsDiscovery) Stop() {
	d.stopOnce.Do(func() { close(d.stopCh) })
}

func (d *LogsDiscovery) syncOnce(ctx context.Context) {
	clusterList := &v1.ClusterList{}
	if err := d.k8sClient.List(ctx, clusterList); err != nil {
		klog.Warningf("[observability] list clusters failed: %v", err)
		return
	}

	seen := make(map[string]bool, len(clusterList.Items))
	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]
		name := cluster.Name
		if !cluster.IsReady() {
			continue
		}
		endpoint := d.resolveEndpoint(cluster)
		if endpoint == "" {
			continue
		}
		seen[name] = true

		existing := d.registry.ForCluster(name)
		if existing != nil && existing.BaseURL() == strings.TrimRight(endpoint, "/") {
			continue
		}
		d.registry.RegisterCluster(name, endpoint)
		klog.Infof("[observability] discovered logs endpoint %s -> %s", name, endpoint)
	}

	for _, name := range d.registry.ClusterNames() {
		if !seen[name] {
			d.registry.RemoveCluster(name)
			klog.Infof("[observability] removed stale logs endpoint %s", name)
		}
	}
}

func (d *LogsDiscovery) resolveEndpoint(cluster *v1.Cluster) string {
	if ep, ok := cluster.Annotations[d.annotationKey]; ok && ep != "" {
		return ep
	}
	return d.defaultEndpoint
}
