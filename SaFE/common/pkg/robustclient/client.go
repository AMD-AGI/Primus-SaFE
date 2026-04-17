/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package robustclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// Client manages per-cluster HTTP client connections to data-plane robust-api instances.
// All SaFE modules (apiserver, resource-manager, etc.) share a single Client via DI.
type Client struct {
	mu       sync.RWMutex
	clusters map[string]*ClusterClient
	defaults ClientConfig
}

type ClientConfig struct {
	Timeout         time.Duration
	HealthInterval  time.Duration
	DefaultPort     int
	ServiceTemplate string
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:         120 * time.Second,
		HealthInterval:  30 * time.Second,
		DefaultPort:     8085,
		ServiceTemplate: "http://robust-analyzer.primus-robust.svc.cluster.local:8085",
	}
}

func NewClient(cfg ClientConfig) *Client {
	return &Client{
		clusters: make(map[string]*ClusterClient),
		defaults: cfg,
	}
}

// RegisterCluster adds or updates a cluster endpoint.
func (c *Client) RegisterCluster(clusterName, endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clusters[clusterName] = &ClusterClient{
		clusterName: clusterName,
		baseURL:     endpoint,
		httpClient:  &http.Client{Timeout: c.defaults.Timeout},
	}
	klog.V(2).Infof("[robustclient] registered cluster %s -> %s", clusterName, endpoint)
}

// RemoveCluster removes a cluster endpoint.
func (c *Client) RemoveCluster(clusterName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.clusters, clusterName)
}

// ForCluster returns a cluster-scoped client. Returns nil if cluster not registered.
func (c *Client) ForCluster(clusterName string) *ClusterClient {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clusters[clusterName]
}

// ClusterNames returns all registered cluster names.
func (c *Client) ClusterNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.clusters))
	for name := range c.clusters {
		names = append(names, name)
	}
	return names
}

// ClusterClient wraps HTTP calls to a specific data cluster's robust-api.
type ClusterClient struct {
	clusterName string
	baseURL     string
	httpClient  *http.Client
}

func (cc *ClusterClient) ClusterName() string { return cc.clusterName }
func (cc *ClusterClient) BaseURL() string     { return cc.baseURL }

// Get sends a GET request to the robust-api and decodes the response envelope.
func (cc *ClusterClient) Get(ctx context.Context, path string, query url.Values, out interface{}) error {
	reqURL := cc.baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	return cc.doAndDecode(req, out)
}

// Post sends a POST request with JSON body and decodes the response envelope.
func (cc *ClusterClient) Post(ctx context.Context, path string, body interface{}, out interface{}) error {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	reqURL := cc.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	return cc.doAndDecode(req, out)
}

// Delete sends a DELETE request and decodes the response envelope.
func (cc *ClusterClient) Delete(ctx context.Context, path string, out interface{}) error {
	reqURL := cc.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return cc.doAndDecode(req, out)
}

// RawPost sends a POST with JSON body and returns the raw response body without envelope unwrapping.
func (cc *ClusterClient) RawPost(ctx context.Context, path string, body interface{}) ([]byte, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	reqURL := cc.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("robust-api %s %s: %w", cc.clusterName, path, err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// RawGet sends a GET and returns the raw response body without envelope unwrapping.
func (cc *ClusterClient) RawGet(ctx context.Context, path string, query url.Values) ([]byte, error) {
	reqURL := cc.baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("robust-api %s %s: %w", cc.clusterName, path, err)
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// HealthCheck pings /healthz on the robust-api. Returns nil if healthy.
func (cc *ClusterClient) HealthCheck(ctx context.Context) error {
	reqURL := cc.baseURL + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

func (cc *ClusterClient) doAndDecode(req *http.Request, out interface{}) error {
	resp, err := cc.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("robust-api %s %s %s: %w", cc.clusterName, req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("robust-api %s returned %d: %s",
			req.URL.Path, resp.StatusCode, truncate(string(body), 500))
	}

	if out == nil {
		return nil
	}

	var envelope ResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Meta.Code != 0 || envelope.Data != nil) {
		if envelope.Meta.Code != 0 && envelope.Meta.Code != 2000 {
			msg := envelope.Meta.Message
			if msg == "" {
				msg = "unknown error"
			}
			return fmt.Errorf("robust-api %s error code %d: %s", req.URL.Path, envelope.Meta.Code, msg)
		}
		if len(bytes.TrimSpace(envelope.Data)) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
			return nil
		}
		return json.Unmarshal(envelope.Data, out)
	}

	return json.Unmarshal(body, out)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
