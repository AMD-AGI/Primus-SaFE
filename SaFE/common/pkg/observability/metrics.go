/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

// Package observability provides SaFE-native clients for the metrics and logs
// backends (VictoriaMetrics / Prometheus and OpenSearch) so the management
// plane can query observability data directly, without proxying through the
// data-plane primus-robust `robust-analyzer` service.
//
// The metrics client speaks the Prometheus HTTP query API
// (`/api/v1/query`, `/api/v1/query_range`), which VictoriaMetrics' vmselect
// implements verbatim. It intentionally mirrors the small set of query
// helpers that robust-api used internally (instant scalar, instant vector,
// range matrix) so callers that previously fanned out to robust-api can be
// repointed at a cluster's own vmselect with minimal churn.
package observability

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"k8s.io/klog/v2"
)

const (
	// defaultMetricsTimeout bounds a single PromQL request. VictoriaMetrics
	// range queries over long windows can be slow, so keep this generous but
	// finite to avoid hanging goroutines in the sync loops.
	defaultMetricsTimeout = 60 * time.Second
)

// MetricsClientConfig configures a per-endpoint metrics client.
type MetricsClientConfig struct {
	// BaseURL is the Prometheus-compatible query root. For VictoriaMetrics
	// this is the vmselect prometheus prefix, e.g.
	// "http://vmselect-primus-safe-vmcluster.primus-safe.svc:8481/select/0/prometheus".
	// For single-node Prometheus it is just the server root, e.g.
	// "http://prometheus.primus-safe.svc:9090".
	BaseURL string
	// Timeout bounds a single HTTP request. Zero uses defaultMetricsTimeout.
	Timeout time.Duration
	// InsecureSkipVerify disables TLS verification, needed when the backend
	// serves HTTPS with a cluster-internal self-signed CA.
	InsecureSkipVerify bool
}

// MetricsClient issues PromQL queries against a single Prometheus-compatible
// endpoint (typically one data cluster's vmselect).
type MetricsClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewMetricsClient builds a metrics client for one backend endpoint.
func NewMetricsClient(cfg MetricsClientConfig) *MetricsClient {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultMetricsTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		}
	}
	return &MetricsClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: &http.Client{Timeout: timeout, Transport: transport},
	}
}

// BaseURL returns the configured query root.
func (c *MetricsClient) BaseURL() string { return c.baseURL }

// Sample is a single (labels, value, timestamp) point from an instant query.
type Sample struct {
	Metric    map[string]string
	Value     float64
	Timestamp time.Time
}

// SeriesPoint is one (timestamp, value) pair inside a range-query series.
type SeriesPoint struct {
	Timestamp time.Time
	Value     float64
}

// Series is a labelled sequence of points from a range query.
type Series struct {
	Metric map[string]string
	Points []SeriesPoint
}

// promResponse mirrors the Prometheus HTTP API envelope. The Result field is
// left raw because its shape depends on resultType (vector vs matrix vs
// scalar), which we decode per query kind.
type promResponse struct {
	Status    string           `json:"status"`
	ErrorType string           `json:"errorType"`
	Error     string           `json:"error"`
	Data      promResponseData `json:"data"`
}

type promResponseData struct {
	ResultType string          `json:"resultType"`
	Result     json.RawMessage `json:"result"`
}

// QueryInstant runs an instant PromQL query and returns every sample in the
// resulting vector. A scalar result is normalized into a single sample with
// no labels.
func (c *MetricsClient) QueryInstant(ctx context.Context, query string) ([]Sample, error) {
	params := url.Values{}
	params.Set("query", query)
	body, err := c.do(ctx, "/api/v1/query", params)
	if err != nil {
		return nil, err
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode instant query response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("promql instant query failed: %s: %s", resp.ErrorType, resp.Error)
	}

	switch resp.Data.ResultType {
	case "vector":
		return parseVector(resp.Data.Result)
	case "scalar":
		s, err := parseScalar(resp.Data.Result)
		if err != nil {
			return nil, err
		}
		return []Sample{s}, nil
	default:
		return nil, fmt.Errorf("unexpected instant result type %q", resp.Data.ResultType)
	}
}

// QueryInstantScalar runs an instant query and returns the first sample value.
// This mirrors the helper robust-api used for single-number panels (avg/max/
// sum aggregations). Returns an error when the query yields no data.
func (c *MetricsClient) QueryInstantScalar(ctx context.Context, query string) (float64, error) {
	samples, err := c.QueryInstant(ctx, query)
	if err != nil {
		return 0, err
	}
	if len(samples) == 0 {
		return 0, fmt.Errorf("promql query returned no data: %s", query)
	}
	return samples[0].Value, nil
}

// QueryRange runs a range PromQL query and returns one Series per label set.
func (c *MetricsClient) QueryRange(ctx context.Context, query string, start, end time.Time, step time.Duration) ([]Series, error) {
	if step <= 0 {
		step = 15 * time.Second
	}
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", formatUnix(start))
	params.Set("end", formatUnix(end))
	params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))

	body, err := c.do(ctx, "/api/v1/query_range", params)
	if err != nil {
		return nil, err
	}

	var resp promResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode range query response: %w", err)
	}
	if resp.Status != "success" {
		return nil, fmt.Errorf("promql range query failed: %s: %s", resp.ErrorType, resp.Error)
	}
	if resp.Data.ResultType != "matrix" {
		return nil, fmt.Errorf("unexpected range result type %q", resp.Data.ResultType)
	}
	return parseMatrix(resp.Data.Result)
}

func (c *MetricsClient) do(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("metrics client base URL not configured")
	}
	reqURL := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create metrics request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metrics request to %s: %w", c.baseURL+path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read metrics response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		klog.V(4).Infof("[observability] metrics backend %s returned %d", c.baseURL+path, resp.StatusCode)
		return nil, fmt.Errorf("metrics backend returned HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return body, nil
}

// ── Prometheus result decoding ──────────────────────────────────────────

// vectorEntry is one element of a Prometheus "vector" result:
//
//	{"metric":{...}, "value":[<unix_ts>, "<value>"]}
type vectorEntry struct {
	Metric map[string]string `json:"metric"`
	Value  [2]interface{}    `json:"value"`
}

// matrixEntry is one element of a Prometheus "matrix" result:
//
//	{"metric":{...}, "values":[[<ts>, "<value>"], ...]}
type matrixEntry struct {
	Metric map[string]string `json:"metric"`
	Values [][2]interface{}  `json:"values"`
}

func parseVector(raw json.RawMessage) ([]Sample, error) {
	var entries []vectorEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode vector result: %w", err)
	}
	samples := make([]Sample, 0, len(entries))
	for _, e := range entries {
		ts, val, err := parseSampleTuple(e.Value)
		if err != nil {
			continue
		}
		samples = append(samples, Sample{Metric: e.Metric, Value: val, Timestamp: ts})
	}
	return samples, nil
}

func parseScalar(raw json.RawMessage) (Sample, error) {
	var tuple [2]interface{}
	if err := json.Unmarshal(raw, &tuple); err != nil {
		return Sample{}, fmt.Errorf("decode scalar result: %w", err)
	}
	ts, val, err := parseSampleTuple(tuple)
	if err != nil {
		return Sample{}, err
	}
	return Sample{Value: val, Timestamp: ts}, nil
}

func parseMatrix(raw json.RawMessage) ([]Series, error) {
	var entries []matrixEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, fmt.Errorf("decode matrix result: %w", err)
	}
	series := make([]Series, 0, len(entries))
	for _, e := range entries {
		points := make([]SeriesPoint, 0, len(e.Values))
		for _, v := range e.Values {
			ts, val, err := parseSampleTuple(v)
			if err != nil {
				continue
			}
			points = append(points, SeriesPoint{Timestamp: ts, Value: val})
		}
		series = append(series, Series{Metric: e.Metric, Points: points})
	}
	return series, nil
}

// parseSampleTuple decodes a Prometheus [<unix_seconds>, "<value>"] pair.
// The timestamp is a JSON number (float seconds) and the value is a string.
func parseSampleTuple(tuple [2]interface{}) (time.Time, float64, error) {
	tsFloat, ok := tuple[0].(float64)
	if !ok {
		return time.Time{}, 0, fmt.Errorf("unexpected timestamp type %T", tuple[0])
	}
	valStr, ok := tuple[1].(string)
	if !ok {
		return time.Time{}, 0, fmt.Errorf("unexpected value type %T", tuple[1])
	}
	val, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		// NaN/Inf are represented as "NaN"/"+Inf"; treat as missing.
		return time.Time{}, 0, fmt.Errorf("parse sample value %q: %w", valStr, err)
	}
	sec := int64(tsFloat)
	nsec := int64((tsFloat - float64(sec)) * 1e9)
	return time.Unix(sec, nsec).UTC(), val, nil
}

func formatUnix(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixMilli())/1000.0, 'f', 3, 64)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
