// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

import promModel "github.com/prometheus/common/model"

type TimePoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

type MetricsSeries struct {
	Labels promModel.Metric `json:"labels"`
	Values []TimePoint      `json:"values"`
}

type MetricsGraph struct {
	Serial int                `json:"serial"`
	Series []MetricsSeries    `json:"series"`
	Config MetricsGraphConfig `json:"config"`
}

type MetricsGraphConfig struct {
	YAxisUnit string `json:"y_axis_unit"`
}
