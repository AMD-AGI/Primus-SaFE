/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package httpclient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// client is an HTTP client implementation that wraps the standard http.Client
// with additional functionality like retry logic and simplified request building.
type client struct {
	*http.Client // Embedded standard HTTP client
}

const (
	DefaultTimeout = 30 * time.Second
	DefaultMaxTry  = 2
)

var (
	once     sync.Once
	instance *client
)

// NewClient creates and returns a singleton instance of HttpClient with default timeout.
func NewClient() Interface {
	return NewClientWithTimeout(DefaultTimeout)
}

// NewClientWithTimeout creates and returns a singleton instance of HttpClient with custom timeout.
// It initializes the client only once using sync.Once.
func NewClientWithTimeout(timeout time.Duration) Interface {
	once.Do(func() {
		instance = &client{
			Client: &http.Client{
				Timeout: timeout,
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{
						InsecureSkipVerify: true,
					},
					TLSHandshakeTimeout:   10 * time.Second,
					MaxIdleConns:          128,
					MaxConnsPerHost:       64,
					IdleConnTimeout:       1 * time.Minute,
					ExpectContinueTimeout: 10 * time.Second,
				},
			},
		}
	})
	return instance
}

// GetBaseClient returns the underlying http.Client instance used by this client.
func (c *client) GetBaseClient() *http.Client {
	return c.Client
}

// Get sends an HTTP GET request to the specified URL with optional headers.
// It's a convenience method that calls the do method with GET method.
func (c *client) Get(url string, headers ...string) (*Result, error) {
	return c.do(url, http.MethodGet, nil, headers...)
}

// Post sends an HTTP POST request to the specified URL with a body and optional headers.
// It's a convenience method that calls the do method with POST method.
func (c *client) Post(url string, body interface{}, headers ...string) (*Result, error) {
	return c.do(url, http.MethodPost, body, headers...)
}

// Put sends an HTTP PUT request to the specified URL with a body and optional headers.
// It's a convenience method that calls the do method with PUT method.
func (c *client) Put(url string, body interface{}, headers ...string) (*Result, error) {
	return c.do(url, http.MethodPut, body, headers...)
}

// Delete sends an HTTP DELETE request to the specified URL with optional headers.
// It's a convenience method that calls the do method with DELETE method.
func (c *client) Delete(url string, headers ...string) (*Result, error) {
	return c.do(url, http.MethodDelete, nil, headers...)
}

// do is the internal method that performs HTTP requests for all HTTP methods.
// It builds the request using BuildRequest and executes it using the Do method.
func (c *client) do(url, method string, body interface{}, headers ...string) (*Result, error) {
	req, err := BuildRequest(url, method, body, headers...)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Do executes the HTTP request with retry logic.
func (c *client) Do(req *http.Request) (*Result, error) {
	var rsp *http.Response
	var err error
	for i := 0; i < DefaultMaxTry; i++ {
		if rsp, err = c.Client.Do(req); err == nil {
			break
		} else if i == DefaultMaxTry-1 {
			return nil, err
		}
	}
	if rsp == nil {
		return nil, fmt.Errorf("no result")
	}
	data, err := io.ReadAll(rsp.Body)
	defer rsp.Body.Close()
	if err != nil {
		return nil, err
	}
	return &Result{StatusCode: rsp.StatusCode, Body: data, Header: rsp.Header}, nil
}

// BuildRequest builds and returns the constructed object.
func BuildRequest(url, method string, body interface{}, headers ...string) (*http.Request, error) {
	if !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}
	reader, err := cvtIOReader(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(headers); i += 2 {
		if i+1 >= len(headers) {
			break
		}
		request.Header.Set(headers[i], headers[i+1])
	}
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

// cvtIOReader converts the given body interface{} to an io.Reader.
func cvtIOReader(body interface{}) (io.Reader, error) {
	if body == nil {
		return nil, nil
	}
	var reader io.Reader
	switch body.(type) {
	case string:
		reader = strings.NewReader(body.(string))
	case io.Reader:
		reader = body.(io.Reader)
	case []byte:
		reader = bytes.NewReader(body.([]byte))
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	return reader, nil
}
