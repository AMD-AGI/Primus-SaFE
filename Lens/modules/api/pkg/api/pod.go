package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	coreErrors "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/gin-gonic/gin"
)

// PodStatsQueryParams - Query parameters for /api/v1/pods/stats
type PodStatsQueryParams struct {
	Cluster   string   `form:"cluster" binding:"required"`
	Namespace string   `form:"namespace"`
	PodName   string   `form:"pod_name"`
	Labels    []string `form:"labels"`
	StartTime string   `form:"start_time"`
	EndTime   string   `form:"end_time"`
	Page      int      `form:"page" binding:"omitempty,min=1"`
	PageSize  int      `form:"page_size" binding:"omitempty,min=1,max=100"`
}

// PodStatsResponse - Response for /api/v1/pods/stats
type PodStatsResponse struct {
	Total int        `json:"total"`
	Page  int        `json:"page"`
	Pods  []PodStats `json:"pods"`
}

// PodStats - Pod statistics
type PodStats struct {
	PodUID          string            `json:"pod_uid"`
	PodName         string            `json:"pod_name"`
	Namespace       string            `json:"namespace"`
	NodeName        string            `json:"node_name"`
	Status          string            `json:"status"`
	Phase           string            `json:"phase"`
	CreatedAt       time.Time         `json:"created_at"`
	AllocatedGPUs   int32             `json:"allocated_gpus"`
	AvgUtilization  float64           `json:"avg_utilization"`
	Running         bool              `json:"running"`
	OwnerUID        string            `json:"owner_uid,omitempty"`
	IP              string            `json:"ip,omitempty"`
}

// OwnerReference - Pod owner reference
type OwnerReference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	UID  string `json:"uid"`
}

// PodDetailResponse - Response for /api/v1/pods/:pod_uid
type PodDetailResponse struct {
	PodUID          string              `json:"pod_uid"`
	PodName         string              `json:"pod_name"`
	Namespace       string              `json:"namespace"`
	NodeName        string              `json:"node_name"`
	Status          string              `json:"status"`
	Phase           string              `json:"phase"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
	AllocatedGPUs   int32               `json:"allocated_gpus"`
	Running         bool                `json:"running"`
	Deleted         bool                `json:"deleted"`
	IP              string              `json:"ip,omitempty"`
	OwnerUID        string              `json:"owner_uid,omitempty"`
	CurrentMetrics  *PodGPUMetrics      `json:"current_metrics,omitempty"`
}

// PodGPUMetrics - GPU metrics for a Pod
type PodGPUMetrics struct {
	Timestamp       time.Time `json:"timestamp"`
	GPUUtilization  float64   `json:"gpu_utilization"`
	MemoryUsed      int32     `json:"memory_used_mb"`
	Power           float64   `json:"power_watts"`
	Temperature     float64   `json:"temperature_celsius"`
}

// PodGPUHistoryParams - Query parameters for /api/v1/pods/:pod_uid/gpu-history
type PodGPUHistoryParams struct {
	StartTime   string `form:"start_time"`
	EndTime     string `form:"end_time"`
	Hours       int    `form:"hours"` // Alternative: specify hours back from now
	Granularity string `form:"granularity" binding:"omitempty,oneof=minute hourly daily"`
}

// PodGPUHistoryResponse - Response for /api/v1/pods/:pod_uid/gpu-history
type PodGPUHistoryResponse struct {
	PodUID      string         `json:"pod_uid"`
	PodName     string         `json:"pod_name"`
	Granularity string         `json:"granularity"`
	DataPoints  []GPUDataPoint `json:"data_points"`
}

// GPUDataPoint - GPU metrics data point
type GPUDataPoint struct {
	Timestamp      time.Time `json:"timestamp"`
	GPUUtilization float64   `json:"gpu_utilization"`
	MemoryUsed     int32     `json:"memory_used_mb"`
	Power          float64   `json:"power_watts"`
	Temperature    float64   `json:"temperature_celsius"`
}

// PodEventsResponse - Response for /api/v1/pods/:pod_uid/events
type PodEventsResponse struct {
	PodUID string     `json:"pod_uid"`
	Events []PodEvent `json:"events"`
}

// PodEvent - Pod event
type PodEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Source    string    `json:"source"`
}

// PodComparisonParams - Query parameters for /api/v1/pods/comparison
type PodComparisonParams struct {
	PodUIDs []string `form:"pod_uids" binding:"required,min=2,max=10"`
	Metrics []string `form:"metrics"`
}

// PodComparisonResponse - Response for /api/v1/pods/comparison
type PodComparisonResponse struct {
	Pods       []PodComparisonItem `json:"pods"`
	Comparison ComparisonSummary   `json:"comparison"`
}

// PodComparisonItem - Pod comparison item
type PodComparisonItem struct {
	PodUID        string             `json:"pod_uid"`
	PodName       string             `json:"pod_name"`
	Namespace     string             `json:"namespace"`
	AllocatedGPUs int32              `json:"allocated_gpus"`
	Metrics       map[string]float64 `json:"metrics"`
}

// ComparisonSummary - Comparison summary
type ComparisonSummary struct {
	HighestUtilization string  `json:"highest_utilization_pod"`
	LowestUtilization  string  `json:"lowest_utilization_pod"`
	AvgUtilization     float64 `json:"avg_utilization"`
}

// getPodStats - GET /api/v1/pods/stats
func getPodStats(c *gin.Context) {
	var params PodStatsQueryParams
	if err := c.ShouldBindQuery(&params); err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithError(err))
		return
	}

	// Set defaults
	if params.Page == 0 {
		params.Page = 1
	}
	if params.PageSize == 0 {
		params.PageSize = 20
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(params.Cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get facade for the specified cluster
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	
	// Query pods with stats
	pods, total, err := queryPodsWithStats(c.Request.Context(), podFacade, params)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}

	response := PodStatsResponse{
		Total: total,
		Page:  params.Page,
		Pods:  pods,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// queryPodsWithStats - Query pods with statistics
func queryPodsWithStats(ctx context.Context, podFacade database.PodFacadeInterface, params PodStatsQueryParams) ([]PodStats, int, error) {
	// Query pods with filters using the enhanced facade method
	gpuPods, total, err := podFacade.QueryPodsWithFilters(
		ctx,
		params.Namespace,
		params.PodName,
		params.StartTime,
		params.EndTime,
		params.Page,
		params.PageSize,
	)
	if err != nil {
		return nil, 0, err
	}

	// Convert to response format
	pods := make([]PodStats, 0, len(gpuPods))
	for _, pod := range gpuPods {
		// Get average GPU utilization from gpu_device table
		avgUtil, _ := podFacade.GetAverageGPUUtilizationByNode(ctx, pod.NodeName)
		
		podStats := PodStats{
			PodUID:         pod.UID,
			PodName:        pod.Name,
			Namespace:      pod.Namespace,
			NodeName:       pod.NodeName,
			Status:         getStatusFromPhase(pod.Phase, pod.Running),
			Phase:          pod.Phase,
			CreatedAt:      pod.CreatedAt,
			AllocatedGPUs:  pod.GpuAllocated,
			AvgUtilization: avgUtil,
			Running:        pod.Running,
			OwnerUID:       pod.OwnerUID,
			IP:             pod.IP,
		}
		pods = append(pods, podStats)
	}

	return pods, int(total), nil
}

// getStatusFromPhase - Convert phase and running status to status string
func getStatusFromPhase(phase string, running bool) string {
	if running {
		return "Running"
	}
	switch phase {
	case "Pending":
		return "Pending"
	case "Succeeded":
		return "Succeeded"
	case "Failed":
		return "Failed"
	default:
		return "Unknown"
	}
}

// getPodDetail - GET /api/v1/pods/:pod_uid
func getPodDetail(c *gin.Context) {
	podUID := c.Param("pod_uid")
	if podUID == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("pod_uid is required"))
		return
	}

	cluster := c.Query("cluster")
	if cluster == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("cluster is required"))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get facade for the specified cluster
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	// Get pod detail
	pod, err := podFacade.GetGpuPodsByPodUid(c.Request.Context(), podUID)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}
	if pod == nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestDataNotExisted).WithMessage("Pod not found"))
		return
	}

	// Get current metrics
	metrics := getCurrentPodMetrics(c.Request.Context(), podFacade, pod.NodeName)

	response := PodDetailResponse{
		PodUID:         pod.UID,
		PodName:        pod.Name,
		Namespace:      pod.Namespace,
		NodeName:       pod.NodeName,
		Status:         getStatusFromPhase(pod.Phase, pod.Running),
		Phase:          pod.Phase,
		CreatedAt:      pod.CreatedAt,
		UpdatedAt:      pod.UpdatedAt,
		AllocatedGPUs:  pod.GpuAllocated,
		Running:        pod.Running,
		Deleted:        pod.Deleted,
		IP:             pod.IP,
		OwnerUID:       pod.OwnerUID,
		CurrentMetrics: metrics,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// getCurrentPodMetrics - Get current GPU metrics for a pod
func getCurrentPodMetrics(ctx context.Context, podFacade database.PodFacadeInterface, nodeName string) *PodGPUMetrics {
	device, err := podFacade.GetLatestGPUMetricsByNode(ctx, nodeName)
	if err != nil || device == nil {
		return nil
	}

	return &PodGPUMetrics{
		Timestamp:      device.UpdatedAt,
		GPUUtilization: device.Utilization,
		MemoryUsed:     device.Memory,
		Power:          device.Power,
		Temperature:    device.Temperature,
	}
}

// getPodGPUHistory - GET /api/v1/pods/:pod_uid/gpu-history
func getPodGPUHistory(c *gin.Context) {
	podUID := c.Param("pod_uid")
	if podUID == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("pod_uid is required"))
		return
	}

	var params PodGPUHistoryParams
	if err := c.ShouldBindQuery(&params); err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithError(err))
		return
	}

	// Default granularity
	if params.Granularity == "" {
		params.Granularity = "hourly"
	}

	cluster := c.Query("cluster")
	if cluster == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("cluster is required"))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get pod info first
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	pod, err := podFacade.GetGpuPodsByPodUid(c.Request.Context(), podUID)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}
	if pod == nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestDataNotExisted).WithMessage("Pod not found"))
		return
	}

	// Parse time range
	var startTime, endTime time.Time

	// Support two modes: hours or start_time/end_time
	if params.Hours > 0 {
		// Use hours parameter
		endTime = time.Now()
		startTime = endTime.Add(-time.Duration(params.Hours) * time.Hour)
	} else if params.StartTime != "" && params.EndTime != "" {
		// Use explicit start_time and end_time
		var parseErr error
		startTime, parseErr = time.Parse(time.RFC3339, params.StartTime)
		if parseErr != nil {
			_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("Invalid start_time format. Use RFC3339 format"))
			return
		}

		endTime, parseErr = time.Parse(time.RFC3339, params.EndTime)
		if parseErr != nil {
			_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("Invalid end_time format. Use RFC3339 format"))
			return
		}
	} else {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("Either 'hours' or both 'start_time' and 'end_time' are required"))
		return
	}

	// Query GPU history data
	dataPoints := queryGPUHistory(c.Request.Context(), podFacade, pod.NodeName, startTime, endTime, params.Granularity)

	response := PodGPUHistoryResponse{
		PodUID:      podUID,
		PodName:     pod.Name,
		Granularity: params.Granularity,
		DataPoints:  dataPoints,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// queryGPUHistory - Query GPU history data with aggregation
func queryGPUHistory(ctx context.Context, podFacade database.PodFacadeInterface, nodeName string, startTime, endTime time.Time, granularity string) []GPUDataPoint {
	// Query GPU device history using facade method
	devices, err := podFacade.QueryGPUHistoryByNode(ctx, nodeName, startTime, endTime)
	if err != nil || len(devices) == 0 {
		return []GPUDataPoint{}
	}

	// Convert to data points (simplified - no aggregation for now)
	dataPoints := make([]GPUDataPoint, 0, len(devices))
	for _, device := range devices {
		dataPoints = append(dataPoints, GPUDataPoint{
			Timestamp:      device.UpdatedAt,
			GPUUtilization: device.Utilization,
			MemoryUsed:     device.Memory,
			Power:          device.Power,
			Temperature:    device.Temperature,
		})
	}

	return dataPoints
}

// getPodEvents - GET /api/v1/pods/:pod_uid/events
func getPodEvents(c *gin.Context) {
	podUID := c.Param("pod_uid")
	if podUID == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("pod_uid is required"))
		return
	}

	cluster := c.Query("cluster")
	if cluster == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("cluster is required"))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get facade for the specified cluster
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()
	
	// Get pod events from database
	events := queryPodEvents(c.Request.Context(), podFacade, podUID)

	response := PodEventsResponse{
		PodUID: podUID,
		Events: events,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

// queryPodEvents - Query pod events
func queryPodEvents(ctx context.Context, podFacade database.PodFacadeInterface, podUID string) []PodEvent {
	// Query pod events using facade method
	gpuPodEvents, err := podFacade.ListPodEventsByUID(ctx, podUID)
	if err != nil || len(gpuPodEvents) == 0 {
		return []PodEvent{}
	}

	// Convert to event format
	events := make([]PodEvent, 0, len(gpuPodEvents))
	for _, event := range gpuPodEvents {
		events = append(events, PodEvent{
			Timestamp: event.CreatedAt,
			Type:      event.EventType,
			Reason:    event.PodPhase,
			Message:   fmt.Sprintf("Pod phase changed to %s, restart count: %d", event.PodPhase, event.RestartCount),
			Source:    "gpu-pods-event",
		})
	}

	return events
}

// comparePods - GET /api/v1/pods/comparison
func comparePods(c *gin.Context) {
	podUIDsParam := c.Query("pod_uids")
	if podUIDsParam == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("pod_uids is required"))
		return
	}

	cluster := c.Query("cluster")
	if cluster == "" {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("cluster is required"))
		return
	}

	// Parse pod UIDs (comma-separated)
	podUIDs := strings.Split(podUIDsParam, ",")
	if len(podUIDs) < 2 || len(podUIDs) > 10 {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestParameterInvalid).WithMessage("pod_uids must contain 2-10 UIDs"))
		return
	}

	// Get cluster clients
	cm := clientsets.GetClusterManager()
	clients, err := cm.GetClientSetByClusterName(cluster)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Get facade for the specified cluster
	podFacade := database.GetFacadeForCluster(clients.ClusterName).GetPod()

	// Query pods
	pods, err := podFacade.ListPodsByUids(c.Request.Context(), podUIDs)
	if err != nil {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.InternalError).WithError(err))
		return
	}

	if len(pods) == 0 {
		_ = c.Error(coreErrors.NewError().WithCode(coreErrors.RequestDataNotExisted).WithMessage("No pods found"))
		return
	}

	// Build comparison items
	comparisonItems := make([]PodComparisonItem, 0, len(pods))
	var totalUtil float64
	var highestUtil float64
	var lowestUtil float64 = 100.0
	var highestPod, lowestPod string

	for _, pod := range pods {
		// Get metrics for each pod
		avgUtil, _ := podFacade.GetAverageGPUUtilizationByNode(c.Request.Context(), pod.NodeName)
		
		metrics := map[string]float64{
			"gpu_utilization": avgUtil,
		}

		comparisonItems = append(comparisonItems, PodComparisonItem{
			PodUID:        pod.UID,
			PodName:       pod.Name,
			Namespace:     pod.Namespace,
			AllocatedGPUs: pod.GpuAllocated,
			Metrics:       metrics,
		})

		totalUtil += avgUtil
		if avgUtil > highestUtil {
			highestUtil = avgUtil
			highestPod = pod.Name
		}
		if avgUtil < lowestUtil {
			lowestUtil = avgUtil
			lowestPod = pod.Name
		}
	}

	summary := ComparisonSummary{
		HighestUtilization: highestPod,
		LowestUtilization:  lowestPod,
		AvgUtilization:     totalUtil / float64(len(pods)),
	}

	response := PodComparisonResponse{
		Pods:       comparisonItems,
		Comparison: summary,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, response))
}

