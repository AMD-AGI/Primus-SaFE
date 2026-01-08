// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package model

type StorageStat struct {
	TotalSpace            float64 `json:"total_space"`
	UsedSpace             float64 `json:"used_space"`
	UsagePercentage       float64 `json:"usage_percentage"`
	TotalInodes           float64 `json:"total_inodes"`
	UsedInodes            float64 `json:"used_inodes"`
	InodesUsagePercentage float64 `json:"inodes_usage_percentage"`
	ReadBandwidth         float64 `json:"read_bandwidth"`
	WriteBandwidth        float64 `json:"write_bandwidth"`
}
