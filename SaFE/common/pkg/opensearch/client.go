/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"bytes"
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
	// When the multi-day path enumerates indexes (e.g. node-2026.04.15,
	// node-2026.04.16,...), OpenSearch returns 404 if any one of them is
	// missing. That happens right after a PVC reset or on a fresh cluster
	// where only today's index exists yet. Telling OpenSearch to ignore
	// missing / unavailable indexes makes the query degrade to "return what
	// exists" instead of failing the whole request.
	sep := "?"
	if strings.Contains(uri, "?") {
		sep = "&"
	}
	uri = uri + sep + "ignore_unavailable=true&allow_no_indices=true"
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

// NormalizeLogResponseMessage rewrites a raw `_search` response so every hit
// exposes a non-empty `message` field even when the source document only has
// the legacy `log` field (clusters whose fluent-bit pipeline did not rename
// `log` -> `message`). The function preserves all other fields and tolerates
// arbitrary unknown keys.
//
// It is intentionally tolerant: malformed input is returned as-is so callers
// can hand the original payload back to the requester for diagnosis instead
// of erroring out.
func NormalizeLogResponseMessage(raw []byte) []byte {
	if len(raw) == 0 {
		return raw
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	hitsRaw, ok := envelope["hits"]
	if !ok {
		return raw
	}
	var hitsLevel map[string]json.RawMessage
	if err := json.Unmarshal(hitsRaw, &hitsLevel); err != nil {
		return raw
	}
	innerRaw, ok := hitsLevel["hits"]
	if !ok {
		return raw
	}
	var hits []map[string]json.RawMessage
	if err := json.Unmarshal(innerRaw, &hits); err != nil {
		return raw
	}

	changed := false
	for i, hit := range hits {
		sourceRaw, ok := hit["_source"]
		if !ok {
			continue
		}
		var source map[string]json.RawMessage
		if err := json.Unmarshal(sourceRaw, &source); err != nil {
			continue
		}
		if !sourceNeedsFallback(source) {
			continue
		}
		logRaw, ok := source["log"]
		if !ok || isEmptyJSONString(logRaw) {
			continue
		}
		source["message"] = logRaw
		patched, err := json.Marshal(source)
		if err != nil {
			continue
		}
		hit["_source"] = patched
		hits[i] = hit
		changed = true
	}
	if !changed {
		return raw
	}

	patchedHits, err := json.Marshal(hits)
	if err != nil {
		return raw
	}
	hitsLevel["hits"] = patchedHits
	patchedHitsLevel, err := json.Marshal(hitsLevel)
	if err != nil {
		return raw
	}
	envelope["hits"] = patchedHitsLevel
	out, err := json.Marshal(envelope)
	if err != nil {
		return raw
	}
	return out
}

func sourceNeedsFallback(source map[string]json.RawMessage) bool {
	msgRaw, ok := source["message"]
	if !ok {
		return true
	}
	return isEmptyJSONString(msgRaw)
}

func isEmptyJSONString(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return true
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return true
	}
	if bytes.Equal(trimmed, []byte(`""`)) {
		return true
	}
	return false
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
