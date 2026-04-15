/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

const defaultLensAPIBaseURL = "http://primus-lens-api.primus-lens.svc.cluster.local:8989/v1"

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

type lensResponseMeta struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type lensResponseEnvelope struct {
	Meta    lensResponseMeta `json:"meta"`
	Data    json.RawMessage  `json:"data"`
	Tracing interface{}      `json:"tracing,omitempty"`
}

type lensWorkloadListResponse struct {
	Data  []lensWorkloadListItem `json:"data"`
	Total int                    `json:"total"`
}

type lensWorkloadListItem struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
	Status    string `json:"status"`
	StartAt   int64  `json:"start_at"`
	EndAt     int64  `json:"end_at"`
}

type lensTrainingPerformancePoint struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
}

func getLensAPIBaseURL() string {
	if v := strings.TrimSpace(os.Getenv("POSTTRAIN_LENS_API_URL")); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultLensAPIBaseURL
}

func resolveLensWorkloadUID(ctx context.Context, workloadID, workspace, cluster, trainType string) (string, error) {
	workloadID = strings.TrimSpace(workloadID)
	if workloadID == "" {
		return "", fmt.Errorf("workload id is empty")
	}

	params := url.Values{}
	if cluster != "" {
		params.Set("cluster", cluster)
	}
	params.Set("name", workloadID)
	params.Set("namespace", workspace)
	params.Set("page_num", "1")
	params.Set("page_size", "20")
	kind := lensWorkloadKind(trainType)
	if kind != "" {
		params.Set("kind", kind)
	}

	var resp lensWorkloadListResponse
	if err := callLensAPI(ctx, "/workloads", params, &resp); err != nil {
		return "", err
	}
	if uid := pickLensWorkloadUID(resp.Data, workloadID, workspace, kind); uid != "" {
		return uid, nil
	}

	if kind != "" {
		params.Del("kind")
		resp = lensWorkloadListResponse{}
		if err := callLensAPI(ctx, "/workloads", params, &resp); err != nil {
			return "", err
		}
		if uid := pickLensWorkloadUID(resp.Data, workloadID, workspace, ""); uid != "" {
			return uid, nil
		}
	}

	return "", fmt.Errorf("lens workload uid not found for %s/%s", workspace, workloadID)
}

func lensWorkloadKind(trainType string) string {
	switch strings.ToLower(strings.TrimSpace(trainType)) {
	case "rl":
		return common.RayJobKind
	case "sft":
		return common.PytorchJobKind
	default:
		return ""
	}
}

func pickLensWorkloadUID(items []lensWorkloadListItem, workloadID, workspace, preferredKind string) string {
	for _, item := range items {
		if item.Name == workloadID && item.Namespace == workspace && (preferredKind == "" || item.Kind == preferredKind) {
			return item.UID
		}
	}
	for _, item := range items {
		if item.Name == workloadID && item.Namespace == workspace {
			return item.UID
		}
	}
	return ""
}

func defaultLensTimeRange(startTime, endTime, createdAt time.Time) (string, string, error) {
	start := startTime
	if start.IsZero() {
		start = createdAt
	}
	if start.IsZero() {
		return "", "", fmt.Errorf("run start time is not available")
	}

	end := endTime
	if end.IsZero() {
		end = time.Now().UTC()
	}
	if end.Before(start) {
		end = start
	}

	return strconv.FormatInt(start.UnixMilli(), 10), strconv.FormatInt(end.UnixMilli(), 10), nil
}

func defaultLensTimeRangeForRun(run *dbclient.PosttrainRunView) (string, string, error) {
	if run == nil {
		return "", "", fmt.Errorf("run is nil")
	}
	var start, end, created time.Time
	if run.StartTime.Valid {
		start = run.StartTime.Time
	}
	if run.EndTime.Valid {
		end = run.EndTime.Time
	}
	if run.CreatedAt.Valid {
		created = run.CreatedAt.Time
	}
	return defaultLensTimeRange(start, end, created)
}

func defaultLensTimeRangeForItem(item PosttrainRunItem) (string, string, error) {
	var start, end, created time.Time
	var err error
	if item.StartTime != "" {
		start, err = timeutil.CvtStrToRFC3339Milli(item.StartTime)
		if err != nil {
			start = time.Time{}
		}
	}
	if item.EndTime != "" {
		end, err = timeutil.CvtStrToRFC3339Milli(item.EndTime)
		if err != nil {
			end = time.Time{}
		}
	}
	if item.CreatedAt != "" {
		created, err = timeutil.CvtStrToRFC3339Milli(item.CreatedAt)
		if err != nil {
			created = time.Time{}
		}
	}
	return defaultLensTimeRange(start, end, created)
}

func fetchLensTrainingPerformanceData(ctx context.Context, lensWorkloadUID, start, end, metricsExpr, dataSource string) ([]PosttrainMetricPoint, []string, string, error) {
	params := url.Values{}
	if start == "" || end == "" {
		return nil, nil, "", fmt.Errorf("start and end timestamps are required")
	}

	availableRows, err := fetchLensAvailableMetrics(ctx, lensWorkloadUID, dataSource)
	if err != nil {
		availableRows = nil
	}
	selectedSource := chooseLensMetricDataSource(availableRows, dataSource)

	params.Set("start", start)
	params.Set("end", end)
	if metricsExpr != "" {
		params.Set("metrics", metricsExpr)
	}
	if selectedSource != "" {
		params.Set("data_source", selectedSource)
	}

	var resp lensMetricsDataResponse
	if err := callLensAPI(ctx, fmt.Sprintf("/workloads/%s/metrics/data", url.PathEscape(lensWorkloadUID)), params, &resp); err != nil {
		return nil, nil, "", err
	}
	if selectedSource == "" {
		selectedSource = strings.TrimSpace(resp.DataSource)
	}

	points := make([]PosttrainMetricPoint, 0, len(resp.Data))
	for _, point := range resp.Data {
		if point.MetricName == "" {
			continue
		}
		points = append(points, PosttrainMetricPoint{
			MetricName: point.MetricName,
			Value:      point.Value,
			Timestamp:  point.Timestamp,
			Iteration:  point.Iteration,
			DataSource: point.DataSource,
		})
	}
	sort.SliceStable(points, func(i, j int) bool {
		if points[i].Timestamp == points[j].Timestamp {
			return points[i].Iteration < points[j].Iteration
		}
		return points[i].Timestamp < points[j].Timestamp
	})

	metrics := availableMetricNamesForSource(availableRows, selectedSource)
	if len(metrics) == 0 {
		metricSet := make(map[string]struct{}, len(points))
		for _, point := range points {
			metricSet[point.MetricName] = struct{}{}
		}
		metrics = make([]string, 0, len(metricSet))
		for metric := range metricSet {
			metrics = append(metrics, metric)
		}
		sort.Strings(metrics)
	}
	if selectedSource == "" {
		selectedSource = inferMetricDataSourceFromPoints(points)
	}
	return points, metrics, selectedSource, nil
}

func fetchLensAvailableMetrics(ctx context.Context, lensWorkloadUID, dataSource string) ([]lensAvailableMetricRow, error) {
	params := url.Values{}
	if strings.TrimSpace(dataSource) != "" {
		params.Set("data_source", strings.TrimSpace(dataSource))
	}
	var resp lensAvailableMetricsResponse
	if err := callLensAPI(ctx, fmt.Sprintf("/workloads/%s/metrics/available", url.PathEscape(lensWorkloadUID)), params, &resp); err != nil {
		return nil, err
	}
	return resp.Metrics, nil
}

func chooseLensMetricDataSource(rows []lensAvailableMetricRow, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		return requested
	}

	lossMetric := pickLossMetricName(metricNamesFromAvailableRows(rows))
	if lossMetric != "" {
		for _, row := range rows {
			if row.Name == lossMetric {
				if source := preferredMetricDataSource(row.DataSource); source != "" {
					return source
				}
			}
		}
	}

	allSources := make([]string, 0)
	seen := make(map[string]struct{})
	for _, row := range rows {
		for _, source := range row.DataSource {
			source = strings.TrimSpace(source)
			if source == "" {
				continue
			}
			if _, ok := seen[source]; ok {
				continue
			}
			seen[source] = struct{}{}
			allSources = append(allSources, source)
		}
	}
	return preferredMetricDataSource(allSources)
}

func preferredMetricDataSource(sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	preferredOrder := []string{"log", "wandb", "tensorflow"}
	normalized := make(map[string]string, len(sources))
	for _, source := range sources {
		trimmed := strings.TrimSpace(source)
		if trimmed == "" {
			continue
		}
		normalized[strings.ToLower(trimmed)] = trimmed
	}
	for _, candidate := range preferredOrder {
		if source, ok := normalized[candidate]; ok {
			return source
		}
	}
	for _, source := range sources {
		if trimmed := strings.TrimSpace(source); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func availableMetricNamesForSource(rows []lensAvailableMetricRow, dataSource string) []string {
	metricSet := make(map[string]struct{})
	for _, row := range rows {
		if strings.TrimSpace(row.Name) == "" {
			continue
		}
		if dataSource == "" || metricRowHasDataSource(row, dataSource) {
			metricSet[row.Name] = struct{}{}
		}
	}
	metrics := make([]string, 0, len(metricSet))
	for metric := range metricSet {
		metrics = append(metrics, metric)
	}
	sort.Strings(metrics)
	return metrics
}

func metricNamesFromAvailableRows(rows []lensAvailableMetricRow) []string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.TrimSpace(row.Name) != "" {
			names = append(names, row.Name)
		}
	}
	return names
}

func metricRowHasDataSource(row lensAvailableMetricRow, dataSource string) bool {
	if dataSource == "" {
		return true
	}
	for _, source := range row.DataSource {
		if strings.EqualFold(strings.TrimSpace(source), strings.TrimSpace(dataSource)) {
			return true
		}
	}
	return false
}

func inferMetricDataSourceFromPoints(points []PosttrainMetricPoint) string {
	for _, point := range points {
		if strings.TrimSpace(point.DataSource) != "" {
			return point.DataSource
		}
	}
	return ""
}

func filterLensTrainingPerformancePoints(points []PosttrainMetricPoint, metricsExpr string) []PosttrainMetricPoint {
	metricsExpr = strings.TrimSpace(metricsExpr)
	if metricsExpr == "" || strings.EqualFold(metricsExpr, "all") {
		return points
	}

	if strings.HasPrefix(metricsExpr, "{") && strings.HasSuffix(metricsExpr, "}") {
		metricsExpr = metricsExpr[1 : len(metricsExpr)-1]
	}
	filterSet := make(map[string]struct{})
	for _, metric := range strings.Split(metricsExpr, ",") {
		metric = strings.TrimSpace(metric)
		if metric == "" {
			continue
		}
		filterSet[metric] = struct{}{}
	}

	filtered := make([]PosttrainMetricPoint, 0, len(points))
	for _, point := range points {
		if _, ok := filterSet[point.MetricName]; ok {
			filtered = append(filtered, point)
		}
	}
	return filtered
}

func buildPosttrainMetricSeries(points []PosttrainMetricPoint, lossMetric string) map[string][]PosttrainMetricSeriesPoint {
	if len(points) == 0 {
		return nil
	}
	series := make(map[string][]PosttrainMetricSeriesPoint)
	for _, point := range points {
		series[point.MetricName] = append(series[point.MetricName], PosttrainMetricSeriesPoint{
			Step:      point.Iteration,
			Value:     point.Value,
			Timestamp: formatMetricTimestamp(point.Timestamp),
		})
	}
	if lossMetric != "" {
		if lossSeries, ok := series[lossMetric]; ok {
			if _, exists := series["loss"]; !exists {
				series["loss"] = append([]PosttrainMetricSeriesPoint(nil), lossSeries...)
			}
		}
	}
	return series
}

func formatMetricTimestamp(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.UnixMilli(ts).UTC().Format(time.RFC3339)
}

func latestLossSummaryFromPoints(points []PosttrainMetricPoint, metrics []string) *lossSummary {
	lossMetric := pickLossMetricName(metrics)
	if lossMetric == "" {
		return nil
	}

	var latest *PosttrainMetricPoint
	for i := range points {
		point := points[i]
		if point.MetricName != lossMetric {
			continue
		}
		if latest == nil || point.Timestamp > latest.Timestamp || (point.Timestamp == latest.Timestamp && point.Iteration > latest.Iteration) {
			latest = &point
		}
	}
	if latest == nil {
		return nil
	}

	return &lossSummary{
		Value:      latest.Value,
		MetricName: latest.MetricName,
		DataSource: latest.DataSource,
	}
}

func fetchLatestLossSummary(ctx context.Context, lensWorkloadUID, start, end string) (*lossSummary, []string, error) {
	points, metrics, _, err := fetchLensTrainingPerformanceData(ctx, lensWorkloadUID, start, end, "", "")
	if err != nil {
		return nil, nil, err
	}
	return latestLossSummaryFromPoints(points, metrics), metrics, nil
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}

	var envelope lensResponseEnvelope
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Meta.Code != 0 || envelope.Data != nil) {
		if envelope.Meta.Code != 0 && envelope.Meta.Code != 2000 {
			message := strings.TrimSpace(envelope.Meta.Message)
			if message == "" {
				message = "unknown lens error"
			}
			return fmt.Errorf("lens api %s returned code %d: %s", apiPath, envelope.Meta.Code, message)
		}
		if out == nil || len(bytes.TrimSpace(envelope.Data)) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
			return nil
		}
		return json.Unmarshal(envelope.Data, out)
	}

	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}
