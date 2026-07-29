/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package model_handlers

import (
	"testing"
	"time"

	"github.com/lib/pq"
	"gotest.tools/assert"

	dbclient "github.com/AMD-AIG-AIMA/SAFE/common/pkg/database/client"
)

// NOTE: The former TestResolveLensWorkloadUID_UnwrapsLensEnvelope and
// TestFetchLatestLossSummary_UsesMetricsData tests spun up an httptest server
// that emulated the primus-lens-api. After migrating posttrain_lens.go to
// use robustclient → data-plane robust-api, those handler paths are no
// longer HTTP-level but go through robustclient.Client, which requires a
// registered per-cluster endpoint. They are covered now by integration
// tests in resource-manager / robustclient packages.

func TestBuildPosttrainMetricSeries_AddsLossAlias(t *testing.T) {
	points := []PosttrainMetricPoint{
		{
			MetricName: "lm_loss",
			Value:      2.3,
			Timestamp:  1200,
			Iteration:  2,
			DataSource: "log",
		},
		{
			MetricName: "lm_loss",
			Value:      1.7,
			Timestamp:  1300,
			Iteration:  3,
			DataSource: "log",
		},
		{
			MetricName: "tflops",
			Value:      100,
			Timestamp:  1100,
			Iteration:  1,
			DataSource: "log",
		},
	}

	series := buildPosttrainMetricSeries(points, "lm_loss")
	assert.Assert(t, series != nil)
	assert.Equal(t, len(series["loss"]), 2)
	assert.Equal(t, series["loss"][0].Step, int32(2))
	assert.Equal(t, series["loss"][1].Value, 1.7)
	assert.Equal(t, len(series["lm_loss"]), 2)
}

func TestDefaultLensTimeRangeForRun_UsesStartAndEndTimes(t *testing.T) {
	start := time.Date(2026, 4, 12, 8, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 12, 8, 30, 0, 0, time.UTC)
	created := time.Date(2026, 4, 12, 7, 59, 0, 0, time.UTC)

	run := &dbclient.PosttrainRunView{
		StartTime: pq.NullTime{Time: start, Valid: true},
		EndTime:   pq.NullTime{Time: end, Valid: true},
		CreatedAt: pq.NullTime{Time: created, Valid: true},
	}

	startMs, endMs, err := defaultLensTimeRangeForRun(run)
	assert.NilError(t, err)
	assert.Equal(t, startMs, "1775980800000")
	assert.Equal(t, endMs, "1775982600000")
}

// --- merged from posttrain_lens_pure_test.go ---

// TestLensWorkloadKind verifies train type to workload kind mapping.
func TestLensWorkloadKind(t *testing.T) {
	if lensWorkloadKind("rl") == "" {
		t.Error("rl should map to a non-empty kind")
	}
	if lensWorkloadKind("sft") == "" {
		t.Error("sft should map to a non-empty kind")
	}
	if lensWorkloadKind("unknown") != "" {
		t.Error("unknown train type should map to empty kind")
	}
}

// TestPickLensWorkloadUID verifies preferred-kind matching then fallback matching.
func TestPickLensWorkloadUID(t *testing.T) {
	items := []lensWorkloadListItem{
		{Name: "wl", Namespace: "ws", Kind: "RayJob", UID: "uid-ray"},
		{Name: "wl", Namespace: "ws", Kind: "PyTorchJob", UID: "uid-pt"},
	}
	if got := pickLensWorkloadUID(items, "wl", "ws", "PyTorchJob"); got != "uid-pt" {
		t.Errorf("expected uid-pt, got %s", got)
	}
	if got := pickLensWorkloadUID(items, "wl", "ws", ""); got != "uid-ray" {
		t.Errorf("expected first match uid-ray, got %s", got)
	}
	if got := pickLensWorkloadUID(items, "missing", "ws", ""); got != "" {
		t.Errorf("expected empty for missing workload, got %s", got)
	}
}

// TestDefaultLensTimeRange verifies start/end normalization.
func TestDefaultLensTimeRange(t *testing.T) {
	if _, _, err := defaultLensTimeRange(time.Time{}, time.Time{}, time.Time{}); err == nil {
		t.Error("expected error when no start time available")
	}
	start := time.Now().Add(-time.Hour)
	s, e, err := defaultLensTimeRange(start, time.Time{}, time.Time{})
	if err != nil || s == "" || e == "" {
		t.Errorf("expected valid range, got s=%s e=%s err=%v", s, e, err)
	}
}

// TestParseMetricsAllowList verifies comma-separated parsing.
func TestParseMetricsAllowList(t *testing.T) {
	if parseMetricsAllowList("") != nil {
		t.Error("empty expr should yield nil")
	}
	set := parseMetricsAllowList("a, b ,, c")
	if len(set) != 3 {
		t.Errorf("expected 3 entries, got %d", len(set))
	}
}

// TestParseRobustTimestampMillis verifies RFC3339 parsing with fallback.
func TestParseRobustTimestampMillis(t *testing.T) {
	if parseRobustTimestampMillis("") != 0 {
		t.Error("empty timestamp should be 0")
	}
	if parseRobustTimestampMillis("bad") != 0 {
		t.Error("invalid timestamp should be 0")
	}
	if parseRobustTimestampMillis("2026-01-01T00:00:00Z") <= 0 {
		t.Error("valid RFC3339 should parse to positive millis")
	}
}

// TestNormalizeMetricName verifies separators and case are stripped.
func TestNormalizeMetricName(t *testing.T) {
	if normalizeMetricName("Train/Loss_1-2 3") != "trainloss123" {
		t.Errorf("unexpected normalized name: %s", normalizeMetricName("Train/Loss_1-2 3"))
	}
}

// TestPickLossMetricName verifies preferred and heuristic loss selection.
func TestPickLossMetricName(t *testing.T) {
	if pickLossMetricName(nil) != "" {
		t.Error("empty metrics should yield empty")
	}
	if got := pickLossMetricName([]string{"acc", "loss"}); got != "loss" {
		t.Errorf("expected loss, got %s", got)
	}
	if got := pickLossMetricName([]string{"acc", "custom_loss"}); got != "custom_loss" {
		t.Errorf("expected custom_loss via heuristic, got %s", got)
	}
}

// TestPreferredMetricDataSource verifies ordering preference.
func TestPreferredMetricDataSource(t *testing.T) {
	if preferredMetricDataSource(nil) != "" {
		t.Error("empty sources should yield empty")
	}
	if got := preferredMetricDataSource([]string{"wandb", "log"}); got != "log" {
		t.Errorf("expected log preferred, got %s", got)
	}
	if got := preferredMetricDataSource([]string{"custom"}); got != "custom" {
		t.Errorf("expected fallback custom, got %s", got)
	}
}

// TestMetricRowHasDataSource verifies case-insensitive source matching.
func TestMetricRowHasDataSource(t *testing.T) {
	row := lensAvailableMetricRow{Name: "loss", DataSource: []string{"Log", "Wandb"}}
	if !metricRowHasDataSource(row, "") {
		t.Error("empty data source should always match")
	}
	if !metricRowHasDataSource(row, "log") {
		t.Error("expected case-insensitive match")
	}
	if metricRowHasDataSource(row, "tensorflow") {
		t.Error("non-present source should not match")
	}
}

// TestMetricNamesFromAvailableRows verifies names extraction skips blanks.
func TestMetricNamesFromAvailableRows(t *testing.T) {
	rows := []lensAvailableMetricRow{{Name: "a"}, {Name: " "}, {Name: "b"}}
	names := metricNamesFromAvailableRows(rows)
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}
}

// TestAvailableMetricNamesForSource verifies filtering by data source and sorting.
func TestAvailableMetricNamesForSource(t *testing.T) {
	rows := []lensAvailableMetricRow{
		{Name: "b", DataSource: []string{"log"}},
		{Name: "a", DataSource: []string{"wandb"}},
	}
	all := availableMetricNamesForSource(rows, "")
	if len(all) != 2 || all[0] != "a" {
		t.Errorf("expected sorted [a b], got %v", all)
	}
	logOnly := availableMetricNamesForSource(rows, "log")
	if len(logOnly) != 1 || logOnly[0] != "b" {
		t.Errorf("expected [b] for log source, got %v", logOnly)
	}
}

// TestInferMetricDataSourceFromPoints verifies the first non-empty source is returned.
func TestInferMetricDataSourceFromPoints(t *testing.T) {
	points := []PosttrainMetricPoint{
		{MetricName: "loss"},
		{MetricName: "acc", DataSource: "wandb"},
	}
	if inferMetricDataSourceFromPoints(points) != "wandb" {
		t.Error("expected wandb data source")
	}
	if inferMetricDataSourceFromPoints(nil) != "" {
		t.Error("empty points should yield empty")
	}
}

// TestFilterLensTrainingPerformancePoints verifies metric filtering semantics.
func TestFilterLensTrainingPerformancePoints(t *testing.T) {
	points := []PosttrainMetricPoint{
		{MetricName: "loss"},
		{MetricName: "acc"},
	}
	if len(filterLensTrainingPerformancePoints(points, "")) != 2 {
		t.Error("empty filter should return all points")
	}
	if len(filterLensTrainingPerformancePoints(points, "all")) != 2 {
		t.Error("'all' filter should return all points")
	}
	filtered := filterLensTrainingPerformancePoints(points, "{loss}")
	if len(filtered) != 1 || filtered[0].MetricName != "loss" {
		t.Errorf("expected only loss, got %v", filtered)
	}
}

// TestLatestLossSummaryFromPoints verifies the latest loss point is selected.
func TestLatestLossSummaryFromPoints(t *testing.T) {
	points := []PosttrainMetricPoint{
		{MetricName: "loss", Value: 1.0, Timestamp: 100, DataSource: "log"},
		{MetricName: "loss", Value: 0.5, Timestamp: 200, DataSource: "log"},
	}
	summary := latestLossSummaryFromPoints(points, []string{"loss"})
	if summary == nil || summary.Value != 0.5 {
		t.Errorf("expected latest loss 0.5, got %+v", summary)
	}
	if latestLossSummaryFromPoints(points, []string{"acc"}) != nil {
		t.Error("no loss metric should yield nil summary")
	}
}

// TestBuildPosttrainMetricSeries verifies series grouping and loss aliasing.
func TestBuildPosttrainMetricSeries(t *testing.T) {
	if buildPosttrainMetricSeries(nil, "") != nil {
		t.Error("empty points should yield nil series")
	}
	points := []PosttrainMetricPoint{
		{MetricName: "train_loss", Value: 1, Iteration: 1, Timestamp: 1000},
	}
	series := buildPosttrainMetricSeries(points, "train_loss")
	if _, ok := series["train_loss"]; !ok {
		t.Error("expected train_loss series")
	}
	if _, ok := series["loss"]; !ok {
		t.Error("expected aliased loss series")
	}
}

// TestFormatMetricTimestamp verifies zero and positive timestamps.
func TestFormatMetricTimestamp(t *testing.T) {
	if formatMetricTimestamp(0) != "" {
		t.Error("zero timestamp should yield empty string")
	}
	if formatMetricTimestamp(1000) == "" {
		t.Error("positive timestamp should yield formatted string")
	}
}

// TestChooseLensMetricDataSource verifies explicit request short-circuits selection.
func TestChooseLensMetricDataSource(t *testing.T) {
	if got := chooseLensMetricDataSource(nil, "wandb"); got != "wandb" {
		t.Errorf("explicit request should win, got %s", got)
	}
	rows := []lensAvailableMetricRow{
		{Name: "loss", DataSource: []string{"log"}},
	}
	if got := chooseLensMetricDataSource(rows, ""); got != "log" {
		t.Errorf("expected log from loss row, got %s", got)
	}
}
