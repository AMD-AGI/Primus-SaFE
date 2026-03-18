/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
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
	Username     string
	Password     string
	Endpoint     string
	DefaultIndex string
}

// Equals compares two SearchClientConfig instances for equality.
func (s SearchClientConfig) Equals(other SearchClientConfig) bool {
	return s.Username == other.Username &&
		s.Password == other.Password &&
		s.Endpoint == other.Endpoint &&
		s.DefaultIndex == other.DefaultIndex
}

// Validate validates the input parameters.
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
	return nil
}

type SearchClient struct {
	SearchClientConfig
	httpClient httpclient.Interface
}

// NewClient create or return the singleton instance of SearchClient.
func NewClient(cfg SearchClientConfig) *SearchClient {
	return &SearchClient{
		SearchClientConfig: cfg,
		httpClient:         httpclient.NewClient(),
	}
}

// SearchByTimeRange search openSearch data by time range.
func (c *SearchClient) SearchByTimeRange(sinceTime, untilTime time.Time, index, uri string, body []byte) ([]byte, error) {
	if index == "" {
		index = c.DefaultIndex
	}
	indexPattern, err := c.generateIndexPattern(index, sinceTime, untilTime)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return c.Request(indexPattern+uri, http.MethodPost, body)
}

// Request send HTTP request to OpenSearch.
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

// generateIndexPattern generates the OpenSearch index name prefix based on the provided time range.
// If the start and end times are equal, it returns the index concatenated with the formatted date.
// If the time range exceeds 30 days, it uses a wildcard (*) to cover all indices.
// For smaller ranges, it iterates through each day in the range and appends the formatted date to the index,
// separating multiple indices with commas.
func (c *SearchClient) generateIndexPattern(index string, sinceTime, untilTime time.Time) (string, error) {
	if sinceTime.Equal(untilTime) {
		return index + sinceTime.Format(IndexDateFormat), nil
	}

	// If the time range is too large, use the wildcard * directly
	days := int(untilTime.Sub(sinceTime).Hours() / 24)
	if days >= 30 {
		return index + "*", nil
	}

	sinceTime = sinceTime.Truncate(time.Hour * 24)
	untilTime = untilTime.Truncate(time.Hour * 24)
	result := ""
	currentDate := sinceTime
	for !currentDate.After(untilTime) {
		if result != "" {
			result += ","
		}
		result += index + currentDate.Format(IndexDateFormat)
		currentDate = currentDate.AddDate(0, 0, 1)
	}
	return result, nil
}
