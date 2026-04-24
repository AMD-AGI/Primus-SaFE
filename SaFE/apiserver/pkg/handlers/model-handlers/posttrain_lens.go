/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
)

// Default robust-api path prefix used by Lens-compat training-performance queries.
// The actual host is resolved per-cluster through robustclient (auto-registered
// via Cluster CR annotation primus-safe.amd.com/robust-api-endpoint).
const (
	robustAPIPathWorkloads        = "/api/v1/workloads"
	robustAPIPathTrainingAvail    = "/api/v1/training/%s/available"
	robustAPIPathTrainingDataRoot = "/api/v1/training/%s/data"
)

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

// robustClientForCluster returns the robust-analyzer client for the given
// data-plane cluster. cluster must be non-empty and registered in the
// robustclient discovery (via Cluster CR annotation). Returns a typed error
// when either the client or the cluster endpoint is missing so upstream
// handlers can translate this into a 503 / clear user-facing message.
func robustClientForCluster(cluster string) (*robustclient.ClusterClient, error) {
	rc := GetRobustClient()
	if rc == nil {
		return nil, fmt.Errorf("robust-analyzer client is not configured; " +
			"ensure apiserver is started with robustclient discovery and the " +
			"target cluster has a registered robust-api endpoint")
	}
	cluster = strings.TrimSpace(cluster)
	if cluster == "" {
		return nil, fmt.Errorf("cluster is required to resolve workload metrics")
	}
	cc := rc.ForCluster(cluster)
	if cc == nil {
		return nil, fmt.Errorf("no robust-api endpoint registered for cluster %q; "+
			"check Cluster CR annotation primus-safe.amd.com/robust-api-endpoint", cluster)
	}
	return cc, nil
}

func resolveLensWorkloadUID(ctx context.Context, workloadID, workspace, cluster, trainType string) (string, error) {
	workloadID = strings.TrimSpace(workloadID)
	if workloadID == "" {
		return "", fmt.Errorf("workload id is empty")
	}

	cc, err := robustClientForCluster(cluster)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("name", workloadID)
	if workspace != "" {
		params.Set("namespace", workspace)
	}
	kind := lensWorkloadKind(trainType)
	if kind != "" {
		params.Set("kind", kind)
	}

	// robust-api GET /api/v1/workloads returns an envelope whose data field is
	// a flat array of workload records. The client.Get helper already unwraps
	// the {meta, data} envelope, so we only need the slice here.
	var rows []lensWorkloadListItem
	if err := cc.Get(ctx, robustAPIPathWorkloads, params, &rows); err != nil {
		return "", err
	}
	if uid := pickLensWorkloadUID(rows, workloadID, workspace, kind); uid != "" {
		return uid, nil
	}

	// Fallback without kind filter (some infra types – e.g. Deployment – don't
	// populate the kind column consistently).
	if kind != "" {
		params.Del("kind")
		rows = nil
		if err := cc.Get(ctx, robustAPIPathWorkloads, params, &rows); err != nil {
			return "", err
		}
		if uid := pickLensWorkloadUID(rows, workloadID, workspace, ""); uid != "" {
			return uid, nil
		}
	}

	return "", fmt.Errorf("robust workload uid not found for %s/%s (cluster=%s)", workspace, workloadID, cluster)
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

// robustTrainingAvailableResponse mirrors the envelope data returned by
// robust-api GET /api/v1/training/:workload_id/available.
type robustTrainingAvailableResponse struct {
	WorkloadID string   `json:"workload_id"`
	Metrics    []string `json:"metrics"`
	Count      int      `json:"count"`
}

// robustTrainingDataResponse mirrors robust-api GET /api/v1/training/:workload_id/data.
// Each point aggregates all metrics reported for a single iteration from one
// data source; we fan it out into flat PosttrainMetricPoint rows.
type robustTrainingDataResponse struct {
	WorkloadID string                  `json:"workload_id"`
	Points     []robustTrainingDataRow `json:"points"`
	Total      int                     `json:"total"`
}

type robustTrainingDataRow struct {
	Source    string             `json:"source"`
	Iteration int32              `json:"iteration"`
	Metrics   map[string]float64 `json:"metrics"`
	Timestamp string             `json:"timestamp"`
}

func fetchLensTrainingPerformanceData(ctx context.Context, cluster, lensWorkloadUID, start, end, metricsExpr, dataSource string) ([]PosttrainMetricPoint, []string, string, error) {
	if start == "" || end == "" {
		return nil, nil, "", fmt.Errorf("start and end timestamps are required")
	}

	availableRows, err := fetchLensAvailableMetrics(ctx, cluster, lensWorkloadUID, dataSource)
	if err != nil {
		availableRows = nil
	}
	selectedSource := chooseLensMetricDataSource(availableRows, dataSource)

	cc, err := robustClientForCluster(cluster)
	if err != nil {
		return nil, nil, "", err
	}

	params := url.Values{}
	// robust-api does not accept arbitrary start/end filters on this endpoint
	// yet; we pass them via query for forward-compat but they are ignored by
	// the current server, which paginates by iteration instead.
	if start != "" {
		params.Set("start", start)
	}
	if end != "" {
		params.Set("end", end)
	}
	if selectedSource != "" {
		params.Set("source", selectedSource)
	}
	params.Set("page_size", "1000")

	var resp robustTrainingDataResponse
	if err := cc.Get(ctx,
		fmt.Sprintf(robustAPIPathTrainingDataRoot, url.PathEscape(lensWorkloadUID)),
		params, &resp); err != nil {
		return nil, nil, "", err
	}

	// metricsExpr is a comma-separated allow-list from the caller. Keep it
	// simple client-side since robust-api does not support metric filter yet.
	allowed := parseMetricsAllowList(metricsExpr)
	points := make([]PosttrainMetricPoint, 0, len(resp.Points)*4)
	for _, row := range resp.Points {
		ts := parseRobustTimestampMillis(row.Timestamp)
		for name, value := range row.Metrics {
			if name == "" {
				continue
			}
			if len(allowed) > 0 {
				if _, ok := allowed[name]; !ok {
					continue
				}
			}
			points = append(points, PosttrainMetricPoint{
				MetricName: name,
				Value:      value,
				Timestamp:  ts,
				Iteration:  row.Iteration,
				DataSource: row.Source,
			})
		}
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

func fetchLensAvailableMetrics(ctx context.Context, cluster, lensWorkloadUID, dataSource string) ([]lensAvailableMetricRow, error) {
	cc, err := robustClientForCluster(cluster)
	if err != nil {
		return nil, err
	}

	params := url.Values{}
	if s := strings.TrimSpace(dataSource); s != "" {
		params.Set("source", s)
	}

	var resp robustTrainingAvailableResponse
	if err := cc.Get(ctx,
		fmt.Sprintf(robustAPIPathTrainingAvail, url.PathEscape(lensWorkloadUID)),
		params, &resp); err != nil {
		return nil, err
	}

	// robust-api returns a flat []string, not per-source rows. Fold it into
	// the Lens-shaped slice so downstream aggregation keeps working. Keep the
	// originally requested data source label so chooseLensMetricDataSource
	// picks it up.
	sourceLabel := strings.TrimSpace(dataSource)
	rows := make([]lensAvailableMetricRow, 0, len(resp.Metrics))
	for _, name := range resp.Metrics {
		if name == "" {
			continue
		}
		row := lensAvailableMetricRow{Name: name, Count: 0}
		if sourceLabel != "" {
			row.DataSource = []string{sourceLabel}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// parseMetricsAllowList splits a comma-separated, user-supplied metric name
// filter into a lookup set. Empty input means "allow everything".
func parseMetricsAllowList(expr string) map[string]struct{} {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	out := map[string]struct{}{}
	for _, part := range strings.Split(expr, ",") {
		name := strings.TrimSpace(part)
		if name != "" {
			out[name] = struct{}{}
		}
	}
	return out
}

// parseRobustTimestampMillis converts robust-api RFC3339 timestamps into the
// millisecond epoch expected by PosttrainMetricPoint. Invalid strings fall
// back to 0 so sorting/display doesn't crash.
func parseRobustTimestampMillis(s string) int64 {
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UnixMilli()
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	return 0
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

func fetchLatestLossSummary(ctx context.Context, cluster, lensWorkloadUID, start, end string) (*lossSummary, []string, error) {
	points, metrics, _, err := fetchLensTrainingPerformanceData(ctx, cluster, lensWorkloadUID, start, end, "", "")
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

