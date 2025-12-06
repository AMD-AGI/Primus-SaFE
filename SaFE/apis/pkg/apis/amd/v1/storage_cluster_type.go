/*
 * Copyright (C) 2025-2025, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package v1

type StorageType string

const (
	NVME StorageType = "nvme"
	SSD  StorageType = "ssd"
	HDD  StorageType = "hdd"
)

type Capacity struct {
	TotalBytes     uint64 `json:"bytesTotal,omitempty"`
	UsedBytes      uint64 `json:"bytesUsed,omitempty"`
	AvailableBytes uint64 `json:"bytesAvailable,omitempty"`
	LastUpdated    string `json:"lastUpdated,omitempty"`
}
