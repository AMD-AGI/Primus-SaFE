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

	commonconfig "github.com/AMD-AIG-AIMA/SAFE/common/pkg/config"
)

// ServiceTokenHeader carries the shared service token the Robust API accepts
// from other components, on reads and on its machine-to-machine writes
// (workloads/sync and the like). It must match the robust-api write guard's
// auth.write_guard.service_token.
const ServiceTokenHeader = "X-Robust-Service-Token"

// Client manages per-cluster HTTP client connections to data-plane robust-analyzer instances.
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
	// ServiceToken returns the Robust service token sent on every request, or
	// "" to send none. It is called per request so a rotated Secret is picked
	// up without a restart. Every caller of this client is a SaFE component
	// acting after its own authentication and authorization, which is what the
	// Robust API's service token stands for.
	ServiceToken func() string
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		Timeout:         120 * time.Second,
		HealthInterval:  30 * time.Second,
		DefaultPort:     8085,
		ServiceTemplate: "http://robust-analyzer.primus-robust.svc.cluster.local:8085",
		ServiceToken:    commonconfig.GetRobustServiceToken,
	}
}

func NewClient(cfg ClientConfig) *Client {
	// Say once whether calls will be authenticated: without the token the
	// Robust API answers every request except /healthz with 401.
	if cfg.ServiceToken == nil || cfg.ServiceToken() == "" {
		klog.Warning("[robustclient] no Robust service token configured (robust.secret_path); " +
			"the Robust API rejects unauthenticated calls")
	} else {
		klog.Info("[robustclient] Robust service token configured")
	}
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
		token:       c.defaults.ServiceToken,
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

// ClusterClient wraps HTTP calls to a specific data cluster's robust-analyzer.
type ClusterClient struct {
	clusterName string
	baseURL     string
	httpClient  *http.Client
	token       func() string
}

// do sends req with the service token attached. Every request goes through
// here, so no method can reach the Robust API without credentials.
func (cc *ClusterClient) do(req *http.Request) (*http.Response, error) {
	if cc.token != nil {
		if token := cc.token(); token != "" {
			req.Header.Set(ServiceTokenHeader, token)
		}
	}
	return cc.httpClient.Do(req)
}

func (cc *ClusterClient) ClusterName() string { return cc.clusterName }
func (cc *ClusterClient) BaseURL() string     { return cc.baseURL }

// Get sends a GET request to the robust-analyzer and decodes the response envelope.
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
// Non-2xx responses are turned into an error including the response body prefix so callers
// don't accidentally parse an HTML/plaintext error page as JSON.
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

	return cc.doRaw(req, path)
}

// RawGet sends a GET and returns the raw response body without envelope unwrapping.
// Non-2xx responses are turned into an error (see RawPost).
func (cc *ClusterClient) RawGet(ctx context.Context, path string, query url.Values) ([]byte, error) {
	reqURL := cc.baseURL + path
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	return cc.doRaw(req, path)
}

func (cc *ClusterClient) doRaw(req *http.Request, path string) ([]byte, error) {
	resp, err := cc.do(req)
	if err != nil {
		return nil, fmt.Errorf("robust-analyzer %s %s: %w", cc.clusterName, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read robust-analyzer response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("robust-analyzer %s %s returned HTTP %d: %s",
			cc.clusterName, path, resp.StatusCode, truncate(string(respBody), 200))
	}
	return respBody, nil
}

// HealthCheck pings /healthz on the robust-analyzer. Returns nil if healthy.
func (cc *ClusterClient) HealthCheck(ctx context.Context) error {
	reqURL := cc.baseURL + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	resp, err := cc.do(req)
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
	resp, err := cc.do(req)
	if err != nil {
		return fmt.Errorf("robust-analyzer %s %s %s: %w", cc.clusterName, req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("robust-analyzer %s returned %d: %s",
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
			return fmt.Errorf("robust-analyzer %s error code %d: %s", req.URL.Path, envelope.Meta.Code, msg)
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
