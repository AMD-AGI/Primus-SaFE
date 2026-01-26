// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

package statistics

import (
	"context"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// FilterWorkloadsWithActivePods filters workloads to only include those with at least one running pod.
// This prevents counting workloads that are marked as "Running" in the database but have no actual active pods.
// Returns the filtered list of workloads.
func FilterWorkloadsWithActivePods(ctx context.Context, clusterName string, workloads []*model.GpuWorkload) []*model.GpuWorkload {
	if len(workloads) == 0 {
		return workloads
	}

	// Get the set of workload UIDs that have running pods
	workloadUidsWithRunningPods, err := database.GetFacadeForCluster(clusterName).GetWorkload().ListWorkloadUidsWithRunningPods(ctx)
	if err != nil {
		log.Warnf("Failed to get workload UIDs with running pods, returning all workloads: %v", err)
		return workloads
	}

	// Filter workloads
	result := make([]*model.GpuWorkload, 0, len(workloads))
	filteredCount := 0
	for _, w := range workloads {
		if _, hasRunningPods := workloadUidsWithRunningPods[w.UID]; hasRunningPods {
			result = append(result, w)
		} else {
			filteredCount++
		}
	}

	if filteredCount > 0 {
		log.Debugf("Filtered out %d workloads without running pods (from %d total)", filteredCount, len(workloads))
	}

	return result
}

// FilterWorkloadsWithActivePodsByFacade is like FilterWorkloadsWithActivePods but accepts a facade interface
// This is useful for testing with mock facades
func FilterWorkloadsWithActivePodsByFacade(ctx context.Context, workloadFacade database.WorkloadFacadeInterface, workloads []*model.GpuWorkload) []*model.GpuWorkload {
	if len(workloads) == 0 {
		return workloads
	}

	// Get the set of workload UIDs that have running pods
	workloadUidsWithRunningPods, err := workloadFacade.ListWorkloadUidsWithRunningPods(ctx)
	if err != nil {
		log.Warnf("Failed to get workload UIDs with running pods, returning all workloads: %v", err)
		return workloads
	}

	// Filter workloads
	result := make([]*model.GpuWorkload, 0, len(workloads))
	filteredCount := 0
	for _, w := range workloads {
		if _, hasRunningPods := workloadUidsWithRunningPods[w.UID]; hasRunningPods {
			result = append(result, w)
		} else {
			filteredCount++
		}
	}

	if filteredCount > 0 {
		log.Debugf("Filtered out %d workloads without running pods (from %d total)", filteredCount, len(workloads))
	}

	return result
}
