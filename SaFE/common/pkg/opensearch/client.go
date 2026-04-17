/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
)

const (
	IndexDateFormat = "2006.01.02"
)

type SearchClientConfig struct {
	DefaultIndex string
}

type SearchClient struct {
	SearchClientConfig
	clusterClient *robustclient.ClusterClient
}

func NewClient(cfg SearchClientConfig, cc *robustclient.ClusterClient) *SearchClient {
	return &SearchClient{
		SearchClientConfig: cfg,
		clusterClient:      cc,
	}
}

type logRawProxyRequest struct {
	URI    string          `json:"uri"`
	Method string          `json:"method"`
	Body   json.RawMessage `json:"body"`
}

type logRawProxyEnvelope struct {
	Meta struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"meta"`
	Data struct {
		StatusCode int             `json:"status_code"`
		Body       json.RawMessage `json:"body"`
	} `json:"data"`
}

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
	return c.Request(indexPattern+uri, "POST", body)
}

func (c *SearchClient) Request(uri, httpMethod string, body []byte) ([]byte, error) {
	if c.clusterClient == nil {
		return nil, commonerrors.NewInternalError("opensearch client not initialized")
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}

	klog.V(4).Infof("proxying opensearch request via robust-analyzer: %s %s", httpMethod, uri)

	proxyReq := logRawProxyRequest{
		URI:    uri,
		Method: httpMethod,
		Body:   json.RawMessage(body),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	rawResp, err := c.clusterClient.RawPost(ctx, "/api/v1/logs/raw", proxyReq)
	if err != nil {
		return nil, fmt.Errorf("robust-analyzer log proxy failed: %w", err)
	}

	var envelope logRawProxyEnvelope
	if err := json.Unmarshal(rawResp, &envelope); err != nil {
		klog.Errorf("[opensearch] failed to parse robust-analyzer response (len=%d): %s",
			len(rawResp), truncateForLog(rawResp, 1024))
		return nil, fmt.Errorf("parse robust-analyzer response: %w (response prefix: %q)",
			err, truncateForLog(rawResp, 200))
	}

	if envelope.Meta.Code != 0 && envelope.Meta.Code != 2000 {
		return nil, fmt.Errorf("robust-analyzer log proxy error %d: %s", envelope.Meta.Code, envelope.Meta.Message)
	}

	if envelope.Data.StatusCode >= 400 {
		return nil, fmt.Errorf("opensearch returned status %d", envelope.Data.StatusCode)
	}

	return []byte(envelope.Data.Body), nil
}

func truncateForLog(b []byte, maxLen int) string {
	if len(b) <= maxLen {
		return string(b)
	}
	return string(b[:maxLen]) + "...(truncated)"
}

func (c *SearchClient) generateIndexPattern(index string, sinceTime, untilTime time.Time) (string, error) {
	if sinceTime.Equal(untilTime) {
		return index + sinceTime.Format(IndexDateFormat), nil
	}

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
