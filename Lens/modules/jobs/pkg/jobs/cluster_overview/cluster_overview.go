package cluster_overview

import (
	"context"
	"time"

	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/clientsets"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database"
	dbmodel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/database/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/fault"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/gpu"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/metadata"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/rdma"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/helper/storage"
	"github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/logger/log"
	coreModel "github.com/AMD-AGI/Primus-SaFE/Lens/core/pkg/model"
	"github.com/AMD-AGI/Primus-SaFE/Lens/jobs/pkg/common"
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
	log.Infof("[Step 1/7] Getting GPU nodes for cluster: %s", clusterName)
	queryStart := time.Now()
	gpuNodes, err := gpu.GetGpuNodes(ctx, clientSets, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get GPU nodes: %v", err)
		return stats, err
	}
	stats.QueryDuration += time.Since(queryStart).Seconds()
	cache.TotalNodes = int32(len(gpuNodes))
	log.Infof("[Step 1/7] Got %d GPU nodes, took: %v", len(gpuNodes), time.Since(queryStart))

	// 2. Get faulty nodes
	log.Infof("[Step 2/7] Checking faulty nodes")
	step2Start := time.Now()
	faultyNodes, err := fault.GetFaultyNodes(ctx, clientSets, gpuNodes)
	if err != nil {
		log.Errorf("Failed to get faulty nodes: %v", err)
		return stats, err
	}
	cache.FaultyNodes = int32(len(faultyNodes))
	cache.HealthyNodes = cache.TotalNodes - cache.FaultyNodes
	log.Infof("[Step 2/7] Found %d faulty nodes, %d healthy nodes, took: %v", len(faultyNodes), cache.HealthyNodes, time.Since(step2Start))

	// 3. Get GPU node idle info
	log.Infof("[Step 3/7] Getting GPU node idle info")
	step3Start := time.Now()
	idle, partialIdle, busy, err := gpu.GetGpuNodeIdleInfo(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get GPU node idle info: %v", err)
		return stats, err
	}
	cache.FullyIdleNodes = int32(idle)
	cache.PartiallyIdleNodes = int32(partialIdle)
	cache.BusyNodes = int32(busy)
	log.Infof("[Step 3/7] Idle nodes: %d fully idle, %d partially idle, %d busy, took: %v", idle, partialIdle, busy, time.Since(step3Start))

	// 4. Calculate GPU usage
	log.Infof("[Step 4/7] Calculating GPU usage")
	step4Start := time.Now()
	usage, err := gpu.CalculateGpuUsage(ctx, storageClientSet, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to calculate GPU usage: %v", err)
		// Don't fail the entire job for this error, just log it
		usage = 0
	}
	cache.Utilization = usage
	log.Infof("[Step 4/7] GPU utilization: %.2f%%, took: %v", usage, time.Since(step4Start))

	// 5. Get cluster GPU allocation rate
	log.Infof("[Step 5/7] Getting cluster GPU allocation rate")
	step5Start := time.Now()
	allocationRate, err := gpu.GetClusterGpuAllocationRate(ctx, clientSets, clusterName, metadata.GpuVendorAMD)
	if err != nil {
		log.Errorf("Failed to get cluster GPU allocation rate: %v", err)
		return stats, err
	}
	cache.AllocationRate = allocationRate
	log.Infof("[Step 5/7] GPU allocation rate: %.2f%%, took: %v", allocationRate, time.Since(step5Start))

	// 6. Get storage statistics
	log.Infof("[Step 6/7] Getting storage statistics")
	step6Start := time.Now()
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
	log.Infof("[Step 6/7] Storage stats: usage %.2f%%, inodes %.2f%%, took: %v", storageStat.UsagePercentage, storageStat.InodesUsagePercentage, time.Since(step6Start))

	// 7. Get RDMA cluster statistics
	log.Infof("[Step 7/7] Getting RDMA cluster statistics")
	step7Start := time.Now()
	rdmaStat, err := rdma.GetRdmaClusterStat(ctx, storageClientSet)
	if err != nil {
		log.Errorf("Failed to get RDMA cluster statistics: %v", err)
		// Don't fail the entire job for this error, just log it
		rdmaStat = coreModel.RdmaClusterStat{}
	}
	cache.RdmaTotalTx = rdmaStat.TotalTx
	cache.RdmaTotalRx = rdmaStat.TotalRx
	log.Infof("[Step 7/7] RDMA stats: Tx=%d, Rx=%d, took: %v", rdmaStat.TotalTx, rdmaStat.TotalRx, time.Since(step7Start))

	// 8. Save to database (upsert logic)
	log.Infof("[Database] Starting to save cluster overview cache to database")
	saveStart := time.Now()
	facade := database.GetFacade().GetClusterOverviewCache()
	
	log.Infof("[Database] Checking for existing cache record")
	existingCache, err := facade.GetClusterOverviewCache(ctx)
	if err != nil {
		log.Errorf("Failed to check existing cluster overview cache: %v", err)
		return stats, err
	}

	if existingCache != nil {
		// Update existing record
		log.Infof("[Database] Updating existing cache record (ID: %d)", existingCache.ID)
		cache.ID = existingCache.ID
		cache.CreatedAt = existingCache.CreatedAt
		err = facade.UpdateClusterOverviewCache(ctx, cache)
		stats.ItemsUpdated = 1
	} else {
		// Create new record
		log.Infof("[Database] Creating new cache record")
		err = facade.CreateClusterOverviewCache(ctx, cache)
		stats.ItemsCreated = 1
	}

	if err != nil {
		log.Errorf("Failed to save cluster overview cache: %v", err)
		return stats, err
	}
	stats.SaveDuration = time.Since(saveStart).Seconds()
	log.Infof("[Database] Successfully saved cache to database, took: %v", time.Since(saveStart))

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
