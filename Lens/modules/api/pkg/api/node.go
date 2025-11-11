package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/node"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/sliceUtil"
	"github.com/gin-gonic/gin"
)

func getClusterGpuAllocationInfo(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := "cluster:gpu:allocation_info"

	var result []model.GpuAllocation
	err = cacheFacade.Get(c, cacheKey, &result)
	if err == nil {
		// Cache hit
		c.JSON(http.StatusOK, rest.SuccessResp(c, result))
		return
	}

	// Cache miss, fallback to real-time calculation
	result, err = gpu.GetGpuNodesAllocation(c, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, rest.SuccessResp(c, result))
}

func getClusterGPUUtilization(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// Try to get from cache first
	cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
	cacheKey := "cluster:gpu:utilization"

	var result model.GPUUtilization
	err = cacheFacade.Get(c, cacheKey, &result)
	if err == nil {
		// Cache hit
		c.JSON(http.StatusOK, rest.SuccessResp(c, &result))
		return
	}

	// Cache miss, fallback to real-time calculation
	usage, err := gpu.CalculateGpuUsage(c, clients.StorageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	allocationRate, err := gpu.GetClusterGpuAllocationRate(c, clients.K8SClientSet, clients.ClusterName, metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	result = model.GPUUtilization{
		AllocationRate: allocationRate,
		Utilization:    usage,
	}
	c.JSON(http.StatusOK, rest.SuccessResp(c, &result))
}
func getGpuUsageHistory(c *gin.Context) {
	startStr := c.Query("start")
	endStr := c.Query("end")
	stepStr := c.DefaultQuery("step", "60") // Default is 60 seconds

	startUnix, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start timestamp"})
		return
	}
	endUnix, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end timestamp"})
		return
	}

	step, err := strconv.Atoi(stepStr)
	if err != nil || step <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid step value, must be positive integer (in seconds)"})
		return
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		c.Error(err)
		return
	}
	storageClient := clients.StorageClientSet

	// Try to get from cache first if querying recent data with default step
	if step == 60 {
		cacheFacade := database.GetFacadeForCluster(clients.ClusterName).GetGenericCache()
		cacheKey := getGpuUsageHistoryCacheKey(startTime, endTime)

		if cacheKey != "" {
			var result model.GpuUtilizationHistory
			err = cacheFacade.Get(c, cacheKey, &result)
			if err == nil {
				// Cache hit, filter data by time range if needed
				filteredResult := filterGpuUsageHistoryByTimeRange(result, startTime, endTime)
				c.JSON(http.StatusOK, rest.SuccessResp(c, filteredResult))
				return
			}
		}
	}

	// Cache miss or non-standard query, fallback to real-time calculation
	usageHistory, err := gpu.GetHistoryGpuUsage(c, storageClient, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		c.Error(err)
		return
	}
	allocationHistory, err := gpu.GetHistoryGpuAllocationRate(c, storageClient, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		c.Error(err)
		return
	}
	vramUtilizationHistory, err := gpu.GetNodeGpuVramUsageHistory(c, storageClient, metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		c.Error(err)
		return
	}
	result := model.GpuUtilizationHistory{
		AllocationRate:  allocationHistory,
		Utilization:     usageHistory,
		VramUtilization: vramUtilizationHistory,
	}

	c.JSON(http.StatusOK, rest.SuccessResp(c, result))
}

// getGpuUsageHistoryCacheKey determines the cache key based on the time range
// Returns empty string if the query doesn't match any cached time ranges
func getGpuUsageHistoryCacheKey(startTime, endTime time.Time) string {
	now := time.Now()
	duration := endTime.Sub(startTime)

	// Allow 5 minutes tolerance for "recent" queries
	tolerance := 5 * time.Minute
	timeSinceEnd := now.Sub(endTime)

	// Check if this is a recent query (end time is close to now)
	if timeSinceEnd > tolerance {
		return "" // Not a recent query, don't use cache
	}

	// Match against cached durations
	if duration >= 50*time.Minute && duration <= 70*time.Minute {
		// ~1 hour query
		return "cluster:gpu:usage_history:1h"
	} else if duration >= 5*time.Hour+30*time.Minute && duration <= 6*time.Hour+30*time.Minute {
		// ~6 hour query
		return "cluster:gpu:usage_history:6h"
	} else if duration >= 23*time.Hour && duration <= 25*time.Hour {
		// ~24 hour query
		return "cluster:gpu:usage_history:24h"
	}

	return "" // Duration doesn't match any cached ranges
}

// filterGpuUsageHistoryByTimeRange filters history data to match the requested time range
func filterGpuUsageHistoryByTimeRange(history model.GpuUtilizationHistory, startTime, endTime time.Time) model.GpuUtilizationHistory {
	startUnix := startTime.Unix()
	endUnix := endTime.Unix()

	return model.GpuUtilizationHistory{
		AllocationRate:  filterTimePoints(history.AllocationRate, startUnix, endUnix),
		Utilization:     filterTimePoints(history.Utilization, startUnix, endUnix),
		VramUtilization: filterTimePoints(history.VramUtilization, startUnix, endUnix),
	}
}

// filterTimePoints filters time points within the given time range
func filterTimePoints(points []model.TimePoint, startUnix, endUnix int64) []model.TimePoint {
	filtered := make([]model.TimePoint, 0, len(points))
	for _, point := range points {
		if point.Timestamp >= startUnix && point.Timestamp <= endUnix {
			filtered = append(filtered, point)
		}
	}
	return filtered
}

func getGPUNodeList(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	page := &model.SearchGpuNodeReq{}
	err = ctx.BindQuery(page)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter := page.ToNodeFilter()

	dbNodes, total, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().SearchNode(ctx, filter)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, struct {
		Data  []model.GPUNode `json:"data"`
		Total int             `json:"total"`
	}{
		Data:  batchCvtDbNode2GpuNodeListNode(dbNodes),
		Total: total,
	}))
}

func getNodeWorkload(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	_ = clients // Cluster validation, may be used in future

	nodeName := ctx.Param("name")
	page := &rest.Page{}
	err = ctx.BindQuery(page)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	workloads, err := workload.GetRunningTopLevelGpuWorkloadByNode(ctx, clients.ClusterName, nodeName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	workloadsResult, _, count, _ := sliceUtil.PaginateSlice(workloads, page.PageNum, page.PageSize)
	nodeViews, err := batchCvtDBWorkload2TopLevelGpuResource(ctx, clients.ClusterName, workloadsResult, nodeName)
	if err != nil {
		_ = ctx.Error(err)

	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, struct {
		Data  []model.WorkloadNodeView `json:"data"`
		Total int                      `json:"total"`
	}{
		Data:  nodeViews,
		Total: count,
	}))
}

func getNodeWorkloadHistory(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	nodeName := ctx.Param("name")
	page := &rest.Page{}
	err = ctx.BindQuery(page)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pods, count, err := database.GetFacadeForCluster(clients.ClusterName).GetPod().GetHistoryGpuPodByNodeName(ctx, nodeName, page.PageNum, page.PageSize)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	uids := []string{}
	for _, pod := range pods {
		uids = append(uids, pod.UID)
	}
	workloadMap := map[string]*dbModel.GpuWorkload{}
	workloads, err := workload.GetTopLevelWorkloadsByPods(ctx, clients.ClusterName, pods)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	for i := range workloads {
		gpuWorkload := workloads[i]
		workloadMap[gpuWorkload.UID] = gpuWorkload
	}
	references := map[string]string{}
	refs, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().ListWorkloadPodReferencesByPodUids(ctx, uids)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	for _, ref := range refs {
		references[ref.PodUID] = ref.WorkloadUID
	}
	result := []model.WorkloadHistoryNodeView{}
	for _, pod := range pods {
		workloadUid := references[pod.UID]
		gpuWorkload := workloadMap[workloadUid]
		view := model.WorkloadHistoryNodeView{
			Kind:         gpuWorkload.Kind,
			Name:         gpuWorkload.Name,
			Namespace:    gpuWorkload.Namespace,
			Uid:          gpuWorkload.UID,
			GpuAllocated: int(pod.GpuAllocated),
			PodName:      pod.Name,
			PodNamespace: pod.Namespace,
			StartTime:    pod.CreatedAt.Unix(),
			EndTime:      pod.UpdatedAt.Unix(),
		}
		result = append(result, view)
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, struct {
		Data  []model.WorkloadHistoryNodeView `json:"data"`
		Total int                             `json:"total"`
	}{
		Data:  result,
		Total: count,
	}))

}

func batchCvtDBWorkload2TopLevelGpuResource(ctx context.Context, clusterName string, dbWorkloads []*dbModel.GpuWorkload, nodeName string) ([]model.WorkloadNodeView, error) {
	result := []model.WorkloadNodeView{}
	for _, w := range dbWorkloads {
		nodeView, err := cvtDBWorkload2TopLevelGpuResource(ctx, clusterName, w, nodeName)
		if err != nil {
			return nil, err
		}
		result = append(result, nodeView)
	}
	return result, nil
}

func cvtDBWorkload2TopLevelGpuResource(ctx context.Context, clusterName string, dbWorkload *dbModel.GpuWorkload, nodeName string) (model.WorkloadNodeView, error) {
	pods, err := workload.GetActivePodsByWorkloadUid(ctx, clusterName, dbWorkload.UID)
	if err != nil {
		return model.WorkloadNodeView{}, err
	}
	result := model.WorkloadNodeView{
		Kind:             dbWorkload.Kind,
		Name:             dbWorkload.Name,
		Namespace:        dbWorkload.Namespace,
		Uid:              dbWorkload.UID,
		GpuAllocated:     0,
		GpuAllocatedNode: 0,
		NodeName:         nodeName,
		Status:           "Running",
	}
	for _, pod := range pods {
		if pod.NodeName == nodeName {
			result.GpuAllocatedNode += int(pod.GpuAllocated)
		}
		result.GpuAllocated += int(pod.GpuAllocated)
	}
	return result, nil
}

func getNodeGpuMetrics(ctx *gin.Context) {
	nodeName := ctx.Param("name")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")
	stepStr := ctx.DefaultQuery("step", "60") // Default is 60 seconds

	startUnix, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid start timestamp"})
		return
	}
	endUnix, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid end timestamp"})
		return
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	step, err := strconv.Atoi(stepStr)
	if err != nil || step <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid step value, must be positive integer (in seconds)"})
		return
	}

	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	storageClient := clients.StorageClientSet

	gpuUtil, err := node.GetNodeGpuUtilHistory(ctx,
		storageClient,
		metadata.GpuVendorAMD,
		nodeName,
		startTime,
		endTime,
		step)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithError(err).
			WithMessage("Fail GetNodeGpuAllocationHistory"))
		return
	}
	gpuAllocationRate, err := node.GetNodeGpuAllocationHistory(ctx,
		storageClient,
		metadata.GpuVendorAMD,
		nodeName,
		startTime,
		endTime,
		step)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithError(err).
			WithMessage("Fail GetNodeGpuAllocationHistory"))
		return
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, struct {
		GpuUtilization    model.MetricsGraph `json:"gpu_utilization"`
		GpuAllocationRate model.MetricsGraph `json:"gpu_allocation_rate"`
	}{
		GpuUtilization: model.MetricsGraph{
			Series: gpuUtil,
			Config: model.MetricsGraphConfig{
				YAxisUnit: "%",
			},
		},
		GpuAllocationRate: model.MetricsGraph{
			Series: gpuAllocationRate,
			Config: model.MetricsGraphConfig{
				YAxisUnit: "%",
			},
		},
	}))
}

func getNodeInfoByName(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	nodeName := ctx.Param("name")
	dbNode, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, nodeName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	if dbNode == nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
		return
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, cvtDbNode2GpuNodeDetail(dbNode)))
}

func cvtDbNode2GpuNodeDetail(dbNode *dbModel.Node) model.GpuNodeDetail {
	return model.GpuNodeDetail{
		Name:              dbNode.Name,
		Health:            dbNode.Status,
		Cpu:               fmt.Sprintf("%d X %s", dbNode.CPUCount, dbNode.CPU),
		Memory:            dbNode.Memory,
		OS:                dbNode.Os,
		StaticGpuDetails:  fmt.Sprintf("%d X %s", dbNode.GpuCount, dbNode.GpuName),
		KubeletVersion:    dbNode.KubeletVersion,
		ContainerdVersion: dbNode.ContainerdVersion,
		GPUDriverVersion:  dbNode.DriverVersion,
	}
}

func batchCvtDbNode2GpuNodeListNode(dbNodes []*dbModel.Node) []model.GPUNode {
	results := []model.GPUNode{}
	for _, dbNode := range dbNodes {
		results = append(results, cvtDbNode2GpuNodeListNode(dbNode))
	}
	return results
}

func cvtDbNode2GpuNodeListNode(dbNode *dbModel.Node) model.GPUNode {
	return model.GPUNode{
		Name:           dbNode.Name,
		Ip:             dbNode.Address,
		GpuName:        dbNode.GpuName,
		GpuCount:       int(dbNode.GpuCount),
		GpuAllocation:  int(dbNode.GpuAllocation),
		GpuUtilization: dbNode.GpuUtilization,
		Status:         dbNode.Status,
		StatusColor:    node.GetStatusColor(dbNode.Status),
	}

}

// getNodeUtilization retrieves the current utilization of a node
func getNodeUtilization(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	nodeName := ctx.Param("name")

	// Get node from database to get GPU info
	dbNode, err := database.GetFacadeForCluster(clients.ClusterName).GetNode().GetNodeByName(ctx, nodeName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	if dbNode == nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
		return
	}

	// Get current utilization from recent metrics (last 1 minute average)
	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	// Get CPU utilization
	cpuUtil, err := node.GetNodeCpuUtilHistory(ctx, clients.StorageClientSet, nodeName, oneMinuteAgo, now, 60)
	cpuUtilValue := 0.0
	if err == nil && len(cpuUtil) > 0 && len(cpuUtil[0].Values) > 0 {
		cpuUtilValue = cpuUtil[0].Values[len(cpuUtil[0].Values)-1].Value
	}

	// Get Memory utilization
	memUtil, err := node.GetNodeMemUtilHistory(ctx, clients.StorageClientSet, nodeName, oneMinuteAgo, now, 60)
	memUtilValue := 0.0
	if err == nil && len(memUtil) > 0 && len(memUtil[0].Values) > 0 {
		memUtilValue = memUtil[0].Values[len(memUtil[0].Values)-1].Value
	}

	result := model.NodeUtilization{
		NodeName:       dbNode.Name,
		CpuUtilization: cpuUtilValue,
		MemUtilization: memUtilValue,
		GpuUtilization: dbNode.GpuUtilization,
		GpuAllocation:  int(dbNode.GpuAllocation),
		Timestamp:      time.Now().Unix(),
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, result))
}

// getNodeUtilizationHistory retrieves the utilization history of a node
func getNodeUtilizationHistory(ctx *gin.Context) {
	nodeName := ctx.Param("name")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")
	stepStr := ctx.DefaultQuery("step", "60") // Default is 60 seconds

	startUnix, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid start timestamp"})
		return
	}
	endUnix, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid end timestamp"})
		return
	}

	step, err := strconv.Atoi(stepStr)
	if err != nil || step <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid step value, must be positive integer (in seconds)"})
		return
	}

	startTime := time.Unix(startUnix, 0)
	endTime := time.Unix(endUnix, 0)

	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	storageClient := clients.StorageClientSet

	// Get CPU utilization history
	cpuUtil, err := node.GetNodeCpuUtilHistory(ctx,
		storageClient,
		nodeName,
		startTime,
		endTime,
		step)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithError(err).
			WithMessage("Fail GetNodeCpuUtilHistory"))
		return
	}

	// Get Memory utilization history
	memUtil, err := node.GetNodeMemUtilHistory(ctx,
		storageClient,
		nodeName,
		startTime,
		endTime,
		step)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithError(err).
			WithMessage("Fail GetNodeMemUtilHistory"))
		return
	}

	// Get GPU utilization history
	gpuUtil, err := node.GetNodeGpuUtilHistory(ctx,
		storageClient,
		metadata.GpuVendorAMD,
		nodeName,
		startTime,
		endTime,
		step)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithError(err).
			WithMessage("Fail GetNodeGpuUtilHistory"))
		return
	}

	// Get GPU allocation rate history
	gpuAllocationRate, err := node.GetNodeGpuAllocationHistory(ctx,
		storageClient,
		metadata.GpuVendorAMD,
		nodeName,
		startTime,
		endTime,
		step)
	if err != nil {
		_ = ctx.Error(errors.NewError().
			WithCode(errors.InternalError).
			WithError(err).
			WithMessage("Fail GetNodeGpuAllocationHistory"))
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, struct {
		CpuUtilization    model.MetricsGraph `json:"cpu_utilization"`
		MemUtilization    model.MetricsGraph `json:"mem_utilization"`
		GpuUtilization    model.MetricsGraph `json:"gpu_utilization"`
		GpuAllocationRate model.MetricsGraph `json:"gpu_allocation_rate"`
	}{
		CpuUtilization: model.MetricsGraph{
			Series: cpuUtil,
			Config: model.MetricsGraphConfig{
				YAxisUnit: "%",
			},
		},
		MemUtilization: model.MetricsGraph{
			Series: memUtil,
			Config: model.MetricsGraphConfig{
				YAxisUnit: "%",
			},
		},
		GpuUtilization: model.MetricsGraph{
			Series: gpuUtil,
			Config: model.MetricsGraphConfig{
				YAxisUnit: "%",
			},
		},
		GpuAllocationRate: model.MetricsGraph{
			Series: gpuAllocationRate,
			Config: model.MetricsGraphConfig{
				YAxisUnit: "%",
			},
		},
	}))
}
