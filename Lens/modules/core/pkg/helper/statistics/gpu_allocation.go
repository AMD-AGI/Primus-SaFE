// Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
// See LICENSE for license information.

// Package statistics provides statistical calculation utilities for GPU resource analysis.
// It includes time-weighted GPU allocation calculations based on workload and pod activity.
package statistics

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
)

// GpuAllocationResult represents the result of GPU allocation calculation
type GpuAllocationResult struct {
	// TotalAllocatedGpu is the time-weighted average GPU allocation count
	TotalAllocatedGpu float64

	// WorkloadCount is the number of active workloads in the time range
	WorkloadCount int

	// PodCount is the number of active pods in the time range
	PodCount int

	// Details contains per-workload allocation details
	Details []WorkloadAllocationDetail
}

// WorkloadAllocationDetail contains allocation details for a single workload
type WorkloadAllocationDetail struct {
	// WorkloadUID is the unique identifier of the workload
	WorkloadUID string

	// WorkloadName is the name of the workload
	WorkloadName string

	// Namespace is the namespace of the workload
	Namespace string

	// WorkloadKind is the type of workload (Job, Deployment, etc.)
	WorkloadKind string

	// AllocatedGpu is the time-weighted average GPU allocation for this workload
	AllocatedGpu float64

	// ActiveDuration is the duration the workload was active within the time range (in seconds)
	ActiveDuration float64

	// PodCount is the number of pods associated with this workload
	PodCount int

	// PodDetails contains per-pod allocation details
	PodDetails []PodAllocationDetail
}

// PodAllocationDetail contains allocation details for a single pod
type PodAllocationDetail struct {
	// PodUID is the unique identifier of the pod
	PodUID string

	// GpuCount is the number of GPUs allocated to this pod
	GpuCount int32

	// ActiveDuration is the duration the pod was active within the time range (in seconds)
	ActiveDuration float64

	// StartTime is when the pod started (or the start of the time range if earlier)
	StartTime time.Time

	// EndTime is when the pod ended (or the end of the time range if later)
	EndTime time.Time
}

// GpuAllocationCalculator calculates time-weighted GPU allocation
type GpuAllocationCalculator struct {
	workloadFacade           database.WorkloadFacadeInterface
	podFacade                database.PodFacadeInterface
	podRunningPeriodsFacade  database.PodRunningPeriodsFacadeInterface
	clusterName              string
}

// NewGpuAllocationCalculator creates a new calculator for the specified cluster
func NewGpuAllocationCalculator(clusterName string) *GpuAllocationCalculator {
	facade := database.GetFacadeForCluster(clusterName)
	return &GpuAllocationCalculator{
		workloadFacade:          facade.GetWorkload(),
		podFacade:               facade.GetPod(),
		podRunningPeriodsFacade: facade.GetPodRunningPeriods(),
		clusterName:             clusterName,
	}
}

// NewGpuAllocationCalculatorWithFacades creates a calculator with custom facades (useful for testing)
func NewGpuAllocationCalculatorWithFacades(
	workloadFacade database.WorkloadFacadeInterface,
	podFacade database.PodFacadeInterface,
	clusterName string,
) *GpuAllocationCalculator {
	return &GpuAllocationCalculator{
		workloadFacade:          workloadFacade,
		podFacade:               podFacade,
		podRunningPeriodsFacade: database.GetFacadeForCluster(clusterName).GetPodRunningPeriods(),
		clusterName:             clusterName,
	}
}

// NewGpuAllocationCalculatorWithAllFacades creates a calculator with all custom facades (useful for testing)
func NewGpuAllocationCalculatorWithAllFacades(
	workloadFacade database.WorkloadFacadeInterface,
	podFacade database.PodFacadeInterface,
	podRunningPeriodsFacade database.PodRunningPeriodsFacadeInterface,
	clusterName string,
) *GpuAllocationCalculator {
	return &GpuAllocationCalculator{
		workloadFacade:          workloadFacade,
		podFacade:               podFacade,
		podRunningPeriodsFacade: podRunningPeriodsFacade,
		clusterName:             clusterName,
	}
}

// CalculateClusterGpuAllocation calculates the time-weighted GPU allocation for the entire cluster
// within the specified time range. Only top-level workloads (ParentUID == "") are considered.
//
// Parameters:
//   - ctx: context for database operations
//   - startTime: start of the time range
//   - endTime: end of the time range
//
// Returns:
//   - GpuAllocationResult containing the total weighted GPU allocation and details
//   - error if any database operation fails
func (c *GpuAllocationCalculator) CalculateClusterGpuAllocation(
	ctx context.Context,
	startTime, endTime time.Time,
) (*GpuAllocationResult, error) {
	return c.calculateGpuAllocation(ctx, startTime, endTime, "")
}

// CalculateNamespaceGpuAllocation calculates the time-weighted GPU allocation for a specific namespace
// within the specified time range. Only top-level workloads (ParentUID == "") are considered.
//
// Parameters:
//   - ctx: context for database operations
//   - namespace: the namespace to calculate allocation for
//   - startTime: start of the time range
//   - endTime: end of the time range
//
// Returns:
//   - GpuAllocationResult containing the weighted GPU allocation and details for the namespace
//   - error if any database operation fails
func (c *GpuAllocationCalculator) CalculateNamespaceGpuAllocation(
	ctx context.Context,
	namespace string,
	startTime, endTime time.Time,
) (*GpuAllocationResult, error) {
	return c.calculateGpuAllocation(ctx, startTime, endTime, namespace)
}

// calculateGpuAllocation is the core calculation logic
// If namespace is empty, it calculates for the entire cluster
// Uses pod_running_periods table for precise running time calculation
func (c *GpuAllocationCalculator) calculateGpuAllocation(
	ctx context.Context,
	startTime, endTime time.Time,
	namespace string,
) (*GpuAllocationResult, error) {
	// Validate time range
	if endTime.Before(startTime) {
		startTime, endTime = endTime, startTime
	}

	totalDuration := endTime.Sub(startTime).Seconds()
	if totalDuration <= 0 {
		return &GpuAllocationResult{}, nil
	}

	// Use pod_running_periods table for accurate calculation
	if c.podRunningPeriodsFacade == nil {
		log.Warnf("podRunningPeriodsFacade is nil, cannot calculate GPU allocation")
		return &GpuAllocationResult{}, nil
	}

	return c.calculateGpuAllocationFromRunningPeriods(ctx, startTime, endTime, namespace, totalDuration)
}

// calculateGpuAllocationFromRunningPeriods calculates GPU allocation using pod_running_periods table
func (c *GpuAllocationCalculator) calculateGpuAllocationFromRunningPeriods(
	ctx context.Context,
	startTime, endTime time.Time,
	namespace string,
	totalDuration float64,
) (*GpuAllocationResult, error) {
	// 1. Query running periods that overlap with the time range
	var runningPeriods []*model.PodRunningPeriods
	var err error

	if namespace != "" {
		runningPeriods, err = c.podRunningPeriodsFacade.ListRunningPeriodsInTimeRangeByNamespace(ctx, namespace, startTime, endTime)
	} else {
		runningPeriods, err = c.podRunningPeriodsFacade.ListRunningPeriodsInTimeRange(ctx, startTime, endTime)
	}

	if err != nil {
		return nil, err
	}

	if len(runningPeriods) == 0 {
		return &GpuAllocationResult{}, nil
	}

	// Build pod UID list from running periods
	podUIDs := make([]string, 0, len(runningPeriods))
	podRunningPeriodsMap := make(map[string][]*model.PodRunningPeriods)
	for _, period := range runningPeriods {
		if _, exists := podRunningPeriodsMap[period.PodUID]; !exists {
			podUIDs = append(podUIDs, period.PodUID)
			podRunningPeriodsMap[period.PodUID] = make([]*model.PodRunningPeriods, 0)
		}
		podRunningPeriodsMap[period.PodUID] = append(podRunningPeriodsMap[period.PodUID], period)
	}

	// 2. Get pod info from gpu_pods table
	gpuPods, err := c.podFacade.ListPodsByUids(ctx, podUIDs)
	if err != nil {
		return nil, err
	}

	gpuPodsMap := make(map[string]*model.GpuPods, len(gpuPods))
	for _, pod := range gpuPods {
		gpuPodsMap[pod.UID] = pod
	}

	// 3. Find workload UIDs through pod references
	workloadUIDs, err := c.workloadFacade.ListWorkloadUidsByPodUids(ctx, podUIDs)
	if err != nil {
		return nil, err
	}

	if len(workloadUIDs) == 0 {
		return &GpuAllocationResult{}, nil
	}

	// 4. Get all workloads and resolve effective top-level ones
	// (handles circular parent references where Workload <-> PyTorchJob point to each other)
	allWorkloads, err := c.workloadFacade.ListWorkloadsByUids(ctx, workloadUIDs)
	if err != nil {
		return nil, err
	}

	workloads := resolveEffectiveTopLevelWorkloads(allWorkloads)

	// Filter by namespace if specified
	if namespace != "" {
		filtered := make([]*model.GpuWorkload, 0)
		for _, w := range workloads {
			if w.Namespace == namespace {
				filtered = append(filtered, w)
			}
		}
		workloads = filtered
	}

	if len(workloads) == 0 {
		return &GpuAllocationResult{}, nil
	}

	// 5. Get pod references for effective top-level workloads
	// Include refs from all workloads in each top-level's subtree
	workloadUIDList := make([]string, 0, len(workloads))
	for _, w := range workloads {
		workloadUIDList = append(workloadUIDList, w.UID)
	}

	podRefs, err := c.getTopLevelWorkloadPodReferences(ctx, workloadUIDList)
	if err != nil {
		return nil, err
	}

	// Also collect pod refs from child workloads that belong to top-level ones
	childToTopLevel := buildChildToTopLevelMap(allWorkloads, workloads)
	for childUID, topLevelUID := range childToTopLevel {
		childRefs, childErr := c.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, childUID)
		if childErr != nil {
			log.Warnf("Failed to get pod references for child workload %s: %v", childUID, childErr)
			continue
		}
		for _, ref := range childRefs {
			if _, exists := podRefs[ref.PodUID]; !exists {
				podRefs[ref.PodUID] = topLevelUID
			}
		}
	}

	// 6. Calculate time-weighted GPU allocation for each top-level workload
	result := &GpuAllocationResult{
		Details: make([]WorkloadAllocationDetail, 0, len(workloads)),
	}

	for _, workload := range workloads {
		detail := c.calculateWorkloadAllocationFromPeriods(
			workload, podRefs, gpuPodsMap, podRunningPeriodsMap,
			startTime, endTime, totalDuration,
		)
		if len(detail.PodDetails) > 0 {
			result.Details = append(result.Details, detail)
			result.TotalAllocatedGpu += detail.AllocatedGpu
			result.PodCount += detail.PodCount
		}
	}

	result.WorkloadCount = len(result.Details)
	return result, nil
}

// calculateWorkloadAllocationFromPeriods calculates time-weighted GPU allocation using running periods
func (c *GpuAllocationCalculator) calculateWorkloadAllocationFromPeriods(
	workload *model.GpuWorkload,
	podRefs map[string]string,
	gpuPodsMap map[string]*model.GpuPods,
	podRunningPeriodsMap map[string][]*model.PodRunningPeriods,
	startTime, endTime time.Time,
	totalDuration float64,
) WorkloadAllocationDetail {
	detail := WorkloadAllocationDetail{
		WorkloadUID:  workload.UID,
		WorkloadName: workload.Name,
		Namespace:    workload.Namespace,
		WorkloadKind: workload.Kind,
		PodDetails:   make([]PodAllocationDetail, 0),
	}

	// Calculate workload active duration
	workloadStart := maxTime(workload.CreatedAt, startTime)
	workloadEnd := endTime
	if !workload.EndAt.IsZero() && workload.EndAt.Before(endTime) {
		workloadEnd = workload.EndAt
	}

	if workloadEnd.After(workloadStart) {
		detail.ActiveDuration = workloadEnd.Sub(workloadStart).Seconds()
	}

	// Calculate time-weighted GPU allocation from pods
	var totalWeightedGpu float64

	for podUID, workloadUID := range podRefs {
		if workloadUID != workload.UID {
			continue
		}

		periods := podRunningPeriodsMap[podUID]
		if len(periods) == 0 {
			continue
		}

		pod := gpuPodsMap[podUID]
		gpuCount := int32(0)
		if pod != nil {
			gpuCount = pod.GpuAllocated
		} else if len(periods) > 0 {
			// Use GPU count from running period if pod not found
			gpuCount = periods[0].GpuAllocated
		}

		// Calculate total active duration from all running periods
		podDetail := c.calculatePodAllocationFromPeriods(podUID, gpuCount, periods, startTime, endTime)
		if podDetail != nil && podDetail.ActiveDuration > 0 {
			detail.PodDetails = append(detail.PodDetails, *podDetail)
			totalWeightedGpu += float64(gpuCount) * podDetail.ActiveDuration / totalDuration
		}
	}

	detail.AllocatedGpu = totalWeightedGpu
	detail.PodCount = len(detail.PodDetails)

	return detail
}

// calculatePodAllocationFromPeriods calculates the active duration from running periods
func (c *GpuAllocationCalculator) calculatePodAllocationFromPeriods(
	podUID string,
	gpuCount int32,
	periods []*model.PodRunningPeriods,
	startTime, endTime time.Time,
) *PodAllocationDetail {
	if len(periods) == 0 {
		return nil
	}

	var totalActiveDuration float64
	var earliestStart, latestEnd time.Time

	for _, period := range periods {
		// Calculate overlap between period and query time range
		periodStart := period.StartAt
		periodEnd := endTime // default to endTime if still running

		if !period.EndAt.IsZero() {
			periodEnd = period.EndAt
		}

		// Check if period has any overlap with query time range
		if !periodEnd.After(startTime) || periodStart.After(endTime) || periodStart.Equal(endTime) {
			continue
		}

		// Calculate overlap
		overlapStart := maxTime(periodStart, startTime)
		overlapEnd := minTime(periodEnd, endTime)

		if overlapEnd.After(overlapStart) {
			totalActiveDuration += overlapEnd.Sub(overlapStart).Seconds()

			// Track overall time range
			if earliestStart.IsZero() || overlapStart.Before(earliestStart) {
				earliestStart = overlapStart
			}
			if latestEnd.IsZero() || overlapEnd.After(latestEnd) {
				latestEnd = overlapEnd
			}
		}
	}

	if totalActiveDuration <= 0 {
		return nil
	}

	return &PodAllocationDetail{
		PodUID:         podUID,
		GpuCount:       gpuCount,
		ActiveDuration: totalActiveDuration,
		StartTime:      earliestStart,
		EndTime:        latestEnd,
	}
}

// resolveEffectiveTopLevelWorkloads finds the effective top-level workloads from a list.
// A workload is effective top-level if:
// 1. It has ParentUID == "" (standard top-level), OR
// 2. It is part of a circular reference chain (e.g. Workload <-> PyTorchJob) where
//    no workload in the chain has ParentUID == ""
// For circular references, the workload with Kind == "Workload" is preferred as the representative.
func resolveEffectiveTopLevelWorkloads(workloads []*model.GpuWorkload) []*model.GpuWorkload {
	workloadMap := make(map[string]*model.GpuWorkload, len(workloads))
	for _, w := range workloads {
		workloadMap[w.UID] = w
	}

	topLevel := make([]*model.GpuWorkload, 0)
	visited := make(map[string]bool)

	for _, w := range workloads {
		if visited[w.UID] {
			continue
		}

		if w.ParentUID == "" {
			topLevel = append(topLevel, w)
			visited[w.UID] = true
			continue
		}

		// Walk up the parent chain to find the root or detect a cycle
		chain := []string{w.UID}
		chainSet := map[string]bool{w.UID: true}
		current := w

		for {
			parent, exists := workloadMap[current.ParentUID]
			if !exists || parent.ParentUID == "" {
				// Parent is outside the set or is top-level - not a circular reference
				break
			}
			if chainSet[parent.UID] {
				// Circular reference detected - pick the "Workload" kind as representative
				var representative *model.GpuWorkload
				for _, uid := range chain {
					candidate := workloadMap[uid]
					if candidate.Kind == "Workload" {
						representative = candidate
						break
					}
				}
				if representative == nil {
					representative = workloadMap[chain[0]]
				}
				if !visited[representative.UID] {
					topLevel = append(topLevel, representative)
				}
				for _, uid := range chain {
					visited[uid] = true
				}
				break
			}
			chain = append(chain, parent.UID)
			chainSet[parent.UID] = true
			current = parent
		}
	}

	return topLevel
}

// buildChildToTopLevelMap builds a mapping from child workload UIDs to their effective top-level workload UID
func buildChildToTopLevelMap(allWorkloads []*model.GpuWorkload, topLevelWorkloads []*model.GpuWorkload) map[string]string {
	topLevelSet := make(map[string]bool, len(topLevelWorkloads))
	for _, w := range topLevelWorkloads {
		topLevelSet[w.UID] = true
	}

	workloadMap := make(map[string]*model.GpuWorkload, len(allWorkloads))
	for _, w := range allWorkloads {
		workloadMap[w.UID] = w
	}

	result := make(map[string]string)
	for _, w := range allWorkloads {
		if topLevelSet[w.UID] {
			continue
		}
		// Walk up to find the top-level ancestor
		current := w
		seen := map[string]bool{w.UID: true}
		for {
			if topLevelSet[current.ParentUID] {
				result[w.UID] = current.ParentUID
				break
			}
			parent, exists := workloadMap[current.ParentUID]
			if !exists || seen[parent.UID] {
				// Parent is a top-level from resolveEffectiveTopLevelWorkloads (circular ref case)
				// or parent not in our set
				if topLevelSet[current.UID] {
					result[w.UID] = current.UID
				}
				break
			}
			seen[parent.UID] = true
			current = parent
		}
	}
	return result
}

// getTopLevelWorkloadPodReferences gets pod references for top-level workloads only
// Returns a map: podUID -> workloadUID
func (c *GpuAllocationCalculator) getTopLevelWorkloadPodReferences(
	ctx context.Context,
	workloadUIDs []string,
) (map[string]string, error) {
	result := make(map[string]string)

	for _, workloadUID := range workloadUIDs {
		refs, err := c.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to get pod references for workload %s: %v", workloadUID, err)
			continue
		}

		for _, ref := range refs {
			result[ref.PodUID] = workloadUID
		}
	}

	return result, nil
}

// maxTime returns the later of two time values
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// CalculateHourlyGpuAllocation is a convenience method that calculates GPU allocation for a specific hour
// This is commonly used for hourly aggregation jobs
func (c *GpuAllocationCalculator) CalculateHourlyGpuAllocation(
	ctx context.Context,
	hour time.Time,
) (*GpuAllocationResult, error) {
	startTime := hour.Truncate(time.Hour)
	endTime := startTime.Add(time.Hour)
	return c.CalculateClusterGpuAllocation(ctx, startTime, endTime)
}

// CalculateHourlyNamespaceGpuAllocation calculates GPU allocation for a specific namespace and hour
func (c *GpuAllocationCalculator) CalculateHourlyNamespaceGpuAllocation(
	ctx context.Context,
	namespace string,
	hour time.Time,
) (*GpuAllocationResult, error) {
	startTime := hour.Truncate(time.Hour)
	endTime := startTime.Add(time.Hour)
	return c.CalculateNamespaceGpuAllocation(ctx, namespace, startTime, endTime)
}
