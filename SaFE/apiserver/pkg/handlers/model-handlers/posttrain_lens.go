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

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/robustclient"
	"github.com/AMD-AIG-AIMA/SAFE/utils/pkg/timeutil"
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

type robustWorkloadListResponse struct {
	Data  []robustWorkloadListItem `json:"data"`
	Total int                      `json:"total"`
}

type robustWorkloadListItem struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	UID       string `json:"uid"`
	Status    string `json:"status"`
	StartAt   int64  `json:"start_at"`
	EndAt     int64  `json:"end_at"`
}

type robustTrainingPerformancePoint struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Timestamp int64   `json:"timestamp"`
}

type lossSummary struct {
	Value      float64
	MetricName string
	DataSource string
}

func resolveRobustWorkloadUID(ctx context.Context, rc *robustclient.Client, workloadID, workspace, cluster, trainType string) (string, error) {
	workloadID = strings.TrimSpace(workloadID)
	if workloadID == "" {
		return "", fmt.Errorf("workload id is empty")
	}

	cc := rc.ForCluster(cluster)
	if cc == nil {
		return "", fmt.Errorf("cluster %s not available in robust client", cluster)
	}

	params := url.Values{}
	params.Set("name", workloadID)
	params.Set("namespace", workspace)
	params.Set("page_num", "1")
	params.Set("page_size", "20")
	kind := robustWorkloadKind(trainType)
	if kind != "" {
		params.Set("kind", kind)
	}

	var resp robustWorkloadListResponse
	if err := cc.Get(ctx, "/api/v1/workloads", params, &resp); err != nil {
		return "", err
	}
	if uid := pickWorkloadUID(resp.Data, workloadID, workspace, kind); uid != "" {
		return uid, nil
	}

	if kind != "" {
		params.Del("kind")
		resp = robustWorkloadListResponse{}
		if err := cc.Get(ctx, "/api/v1/workloads", params, &resp); err != nil {
			return "", err
		}
		if uid := pickWorkloadUID(resp.Data, workloadID, workspace, ""); uid != "" {
			return uid, nil
		}
	}

	return "", fmt.Errorf("workload uid not found for %s/%s in cluster %s", workspace, workloadID, cluster)
}

func robustWorkloadKind(trainType string) string {
	switch strings.ToLower(strings.TrimSpace(trainType)) {
	case "rl":
		return common.RayJobKind
	case "sft":
		return common.PytorchJobKind
	default:
		return ""
	}
}

func pickWorkloadUID(items []robustWorkloadListItem, workloadID, workspace, preferredKind string) string {
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

func fetchTrainingPerformanceData(ctx context.Context, rc *robustclient.Client, cluster, workloadUID, start, end string) ([]PosttrainMetricPoint, []string, error) {
	cc := rc.ForCluster(cluster)
	if cc == nil {
		return nil, nil, fmt.Errorf("cluster %s not available", cluster)
	}

	if start == "" || end == "" {
		return nil, nil, fmt.Errorf("start and end timestamps are required")
	}

	params := url.Values{}
	params.Set("start", start)
	params.Set("end", end)

	var resp []robustTrainingPerformancePoint
	if err := cc.Get(ctx, fmt.Sprintf("/api/v1/workloads/%s/trainingPerformance", url.PathEscape(workloadUID)), params, &resp); err != nil {
		return nil, nil, err
	}

	points := make([]PosttrainMetricPoint, 0, len(resp))
	metricSet := make(map[string]struct{}, len(resp))
	for _, point := range resp {
		if point.Metric == "" {
			continue
		}
		points = append(points, PosttrainMetricPoint{
			MetricName: point.Metric,
			Value:      point.Value,
			Timestamp:  point.Timestamp,
			DataSource: "log",
		})
		metricSet[point.Metric] = struct{}{}
	}

	metrics := make([]string, 0, len(metricSet))
	for metric := range metricSet {
		metrics = append(metrics, metric)
	}
	sort.Strings(metrics)
	return points, metrics, nil
}

func filterTrainingPerformancePoints(points []PosttrainMetricPoint, metricsExpr string) []PosttrainMetricPoint {
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

func fetchLatestLossSummary(ctx context.Context, rc *robustclient.Client, cluster, workloadUID, start, end string) (*lossSummary, []string, error) {
	points, metrics, err := fetchTrainingPerformanceData(ctx, rc, cluster, workloadUID, start, end)
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
