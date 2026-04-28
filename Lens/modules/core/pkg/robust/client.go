// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package robust provides an HTTP client for calling the Robust data-plane API.
// Each Primus-Robust cluster exposes its API at robust-analyzer.primus-robust.svc:8085.
// This client is used by the Lens control-plane API to delegate data-plane queries
// to the Robust instance running in the target cluster.
package robust

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

const (
	DefaultTimeout    = 30 * time.Second
	DefaultBasePath   = "/api/v1"
	DefaultPort       = 8085
	DefaultServiceFmt = "http://robust-analyzer.primus-robust.svc:%d"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	cluster    string
}

type Option func(*Client)

func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.httpClient = hc
	}
}

// NewClient creates a Robust API client for the given cluster.
// baseURL should be like "http://robust-analyzer.primus-robust.svc:8085".
func NewClient(baseURL, clusterName string, opts ...Option) *Client {
	c := &Client{
		baseURL: baseURL,
		cluster: clusterName,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// NewClientForCluster creates a client using the default in-cluster service DNS.
func NewClientForCluster(clusterName string, opts ...Option) *Client {
	baseURL := fmt.Sprintf(DefaultServiceFmt, DefaultPort)
	return NewClient(baseURL, clusterName, opts...)
}

func (c *Client) ClusterName() string { return c.cluster }
func (c *Client) BaseURL() string     { return c.baseURL }

// Get performs a GET request to the Robust API and decodes JSON into result.
// The Robust API envelope is automatically stripped before decoding.
func (c *Client) Get(ctx context.Context, path string, params url.Values, result interface{}) error {
	u := c.baseURL + DefaultBasePath + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("robust: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("robust: %s %s: %w", c.cluster, path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("robust: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		log.Warnf("Robust API error [%s] %s -> %d: %s", c.cluster, path, resp.StatusCode, truncate(body, 200))
		return fmt.Errorf("robust: %s returned %d", path, resp.StatusCode)
	}

	if result != nil {
		data := unwrapEnvelope(body)
		if err := json.Unmarshal(data, result); err != nil {
			return fmt.Errorf("robust: decode response for %s: %w", path, err)
		}
	}
	return nil
}

// GetRaw performs a GET request and returns the raw JSON bytes.
// The Robust API wraps responses in {"data": ..., "meta": {...}} envelope;
// this method strips the envelope and returns only the "data" payload.
func (c *Client) GetRaw(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	u := c.baseURL + DefaultBasePath + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("robust: create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("robust: %s %s: %w", c.cluster, path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("robust: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("robust: %s returned %d: %s", path, resp.StatusCode, truncate(body, 200))
	}

	return unwrapEnvelope(body), nil
}

// unwrapEnvelope strips the Robust API {"data": ..., "meta": ...} envelope,
// returning only the "data" payload. If the response doesn't have this
// structure, the original body is returned as-is.
func unwrapEnvelope(body []byte) json.RawMessage {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Data) > 0 {
		return envelope.Data
	}
	return json.RawMessage(body)
}

// Post performs a POST request with a JSON body and decodes the JSON response.
func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.doJSON(ctx, http.MethodPost, path, body, result)
}

// PostRaw performs a POST request with a JSON body and returns raw JSON bytes.
func (c *Client) PostRaw(ctx context.Context, path string, body interface{}) (json.RawMessage, error) {
	return c.doRawJSON(ctx, http.MethodPost, path, body)
}

// Put performs a PUT request with a JSON body and decodes the JSON response.
func (c *Client) Put(ctx context.Context, path string, body interface{}, result interface{}) error {
	return c.doJSON(ctx, http.MethodPut, path, body, result)
}

// PutRaw performs a PUT request with a JSON body and returns raw JSON bytes.
func (c *Client) PutRaw(ctx context.Context, path string, body interface{}) (json.RawMessage, error) {
	return c.doRawJSON(ctx, http.MethodPut, path, body)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body interface{}, result interface{}) error {
	u := c.baseURL + DefaultBasePath + path

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("robust: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return fmt.Errorf("robust: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("robust: %s %s %s: %w", method, c.cluster, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("robust: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		log.Warnf("Robust API error [%s] %s %s -> %d: %s", c.cluster, method, path, resp.StatusCode, truncate(respBody, 200))
		return fmt.Errorf("robust: %s %s returned %d", method, path, resp.StatusCode)
	}

	if result != nil {
		data := unwrapEnvelope(respBody)
		if err := json.Unmarshal(data, result); err != nil {
			return fmt.Errorf("robust: decode response for %s %s: %w", method, path, err)
		}
	}
	return nil
}

func (c *Client) doRawJSON(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	u := c.baseURL + DefaultBasePath + path

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("robust: marshal body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, fmt.Errorf("robust: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("robust: %s %s %s: %w", method, c.cluster, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("robust: read body: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("robust: %s %s returned %d: %s", method, path, resp.StatusCode, truncate(respBody, 200))
	}

	return unwrapEnvelope(respBody), nil
}

func truncate(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
