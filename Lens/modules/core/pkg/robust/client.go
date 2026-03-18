// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package robust provides a pure HTTP client for communicating with the
// Primus-Robust API. It has no Go module dependency on the Robust codebase —
// all communication is via JSON over HTTP.
package robust

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"k8s.io/klog/v2"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	retryMax   int
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		retryMax: 2,
	}
}

func (c *Client) HealthCheck(ctx context.Context) error {
	_, err := c.doRaw(ctx, "GET", "/healthz", nil, nil)
	return err
}

func (c *Client) Get(ctx context.Context, path string, params url.Values, result interface{}) error {
	body, err := c.doRaw(ctx, "GET", path, params, nil)
	if err != nil {
		return err
	}
	defer body.Close()
	return json.NewDecoder(body).Decode(result)
}

func (c *Client) GetStream(ctx context.Context, path string, params url.Values) (io.ReadCloser, int64, string, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, 0, "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, "", err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, 0, "", fmt.Errorf("robust api: %d %s", resp.StatusCode, path)
	}
	return resp.Body, resp.ContentLength, resp.Header.Get("Content-Type"), nil
}

func (c *Client) Post(ctx context.Context, path string, body interface{}, result interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = io.NopCloser(
			&bytesReader{data: data, pos: 0},
		)
	}
	respBody, err := c.doRaw(ctx, "POST", path, nil, bodyReader)
	if err != nil {
		return err
	}
	defer respBody.Close()
	if result != nil {
		return json.NewDecoder(respBody).Decode(result)
	}
	return nil
}

func (c *Client) doRaw(ctx context.Context, method, path string, params url.Values, body io.Reader) (io.ReadCloser, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	var lastErr error
	for attempt := 0; attempt <= c.retryMax; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < c.retryMax {
				klog.V(2).Infof("[robust-client] %s %s attempt %d failed: %v", method, path, attempt+1, err)
				time.Sleep(time.Duration(attempt+1) * 200 * time.Millisecond)
			}
			continue
		}

		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("robust api: %d %s", resp.StatusCode, path)
			if attempt < c.retryMax {
				klog.V(2).Infof("[robust-client] %s %s attempt %d: %d", method, path, attempt+1, resp.StatusCode)
				time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
			}
			continue
		}

		if resp.StatusCode >= 400 {
			resp.Body.Close()
			return nil, fmt.Errorf("robust api: %d %s", resp.StatusCode, path)
		}

		return resp.Body, nil
	}
	return nil, lastErr
}

type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
