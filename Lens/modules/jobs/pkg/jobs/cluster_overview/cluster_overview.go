package cluster_overview

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/rdma"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/trace"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/utils/k8sUtil"
	"github.com/AMD-AGI/Primus-SaFE/Lens/modules/jobs/pkg/common"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

type ClusterOverviewJob struct {
}

// Run executes the cluster overview caching job
func (j *ClusterOverviewJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	// Create main trace span
	span, ctx := trace.StartSpanFromContext(ctx, "cluster_overview_job.Run")
	defer trace.FinishSpan(span)

	// Record total job start time
	jobStartTime := time.Now()

	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	span.SetAttributes(
		attribute.String("job.name", "cluster_overview"),
		attribute.String("cluster.name", clusterName),
	)

	// Initialize cache object
	cache := &dbmodel.ClusterOverviewCache{
		ClusterName: clusterName,
	}

	// 1. Get GPU nodes
	gpuNodesSpan, gpuNodesCtx := trace.StartSpanFromContext(ctx, "getGpuNodes")
	gpuNodesSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	queryStart := time.Now()
	gpuNodes, err := gpu.GetGpuNodes(gpuNodesCtx, clientSets, metadata.GpuVendorAMD)
	if err != nil {
		gpuNodesSpan.RecordError(err)
		gpuNodesSpan.SetAttributes(attribute.String("error.message", err.Error()))
		gpuNodesSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(gpuNodesSpan)

		log.Errorf("Failed to get GPU nodes: %v", err)
		span.SetStatus(codes.Error, "Failed to get GPU nodes")
		return stats, err
	}

	duration := time.Since(queryStart)
	gpuNodesSpan.SetAttributes(
		attribute.Int("nodes.count", len(gpuNodes)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	gpuNodesSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(gpuNodesSpan)

	stats.QueryDuration += duration.Seconds()
	cache.TotalNodes = int32(len(gpuNodes))

	// 2. Get faulty nodes from database
	faultyNodesSpan, faultyNodesCtx := trace.StartSpanFromContext(ctx, "getFaultyNodesFromDB")
	faultyNodesSpan.SetAttributes(attribute.Int("nodes.input_count", len(gpuNodes)))

	step2Start := time.Now()
	faultyNodes, err := j.getFaultyNodesFromDB(faultyNodesCtx, gpuNodes)
	if err != nil {
		faultyNodesSpan.RecordError(err)
		faultyNodesSpan.SetAttributes(attribute.String("error.message", err.Error()))
		faultyNodesSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(faultyNodesSpan)

		log.Errorf("Failed to get faulty nodes from database: %v", err)
		span.SetStatus(codes.Error, "Failed to get faulty nodes")
		return stats, err
	}

	duration = time.Since(step2Start)
	cache.FaultyNodes = int32(len(faultyNodes))
	cache.HealthyNodes = cache.TotalNodes - cache.FaultyNodes

	faultyNodesSpan.SetAttributes(
		attribute.Int("nodes.faulty_count", len(faultyNodes)),
		attribute.Int("nodes.healthy_count", int(cache.HealthyNodes)),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	faultyNodesSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(faultyNodesSpan)

	// 3. Get GPU node idle info
	idleInfoSpan, idleInfoCtx := trace.StartSpanFromContext(ctx, "getGpuNodeIdleInfo")
	idleInfoSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	step3Start := time.Now()
	idle, partialIdle, busy, err := gpu.GetGpuNodeIdleInfoFromDB(idleInfoCtx, database.GetFacade().GetPod(), database.GetFacade().GetNode())
	if err != nil {
		idleInfoSpan.RecordError(err)
		idleInfoSpan.SetAttributes(attribute.String("error.message", err.Error()))
		idleInfoSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(idleInfoSpan)

		log.Errorf("Failed to get GPU node idle info: %v", err)
		span.SetStatus(codes.Error, "Failed to get GPU node idle info")
		return stats, err
	}

	duration = time.Since(step3Start)
	cache.FullyIdleNodes = int32(idle)
	cache.PartiallyIdleNodes = int32(partialIdle)
	cache.BusyNodes = int32(busy)

	idleInfoSpan.SetAttributes(
		attribute.Int("nodes.fully_idle", idle),
		attribute.Int("nodes.partially_idle", partialIdle),
		attribute.Int("nodes.busy", busy),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	idleInfoSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(idleInfoSpan)

	// 4. Calculate GPU usage
	usageSpan, usageCtx := trace.StartSpanFromContext(ctx, "calculateGpuUsage")
	usageSpan.SetAttributes(attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)))

	step4Start := time.Now()
	usage, err := gpu.CalculateGpuUsage(usageCtx, storageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		usageSpan.RecordError(err)
		usageSpan.SetAttributes(attribute.String("error.message", err.Error()))
		usageSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(usageSpan)

		log.Errorf("Failed to calculate GPU usage: %v", err)
		// Don't fail the entire job for this error
		usage = 0
	} else {
		duration = time.Since(step4Start)
		usageSpan.SetAttributes(
			attribute.Float64("gpu.utilization", usage),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		usageSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(usageSpan)
	}
	cache.Utilization = usage

	// 5. Get cluster GPU allocation rate
	allocationSpan, allocationCtx := trace.StartSpanFromContext(ctx, "getClusterGpuAllocationRate")
	allocationSpan.SetAttributes(
		attribute.String("cluster.name", clusterName),
		attribute.String("gpu.vendor", string(metadata.GpuVendorAMD)),
	)

	step5Start := time.Now()
	allocationRate, err := gpu.GetClusterGpuAllocationRate(allocationCtx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		allocationSpan.RecordError(err)
		allocationSpan.SetAttributes(attribute.String("error.message", err.Error()))
		allocationSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(allocationSpan)

		log.Errorf("Failed to get cluster GPU allocation rate: %v", err)
		span.SetStatus(codes.Error, "Failed to get GPU allocation rate")
		return stats, err
	}

	duration = time.Since(step5Start)
	cache.AllocationRate = allocationRate

	allocationSpan.SetAttributes(
		attribute.Float64("gpu.allocation_rate", allocationRate),
		attribute.Float64("duration_ms", float64(duration.Milliseconds())),
	)
	allocationSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(allocationSpan)

	// 6. Get storage statistics
	storageSpan, storageCtx := trace.StartSpanFromContext(ctx, "getStorageStatistics")

	step6Start := time.Now()
	storageStat, err := storage.GetStorageStatWithClientSet(storageCtx, storageClientSet)
	if err != nil {
		storageSpan.RecordError(err)
		storageSpan.SetAttributes(attribute.String("error.message", err.Error()))
		storageSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(storageSpan)

		log.Errorf("Failed to get storage statistics: %v", err)
		// Don't fail the entire job for this error
		storageStat = &coreModel.StorageStat{}
	} else {
		duration = time.Since(step6Start)
		storageSpan.SetAttributes(
			attribute.Float64("storage.usage_percentage", storageStat.UsagePercentage),
			attribute.Float64("storage.inodes_usage_percentage", storageStat.InodesUsagePercentage),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		storageSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(storageSpan)
	}

	cache.StorageTotalSpace = storageStat.TotalSpace
	cache.StorageUsedSpace = storageStat.UsedSpace
	cache.StorageUsagePercentage = storageStat.UsagePercentage
	cache.StorageTotalInodes = storageStat.TotalInodes
	cache.StorageUsedInodes = storageStat.UsedInodes
	cache.StorageInodesUsagePercentage = storageStat.InodesUsagePercentage
	cache.StorageReadBandwidth = storageStat.ReadBandwidth
	cache.StorageWriteBandwidth = storageStat.WriteBandwidth

	// 7. Get RDMA cluster statistics
	rdmaSpan, rdmaCtx := trace.StartSpanFromContext(ctx, "getRdmaClusterStatistics")

	step7Start := time.Now()
	rdmaStat, err := rdma.GetRdmaClusterStat(rdmaCtx, storageClientSet)
	if err != nil {
		rdmaSpan.RecordError(err)
		rdmaSpan.SetAttributes(attribute.String("error.message", err.Error()))
		rdmaSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(rdmaSpan)

		log.Errorf("Failed to get RDMA cluster statistics: %v", err)
		// Don't fail the entire job for this error
		rdmaStat = coreModel.RdmaClusterStat{}
	} else {
		duration = time.Since(step7Start)
		rdmaSpan.SetAttributes(
			attribute.Int64("rdma.total_tx", int64(rdmaStat.TotalTx)),
			attribute.Int64("rdma.total_rx", int64(rdmaStat.TotalRx)),
			attribute.Float64("duration_ms", float64(duration.Milliseconds())),
		)
		rdmaSpan.SetStatus(codes.Ok, "")
		trace.FinishSpan(rdmaSpan)
	}

	cache.RdmaTotalTx = rdmaStat.TotalTx
	cache.RdmaTotalRx = rdmaStat.TotalRx

	// 8. Save to database (upsert logic)
	saveSpan, saveCtx := trace.StartSpanFromContext(ctx, "saveClusterOverviewCache")
	saveSpan.SetAttributes(attribute.String("cluster.name", clusterName))

	saveStart := time.Now()
	facade := database.GetFacade().GetClusterOverviewCache()

	existingCache, err := facade.GetClusterOverviewCache(saveCtx)
	if err != nil {
		saveSpan.RecordError(err)
		saveSpan.SetAttributes(attribute.String("error.message", err.Error()))
		saveSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(saveSpan)

		log.Errorf("Failed to check existing cluster overview cache: %v", err)
		span.SetStatus(codes.Error, "Failed to check existing cache")
		return stats, err
	}

	if existingCache != nil {
		cache.ID = existingCache.ID
		cache.CreatedAt = existingCache.CreatedAt
		err = facade.UpdateClusterOverviewCache(saveCtx, cache)
		stats.ItemsUpdated = 1
		saveSpan.SetAttributes(
			attribute.String("operation", "update"),
			attribute.Int64("cache.id", int64(existingCache.ID)),
		)
	} else {
		err = facade.CreateClusterOverviewCache(saveCtx, cache)
		stats.ItemsCreated = 1
		saveSpan.SetAttributes(attribute.String("operation", "create"))
	}

	if err != nil {
		saveSpan.RecordError(err)
		saveSpan.SetAttributes(attribute.String("error.message", err.Error()))
		saveSpan.SetStatus(codes.Error, err.Error())
		trace.FinishSpan(saveSpan)

		log.Errorf("Failed to save cluster overview cache: %v", err)
		span.SetStatus(codes.Error, "Failed to save cache")
		return stats, err
	}

	duration = time.Since(saveStart)
	stats.SaveDuration = duration.Seconds()
	saveSpan.SetAttributes(attribute.Float64("duration_ms", float64(duration.Milliseconds())))
	saveSpan.SetStatus(codes.Ok, "")
	trace.FinishSpan(saveSpan)

	stats.RecordsProcessed = 1
	stats.AddCustomMetric("total_nodes", len(gpuNodes))
	stats.AddCustomMetric("healthy_nodes", cache.HealthyNodes)
	stats.AddCustomMetric("faulty_nodes", cache.FaultyNodes)
	stats.AddMessage("Cluster overview cache updated successfully")

	// Record total job duration
	totalDuration := time.Since(jobStartTime)
	span.SetAttributes(attribute.Float64("total_duration_ms", float64(totalDuration.Milliseconds())))
	span.SetStatus(codes.Ok, "")
	return stats, nil
}

// getFaultyNodesFromDB gets faulty nodes from database based on taints and K8SStatus
func (j *ClusterOverviewJob) getFaultyNodesFromDB(ctx context.Context, nodeNames []string) ([]string, error) {
	span, ctx := trace.StartSpanFromContext(ctx, "getFaultyNodesFromDB.query")
	defer trace.FinishSpan(span)

	span.SetAttributes(attribute.Int("nodes.input_count", len(nodeNames)))

	if len(nodeNames) == 0 {
		span.SetStatus(codes.Ok, "No nodes to check")
		return []string{}, nil
	}

	nodeFacade := database.GetFacade().GetNode()
	faultyNodes := []string{}
	queryErrorCount := 0
	notFoundCount := 0

	for _, nodeName := range nodeNames {
		dbNode, err := nodeFacade.GetNodeByName(ctx, nodeName)
		if err != nil {
			queryErrorCount++
			log.Errorf("Failed to get node %s from database: %v", nodeName, err)
			continue
		}

		if dbNode == nil {
			notFoundCount++
			continue
		}

		// Check if node has taints
		hasTaints := false
		if dbNode.Taints != nil && len(dbNode.Taints) > 0 {
			if taintsList, ok := dbNode.Taints["taints"]; ok {
				if taints, ok := taintsList.([]interface{}); ok && len(taints) > 0 {
					hasTaints = true
				}
			}
		}

		// Check if node K8SStatus is not Ready
		isNotReady := dbNode.K8sStatus != k8sUtil.NodeStatusReady

		// Node is faulty if it has taints or is not ready
		if hasTaints || isNotReady {
			faultyNodes = append(faultyNodes, nodeName)
		}
	}

	span.SetAttributes(
		attribute.Int("nodes.faulty_count", len(faultyNodes)),
		attribute.Int("nodes.query_error_count", queryErrorCount),
		attribute.Int("nodes.not_found_count", notFoundCount),
	)
	span.SetStatus(codes.Ok, "")

	return faultyNodes, nil
}

// Schedule returns the cron schedule for this job
func (j *ClusterOverviewJob) Schedule() string {
	// Run every 30 seconds - can be adjusted based on cluster size and requirements
	return "@every 30s"
}
