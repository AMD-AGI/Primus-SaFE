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
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

type client struct {
	*http.Client
}

const (
	DefaultTimeout = 30 * time.Second
	DefaultMaxTry  = 2
)

var (
	once     sync.Once
	instance *client
)

func NewHttpClient() Interface {
	once.Do(func() {
		instance = &client{
			Client: &http.Client{
				Timeout: DefaultTimeout,
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

func (c *client) Get(url string, headers ...string) (*Result, error) {
	return c.do(url, http.MethodGet, nil, headers...)
}

func (c *client) Post(url string, body interface{}, headers ...string) (*Result, error) {
	return c.do(url, http.MethodPost, body, headers...)
}

func (c *client) Put(url string, body interface{}, headers ...string) (*Result, error) {
	return c.do(url, http.MethodPut, body, headers...)
}

func (c *client) Delete(url string, headers ...string) (*Result, error) {
	return c.do(url, http.MethodDelete, nil, headers...)
}

func (c *client) do(url, method string, body interface{}, headers ...string) (*Result, error) {
	req, err := BuildRequest(url, method, body, headers...)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

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
	data, err := ioutil.ReadAll(rsp.Body)
	defer rsp.Body.Close()
	if err != nil {
		return nil, err
	}
	return &Result{StatusCode: rsp.StatusCode, Body: data, Header: rsp.Header}, nil
}

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
