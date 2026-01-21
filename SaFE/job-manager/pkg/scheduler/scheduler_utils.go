/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package scheduler

import (
	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

const (
	CronjobReason    = "The Execution time has not been reached"
	DependencyReason = "Dependency cannot be satisfied"
)

// formatResourceName formats resource names for display purposes.
func formatResourceName(key string) string {
	if key == common.NvidiaGpu || key == common.AmdGpu {
		return "gpu"
	}
	return key
}

type WorkloadList []*v1.Workload

// Len implements sort.Interface by returning the length of the slice.
func (workloads WorkloadList) Len() int {
	return len(workloads)
}

// Swap implements sort.Interface by swapping elements at the given indices.
func (workloads WorkloadList) Swap(i, j int) {
	workloads[i], workloads[j] = workloads[j], workloads[i]
}

// Less implements sort.Interface for sorting.
func (workloads WorkloadList) Less(i, j int) bool {
	if isReScheduledForFailover(workloads[i]) && !isReScheduledForFailover(workloads[j]) {
		return true
	} else if !isReScheduledForFailover(workloads[i]) && isReScheduledForFailover(workloads[j]) {
		return false
	}
	if !workloads[i].IsDependenciesEnd() && workloads[j].IsDependenciesEnd() {
		return false
	}
	if workloads[i].Spec.Priority > workloads[j].Spec.Priority {
		return true
	} else if workloads[i].Spec.Priority < workloads[j].Spec.Priority {
		return false
	}
	if workloads[i].CreationTimestamp.Time.Before(workloads[j].CreationTimestamp.Time) {
		return true
	}
	if workloads[i].CreationTimestamp.Time.Equal(workloads[j].CreationTimestamp.Time) && workloads[i].Name < workloads[j].Name {
		return true
	}
	return false
}

// isReScheduledForFailover checks if a workload is rescheduled due to failover.
func isReScheduledForFailover(workload *v1.Workload) bool {
	if v1.IsWorkloadReScheduled(workload) && !v1.IsWorkloadPreempted(workload) {
		return true
	}
	return false
}
