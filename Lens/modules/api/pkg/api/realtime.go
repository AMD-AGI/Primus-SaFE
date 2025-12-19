package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	coreErrors "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// RealtimeStatusParams - Query parameters for /api/v1/realtime/status
type RealtimeStatusParams struct {
	Cluster string   `form:"cluster" binding:"required"`
	Include []string `form:"include"` // nodes, pods, workloads, alerts
}

// RealtimeStatusResponse - Response for /api/v1/realtime/status
type RealtimeStatusResponse struct {
	Cluster            string               `json:"cluster"`
	Timestamp          time.Time            `json:"timestamp"`
	CurrentGPUUsage    GPUUsageSummary      `json:"current_gpu_usage"`
	RunningTasks       int                  `json:"running_tasks"`
	AvailableResources ResourceAvailability `json:"available_resources"`
	Alerts             []Alert              `json:"alerts,omitempty"`
	Nodes              []NodeStatus         `json:"nodes,omitempty"`
	RecentEvents       []Event              `json:"recent_events,omitempty"`
}

// GPUUsageSummary - GPU usage summary
type GPUUsageSummary struct {
	TotalGPUs       int     `json:"total_gpus"`
	AllocatedGPUs   int     `json:"allocated_gpus"`
	UtilizedGPUs    int     `json:"utilized_gpus"` // >50% utilization
	AllocationRate  float64 `json:"allocation_rate"`
	UtilizationRate float64 `json:"utilization_rate"`
}

// ResourceAvailability - Available resources
type ResourceAvailability struct {
	AvailableGPUs    int `json:"available_gpus"`
	AvailableNodes   int `json:"available_nodes"`
	MaxContiguousGPU int `json:"max_contiguous_gpu"`
}

// Alert - Alert information
type Alert struct {
	ID        string    `json:"id"`
	Severity  string    `json:"severity"` // critical, warning, info
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// NodeStatus - Node status summary
type NodeStatus struct {
	NodeName      string  `json:"node_name"`
	Status        string  `json:"status"`
	TotalGPUs     int32   `json:"total_gpus"`
	AllocatedGPUs int32   `json:"allocated_gpus"`
	Utilization   float64 `json:"utilization"`
}

// Event - Recent event
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Object    string    `json:"object"`
	Message   string    `json:"message"`
}

// RunningTasksParams - Query parameters for /api/v1/realtime/running-tasks
type RunningTasksParams struct {
	Cluster   string `form:"cluster" binding:"required"`
	Namespace string `form:"namespace"`
}

// RunningTasksResponse - Response for /api/v1/realtime/running-tasks
type RunningTasksResponse struct {
	Cluster    string        `json:"cluster"`
	Timestamp  time.Time     `json:"timestamp"`
	TotalTasks int           `json:"total_tasks"`
	Tasks      []RunningTask `json:"tasks"`
}

// RunningTask - Running GPU task
type RunningTask struct {
	PodUID        string    `json:"pod_uid"`
	PodName       string    `json:"pod_name"`
	Namespace     string    `json:"namespace"`
	WorkloadType  string    `json:"workload_type"` // Job, Deployment, etc.
	WorkloadName  string    `json:"workload_name"`
	NodeName      string    `json:"node_name"`
	AllocatedGPUs int32     `json:"allocated_gpus"`
	RunningTime   int64     `json:"running_time_seconds"`
	StartedAt     time.Time `json:"started_at"`
	Owner         string    `json:"owner"`
}

// getRealtimeStatus - GET /api/v1/realtime/status
func getRealtimeStatus(c *gin.Context) {
	var params RealtimeStatusParams
	if err := c.ShouldBindQuery(&params); err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithError(err))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(params.Cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Parse include fields
	includeMap := make(map[string]bool)
	for _, field := range params.Include {
		includeMap[strings.ToLower(field)] = true
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := fmt.Sprintf("realtime_status:%s", params.Cluster)

	var response RealtimeStatusResponse
	err = cacheFacade.Get(c.Request.Context(), cacheKey, &response)
	if err == nil && response.Cluster != "" && !response.Timestamp.IsZero() {
		// Cache hit with valid data - apply include filters
		response = filterRealtimeResponse(response, includeMap)
		c.JSON(http.StatusOK, rest.SuccessResp(c, response))
		return
	}

	// Cache miss - build response
	response, err = buildRealtimeStatus(c.Request.Context(), clients.ClusterName, includeMap)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}

	// Cache the result for 30 seconds
	expiresAt := time.Now().Add(30 * time.Second)
	_ = cacheFacade.Set(c.Request.Context(), cacheKey, response, &expiresAt)

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// buildRealtimeStatus - Build real-time status response
func buildRealtimeStatus(ctx context.Context, clusterName string, includeMap map[string]bool) (RealtimeStatusResponse, error) {
	facade := database.GetFacadeForCluster(clusterName)

	response := RealtimeStatusResponse{
		Cluster:   clusterName,
		Timestamp: time.Now(),
	}

	// Get GPU usage summary
	gpuUsage, err := buildGPUUsageSummary(ctx, facade)
	if err != nil {
		return response, err
	}
	response.CurrentGPUUsage = gpuUsage

	// Get running tasks count
	pods, err := facade.GetPod().ListActiveGpuPods(ctx)
	if err != nil {
		return response, err
	}
	response.RunningTasks = len(pods)

	// Get available resources
	availableRes, err := buildResourceAvailability(ctx, facade)
	if err != nil {
		return response, err
	}
	response.AvailableResources = availableRes

	// Include optional fields
	if includeMap["nodes"] {
		nodes, _ := buildNodeStatusList(ctx, facade)
		response.Nodes = nodes
	}

	if includeMap["alerts"] {
		alerts, _ := buildAlertsList(ctx, facade)
		response.Alerts = alerts
	}

	if includeMap["events"] {
		events, _ := buildRecentEventsList(ctx, facade)
		response.RecentEvents = events
	}

	return response, nil
}

// buildGPUUsageSummary - Build GPU usage summary
func buildGPUUsageSummary(ctx context.Context, facade database.FacadeInterface) (GPUUsageSummary, error) {
	nodes, err := facade.GetNode().ListGpuNodes(ctx)
	if err != nil {
		return GPUUsageSummary{}, err
	}

	var totalGPUs, allocatedGPUs, utilizedGPUs int32
	var totalUtilization float64

	for _, node := range nodes {
		totalGPUs += node.GpuCount
		allocatedGPUs += node.GpuAllocation

		// Count GPUs with >50% utilization
		if node.GpuUtilization > 50 {
			utilizedGPUs += node.GpuAllocation
		}

		totalUtilization += node.GpuUtilization * float64(node.GpuCount)
	}

	allocationRate := 0.0
	if totalGPUs > 0 {
		allocationRate = float64(allocatedGPUs) / float64(totalGPUs) * 100
	}

	utilizationRate := 0.0
	if totalGPUs > 0 {
		utilizationRate = totalUtilization / float64(totalGPUs)
	}

	return GPUUsageSummary{
		TotalGPUs:       int(totalGPUs),
		AllocatedGPUs:   int(allocatedGPUs),
		UtilizedGPUs:    int(utilizedGPUs),
		AllocationRate:  allocationRate,
		UtilizationRate: utilizationRate,
	}, nil
}

// buildResourceAvailability - Build resource availability
func buildResourceAvailability(ctx context.Context, facade database.FacadeInterface) (ResourceAvailability, error) {
	nodes, err := facade.GetNode().ListGpuNodes(ctx)
	if err != nil {
		return ResourceAvailability{}, err
	}

	var availableGPUs, availableNodes int
	var maxContiguous int

	for _, node := range nodes {
		available := node.GpuCount - node.GpuAllocation
		if available > 0 {
			availableGPUs += int(available)
			availableNodes++

			// Track largest contiguous block
			if int(available) > maxContiguous {
				maxContiguous = int(available)
			}
		}
	}

	return ResourceAvailability{
		AvailableGPUs:    availableGPUs,
		AvailableNodes:   availableNodes,
		MaxContiguousGPU: maxContiguous,
	}, nil
}

// buildNodeStatusList - Build node status list
func buildNodeStatusList(ctx context.Context, facade database.FacadeInterface) ([]NodeStatus, error) {
	nodes, err := facade.GetNode().ListGpuNodes(ctx)
	if err != nil {
		return nil, err
	}

	nodeStatuses := make([]NodeStatus, 0, len(nodes))
	for _, node := range nodes {
		nodeStatuses = append(nodeStatuses, NodeStatus{
			NodeName:      node.Name,
			Status:        node.Status,
			TotalGPUs:     node.GpuCount,
			AllocatedGPUs: node.GpuAllocation,
			Utilization:   node.GpuUtilization,
		})
	}

	return nodeStatuses, nil
}

// buildAlertsList - Build alerts list (placeholder)
func buildAlertsList(ctx context.Context, facade database.FacadeInterface) ([]Alert, error) {
	// Placeholder - could integrate with alert system
	// For now, return empty list
	return []Alert{}, nil
}

// buildRecentEventsList - Build recent events list
func buildRecentEventsList(ctx context.Context, facade database.FacadeInterface) ([]Event, error) {
	// Get recent pod events (last 10)
	pods, err := facade.GetPod().ListActiveGpuPods(ctx)
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0)

	// Limit to first 10 pods
	limit := 10
	if len(pods) > limit {
		pods = pods[:limit]
	}

	for _, pod := range pods {
		events = append(events, Event{
			Timestamp: pod.CreatedAt,
			Type:      "PodCreated",
			Object:    fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
			Message:   fmt.Sprintf("Pod created with %d GPUs", pod.GpuAllocated),
		})
	}

	return events, nil
}

// filterRealtimeResponse - Filter response based on include fields
func filterRealtimeResponse(response RealtimeStatusResponse, includeMap map[string]bool) RealtimeStatusResponse {
	if !includeMap["nodes"] {
		response.Nodes = nil
	}
	if !includeMap["alerts"] {
		response.Alerts = nil
	}
	if !includeMap["events"] {
		response.RecentEvents = nil
	}
	return response
}

// getRunningTasks - GET /api/v1/realtime/running-tasks
func getRunningTasks(c *gin.Context) {
	var params RunningTasksParams
	if err := c.ShouldBindQuery(&params); err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithError(err))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(params.Cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := fmt.Sprintf("running_tasks:%s:%s", params.Cluster, params.Namespace)

	var response RunningTasksResponse
	err = cacheFacade.Get(c.Request.Context(), cacheKey, &response)
	if err == nil && response.Cluster != "" && !response.Timestamp.IsZero() {
		// Cache hit with valid data
		c.JSON(http.StatusOK, rest.SuccessResp(c, response))
		return
	}

	// Cache miss - build response
	response, err = buildRunningTasksResponse(c.Request.Context(), clients.ClusterName, params.Namespace)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}

	// Cache the result for 30 seconds
	expiresAtTasks := time.Now().Add(30 * time.Second)
	_ = cacheFacade.Set(c.Request.Context(), cacheKey, response, &expiresAtTasks)

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// buildRunningTasksResponse - Build running tasks response
func buildRunningTasksResponse(ctx context.Context, clusterName, namespace string) (RunningTasksResponse, error) {
	facade := database.GetFacadeForCluster(clusterName)
	podFacade := facade.GetPod()

	// Get all active GPU pods
	pods, err := podFacade.ListActiveGpuPods(ctx)
	if err != nil {
		return RunningTasksResponse{}, err
	}

	// Filter by namespace if specified
	if namespace != "" {
		filteredPods := make([]*dbModel.GpuPods, 0)
		for _, pod := range pods {
			if pod.Namespace == namespace {
				filteredPods = append(filteredPods, pod)
			}
		}
		pods = filteredPods
	}

	// Build task list
	tasks := make([]RunningTask, 0, len(pods))
	now := time.Now()

	for _, pod := range pods {
		// Calculate running time
		runningTime := int64(now.Sub(pod.CreatedAt).Seconds())

		// Get workload info from owner reference
		workloadType, workloadName := parseOwnerReference(pod.OwnerUID)

		task := RunningTask{
			PodUID:        pod.UID,
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			WorkloadType:  workloadType,
			WorkloadName:  workloadName,
			NodeName:      pod.NodeName,
			AllocatedGPUs: pod.GpuAllocated,
			RunningTime:   runningTime,
			StartedAt:     pod.CreatedAt,
			Owner:         pod.OwnerUID,
		}
		tasks = append(tasks, task)
	}

	return RunningTasksResponse{
		Cluster:    clusterName,
		Timestamp:  time.Now(),
		TotalTasks: len(tasks),
		Tasks:      tasks,
	}, nil
}

// parseOwnerReference - Parse owner reference to get workload type and name
func parseOwnerReference(ownerUID string) (string, string) {
	// Simplified implementation - in real scenario, would query workload table
	// For now, return placeholder values
	if ownerUID == "" {
		return "Unknown", "Unknown"
	}
	return "Job", ownerUID[:8] // Simplified - just use first 8 chars of UID
}

