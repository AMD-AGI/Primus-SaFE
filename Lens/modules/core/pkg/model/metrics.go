// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

type GrafanaMetricsSeries struct {
	Name   string       `json:"name"`
	Points [][2]float64 `json:"points"`
}
