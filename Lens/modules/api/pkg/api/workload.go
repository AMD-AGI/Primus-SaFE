package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/constant"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/filter"
	dbModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/errors"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/workload"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model/rest"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/sliceUtil"
	"github.com/gin-gonic/gin"
)

func getConsumerInfo(c *gin.Context) {
	cm := clientsets.GetClusterManager()
	page := &rest.Page{}
	err := c.BindQuery(page)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	clusterName := c.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = c.Error(err)
		return
	}
	runningWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().ListRunningWorkload(c)
	if err != nil {
		_ = c.Error(err)
		return
	}
	result := []model.TopLevelGpuResource{}
	for _, dbWorkload := range runningWorkload {
		r := model.TopLevelGpuResource{
			Kind:      dbWorkload.Kind,
			Name:      dbWorkload.Name,
			Namespace: dbWorkload.Namespace,
			Uid:       dbWorkload.UID,
			Stat: model.GpuStat{
				GpuRequest:     int(dbWorkload.GpuRequest),
				GpuUtilization: 0,
			},
			Pods:   nil,
			Source: getSource(dbWorkload),
		}
		cm := clientsets.GetClusterManager()
		// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
		clusterName := c.Query("cluster")
		clients, err2 := cm.GetClusterClientsOrDefault(clusterName)
		if err2 != nil {
			// If failed to get cluster, fall back to current cluster
			clients = cm.GetCurrentClusterClients()
		}
		storageClient := clients.StorageClientSet
		r.Stat.GpuUtilization, _ = workload.GetCurrentWorkloadGpuUtilization(c, dbWorkload.UID, storageClient)
		result = append(result, r)
	}
	data, _, total, _ := sliceUtil.PaginateSlice(result, page.PageNum, page.PageSize)
	c.JSON(http.StatusOK, rest.SuccessResp(c, struct {
		Data  []model.TopLevelGpuResource `json:"data"`
		Total int                         `json:"total"`
	}{
		Data:  data,
		Total: total,
	}))
}

func listWorkloads(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	page := &model.SearchWorkloadReq{}
	err = ctx.BindQuery(page)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	f := &filter.WorkloadFilter{
		Limit:  page.PageSize,
		Offset: (page.PageNum - 1) * page.PageSize,
	}
	if page.Name != "" {
		f.Name = &page.Name
	}
	if page.Kind != "" {
		f.Kind = &page.Kind
	}
	if page.Namespace != "" {
		f.Namespace = &page.Namespace
	}
	if page.Status != "" {
		f.Status = &page.Status
	}
	if page.OrderBy != "" {
		switch page.OrderBy {
		case "start_at":
			f.OrderBy = "created_at"
		case "end_at":
			f.OrderBy = "end_at"
		}
	}
	workloads, count, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	result := []model.WorkloadListItem{}
	for _, w := range workloads {
		item, _ := cvtDBWorkloadListItem(ctx, clients.ClusterName, w)
		result = append(result, item)
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, struct {
		Data  []model.WorkloadListItem `json:"data"`
		Total int                      `json:"total"`
	}{
		Data:  result,
		Total: count,
	}))
}

func getWorkloadsMetadata(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	namespaces, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetWorkloadsNamespaceList(ctx)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	kinds, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetWorkloadKindList(ctx)
	if err != nil {
		_ = ctx.Error(err)

	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, struct {
		Namespaces []string `json:"namespaces"`
		Kinds      []string `json:"kinds"`
	}{
		Namespaces: namespaces,
		Kinds:      kinds,
	}))
}

func cvtDBWorkloadListItem(ctx context.Context, clusterName string, dbWorkload *dbModel.GpuWorkload) (model.WorkloadListItem, error) {
	result := model.WorkloadListItem{
		Kind:         dbWorkload.Kind,
		Name:         dbWorkload.Name,
		Namespace:    dbWorkload.Namespace,
		Uid:          dbWorkload.UID,
		GpuAllocated: 8,
		GpuAllocation: model.GpuAllocationInfo{
			"AMD_Instinct_MI300X_OAM": 8,
		},
		Status:      dbWorkload.Status,
		StatusColor: metadata.GetWorkloadStatusColor(dbWorkload.Status),
		StartAt:     dbWorkload.CreatedAt.Unix(),
		EndAt:       dbWorkload.UpdatedAt.Unix(),
		Source:      getSource(dbWorkload),
	}
	gpuAllocation, err := workload.GetWorkloadResource(ctx, clusterName, dbWorkload.UID)
	if err != nil {
		log.Errorf("Failed to get gpu allocation info: %v", err)
	} else {
		result.GpuAllocation = gpuAllocation
	}
	gpuAllocated, err := workload.GetWorkloadGpuAllocatedCount(ctx, clusterName, dbWorkload.UID)
	if err != nil {
		log.Errorf("Failed to get gpu allocated count: %v", err)
	} else {
		result.GpuAllocated = gpuAllocated
	}

	// 获取workload统计信息
	statistic, err := database.GetFacadeForCluster(clusterName).GetWorkloadStatistic().GetByUID(ctx, clusterName, dbWorkload.UID)
	if err != nil {
		log.Errorf("Failed to get workload statistic: %v", err)
	} else if statistic != nil {
		// 设置历史统计数据
		result.AvgGpuUtilization = statistic.AvgGpuUtilization
		result.P50GpuUtilization = statistic.P50GpuUtilization
		result.P90GpuUtilization = statistic.P90GpuUtilization
		result.P95GpuUtilization = statistic.P95GpuUtilization

		// 如果workload是running状态，设置当前GPU使用率，否则为nil
		if dbWorkload.Status == metadata.WorkloadStatusRunning {
			result.InstantGpuUtilization = &statistic.InstantGpuUtilization
		} else {
			result.InstantGpuUtilization = nil
		}
	}

	return result, nil
}

func getWorkloadHierarchy(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	uid := ctx.Param("uid")
	rootWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetGpuWorkloadByUid(ctx, uid)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	if rootWorkload == nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
		return
	}
	tree, err := buildHierarchy(ctx, clients.ClusterName, uid)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, tree))
}

func getWorkloadHierarchyByKindName(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	// Get filter parameters from query
	kind := ctx.Query("kind")
	name := ctx.Query("name")
	namespace := ctx.Query("namespace")

	// Validate required parameters
	if kind == "" || name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "kind and name are required"})
		return
	}

	// Build filter
	f := &filter.WorkloadFilter{
		Kind: &kind,
		Name: &name,
	}
	if namespace != "" {
		f.Namespace = &namespace
	}

	// Query workload by kind and name
	workloads, _, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().QueryWorkload(ctx, f)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	if len(workloads) == 0 {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
		return
	}

	// If multiple workloads found (same kind+name in different namespaces), return the first one
	// Or if namespace is specified, it should be unique
	rootWorkload := workloads[0]

	// Build hierarchy tree
	tree, err := buildHierarchy(ctx, clients.ClusterName, rootWorkload.UID)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, tree))
}

func buildHierarchy(ctx context.Context, clusterName string, uid string) (*model.WorkloadHierarchyItem, error) {
	workload, err := database.GetFacadeForCluster(clusterName).GetWorkload().GetGpuWorkloadByUid(ctx, uid)
	if err != nil {
		return nil, err
	}
	if workload == nil {
		return nil, nil
	}

	node := &model.WorkloadHierarchyItem{
		Kind:      workload.Kind,
		Name:      workload.Name,
		Namespace: workload.Namespace,
		Uid:       workload.UID,
		Children:  []model.WorkloadHierarchyItem{},
	}

	children, _, err := database.GetFacadeForCluster(clusterName).GetWorkload().QueryWorkload(ctx, &filter.WorkloadFilter{
		ParentUid: &uid,
	})
	if err != nil {
		return nil, err
	}

	for _, child := range children {
		childNode, err := buildHierarchy(ctx, clusterName, child.UID)
		if err != nil {
			return nil, err
		}
		if childNode != nil {
			node.Children = append(node.Children, *childNode)
		}
	}

	return node, nil
}

func getWorkloadInfo(ctx *gin.Context) {
	cm := clientsets.GetClusterManager()
	// Get cluster name from query parameter, priority: specified cluster > default cluster > current cluster
	clusterName := ctx.Query("cluster")
	clients, err := cm.GetClusterClientsOrDefault(clusterName)
	if err != nil {
		_ = ctx.Error(err)
		return
	}

	uid := ctx.Param("uid")
	dbWorkload, err := database.GetFacadeForCluster(clients.ClusterName).GetWorkload().GetGpuWorkloadByUid(ctx, uid)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	if dbWorkload == nil {
		_ = ctx.Error(errors.NewError().WithCode(errors.RequestDataNotExisted))
		return
	}
	workloadInfo := &model.WorkloadInfo{
		ApiVersion:    dbWorkload.GroupVersion,
		Kind:          dbWorkload.Kind,
		Name:          dbWorkload.Name,
		Namespace:     dbWorkload.Namespace,
		Uid:           dbWorkload.UID,
		GpuAllocation: nil,
		Pods:          []model.WorkloadInfoPod{},
		StartTime:     dbWorkload.CreatedAt.Unix(),
		EndTime:       dbWorkload.EndAt.Unix(),
		Source:        getSource(dbWorkload),
	}
	if dbWorkload.EndAt.Unix() < int64(8*time.Hour) {
		workloadInfo.EndTime = 0
	}
	pods, err := workload.GetWorkloadPods(ctx, clients.ClusterName, dbWorkload.UID)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	for _, pod := range pods {
		workloadInfo.Pods = append(workloadInfo.Pods, model.WorkloadInfoPod{
			NodeName:     pod.NodeName,
			PodNamespace: pod.Namespace,
			PodName:      pod.Name,
		})
	}
	gpuAllocation, err := workload.GetWorkloadResource(ctx, clients.ClusterName, dbWorkload.UID)
	if err != nil {
		_ = ctx.Error(err)

	}
	workloadInfo.GpuAllocation = gpuAllocation

	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, workloadInfo))

}

func getWorkloadMetrics(ctx *gin.Context) {
	uid := ctx.Param("uid")
	startStr := ctx.Query("start")
	endStr := ctx.Query("end")
	stepStr := ctx.DefaultQuery("step", "60")

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

	result := map[string]model.MetricsGraph{}
	gpuUtil, err := workload.GetWorkloadGpuUtilMetrics(ctx, uid, startTime, endTime, step, storageClient)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	gpuUtil.Serial = 1
	gpuMemUtil, err := workload.GetWorkloadGpuMemoryUtilMetrics(ctx, uid, startTime, endTime, step, storageClient)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	gpuMemUtil.Serial = 2
	powerUtil, err := workload.GetWorkloadGpuPowerMetrics(ctx, uid, startTime, endTime, step, storageClient)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	powerUtil.Serial = 3
	tflopsMetrics, err := workload.GetTFLOPSMetrics(ctx, uid, startTime, endTime, step, storageClient)
	if err != nil {
		_ = ctx.Error(err)
		return
	}
	result["GPU Utilization"] = *gpuUtil
	result["GPU Memory Utilization"] = *gpuMemUtil
	result["GPU Power"] = *powerUtil
	if tflopsMetrics != nil {
		tflopsMetrics.Serial = 4
		result["TrainingPerformance"] = *tflopsMetrics
	}
	ctx.JSON(http.StatusOK, rest.SuccessResp(ctx, result))
}

func getSource(dbWorkload *dbModel.GpuWorkload) string {
	if dbWorkload.Source == "" {
		return constant.ContainerSourceK8S
	}
	return dbWorkload.Source
}
