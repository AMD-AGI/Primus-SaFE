/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/httpclient"
)

const (
	IndexDateFormat = "2006.01.02"
)

type SearchClientConfig struct {
	Username string
	Password string
	Endpoint string
	Prefix   string
}

func (s SearchClientConfig) Equals(other SearchClientConfig) bool {
	return s.Username == other.Username &&
		s.Password == other.Password &&
		s.Endpoint == other.Endpoint &&
		s.Prefix == other.Prefix
}

func (s SearchClientConfig) Validate() error {
	if s.Endpoint == "" {
		return fmt.Errorf("opensearch endpoint is empty")
	}
	if s.Username == "" {
		return fmt.Errorf("opensearch username is empty")
	}
	if s.Password == "" {
		return fmt.Errorf("opensearch password is empty")
	}
	if s.Prefix == "" {
		return fmt.Errorf("opensearch index prefix is empty")
	}
	return nil
}

type SearchClient struct {
	SearchClientConfig
	httpClient httpclient.Interface
}

// NewClient() *SearchClient
// Create or return the singleton instance of SearchClient
// Gets OpenSearch endpoint, index prefix, username and password from configuration
// Initializes HTTP client
// Returns: SearchClient instance
func NewClient(cfg SearchClientConfig) *SearchClient {
	return &SearchClient{
		SearchClientConfig: cfg,
		httpClient:         httpclient.NewHttpClient(),
	}
}

// Search OpenSearch data by time range
// Parameters:
//
//	sinceTime: Start time
//	untilTime: End time
//	uri: the endpoint of opensearch service
//	body: Request body
//
// Returns:
//
//	[]byte: Response data
//	error: Error information
//
// Function: Builds index names within time range, then sends POST request
func (c *SearchClient) SearchByTimeRange(sinceTime, untilTime time.Time, uri string, body []byte) ([]byte, error) {
	index, err := c.generateQueryIndex(sinceTime, untilTime)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return c.Request(index+uri, http.MethodPost, body)
}

// Send HTTP request to OpenSearch
// Parameters:
//
//	uri: Full API path
//	httpMethod: HTTP method (such as GET, POST, etc.)
//	body: Request body data
//
// Returns:
//
//	[]byte: Response body data
//	error: Error information
//
// Function: Builds HTTP request with authentication, sends request and processes response
func (c *SearchClient) Request(uri, httpMethod string, body []byte) ([]byte, error) {
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	url := c.Endpoint + uri
	klog.Infof("request to openSearch, url: %s, body: %s", url, body)
	req, err := httpclient.BuildRequest(url, httpMethod, body)
	if err != nil {
		return nil, commonerrors.NewBadRequest(err.Error())
	}
	req.SetBasicAuth(c.Username, c.Password)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if !resp.IsSuccess() {
		return nil, fmt.Errorf("failed to request openSearch: %s", string(resp.Body))
	}
	return resp.Body, nil
}

// Generate OpenSearch index name based on time range
// Parameters:
//
//	sinceTime: Start time
//	untilTime: End time
//
// Returns:
//
//	string: Index name (may contain multiple indices or wildcard)
//	error: Error information
//
// Logic:
//  1. If start time equals end time, return single date index
//  2. If time range exceeds 30 days, use wildcard *
//  3. Otherwise generate all index names within date range, separated by comma
func (c *SearchClient) generateQueryIndex(sinceTime, untilTime time.Time) (string, error) {
	if sinceTime.Equal(untilTime) {
		return c.Prefix + sinceTime.Format(IndexDateFormat), nil
	}

	// If the time range is too large, use the wildcard * directly
	days := int(untilTime.Sub(sinceTime).Hours() / 24)
	if days >= 30 {
		return c.Prefix + "*", nil
	}

	sinceTime = sinceTime.Truncate(time.Hour * 24)
	untilTime = untilTime.Truncate(time.Hour * 24)
	result := ""
	currentDate := sinceTime
	for !currentDate.After(untilTime) {
		if result != "" {
			result += ","
		}
		result += c.Prefix + currentDate.Format(IndexDateFormat)
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	return result, nil
}
