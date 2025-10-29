package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbModel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/errors"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/node"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/model/rest"
	"github.com/AMD-AGI/primus-lens/core/pkg/utils/sliceUtil"
	"github.com/gin-gonic/gin"
)

func getClusterGpuAllocationInfo(c *gin.Context) {
	result, err := gpu.GetGpuNodesAllocation(c, clientsets.GetCurrentClusterK8SClientSet(), metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	c.JSON(http.StatusOK, rest.SuccessResp(c, result))
}

func getClusterGPUUtilization(c *gin.Context) {
	usage, err := gpu.CalculateGpuUsage(c, clientsets.GetCurrentClusterStorageClientSet(), metadata.GpuVendorAMD)
	if err != nil {
		_ = c.Error(err)
		return
	}
	allocationRate, err := gpu.GetClusterGpuAllocationRate(c, clientsets.GetCurrentClusterK8SClientSet(), metadata.GpuVendorAMD)
	result := &model.GPUUtilization{
		AllocationRate: allocationRate,
		Utilization:    usage,
	}
	c.JSON(http.StatusOK, rest.SuccessResp(c, result))
}
func getGpuUsageHistory(c *gin.Context) {
	startStr := c.Query("start")
	endStr := c.Query("end")
	stepStr := c.DefaultQuery("step", "60") // 默认为60秒

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

	usageHistory, err := gpu.GetHistoryGpuUsage(c, clientsets.GetCurrentClusterStorageClientSet(), metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		c.Error(err)
		return
	}
	allocationHistory, err := gpu.GetHistoryGpuAllocationRate(c, clientsets.GetCurrentClusterStorageClientSet(), metadata.GpuVendorAMD, startTime, endTime, step)
	if err != nil {
		c.Error(err)
		return
	}
	vramUtilizationHistory, err := gpu.GetNodeGpuVramUsageHistory(c, clientsets.GetCurrentClusterStorageClientSet(), metadata.GpuVendorAMD, startTime, endTime, step)
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

func getGPUNodeList(ctx *gin.Context) {
	page := &model.SearchGpuNodeReq{}
	err := ctx.BindQuery(page)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	filter := page.ToNodeFilter()

	dbNodes, total, err := database.SearchNode(ctx, filter)
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
	nodeName := ctx.Param("name")
	page := &rest.Page{}
	err := ctx.BindQuery(page)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	workloads, err := workload.GetRunningTopLevelGpuWorkloadByNode(ctx, nodeName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	workloadsResult, _, count, _ := sliceUtil.PaginateSlice(workloads, page.PageNum, page.PageSize)
	nodeViews, err := batchCvtDBWorkload2TopLevelGpuResource(ctx, workloadsResult, nodeName)
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
	nodeName := ctx.Param("name")
	page := &rest.Page{}
	err := ctx.BindQuery(page)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	pods, count, err := database.GetHistoryGpuPodByNodeName(ctx, nodeName, page.PageNum, page.PageSize)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	uids := []string{}
	for _, pod := range pods {
		uids = append(uids, pod.UID)
	}
	workloadMap := map[string]*dbModel.GpuWorkload{}
	workloads, err := workload.GetTopLevelWorkloadsByPods(ctx, pods)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	for i := range workloads {
		gpuWorkload := workloads[i]
		workloadMap[gpuWorkload.UID] = gpuWorkload
	}
	references := map[string]string{}
	refs, err := database.ListWorkloadPodReferencesByPodUids(ctx, uids)
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

func batchCvtDBWorkload2TopLevelGpuResource(ctx context.Context, dbWorkloads []*dbModel.GpuWorkload, nodeName string) ([]model.WorkloadNodeView, error) {
	result := []model.WorkloadNodeView{}
	for _, w := range dbWorkloads {
		nodeView, err := cvtDBWorkload2TopLevelGpuResource(ctx, w, nodeName)
		if err != nil {
			return nil, err
		}
		result = append(result, nodeView)
	}
	return result, nil
}

func cvtDBWorkload2TopLevelGpuResource(ctx context.Context, dbWorkload *dbModel.GpuWorkload, nodeName string) (model.WorkloadNodeView, error) {
	pods, err := workload.GetActivePodsByWorkloadUid(ctx, dbWorkload.UID)
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
	stepStr := ctx.DefaultQuery("step", "60") // 默认为60秒

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
	gpuUtil, err := node.GetNodeGpuUtilHistory(ctx,
		clientsets.GetCurrentClusterStorageClientSet(),
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
		clientsets.GetCurrentClusterStorageClientSet(),
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
	nodeName := ctx.Param("name")
	dbNode, err := database.GetNodeByName(ctx, nodeName)
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
