// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package statistics

import (
	"testing"
	"time"
)

func TestMaxTime(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 11, 0, 0, 0, time.UTC)

	if result := maxTime(t1, t2); !result.Equal(t2) {
		t.Errorf("maxTime(%v, %v) = %v, want %v", t1, t2, result, t2)
	}

	if result := maxTime(t2, t1); !result.Equal(t2) {
		t.Errorf("maxTime(%v, %v) = %v, want %v", t2, t1, result, t2)
	}

	if result := maxTime(t1, t1); !result.Equal(t1) {
		t.Errorf("maxTime(%v, %v) = %v, want %v", t1, t1, result, t1)
	}
}

func TestGpuAllocationResult_Empty(t *testing.T) {
	result := &GpuAllocationResult{}

	if result.TotalAllocatedGpu != 0 {
		t.Errorf("TotalAllocatedGpu = %v, want 0", result.TotalAllocatedGpu)
	}

	if result.WorkloadCount != 0 {
		t.Errorf("WorkloadCount = %v, want 0", result.WorkloadCount)
	}

	if result.PodCount != 0 {
		t.Errorf("PodCount = %v, want 0", result.PodCount)
	}
}

func TestWorkloadAllocationDetail_Fields(t *testing.T) {
	detail := WorkloadAllocationDetail{
		WorkloadUID:    "uid-123",
		WorkloadName:   "my-job",
		Namespace:      "ml-team",
		WorkloadKind:   "Job",
		AllocatedGpu:   8.5,
		ActiveDuration: 1800.0,
		PodCount:       3,
	}

	if detail.WorkloadUID != "uid-123" {
		t.Errorf("WorkloadUID = %v, want uid-123", detail.WorkloadUID)
	}

	if detail.Namespace != "ml-team" {
		t.Errorf("Namespace = %v, want ml-team", detail.Namespace)
	}

	if detail.AllocatedGpu != 8.5 {
		t.Errorf("AllocatedGpu = %v, want 8.5", detail.AllocatedGpu)
	}
}
