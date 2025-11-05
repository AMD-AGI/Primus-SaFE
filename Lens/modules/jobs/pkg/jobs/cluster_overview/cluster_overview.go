package cluster_overview

import (
	"context"
	"time"

	"github.com/AMD-AGI/primus-lens/core/pkg/clientsets"
	"github.com/AMD-AGI/primus-lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/primus-lens/core/pkg/database/model"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/fault"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/rdma"
	"github.com/AMD-AGI/primus-lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/primus-lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/primus-lens/core/pkg/model"
	"github.com/AMD-AGI/primus-lens/jobs/pkg/common"
)

type ClusterOverviewJob struct {
}

// Run executes the cluster overview caching job
func (j *ClusterOverviewJob) Run(ctx context.Context, clientSets *clientsets.K8SClientSet, storageClientSet *clientsets.StorageClientSet) (*common.ExecutionStats, error) {
	stats := common.NewExecutionStats()

	// Use current cluster name for job running in current cluster
	clusterName := clientsets.GetClusterManager().GetCurrentClusterName()

	log.Infof("Starting cluster overview cache job for cluster: %s", clusterName)
	startTime := time.Now()

	// Initialize cache object
	cache := &dbmodel.ClusterOverviewCache{
		ClusterName: clusterName,
	}

	// 1. Get GPU nodes
	queryStart := time.Now()
	gpuNodes, err := gpu.GetGpuNodes(ctx, clientSets, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get GPU nodes: %v", err)
		return stats, err
	}
	stats.QueryDuration += time.Since(queryStart).Seconds()
	cache.TotalNodes = int32(len(gpuNodes))

	// 2. Get faulty nodes
	faultyNodes, err := fault.GetFaultyNodes(ctx, clientSets, gpuNodes)
	if err != nil {
		log.Errorf("Failed to get faulty nodes: %v", err)
		return stats, err
	}
	cache.FaultyNodes = int32(len(faultyNodes))
	cache.HealthyNodes = cache.TotalNodes - cache.FaultyNodes

	// 3. Get GPU node idle info
	idle, partialIdle, busy, err := gpu.GetGpuNodeIdleInfo(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get GPU node idle info: %v", err)
		return stats, err
	}
	cache.FullyIdleNodes = int32(idle)
	cache.PartiallyIdleNodes = int32(partialIdle)
	cache.BusyNodes = int32(busy)

	// 4. Calculate GPU usage
	usage, err := gpu.CalculateGpuUsage(ctx, storageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to calculate GPU usage: %v", err)
		// Don't fail the entire job for this error, just log it
		usage = 0
	}
	cache.Utilization = usage

	// 5. Get cluster GPU allocation rate
	allocationRate, err := gpu.GetClusterGpuAllocationRate(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get cluster GPU allocation rate: %v", err)
		return stats, err
	}
	cache.AllocationRate = allocationRate

	// 6. Get storage statistics
	storageStat, err := storage.GetStorageStatWithClientSet(ctx, storageClientSet)
	if err != nil {
		log.Errorf("Failed to get storage statistics: %v", err)
		// Don't fail the entire job for this error, just log it
		storageStat = &coreModel.StorageStat{}
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
	rdmaStat, err := rdma.GetRdmaClusterStat(ctx, storageClientSet)
	if err != nil {
		log.Errorf("Failed to get RDMA cluster statistics: %v", err)
		// Don't fail the entire job for this error, just log it
		rdmaStat = coreModel.RdmaClusterStat{}
	}
	cache.RdmaTotalTx = rdmaStat.TotalTx
	cache.RdmaTotalRx = rdmaStat.TotalRx

	// 8. Save to database (upsert logic)
	saveStart := time.Now()
	facade := database.GetFacade().GetClusterOverviewCache()
	existingCache, err := facade.GetClusterOverviewCache(ctx)
	if err != nil {
		log.Errorf("Failed to check existing cluster overview cache: %v", err)
		return stats, err
	}

	if existingCache != nil {
		// Update existing record
		cache.ID = existingCache.ID
		cache.CreatedAt = existingCache.CreatedAt
		err = facade.UpdateClusterOverviewCache(ctx, cache)
		stats.ItemsUpdated = 1
	} else {
		// Create new record
		err = facade.CreateClusterOverviewCache(ctx, cache)
		stats.ItemsCreated = 1
	}

	if err != nil {
		log.Errorf("Failed to save cluster overview cache: %v", err)
		return stats, err
	}
	stats.SaveDuration = time.Since(saveStart).Seconds()

	duration := time.Since(startTime)
	stats.RecordsProcessed = 1
	stats.AddCustomMetric("total_nodes", len(gpuNodes))
	stats.AddCustomMetric("healthy_nodes", cache.HealthyNodes)
	stats.AddCustomMetric("faulty_nodes", cache.FaultyNodes)
	stats.AddMessage("Cluster overview cache updated successfully")

	log.Infof("Cluster overview cache job completed successfully for cluster: %s, took: %v", clusterName, duration)

	return stats, nil
}

// Schedule returns the cron schedule for this job
func (j *ClusterOverviewJob) Schedule() string {
	// Run every 30 seconds - can be adjusted based on cluster size and requirements
	return "@every 30s"
}
