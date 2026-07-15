/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"time"

	"k8s.io/klog/v2"

	commonerrors "github.com/AMD-AIG-AIMA/SAFE/common/pkg/errors"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/observability"
)

const (
	IndexDateFormat = "2006.01.02"
)

type SearchClientConfig struct {
	DefaultIndex string
}

// SearchClient issues OpenSearch queries for one data cluster. It wraps a
// direct observability.LogsClient (SaFE-native, no robust-analyzer proxy).
type SearchClient struct {
	SearchClientConfig
	logsClient *observability.LogsClient
	// searchFunc, when non-nil, overrides SearchByTimeRange. It is only set by
	// test hooks (see testhook.go) and is always nil in production, so it has
	// no effect on the real request path.
	searchFunc func(sinceTime, untilTime time.Time, index, uri string, body []byte) ([]byte, error)
}

// NewClient builds a SearchClient backed by a direct OpenSearch LogsClient.
func NewClient(cfg SearchClientConfig, lc *observability.LogsClient) *SearchClient {
	return &SearchClient{
		SearchClientConfig: cfg,
		logsClient:         lc,
	}
}

func (c *SearchClient) SearchByTimeRange(sinceTime, untilTime time.Time, index, uri string, body []byte) ([]byte, error) {
	if c.searchFunc != nil {
		return c.searchFunc(sinceTime, untilTime, index, uri, body)
	}
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

// Request issues an OpenSearch HTTP request directly and returns the raw
// response body. uri is the index pattern + path (e.g.
// "node-2026.07.09/_search?..."); it is sent verbatim against the cluster's
// OpenSearch endpoint.
func (c *SearchClient) Request(uri, httpMethod string, body []byte) ([]byte, error) {
	if c.logsClient == nil {
		return nil, commonerrors.NewInternalError("opensearch client not initialized")
	}
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	klog.V(4).Infof("[opensearch] direct request: %s %s", httpMethod, uri)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	return c.logsClient.Request(ctx, httpMethod, uri, body)
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
