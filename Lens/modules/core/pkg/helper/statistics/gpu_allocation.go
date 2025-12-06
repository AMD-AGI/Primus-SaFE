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
	workloadFacade database.WorkloadFacadeInterface
	podFacade      database.PodFacadeInterface
	clusterName    string
}

// NewGpuAllocationCalculator creates a new calculator for the specified cluster
func NewGpuAllocationCalculator(clusterName string) *GpuAllocationCalculator {
	return &GpuAllocationCalculator{
		workloadFacade: database.GetFacadeForCluster(clusterName).GetWorkload(),
		podFacade:      database.GetFacadeForCluster(clusterName).GetPod(),
		clusterName:    clusterName,
	}
}

// NewGpuAllocationCalculatorWithFacades creates a calculator with custom facades (useful for testing)
func NewGpuAllocationCalculatorWithFacades(
	workloadFacade database.WorkloadFacadeInterface,
	podFacade database.PodFacadeInterface,
	clusterName string,
) *GpuAllocationCalculator {
	return &GpuAllocationCalculator{
		workloadFacade: workloadFacade,
		podFacade:      podFacade,
		clusterName:    clusterName,
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

	// 1. Query top-level workloads active within the time range
	workloads, err := c.workloadFacade.ListActiveTopLevelWorkloads(ctx, startTime, endTime, namespace)
	if err != nil {
		return nil, err
	}

	if len(workloads) == 0 {
		return &GpuAllocationResult{}, nil
	}

	// 2. Build workload UID to workload mapping
	workloadMap := make(map[string]*model.GpuWorkload)
	workloadUIDs := make([]string, 0, len(workloads))
	for _, w := range workloads {
		workloadMap[w.UID] = w
		workloadUIDs = append(workloadUIDs, w.UID)
	}

	// 3. Get all descendant workloads (for finding all related pods)
	allWorkloadUIDs, err := c.collectAllDescendantWorkloadUIDs(ctx, workloadUIDs)
	if err != nil {
		return nil, err
	}

	// 4. Get pod references for all workloads (including descendants)
	podRefs, err := c.getWorkloadPodReferences(ctx, allWorkloadUIDs)
	if err != nil {
		return nil, err
	}

	// 5. Get pod resources for all referenced pods
	podUIDs := make([]string, 0, len(podRefs))
	for podUID := range podRefs {
		podUIDs = append(podUIDs, podUID)
	}

	podResources, err := c.getPodResources(ctx, podUIDs)
	if err != nil {
		return nil, err
	}

	// 6. Build workload -> pods mapping (map descendant pods to top-level workloads)
	topLevelWorkloadPods := c.buildTopLevelWorkloadPodsMapping(workloadUIDs, allWorkloadUIDs, podRefs, podResources)

	// 7. Calculate time-weighted GPU allocation for each top-level workload
	result := &GpuAllocationResult{
		Details: make([]WorkloadAllocationDetail, 0, len(workloads)),
	}

	for _, workload := range workloads {
		pods := topLevelWorkloadPods[workload.UID]
		detail := c.calculateWorkloadAllocation(workload, pods, startTime, endTime, totalDuration)
		result.Details = append(result.Details, detail)
		result.TotalAllocatedGpu += detail.AllocatedGpu
		result.PodCount += detail.PodCount
	}

	result.WorkloadCount = len(workloads)

	return result, nil
}

// collectAllDescendantWorkloadUIDs collects all descendant workload UIDs starting from the given top-level UIDs
func (c *GpuAllocationCalculator) collectAllDescendantWorkloadUIDs(
	ctx context.Context,
	topLevelUIDs []string,
) (map[string]string, error) {
	// Map: workloadUID -> topLevelParentUID
	result := make(map[string]string)

	// Initialize with top-level workloads
	for _, uid := range topLevelUIDs {
		result[uid] = uid
	}

	// BFS to find all descendants
	currentLevel := topLevelUIDs
	for len(currentLevel) > 0 {
		var nextLevel []string

		for _, parentUID := range currentLevel {
			topLevelParent := result[parentUID]

			children, err := c.workloadFacade.ListChildrenWorkloadByParentUid(ctx, parentUID)
			if err != nil {
				log.Warnf("Failed to get children for workload %s: %v", parentUID, err)
				continue
			}

			for _, child := range children {
				if _, exists := result[child.UID]; !exists {
					result[child.UID] = topLevelParent
					nextLevel = append(nextLevel, child.UID)
				}
			}
		}

		currentLevel = nextLevel
	}

	return result, nil
}

// getWorkloadPodReferences gets pod references for the given workload UIDs
// Returns a map: podUID -> topLevelWorkloadUID
func (c *GpuAllocationCalculator) getWorkloadPodReferences(
	ctx context.Context,
	workloadUIDs map[string]string,
) (map[string]string, error) {
	result := make(map[string]string)

	for workloadUID, topLevelUID := range workloadUIDs {
		refs, err := c.workloadFacade.ListWorkloadPodReferenceByWorkloadUid(ctx, workloadUID)
		if err != nil {
			log.Warnf("Failed to get pod references for workload %s: %v", workloadUID, err)
			continue
		}

		for _, ref := range refs {
			// Map pod to its top-level workload
			result[ref.PodUID] = topLevelUID
		}
	}

	return result, nil
}

// getPodResources gets PodResource for the given pod UIDs using batch query
func (c *GpuAllocationCalculator) getPodResources(
	ctx context.Context,
	podUIDs []string,
) (map[string]*model.PodResource, error) {
	if len(podUIDs) == 0 {
		return make(map[string]*model.PodResource), nil
	}

	// Use batch query from database layer
	resources, err := c.podFacade.ListPodResourcesByUids(ctx, podUIDs)
	if err != nil {
		return nil, err
	}

	// Convert to map
	result := make(map[string]*model.PodResource, len(resources))
	for _, r := range resources {
		result[r.UID] = r
	}

	return result, nil
}

// buildTopLevelWorkloadPodsMapping builds a mapping from top-level workload UID to its pods
func (c *GpuAllocationCalculator) buildTopLevelWorkloadPodsMapping(
	topLevelUIDs []string,
	allWorkloadUIDs map[string]string,
	podRefs map[string]string,
	podResources map[string]*model.PodResource,
) map[string][]*model.PodResource {
	result := make(map[string][]*model.PodResource)

	// Initialize result map with top-level UIDs
	for _, uid := range topLevelUIDs {
		result[uid] = make([]*model.PodResource, 0)
	}

	// Map pods to their top-level workloads
	for podUID, topLevelUID := range podRefs {
		if resource, exists := podResources[podUID]; exists {
			result[topLevelUID] = append(result[topLevelUID], resource)
		}
	}

	return result
}

// calculateWorkloadAllocation calculates time-weighted GPU allocation for a workload
func (c *GpuAllocationCalculator) calculateWorkloadAllocation(
	workload *model.GpuWorkload,
	pods []*model.PodResource,
	startTime, endTime time.Time,
	totalDuration float64,
) WorkloadAllocationDetail {
	detail := WorkloadAllocationDetail{
		WorkloadUID:  workload.UID,
		WorkloadName: workload.Name,
		Namespace:    workload.Namespace,
		WorkloadKind: workload.Kind,
		PodDetails:   make([]PodAllocationDetail, 0, len(pods)),
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

	for _, pod := range pods {
		podDetail := c.calculatePodAllocation(pod, startTime, endTime, totalDuration)
		detail.PodDetails = append(detail.PodDetails, podDetail)
		totalWeightedGpu += float64(pod.GpuAllocated) * podDetail.ActiveDuration / totalDuration
	}

	detail.AllocatedGpu = totalWeightedGpu
	detail.PodCount = len(pods)

	return detail
}

// calculatePodAllocation calculates the active duration and weighted allocation for a pod
func (c *GpuAllocationCalculator) calculatePodAllocation(
	pod *model.PodResource,
	startTime, endTime time.Time,
	totalDuration float64,
) PodAllocationDetail {
	// Calculate the overlap between pod lifetime and the query time range
	podStart := maxTime(pod.CreatedAt, startTime)
	podEnd := endTime
	if !pod.EndAt.IsZero() && pod.EndAt.Before(endTime) {
		podEnd = pod.EndAt
	}

	var activeDuration float64
	if podEnd.After(podStart) {
		activeDuration = podEnd.Sub(podStart).Seconds()
	}

	return PodAllocationDetail{
		PodUID:         pod.UID,
		GpuCount:       pod.GpuAllocated,
		ActiveDuration: activeDuration,
		StartTime:      podStart,
		EndTime:        podEnd,
	}
}

// maxTime returns the later of two time values
func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
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
