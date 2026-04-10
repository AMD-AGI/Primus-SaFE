/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

const defaultLensAPIBaseURL = "http://primus-lens-api.primus-lens.svc.cluster.local:8989/api/v1"

var preferredLossMetricNames = []string{
	"loss",
	"LmLoss",
	"lm_loss",
	"train_loss",
	"train/loss",
	"actor_loss",
	"actor/loss",
}

type lensAvailableMetricsResponse struct {
	WorkloadUID string                   `json:"workload_uid"`
	Metrics     []lensAvailableMetricRow `json:"metrics"`
	TotalCount  int                      `json:"total_count"`
}

type lensAvailableMetricRow struct {
	Name       string   `json:"name"`
	DataSource []string `json:"data_source"`
	Count      int      `json:"count"`
}

type lensMetricsDataResponse struct {
	WorkloadUID string                `json:"workload_uid"`
	DataSource  string                `json:"data_source,omitempty"`
	Data        []lensMetricDataPoint `json:"data"`
	TotalCount  int                   `json:"total_count"`
}

type lensMetricDataPoint struct {
	MetricName string  `json:"metric_name"`
	Value      float64 `json:"value"`
	Timestamp  int64   `json:"timestamp"`
	Iteration  int32   `json:"iteration"`
	DataSource string  `json:"data_source,omitempty"`
}

type lossSummary struct {
	Value      float64
	MetricName string
	DataSource string
}

func getLensAPIBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("POSTTRAIN_LENS_API_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultLensAPIBaseURL
}

func fetchLensAvailableMetrics(ctx context.Context, workloadUID, cluster string) ([]string, error) {
	var resp lensAvailableMetricsResponse
	params := url.Values{}
	if cluster != "" {
		params.Set("cluster", cluster)
	}
	if err := callLensAPI(ctx, fmt.Sprintf("/workloads/%s/metrics/available", url.PathEscape(workloadUID)), params, &resp); err != nil {
		return nil, err
	}
	metrics := make([]string, 0, len(resp.Metrics))
	for _, item := range resp.Metrics {
		if item.Name == "" {
			continue
		}
		metrics = append(metrics, item.Name)
	}
	sort.Strings(metrics)
	return metrics, nil
}

func fetchLensMetricData(ctx context.Context, workloadUID, cluster, metrics, dataSource, start, end string) ([]PosttrainMetricPoint, error) {
	var resp lensMetricsDataResponse
	params := url.Values{}
	if cluster != "" {
		params.Set("cluster", cluster)
	}
	if metrics != "" {
		params.Set("metrics", metrics)
	}
	if dataSource != "" {
		params.Set("data_source", dataSource)
	}
	if start != "" && end != "" {
		params.Set("start", start)
		params.Set("end", end)
	}
	if err := callLensAPI(ctx, fmt.Sprintf("/workloads/%s/metrics/data", url.PathEscape(workloadUID)), params, &resp); err != nil {
		return nil, err
	}
	points := make([]PosttrainMetricPoint, 0, len(resp.Data))
	for _, point := range resp.Data {
		points = append(points, PosttrainMetricPoint{
			MetricName: point.MetricName,
			Value:      point.Value,
			Timestamp:  point.Timestamp,
			Iteration:  point.Iteration,
			DataSource: point.DataSource,
		})
	}
	return points, nil
}

func fetchLatestLossSummary(ctx context.Context, workloadUID, cluster string) (*lossSummary, []string, error) {
	availableMetrics, err := fetchLensAvailableMetrics(ctx, workloadUID, cluster)
	if err != nil {
		return nil, nil, err
	}
	lossMetric := pickLossMetricName(availableMetrics)
	if lossMetric == "" {
		return nil, availableMetrics, nil
	}
	points, err := fetchLensMetricData(ctx, workloadUID, cluster, lossMetric, "", "", "")
	if err != nil {
		return nil, availableMetrics, err
	}
	if len(points) == 0 {
		return nil, availableMetrics, nil
	}
	sort.Slice(points, func(i, j int) bool {
		if points[i].Timestamp == points[j].Timestamp {
			return points[i].Iteration < points[j].Iteration
		}
		return points[i].Timestamp < points[j].Timestamp
	})
	latest := points[len(points)-1]
	return &lossSummary{
		Value:      latest.Value,
		MetricName: latest.MetricName,
		DataSource: latest.DataSource,
	}, availableMetrics, nil
}

func pickLossMetricName(metrics []string) string {
	if len(metrics) == 0 {
		return ""
	}
	normalized := make(map[string]string, len(metrics))
	for _, metric := range metrics {
		normalized[normalizeMetricName(metric)] = metric
	}
	for _, candidate := range preferredLossMetricNames {
		if metric, ok := normalized[normalizeMetricName(candidate)]; ok {
			return metric
		}
	}
	for _, metric := range metrics {
		name := normalizeMetricName(metric)
		if strings.Contains(name, "loss") && !strings.Contains(name, "lossscale") {
			return metric
		}
	}
	return ""
}

func normalizeMetricName(metric string) string {
	metric = strings.ToLower(metric)
	metric = strings.ReplaceAll(metric, "/", "")
	metric = strings.ReplaceAll(metric, "_", "")
	metric = strings.ReplaceAll(metric, "-", "")
	metric = strings.ReplaceAll(metric, " ", "")
	return metric
}

func callLensAPI(ctx context.Context, apiPath string, params url.Values, out interface{}) error {
	baseURL := getLensAPIBaseURL()
	reqURL := baseURL + apiPath
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lens api %s returned status %d", apiPath, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
